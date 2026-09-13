package sniffer

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/elazarl/goproxy"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/res-sniffer/res-sniffer/internal/model"
)

// DefaultPlugin 默认嗅探插件，通过 Content-Type 识别资源
type DefaultPlugin struct {
	config   SnifferConfig
	eventBus EventBus
}

// NewDefaultPlugin 创建默认插件
func NewDefaultPlugin(config SnifferConfig, eventBus EventBus) *DefaultPlugin {
	return &DefaultPlugin{
		config:   config,
		eventBus: eventBus,
	}
}

func (p *DefaultPlugin) Domains() []string {
	return []string{"default"}
}

func (p *DefaultPlugin) OnRequest(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	return r, nil
}

func (p *DefaultPlugin) OnResponse(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
	if resp == nil || resp.Request == nil {
		return resp
	}
	if resp.StatusCode != 200 && resp.StatusCode != 206 && resp.StatusCode != 304 {
		return resp
	}

	contentType := resp.Header.Get("Content-Type")
	classify, suffix := p.config.TypeSuffix(contentType)
	if classify == "" {
		return resp
	}

	rawURL := resp.Request.URL.String()

	// octet-stream 时从 URL 提取扩展名
	if suffix == "default" {
		ext := filepath.Ext(filepath.Base(strings.Split(strings.Split(rawURL, "?")[0], "#")[0]))
		if ext != "" {
			suffix = ext
		} else {
			suffix = ".bin"
		}
	}

	// 去重
	urlSign := md5Hex(rawURL)
	if p.config.IsMarked(urlSign) {
		return resp
	}

	// 解析大小
	var size int64
	if cl := resp.Header.Get("content-length"); cl != "" {
		if v, err := strconv.ParseInt(cl, 10, 64); err == nil {
			size = v
		}
	}

	// 生成 ID
	id, err := gonanoid.New()
	if err != nil {
		id = urlSign
	}

	// 收集请求头
	headers := make(map[string]string)
	for k, vs := range resp.Request.Header {
		if len(vs) > 0 {
			headers[k] = vs[0]
		}
	}

	media := &model.MediaInfo{
		ID:          id,
		URL:         rawURL,
		URLSign:     urlSign,
		Size:        size,
		Domain:      getTopLevelDomain(rawURL),
		Classify:    classify,
		Suffix:      suffix,
		Status:      model.StatusReady,
		ContentType: contentType,
		DetectedAt:  time.Now(),
		Headers:     headers,
	}

	p.config.Mark(urlSign)
	p.eventBus.EmitResourceNew(media)

	return resp
}

// md5Hex 计算字符串的 md5 hex
func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

// getTopLevelDomain 从 host 或 URL 提取顶级域名
func getTopLevelDomain(raw string) string {
	// 如果是 URL 先解析
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		raw = u.Host
	}
	// 去掉端口
	if idx := strings.LastIndex(raw, ":"); idx != -1 {
		// 检查是否是 IPv6
		if !strings.Contains(raw, "]") {
			raw = raw[:idx]
		}
	}
	// 简化处理：取最后两段
	parts := strings.Split(strings.TrimSuffix(raw, "."), ".")
	if len(parts) >= 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return raw
}

// GetFileNameFromURL 从 URL 提取文件名
func GetFileNameFromURL(rawURL string) string {
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

// MarshalHeaders 将请求头序列化为 JSON 字符串
func MarshalHeaders(h http.Header) string {
	data, _ := json.Marshal(h)
	return string(data)
}
