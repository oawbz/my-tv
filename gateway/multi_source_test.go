package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestMultipleM3UsMergeAliasesAndCacheRemoteIndependently(t *testing.T) {
	var fetches atomic.Int32
	var broken atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/remote.m3u" {
			http.NotFound(w, r)
			return
		}
		fetches.Add(1)
		if r.Header.Get("User-Agent") != defaultUpstreamUA {
			http.Error(w, "wrong UA", 403)
			return
		}
		if broken.Load() {
			http.Error(w, "unavailable", 503)
			return
		}
		fmt.Fprint(w, "#EXTM3U\n#EXTINF:-1,CCTV-1高清\nhttp://example.test/c1-remote\n#EXTINF:-1,河北台\nhttp://example.test/hebei-remote\n")
	}))
	defer upstream.Close()
	file := filepath.Join(t.TempDir(), "local.m3u")
	writeLocal := func(extra string) {
		t.Helper()
		data := "#EXTM3U\n#EXTINF:-1,CCTV1综合\nhttp://example.test/c1-local\n#EXTINF:-1,河北卫视\nhttp://example.test/hebei-local\n" + extra
		if err := os.WriteFile(file, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeLocal("")
	cfg := defaultConfig()
	cfg.M3USources = []string{file, upstream.URL + "/remote.m3u"}
	catalog, err := buildCatalog(cfg.Channels)
	if err != nil {
		t.Fatal(err)
	}
	g := &gateway{sources: cfg.sources(), catalog: catalog, cacheTTL: 3 * time.Hour, client: &http.Client{Timeout: time.Second}, byKey: map[string]*channel{}, refs: map[string]reference{}}
	server := httptest.NewServer(g.router())
	defer server.Close()
	list := func() []apiChannel {
		t.Helper()
		resp, err := http.Get(server.URL + "/channels.json")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var doc struct {
			Channels []apiChannel `json:"channels"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("decode: %v %s", err, body)
		}
		return doc.Channels
	}
	channels := list()
	if len(channels) != 2 || channels[0].Name != "CCTV1 综合" || channels[1].Name != "河北卫视" || fetches.Load() != 1 {
		t.Fatalf("channels=%v fetches=%d", channels, fetches.Load())
	}
	if len(g.channels[0].URLs) != 2 || len(g.channels[1].URLs) != 2 {
		t.Fatalf("unmerged source URLs: %v %v", g.channels[0].URLs, g.channels[1].URLs)
	}
	stableURL := channels[0].URLs[0]
	writeLocal("#EXTINF:-1,CCTV2财经\nhttp://example.test/c2-local\n")
	channels = list()
	if len(channels) != 3 || channels[0].URLs[0] != stableURL || fetches.Load() != 1 {
		t.Fatalf("local reload changed remote cache or playback URL: channels=%v fetches=%d", channels, fetches.Load())
	}
	broken.Store(true)
	g.sourceCacheMu.Lock()
	snapshot := g.sourceCache[upstream.URL+"/remote.m3u"]
	snapshot.fetchedAt = time.Now().Add(-4 * time.Hour)
	g.sourceCache[upstream.URL+"/remote.m3u"] = snapshot
	g.sourceCacheMu.Unlock()
	channels = list()
	if len(channels) != 3 || len(g.channels[0].URLs) != 2 || fetches.Load() != 2 {
		t.Fatalf("stale source lost: channels=%v fetches=%d", channels, fetches.Load())
	}
	list()
	if fetches.Load() != 2 {
		t.Fatalf("failed remote source retried too soon: %d", fetches.Load())
	}
	broken.Store(false)
	g.sourceCacheMu.Lock()
	snapshot = g.sourceCache[upstream.URL+"/remote.m3u"]
	snapshot.retryAt = time.Now().Add(-time.Second)
	g.sourceCache[upstream.URL+"/remote.m3u"] = snapshot
	g.sourceCacheMu.Unlock()
	channels = list()
	if len(channels) != 3 || fetches.Load() != 3 {
		t.Fatalf("recovered remote source was not refreshed: channels=%v fetches=%d", channels, fetches.Load())
	}
}
