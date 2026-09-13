package model

import (
	"time"
)

// MediaInfo 表示一个被嗅探到的网络资源
type MediaInfo struct {
	ID          string            `json:"id"`
	URL         string            `json:"url"`
	URLSign     string            `json:"url_sign"` // md5(url)，用于去重
	CoverURL    string            `json:"cover_url"`
	Size        int64             `json:"size"`
	Domain      string            `json:"domain"`
	Classify    string            `json:"classify"` // video/audio/image/m3u8/live/pdf/doc/stream/font
	Suffix      string            `json:"suffix"`   // .mp4 .mp3 etc
	Status      string            `json:"status"`   // ready/running/done/error
	SavePath    string            `json:"save_path"`
	ContentType string            `json:"content_type"`
	Description string            `json:"description"`
	DetectedAt  time.Time         `json:"detected_at"`
	Headers     map[string]string `json:"-"` // 原始请求头，用于下载
}

// DownloadTask 表示一个下载任务
type DownloadTask struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	SavePath    string    `json:"save_path"`
	TotalSize   int64     `json:"total_size"`
	Downloaded  int64     `json:"downloaded"`
	Status      string    `json:"status"` // pending/running/done/error/cancelled
	Progress    float64   `json:"progress"` // 0-100
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// MimeInfo 描述一个 Content-Type 对应的资源分类和文件后缀
type MimeInfo struct {
	Type   string `json:"Type"`
	Suffix string `json:"Suffix"`
}

// APIResponse 统一的 API 响应格式
type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// 下载状态常量
const (
	StatusReady     = "ready"
	StatusRunning   = "running"
	StatusDone      = "done"
	StatusError     = "error"
	StatusCancelled = "cancelled"
	StatusPending   = "pending"
)
