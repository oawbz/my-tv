package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
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

func (g *gateway) serveLogo(c *gin.Context, raw string) {
	if validURL(raw) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		obj, err := g.sharedFetch(ctx, raw, "", "")
		if err == nil && obj != nil {
			if obj.hub != nil {
				obj.hub.once.Do(func() { close(obj.hub.ready) })
				obj.hub.cancel()
				obj.hub.body.Close()
			} else if len(obj.data) <= 2<<20 {
				ct := http.DetectContentType(obj.data)
				if ct == "image/png" || ct == "image/jpeg" || ct == "image/gif" || ct == "image/webp" {
					c.Header("Cache-Control", g.cacheControl("public, max-age=300"))
					c.Data(200, ct, obj.data)
					return
				}
			}
		}
	}
	c.Header("Cache-Control", g.cacheControl("public, max-age=60"))
	c.Data(200, "image/png", defaultLogo())
}
