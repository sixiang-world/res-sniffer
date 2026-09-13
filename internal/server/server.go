package server

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/elazarl/goproxy"
	"github.com/rs/zerolog/log"
	"github.com/res-sniffer/res-sniffer/internal/config"
	"github.com/res-sniffer/res-sniffer/internal/model"
	"github.com/res-sniffer/res-sniffer/internal/proxy"
	"github.com/res-sniffer/res-sniffer/internal/sniffer"
)

// Server HTTP API + 代理 统一服务器
type Server struct {
	store     *Store
	sse       *SSEHub
	proxy     *proxy.Proxy
	certMgr   *proxy.CertManager
	ruleSet   *proxy.RuleSet
	sniffReg  *sniffer.Registry
	startTime time.Time
	httpSrv   *http.Server
}

// New 创建服务器
func New(certMgr *proxy.CertManager, ruleSet *proxy.RuleSet, eventBus *SSEHub) *Server {
	s := &Server{
		store:     NewStore(),
		sse:       eventBus,
		certMgr:   certMgr,
		ruleSet:   ruleSet,
		startTime: time.Now(),
	}

	// 初始化嗅探插件注册表（使用合并事件总线：SSE + 存储）
	cfg := config.Get()
	combinedBus := NewCombinedEventBus(eventBus, s.store)
	s.sniffReg = sniffer.NewRegistry(combinedBus, cfg.TypeSuffix)

	// 注册默认插件和微信插件
	s.sniffReg.Register(sniffer.NewDefaultPlugin(s.sniffReg.GetConfig(), combinedBus))
	s.sniffReg.Register(sniffer.NewWeChatPlugin(s.sniffReg.GetConfig(), combinedBus))

	// 创建代理
	s.proxy = proxy.NewProxy(certMgr, ruleSet)

	// 配置代理的嗅探回调
	s.proxy.Setup(s.proxyOnRequest, s.proxyOnResponse)

	return s
}

// proxyOnRequest 代理请求事件回调
func (s *Server) proxyOnRequest(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	plugin := s.sniffReg.Match(req.Host)
	if plugin != nil {
		return plugin.OnRequest(req, ctx)
	}
	return req, nil
}

// proxyOnResponse 代理响应事件回调
func (s *Server) proxyOnResponse(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	if resp == nil || resp.Request == nil {
		return resp
	}

	plugin := s.sniffReg.Match(resp.Request.Host)
	if plugin != nil {
		result := plugin.OnResponse(resp, ctx)
		if result != nil {
			return result
		}
	}

	// 没有匹配的插件返回时，交给默认插件
	if defaultP := s.sniffReg.GetDefault(); defaultP != nil {
		return defaultP.OnResponse(resp, ctx)
	}

	return resp
}

// Start 启动 HTTP 服务器（同时服务 API 和代理）
func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.httpSrv = &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// API 和 SSE 请求走 mux，其他走代理
			if strings.HasPrefix(r.URL.Path, "/api/") ||
				r.URL.Path == "/events" ||
				r.URL.Path == "/cert" {
				mux.ServeHTTP(w, r)
			} else {
				// 代理流量
				s.proxy.ServeHTTP(w, r)
			}
		}),
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("监听 %s 失败: %w", addr, err)
	}

	log.Info().Str("addr", addr).Msg("服务器启动（API + 代理同端口）")

	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("HTTP 服务器错误")
		}
	}()

	return nil
}

// StartProxyOnly 仅启动代理（独立监听，不启动 API）
func (s *Server) StartProxyOnly(addr string) error {
	return s.proxy.Start(addr)
}

// StartAPIOnly 仅启动 API 服务器
func (s *Server) StartAPIOnly(addr string) error {
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	s.httpSrv = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("监听 %s 失败: %w", addr, err)
	}

	log.Info().Str("addr", addr).Msg("API 服务器启动")

	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("HTTP 服务器错误")
		}
	}()

	return nil
}

// Stop 停止服务器
func (s *Server) Stop() {
	if s.httpSrv != nil {
		_ = s.httpSrv.Close()
	}
	s.proxy.Stop()
	log.Info().Msg("服务器已停止")
}

// registerRoutes 注册 API 路由
func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/health", s.HealthHandler)
	mux.HandleFunc("GET /api/v1/config", s.GetConfigHandler)
	mux.HandleFunc("PUT /api/v1/config", s.PutConfigHandler)
	mux.HandleFunc("GET /api/v1/resources", s.ListResourcesHandler)
	mux.HandleFunc("GET /api/v1/resources/{id}", s.GetResourceHandler)
	mux.HandleFunc("DELETE /api/v1/resources/{id}", s.DeleteResourceHandler)
	mux.HandleFunc("POST /api/v1/resources/{id}/download", s.DownloadResourceHandler)
	mux.HandleFunc("POST /api/v1/download", s.DirectDownloadHandler)
	mux.HandleFunc("GET /api/v1/downloads", s.ListDownloadsHandler)
	mux.HandleFunc("GET /api/v1/downloads/{id}", s.GetDownloadHandler)
	mux.HandleFunc("POST /api/v1/downloads/{id}/cancel", s.CancelDownloadHandler)
	mux.HandleFunc("GET /api/v1/proxy/status", s.ProxyStatusHandler)
	mux.HandleFunc("POST /api/v1/proxy/start", s.ProxyStartHandler)
	mux.HandleFunc("POST /api/v1/proxy/stop", s.ProxyStopHandler)
	mux.HandleFunc("GET /api/v1/cert", s.CertHandler)
	mux.HandleFunc("GET /api/v1/events", s.sse.HandleSSE)
}

// GetProxy 返回代理实例
func (s *Server) GetProxy() *proxy.Proxy {
	return s.proxy
}

// --- 辅助函数 ---

// generateID 生成短 ID
func generateID() string {
	return md5Hex(time.Now().String())[:12]
}

func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

// getFileNameFromURL 从 URL 提取文件名
func getFileNameFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	name := path.Base(parsed.Path)
	if name == "" || name == "/" {
		return ""
	}
	if decoded, err := url.QueryUnescape(name); err == nil {
		name = decoded
	}
	return name
}

// NotifyResourceNew 通知新资源（由默认插件通过 EventBus 触发）
func (s *Server) NotifyResourceNew(m *model.MediaInfo) {
	s.store.AddResource(m)
	s.sse.EmitResourceNew(m)
}
