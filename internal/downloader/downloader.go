package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	maxRetries  = 3               // 最大重试次数
	retryDelay  = 3 * time.Second // 重试延迟
	minPartSize = 1 * 1024 * 1024 // 最小分片大小（1MB）
)

// ProgressCallback 下载进度回调
type ProgressCallback func(downloaded, total int64, progress float64)

// Task 单个分片下载任务
type Task struct {
	taskID         int
	rangeStart     int64
	rangeEnd       int64
	downloadedSize int64
	isCompleted    bool
	err            error
}

// Downloader 多线程分片下载器
type Downloader struct {
	URL         string
	SavePath    string
	Threads     int
	Headers     map[string]string
	TotalSize   int64
	IsMultiPart bool
	file        *os.File
	tasks       []*Task
	ctx         context.Context
	cancelFunc  context.CancelFunc
	progressCb  ProgressCallback
}

// New 创建下载器
func New(downloadURL, savePath string, threads int, headers map[string]string) *Downloader {
	ctx, cancel := context.WithCancel(context.Background())
	if threads <= 0 {
		threads = 4
	}
	if headers == nil {
		headers = make(map[string]string)
	}
	return &Downloader{
		URL:        downloadURL,
		SavePath:   savePath,
		Threads:    threads,
		Headers:    headers,
		ctx:        ctx,
		cancelFunc: cancel,
	}
}

// SetProgressCallback 设置进度回调
func (d *Downloader) SetProgressCallback(cb ProgressCallback) {
	d.progressCb = cb
}

// buildClient 构建 HTTP 客户端
func (d *Downloader) buildClient() *http.Client {
	transport := &http.Transport{
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	}
	return &http.Client{Transport: transport}
}

var forbiddenHeaders = map[string]struct{}{
	"accept-encoding":   {},
	"content-length":    {},
	"host":              {},
	"connection":        {},
	"keep-alive":        {},
	"proxy-connection":  {},
	"transfer-encoding": {},
	"sec-fetch-site":     {},
	"sec-fetch-mode":     {},
	"sec-fetch-dest":     {},
	"sec-fetch-user":     {},
	"sec-ch-ua":          {},
	"sec-ch-ua-mobile":   {},
	"sec-ch-ua-platform": {},
	"if-none-match":      {},
	"if-modified-since":  {},
	"x-forwarded-for":    {},
	"x-real-ip":          {},
}

// setHeaders 设置请求头
func (d *Downloader) setHeaders(req *http.Request) {
	for key, value := range d.Headers {
		lk := strings.ToLower(key)
		if _, forbidden := forbiddenHeaders[lk]; forbidden {
			continue
		}
		req.Header.Set(key, value)
	}
}

