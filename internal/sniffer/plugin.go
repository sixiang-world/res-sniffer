package sniffer

import (
	"net/http"
	"sync"

	"github.com/elazarl/goproxy"
	"github.com/res-sniffer/res-sniffer/internal/model"
)

// EventBus 事件总线，用于向服务器推送嗅探到的资源和下载进度
type EventBus interface {
	// EmitResourceNew 推送新嗅探到的资源
	EmitResourceNew(media *model.MediaInfo)
	// EmitDownloadProgress 推送下载进度
	EmitDownloadProgress(taskID string, downloaded int64, total int64, progress float64)
	// EmitDownloadDone 推送下载完成
	EmitDownloadDone(taskID string, savePath string)
	// EmitDownloadError 推送下载错误
	EmitDownloadError(taskID string, errMsg string)
}

// Plugin 资源嗅探插件接口
type Plugin interface {
	// Domains 返回该插件匹配的顶级域名列表
	Domains() []string
	// OnRequest 请求拦截处理
	OnRequest(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response)
	// OnResponse 响应拦截处理
	OnResponse(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response
}

// PluginContext 传递给插件的上下文
type PluginContext struct {
	EventBus   EventBus
	Config     SnifferConfig
	markedSet  sync.Map // urlSign -> bool，去重标记
}

// SnifferConfig 嗅探器配置接口
type SnifferConfig interface {
	// TypeSuffix 根据 Content-Type 获取分类和后缀
	TypeSuffix(mime string) (classify, suffix string)
	// IsMarked 检查 urlSign 是否已标记
	IsMarked(sign string) bool
	// Mark 标记 urlSign
	Mark(sign string)
}

// defaultSnifferConfig 基于全局配置的嗅探配置
type defaultSnifferConfig struct {
	mimeFunc func(mime string) (string, string)
	marked   *sync.Map
}

func (d *defaultSnifferConfig) TypeSuffix(mime string) (string, string) {
	return d.mimeFunc(mime)
}

func (d *defaultSnifferConfig) IsMarked(sign string) bool {
	_, ok := d.marked.Load(sign)
	return ok
}

func (d *defaultSnifferConfig) Mark(sign string) {
	d.marked.Store(sign, true)
}

// Registry 插件注册表
type Registry struct {
	plugins   map[string]Plugin // domain -> plugin
	defaultP  Plugin
	config    SnifferConfig
	eventBus  EventBus
	marked    sync.Map
}

// NewRegistry 创建插件注册表
func NewRegistry(eventBus EventBus, mimeFunc func(string) (string, string)) *Registry {
	r := &Registry{
		plugins: make(map[string]Plugin),
		eventBus: eventBus,
	}
	r.config = &defaultSnifferConfig{
		mimeFunc: mimeFunc,
		marked:   &r.marked,
	}
	return r
}

// Register 注册插件
func (r *Registry) Register(p Plugin) {
	for _, domain := range p.Domains() {
		if domain == "default" {
			r.defaultP = p
		} else {
			r.plugins[domain] = p
		}
	}
}

// GetConfig 返回嗅探配置
func (r *Registry) GetConfig() SnifferConfig {
	return r.config
}

// GetEventBus 返回事件总线
func (r *Registry) GetEventBus() EventBus {
	return r.eventBus
}

// Match 按 host 匹配插件
func (r *Registry) Match(host string) Plugin {
	// 提取顶级域名
	domain := getTopLevelDomain(host)
	if p, ok := r.plugins[domain]; ok {
		return p
	}
	return r.defaultP
}

// GetDefault 获取默认插件
func (r *Registry) GetDefault() Plugin {
	return r.defaultP
}
