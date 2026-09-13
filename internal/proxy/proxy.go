package proxy

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/elazarl/goproxy"
	"github.com/rs/zerolog/log"
)

// Proxy MITM 代理核心
type Proxy struct {
	server   *goproxy.ProxyHttpServer
	certMgr  *CertManager
	ruleSet  *RuleSet
	running  bool
	ln       net.Listener
}

// NewProxy 创建代理实例
func NewProxy(certMgr *CertManager, ruleSet *RuleSet) *Proxy {
	return &Proxy{
		certMgr: certMgr,
		ruleSet: ruleSet,
	}
}

// OnRequestFunc 请求事件回调
type OnRequestFunc func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response)

// OnResponseFunc 响应事件回调
type OnResponseFunc func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response

// Setup 配置代理服务器
func (p *Proxy) Setup(onReq OnRequestFunc, onResp OnResponseFunc) error {
	// 配置 goproxy 使用我们的 CA 证书
	caCert := p.certMgr.GetCert()
	goproxy.GoproxyCa = *caCert
	goproxy.OkConnect = &goproxy.ConnectAction{
		Action:    goproxy.ConnectAccept,
		TLSConfig: goproxy.TLSConfigFromCA(caCert),
	}
	goproxy.MitmConnect = &goproxy.ConnectAction{
		Action:    goproxy.ConnectMitm,
		TLSConfig: goproxy.TLSConfigFromCA(caCert),
	}
	goproxy.HTTPMitmConnect = &goproxy.ConnectAction{
		Action:    goproxy.ConnectHTTPMitm,
		TLSConfig: goproxy.TLSConfigFromCA(caCert),
	}
	goproxy.RejectConnect = &goproxy.ConnectAction{
		Action:    goproxy.ConnectReject,
		TLSConfig: goproxy.TLSConfigFromCA(caCert),
	}

	p.server = goproxy.NewProxyHttpServer()
	p.server.Verbose = false

	// 配置 Transport
	transport := &http.Transport{
		DisableKeepAlives: false,
		DialContext: (&net.Dialer{
			Timeout: 60 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   60 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}
	p.server.Tr = transport

	// 根据规则决定是否 MITM
	p.server.OnRequest().HandleConnectFunc(func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		if p.ruleSet.ShouldMitm(host) {
			return goproxy.MitmConnect, host
		}
		return goproxy.OkConnect, host
	})

	// 注册请求/响应回调
	if onReq != nil {
		p.server.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
			return onReq(req, ctx)
		})
	}
	if onResp != nil {
		p.server.OnResponse().DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
			return onResp(resp, ctx)
		})
	}

	return nil
}

// Start 在指定地址启动代理
func (p *Proxy) Start(addr string) error {
	if p.server == nil {
		if err := p.Setup(nil, nil); err != nil {
			return err
		}
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	p.ln = ln
	p.running = true

	log.Info().Str("addr", addr).Msg("代理服务器启动")

	go func() {
		if err := http.Serve(ln, p.server); err != nil {
			log.Error().Err(err).Msg("代理服务器停止")
			p.running = false
		}
	}()

	return nil
}

// Stop 停止代理
func (p *Proxy) Stop() {
	if p.ln != nil {
		_ = p.ln.Close()
	}
	p.running = false
	log.Info().Msg("代理服务器已停止")
}

// IsRunning 返回代理是否运行中
func (p *Proxy) IsRunning() bool {
	return p.running
}

// ServeHTTP 实现 http.Handler 接口，供统一监听使用
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if p.server != nil {
		p.server.ServeHTTP(w, r)
	}
}

// GetProxyURL 返回代理地址
func (p *Proxy) GetProxyURL() *url.URL {
	if p.ln != nil {
		return &url.URL{
			Scheme: "http",
			Host:   p.ln.Addr().String(),
		}
	}
	return nil
}
