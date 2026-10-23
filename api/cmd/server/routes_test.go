package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseCoordinate(t *testing.T) {
	tests := []struct{name,value string;ok bool}{
		{"valid","21.1702,72.8311",true},
		{"negative","-90,-180",true},
		{"max","90,180",true},
		{"too many parts","21,72,9",false},
		{"not numeric","north,east",false},
		{"latitude out of range","91,72",false},
		{"longitude out of range","21,181",false},
		{"empty","",false},
	}
	for _,tt:=range tests { t.Run(tt.name,func(t *testing.T){_,_,ok:=parseCoordinate(tt.value);if ok!=tt.ok{t.Fatalf("parseCoordinate(%q) ok=%v, want %v",tt.value,ok,tt.ok)}}) }
}

func TestHealthDoesNotRequireDatabase(t *testing.T) {
	s:=&server{}
	rec:=httptest.NewRecorder()
	req:=httptest.NewRequest(http.MethodGet,"/healthz",nil)
	s.health(rec,req)
	if rec.Code!=http.StatusOK { t.Fatalf("status=%d, want %d",rec.Code,http.StatusOK) }
	if rec.Header().Get("Content-Type")!="application/json; charset=utf-8" { t.Fatalf("unexpected content type %q",rec.Header().Get("Content-Type")) }
}

func TestDirectionsRequireConfiguredProvider(t *testing.T) {
	t.Setenv("MAPTILER_API_KEY","")
	s:=&server{}
	rec:=httptest.NewRecorder()
	req:=httptest.NewRequest(http.MethodPost,"/api/v1/routes",nil)
	s.directions(rec,req)
	if rec.Code!=http.StatusServiceUnavailable { t.Fatalf("status=%d, want %d",rec.Code,http.StatusServiceUnavailable) }
}
