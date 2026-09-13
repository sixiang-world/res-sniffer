package sniffer

import (
	"net/http"
	"strings"

	"github.com/elazarl/goproxy"
)

// WeChatPlugin 微信视频号插件（简化版 v0.1）
// v0.1 不实现 JS 注入和 objectDesc 拦截，仅保留接口
// 后续版本可在此插件中实现微信视频号的特殊处理逻辑
type WeChatPlugin struct {
	config   SnifferConfig
	eventBus EventBus
}

// NewWeChatPlugin 创建微信视频号插件
func NewWeChatPlugin(config SnifferConfig, eventBus EventBus) *WeChatPlugin {
	return &WeChatPlugin{
		config:   config,
		eventBus: eventBus,
	}
}

func (p *WeChatPlugin) Domains() []string {
	return []string{"qq.com"}
}

func (p *WeChatPlugin) OnRequest(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	// v0.1 不实现特殊请求处理，直接透传
	return nil, nil
}

func (p *WeChatPlugin) OnResponse(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	// v0.1 简化版：对微信视频号域名的响应不做特殊处理
	// 后续版本可在此实现 JS 注入和 objectDesc 拦截
	if resp == nil || resp.Request == nil {
		return nil
	}

	host := resp.Request.Host
	// 如果是微信视频号的视频请求，交给默认插件处理
	if strings.HasSuffix(host, "finder.video.qq.com") ||
		strings.HasSuffix(host, "channels.weixin.qq.com") {
		return nil // 返回 nil 让默认插件处理
	}

	return nil
}
