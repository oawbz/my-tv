package main

import (
	"context"
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

const goodSourceTTL = 5 * time.Minute

func (ch *channel) recentGood() string {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if time.Since(ch.goodAt) > goodSourceTTL || time.Now().Before(ch.badUntil[ch.lastGood]) {
		return ""
	}
	return ch.lastGood
}

func (ch *channel) candidates(exclude string) []string {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	healthy := make([]string, 0, len(ch.URLs))
	quarantined := make([]string, 0)
	now := time.Now()
	for _, raw := range ch.URLs {
		if raw == exclude {
			continue
		}
		if now.Before(ch.badUntil[raw]) {
			quarantined = append(quarantined, raw)
		} else {
			healthy = append(healthy, raw)
		}
	}
	if len(healthy) > 0 {
		return healthy
	}
	return quarantined
}

func (g *gateway) trySource(c *gin.Context, ch *channel, raw string) bool {
	if err := g.relay(c, raw, ch.key, raw, 0); err != nil {
		g.demote(ch.key, raw)
		log.Printf("play %s: %v", ch.Name, err)
		return c.Writer.Written()
	}
	g.markGood(ch.key, raw)
	return true
}

func (g *gateway) markGood(channelKey, raw string) {
	g.mu.RLock()
	ch := g.byKey[channelKey]
	g.mu.RUnlock()
	if ch == nil {
		return
	}
	ch.mu.Lock()
	found := false
	for _, candidate := range ch.URLs {
		if candidate == raw {
			found = true
			break
		}
	}
	if !found {
		ch.mu.Unlock()
		return
	}
	ch.lastGood, ch.goodAt = raw, time.Now()
	ch.mu.Unlock()
}

type probeOutcome struct {
	raw string
	err error
}

// Race probes when no recent source is known. The first source to respond is
// tried immediately; slow probes cannot hold back an already playable source.
func (g *gateway) playCandidates(c *gin.Context, ch *channel) bool {
	good := ch.recentGood()
	if good != "" && g.trySource(c, ch, good) {
		return true
	}
	candidates := ch.candidates(good)
	if len(candidates) == 0 {
		return false
	}
	if len(candidates) == 1 {
		return g.trySource(c, ch, candidates[0])
	}
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	results := make(chan probeOutcome, len(candidates))
	sem := make(chan struct{}, 4)
	for _, raw := range candidates {
		go func(raw string) {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			_, err := g.probe(ctx, raw)
			if err != nil {
				g.invalidateShared(raw)
			}
			select {
			case results <- probeOutcome{raw, err}:
			case <-ctx.Done():
			}
		}(raw)
	}
	// A buffered channel lets remaining probes finish without blocking the
	// playback request after a source succeeds.
	tried := make(map[string]bool, len(candidates))
	for range candidates {
		var result probeOutcome
		select {
		case result = <-results:
		case <-ctx.Done():
			return false
		}
		if result.err != nil {
			continue
		}
		tried[result.raw] = true
		if g.trySource(c, ch, result.raw) {
			return true
		}
	}
	// Some origins reject Range probes but accept a normal playback GET.
	// Preserve that fallback only when every successful probe has failed.
	for _, raw := range candidates {
		if !tried[raw] && g.trySource(c, ch, raw) {
			return true
		}
	}
	return false
}
