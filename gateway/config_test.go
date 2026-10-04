package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestConfigGeneratedAndReused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg, e := loadConfig(path)
	if e != nil {
		t.Fatal(e)
	}
	if cfg.Listen != "0.0.0.0:2219" || !reflect.DeepEqual(cfg.M3USources, []string{"./index.m3u"}) || !reflect.DeepEqual(cfg.Tokens, []string{}) || cfg.CacheDuration != "3h" || cfg.UserAgent != defaultUpstreamUA || len(cfg.Channels) != 49 || cfg.Channels[18].ID != "河北卫视" || !reflect.DeepEqual(cfg.Channels[18].Aliases, []string{"河北台"}) {
		t.Fatalf("defaults: %+v", cfg)
	}
	for _, ch := range cfg.Channels {
		if ch.ID == "CCTV5+ 体育赛事" {
			t.Fatal("CCTV5+ remains in default channels")
		}
	}
	generated, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var fields map[string]json.RawMessage
	if e := json.Unmarshal(generated, &fields); e != nil {
		t.Fatal(e)
	}
	if _, ok := fields["public_base"]; ok {
		t.Fatal("generated obsolete public_base")
	}
	info, e := os.Stat(path)
	if e != nil {
		t.Fatal(e)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissions: %v", info.Mode())
	}
	cfg.Listen = "127.0.0.1:8888"
	cfg.M3USources = []string{"./other.m3u"}
	cfg.Tokens = []string{"device-one", "device-two"}
	cfg.CacheDuration = "6h"
	cfg.UserAgent = "AptvPlayer/2.0"
	data, _ := json.Marshal(cfg)
	if e = os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
	loaded, e := loadConfig(path)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(loaded, cfg) {
		t.Fatalf("not reused: %+v", loaded)
	}
	if e = os.WriteFile(path, []byte(`{"listen":"invalid"}`), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = loadConfig(path); e == nil {
		t.Fatal("invalid listen accepted")
	}
	cfg.Listen = "127.0.0.1:8888"
	cfg.CacheDuration = "invalid"
	data, _ = json.Marshal(cfg)
	if e = os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = loadConfig(path); e == nil {
		t.Fatal("invalid cache duration accepted")
	}
}

func TestConfigRejectsLegacyAndMissingSources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	for _, data := range []string{
		`{"m3u":"./old.m3u"}`,
		`{"m3u_sources":["./index.m3u"],"public_base":"https://example.com/tv"}`,
		`{"listen":"127.0.0.1:2219"}`,
		`{"m3u_sources":[]}`,
		`{"m3u_sources":null}`,
		`{"m3u_sources":["./index.m3u"],"tokens":[""]}`,
		`{"m3u_sources":["./index.m3u"],"tokens":[" has-space "]}`,
	} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadConfig(path); err == nil {
			t.Fatalf("invalid sources accepted: %s", data)
		}
	}
}

func TestMultipleSourcesAndAliasesConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := defaultConfig()
	cfg.M3USources = []string{" ./one.m3u ", "https://example.com/two.m3u", "./one.m3u"}
	cfg.Channels[0].Aliases = []string{"CCTV-1高清"}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.sources(), []string{"./one.m3u", "https://example.com/two.m3u"}) {
		t.Fatalf("sources: %v", loaded.sources())
	}
	catalog, err := buildCatalog(loaded.Channels)
	if err != nil || catalog.byName[normalizedName("CCTV-1高清")].id != "CCTV1 综合" {
		t.Fatalf("aliases: %v %v", catalog, err)
	}
	cfg.Channels[0].Aliases = []string{"河北卫视"}
	data, _ = json.Marshal(cfg)
	os.WriteFile(path, data, 0600)
	if _, err := loadConfig(path); err == nil {
		t.Fatal("conflicting alias accepted")
	}
	cfg.Channels[0].Aliases = nil
	cfg.Channels[0].Name = "河北卫视"
	data, _ = json.Marshal(cfg)
	os.WriteFile(path, data, 0600)
	if _, err := loadConfig(path); err == nil {
		t.Fatal("conflicting display name accepted")
	}
}
