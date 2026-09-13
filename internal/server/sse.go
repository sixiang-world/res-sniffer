package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/res-sniffer/res-sniffer/internal/model"
)

// SSEHub Server-Sent Events 事件流管理
type SSEHub struct {
	mu      sync.RWMutex
	clients map[chan string]bool
}

// NewSSEHub 创建 SSE 事件中心
func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[chan string]bool),
	}
}

// HandleSSE 处理 SSE 连接
func (h *SSEHub) HandleSSE(w http.ResponseWriter, r *http.Request) {
	// 设置 SSE 头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// 创建客户端 channel
	ch := make(chan string, 16)
	h.mu.Lock()
	h.clients[ch] = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, ch)
		close(ch)
		h.mu.Unlock()
	}()

	// 发送初始连接事件
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ok\"}\n\n")
	flusher.Flush()

	// 监听消息和客户端断开
	for {
		select {
		case msg := <-ch:
			fmt.Fprint(w, msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// broadcast 向所有客户端广播事件
func (h *SSEHub) broadcast(eventType string, data interface{}) {
	msg := formatSSEEvent(eventType, data)

	h.mu.RLock()
	defer h.mu.RUnlock()

	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
			// channel 满了就跳过，避免阻塞
		}
	}
}

// formatSSEEvent 格式化为 SSE 事件字符串
func formatSSEEvent(eventType string, data interface{}) string {
	// data 应该已经是 JSON 字符串
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(jsonBytes))
}

// --- 事件发送方法（实现 sniffer.EventBus 接口） ---

// EmitResourceNew 推送新资源事件
func (h *SSEHub) EmitResourceNew(media *model.MediaInfo) {
	h.broadcast("resource.new", media)
}

// EmitDownloadProgress 推送下载进度事件
func (h *SSEHub) EmitDownloadProgress(taskID string, downloaded int64, total int64, progress float64) {
	h.broadcast("download.progress", map[string]interface{}{
		"task_id":    taskID,
		"downloaded": downloaded,
		"total":      total,
		"progress":   progress,
	})
}

// EmitDownloadDone 推送下载完成事件
func (h *SSEHub) EmitDownloadDone(taskID string, savePath string) {
	h.broadcast("download.done", map[string]interface{}{
		"task_id":   taskID,
		"save_path": savePath,
	})
}

// EmitDownloadError 推送下载错误事件
func (h *SSEHub) EmitDownloadError(taskID string, errMsg string) {
	h.broadcast("download.error", map[string]interface{}{
		"task_id": taskID,
		"error":   errMsg,
	})
}
