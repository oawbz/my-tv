package main

import (
	"strconv"
	"strings"
	"time"
)

// A slow segment is only actionable after several consecutive upstream
// downloads. Samples come from shared upstream fetches, not each viewer.
type sourceQuality struct {
	slowStreak int
	lastSlow   time.Time
}

func parseSegmentDuration(line string) time.Duration {
	value := strings.TrimPrefix(line, "#EXTINF:")
	if comma := strings.IndexByte(value, ','); comma >= 0 {
		value = value[:comma]
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || seconds <= 0 || seconds > 60 {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}

func (g *gateway) observeSegment(channelKey, sourceURL string, duration, elapsed time.Duration, fetchErr error) {
	if duration <= 0 || channelKey == "" || sourceURL == "" {
		return
	}
	g.mu.RLock()
	ch := g.byKey[channelKey]
	g.mu.RUnlock()
	if ch == nil {
		return
	}
	ch.mu.Lock()
	if ch.quality == nil {
		ch.quality = make(map[string]*sourceQuality)
	}
	q := ch.quality[sourceURL]
	if q == nil {
		q = &sourceQuality{}
		ch.quality[sourceURL] = q
	}
	threshold := duration + duration/2
	if threshold < 3*time.Second {
		threshold = 3 * time.Second
	}
	now := time.Now()
	if fetchErr != nil || elapsed > threshold {
		if now.Sub(q.lastSlow) > 90*time.Second {
			q.slowStreak = 0
		}
		q.slowStreak++
		q.lastSlow = now
	} else {
		q.slowStreak = 0
	}
	demote := fetchErr != nil || q.slowStreak >= 3
	if demote {
		q.slowStreak = 0
	}
	ch.mu.Unlock()
	if demote {
		g.demote(channelKey, sourceURL)
	}
}
