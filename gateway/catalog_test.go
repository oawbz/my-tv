package main

import (
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestChannelConfigReloadAndLastGood(t *testing.T) {
	dir := t.TempDir()
	m3u := filepath.Join(dir, "index.m3u")
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(m3u, []byte("#EXTM3U\n#EXTINF:-1 tvg-logo=\"https://example.test/source.png\",河北台\nhttps://example.test/hebei.m3u8\n#EXTINF:-1,CCTV5+ 体育赛事\nhttps://example.test/cctv5plus.m3u8\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.M3USources = []string{m3u}
	writeConfig := func() [32]byte {
		t.Helper()
		data, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(configPath, data, 0600); err != nil {
			t.Fatal(err)
		}
		return sha256.Sum256(data)
	}
	hash := writeConfig()
	catalog, err := buildCatalog(cfg.Channels)
	if err != nil {
		t.Fatal(err)
	}
	g := &gateway{sources: cfg.sources(), catalog: catalog, configPath: configPath, configHash: hash, client: &http.Client{Timeout: time.Second}, byKey: map[string]*channel{}}
	server := httptest.NewServer(g.router())
	defer server.Close()
	get := func() struct {
		Channels []apiChannel `json:"channels"`
	} {
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
			t.Fatal(err)
		}
		return doc
	}
	initial := get()
	if len(initial.Channels) != 1 || initial.Channels[0].Name != "河北卫视" {
		t.Fatalf("initial channels: %+v", initial)
	}
	play := initial.Channels[0].URLs[0]
	logo := initial.Channels[0].Logo
	cfg.Channels[18].Name = "河北新闻"
	cfg.Channels[18].Logo = "https://example.test/custom.png"
	cfg.Channels[18].Group = "本地"
	writeConfig()
	updated := get()
	if len(updated.Channels) != 1 || updated.Channels[0].Name != "河北新闻" || updated.Channels[0].Group != "本地" || updated.Channels[0].URLs[0] != play || updated.Channels[0].Logo != logo {
		t.Fatalf("updated channels: %+v", updated)
	}
	g.mu.RLock()
	if g.channels[0].Logo != "https://example.test/custom.png" {
		t.Errorf("configured logo not selected: %s", g.channels[0].Logo)
	}
	g.mu.RUnlock()
	if err := os.WriteFile(configPath, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := get(); len(got.Channels) != 1 || got.Channels[0].Name != "河北新闻" {
		t.Fatalf("invalid config replaced last good channels: %+v", got)
	}
	cfg.Channels[18].Name = ""
	cfg.Channels[18].Logo = ""
	writeConfig()
	if got := get(); len(got.Channels) != 1 || got.Channels[0].Name != "河北卫视" {
		t.Fatalf("empty name did not fall back to id: %+v", got)
	}
	g.mu.RLock()
	if g.channels[0].Logo != "https://example.test/source.png" {
		t.Errorf("upstream logo not restored: %s", g.channels[0].Logo)
	}
	g.mu.RUnlock()
	cfg.Channels[18].Aliases = nil
	writeConfig()
	if got := get(); len(got.Channels) != 0 {
		t.Fatalf("source-less channel still listed: %+v", got)
	}
	cfg.Channels = []channelConfig{}
	writeConfig()
	if got := get(); len(got.Channels) != 2 || got.Channels[0].Name != "河北台" || got.Channels[1].Name != "CCTV5+ 体育赛事" {
		t.Fatalf("empty channels did not enable pass-through: %+v", got)
	}
}

func TestEmptyChannelsPassThroughAllUpstreamEntries(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.m3u")
	second := filepath.Join(dir, "second.m3u")
	if err := os.WriteFile(first, []byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"CCTV1.cn@SD\" tvg-logo=\"https://example.test/cctv.png\" group-title=\"General\",CCTV-1 (720p)\nhttps://example.test/one.m3u8\n#EXTINF:-1 group-title=\"General\",Anhui TV\nhttps://example.test/anhui.m3u8\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("#EXTM3U\n#EXTINF:-1,CCTV-1 (720p)\nhttps://example.test/two.m3u8\n#EXTINF:-1,CCTV-1 HD (1080p)\nhttps://example.test/hd.m3u8\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.Channels = []channelConfig{}
	cfg.M3USources = []string{first, second}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseConfig(data)
	if err != nil || len(parsed.Channels) != 0 {
		t.Fatalf("empty channels rejected: %+v %v", parsed, err)
	}
	catalog, err := buildCatalog(parsed.Channels)
	if err != nil {
		t.Fatal(err)
	}
	g := &gateway{sources: parsed.sources(), catalog: catalog, client: &http.Client{Timeout: time.Second}, byKey: map[string]*channel{}}
	server := httptest.NewServer(g.router())
	defer server.Close()
	response, err := http.Get(server.URL + "/channels.json")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var doc struct {
		Channels []apiChannel `json:"channels"`
	}
	if err := json.NewDecoder(response.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Channels) != 3 || doc.Channels[0].Name != "CCTV-1 (720p)" || doc.Channels[1].Name != "Anhui TV" || doc.Channels[2].Name != "CCTV-1 HD (1080p)" || doc.Channels[0].Group != "General" {
		t.Fatalf("pass-through order or metadata: %+v", doc.Channels)
	}
	if len(g.channels[0].URLs) != 2 || g.channels[0].Logo != "https://example.test/cctv.png" {
		t.Fatalf("same-name sources not merged: %+v", g.channels[0])
	}
	if !strings.HasPrefix(doc.Channels[0].URLs[0], server.URL+"/play/") {
		t.Fatalf("play URL did not use gateway: %s", doc.Channels[0].URLs[0])
	}
	m3uResponse, err := http.Get(server.URL + "/channels.m3u")
	if err != nil {
		t.Fatal(err)
	}
	defer m3uResponse.Body.Close()
	buf := new(strings.Builder)
	if _, err := io.Copy(buf, m3uResponse.Body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "CCTV-1 HD (1080p)") || !strings.Contains(buf.String(), `tvg-id="CCTV1.cn@SD"`) || strings.Contains(buf.String(), "https://example.test/one.m3u8") {
		t.Fatalf("M3U output was filtered or exposed upstream URLs: %s", buf.String())
	}
}
