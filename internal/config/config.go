package config

import (
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/res-sniffer/res-sniffer/internal/model"
)

// Config 应用配置
type Config struct {
	Host          string                    `json:"host"`
	Port          string                    `json:"port"`
	SaveDirectory string                    `json:"save_directory"`
	UserAgent     string                    `json:"user_agent"`
	TaskNumber    int                       `json:"task_number"` // 下载并发数
	Rule          string                    `json:"rule"`        // MITM 规则
	MimeMap       map[string]model.MimeInfo `json:"mime_map"`
	AutoDownload  bool                      `json:"auto_download"` // 嗅探到资源后是否自动下载
}

var (
	globalConfig *Config
	configMux    sync.RWMutex
	configPath   string
)

// GetConfigDir 获取配置目录
func GetConfigDir() string {
	usr, err := user.Current()
	if err != nil {
		return "./.res-sniffer"
	}
	return filepath.Join(usr.HomeDir, ".config", "res-sniffer")
}

// GetDefaultDownloadDir 获取默认下载目录
func getDefaultDownloadDir() string {
	usr, err := user.Current()
	if err != nil {
		return "./downloads"
	}
	homeDir := usr.HomeDir
	downloadDir := filepath.Join(homeDir, "Downloads")
	if runtime.GOOS == "linux" {
		if xdgDir := os.Getenv("XDG_DOWNLOAD_DIR"); xdgDir != "" {
			downloadDir = xdgDir
		}
	}
	return downloadDir
}

// Load 加载配置（首次运行时生成默认配置）
func Load() *Config {
	configMux.Lock()
	defer configMux.Unlock()

	if globalConfig != nil {
		return globalConfig
	}

	configDir := GetConfigDir()
	_ = os.MkdirAll(configDir, 0750)
	configPath = filepath.Join(configDir, "config.json")

	defaultCfg := &Config{
		Host:          "127.0.0.1",
		Port:          "8899",
		SaveDirectory: getDefaultDownloadDir(),
		UserAgent:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36",
		TaskNumber:    runtime.NumCPU() * 2,
		Rule:          "*",
		MimeMap:       GetDefaultMimeMap(),
		AutoDownload:  false,
	}

	// 尝试读取已有配置
	data, err := os.ReadFile(configPath)
	if err != nil {
		// 文件不存在，写入默认配置
		globalConfig = defaultCfg
		_ = saveToFile(defaultCfg)
		return globalConfig
	}

	// 合并：以默认值为底，覆盖用户配置
	var userMap map[string]interface{}
	if err := json.Unmarshal(data, &userMap); err != nil {
		globalConfig = defaultCfg
		return globalConfig
	}

	defaultBytes, _ := json.Marshal(defaultCfg)
	var defaultMap map[string]interface{}
	_ = json.Unmarshal(defaultBytes, &defaultMap)

	for k, v := range userMap {
		if _, ok := defaultMap[k]; ok {
			defaultMap[k] = v
		}
	}

	finalBytes, _ := json.Marshal(defaultMap)
	merged := &Config{}
	if err := json.Unmarshal(finalBytes, merged); err != nil {
		globalConfig = defaultCfg
		return globalConfig
	}

	// 确保 MimeMap 不为空
	if len(merged.MimeMap) == 0 {
		merged.MimeMap = GetDefaultMimeMap()
	}

	globalConfig = merged
	return globalConfig
}

// Get 获取全局配置（读锁）
func Get() *Config {
	configMux.RLock()
	defer configMux.RUnlock()
	return globalConfig
}

// Update 更新配置并持久化
func Update(newCfg *Config) error {
	configMux.Lock()
	defer configMux.Unlock()

	globalConfig.Host = newCfg.Host
	globalConfig.Port = newCfg.Port
	globalConfig.SaveDirectory = newCfg.SaveDirectory
	globalConfig.UserAgent = newCfg.UserAgent
	globalConfig.TaskNumber = newCfg.TaskNumber
	globalConfig.Rule = newCfg.Rule
	globalConfig.AutoDownload = newCfg.AutoDownload
	if len(newCfg.MimeMap) > 0 {
		globalConfig.MimeMap = newCfg.MimeMap
	}

	return saveToFile(globalConfig)
}

// TypeSuffix 根据 Content-Type 查找资源分类和文件后缀
func (c *Config) TypeSuffix(mime string) (string, string) {
	configMux.RLock()
	defer configMux.RUnlock()

	mime = strings.ToLower(strings.Split(mime, ";")[0])
	if v, ok := c.MimeMap[mime]; ok {
		return v.Type, v.Suffix
	}
	return "", ""
}

