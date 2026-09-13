package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/res-sniffer/res-sniffer/internal/config"
	"github.com/res-sniffer/res-sniffer/internal/downloader"
	"github.com/res-sniffer/res-sniffer/internal/model"
)

// Store 全局数据存储（内存中）
type Store struct {
	mu          sync.RWMutex
	resources   map[string]*model.MediaInfo   // id -> MediaInfo
	downloads   map[string]*model.DownloadTask // id -> DownloadTask
	activeDLers map[string]*downloader.Downloader // id -> downloader
}

// NewStore 创建数据存储
func NewStore() *Store {
	return &Store{
		resources:   make(map[string]*model.MediaInfo),
		downloads:    make(map[string]*model.DownloadTask),
		activeDLers:  make(map[string]*downloader.Downloader),
	}
}

// AddResource 添加嗅探到的资源
func (s *Store) AddResource(m *model.MediaInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources[m.ID] = m
}

// ListResources 列出资源（可按类型过滤）
func (s *Store) ListResources(filterType string, limit int) []*model.MediaInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*model.MediaInfo
	for _, m := range s.resources {
		if filterType != "" {
			types := strings.Split(filterType, ",")
			match := false
			for _, t := range types {
				if m.Classify == strings.TrimSpace(t) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		list = append(list, m)
	}

	// 按时间倒序
	sort.Slice(list, func(i, j int) bool {
		return list[i].DetectedAt.After(list[j].DetectedAt)
	})

	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list
}

// GetResource 获取单个资源
func (s *Store) GetResource(id string) (*model.MediaInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.resources[id]
	return m, ok
}

// DeleteResource 删除资源记录
func (s *Store) DeleteResource(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.resources[id]; ok {
		delete(s.resources, id)
		return true
	}
	return false
}

// AddDownloadTask 添加下载任务
func (s *Store) AddDownloadTask(t *model.DownloadTask) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.downloads[t.ID] = t
}

// UpdateDownloadTask 更新下载任务状态
func (s *Store) UpdateDownloadTask(id string, fn func(*model.DownloadTask)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.downloads[id]; ok {
		fn(t)
	}
}

// ListDownloadTasks 列出下载任务
func (s *Store) ListDownloadTasks() []*model.DownloadTask {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []*model.DownloadTask
	for _, t := range s.downloads {
		list = append(list, t)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})
	return list
}

// GetDownloadTask 获取下载任务
func (s *Store) GetDownloadTask(id string) (*model.DownloadTask, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.downloads[id]
	return t, ok
}

// StoreDownloader 存储活跃下载器
func (s *Store) StoreDownloader(id string, d *downloader.Downloader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeDLers[id] = d
}

// RemoveDownloader 移除活跃下载器
func (s *Store) RemoveDownloader(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.activeDLers, id)
}

// GetDownloader 获取活跃下载器
func (s *Store) GetDownloader(id string) (*downloader.Downloader, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.activeDLers[id]
	return d, ok
}

// --- HTTP 响应辅助函数 ---

func writeJSON(w http.ResponseWriter, code int, message string, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(model.APIResponse{
		Code:    code,
		Message: message,
		Data:    data,
	})
}

func writeSuccess(w http.ResponseWriter, data interface{}) {
	writeJSON(w, 0, "ok", data)
}

func writeError(w http.ResponseWriter, msg string) {
	writeJSON(w, -1, msg, nil)
}

// --- API 处理器 ---

// HealthHandler 健康检查
func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	writeSuccess(w, map[string]interface{}{
		"version":     "v0.1.0",
		"proxy_running": s.proxy.IsRunning(),
		"uptime":      time.Since(s.startTime).String(),
	})
}

// GetConfigHandler 获取配置
func (s *Server) GetConfigHandler(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	writeSuccess(w, cfg)
}

// PutConfigHandler 更新配置
func (s *Server) PutConfigHandler(w http.ResponseWriter, r *http.Request) {
	var newCfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		writeError(w, "解析请求体失败: "+err.Error())
		return
	}
	if err := config.Update(&newCfg); err != nil {
		writeError(w, "更新配置失败: "+err.Error())
		return
	}
	// 重新加载规则
	s.ruleSet.Load(newCfg.Rule)
	writeSuccess(w, nil)
}

// ListResourcesHandler 列出资源
func (s *Server) ListResourcesHandler(w http.ResponseWriter, r *http.Request) {
	typeFilter := r.URL.Query().Get("type")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}
	}
	resources := s.store.ListResources(typeFilter, limit)
	writeSuccess(w, resources)
}

// GetResourceHandler 获取单个资源
func (s *Server) GetResourceHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m, ok := s.store.GetResource(id)
	if !ok {
		writeError(w, "资源不存在")
		return
	}
	writeSuccess(w, m)
}

// DeleteResourceHandler 删除资源
func (s *Server) DeleteResourceHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.store.DeleteResource(id) {
		writeError(w, "资源不存在")
		return
	}
	writeSuccess(w, nil)
}

// DownloadResourceHandler 下载已嗅探的资源
func (s *Server) DownloadResourceHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m, ok := s.store.GetResource(id)
	if !ok {
		writeError(w, "资源不存在")
		return
	}
	taskID, err := s.startDownload(m.URL, m.Headers, "")
	if err != nil {
		writeError(w, "启动下载失败: "+err.Error())
		return
	}
	writeSuccess(w, map[string]string{"task_id": taskID})
}

