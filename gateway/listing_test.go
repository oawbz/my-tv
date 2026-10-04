package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHomeAndM3UListing(t *testing.T) {
	file := filepath.Join(t.TempDir(), "upstream.m3u")
	playlist := "#EXTM3U\n" +
		"#EXTINF:-1 tvg-logo=\"http://logo.example/hebei.png\",河北卫视\nhttp://media.example/hebei.m3u8\n" +
		"#EXTINF:-1,CCTV1综合\nhttp://media.example/cctv-a.m3u8\n" +
		"#EXTINF:-1,CCTV1综合\nhttp://media.example/cctv-b.m3u8\n" +
		"#EXTINF:-1,未预设频道\nhttp://media.example/other.m3u8\n"
	if err := os.WriteFile(file, []byte(playlist), 0600); err != nil {
		t.Fatal(err)
	}
	g := &gateway{sources: []string{file}, byKey: map[string]*channel{}, refs: map[string]reference{}}
	server := httptest.NewServer(g.router())
	defer server.Close()
	home, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	homeBody, err := io.ReadAll(home.Body)
	home.Body.Close()
	if err != nil || home.StatusCode != 200 || !strings.Contains(home.Header.Get("Content-Type"), "text/html") || !strings.Contains(string(homeBody), server.URL+"/channels.m3u") || !strings.Contains(string(homeBody), server.URL+"/channels.json") {
		t.Fatalf("bad home: status=%d err=%v body=%s", home.StatusCode, err, homeBody)
	}
	if len(g.channels) != 0 {
		t.Fatal("home unexpectedly loaded upstream M3U")
	}
	response, err := http.Get(server.URL + "/channels.m3u")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || !strings.Contains(response.Header.Get("Content-Type"), "audio/x-mpegurl") {
		t.Fatalf("bad M3U response: status=%d err=%v", response.StatusCode, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 5 || lines[0] != "#EXTM3U" || !strings.Contains(lines[1], "CCTV1 综合") || !strings.Contains(lines[3], "河北卫视") || lines[2] != server.URL+"/play/"+keyFor(normalizedName("CCTV1综合"))+"/index.m3u8" || lines[4] != server.URL+"/play/"+keyFor(normalizedName("河北卫视"))+"/index.m3u8" {
		t.Fatalf("unexpected M3U: %s", data)
	}
	if !strings.Contains(lines[3], server.URL+"/logo/") || strings.Contains(string(data), "media.example") || strings.Contains(string(data), "未预设频道") {
		t.Fatalf("M3U exposed upstream or omitted logo: %s", data)
	}
	jsonResponse, err := http.Get(server.URL + "/channels.json")
	if err != nil {
		t.Fatal(err)
	}
	defer jsonResponse.Body.Close()
	var doc struct {
		Channels []apiChannel `json:"channels"`
	}
	if err := json.NewDecoder(jsonResponse.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Channels) != 2 || doc.Channels[0].URLs[0] != lines[2] || doc.Channels[1].URLs[0] != lines[4] {
		t.Fatalf("JSON and M3U disagree: %+v", doc)
	}
}

func TestM3UFieldRemovesLineBreaksAndQuotes(t *testing.T) {
	if got := m3uField("name\"\r\n#EXTINF:-1"); got != "name'  #EXTINF:-1" {
		t.Fatalf("unsafe field: %q", got)
	}
}
