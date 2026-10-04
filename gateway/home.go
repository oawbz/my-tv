package main

import (
	"bytes"
	"html/template"
	"net/http"

	"github.com/gin-gonic/gin"
)

var homeTemplate = template.Must(template.New("home").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="referrer" content="no-referrer">
  <title>M3U 网关</title>
  <style>
    :root { color-scheme: light; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    body { margin: 0; background: #f5f7fb; color: #182230; }
    main { max-width: 760px; margin: 9vh auto; padding: 0 20px 56px; }
    h1 { font-size: 2rem; margin: 0 0 12px; }
    .intro { color: #475467; line-height: 1.7; margin-bottom: 28px; }
    .card { background: #fff; border: 1px solid #e4e7ec; border-radius: 14px; padding: 22px; margin: 16px 0; box-shadow: 0 4px 16px #1018280a; }
    h2 { font-size: 1.1rem; margin: 0 0 8px; }
    p { line-height: 1.7; }
    .card p { margin: 0 0 12px; color: #475467; }
    .url { display: block; overflow-wrap: anywhere; background: #f2f4f7; border-radius: 8px; padding: 12px; color: #175cd3; text-decoration: none; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
    .url:hover { text-decoration: underline; }
    .note { color: #667085; font-size: .92rem; margin-top: 24px; }
    form { display: flex; gap: 10px; margin: 20px 0 28px; }
    input { flex: 1; min-width: 0; padding: 11px 12px; border: 1px solid #98a2b3; border-radius: 8px; font: inherit; }
    button { padding: 11px 18px; border: 0; border-radius: 8px; background: #175cd3; color: #fff; font: inherit; cursor: pointer; }
    .error { color: #b42318; }
    @media (max-width: 480px) { form { flex-direction: column; } }
  </style>
</head>
<body>
<main>
  <h1>M3U 转发网关</h1>
  <p class="intro">把下面的订阅地址填入播放器或客户端。频道列表只显示当前有播放源的预设频道；播放和台标都由本机网关提供。</p>
  {{if .AuthEnabled}}
  <form action="/" method="get">
    <input type="password" name="toke" aria-label="访问 token" placeholder="输入访问 token" autocomplete="off" required>
    <button type="submit">生成连接</button>
  </form>
  {{if .Invalid}}<p class="error">Token 不正确，请检查配置后重试。</p>{{end}}
  {{end}}
  {{if .LinksReady}}
  <section class="card">
    <h2>M3U 播放列表</h2>
    <p>适用于支持 M3U 订阅的 IPTV 播放器。每个频道只有一个网关播放地址，播放时自动选源。</p>
    <a class="url" href="{{.M3UURL}}">{{.M3UURL}}</a>
  </section>
  <section class="card">
    <h2>JSON 频道列表</h2>
    <p>适用于本项目客户端使用的频道列表格式。</p>
    <a class="url" href="{{.JSONURL}}">{{.JSONURL}}</a>
  </section>
  {{else}}
  <p class="note">请先输入配置文件中的 token，生成可用的订阅连接。</p>
  {{end}}
  <p class="note">首次运行会在网关工作目录生成 config.json。把上游 M3U 路径或 URL 写入 m3u_sources，修改配置后重启网关。远程列表按访问触发刷新。</p>
</main>
</body>
</html>`))

func (g *gateway) serveHome(c *gin.Context) {
	base := requestBase(c)
	token := c.Query(tokenQueryKey)
	authEnabled := len(g.tokens) != 0
	ready := !authEnabled || (token != "" && g.validToken(token))
	data := struct {
		AuthEnabled bool
		LinksReady  bool
		Invalid     bool
		M3UURL      string
		JSONURL     string
	}{AuthEnabled: authEnabled, LinksReady: ready, Invalid: authEnabled && token != "" && !ready}
	if ready {
		data.M3UURL = g.withToken(base+"/channels.m3u", token)
		data.JSONURL = g.withToken(base+"/channels.json", token)
	}
	var body bytes.Buffer
	if err := homeTemplate.Execute(&body, data); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Data(http.StatusOK, "text/html; charset=utf-8", body.Bytes())
}