// init 初始化：HEAD 请求探测大小，准备文件
func (d *Downloader) init() error {
	// 构造 Referer
	parsedURL, err := url.Parse(d.URL)
	if err != nil {
		return fmt.Errorf("解析 URL 失败: %w", err)
	}
	referer := parsedURL.Scheme + "://" + parsedURL.Host + "/"

	req, err := http.NewRequest("HEAD", d.URL, nil)
	if err != nil {
		return fmt.Errorf("创建 HEAD 请求失败: %w", err)
	}

	// 设置默认 User-Agent
	if _, ok := d.Headers["User-Agent"]; !ok {
		d.Headers["User-Agent"] = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
	}
	if _, ok := d.Headers["Referer"]; !ok {
		d.Headers["Referer"] = referer
	}

	d.setHeaders(req)

	// HEAD 请求带重试
	var resp *http.Response
	for retries := 0; retries < maxRetries; retries++ {
		resp, err = d.buildClient().Do(req)
		if err == nil {
			break
		}
		if retries < maxRetries-1 {
			time.Sleep(retryDelay)
			log.Warn().Err(err).Int("retry", retries+1).Msg("HEAD 请求失败，重试中")
		}
	}
	if err != nil {
		return fmt.Errorf("HEAD 请求失败（已重试 %d 次）: %w", maxRetries, err)
	}
	defer resp.Body.Close()

	d.TotalSize = resp.ContentLength
	if d.TotalSize <= 0 {
		d.IsMultiPart = false
		d.TotalSize = -1
	} else if resp.Header.Get("Accept-Ranges") == "bytes" && d.TotalSize > minPartSize {
		d.IsMultiPart = true
	} else {
		d.IsMultiPart = false
	}

	// 创建目录
	dir := filepath.Dir(d.SavePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 打开/创建文件
	d.file, err = os.OpenFile(d.SavePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	if d.TotalSize > 0 {
		if err := d.file.Truncate(d.TotalSize); err != nil {
			d.file.Close()
			return fmt.Errorf("文件预分配失败: %w", err)
		}
	}
	return nil
}

// createTasks 创建分片下载任务
func (d *Downloader) createTasks() {
	d.tasks = nil
	if d.IsMultiPart {
		eachSize := d.TotalSize / int64(d.Threads)
		if eachSize < minPartSize {
			d.Threads = int(d.TotalSize / minPartSize)
			if d.Threads < 1 {
				d.Threads = 1
			}
			eachSize = d.TotalSize / int64(d.Threads)
		}

		for i := 0; i < d.Threads; i++ {
			start := eachSize * int64(i)
			end := eachSize*int64(i+1) - 1
			if i == d.Threads-1 {
				end = d.TotalSize - 1
			}
			d.tasks = append(d.tasks, &Task{
				taskID:     i,
				rangeStart: start,
				rangeEnd:   end,
			})
		}
	} else {
		d.Threads = 1
		end := int64(-1)
		if d.TotalSize > 0 {
			end = d.TotalSize - 1
		}
		d.tasks = append(d.tasks, &Task{
			taskID:     0,
			rangeStart: 0,
			rangeEnd:   end,
		})
	}
}

// Start 开始下载
func (d *Downloader) Start() error {
	if err := d.init(); err != nil {
		return err
	}
	d.createTasks()

	err := d.doDownload()

	if d.file != nil {
		d.file.Close()
	}
	return err
}

// doDownload 执行实际下载
func (d *Downloader) doDownload() error {
	wg := &sync.WaitGroup{}
	progressChan := make(chan struct{ taskID int; bytes int64 }, len(d.tasks))
	errorChan := make(chan error, len(d.tasks))

	for _, task := range d.tasks {
		wg.Add(1)
		go d.runTask(wg, progressChan, errorChan, task)
	}

	// 进度聚合
	go func() {
		taskProgress := make([]int64, len(d.tasks))
		totalDownloaded := int64(0)

		for p := range progressChan {
			taskProgress[p.taskID] += p.bytes
			totalDownloaded += p.bytes

			if d.progressCb != nil {
				prog := 0.0
				if d.TotalSize > 0 {
					prog = float64(totalDownloaded) / float64(d.TotalSize) * 100
				}
				d.progressCb(totalDownloaded, d.TotalSize, prog)
			}
		}
	}()

	go func() {
		wg.Wait()
		close(progressChan)
		close(errorChan)
	}()

	var errs []error
	for err := range errorChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		// 分片下载失败时降级为单线程
		if d.IsMultiPart {
			log.Warn().Msg("分片下载失败，降级为单线程")
			d.IsMultiPart = false
			d.Threads = 1
			d.createTasks()
			return d.doDownload()
		}
		return fmt.Errorf("下载失败: %v", errs[0])
	}

	// 验证
	for _, task := range d.tasks {
		if !task.isCompleted {
			return fmt.Errorf("分片 %d 未完成", task.taskID)
		}
	}

	return nil
}

// runTask 执行单个分片任务（带重试）
func (d *Downloader) runTask(wg *sync.WaitGroup, progressChan chan struct{ taskID int; bytes int64 }, errorChan chan error, task *Task) {
	defer wg.Done()

	for retries := 0; retries < maxRetries; retries++ {
		err := d.downloadTask(progressChan, task)
		if err == nil {
			task.isCompleted = true
			return
		}

		if strings.Contains(err.Error(), "cancelled") {
			errorChan <- err
			return
		}

		task.err = err
		log.Warn().Err(err).Int("task", task.taskID).Int("retry", retries+1).Msg("分片下载失败")

		if retries < maxRetries-1 {
			select {
			case <-d.ctx.Done():
				errorChan <- fmt.Errorf("分片 %d 被取消", task.taskID)
				return
			case <-time.After(retryDelay):
			}
		}
	}
	errorChan <- fmt.Errorf("分片 %d 重试 %d 次后仍失败: %v", task.taskID, maxRetries, task.err)
}

// downloadTask 执行单次分片下载
func (d *Downloader) downloadTask(progressChan chan struct{ taskID int; bytes int64 }, task *Task) error {
	select {
	case <-d.ctx.Done():
		return fmt.Errorf("下载已取消")
	default:
	}

	req, err := http.NewRequestWithContext(d.ctx, "GET", d.URL, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	d.setHeaders(req)

	if d.IsMultiPart {
		start := task.rangeStart + task.downloadedSize
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, task.rangeEnd))
	}

	client := d.buildClient()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	if d.IsMultiPart && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("服务器不支持 Range 请求，状态码: %d", resp.StatusCode)
	} else if !d.IsMultiPart && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("状态码异常: %d", resp.StatusCode)
	}

	buf := make([]byte, 32*1024)
	for {
		select {
		case <-d.ctx.Done():
			return fmt.Errorf("下载已取消")
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			writeSize := int64(n)
			offset := task.rangeStart + task.downloadedSize
			_, writeErr := d.file.WriteAt(buf[:writeSize], offset)
			if writeErr != nil {
				return fmt.Errorf("写入文件失败 offset=%d: %w", offset, writeErr)
			}

			task.downloadedSize += writeSize
			progressChan <- struct{ taskID int; bytes int64 }{task.taskID, writeSize}

			if d.TotalSize > 0 && task.rangeStart+task.downloadedSize-1 >= task.rangeEnd {
				return nil
			}
		}

		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("读取响应失败: %w", err)
		}
	}
}

// Cancel 取消下载
func (d *Downloader) Cancel() {
	if d.cancelFunc != nil {
		d.cancelFunc()
	}
	if d.file != nil {
		d.file.Close()
	}
}
