package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGatewayM3UPlayback(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slow.m3u8":
			time.Sleep(120 * time.Millisecond)
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			io.WriteString(w, "#EXTM3U\nslow.ts\n")
		case "/fast.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			io.WriteString(w, "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"key.bin\"\nsub.m3u8\n")
		case "/sub.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			io.WriteString(w, "#EXTM3U\nsegment.ts\n")
		case "/segment.ts":
			w.Header().Set("Content-Type", "video/mp2t")
			io.WriteString(w, "fast-media")
		case "/key.bin":
			io.WriteString(w, "secret")
		case "/logo.webp":
			w.Header().Set("Content-Type", "image/png")
			w.Write(defaultLogo())
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	dir := t.TempDir()
	file := filepath.Join(dir, "list.m3u")
	list := "#EXTM3U\n#EXTINF:-1 tvg-id=\"c1\" tvg-logo=\"" + upstream.URL + "/logo.webp\" group-title=\"央视\",CCTV1综合\n" + upstream.URL + "/slow.m3u8\n#EXTINF:-1 tvg-id=\"c1\" tvg-logo=\"" + upstream.URL + "/logo.webp\" group-title=\"央视\",CCTV1综合\n" + upstream.URL + "/fast.m3u8\n"
	if e := os.WriteFile(file, []byte(list), 0600); e != nil {
		t.Fatal(e)
	}
	g := &gateway{sources: []string{file}, client: &http.Client{Timeout: time.Second}, probeClient: &http.Client{Timeout: time.Second}, byKey: map[string]*channel{}, refs: map[string]reference{}}
	if e := g.refresh(context.Background()); e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(g.router())
	defer server.Close()
	var api struct {
		Version  int          `json:"version"`
		Channels []apiChannel `json:"channels"`
	}
	resp, e := http.Get(server.URL + "/channels.json")
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	if e = json.NewDecoder(resp.Body).Decode(&api); e != nil {
		t.Fatal(e)
	}
	if api.Version != 1 || len(api.Channels) != 1 || len(api.Channels[0].URLs) != 1 || api.Channels[0].Group != "央视" {
		t.Fatalf("bad API: %+v", api)
	}
	resp, e = http.Get(api.Channels[0].Logo)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Equal(b, defaultLogo()) {
		t.Fatalf("logo: %q", b)
	}
	resp, e = http.Get(api.Channels[0].URLs[0])
	if e != nil {
		t.Fatal(e)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	body := string(b)
	if !strings.Contains(body, "/resource/") || strings.Contains(body, upstream.URL) || strings.Contains(body, "slow.ts") {
		t.Fatalf("manifest: %s", body)
	}
	var subURL, keyURL string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, server.URL+"/resource/") {
			subURL = line
		}
		if strings.Contains(line, "URI=\"") {
			keyURL = strings.Split(line, "\"")[1]
		}
	}
	if subURL == "" || keyURL == "" {
		t.Fatalf("missing rewritten URLs: %s", body)
	}
	resp, e = http.Get(keyURL)
	if e != nil {
		t.Fatal(e)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != "secret" {
		t.Fatalf("key: %q", b)
	}
	resp, e = http.Get(subURL)
	if e != nil {
		t.Fatal(e)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	segmentURL := lines[len(lines)-1]
	resp, e = http.Get(segmentURL)
	if e != nil {
		t.Fatal(e)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != "fast-media" {
		t.Fatalf("segment: %q", b)
	}
}

func TestFallbackWhenFastestFails(t *testing.T) {
	count := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/flaky" {
			count++
			if count > 1 {
				http.Error(w, "failed", 502)
				return
			}
			io.WriteString(w, "#EXTM3U\nfirst.ts\n")
			return
		}
		if r.URL.Path == "/stable" {
			time.Sleep(30 * time.Millisecond)
			io.WriteString(w, "#EXTM3U\nstable.ts\n")
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	g := &gateway{client: &http.Client{Timeout: time.Second}, probeClient: &http.Client{Timeout: time.Second}, refs: map[string]reference{}}
	ch := &channel{inputChannel: inputChannel{URLs: []string{upstream.URL + "/flaky", upstream.URL + "/stable"}}}
	ranked := g.ranked(context.Background(), ch)
	if ranked[0] != upstream.URL+"/flaky" {
		t.Fatalf("ranking: %v", ranked)
	}
	g.base = "http://gateway.test"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	if e := g.relay(c, ranked[0], "", ""); e == nil {
		t.Fatal("expected failed source")
	}
	if e := g.relay(c, ranked[1], "", ""); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(w.Body.String(), "/resource/") {
		t.Fatal(w.Body.String())
	}
}
