package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestColdSelectionDoesNotWaitForSlowProbeAndWarmSelectionSkipsProbes(t *testing.T) {
	var slowFetches, fastFetches atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slow.m3u8":
			slowFetches.Add(1)
			time.Sleep(600 * time.Millisecond)
			io.WriteString(w, "#EXTM3U\nslow.ts\n")
		case "/fast.m3u8":
			fastFetches.Add(1)
			if r.Header.Get("Range") != "" {
				t.Error("HLS fetch should not use Range")
			}
			io.WriteString(w, "#EXTM3U\nfast.ts\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	ch := &channel{inputChannel: inputChannel{Name: "CCTV1综合", URLs: []string{upstream.URL + "/slow.m3u8", upstream.URL + "/fast.m3u8"}}, key: "one"}
	g := &gateway{channels: []*channel{ch}, byKey: map[string]*channel{"one": ch}, refs: map[string]reference{}, client: &http.Client{Timeout: time.Second}, probeClient: &http.Client{Timeout: time.Second}}
	server := httptest.NewServer(g.router())
	defer server.Close()
	play := func() time.Duration {
		t.Helper()
		start := time.Now()
		resp, err := http.Get(server.URL + "/play/one/index.m3u8")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "/resource/") {
			t.Fatalf("play status %d: %s", resp.StatusCode, body)
		}
		return time.Since(start)
	}
	if elapsed := play(); elapsed >= 400*time.Millisecond {
		t.Fatalf("cold play waited for slow probe: %v", elapsed)
	}
	if ch.lastGood != upstream.URL+"/fast.m3u8" {
		t.Fatalf("selected %s", ch.lastGood)
	}
	if fastFetches.Load() != 1 {
		t.Fatalf("cold HLS play fetched upstream %d times", fastFetches.Load())
	}
	firstSlowFetches := slowFetches.Load()
	time.Sleep(1100 * time.Millisecond) // Expire the short HLS manifest cache.
	if elapsed := play(); elapsed >= 400*time.Millisecond {
		t.Fatalf("warm play took too long: %v", elapsed)
	}
	if slowFetches.Load() != firstSlowFetches {
		t.Fatalf("warm play reprobed slow source: %d", slowFetches.Load())
	}
	if fastFetches.Load() != 2 {
		t.Fatalf("expected one fetch per play, got %d", fastFetches.Load())
	}
}
