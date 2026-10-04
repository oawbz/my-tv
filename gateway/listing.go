package main

import (
	"log"
	"strings"

	"github.com/gin-gonic/gin"
)

func (g *gateway) listedChannels(c *gin.Context) []*channel {
	g.ensureCatalogFresh(c.Request.Context())
	if err := g.ensureFresh(c.Request.Context(), true, false); err != nil {
		log.Printf("M3U refresh: %v", err)
	}
	g.mu.RLock()
	channels := append([]*channel(nil), g.channels...)
	g.mu.RUnlock()
	return channels
}

var m3uFieldReplacer = strings.NewReplacer("\r", " ", "\n", " ", `"`, "'")

func m3uField(value string) string {
	return m3uFieldReplacer.Replace(value)
}
