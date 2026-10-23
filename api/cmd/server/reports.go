package main

import (
  "context"
  "crypto/sha256"
  "encoding/hex"
  "encoding/json"
  "encoding/xml"
  "fmt"
  "io"
  "net/http"
  "net/url"
  "os"
  "strings"
  "time"
)

type safetyReportInput struct {
  Title string `json:"title"`
  Description string `json:"description"`
  Category string `json:"category"`
  Latitude float64 `json:"latitude"`
  Longitude float64 `json:"longitude"`
  OccurredAt *time.Time `json:"occurred_at"`
}
func (s *server) listSafetyReports(w http.ResponseWriter,r *http.Request) {
 if !s.requireDB(w){return}
 rows,err:=s.db.Query(r.Context(),`SELECT id,title,description,category,latitude,longitude,source_url,source_name,status,occurred_at,created_at FROM safety_reports WHERE status='approved' ORDER BY COALESCE(occurred_at,created_at) DESC LIMIT 200`)
 if err!=nil { writeJSON(w,500,map[string]string{"error":"safety_reports_query_failed"});return }; defer rows.Close()
 items:=make([]map[string]any,0)
 for rows.Next(){var id,title,desc,category,sourceName,status string;var lat,lon float64;var sourceURL *string;var occurred *time.Time;var created time.Time
 if err:=rows.Scan(&id,&title,&desc,&category,&lat,&lon,&sourceURL,&sourceName,&status,&occurred,&created);err!=nil{writeJSON(w,500,map[string]string{"error":"safety_reports_read_failed"});return}
 items=append(items,map[string]any{"id":id,"title":title,"description":desc,"category":category,"latitude":lat,"longitude":lon,"source_url":sourceURL,"source_name":sourceName,"status":status,"occurred_at":occurred,"created_at":created})}
 if rows.Err()!=nil{writeJSON(w,500,map[string]string{"error":"safety_reports_read_failed"});return}
 writeJSON(w,200,map[string]any{"reports":items})
}
func validReportCategory(c string)bool{switch c{case "accident","roadblock","hazard","crime","weather","other":return true};return false}
func (s *server) submitSafetyReport(w http.ResponseWriter,r *http.Request){
 if !s.requireDB(w){return}; userID:=r.Context().Value(userIDKey).(string)
 var in safetyReportInput
 if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,16<<10)).Decode(&in);err!=nil{writeJSON(w,400,map[string]string{"error":"invalid_json"});return}
 in.Title=strings.TrimSpace(in.Title);in.Description=strings.TrimSpace(in.Description);in.Category=strings.TrimSpace(in.Category)
 if len(in.Title)<5||len(in.Title)>180||len(in.Description)<10||len(in.Description)>3000||!validReportCategory(in.Category)||in.Latitude < -90||in.Latitude>90||in.Longitude < -180||in.Longitude>180{writeJSON(w,400,map[string]string{"error":"invalid_report_fields"});return}
 var id string
 err:=s.db.QueryRow(r.Context(),`INSERT INTO safety_reports(submitted_by,title,description,category,latitude,longitude,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`,userID,in.Title,in.Description,in.Category,in.Latitude,in.Longitude,in.OccurredAt).Scan(&id)
 if err!=nil{writeJSON(w,500,map[string]string{"error":"safety_report_submit_failed"});return}
 writeJSON(w,201,map[string]any{"report":map[string]any{"id":id,"status":"pending"},"message":"Report submitted for admin review; it is not public until approved."})
}
func (s *server) isAdmin(ctx context.Context,userID string)bool{
 allowed:=strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL")));if allowed==""{return false}
 var email string;err:=s.db.QueryRow(ctx,`SELECT lower(email) FROM app_users WHERE id=$1`,userID).Scan(&email)
 return err==nil&&email==allowed
}
func (s *server) requireAdmin(w http.ResponseWriter,r *http.Request)bool{
 if !s.requireDB(w){return false};id,ok:=r.Context().Value(userIDKey).(string);if !ok||!s.isAdmin(r.Context(),id){writeJSON(w,http.StatusForbidden,map[string]string{"error":"admin_access_required"});return false};return true
}
func (s *server) listPendingSafetyReports(w http.ResponseWriter,r *http.Request){
 if !s.requireAdmin(w,r){return}
 rows,err:=s.db.Query(r.Context(),`SELECT id,title,description,category,latitude,longitude,source_url,source_name,status,created_at FROM safety_reports WHERE status='pending' ORDER BY created_at ASC LIMIT 500`)
 if err!=nil{writeJSON(w,500,map[string]string{"error":"pending_reports_query_failed"});return};defer rows.Close()
 items:=make([]map[string]any,0)
 for rows.Next(){var id,title,desc,cat,sourceName,status string;var lat,lon float64;var sourceURL *string;var created time.Time
 if err:=rows.Scan(&id,&title,&desc,&cat,&lat,&lon,&sourceURL,&sourceName,&status,&created);err!=nil{writeJSON(w,500,map[string]string{"error":"pending_reports_read_failed"});return}
 items=append(items,map[string]any{"id":id,"title":title,"description":desc,"category":cat,"latitude":lat,"longitude":lon,"source_url":sourceURL,"source_name":sourceName,"status":status,"created_at":created})}
 writeJSON(w,200,map[string]any{"reports":items})
}
func (s *server) moderateSafetyReport(w http.ResponseWriter,r *http.Request){
 if !s.requireAdmin(w,r){return}
 var in struct{Status string `json:"status"`}
 if err:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1024)).Decode(&in);err!=nil|| (in.Status!="approved"&&in.Status!="rejected"){writeJSON(w,400,map[string]string{"error":"status_must_be_approved_or_rejected"});return}
 reviewer:=r.Context().Value(userIDKey).(string)
 tag,err:=s.db.Exec(r.Context(),`UPDATE safety_reports SET status=$1,reviewed_at=now(),reviewed_by=$2 WHERE id=$3 AND status='pending'`,in.Status,reviewer,r.PathValue("id"))
 if err!=nil{writeJSON(w,500,map[string]string{"error":"report_moderation_failed"});return};if tag.RowsAffected()==0{writeJSON(w,404,map[string]string{"error":"pending_report_not_found"});return}
 writeJSON(w,200,map[string]string{"status":in.Status})
}
func (s *server) deleteSafetyReport(w http.ResponseWriter,r *http.Request){
 if !s.requireAdmin(w,r){return}
 tag,err:=s.db.Exec(r.Context(),`DELETE FROM safety_reports WHERE id=$1`,r.PathValue("id"))
 if err!=nil{writeJSON(w,500,map[string]string{"error":"report_delete_failed"});return};if tag.RowsAffected()==0{writeJSON(w,404,map[string]string{"error":"report_not_found"});return};w.WriteHeader(http.StatusNoContent)
}