// DirectDownloadHandler 直接下载 URL
func (s *Server) DirectDownloadHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL      string            `json:"url"`
		SavePath string            `json:"save_path"`
		Headers  map[string]string `json:"headers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "解析请求体失败: "+err.Error())
		return
	}
	if req.URL == "" {
		writeError(w, "url 不能为空")
		return
	}
	taskID, err := s.startDownload(req.URL, req.Headers, req.SavePath)
	if err != nil {
		writeError(w, "启动下载失败: "+err.Error())
		return
	}
	writeSuccess(w, map[string]string{"task_id": taskID})
}

// ListDownloadsHandler 列出下载任务
func (s *Server) ListDownloadsHandler(w http.ResponseWriter, r *http.Request) {
	tasks := s.store.ListDownloadTasks()
	writeSuccess(w, tasks)
}

// GetDownloadHandler 获取下载任务状态
func (s *Server) GetDownloadHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, ok := s.store.GetDownloadTask(id)
	if !ok {
		writeError(w, "任务不存在")
		return
	}
	writeSuccess(w, t)
}

// CancelDownloadHandler 取消下载任务
func (s *Server) CancelDownloadHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if d, ok := s.store.GetDownloader(id); ok {
		d.Cancel()
		s.store.RemoveDownloader(id)
		s.store.UpdateDownloadTask(id, func(t *model.DownloadTask) {
			t.Status = model.StatusCancelled
		})
		writeSuccess(w, nil)
		return
	}
	writeError(w, "任务不存在或已完成")
}

// ProxyStatusHandler 代理状态
func (s *Server) ProxyStatusHandler(w http.ResponseWriter, r *http.Request) {
	writeSuccess(w, map[string]interface{}{
		"running": s.proxy.IsRunning(),
	})
}

// ProxyStartHandler 启动代理
func (s *Server) ProxyStartHandler(w http.ResponseWriter, r *http.Request) {
	if s.proxy.IsRunning() {
		writeSuccess(w, map[string]string{"status": "already running"})
		return
	}
	cfg := config.Get()
	addr := cfg.Host + ":" + cfg.Port
	if err := s.proxy.Start(addr); err != nil {
		writeError(w, "启动代理失败: "+err.Error())
		return
	}
	writeSuccess(w, map[string]string{"status": "started", "addr": addr})
}

// ProxyStopHandler 停止代理
func (s *Server) ProxyStopHandler(w http.ResponseWriter, r *http.Request) {
	s.proxy.Stop()
	writeSuccess(w, map[string]string{"status": "stopped"})
}

// CertHandler 下载 CA 证书
func (s *Server) CertHandler(w http.ResponseWriter, r *http.Request) {
	data, err := s.certMgr.ExportCert()
	if err != nil {
		writeError(w, "读取证书失败: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	w.Header().Set("Content-Disposition", "attachment; filename=res-sniffer-ca.crt")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// --- 下载管理 ---

// startDownload 启动下载任务
func (s *Server) startDownload(downloadURL string, headers map[string]string, savePath string) (string, error) {
	cfg := config.Get()

	// 生成任务 ID
	taskID := generateID()

	// 确定保存路径
	if savePath == "" {
		fileName := getFileNameFromURL(downloadURL)
		if fileName == "" {
			fileName = taskID
		}
		// 加时间戳避免重名
		savePath = fmt.Sprintf("%s/%s_%s", cfg.SaveDirectory, fileName, time.Now().Format("20060102150405"))
	}

	task := &model.DownloadTask{
		ID:        taskID,
		URL:       downloadURL,
		SavePath:  savePath,
		Status:    model.StatusPending,
		CreatedAt: time.Now(),
	}
	s.store.AddDownloadTask(task)

	// 创建下载器
	dl := downloader.New(downloadURL, savePath, cfg.TaskNumber, headers)
	dl.SetProgressCallback(func(downloaded, total int64, progress float64) {
		s.store.UpdateDownloadTask(taskID, func(t *model.DownloadTask) {
			t.Downloaded = downloaded
			t.TotalSize = total
			t.Progress = progress
			t.Status = model.StatusRunning
		})
		s.sse.EmitDownloadProgress(taskID, downloaded, total, progress)
	})

	s.store.StoreDownloader(taskID, dl)

	// 异步下载
	go func() {
		log.Info().Str("task_id", taskID).Str("url", downloadURL).Msg("开始下载")
		err := dl.Start()
		s.store.RemoveDownloader(taskID)

		s.store.UpdateDownloadTask(taskID, func(t *model.DownloadTask) {
			t.CompletedAt = time.Now()
			if err != nil {
				t.Status = model.StatusError
				t.Error = err.Error()
				s.sse.EmitDownloadError(taskID, err.Error())
				log.Error().Err(err).Str("task_id", taskID).Msg("下载失败")
			} else {
				t.Status = model.StatusDone
				t.Progress = 100
				s.sse.EmitDownloadDone(taskID, savePath)
				log.Info().Str("task_id", taskID).Str("path", savePath).Msg("下载完成")
			}
		})
	}()

	return taskID, nil
}
