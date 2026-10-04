package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenAuthenticationAcrossPlaylistAndResources(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		switch r.URL.Path {
		case "/live.m3u8":
			io.WriteString(w, "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"key.bin\"\nsub.m3u8\n")
		case "/sub.m3u8":
			io.WriteString(w, "#EXTM3U\nsegment.ts\n")
		case "/key.bin":
			io.WriteString(w, "secret")
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
	ch := &channel{inputChannel: inputChannel{Name: "CCTV1 综合", Group: "央视", URLs: []string{upstream.URL + "/live.m3u8"}, Logo: upstream.URL + "/logo"}, key: "one"}
	g := &gateway{tokens: []string{"first", "second&+"}, channels: []*channel{ch}, byKey: map[string]*channel{"one": ch}, refs: map[string]reference{}, client: &http.Client{Timeout: time.Second}, probeClient: &http.Client{Timeout: time.Second}}
	server := httptest.NewServer(g.router())
	defer server.Close()
	read := func(raw string) (int, string) {
		t.Helper()
		resp, err := http.Get(raw)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, string(body)
	}
	if status, body := read(server.URL + "/"); status != 200 || !strings.Contains(body, `name="toke"`) || strings.Contains(body, server.URL+"/channels.m3u") {
		t.Fatalf("unauthenticated home: %d %s", status, body)
	}
	if status, body := read(server.URL + "/?toke=wrong"); status != 200 || !strings.Contains(body, "Token 不正确") || strings.Contains(body, server.URL+"/channels.json?toke=") {
		t.Fatalf("invalid home token: %d %s", status, body)
	}
	if status, body := read(server.URL + "/?toke=first"); status != 200 || !strings.Contains(body, server.URL+"/channels.m3u?toke=first") || !strings.Contains(body, server.URL+"/channels.json?toke=first") || strings.Contains(body, "/stats") {
		t.Fatalf("home links: %d %s", status, body)
	}
	if status, _ := read(server.URL + "/stats?toke=first"); status != 404 {
		t.Fatalf("removed stats endpoint: %d", status)
	}
	if status, _ := read(server.URL + "/healthz?toke=first"); status != 404 {
		t.Fatalf("removed health endpoint: %d", status)
	}
	for _, path := range []string{"/channels.json", "/channels.m3u", "/play/one/index.m3u8", "/logo/one", "/resource/missing"} {
		if status, _ := read(server.URL + path); status != 401 {
			t.Fatalf("%s without token: %d", path, status)
		}
		if status, _ := read(server.URL + path + "?toke=wrong"); status != 401 {
			t.Fatalf("%s with wrong token: %d", path, status)
		}
	}
	if upstreamCalls.Load() != 0 {
		t.Fatalf("unauthorized requests reached upstream: %d", upstreamCalls.Load())
	}
	response, err := http.Get(server.URL + "/channels.json?toke=first")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Channels []apiChannel `json:"channels"`
	}
	if err := json.NewDecoder(response.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 || len(doc.Channels) != 1 || !strings.HasSuffix(doc.Channels[0].URLs[0], "?toke=first") || !strings.HasSuffix(doc.Channels[0].Logo, "?toke=first") || response.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("authenticated JSON: status=%d channels=%+v cache=%q", response.StatusCode, doc.Channels, response.Header.Get("Cache-Control"))
	}
	encodedSecond := url.QueryEscape("second&+")
	if status, body := read(server.URL + "/channels.m3u?toke=" + encodedSecond); status != 200 || !strings.Contains(body, "?toke="+encodedSecond) || strings.Contains(body, upstream.URL) {
		t.Fatalf("authenticated M3U: %d %s", status, body)
	}
	if status, _ := read(doc.Channels[0].Logo); status != 200 {
		t.Fatalf("authenticated logo: %d", status)
	}
	status, manifest := read(doc.Channels[0].URLs[0])
	if status != 200 || !strings.Contains(manifest, "?toke=first") {
		t.Fatalf("authenticated manifest: %d %s", status, manifest)
	}
	var subURL, keyURL string
	for _, line := range strings.Split(manifest, "\n") {
		if strings.HasPrefix(line, server.URL+"/resource/") {
			subURL = line
		}
		if strings.Contains(line, `URI="`) {
			keyURL = strings.Split(line, `"`)[1]
		}
	}
	if subURL == "" || keyURL == "" || !strings.HasSuffix(subURL, "?toke=first") || !strings.HasSuffix(keyURL, "?toke=first") {
		t.Fatalf("resource links lost token: %s", manifest)
	}
	withoutToken := strings.SplitN(subURL, "?", 2)[0]
	if status, _ := read(withoutToken); status != 401 {
		t.Fatalf("resource without token: %d", status)
	}
	if status, body := read(keyURL); status != 200 || body != "secret" {
		t.Fatalf("authenticated key: %d %s", status, body)
	}
	status, sub := read(subURL)
	if status != 200 || !strings.Contains(sub, "?toke=first") {
		t.Fatalf("authenticated submanifest: %d %s", status, sub)
	}
	segmentURL := strings.TrimSpace(strings.Split(sub, "\n")[1])
	if status, body := read(segmentURL); status != 200 || body != "media" {
		t.Fatalf("authenticated segment: %d %s", status, body)
	}
}

func TestNoConfiguredTokensKeepsRoutesOpen(t *testing.T) {
	g := &gateway{tokens: nil}
	server := httptest.NewServer(g.router())
	defer server.Close()
	resp, err := http.Get(server.URL + "/channels.m3u")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(data) != "#EXTM3U\n" || resp.Header.Get("Cache-Control") != "public, max-age=60" {
		t.Fatalf("open M3U: %d %s", resp.StatusCode, data)
	}
}
