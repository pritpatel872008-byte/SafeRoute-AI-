package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type savedPlaceInput struct {
	Label string `json:"label"`
	Latitude float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func (s *server) listSavedPlaces(w http.ResponseWriter,r *http.Request) {
	userID:=r.Context().Value(userIDKey).(string)
	rows,err:=s.db.Query(r.Context(),`SELECT id,label,latitude,longitude,created_at FROM saved_places WHERE user_id=$1 ORDER BY created_at DESC`,userID)
	if err!=nil { writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"saved_places_query_failed"});return }; defer rows.Close()
	items:=make([]map[string]any,0)
	for rows.Next(){var id,label string;var lat,lon float64;var created time.Time;if err:=rows.Scan(&id,&label,&lat,&lon,&created);err!=nil{writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"saved_places_read_failed"});return};items=append(items,map[string]any{"id":id,"label":label,"latitude":lat,"longitude":lon,"created_at":created})}
	if rows.Err()!=nil{writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"saved_places_read_failed"});return}
	writeJSON(w,http.StatusOK,map[string]any{"places":items})
}
func (s *server) createSavedPlace(w http.ResponseWriter,r *http.Request) {
	userID:=r.Context().Value(userIDKey).(string);var in savedPlaceInput
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,8<<10)).Decode(&in);err!=nil{writeJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid_json"});return}
	in.Label=strings.TrimSpace(in.Label)
	if in.Label==""||len(in.Label)>120||in.Latitude < -90||in.Latitude>90||in.Longitude < -180||in.Longitude>180{writeJSON(w,http.StatusBadRequest,map[string]string{"error":"valid_label_and_coordinates_required"});return}
	var id string;err:=s.db.QueryRow(r.Context(),`INSERT INTO saved_places(user_id,label,latitude,longitude) VALUES($1,$2,$3,$4) RETURNING id`,userID,in.Label,in.Latitude,in.Longitude).Scan(&id)
	if err!=nil{writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"saved_place_create_failed"});return}
	writeJSON(w,http.StatusCreated,map[string]any{"place":map[string]any{"id":id,"label":in.Label,"latitude":in.Latitude,"longitude":in.Longitude}})
}
func (s *server) deleteSavedPlace(w http.ResponseWriter,r *http.Request) {
	userID:=r.Context().Value(userIDKey).(string);tag,err:=s.db.Exec(r.Context(),`DELETE FROM saved_places WHERE id=$1 AND user_id=$2`,r.PathValue("id"),userID)
	if err!=nil{writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"saved_place_delete_failed"});return};if tag.RowsAffected()==0{writeJSON(w,http.StatusNotFound,map[string]string{"error":"saved_place_not_found"});return};w.WriteHeader(http.StatusNoContent)
}

type routeHistoryInput struct {
	Origin map[string]any `json:"origin"`
	Destination map[string]any `json:"destination"`
	Vehicle string `json:"vehicle"`
	Provider string `json:"provider"`
	RouteSummary map[string]any `json:"route_summary"`
}
func (s *server) listRouteHistory(w http.ResponseWriter,r *http.Request) {
	userID:=r.Context().Value(userIDKey).(string);rows,err:=s.db.Query(r.Context(),`SELECT id,origin,destination,vehicle_type,provider,route_summary,created_at FROM route_history WHERE user_id=$1 ORDER BY created_at DESC LIMIT 50`,userID)
	if err!=nil{writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"route_history_query_failed"});return};defer rows.Close()
	items:=make([]map[string]any,0)
	for rows.Next(){var id,vehicle,provider string;var origin,dest,summary []byte;var created time.Time;if err:=rows.Scan(&id,&origin,&dest,&vehicle,&provider,&summary,&created);err!=nil{writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"route_history_read_failed"});return};var o,d,su any;_ = json.Unmarshal(origin,&o);_ = json.Unmarshal(dest,&d);_ = json.Unmarshal(summary,&su);items=append(items,map[string]any{"id":id,"origin":o,"destination":d,"vehicle":vehicle,"provider":provider,"route_summary":su,"created_at":created})}
	if rows.Err()!=nil{writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"route_history_read_failed"});return};writeJSON(w,http.StatusOK,map[string]any{"routes":items})
}
func (s *server) createRouteHistory(w http.ResponseWriter,r *http.Request) {
	userID:=r.Context().Value(userIDKey).(string);var in routeHistoryInput
	if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,64<<10)).Decode(&in);err!=nil{writeJSON(w,http.StatusBadRequest,map[string]string{"error":"invalid_json"});return}
	switch in.Vehicle{case "car","motorcycle","truck","walking":default:writeJSON(w,http.StatusBadRequest,map[string]string{"error":"unsupported_vehicle"});return}
	if in.Origin==nil||in.Destination==nil||in.RouteSummary==nil{writeJSON(w,http.StatusBadRequest,map[string]string{"error":"origin_destination_and_summary_required"});return}
	o,_:=json.Marshal(in.Origin);d,_:=json.Marshal(in.Destination);summary,_:=json.Marshal(in.RouteSummary);provider:=strings.TrimSpace(in.Provider);if provider==""{provider="MapTiler"}
	var id string;err:=s.db.QueryRow(r.Context(),`INSERT INTO route_history(user_id,origin,destination,vehicle_type,provider,route_summary) VALUES($1,$2::jsonb,$3::jsonb,$4,$5,$6::jsonb) RETURNING id`,userID,o,d,in.Vehicle,provider,summary).Scan(&id)
	if err!=nil{writeJSON(w,http.StatusInternalServerError,map[string]string{"error":"route_history_save_failed"});return};writeJSON(w,http.StatusCreated,map[string]string{"id":id})
}
