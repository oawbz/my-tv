package main

import (
	"crypto/subtle"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

const tokenQueryKey = "toke"

func (g *gateway) validToken(value string) bool {
	if len(g.tokens) == 0 {
		return true
	}
	valid := 0
	for _, allowed := range g.tokens {
		valid |= subtle.ConstantTimeCompare([]byte(value), []byte(allowed))
	}
	return valid == 1
}

func (g *gateway) requireToken(c *gin.Context) {
	if c.Request.URL.Path == "/" || len(g.tokens) == 0 {
		c.Next()
		return
	}
	if !g.validToken(c.Query(tokenQueryKey)) {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Next()
}

func (g *gateway) withToken(rawURL, token string) string {
	if len(g.tokens) == 0 {
		return rawURL
	}
	return rawURL + "?" + tokenQueryKey + "=" + url.QueryEscape(token)
}

func (g *gateway) cacheControl(publicValue string) string {
	if len(g.tokens) != 0 {
		return "private, no-store"
	}
	return publicValue
}
