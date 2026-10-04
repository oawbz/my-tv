package main

import (
	"fmt"
	"strings"
)

type channelConfig struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Group   string   `json:"group,omitempty"`
	Aliases []string `json:"aliases,omitempty"`
	Logo    string   `json:"logo"`
}

type catalogEntry struct {
	id, name, group, logo, key string
}

type channelCatalog struct {
	ordered []*catalogEntry
	byName  map[string]*catalogEntry
}

func buildCatalog(channels []channelConfig) (*channelCatalog, error) {
	if channels == nil {
		return nil, fmt.Errorf("channels is required")
	}
	result := &channelCatalog{byName: make(map[string]*catalogEntry)}
	ids := make(map[string]bool)
	for _, cfg := range channels {
		id := strings.TrimSpace(cfg.ID)
		name := strings.TrimSpace(cfg.Name)
		group := strings.TrimSpace(cfg.Group)
		logo := strings.TrimSpace(cfg.Logo)
		if id == "" || strings.ContainsAny(id+name+group+logo, "\r\n") {
			return nil, fmt.Errorf("invalid channel id/name/group/logo for %q", cfg.ID)
		}
		if name == "" {
			name = id
		}
		if logo != "" && !validURL(logo) {
			return nil, fmt.Errorf("invalid logo for channel %q", id)
		}
		normalizedID := normalizedName(id)
		if ids[normalizedID] {
			return nil, fmt.Errorf("duplicate channel id %q", id)
		}
		ids[normalizedID] = true
		entry := &catalogEntry{id: id, name: name, group: group, logo: logo, key: keyFor(normalizedID)}
		for _, alias := range append([]string{id, name}, cfg.Aliases...) {
			alias = strings.TrimSpace(alias)
			key := normalizedName(alias)
			if key == "" || strings.ContainsAny(alias, "\r\n") {
				return nil, fmt.Errorf("invalid alias for channel %q", id)
			}
			if previous := result.byName[key]; previous != nil && previous != entry {
				return nil, fmt.Errorf("channel name %q conflicts with %q", alias, previous.id)
			}
			result.byName[key] = entry
		}
		result.ordered = append(result.ordered, entry)
	}
	return result, nil
}

var fallbackCatalog = func() *channelCatalog {
	catalog, err := buildCatalog(defaultChannels())
	if err != nil {
		panic(err)
	}
	return catalog
}()
