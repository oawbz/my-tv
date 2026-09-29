package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigGeneratedAndReused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg, e := loadConfig(path)
	if e != nil {
		t.Fatal(e)
	}
	if cfg.Listen != "0.0.0.0:2219" || cfg.M3U != "./index.m3u" || cfg.CacheDuration != "3h" || cfg.UserAgent != defaultUpstreamUA {
		t.Fatalf("defaults: %+v", cfg)
	}
	info, e := os.Stat(path)
	if e != nil {
		t.Fatal(e)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissions: %v", info.Mode())
	}
	cfg.Listen = "127.0.0.1:8888"
	cfg.M3U = "./other.m3u"
	cfg.PublicBase = "https://awbz.cn/tv"
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
	if loaded != cfg {
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
