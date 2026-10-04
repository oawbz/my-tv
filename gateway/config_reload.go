package main

import (
	"context"
	"crypto/sha256"
	"log"
	"os"
	"time"
)

// Reload the channel table on demand. A partial or invalid edit leaves the
// previous table available until the next changed file is accessed.
func (g *gateway) ensureCatalogFresh(ctx context.Context) {
	if g.configPath == "" {
		return
	}
	data, err := os.ReadFile(g.configPath)
	if err != nil {
		return
	}
	hash := sha256.Sum256(data)
	// A remote refresh may hold this lock while waiting on the upstream.
	// Keep serving the current catalog and check edits on the next request.
	if !g.refreshMu.TryLock() {
		return
	}
	defer g.refreshMu.Unlock()
	if hash == g.configHash || hash == g.rejectedHash {
		return
	}
	cfg, err := parseConfig(data)
	if err != nil {
		g.rejectedHash = hash
		log.Printf("config reload: %v; keeping previous channels", err)
		return
	}
	catalog, err := buildCatalog(cfg.Channels)
	if err != nil {
		g.rejectedHash = hash
		log.Printf("config reload: %v; keeping previous channels", err)
		return
	}
	previous := g.catalog
	g.catalog = catalog
	g.mu.RLock()
	loaded := !g.loadedAt.IsZero()
	g.mu.RUnlock()
	if loaded {
		refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = g.runRefresh(refreshCtx, false)
		cancel()
		if err != nil {
			g.catalog = previous
			log.Printf("config reload: %v; keeping previous channels", err)
			return
		}
	}
	g.configHash = hash
	g.rejectedHash = [32]byte{}
	log.Printf("loaded channel config (%d entries)", len(catalog.ordered))
}
