package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFleetHandler(t *testing.T) {
	mtx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		if page == "" || page == "0" {
			fmt.Fprint(w, `{"pageCount":2,"items":[{"name":"DL1PC0001_1","ready":true},{"name":"DL1PC0001_2","ready":true}]}`)
		} else {
			fmt.Fprint(w, `{"pageCount":2,"items":[{"name":"DL1PC0002_1","ready":true}]}`)
		}
	}))
	defer mtx.Close()

	api := newAPIServer(mtx.URL)
	req := httptest.NewRequest("GET", "/api/fleet", nil)
	rec := httptest.NewRecorder()
	api.handleFleet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got fleetSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if got.Totals.BusesOnline != 2 || got.Totals.CamsOnline != 3 {
		t.Fatalf("totals = %+v, want 2 buses / 3 cams", got.Totals)
	}
}

// TestFleetHandlerMtxDown verifies that handleFleet returns 502 when MediaMTX
// is unreachable (closed server → connection refused).
func TestFleetHandlerMtxDown(t *testing.T) {
	// Start then immediately close a server so its URL is valid but unreachable.
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := closed.URL
	closed.Close()

	api := newAPIServer(closedURL)
	req := httptest.NewRequest("GET", "/api/fleet", nil)
	rec := httptest.NewRecorder()
	api.handleFleet(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 Bad Gateway", rec.Code)
	}
}

func TestBusDetailHandler(t *testing.T) {
	mtx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"pageCount":1,"items":[
			{"name":"DL1PC0001_1","ready":true,"tracks":["H264"],"bytesReceived":1000},
			{"name":"DL1PC0001_2","ready":true,"tracks":["H264"],"bytesReceived":2000},
			{"name":"DL1PC0002_1","ready":true}
		]}`)
	}))
	defer mtx.Close()

	api := newAPIServer(mtx.URL)
	req := httptest.NewRequest("GET", "/api/bus/DL1PC0001", nil)
	req.SetPathValue("id", "DL1PC0001")
	rec := httptest.NewRecorder()
	api.handleBusDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got busDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if got.ID != "DL1PC0001" || len(got.Cams) != 2 {
		t.Fatalf("detail = %+v, want bus DL1PC0001 with 2 cams", got)
	}
	if got.Cams[0].Path != "DL1PC0001_1" {
		t.Fatalf("cam[0].Path = %q, want DL1PC0001_1", got.Cams[0].Path)
	}
}

func streamTestMtx() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"pageCount":1,"items":[
			{"name":"DL1PC0001_1","ready":true,"tracks":["H264"],"bytesReceived":1000},
			{"name":"DL1PC0001_2","ready":true,"tracks":["H264"],"bytesReceived":2000},
			{"name":"DL1PC0002_1","ready":true}
		]}`)
	}))
}

func TestStreamLiveHandler_AllCams(t *testing.T) {
	mtx := streamTestMtx()
	defer mtx.Close()

	api := newAPIServer(mtx.URL)
	req := httptest.NewRequest("GET", "/api/stream/DL1PC0001", nil)
	req.SetPathValue("id", "DL1PC0001")
	rec := httptest.NewRecorder()
	api.handleStreamLive(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []streamInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Path != "DL1PC0001_1" || got[0].WhepURL != "/whep/DL1PC0001_1/whep" || got[0].HLSURL != "/live/DL1PC0001_1/index.m3u8" {
		t.Fatalf("got[0] = %+v, unexpected", got[0])
	}
}

func TestStreamLiveHandler_SingleCam(t *testing.T) {
	mtx := streamTestMtx()
	defer mtx.Close()

	api := newAPIServer(mtx.URL)
	req := httptest.NewRequest("GET", "/api/stream/DL1PC0001?cam=2", nil)
	req.SetPathValue("id", "DL1PC0001")
	rec := httptest.NewRecorder()
	api.handleStreamLive(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []streamInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if len(got) != 1 || got[0].Path != "DL1PC0001_2" {
		t.Fatalf("got = %+v, want single DL1PC0001_2", got)
	}
}

func TestStreamLiveHandler_CamNotFound(t *testing.T) {
	mtx := streamTestMtx()
	defer mtx.Close()

	api := newAPIServer(mtx.URL)
	req := httptest.NewRequest("GET", "/api/stream/DL1PC0001?cam=9", nil)
	req.SetPathValue("id", "DL1PC0001")
	rec := httptest.NewRecorder()
	api.handleStreamLive(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "[]\n" {
		t.Fatalf("body = %q, want []", rec.Body.String())
	}
}

func TestStreamRecordingHandler_Valid(t *testing.T) {
	mtx := streamTestMtx()
	defer mtx.Close()

	api := newAPIServer(mtx.URL)
	req := httptest.NewRequest("GET", "/api/stream/DL1PC0001/recording?from=2026-01-01T00:00:00Z&to=2026-01-01T00:02:00Z", nil)
	req.SetPathValue("id", "DL1PC0001")
	rec := httptest.NewRecorder()
	api.handleStreamRecording(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var got []recordingInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	for _, r := range got {
		if !strings.Contains(r.URL, "duration=120") {
			t.Fatalf("url %q missing duration=120", r.URL)
		}
	}
	if !strings.Contains(got[0].URL, "path=DL1PC0001_1") && !strings.Contains(got[1].URL, "path=DL1PC0001_1") {
		t.Fatalf("no url contains path=DL1PC0001_1: %+v", got)
	}
	if !strings.Contains(got[0].URL, "path=DL1PC0001_2") && !strings.Contains(got[1].URL, "path=DL1PC0001_2") {
		t.Fatalf("no url contains path=DL1PC0001_2: %+v", got)
	}
}

func TestStreamRecordingHandler_InvalidRange(t *testing.T) {
	mtx := streamTestMtx()
	defer mtx.Close()

	api := newAPIServer(mtx.URL)
	req := httptest.NewRequest("GET", "/api/stream/DL1PC0001/recording?from=2026-01-01T00:02:00Z&to=2026-01-01T00:00:00Z", nil)
	req.SetPathValue("id", "DL1PC0001")
	rec := httptest.NewRecorder()
	api.handleStreamRecording(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestStreamRecordingHandler_MissingParams(t *testing.T) {
	mtx := streamTestMtx()
	defer mtx.Close()

	api := newAPIServer(mtx.URL)
	req := httptest.NewRequest("GET", "/api/stream/DL1PC0001/recording?to=2026-01-01T00:00:00Z", nil)
	req.SetPathValue("id", "DL1PC0001")
	rec := httptest.NewRecorder()
	api.handleStreamRecording(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
