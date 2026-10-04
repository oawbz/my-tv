package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNormalizeLogoFormatsAndBounds(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1024, 256))
	img.Set(2, 2, color.NRGBA{255, 0, 0, 255})
	for _, format := range []string{"png", "jpeg", "gif", "webp"} {
		t.Run(format, func(t *testing.T) {
			var b bytes.Buffer
			switch format {
			case "png":
				png.Encode(&b, img)
			case "jpeg":
				jpeg.Encode(&b, img, nil)
			case "gif":
				gif.Encode(&b, img, nil)
			case "webp":
				data, e := os.ReadFile("testdata/cctv3.webp")
				if e != nil {
					t.Fatal(e)
				}
				b.Write(data)
			}
			data, e := normalizeLogo(b.Bytes())
			if e != nil {
				t.Fatal(e)
			}
			cfg, e := png.DecodeConfig(bytes.NewReader(data))
			if e != nil || cfg.Width > 512 || cfg.Height > 512 {
				t.Fatalf("bad normalized PNG: %+v %v", cfg, e)
			}
		})
	}
	for _, data := range [][]byte{[]byte("not an image"), make([]byte, maxLogoBytes+1), defaultLogo()[:20]} {
		if _, e := normalizeLogo(data); e == nil {
			t.Fatal("invalid logo accepted")
		}
	}
	var huge bytes.Buffer
	png.Encode(&huge, image.NewGray(image.Rect(0, 0, 4097, 1)))
	if _, e := normalizeLogo(huge.Bytes()); e == nil {
		t.Fatal("oversized dimensions accepted")
	}
}
func TestLogoConversionCachedAndCoalesced(t *testing.T) {
	webp, e := os.ReadFile("testdata/cctv3.webp")
	if e != nil {
		t.Fatal(e)
	}
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		w.Write(webp)
	}))
	defer upstream.Close()
	g := &gateway{client: &http.Client{Timeout: time.Second}}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			obj, e := g.sharedFetchUsing(context.Background(), upstream.URL, "", "", "logo-png-v1\x00", g.fetchPNGLogo)
			if e != nil {
				t.Error(e)
				return
			}
			if _, e := png.DecodeConfig(bytes.NewReader(obj.data)); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if requests.Load() != 1 {
		t.Fatalf("upstream fetched %d times", requests.Load())
	}
}
