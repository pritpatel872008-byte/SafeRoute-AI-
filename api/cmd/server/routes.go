package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type routeRequest struct {
	Origin string `json:"origin"`
	Destination string `json:"destination"`
	Vehicle string `json:"vehicle"`
}

type geoFeature struct {
	Geometry struct { Coordinates []float64 `json:"coordinates"` } `json:"geometry"`
	PlaceName string `json:"place_name"`
}

type geocodeResponse struct { Features []geoFeature `json:"features"` }

func (s *server) maptilerKey() string { return strings.TrimSpace(env("MAPTILER_API_KEY", "")) }

func (s *server) geocode(w http.ResponseWriter, r *http.Request) {
	key := s.maptilerKey()
	if key == "" { writeJSON(w,http.StatusServiceUnavailable,map[string]string{"error":"routing_provider_not_configured"}); return }
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q)<2 || len(q)>250 { writeJSON(w,http.StatusBadRequest,map[string]string{"error":"query_must_be_2_to_250_characters"}); return }
	ctx,cancel:=context.WithTimeout(r.Context(),8*time.Second); defer cancel()
	u:="https://api.maptiler.com/geocoding/"+url.PathEscape(q)+".json?key="+url.QueryEscape(key)+"&limit=5&language=en"
	req,err:=http.NewRequestWithContext(ctx,http.MethodGet,u,nil)
	if err!=nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"geocoding_request_failed"}); return }
	resp,err:=http.DefaultClient.Do(req)
	if err!=nil { writeJSON(w,http.StatusBadGateway,map[string]string{"error":"geocoding_provider_unreachable"}); return }
	defer resp.Body.Close()
	if resp.StatusCode!=http.StatusOK { writeJSON(w,http.StatusBadGateway,map[string]string{"error":"geocoding_provider_rejected_request","provider_status":strconv.Itoa(resp.StatusCode)}); return }
	var result geocodeResponse
	if err:=json.NewDecoder(io.LimitReader(resp.Body,2<<20)).Decode(&result);err!=nil { writeJSON(w,http.StatusBadGateway,map[string]string{"error":"invalid_geocoding_response"});return }
	features:=make([]map[string]any,0,len(result.Features))
	for _,f:=range result.Features { if len(f.Geometry.Coordinates)>=2 { features=append(features,map[string]any{"label":f.PlaceName,"longitude":f.Geometry.Coordinates[0],"latitude":f.Geometry.Coordinates[1]}) } }
	writeJSON(w,http.StatusOK,map[string]any{"features":features})
}

type directionsResponse struct {
	Routes []struct {
		Distance float64 `json:"distance"`
		Duration float64 `json:"duration"`
		Geometry struct { Type string `json:"type"`; Coordinates [][]float64 `json:"coordinates"` } `json:"geometry"`
		Legs []struct { Steps []struct { Name string `json:"name"`; Distance float64 `json:"distance"`; Duration float64 `json:"duration"`; Maneuver struct { Type string `json:"type"`; Modifier string `json:"modifier"`; Location []float64 `json:"location"` } `json:"maneuver"` } `json:"steps"` } `json:"legs"`
	} `json:"routes"`
}

func parseCoordinate(value string) (float64,float64,bool) {
	parts:=strings.Split(strings.TrimSpace(value),",")
	if len(parts)!=2 { return 0,0,false }
	lat,err1:=strconv.ParseFloat(strings.TrimSpace(parts[0]),64)
	lon,err2:=strconv.ParseFloat(strings.TrimSpace(parts[1]),64)
	if err1!=nil || err2!=nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 { return 0,0,false }
	return lat,lon,true
}

func (s *server) directions(w http.ResponseWriter,r *http.Request) {
	key:=s.maptilerKey()
	if key=="" { writeJSON(w,http.StatusServiceUnavailable,map[string]string{"error":"routing_provider_not_configured"});return }
	var input routeRequest
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,16<<10)).Decode(&input);err!=nil { writeJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid_json"});return }
	input.Origin=strings.TrimSpace(input.Origin);input.Destination=strings.TrimSpace(input.Destination)
	switch input.Vehicle { case "car","motorcycle","truck","walking": default: writeJSON(w,http.StatusBadRequest,map[string]string{"error":"unsupported_vehicle"});return }
	if input.Vehicle=="truck" { writeJSON(w,http.StatusUnprocessableEntity,map[string]string{"error":"truck_specific_routing_not_supported","detail":"The configured provider does not expose a verified truck routing profile. Use a dedicated truck routing provider before relying on height, weight, axle or restricted-road suitability."});return }
	olat,olon,ok1:=parseCoordinate(input.Origin); dlat,dlon,ok2:=parseCoordinate(input.Destination)
	if !ok1 || !ok2 { writeJSON(w,http.StatusBadRequest,map[string]string{"error":"select_geocoded_origin_and_destination","detail":"Choose locations from the geocoding suggestions so exact coordinates are used."});return }
	profile:="driving"
	if input.Vehicle=="walking" { profile="walking" }
	if input.Vehicle=="motorcycle" { profile="driving" }
	coords:=fmt.Sprintf("%.7f,%.7f;%.7f,%.7f",olon,olat,dlon,dlat)
	ctx,cancel:=context.WithTimeout(r.Context(),15*time.Second);defer cancel()
	u:="https://api.maptiler.com/directions/"+profile+"/"+coords+".json?key="+url.QueryEscape(key)+"&overview=full&geometries=geojson&steps=true"
	req,err:=http.NewRequestWithContext(ctx,http.MethodGet,u,nil)
	if err!=nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"directions_request_failed"});return }
	resp,err:=http.DefaultClient.Do(req)
	if err!=nil { writeJSON(w,http.StatusBadGateway,map[string]string{"error":"directions_provider_unreachable"});return }
	defer resp.Body.Close()
	if resp.StatusCode!=http.StatusOK { writeJSON(w,http.StatusBadGateway,map[string]string{"error":"directions_provider_rejected_request","provider_status":strconv.Itoa(resp.StatusCode)});return }
	var result directionsResponse
	if err:=json.NewDecoder(io.LimitReader(resp.Body,8<<20)).Decode(&result);err!=nil { writeJSON(w,http.StatusBadGateway,map[string]string{"error":"invalid_directions_response"});return }
	if len(result.Routes)==0 { writeJSON(w,http.StatusNotFound,map[string]string{"error":"no_route_found"});return }
	route:=result.Routes[0]
	steps:=make([]map[string]any,0)
	for _,leg:=range route.Legs { for _,step:=range leg.Steps { steps=append(steps,map[string]any{"instruction":strings.TrimSpace(strings.Join([]string{step.Maneuver.Type,step.Maneuver.Modifier,step.Name}," ")),"distance_m":step.Distance,"duration_s":step.Duration,"location":step.Maneuver.Location}) } }
	writeJSON(w,http.StatusOK,map[string]any{"provider":"MapTiler","profile":profile,"vehicle":input.Vehicle,"distance_m":route.Distance,"duration_s":route.Duration,"geometry":route.Geometry,"steps":steps,"warnings":[]string{"Route results depend on provider coverage and may not reflect temporary closures or all vehicle restrictions. Follow posted signs."}})
}
