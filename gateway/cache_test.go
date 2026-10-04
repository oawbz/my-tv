package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRemoteM3UOnDemandCacheAndUserAgent(t *testing.T) {
	var fetches atomic.Int32
	var broken atomic.Bool
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != defaultUpstreamUA {
			http.Error(w, "wrong UA", 403)
			return
		}
		switch r.URL.Path {
		case "/channels.m3u":
			fetches.Add(1)
			if broken.Load() {
				http.Error(w, "unavailable", 503)
				return
			}
			io.WriteString(w, "#EXTM3U\n#EXTINF:-1 tvg-logo=\""+upstream.URL+"/logo\",CCTV1综合\n"+upstream.URL+"/stream.m3u8\n")
		case "/stream.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			io.WriteString(w, "#EXTM3U\nsegment.ts\n")
		case "/segment.ts":
			io.WriteString(w, "media")
		case "/logo":
			w.Header().Set("Content-Type", "image/png")
			w.Write(defaultLogo())
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	g := &gateway{sources: []string{upstream.URL + "/channels.m3u"}, cacheTTL: 3 * time.Hour, client: &http.Client{Timeout: time.Second}, probeClient: &http.Client{Timeout: time.Second}, byKey: map[string]*channel{}, refs: map[string]reference{}}
	server := httptest.NewServer(g.router())
	defer server.Close()
	if fetches.Load() != 0 {
		t.Fatal("fetched before access")
	}
	get := func(path string) *http.Response {
		t.Helper()
		r, e := http.Get(server.URL + path)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	r := get("/")
	r.Body.Close()
	if fetches.Load() != 0 {
		t.Fatal("home page fetched M3U")
	}
	r = get("/channels.json")
	var doc struct {
		Channels []apiChannel `json:"channels"`
	}
	if e := json.NewDecoder(r.Body).Decode(&doc); e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if fetches.Load() != 1 || len(doc.Channels) != 1 {
		t.Fatalf("fetches=%d channels=%d", fetches.Load(), len(doc.Channels))
	}
	r = get("/channels.json")
	r.Body.Close()
	if fetches.Load() != 1 {
		t.Fatal("cache missed")
	}
	r = get(strings.TrimPrefix(doc.Channels[0].Logo, server.URL))
	r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatal("logo UA")
	}
	r = get(strings.TrimPrefix(doc.Channels[0].URLs[0], server.URL))
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != 200 || !strings.Contains(string(body), "/resource/") {
		t.Fatalf("play UA: %d %s", r.StatusCode, body)
	}
	if fetches.Load() != 1 {
		t.Fatal("playback caused refresh before TTL")
	}
	g.sourceCacheMu.Lock()
	snapshot := g.sourceCache[upstream.URL+"/channels.m3u"]
	snapshot.fetchedAt = time.Now().Add(-4 * time.Hour)
	g.sourceCache[upstream.URL+"/channels.m3u"] = snapshot
	g.sourceCacheMu.Unlock()
	if fetches.Load() != 1 {
		t.Fatal("background refresh")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := http.Get(server.URL + "/channels.json")
			if e == nil {
				r.Body.Close()
			} else {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if fetches.Load() != 2 {
		t.Fatalf("concurrent refreshes: %d", fetches.Load())
	}
	broken.Store(true)
	g.sourceCacheMu.Lock()
	snapshot = g.sourceCache[upstream.URL+"/channels.m3u"]
	snapshot.fetchedAt = time.Now().Add(-4 * time.Hour)
	g.sourceCache[upstream.URL+"/channels.m3u"] = snapshot
	g.sourceCacheMu.Unlock()
	r = get("/channels.json")
	var stale struct {
		Channels []apiChannel `json:"channels"`
	}
	json.NewDecoder(r.Body).Decode(&stale)
	r.Body.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		g.sourceCacheMu.Lock()
		retryAt := g.sourceCache[upstream.URL+"/channels.m3u"].retryAt
		g.sourceCacheMu.Unlock()
		if fetches.Load() == 3 && !retryAt.IsZero() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if fetches.Load() != 3 || len(stale.Channels) != 1 {
		t.Fatalf("stale cache: fetches=%d channels=%d", fetches.Load(), len(stale.Channels))
	}
	r = get("/channels.json")
	r.Body.Close()
	if fetches.Load() != 3 {
		t.Fatal("failure retry not throttled")
	}
}
