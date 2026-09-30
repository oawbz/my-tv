package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const maxPlaylist = 8 << 20
const maxManifest = 2 << 20
const defaultUpstreamUA = "AptvPlayer/1.4.0"

var attrRE = regexp.MustCompile(`([\w-]+)="([^"]*)"`)
var uriRE = regexp.MustCompile(`URI="([^"]+)"`)

type inputChannel struct {
	Name  string   `json:"name"`
	Group string   `json:"group"`
	URLs  []string `json:"urls"`
	Logo  string   `json:"logo"`
	ID    string   `json:"id"`
}
type channel struct {
	inputChannel
	key      string
	mu       sync.Mutex
	ranked   []string
	badUntil map[string]time.Time
	quality  map[string]*sourceQuality
}
type apiChannel struct {
	Name  string   `json:"name"`
	Group string   `json:"group"`
	URLs  []string `json:"urls"`
	Logo  string   `json:"logo"`
}
type reference struct {
	url        string
	channelKey string
	sourceURL  string
	duration   time.Duration
	expires    time.Time
}
type gateway struct {
	mu           sync.RWMutex
	refreshMu    sync.Mutex
	channels     []*channel
	byKey        map[string]*channel
	refs         map[string]reference
	lastRefSweep time.Time
	loadedAt     time.Time
	nextRetry    time.Time
	sources      []string
	base         string
	userAgent    string
	cacheTTL     time.Duration
	client       *http.Client
	probeClient  *http.Client
	sharedMu     sync.Mutex
	objects      map[string]*sharedObject
	flights      map[string]*sharedFlight
	streams      map[string]*streamHub
	streamMeta   map[string]*sharedObject
	cacheBytes   int
	probeMu      sync.Mutex
	probeResults map[string]probeResult
	probeFlights map[string]*probeFlight
}

func validURL(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil
}
func (g *gateway) upstreamUA() string {
	if g.userAgent != "" {
		return g.userAgent
	}
	return defaultUpstreamUA
}

// ensureFresh is called only by channel listing and playback requests.
// Remote M3U requests use a TTL; local files are reread on listing requests.
func (g *gateway) ensureFresh(ctx context.Context, onListing, force bool) error {
	if len(g.sources) == 0 {
		return nil
	}
	started := time.Now()
	g.refreshMu.Lock()
	defer g.refreshMu.Unlock()
	g.mu.RLock()
	loadedAt, nextRetry := g.loadedAt, g.nextRetry
	g.mu.RUnlock()
	now := time.Now()
	if now.Before(nextRetry) {
		return nil
	}
	remote := validURL(g.sources[0])
	if remote {
		ttl := g.cacheTTL
		if ttl <= 0 {
			ttl = 3 * time.Hour
		}
		if !force && !loadedAt.IsZero() && now.Sub(loadedAt) < ttl {
			return nil
		}
		if force && loadedAt.After(started) {
			return nil
		}
	} else if (!onListing && !force) || loadedAt.After(started) {
		return nil
	}
	refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := g.refresh(refreshCtx); err != nil {
		g.mu.Lock()
		g.nextRetry = time.Now().Add(30 * time.Second)
		g.mu.Unlock()
		return err
	}
	g.mu.Lock()
	g.loadedAt = time.Now()
	g.nextRetry = time.Time{}
	g.mu.Unlock()
	return nil
}
func identity(c inputChannel) string {
	return normalizedName(c.Name)
}
func keyFor(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:8]) }

