package main

import (
	"context"
	"time"
)

type probeResult struct {
	duration time.Duration
	err      error
	expires  time.Time
}
type probeFlight struct {
	done   chan struct{}
	result probeResult
}

// Nearby playback requests share one speed check per upstream URL.
func (g *gateway) probe(ctx context.Context, raw string) (time.Duration, error) {
	key := g.upstreamUA() + "\x00" + raw
	g.probeMu.Lock()
	if result, ok := g.probeResults[key]; ok && time.Now().Before(result.expires) {
		g.probeMu.Unlock()
		return result.duration, result.err
	}
	if flight := g.probeFlights[key]; flight != nil {
		g.probeMu.Unlock()
		select {
		case <-flight.done:
			return flight.result.duration, flight.result.err
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	if g.probeFlights == nil {
		g.probeFlights = map[string]*probeFlight{}
	}
	flight := &probeFlight{done: make(chan struct{})}
	g.probeFlights[key] = flight
	g.probeMu.Unlock()
	// The probe may be shared by another viewer; keep its short lifetime
	// independent of the request that happened to start it.
	probeCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	duration, err := g.probeDirect(probeCtx, raw)
	cancel()
	result := probeResult{duration, err, time.Now().Add(500 * time.Millisecond)}
	g.probeMu.Lock()
	if g.probeResults == nil {
		g.probeResults = map[string]probeResult{}
	}
	if len(g.probeResults) > 4096 {
		now := time.Now()
		for oldKey, old := range g.probeResults {
			if now.After(old.expires) {
				delete(g.probeResults, oldKey)
			}
		}
	}
	g.probeResults[key] = result
	flight.result = result
	delete(g.probeFlights, key)
	close(flight.done)
	g.probeMu.Unlock()
	return duration, err
}