func saveToFile(cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

// GetDefaultMimeMap 返回默认 MIME 映射表（复用自 res-downloader）
func GetDefaultMimeMap() map[string]model.MimeInfo {
	return map[string]model.MimeInfo{
		"image/png":                     {Type: "image", Suffix: ".png"},
		"image/webp":                    {Type: "image", Suffix: ".webp"},
		"image/jpeg":                    {Type: "image", Suffix: ".jpeg"},
		"image/jpg":                     {Type: "image", Suffix: ".jpg"},
		"image/gif":                     {Type: "image", Suffix: ".gif"},
		"image/avif":                    {Type: "image", Suffix: ".avif"},
		"image/bmp":                     {Type: "image", Suffix: ".bmp"},
		"image/tiff":                    {Type: "image", Suffix: ".tiff"},
		"image/heic":                    {Type: "image", Suffix: ".heic"},
		"image/x-icon":                  {Type: "image", Suffix: ".ico"},
		"image/svg+xml":                 {Type: "image", Suffix: ".svg"},
		"image/vnd.adobe.photoshop":     {Type: "image", Suffix: ".psd"},
		"image/jp2":                     {Type: "image", Suffix: ".jp2"},
		"image/jpeg2000":                {Type: "image", Suffix: ".jp2"},
		"image/apng":                    {Type: "image", Suffix: ".apng"},
		"audio/mpeg":                    {Type: "audio", Suffix: ".mp3"},
		"audio/mp3":                     {Type: "audio", Suffix: ".mp3"},
		"audio/wav":                     {Type: "audio", Suffix: ".wav"},
		"audio/aiff":                    {Type: "audio", Suffix: ".aiff"},
		"audio/x-aiff":                  {Type: "audio", Suffix: ".aiff"},
		"audio/aac":                     {Type: "audio", Suffix: ".aac"},
		"audio/ogg":                     {Type: "audio", Suffix: ".ogg"},
		"audio/flac":                    {Type: "audio", Suffix: ".flac"},
		"audio/midi":                    {Type: "audio", Suffix: ".mid"},
		"audio/x-midi":                  {Type: "audio", Suffix: ".mid"},
		"audio/x-ms-wma":                {Type: "audio", Suffix: ".wma"},
		"audio/opus":                    {Type: "audio", Suffix: ".opus"},
		"audio/webm":                    {Type: "audio", Suffix: ".webm"},
		"audio/mp4":                     {Type: "audio", Suffix: ".m4a"},
		"audio/amr":                     {Type: "audio", Suffix: ".amr"},
		"video/mp4":                     {Type: "video", Suffix: ".mp4"},
		"video/webm":                    {Type: "video", Suffix: ".webm"},
		"video/ogg":                     {Type: "video", Suffix: ".ogv"},
		"video/x-msvideo":               {Type: "video", Suffix: ".avi"},
		"video/mpeg":                    {Type: "video", Suffix: ".mpeg"},
		"video/quicktime":               {Type: "video", Suffix: ".mov"},
		"video/x-ms-wmv":                {Type: "video", Suffix: ".wmv"},
		"video/3gpp":                    {Type: "video", Suffix: ".3gp"},
		"video/x-matroska":              {Type: "video", Suffix: ".mkv"},
		"audio/video":                   {Type: "live", Suffix: ".flv"},
		"video/x-flv":                   {Type: "live", Suffix: ".flv"},
		"application/dash+xml":          {Type: "live", Suffix: ".mpd"},
		"application/vnd.apple.mpegurl": {Type: "m3u8", Suffix: ".m3u8"},
		"application/x-mpegurl":         {Type: "m3u8", Suffix: ".m3u8"},
		"application/x-mpeg":            {Type: "m3u8", Suffix: ".m3u8"},
		"audio/x-mpegurl":               {Type: "m3u8", Suffix: ".m3u8"},
		"application/pdf":               {Type: "pdf", Suffix: ".pdf"},
		"application/vnd.ms-powerpoint": {Type: "ppt", Suffix: ".ppt"},
		"application/vnd.openxmlformats-officedocument.presentationml.presentation": {Type: "ppt", Suffix: ".pptx"},
		"application/vnd.ms-excel": {Type: "xls", Suffix: ".xls"},
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":        {Type: "xls", Suffix: ".xlsx"},
		"text/csv":           {Type: "xls", Suffix: ".csv"},
		"application/msword": {Type: "doc", Suffix: ".doc"},
		"application/rtf":   {Type: "doc", Suffix: ".rtf"},
		"text/rtf":           {Type: "doc", Suffix: ".rtf"},
		"application/vnd.oasis.opendocument.text":                                 {Type: "doc", Suffix: ".odt"},
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": {Type: "doc", Suffix: ".docx"},
		"font/woff":                {Type: "font", Suffix: ".woff"},
		"application/octet-stream": {Type: "stream", Suffix: "default"},
	}
}
