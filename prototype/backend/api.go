package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"
)

type apiServer struct {
	mtxAPIBase string // e.g. http://mediamtx:9997/v3
	tracker    *fleetTracker
	client     *http.Client

	cacheMu     sync.Mutex
	cachedFleet *fleetSummary
	cachedPaths []mtxPath
	cachedAt    time.Time
	refreshing  bool
	refreshDone chan struct{}
	lastErr     error
}

const fleetCacheTTL = 2 * time.Second

func newAPIServer(mtxAPIBase string) *apiServer {
	return &apiServer{
		mtxAPIBase: mtxAPIBase,
		tracker:    newFleetTracker(),
		client:     &http.Client{Timeout: 5 * time.Second},
	}
}

type mtxPathList struct {
	PageCount int       `json:"pageCount"`
	Items     []mtxPath `json:"items"`
}

// fetchAllPaths pages through MediaMTX /paths/list.
func (a *apiServer) fetchAllPaths() ([]mtxPath, error) {
	var all []mtxPath
	for page := 0; ; page++ {
		url := fmt.Sprintf("%s/paths/list?itemsPerPage=500&page=%d", a.mtxAPIBase, page)
		resp, err := a.client.Get(url)
		if err != nil {
			return nil, err
		}
		// Issue 2: check HTTP status before decoding.
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("mediamtx returned HTTP %d for %s", resp.StatusCode, url)
		}
		var list mtxPathList
		err = json.NewDecoder(resp.Body).Decode(&list)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		all = append(all, list.Items...)
		// Issue 3: guard against pageCount == 0 to avoid integer underflow.
		if list.PageCount == 0 || page >= list.PageCount-1 {
			break
		}
	}
	return all, nil
}

// snapshot returns cached paths+summary, refreshing from MediaMTX when stale.
// Issue 1: stampede-safe — lock is NOT held across HTTP fetches.
func (a *apiServer) snapshot() (*fleetSummary, []mtxPath, error) {
	a.cacheMu.Lock()

	// Cache is fresh — return immediately.
	if a.cachedFleet != nil && time.Since(a.cachedAt) < fleetCacheTTL {
		fleet, paths := a.cachedFleet, a.cachedPaths
		a.cacheMu.Unlock()
		return fleet, paths, nil
	}

	// Another goroutine is already refreshing — wait for it.
	if a.refreshing {
		done := a.refreshDone
		a.cacheMu.Unlock()
		<-done
		a.cacheMu.Lock()
		fleet, paths, err := a.cachedFleet, a.cachedPaths, a.lastErr
		a.cacheMu.Unlock()
		if fleet == nil && err == nil {
			err = fmt.Errorf("cache unavailable after refresh")
		}
		return fleet, paths, err
	}

	// We are the designated refresher.
	a.refreshing = true
	a.refreshDone = make(chan struct{})
	done := a.refreshDone
	a.cacheMu.Unlock()

	// Fetch and build WITHOUT holding the lock.
	paths, fetchErr := a.fetchAllPaths()
	var summary fleetSummary
	if fetchErr == nil {
		summary = a.tracker.build(paths, time.Now())
	}

	// Write results back under the lock.
	a.cacheMu.Lock()
	if fetchErr == nil {
		a.cachedFleet = &summary
		a.cachedPaths = paths
		a.cachedAt = time.Now()
		a.lastErr = nil
	} else {
		a.lastErr = fetchErr
	}
	a.refreshing = false
	a.cacheMu.Unlock()

	// Wake all waiters.
	close(done)

	if fetchErr != nil {
		return nil, nil, fetchErr
	}
	return a.cachedFleet, a.cachedPaths, nil
}

func (a *apiServer) handleFleet(w http.ResponseWriter, _ *http.Request) {
	summary, _, err := a.snapshot()
	if err != nil {
		log.Printf("fleet: mediamtx api error: %v", err)
		http.Error(w, "mediamtx unavailable", http.StatusBadGateway)
		return
	}
	writeJSON(w, summary)
}

type camDetail struct {
	Cam           int      `json:"cam"`
	Path          string   `json:"path"`
	Ready         bool     `json:"ready"`
	Tracks        []string `json:"tracks"`
	BytesReceived uint64   `json:"bytesReceived"`
	Readers       int      `json:"readers"`
}

