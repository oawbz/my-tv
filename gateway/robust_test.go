package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMalformedM3UAndOrder(t *testing.T) {
	raw := "\xef\xbb\xbf#EXTM3U\n#EXTINF:-1 tvg-logo=\"bad\" group-title=\"其他\",浙江卫视\nhttp://example.test/z\n#EXTINF:-1 invalid\nhttp://example.test/ignored\n#EXTINF:-1,坏地址\nfile:///tmp/no.ts\n#EXTINF:-1 group-title=\"A,B\",CCTV2财经\nhttp://example.test/c2\n#EXTINF:-1,CCTV1综合\nhttp://example.test/c1\n#EXTINF:-1,CCTV1综合\nhttp://example.test/c1b\n#EXTINF:-1,其他频道\nhttp://example.test/other\n"
	dir := t.TempDir()
	file := filepath.Join(dir, "channels.m3u")
	if e := os.WriteFile(file, []byte(raw), 0600); e != nil {
		t.Fatal(e)
	}
	g := &gateway{sources: []string{file}, byKey: map[string]*channel{}, refs: map[string]reference{}}
	if e := g.refresh(context.Background()); e != nil {
		t.Fatal(e)
	}
	want := []string{"CCTV1 综合", "CCTV2 财经", "浙江卫视"}
	if len(g.channels) != len(want) {
		t.Fatalf("channels=%d", len(g.channels))
	}
	for i, ch := range g.channels {
		if ch.Name != want[i] {
			t.Fatalf("index %d got %s", i, ch.Name)
		}
	}
	if len(g.channels[0].URLs) != 2 {
		t.Fatalf("sources: %v", g.channels[0].URLs)
	}
	if g.channels[1].Group != "央视" || g.channels[2].Group != "地方卫视" {
		t.Fatal("reference groups not applied")
	}
	jsonFile := filepath.Join(dir, "bad.json")
	if e := os.WriteFile(jsonFile, []byte(`{"channels":[]}`), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := g.loadSource(context.Background(), jsonFile); e == nil {
		t.Fatal("JSON must not be accepted")
	}
}

func TestInvalidManifestSwitchAndLogoFallback(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bad.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Write([]byte("#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"key\"\n#EXTINF:5,\nfile:///invalid.ts\n"))
		case "/good.m3u8":
			time.Sleep(15 * time.Millisecond)
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Write([]byte("#EXTM3U\nsegment.ts\n"))
		case "/logo":
			http.Error(w, "missing", 404)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	ch := &channel{inputChannel: inputChannel{Name: "Test", Group: "其他", URLs: []string{upstream.URL + "/bad.m3u8", upstream.URL + "/good.m3u8"}, Logo: upstream.URL + "/logo"}, key: "test"}
	g := &gateway{channels: []*channel{ch}, byKey: map[string]*channel{"test": ch}, refs: map[string]reference{}, client: &http.Client{Timeout: time.Second}, probeClient: &http.Client{Timeout: time.Second}}
	server := httptest.NewServer(g.router())
	defer server.Close()
	resp, e := http.Get(server.URL + "/play/test/index.m3u8")
	if e != nil {
		t.Fatal(e)
	}
	body := new(bytes.Buffer)
	body.ReadFrom(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(body.String(), "/resource/") {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body.String())
	}
	ch.mu.Lock()
	badUntil := ch.badUntil[upstream.URL+"/bad.m3u8"]
	lastGood := ch.lastGood
	ch.mu.Unlock()
	if !time.Now().Before(badUntil) || lastGood != upstream.URL+"/good.m3u8" {
		t.Fatalf("failed source not demoted: badUntil=%v lastGood=%s", badUntil, lastGood)
	}
	resp, e = http.Get(server.URL + "/logo/test")
	if e != nil {
		t.Fatal(e)
	}
	body.Reset()
	body.ReadFrom(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" || !bytes.Equal(body.Bytes(), defaultLogo()) {
		t.Fatalf("fallback logo: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}
