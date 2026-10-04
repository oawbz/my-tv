package main

import (
	"bytes"
	"context"
	"errors"
	_ "golang.org/x/image/webp"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var logoOnce sync.Once
var logoPNG []byte

func defaultLogo() []byte {
	logoOnce.Do(func() {
		img := image.NewRGBA(image.Rect(0, 0, 128, 128))
		dark := color.RGBA{38, 50, 56, 255}
		white := color.RGBA{255, 255, 255, 255}
		for y := 0; y < 128; y++ {
			for x := 0; x < 128; x++ {
				img.Set(x, y, dark)
			}
		}
		for y := 39; y < 89; y++ {
			for x := 26; x < 102; x++ {
				img.Set(x, y, white)
			}
		}
		for y := 49; y < 79; y++ {
			for x := 36; x < 92; x++ {
				img.Set(x, y, dark)
			}
		}
		for y := 54; y < 74; y++ {
			for x := 53; x < 53+(y-54)/2+1; x++ {
				img.Set(x, y, white)
			}
		}
		var b bytes.Buffer
		_ = png.Encode(&b, img)
		logoPNG = b.Bytes()
	})
	return logoPNG
}

const maxLogoBytes = 2 << 20
const maxLogoPixels = 4 * 1024 * 1024
const maxLogoDimension = 4096
const logoOutputDimension = 512

func normalizeLogo(data []byte) ([]byte, error) {
	if len(data) == 0 || len(data) > maxLogoBytes {
		return nil, errors.New("logo too large or empty")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxLogoDimension || config.Height > maxLogoDimension || int64(config.Width)*int64(config.Height) > maxLogoPixels {
		return nil, errors.New("logo dimensions too large")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	width, height := config.Width, config.Height
	if width > logoOutputDimension || height > logoOutputDimension {
		if width >= height {
			height = max(1, height*logoOutputDimension/width)
			width = logoOutputDimension
		} else {
			width = max(1, width*logoOutputDimension/height)
			height = logoOutputDimension
		}
		resized := image.NewNRGBA(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				resized.Set(x, y, img.At(img.Bounds().Min.X+x*config.Width/width, img.Bounds().Min.Y+y*config.Height/height))
			}
		}
		img = resized
	}
	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Src)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, out); err != nil {
		return nil, err
	}
	if encoded.Len() > maxLogoBytes {
		return nil, errors.New("encoded logo too large")
	}
	return encoded.Bytes(), nil
}

func (g *gateway) fetchPNGLogo(raw, _, _ string) (*sharedObject, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", g.upstreamUA())
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > maxLogoBytes {
		return nil, errors.New("invalid logo response")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxLogoBytes+1))
	if err != nil {
		return nil, err
	}
	data, err = normalizeLogo(data)
	if err != nil {
		return nil, err
	}
	return &sharedObject{data: data, status: 200, header: http.Header{"Content-Type": []string{"image/png"}}, expires: time.Now().Add(5 * time.Minute)}, nil
}

func (g *gateway) serveLogo(c *gin.Context, raw string) {
	if validURL(raw) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		obj, err := g.sharedFetchUsing(ctx, raw, "", "", "logo-png-v1\x00", g.fetchPNGLogo)
		if err == nil && obj != nil {
			c.Header("Cache-Control", g.cacheControl("public, max-age=300"))
			c.Data(200, "image/png", obj.data)
			return
		}
	}
	c.Header("Cache-Control", g.cacheControl("public, max-age=60"))
	c.Data(200, "image/png", defaultLogo())
}