type busDetail struct {
	ID   string      `json:"id"`
	Cams []camDetail `json:"cams"`
}

func (a *apiServer) handleBusDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, paths, err := a.snapshot()
	if err != nil {
		log.Printf("bus detail: mediamtx api error: %v", err)
		http.Error(w, "mediamtx unavailable", http.StatusBadGateway)
		return
	}
	detail := busDetail{ID: id, Cams: []camDetail{}}
	for _, p := range paths {
		busID, cam, ok := parseBusPath(p.Name)
		if !ok || busID != id {
			continue
		}
		// Issue 4: normalize nil tracks to []string{} so JSON encodes [] not null.
		tracks := p.Tracks
		if tracks == nil {
			tracks = []string{}
		}
		detail.Cams = append(detail.Cams, camDetail{
			Cam:           cam,
			Path:          p.Name,
			Ready:         p.Ready,
			Tracks:        tracks,
			BytesReceived: p.BytesReceived,
			Readers:       len(p.Readers),
		})
	}
	writeJSON(w, detail)
}

// camsForBus filters paths to those belonging to busId, sorted by cam number.
func camsForBus(paths []mtxPath, busID string) []mtxPath {
	var out []mtxPath
	for _, p := range paths {
		id, _, ok := parseBusPath(p.Name)
		if !ok || id != busID {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		_, ci, _ := parseBusPath(out[i].Name)
		_, cj, _ := parseBusPath(out[j].Name)
		return ci < cj
	})
	return out
}

type streamInfo struct {
	Cam     int    `json:"cam"`
	Path    string `json:"path"`
	Ready   bool   `json:"ready"`
	WhepURL string `json:"whepUrl"`
	HLSURL  string `json:"hlsUrl"`
}

func (a *apiServer) handleStreamLive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	camFilter := r.URL.Query().Get("cam")

	_, paths, err := a.snapshot()
	if err != nil {
		log.Printf("stream live: mediamtx api error: %v", err)
		http.Error(w, "mediamtx unavailable", http.StatusBadGateway)
		return
	}

	result := []streamInfo{}
	for _, p := range camsForBus(paths, id) {
		_, cam, _ := parseBusPath(p.Name)
		if camFilter != "" {
			wantCam, err := strconv.Atoi(camFilter)
			if err != nil || cam != wantCam {
				continue
			}
		}
		result = append(result, streamInfo{
			Cam:     cam,
			Path:    p.Name,
			Ready:   p.Ready,
			WhepURL: "/whep/" + p.Name + "/whep",
			HLSURL:  "/live/" + p.Name + "/index.m3u8",
		})
	}
	writeJSON(w, result)
}

type recordingInfo struct {
	Cam  int    `json:"cam"`
	Path string `json:"path"`
	URL  string `json:"url"`
}

func (a *apiServer) handleStreamRecording(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	camFilter := r.URL.Query().Get("cam")

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	if fromStr == "" || toStr == "" {
		http.Error(w, "from and to query params are required", http.StatusBadRequest)
		return
	}
	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		http.Error(w, "invalid from: "+err.Error(), http.StatusBadRequest)
		return
	}
	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		http.Error(w, "invalid to: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !to.After(from) {
		http.Error(w, "to must be after from", http.StatusBadRequest)
		return
	}
	duration := to.Sub(from).Seconds()

	_, paths, err := a.snapshot()
	if err != nil {
		log.Printf("stream recording: mediamtx api error: %v", err)
		http.Error(w, "mediamtx unavailable", http.StatusBadGateway)
		return
	}

	result := []recordingInfo{}
	for _, p := range camsForBus(paths, id) {
		_, cam, _ := parseBusPath(p.Name)
		if camFilter != "" {
			wantCam, err := strconv.Atoi(camFilter)
			if err != nil || cam != wantCam {
				continue
			}
		}
		q := url.Values{}
		q.Set("path", p.Name)
		q.Set("start", from.Format(time.RFC3339))
		q.Set("duration", strconv.FormatFloat(duration, 'f', -1, 64))
		q.Set("format", "mp4")
		result = append(result, recordingInfo{
			Cam:  cam,
			Path: p.Name,
			URL:  "/playback/get?" + q.Encode(),
		})
	}
	writeJSON(w, result)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: %v", err)
	}
}
