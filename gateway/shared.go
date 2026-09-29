package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const maxSharedObject = 8 << 20
const maxSharedCache = 128 << 20

type sharedObject struct {
	data     []byte
	manifest bool
	status   int
	url      *url.URL
	header   http.Header
	hub      *streamHub
	expires  time.Time
}
type sharedFlight struct {
	done   chan struct{}
	object *sharedObject
	err    error
}

func (g *gateway) invalidateShared(raw string) {
	key := g.upstreamUA() + "\x00" + raw + "\x00\x00"
	g.sharedMu.Lock()
	if item := g.objects[key]; item != nil {
		g.cacheBytes -= len(item.data)
		delete(g.objects, key)
	}
	g.sharedMu.Unlock()
}

type streamHub struct {
	mu          sync.Mutex
	ready       chan struct{}
	done        chan struct{}
	once        sync.Once
	clients     map[chan []byte]struct{}
	history     [][]byte
	historySize int
	replay      bool
	closed      bool
	cancel      context.CancelFunc
	body        io.ReadCloser
	bodyReader  *bufio.Reader
}

func (h *streamHub) broadcast(g *gateway, key string, reader io.Reader, meta *sharedObject) {
	<-h.ready
	defer h.body.Close()
	buffer := make([]byte, 32<<10)
	complete := true
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			chunk := append([]byte(nil), buffer[:n]...)
			h.mu.Lock()
			if h.replay && h.historySize+len(chunk) <= maxSharedObject {
				h.history = append(h.history, chunk)
				h.historySize += len(chunk)
			} else {
				h.replay = false
				h.history = nil
				h.historySize = 0
			}
			for client := range h.clients {
				select {
				case client <- chunk:
				default:
					close(client)
					delete(h.clients, client)
				}
			}
			empty := len(h.clients) == 0
			h.mu.Unlock()
			if empty {
				complete = false
				break
			}
		}
		if err != nil {
			if err != io.EOF {
				complete = false
			}
			break
		}
	}
	h.cancel()
	h.mu.Lock()
	cached := []byte(nil)
	if complete && h.replay {
		for _, chunk := range h.history {
			cached = append(cached, chunk...)
		}
	}
	h.closed = true
	for client := range h.clients {
		close(client)
		delete(h.clients, client)
	}
	h.mu.Unlock()
	g.sharedMu.Lock()
	if g.streams[key] == h {
		delete(g.streams, key)
	}
	delete(g.streamMeta, key)
	if complete && h.replay && len(cached) > 0 {
		copyMeta := *meta
		copyMeta.hub = nil
		copyMeta.data = cached
		copyMeta.expires = time.Now().Add(5 * time.Minute)
		g.storeObjectLocked(key, &copyMeta)
	}
	g.sharedMu.Unlock()
	close(h.done)
}

func (h *streamHub) serve(c *gin.Context, meta *sharedObject) error {
	ch := make(chan []byte, 256)
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return errors.New("shared stream ended")
	}
	history := append([][]byte(nil), h.history...)
	h.clients[ch] = struct{}{}
	h.once.Do(func() { close(h.ready) })
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		if _, ok := h.clients[ch]; ok {
			delete(h.clients, ch)
			close(ch)
		}
		empty := len(h.clients) == 0
		h.mu.Unlock()
		if empty {
			h.cancel()
			h.body.Close()
		}
	}()
	for _, header := range []string{"Content-Type", "Content-Range", "Accept-Ranges"} {
		if v := meta.header.Get(header); v != "" {
			c.Header(header, v)
		}
	}
	c.Status(meta.status)
	send := func(data []byte) bool {
		if _, err := c.Writer.Write(data); err != nil {
			return false
		}
		c.Writer.Flush()
		return true
	}
	for _, chunk := range history {
		if !send(chunk) {
			return nil
		}
	}
	for {
		select {
		case chunk, ok := <-ch:
			if !ok {
				return nil
			}
			if !send(chunk) {
				return nil
			}
		case <-c.Request.Context().Done():
			return nil
		}
	}
}

func (g *gateway) storeObjectLocked(key string, obj *sharedObject) {
	if len(obj.data) > maxSharedCache {
		return
	}
	if g.objects == nil {
		g.objects = map[string]*sharedObject{}
	}
	now := time.Now()
	for k, item := range g.objects {
		if now.After(item.expires) {
			g.cacheBytes -= len(item.data)
			delete(g.objects, k)
		}
	}
	if g.cacheBytes+len(obj.data) > maxSharedCache {
		g.objects = map[string]*sharedObject{}
		g.cacheBytes = 0
	}
	if existing := g.objects[key]; existing != nil {
		g.cacheBytes -= len(existing.data)
	}
	g.objects[key] = obj
	g.cacheBytes += len(obj.data)
}