// RSS import only fetches administrator-configured HTTPS feeds. Feed entries are marked pending,
// retain their source link, and are never treated as verified incidents until reviewed.
type rssFeed struct { Channel struct { Items []struct { Title string `xml:"title"`; Link string `xml:"link"`; Description string `xml:"description"`; GUID string `xml:"guid"`; PubDate string `xml:"pubDate"` } `xml:"item"` } `xml:"channel"` }
func (s *server) importSafetyFeeds(w http.ResponseWriter,r *http.Request){
 if !s.requireAdmin(w,r){return}
 raw:=strings.TrimSpace(os.Getenv("SAFETY_RSS_FEEDS"));if raw==""{writeJSON(w,503,map[string]string{"error":"SAFETY_RSS_FEEDS_not_configured","detail":"Set a comma-separated list of trusted public HTTPS RSS feed URLs in the API environment."});return}
 client:=&http.Client{Timeout:8*time.Second,CheckRedirect:func(req *http.Request,via []*http.Request)error{if req.URL.Scheme!="https"{return fmt.Errorf("redirect_to_non_https_blocked")};if len(via)>=3{return fmt.Errorf("too_many_redirects")};return nil}}
 inserted,skipped,failed:=0,0,0
 feeds:=strings.Split(raw,",");if len(feeds)>10{feeds=feeds[:10]}
 for _,feedURL:=range feeds{
  parsed,err:=url.Parse(strings.TrimSpace(feedURL));if err!=nil||parsed.Scheme!="https"||parsed.Host==""{failed++;continue}
  req,err:=http.NewRequestWithContext(r.Context(),http.MethodGet,parsed.String(),nil);if err!=nil{failed++;continue};req.Header.Set("User-Agent","SafeRouteAI-SafetyFeedImporter/1.0")
  resp,err:=client.Do(req);if err!=nil{failed++;continue}
  if resp.StatusCode!=200{resp.Body.Close();failed++;continue}
  body,err:=io.ReadAll(io.LimitReader(resp.Body,2<<20));resp.Body.Close();if err!=nil{failed++;continue}
  var doc rssFeed;if xml.Unmarshal(body,&doc)!=nil{failed++;continue}
  for _,item:=range doc.Channel.Items{
   title:=strings.TrimSpace(item.Title);link:=strings.TrimSpace(item.Link);desc:=strings.TrimSpace(item.Description);external:=strings.TrimSpace(item.GUID);if external==""{external=link}
   linkURL,e:=url.Parse(link);if title==""||len(title)>180||len(desc)<10||len(desc)>3000||e!=nil||linkURL.Scheme!="https"||linkURL.Host==""{skipped++;continue}
   sum:=sha256.Sum256([]byte(parsed.Host+"|"+external));externalID:=hex.EncodeToString(sum[:])
   var occurred any
   if t,e:=time.Parse(time.RFC1123Z,item.PubDate);e==nil{occurred=t}
   tag,e:=s.db.Exec(r.Context(),`INSERT INTO safety_reports(title,description,category,latitude,longitude,source_url,source_name,status,external_id,occurred_at) VALUES($1,$2,'other',0,0,$3,$4,'pending',$5,$6) ON CONFLICT(external_id) DO NOTHING`,title,desc,link,parsed.Host,externalID,occurred)
   if e!=nil{failed++}else if tag.RowsAffected()>0{inserted++}else{skipped++}
  }
 }
 writeJSON(w,200,map[string]any{"inserted":inserted,"skipped":skipped,"failed_feeds":failed,"message":"Imported items remain pending and do not appear as verified incidents until reviewed. RSS items without coordinates are not map-pinned."})
}