func parseM3U(data []byte, base *url.URL) ([]inputChannel, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if !bytes.HasPrefix(bytes.TrimSpace(data), []byte("#EXTM3U")) {
		return nil, errors.New("missing #EXTM3U")
	}
	var out []inputChannel
	var pending *inputChannel
	for _, rawLine := range bytes.Split(data, []byte("\n")) {
		if len(rawLine) > 256<<10 {
			pending = nil
			continue
		}
		line := strings.TrimSpace(string(rawLine))
		if strings.HasPrefix(line, "#EXTINF:") {
			left, name, ok := splitEXTINF(line)
			if !ok {
				pending = nil
				continue
			}
			attrs := map[string]string{}
			for _, m := range attrRE.FindAllStringSubmatch(left, -1) {
				attrs[m[1]] = m[2]
			}
			name = strings.TrimSpace(name)
			if name == "" {
				name = attrs["tvg-name"]
			}
			if name == "" {
				pending = nil
				continue
			}
			pending = &inputChannel{Name: name, Group: attrs["group-title"], Logo: attrs["tvg-logo"], ID: attrs["tvg-id"]}
			if pending.Group == "" {
				pending.Group = "其他"
			}
			if pending.Logo != "" && base != nil {
				if u, e := base.Parse(pending.Logo); e == nil {
					pending.Logo = u.String()
				}
			}
		} else if line != "" && !strings.HasPrefix(line, "#") && pending != nil {
			u, e := url.Parse(line)
			if e == nil && base != nil {
				u = base.ResolveReference(u)
			}
			if e == nil && validURL(u.String()) {
				pending.URLs = []string{u.String()}
				out = append(out, *pending)
			}
			pending = nil
		}
	}
	return out, nil
}

func splitEXTINF(line string) (string, string, bool) {
	quoted := false
	for i, r := range line {
		if r == '"' {
			quoted = !quoted
		}
		if r == ',' && !quoted {
			return line[:i], line[i+1:], true
		}
	}
	return "", "", false
}

