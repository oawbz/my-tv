package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPlaybackReusesGoodSourceAndRepairsChangedM3U(t *testing.T) {
	var changed atomic.Bool
	var playlistLoads atomic.Int32
	var oldRequests atomic.Int32
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/channels.m3u":
			playlistLoads.Add(1)
			stream := "/old.m3u8"
			if changed.Load() {
				stream = "/new.m3u8"
			}
			io.WriteString(w, "#EXTM3U\n#EXTINF:-1,CCTV1综合\n"+upstream.URL+stream+"\n")
		case "/old.m3u8":
			oldRequests.Add(1)
			if changed.Load() {
				http.Error(w, "gone", 404)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			io.WriteString(w, "#EXTM3U\nold.ts\n")
		case "/new.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			io.WriteString(w, "#EXTM3U\nnew.ts\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	g := &gateway{sources: []string{upstream.URL + "/channels.m3u"}, cacheTTL: 3 * time.Hour, client: &http.Client{Timeout: time.Second}, probeClient: &http.Client{Timeout: time.Second}, byKey: map[string]*channel{}, refs: map[string]reference{}}
	server := httptest.NewServer(g.router())
	defer server.Close()
	response, e := http.Get(server.URL + "/channels.json")
	if e != nil {
		t.Fatal(e)
	}
	var doc struct {
		Channels []apiChannel `json:"channels"`
	}
	if e = json.NewDecoder(response.Body).Decode(&doc); e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if len(doc.Channels) != 1 || playlistLoads.Load() != 1 {
		t.Fatalf("initial channels=%d loads=%d", len(doc.Channels), playlistLoads.Load())
	}
	playURL := doc.Channels[0].URLs[0]
	play := func() string {
		t.Helper()
		r, e := http.Get(playURL)
		if e != nil {
			t.Fatal(e)
		}
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		if r.StatusCode != 200 {
			t.Fatalf("play %d: %s", r.StatusCode, b)
		}
		return string(b)
	}
	play()
	first := oldRequests.Load()
	if first != 1 {
		t.Fatalf("single source should be fetched once: %d", first)
	}
	time.Sleep(600 * time.Millisecond)
	play()
	if oldRequests.Load() > first+1 {
		t.Fatal("second playback made redundant requests")
	}
	if playlistLoads.Load() != 1 {
		t.Fatal("playlist refreshed before TTL")
	}
	changed.Store(true)
	time.Sleep(600 * time.Millisecond)
	result := play()
	if !strings.Contains(result, "/resource/") {
		t.Fatal(result)
	}
	if playlistLoads.Load() != 2 {
		t.Fatalf("did not repair M3U: %d", playlistLoads.Load())
	}
	response, e = http.Get(server.URL + "/channels.json")
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	doc.Channels = nil
	json.NewDecoder(response.Body).Decode(&doc)
	if len(doc.Channels) != 1 || doc.Channels[0].URLs[0] != playURL {
		t.Fatal("gateway playback URL changed")
	}
}
