# res-sniffer

> AI 友好的网络资源嗅探下载工具 v0.1.0

基于 MITM 代理原理，自动嗅探浏览器/应用通过代理的视频、音频、图片、m3u8 等网络资源，支持 CLI 和 HTTP API 两种模式。

## 架构

```
┌─────────────────────────────────────────────┐
│                  res-sniffer                 │
│                                              │
│  ┌──────────┐    ┌──────────┐    ┌────────┐ │
│  │   CLI    │───▶│  Server  │◀──▶│  SSE   │ │
│  │ (cobra)  │    │  (HTTP)  │    │ (推送) │ │
│  └──────────┘    └────┬─────┘    └────────┘ │
│                       │                      │
│              ┌────────▼────────┐            │
│              │     Proxy        │            │
│              │  (goproxy MITM)  │            │
│              └────────┬────────┘            │
│                       │                      │
│         ┌─────────────┼─────────────┐      │
│         ▼             ▼             ▼       │
│    ┌─────────┐  ┌─────────┐  ┌──────────┐  │
│    │ Sniffer │  │  Rule   │  │   Cert   │  │
│    │(插件系统)│  │(规则引擎)│  │(CA证书)  │  │
│    └────┬────┘  └─────────┘  └──────────┘  │
│         │                                   │
│    ┌────▼────┐    ┌──────────────┐         │
│    │Default  │    │  Downloader  │         │
│    │Plugin   │    │ (分片并发)   │         │
│    │(CT识别) │    └──────────────┘         │
│    └─────────┘                              │
│    ┌─────────┐                              │
│    │WeChat   │  (v0.1 简化版)              │
│    │Plugin   │                              │
│    └─────────┘                              │
└─────────────────────────────────────────────┘
```

## 安装

### 从源码编译

```bash
git clone <repo>
cd res-sniffer
go build -o res-sniffer ./cmd/res-sniffer/
```

### 前置要求

- Go 1.23+
- Linux/macOS（Windows 支持待完善）

## 快速开始

### 1. 启动服务

```bash
# 默认模式：代理 + API 同端口
./res-sniffer start

# 指定端口
./res-sniffer start --port 9000 --host 0.0.0.0
```

### 2. 安装 CA 证书（HTTPS 嗅探必需）

```bash
# 导出证书
./res-sniffer cert export --output ./ca.crt

# 安装到系统（Linux）
./res-sniffer cert install
# 然后执行: sudo update-ca-certificates
```

### 3. 配置浏览器代理

将浏览器 HTTP/HTTPS 代理设置为 `127.0.0.1:8899`，访问网页后即可嗅探资源。

## CLI 用法

```bash
# 启动代理 + API 服务器
res-sniffer start [--port 8899] [--host 127.0.0.1] [--save-dir ./downloads] [--no-api]

# 仅代理模式
res-sniffer proxy [--port 8899] [--save-dir ./downloads]

# 仅 API 模式
res-sniffer server [--port 8899] [--host 127.0.0.1]

# 直接下载 URL
res-sniffer download <url> [--save-dir ./downloads]

# CA 证书管理
res-sniffer cert export [--output ./ca.crt]
res-sniffer cert install

# 配置
res-sniffer config show

# 列出嗅探资源
res-sniffer list [--type video,audio] [--limit 20]

# 版本
res-sniffer version
```

## API 文档

基础路径：`http://127.0.0.1:8899/api/v1`

所有响应格式：`{"code": 0, "message": "ok", "data": {...}}`

### 接口列表

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /health | 健康检查 |
| GET | /config | 获取配置 |
| PUT | /config | 更新配置 |
| GET | /resources | 列出资源（支持 type、limit 参数） |
| GET | /resources/{id} | 获取资源详情 |
| DELETE | /resources/{id} | 删除资源 |
| POST | /resources/{id}/download | 下载已嗅探资源 |
| POST | /download | 直接下载 URL |
| GET | /downloads | 列出下载任务 |
| GET | /downloads/{id} | 获取下载任务状态 |
| POST | /downloads/{id}/cancel | 取消下载 |
| GET | /proxy/status | 代理状态 |
| POST | /proxy/start | 启动代理 |
| POST | /proxy/stop | 停止代理 |
| GET | /cert | 下载 CA 证书 |
| GET | /events | SSE 事件流 |

### SSE 事件

连接 `GET /api/v1/events` 后可接收以下事件：

- `resource.new` — 新资源被嗅探到
- `download.progress` — 下载进度更新
- `download.done` — 下载完成
- `download.error` — 下载出错

## 配置

配置文件位置：`~/.config/res-sniffer/config.json`

```json
{
  "host": "127.0.0.1",
  "port": "8899",
  "save_directory": "~/Downloads",
  "task_number": 8,
  "rule": "*",
  "auto_download": false,
  "mime_map": { ... }
}
```

### MITM 规则语法

- `*` — 解密全部 HTTPS 流量（默认）
- `*.example.com` — 仅解密 example.com 及其子域名
- `!ads.com` — 排除特定域名
- 多行规则用换行分隔，支持 `#` 注释

## CA 证书安装说明

### Linux

```bash
# 导出证书
./res-sniffer cert export --output /tmp/res-sniffer-ca.crt

# 安装到系统信任库
sudo cp /tmp/res-sniffer-ca.crt /usr/local/share/ca-certificates/res-sniffer.crt
sudo update-ca-certificates
```

### macOS

```bash
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain ca.crt
```

### Windows

双击 `ca.crt` → 安装证书 → 本地计算机 → 受信任的根证书颁发机构

## 项目结构

```
res-sniffer/
├── cmd/res-sniffer/main.go    # CLI 入口
├── internal/
│   ├── model/model.go         # 数据模型
│   ├── config/config.go       # 配置管理
│   ├── proxy/                  # MITM 代理
│   │   ├── proxy.go            # 代理核心
│   │   ├── cert.go             # CA 证书
│   │   └── rule.go             # 规则引擎
│   ├── sniffer/                # 资源嗅探插件
│   │   ├── plugin.go           # 插件接口
│   │   ├── default.go          # 默认插件
│   │   └── wechat.go           # 微信插件（简化版）
│   ├── downloader/downloader.go # 多线程下载器
│   └── server/                  # HTTP API
│       ├── server.go           # 服务器核心
│       ├── handlers.go         # API 处理器
│       ├── sse.go              # SSE 推送
│       └── eventbus.go         # 事件总线
├── examples/test_api.sh       # API 测试脚本
├── go.mod / go.sum
├── README.md
└── LICENSE
```

## License

MIT