func (g *gateway) sharedFetch(ctx context.Context, raw, rangeHeader, ifRange string) (*sharedObject, error) {
	key := g.upstreamUA() + "\x00" + raw + "\x00" + rangeHeader + "\x00" + ifRange
	g.sharedMu.Lock()
	if item := g.objects[key]; item != nil && time.Now().Before(item.expires) {
		g.sharedMu.Unlock()
		return item, nil
	}
	if hub := g.streams[key]; hub != nil {
		hub.mu.Lock()
		closed := hub.closed
		hub.mu.Unlock()
		if !closed {
			obj := g.streamMeta[key]
			g.sharedMu.Unlock()
			return obj, nil
		}
		done := hub.done
		g.sharedMu.Unlock()
		select {
		case <-done:
			return g.sharedFetch(ctx, raw, rangeHeader, ifRange)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if flight := g.flights[key]; flight != nil {
		g.sharedMu.Unlock()
		select {
		case <-flight.done:
			return flight.object, flight.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if g.flights == nil {
		g.flights = map[string]*sharedFlight{}
	}
	flight := &sharedFlight{done: make(chan struct{})}
	g.flights[key] = flight
	g.sharedMu.Unlock()
	obj, err := g.fetchObject(raw, rangeHeader, ifRange)
	g.sharedMu.Lock()
	flight.object = obj
	flight.err = err
	delete(g.flights, key)
	close(flight.done)
	if err == nil && obj != nil {
		if obj.hub != nil {
			if g.streams == nil {
				g.streams = map[string]*streamHub{}
				g.streamMeta = map[string]*sharedObject{}
			}
			g.streams[key] = obj.hub
			g.streamMeta[key] = obj
		} else if len(obj.data) > 0 {
			g.storeObjectLocked(key, obj)
		}
	}
	g.sharedMu.Unlock()
	if obj != nil && obj.hub != nil {
		go obj.hub.broadcast(g, key, obj.hub.bodyReader, obj)
	}
	return obj, err
}

func (g *gateway) fetchObject(raw, rangeHeader, ifRange string) (*sharedObject, error) {
	ctx, cancel := context.WithCancel(context.Background())
	timer := time.AfterFunc(20*time.Second, cancel)
	defer timer.Stop()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("User-Agent", g.upstreamUA())
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	if ifRange != "" {
		req.Header.Set("If-Range", ifRange)
	}
	resp, err := g.client.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("upstream HTTP %d", resp.StatusCode)
	}
	reader := bufio.NewReader(resp.Body)
	obj := &sharedObject{status: resp.StatusCode, url: resp.Request.URL, header: resp.Header.Clone()}
	if isManifest(resp, reader) {
		data, err := io.ReadAll(io.LimitReader(reader, maxManifest+1))
		resp.Body.Close()
		cancel()
		if err != nil {
			return nil, err
		}
		if len(data) > maxManifest {
			return nil, errors.New("manifest too large")
		}
		if !bytes.HasPrefix(bytes.TrimSpace(data), []byte("#EXTM3U")) {
			return nil, errors.New("invalid HLS manifest")
		}
		obj.data = data
		obj.manifest = true
		obj.expires = time.Now().Add(time.Second)
		return obj, nil
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	bufferable := resp.ContentLength >= 0 && resp.ContentLength <= maxSharedObject
	if strings.HasPrefix(contentType, "image/") {
		bufferable = true
	}
	if bufferable {
		data, err := io.ReadAll(io.LimitReader(reader, maxSharedObject+1))
		resp.Body.Close()
		cancel()
		if err != nil {
			return nil, err
		}
		if len(data) > maxSharedObject {
			return nil, errors.New("media object too large")
		}
		if len(data) == 0 {
			return nil, errors.New("empty media object")
		}
		obj.data = data
		obj.expires = time.Now().Add(5 * time.Minute)
		return obj, nil
	}
	replay := strings.HasSuffix(strings.ToLower(resp.Request.URL.Path), ".ts") || strings.HasSuffix(strings.ToLower(resp.Request.URL.Path), ".m4s") || strings.HasSuffix(strings.ToLower(resp.Request.URL.Path), ".aac")
	hub := &streamHub{ready: make(chan struct{}), done: make(chan struct{}), clients: map[chan []byte]struct{}{}, replay: replay, cancel: cancel, body: resp.Body, bodyReader: reader}
	obj.hub = hub
	return obj, nil
}
