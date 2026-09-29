package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

type config struct {
	Listen        string `json:"listen"`
	M3U           string `json:"m3u"`
	PublicBase    string `json:"public_base"`
	CacheDuration string `json:"cache_duration"`
	UserAgent     string `json:"user_agent"`
}

func defaultConfig() config {
	return config{Listen: "0.0.0.0:2219", M3U: "./index.m3u", PublicBase: "", CacheDuration: "3h", UserAgent: defaultUpstreamUA}
}

func loadConfig(path string) (config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		file, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e == nil {
			encoder := json.NewEncoder(file)
			encoder.SetIndent("", "  ")
			e = encoder.Encode(cfg)
			closeErr := file.Close()
			if e == nil {
				e = closeErr
			}
			if e != nil {
				return cfg, e
			}
			return cfg, nil
		}
		if !os.IsExist(e) {
			return cfg, e
		}
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return cfg, err
	}
	if err = json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("config.json: %w", err)
	}
	cfg.Listen = strings.TrimSpace(cfg.Listen)
	cfg.M3U = strings.TrimSpace(cfg.M3U)
	cfg.PublicBase = strings.TrimRight(strings.TrimSpace(cfg.PublicBase), "/")
	cfg.CacheDuration = strings.TrimSpace(cfg.CacheDuration)
	cfg.UserAgent = strings.TrimSpace(cfg.UserAgent)
	if _, _, err = net.SplitHostPort(cfg.Listen); err != nil {
		return cfg, fmt.Errorf("invalid listen: %w", err)
	}
	if cfg.M3U == "" {
		return cfg, fmt.Errorf("m3u cannot be empty")
	}
	if cfg.PublicBase != "" && !validURL(cfg.PublicBase) {
		return cfg, fmt.Errorf("invalid public_base URL")
	}
	duration, err := time.ParseDuration(cfg.CacheDuration)
	if err != nil || duration <= 0 {
		return cfg, fmt.Errorf("cache_duration must be a positive duration such as 3h")
	}
	if cfg.UserAgent == "" || strings.ContainsAny(cfg.UserAgent, "\r\n") {
		return cfg, fmt.Errorf("invalid user_agent")
	}
	return cfg, nil
}
