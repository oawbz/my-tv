package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSlowSegmentsRequireRepeatedSamples(t *testing.T) {
	ch := &channel{inputChannel: inputChannel{URLs: []string{"source-a", "source-b"}}, key: "one"}
	g := &gateway{byKey: map[string]*channel{"one": ch}}
	for i := 0; i < 2; i++ {
		g.observeSegment("one", "source-a", 2*time.Second, 4*time.Second, nil)
	}
	if !ch.badUntil["source-a"].IsZero() {
		t.Fatal("source switched after fewer than three slow segments")
	}
	g.observeSegment("one", "source-a", 2*time.Second, time.Second, nil)
	g.observeSegment("one", "source-a", 2*time.Second, 4*time.Second, nil)
	if !ch.badUntil["source-a"].IsZero() {
		t.Fatal("good segment did not clear the slow streak")
	}
	for i := 0; i < 2; i++ {
		g.observeSegment("one", "source-a", 2*time.Second, 4*time.Second, nil)
	}
	if !time.Now().Before(ch.badUntil["source-a"]) {
		t.Fatal("three slow segments did not demote source")
	}
}

func TestFailedSegmentSwitchesOnNextPlaybackRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			io.WriteString(w, "#EXTM3U\n#EXTINF:2,\na.ts\n")
		case "/b.m3u8":
			if r.Header.Get("Range") != "" {
				time.Sleep(30 * time.Millisecond)
			}
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			io.WriteString(w, "#EXTM3U\n#EXTINF:2,\nb.ts\n")
		case "/a.ts":
			http.Error(w, "failed", http.StatusBadGateway)
		case "/b.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			io.WriteString(w, "segment")
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	ch := &channel{inputChannel: inputChannel{Name: "CCTV1综合", URLs: []string{upstream.URL + "/a.m3u8", upstream.URL + "/b.m3u8"}}, key: "one"}
	g := &gateway{channels: []*channel{ch}, byKey: map[string]*channel{"one": ch}, refs: map[string]reference{}, client: &http.Client{Timeout: time.Second}, probeClient: &http.Client{Timeout: time.Second}}
	server := httptest.NewServer(g.router())
	defer server.Close()
	play := func() string {
		t.Helper()
		resp, err := http.Get(server.URL + "/play/one/index.m3u8")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("play status %d", resp.StatusCode)
		}
		return string(body)
	}
	first := play()
	if !strings.Contains(first, "#EXTINF:2") || !strings.Contains(first, "/resource/") {
		t.Fatal(first)
	}
	segment := strings.TrimSpace(strings.Split(first, "\n")[2])
	if g.refs[segment[strings.LastIndex(segment, "/")+1:]].sourceURL != upstream.URL+"/a.m3u8" {
		t.Fatal("did not select faster primary source")
	}
	resp, err := http.Get(segment)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("failed segment status %d", resp.StatusCode)
	}
	if !time.Now().Before(ch.badUntil[upstream.URL+"/a.m3u8"]) {
		t.Fatal("failed source was not demoted")
	}
	second := play()
	segment = strings.TrimSpace(strings.Split(second, "\n")[2])
	if g.refs[segment[strings.LastIndex(segment, "/")+1:]].sourceURL != upstream.URL+"/b.m3u8" {
		t.Fatal("retry reused the failed source")
	}
	resp, err = http.Get(segment)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("backup segment status %d", resp.StatusCode)
	}
}
