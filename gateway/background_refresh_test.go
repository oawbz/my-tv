package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestExpiredRemoteM3URefreshDoesNotBlockPlayback(t *testing.T) {
	var loads atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	finishRefresh := func() { releaseOnce.Do(func() { close(release) }) }
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/channels.m3u":
			n := loads.Add(1)
			if n == 2 {
				close(started)
				<-release
			}
			stream := "old.m3u8"
			if n > 1 {
				stream = "new.m3u8"
			}
			fmt.Fprintf(w, "#EXTM3U\n#EXTINF:-1,CCTV1综合\n%s/%s\n", upstream.URL, stream)
		case "/old.m3u8", "/new.m3u8":
			fmt.Fprint(w, "#EXTM3U\nsegment.ts\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	g := &gateway{sources: []string{upstream.URL + "/channels.m3u"}, cacheTTL: 3 * time.Hour, client: &http.Client{Timeout: time.Second}, byKey: map[string]*channel{}, refs: map[string]reference{}}
	// Production requests check config.json before checking the remote cache.
	configData := []byte(`{}`)
	g.configPath = filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(g.configPath, configData, 0600); err != nil {
		t.Fatal(err)
	}
	g.configHash = sha256.Sum256(configData)
	requestClient := &http.Client{Timeout: time.Second}
	server := httptest.NewServer(g.router())
	defer server.Close()
	defer finishRefresh()
	resp, err := http.Get(server.URL + "/channels.json")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if loads.Load() != 1 {
		t.Fatalf("initial loads: %d", loads.Load())
	}
	g.sourceCacheMu.Lock()
	snapshot := g.sourceCache[upstream.URL+"/channels.m3u"]
	snapshot.fetchedAt = time.Now().Add(-4 * time.Hour)
	g.sourceCache[upstream.URL+"/channels.m3u"] = snapshot
	g.sourceCacheMu.Unlock()
	play := func() error {
		start := time.Now()
		resp, err := requestClient.Get(server.URL + "/play/" + keyFor(normalizedName("CCTV1综合")) + "/index.m3u8")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode != 200 || !strings.Contains(string(body), "/resource/") {
			return fmt.Errorf("play status %d: %s", resp.StatusCode, body)
		}
		if elapsed := time.Since(start); elapsed > 300*time.Millisecond {
			return fmt.Errorf("play waited for M3U refresh: %v", elapsed)
		}
		return nil
	}
	if err := play(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background refresh did not start")
	}
	listingStarted := time.Now()
	listing, err := requestClient.Get(server.URL + "/channels.json")
	if err != nil {
		t.Fatal(err)
	}
	listing.Body.Close()
	if listing.StatusCode != 200 || time.Since(listingStarted) > 300*time.Millisecond {
		t.Fatal("channel listing waited for stale remote M3U")
	}
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := play(); err != nil {
				errors <- err
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	if loads.Load() != 2 {
		t.Fatalf("refreshes were not coalesced: %d", loads.Load())
	}
	finishRefresh()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		g.mu.RLock()
		ch := g.byKey[keyFor(normalizedName("CCTV1综合"))]
		updated := ch != nil && len(ch.URLs) == 1 && strings.HasSuffix(ch.URLs[0], "/new.m3u8")
		g.mu.RUnlock()
		if updated {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("background refresh did not update channel")
}
