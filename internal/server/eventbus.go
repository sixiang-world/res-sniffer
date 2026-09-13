package server

import (
	"github.com/res-sniffer/res-sniffer/internal/model"
	"github.com/res-sniffer/res-sniffer/internal/sniffer"
)

// CombinedEventBus 合并事件总线：同时推送 SSE 和写入存储
type CombinedEventBus struct {
	sse   *SSEHub
	store *Store
}

// NewCombinedEventBus 创建合并事件总线
func NewCombinedEventBus(sse *SSEHub, store *Store) sniffer.EventBus {
	return &CombinedEventBus{
		sse:   sse,
		store: store,
	}
}

// EmitResourceNew 推送新资源：写入存储 + SSE 广播
func (b *CombinedEventBus) EmitResourceNew(media *model.MediaInfo) {
	b.store.AddResource(media)
	b.sse.EmitResourceNew(media)
}

// EmitDownloadProgress 推送下载进度
func (b *CombinedEventBus) EmitDownloadProgress(taskID string, downloaded int64, total int64, progress float64) {
	b.sse.EmitDownloadProgress(taskID, downloaded, total, progress)
}

// EmitDownloadDone 推送下载完成
func (b *CombinedEventBus) EmitDownloadDone(taskID string, savePath string) {
	b.sse.EmitDownloadDone(taskID, savePath)
}

// EmitDownloadError 推送下载错误
func (b *CombinedEventBus) EmitDownloadError(taskID string, errMsg string) {
	b.sse.EmitDownloadError(taskID, errMsg)
}
