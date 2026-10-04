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
	Listen        string          `json:"listen"`
	M3USources    []string        `json:"m3u_sources"`
	Channels      []channelConfig `json:"channels"`
	Tokens        []string        `json:"tokens"`
	CacheDuration string          `json:"cache_duration"`
	UserAgent     string          `json:"user_agent"`
}

func defaultConfig() config {
	return config{Listen: "0.0.0.0:2219", M3USources: []string{"./index.m3u"}, Channels: defaultChannels(), Tokens: []string{}, CacheDuration: "3h", UserAgent: defaultUpstreamUA}
}

func (cfg config) sources() []string {
	input := cfg.M3USources
	seen := make(map[string]bool, len(input))
	out := make([]string, 0, len(input))
	for _, source := range input {
		if !seen[source] {
			seen[source] = true
			out = append(out, source)
		}
	}
	return out
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
	return parseConfig(data)
}

func parseConfig(data []byte) (config, error) {
	cfg := defaultConfig()
	// JSON decoding can reuse existing slice elements and retain omitted fields.
	// Start required lists empty so removed aliases cannot survive a reload.
	cfg.Channels = nil
	cfg.M3USources = nil
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return cfg, fmt.Errorf("config.json: %w", err)
	}
	if _, legacy := fields["m3u"]; legacy {
		return cfg, fmt.Errorf("config.json: use m3u_sources instead of m3u")
	}
	if _, legacy := fields["public_base"]; legacy {
		return cfg, fmt.Errorf("config.json: public_base is no longer supported")
	}
	if _, legacy := fields["channel_aliases"]; legacy {
		return cfg, fmt.Errorf("config.json: use channels[].aliases instead of channel_aliases")
	}
	if _, present := fields["m3u_sources"]; !present {
		return cfg, fmt.Errorf("config.json: m3u_sources is required")
	}
	if _, present := fields["channels"]; !present {
		return cfg, fmt.Errorf("config.json: channels is required")
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("config.json: %w", err)
	}
	cfg.Listen = strings.TrimSpace(cfg.Listen)
	for i := range cfg.M3USources {
		cfg.M3USources[i] = strings.TrimSpace(cfg.M3USources[i])
	}
	cfg.CacheDuration = strings.TrimSpace(cfg.CacheDuration)
	cfg.UserAgent = strings.TrimSpace(cfg.UserAgent)
	if _, _, err := net.SplitHostPort(cfg.Listen); err != nil {
		return cfg, fmt.Errorf("invalid listen: %w", err)
	}
	if len(cfg.sources()) == 0 {
		return cfg, fmt.Errorf("m3u_sources must contain at least one source")
	}
	for _, source := range cfg.sources() {
		if source == "" || strings.ContainsAny(source, "\r\n") {
			return cfg, fmt.Errorf("invalid m3u source")
		}
	}
	for _, token := range cfg.Tokens {
		if token == "" || strings.TrimSpace(token) != token || strings.ContainsAny(token, "\r\n") {
			return cfg, fmt.Errorf("tokens must contain nonempty values without surrounding whitespace")
		}
	}
	duration, err := time.ParseDuration(cfg.CacheDuration)
	if err != nil || duration <= 0 {
		return cfg, fmt.Errorf("cache_duration must be a positive duration such as 3h")
	}
	if cfg.UserAgent == "" || strings.ContainsAny(cfg.UserAgent, "\r\n") {
		return cfg, fmt.Errorf("invalid user_agent")
	}
	if _, err := buildCatalog(cfg.Channels); err != nil {
		return cfg, err
	}
	return cfg, nil
}