func (g *gateway) loadSource(ctx context.Context, source string) ([]inputChannel, error) {
	var data []byte
	var err error
	var base *url.URL
	if validURL(source) {
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if e != nil {
			return nil, e
		}
		req.Header.Set("User-Agent", g.upstreamUA())
		resp, e := g.client.Do(req)
		if e != nil {
			return nil, e
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		base = resp.Request.URL
		data, err = io.ReadAll(io.LimitReader(resp.Body, maxPlaylist+1))
	} else {
		file, e := os.Open(source)
		if e != nil {
			return nil, e
		}
		defer file.Close()
		data, err = io.ReadAll(io.LimitReader(file, maxPlaylist+1))
		p, _ := filepath.Abs(source)
		base = &url.URL{Scheme: "file", Path: p}
	}
	if err != nil {
		return nil, err
	}
	if len(data) > maxPlaylist {
		return nil, errors.New("source too large")
	}
	return parseM3U(data, base)
}

func (g *gateway) refresh(ctx context.Context) error {
	merged := map[string]*channel{}
	order := []string{}
	succeeded := 0
	for _, source := range g.sources {
		list, err := g.loadSource(ctx, source)
		if err != nil {
			log.Printf("source %s: %v", source, err)
			continue
		}
		succeeded++
		for _, item := range list {
			item.Name = strings.TrimSpace(item.Name)
			if item.Name == "" {
				continue
			}
			if item.Group == "" {
				item.Group = "其他"
			}
			id := identity(item)
			ch := merged[id]
			if ch == nil {
				ch = &channel{inputChannel: inputChannel{Name: item.Name, Group: item.Group, Logo: item.Logo, ID: item.ID}, key: keyFor(id)}
				merged[id] = ch
				order = append(order, id)
			}
			if !validURL(ch.Logo) && validURL(item.Logo) {
				ch.Logo = item.Logo
			}
			for _, raw := range item.URLs {
				if !validURL(raw) {
					continue
				}
				found := false
				for _, old := range ch.URLs {
					if old == raw {
						found = true
						break
					}
				}
				if !found {
					ch.URLs = append(ch.URLs, raw)
				}
			}
		}
	}
	if succeeded == 0 {
		return errors.New("all sources failed")
	}
	result := make([]*channel, 0, len(order))
	g.mu.RLock()
	old := g.byKey
	g.mu.RUnlock()
	for _, id := range order {
		ch := merged[id]
		if len(ch.URLs) == 0 {
			continue
		}
		if prior := old[ch.key]; prior != nil {
			prior.mu.Lock()
			if strings.Join(prior.URLs, "\x00") == strings.Join(ch.URLs, "\x00") {
				ch.badUntil = make(map[string]time.Time, len(prior.badUntil))
				for raw, until := range prior.badUntil {
					ch.badUntil[raw] = until
				}
				ch.quality = make(map[string]*sourceQuality, len(prior.quality))
				for raw, q := range prior.quality {
					copy := *q
					ch.quality[raw] = &copy
				}
			}
			prior.mu.Unlock()
		}
		result = append(result, ch)
	}
	result = orderChannels(result)
	byKey := make(map[string]*channel, len(result))
	for _, ch := range result {
		byKey[ch.key] = ch
	}
	g.mu.Lock()
	g.channels = result
	g.byKey = byKey
	g.mu.Unlock()
	log.Printf("loaded %d channels", len(result))
	return nil
}

func (g *gateway) probeDirect(ctx context.Context, raw string) (time.Duration, error) {
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if e != nil {
		return 0, e
	}
	req.Header.Set("Range", "bytes=0-1023")
	req.Header.Set("User-Agent", g.upstreamUA())
	start := time.Now()
	resp, e := g.probeClient.Do(req)
	if e != nil {
		return 0, e
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var first [512]byte
	n, e := resp.Body.Read(first[:])
	if n == 0 {
		if e == nil {
			e = io.EOF
		}
		return 0, e
	}
	sample := bytes.ToLower(bytes.TrimSpace(first[:n]))
	if bytes.HasPrefix(sample, []byte("<html")) || bytes.HasPrefix(sample, []byte("<!doctype html")) {
		return 0, errors.New("HTML response")
	}
	return time.Since(start), nil
}
func (g *gateway) ranked(ctx context.Context, ch *channel) []string {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	type result struct {
		url      string
		duration time.Duration
		ok       bool
	}
	results := make([]result, len(ch.URLs))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, raw := range ch.URLs {
		wg.Add(1)
		go func(i int, raw string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			pctx, cancel := context.WithTimeout(ctx, 4*time.Second)
			defer cancel()
			d, e := g.probe(pctx, raw)
			if e != nil {
				g.invalidateShared(raw)
			}
			results[i] = result{raw, d, e == nil}
		}(i, raw)
	}
	wg.Wait()
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].ok != results[j].ok {
			return results[i].ok
		}
		return results[i].duration < results[j].duration
	})
	ranked := make([]string, 0, len(results))
	quarantined := make([]string, 0)
	for _, r := range results {
		if r.ok {
			if time.Now().Before(ch.badUntil[r.url]) {
				quarantined = append(quarantined, r.url)
			} else {
				ranked = append(ranked, r.url)
			}
		}
	}
	if len(ranked) == 0 {
		if len(quarantined) > 0 {
			ranked = quarantined
		} else {
			ranked = append(ranked, ch.URLs...)
		}
	}
	ch.ranked = ranked
	return append([]string(nil), ranked...)
}
func (g *gateway) demote(channelKey, sourceURL string) {
	g.mu.RLock()
	ch := g.byKey[channelKey]
	g.mu.RUnlock()
	if ch == nil {
		return
	}
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.badUntil == nil {
		ch.badUntil = map[string]time.Time{}
	}
	ch.badUntil[sourceURL] = time.Now().Add(2 * time.Minute)
	for i, raw := range ch.ranked {
		if raw == sourceURL {
			ch.ranked = append(ch.ranked[:i], ch.ranked[i+1:]...)
			return
		}
	}
}
func (g *gateway) register(raw, channelKey, sourceURL string, duration time.Duration) string {
	var b [18]byte
	_, _ = rand.Read(b[:])
	token := hex.EncodeToString(b[:])
	g.mu.Lock()
	now := time.Now()
	if now.Sub(g.lastRefSweep) >= time.Minute {
		for key, ref := range g.refs {
			if now.After(ref.expires) {
				delete(g.refs, key)
			}
		}
		g.lastRefSweep = now
	}
	g.refs[token] = reference{raw, channelKey, sourceURL, duration, now.Add(15 * time.Minute)}
	g.mu.Unlock()
	return token
}
func (g *gateway) publicBase(c *gin.Context) string {
	if g.base != "" {
		return g.base
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}
func (g *gateway) resourceURL(base, raw, channelKey, sourceURL string, duration time.Duration) string {
	return base + "/resource/" + g.register(raw, channelKey, sourceURL, duration)
}
func (g *gateway) rewrite(data []byte, upstream *url.URL, base, channelKey, sourceURL string) ([]byte, int) {
	lines := strings.Split(string(data), "\n")
	valid := 0
	var segmentDuration time.Duration
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			if strings.HasPrefix(trimmed, "#EXTINF:") {
				segmentDuration = parseSegmentDuration(trimmed)
			}
			if strings.Contains(trimmed, "URI=\"") {
				lines[i] = uriRE.ReplaceAllStringFunc(line, func(match string) string {
					sub := uriRE.FindStringSubmatch(match)
					u, e := upstream.Parse(sub[1])
					if e != nil || !validURL(u.String()) {
						return match
					}
					if strings.HasPrefix(trimmed, "#EXT-X-MEDIA:") || strings.HasPrefix(trimmed, "#EXT-X-I-FRAME-STREAM-INF:") || strings.HasPrefix(trimmed, "#EXT-X-IMAGE-STREAM-INF:") {
						valid++
					}
					return `URI="` + g.resourceURL(base, u.String(), channelKey, sourceURL, 0) + `"`
				})
			}
		} else {
			u, e := upstream.Parse(trimmed)
			if e == nil && validURL(u.String()) {
				valid++
				lines[i] = g.resourceURL(base, u.String(), channelKey, sourceURL, segmentDuration)
			}
			segmentDuration = 0
		}
	}
	return []byte(strings.Join(lines, "\n")), valid
}
func isManifest(resp *http.Response, reader *bufio.Reader) bool {
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "mpegurl") || strings.HasSuffix(strings.ToLower(resp.Request.URL.Path), ".m3u8") {
		return true
	}
	peek, _ := reader.Peek(7)
	return bytes.Equal(peek, []byte("#EXTM3U"))
}
func (g *gateway) relay(c *gin.Context, raw, channelKey, sourceURL string, segmentDuration ...time.Duration) error {
	var duration time.Duration
	if len(segmentDuration) > 0 {
		duration = segmentDuration[0]
	}
	var obj *sharedObject
	for attempt := 0; attempt < 2; attempt++ {
		fetched, err := g.sharedFetch(c.Request.Context(), raw, c.GetHeader("Range"), c.GetHeader("If-Range"))
		if err != nil {
			return err
		}
		obj = fetched
		if duration > 0 {
			if obj.hub != nil {
				obj.hub.setQualityObserver(g, channelKey, sourceURL, duration)
			} else if !obj.manifest && obj.observed.CompareAndSwap(false, true) {
				g.observeSegment(channelKey, sourceURL, duration, obj.fetchDuration, nil)
			}
		}
		if obj.hub == nil {
			break
		}
		if err := obj.hub.serve(c, obj); err == nil {
			return nil
		} else if attempt == 1 {
			return err
		}
	}
	if obj.manifest {
		var valid int
		data, valid := g.rewrite(obj.data, obj.url, g.publicBase(c), channelKey, sourceURL)
		if valid == 0 {
			g.invalidateShared(raw)
			return errors.New("HLS manifest has no playable URLs")
		}
		c.Header("Content-Type", "application/vnd.apple.mpegurl")
		c.Header("Cache-Control", "no-store")
		c.Data(http.StatusOK, "application/vnd.apple.mpegurl", data)
		return nil
	}
	for _, h := range []string{"Content-Type", "Content-Range", "Accept-Ranges"} {
		if v := obj.header.Get(h); v != "" {
			c.Header(h, v)
		}
	}
	c.Data(obj.status, obj.header.Get("Content-Type"), obj.data)
	return nil
}
func (g *gateway) playCandidates(c *gin.Context, ch *channel) bool {
	for _, raw := range g.ranked(c.Request.Context(), ch) {
		if err := g.relay(c, raw, ch.key, raw, 0); err == nil {
			return true
		} else {
			g.demote(ch.key, raw)
			log.Printf("play %s: %v", ch.Name, err)
			if c.Writer.Written() {
				return true
			}
		}
	}
	return false
}
func (g *gateway) router() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/healthz", func(c *gin.Context) {
		g.mu.RLock()
		n := len(g.channels)
		g.mu.RUnlock()
		c.JSON(200, gin.H{"channels": n})
	})
	r.GET("/channels.json", func(c *gin.Context) {
		if err := g.ensureFresh(c.Request.Context(), true, false); err != nil {
			log.Printf("M3U refresh: %v", err)
		}
		base := g.publicBase(c)
		g.mu.RLock()
		chs := append([]*channel(nil), g.channels...)
		g.mu.RUnlock()
		out := make([]apiChannel, 0, len(chs))
		for _, ch := range chs {
			logo := base + "/logo/" + ch.key
			out = append(out, apiChannel{ch.Name, ch.Group, []string{base + "/play/" + ch.key + "/index.m3u8"}, logo})
		}
		c.Header("Cache-Control", "public, max-age=60")
		c.JSON(200, gin.H{"version": 1, "channels": out})
	})
	r.GET("/play/:key/index.m3u8", func(c *gin.Context) {
		if err := g.ensureFresh(c.Request.Context(), false, false); err != nil {
			log.Printf("M3U refresh: %v", err)
		}
		g.mu.RLock()
		ch := g.byKey[c.Param("key")]
		g.mu.RUnlock()
		if ch == nil {
			c.Status(404)
			return
		}
		if g.playCandidates(c, ch) {
			return
		}
		if err := g.ensureFresh(c.Request.Context(), false, true); err != nil {
			log.Printf("M3U repair: %v", err)
		}
		g.mu.RLock()
		updated := g.byKey[ch.key]
		g.mu.RUnlock()
		if updated != nil && strings.Join(updated.URLs, "\x00") != strings.Join(ch.URLs, "\x00") && g.playCandidates(c, updated) {
			return
		}
		if !c.Writer.Written() {
			c.Status(http.StatusBadGateway)
		}
	})
	r.GET("/logo/:key", func(c *gin.Context) {
		g.mu.RLock()
		ch := g.byKey[c.Param("key")]
		g.mu.RUnlock()
		if ch == nil {
			c.Status(404)
			return
		}
		g.serveLogo(c, ch.Logo)
	})
	r.GET("/resource/:token", func(c *gin.Context) {
		g.mu.RLock()
		ref, ok := g.refs[c.Param("token")]
		g.mu.RUnlock()
		if !ok || time.Now().After(ref.expires) {
			c.Status(404)
			return
		}
		if e := g.relay(c, ref.url, ref.channelKey, ref.sourceURL, ref.duration); e != nil {
			g.demote(ref.channelKey, ref.sourceURL)
			if !c.Writer.Written() {
				c.Status(502)
			}
		}
	})
	return r
}
func main() {
	gin.SetMode(gin.ReleaseMode)
	cfg, err := loadConfig("config.json")
	if err != nil {
		log.Fatal(err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Some IPTV origins send unsolicited redirects on idle HTTP/1.1 connections.
	// Do not return upstream connections to the idle pool.
	transport.DisableKeepAlives = true
	transport.ResponseHeaderTimeout = 10 * time.Second
	cacheTTL, _ := time.ParseDuration(cfg.CacheDuration)
	g := &gateway{sources: []string{cfg.M3U}, base: cfg.PublicBase, userAgent: cfg.UserAgent, cacheTTL: cacheTTL, client: &http.Client{Transport: transport}, probeClient: &http.Client{Transport: transport, Timeout: 4 * time.Second}, byKey: map[string]*channel{}, refs: map[string]reference{}}
	log.Printf("listening on %s, public base %s", cfg.Listen, g.base)
	log.Fatal(http.ListenAndServe(cfg.Listen, g.router()))
}
