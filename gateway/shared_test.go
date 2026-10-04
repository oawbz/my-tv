package main

import (
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrentHLSViewersShareUpstream(t *testing.T) {
	var probes, manifests, segments, logos atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/live.m3u8":
			if r.Header.Get("Range") != "" {
				probes.Add(1)
			} else {
				manifests.Add(1)
				time.Sleep(80 * time.Millisecond)
			}
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			io.WriteString(w, "#EXTM3U\nsegment.ts\n")
		case "/segment.ts":
			segments.Add(1)
			time.Sleep(80 * time.Millisecond)
			w.Header().Set("Content-Type", "video/mp2t")
			io.WriteString(w, "segment-bytes")
		case "/logo":
			logos.Add(1)
			time.Sleep(80 * time.Millisecond)
			w.Header().Set("Content-Type", "image/png")
			w.Write(defaultLogo())
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	ch := &channel{inputChannel: inputChannel{Name: "CCTV1 综合", URLs: []string{upstream.URL + "/live.m3u8"}, Logo: upstream.URL + "/logo"}, key: "one"}
	g := &gateway{channels: []*channel{ch}, byKey: map[string]*channel{"one": ch}, refs: map[string]reference{}, client: &http.Client{Timeout: time.Second}, probeClient: &http.Client{Timeout: time.Second}}
	server := httptest.NewServer(g.router())
	defer server.Close()
	const viewers = 12
	urls := make([]string, viewers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < viewers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			r, e := http.Get(server.URL + "/play/one/index.m3u8")
			if e != nil {
				t.Error(e)
				return
			}
			b, _ := io.ReadAll(r.Body)
			r.Body.Close()
			if r.StatusCode != 200 {
				t.Errorf("playlist status %d", r.StatusCode)
				return
			}
			parts := strings.Split(strings.TrimSpace(string(b)), "\n")
			urls[i] = parts[len(parts)-1]
		}(i)
	}
	close(start)
	wg.Wait()
	if probes.Load() != 0 || manifests.Load() != 1 {
		t.Fatalf("upstream probes=%d manifests=%d", probes.Load(), manifests.Load())
	}
	start = make(chan struct{})
	for i := 0; i < viewers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			r, e := http.Get(urls[i])
			if e != nil {
				t.Error(e)
				return
			}
			b, _ := io.ReadAll(r.Body)
			r.Body.Close()
			if r.StatusCode != 200 || string(b) != "segment-bytes" {
				t.Errorf("segment %d: %s", r.StatusCode, b)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if segments.Load() != 1 {
		t.Fatalf("upstream segments=%d", segments.Load())
	}
	start = make(chan struct{})
	for i := 0; i < viewers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r, e := http.Get(server.URL + "/logo/one")
			if e != nil {
				t.Error(e)
				return
			}
			b, _ := io.ReadAll(r.Body)
			r.Body.Close()
			if r.StatusCode != 200 || len(b) == 0 {
				t.Errorf("logo status=%d", r.StatusCode)
			}
		}()
	}
	close(start)
	wg.Wait()
	if logos.Load() != 1 {
		t.Fatalf("upstream logos=%d", logos.Load())
	}
}

func TestConcurrentDirectViewersBroadcast(t *testing.T) {
	var opens atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		opens.Add(1)
		w.Header().Set("Content-Type", "video/mp2t")
		w.WriteHeader(200)
		time.Sleep(80 * time.Millisecond)
		for i := 0; i < 8; i++ {
			if _, e := io.WriteString(w, "packet"); e != nil {
				return
			}
			w.(http.Flusher).Flush()
			time.Sleep(20 * time.Millisecond)
		}
	}))
	defer upstream.Close()
	g := &gateway{client: &http.Client{}, refs: map[string]reference{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := gin.CreateTestContext(w)
		c.Request = r
		if e := g.relay(c, upstream.URL+"/stream", "", ""); e != nil {
			t.Error(e)
		}
	}))
	defer server.Close()
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r, e := http.Get(server.URL)
			if e != nil {
				t.Error(e)
				return
			}
			b, _ := io.ReadAll(r.Body)
			r.Body.Close()
			if r.StatusCode != 200 || len(b) == 0 {
				t.Errorf("broadcast %d %d", r.StatusCode, len(b))
			}
		}()
	}
	close(start)
	wg.Wait()
	if opens.Load() != 1 {
		t.Fatalf("upstream direct connections=%d", opens.Load())
	}
}
