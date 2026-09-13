---
name: res-sniffer
description: Use when the user needs to sniff, detect, or download network media resources (videos, audio, images, m3u8 streams, live streams) via a local MITM proxy, or wants to control the res-sniffer CLI / HTTP API server for resource capture and download tasks.
---

# res-sniffer

## Overview

res-sniffer is an AI-friendly network resource sniffer and downloader. It starts a local MITM (Man-in-the-Middle) proxy that intercepts HTTP/HTTPS traffic, detects media resources (video, audio, image, m3u8, live stream, PDF, documents) by Content-Type and domain rules, and downloads them with multi-threaded concurrent support.

It provides two modes:
- **CLI mode**: command-line operations for direct use and scripting
- **Server mode**: HTTP REST API + SSE (Server-Sent Events) for AI / programmatic integration

## When to Use

- User wants to download videos/music/images from web pages or apps that don't provide direct download links
- User needs to batch-capture media resources from browsing sessions
- User asks for a local proxy-based resource downloader
- User wants to integrate resource sniffing into an automated workflow via API
- User mentions "嗅探", "下载视频", "抓包下载", "mitm 下载", "资源嗅探"

## Prerequisites

- Binary: `res-sniffer` (installed via `go install github.com/res-sniffer/res-sniffer/cmd/res-sniffer@latest` or downloaded from releases)
- CA certificate must be installed in the system/browser for HTTPS interception (see Certificate Setup below)
- Default proxy address: `127.0.0.1:8899`

## Quick Start

### 1. Start proxy + API server (recommended)
```bash
res-sniffer start --port 8899 --save-dir ~/Downloads/res-sniffer
```

### 2. Export and install CA certificate
```bash
res-sniffer cert export --output ~/res-sniffer-ca.crt
# Linux (Debian/Ubuntu):
sudo cp ~/res-sniffer-ca.crt /usr/local/share/ca-certificates/
sudo update-ca-certificates
# Or download via API: curl http://127.0.0.1:8899/api/v1/cert -o ca.crt
```

### 3. Set system/browser proxy to `127.0.0.1:8899`

### 4. Browse normally — detected resources appear via SSE or the resources API

## CLI Commands

### `res-sniffer start`
Start both MITM proxy and API server.
```
res-sniffer start [flags]
Flags:
  --port string       Proxy/API port (default "8899")
  --host string       Listen address (default "127.0.0.1")
  --save-dir string   Download save directory (default "~/Downloads/res-sniffer")
  --no-api            Start proxy only, disable API server
  --no-auto-download  Sniff only, don't auto-download detected resources
```

### `res-sniffer proxy`
Start proxy only (no API server), auto-download detected resources.
```
res-sniffer proxy [--port 8899] [--save-dir ./downloads]
```

### `res-sniffer server`
Start API server only (proxy controlled via API).
```
res-sniffer server [--port 8899] [--host 127.0.0.1]
```

### `res-sniffer download <url>`
Direct download a URL without proxy.
```
res-sniffer download "https://example.com/video.mp4" --save-dir ./downloads
```

### `res-sniffer cert export`
Export CA certificate to file.
```
res-sniffer cert export --output ./ca.crt
```

### `res-sniffer cert install`
Attempt to install CA certificate to system trust store (may require sudo).

### `res-sniffer config show`
Display current configuration as JSON.

### `res-sniffer list`
List sniffed resources (requires running server).
```
res-sniffer list --type video,audio --limit 20
```

### `res-sniffer version`
Print version.

## HTTP API Reference

Base URL: `http://127.0.0.1:8899/api/v1`

All responses use envelope: `{"code": 0, "message": "ok", "data": {...}}` where `code=0` means success.

### Health
```
GET /health
Response: {"version":"v0.1.0","proxy_running":true,"api_running":true}
```

### Configuration
```
GET  /config          → Get current config
PUT  /config          → Update config (JSON body, partial update supported)
```

### Resources (sniffed media)
```
GET    /resources?type=video&limit=50   → List detected resources
GET    /resources/{id}                  → Get resource detail
DELETE /resources/{id}                  → Remove resource record
POST   /resources/{id}/download         → Download a detected resource
```

Resource object:
```json
{
  "id": "abc123",
  "url": "https://cdn.example.com/video.mp4",
  "url_sign": "md5hash",
  "cover_url": "",
  "size": 10485760,
  "domain": "example.com",
  "classify": "video",
  "suffix": ".mp4",
  "status": "ready",
  "save_path": "",
  "content_type": "video/mp4",
  "description": "",
  "detected_at": "2026-09-13T12:00:00Z"
}
```

### Direct Download
```
POST /download
Body: {"url":"https://...","save_path":"/optional/path","headers":{"User-Agent":"..."}}
Response: {"task_id":"xyz789"}
```

### Download Tasks
```
GET  /downloads              → List all download tasks
GET  /downloads/{id}         → Get task status/progress
POST /downloads/{id}/cancel  → Cancel a running download
```

Download task object:
```json
{
  "id": "xyz789",
  "url": "https://cdn.example.com/video.mp4",
  "save_path": "/home/user/Downloads/res-sniffer/video.mp4",
  "total_size": 10485760,
  "downloaded": 5242880,
  "status": "running",
  "progress": 50.0,
  "error": "",
  "created_at": "2026-09-13T12:00:00Z"
}
```

### Proxy Control
```
GET  /proxy/status   → {"running": true, "port": "8899", "mitm_rule": "*"}
POST /proxy/start    → Start proxy
POST /proxy/stop     → Stop proxy
```

### Certificate
```
GET /cert  → Download CA certificate file (res-sniffer-ca.crt)
```

### SSE Events (Real-time)
```
GET /events  → text/event-stream
```

Event types:
- `resource.new` — a new media resource was detected
- `download.progress` — download progress updated (includes task_id, progress, downloaded, total)
- `download.done` — download completed (includes task_id, save_path)
- `download.error` — download failed (includes task_id, error)

Example SSE consumer:
```bash
curl -N http://127.0.0.1:8899/api/v1/events
```

## Certificate Setup (Critical for HTTPS)

The MITM proxy needs its CA certificate trusted to intercept HTTPS traffic.

1. **Export cert**: `res-sniffer cert export --output ca.crt` or `curl http://127.0.0.1:8899/api/v1/cert -o ca.crt`
2. **Install system-wide**:
   - Linux (Debian/Ubuntu): `sudo cp ca.crt /usr/local/share/ca-certificates/res-sniffer.crt && sudo update-ca-certificates`
   - macOS: `sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain ca.crt`
   - Windows: Right-click → Install Certificate → Local Machine → Trusted Root Certification Authorities
3. **Browser-specific**: Firefox uses its own cert store; import via Settings → Privacy & Security → Certificates
4. For mobile devices: download cert via browser after setting proxy, then install in device settings

## Resource Classification

| Classify | Content-Types (examples) | Suffix |
|----------|-------------------------|--------|
| video | video/mp4, video/webm, video/quicktime | .mp4, .webm, .mov |
| audio | audio/mpeg, audio/wav, audio/flac, audio/mp4 | .mp3, .wav, .flac, .m4a |
| image | image/png, image/jpeg, image/webp, image/gif | .png, .jpg, .webp, .gif |
| m3u8 | application/vnd.apple.mpegurl, application/x-mpegurl | .m3u8 |
| live | video/x-flv, audio/video, application/dash+xml | .flv, .mpd |
| pdf | application/pdf | .pdf |
| doc | application/msword, application/vnd.openxmlformats... | .doc, .docx |
| xls | application/vnd.ms-excel, text/csv | .xls, .xlsx, .csv |

## Typical AI Workflow

1. Start server: `res-sniffer start --save-dir /path/to/downloads`
2. Verify health: `GET /api/v1/health`
3. Subscribe to SSE: `GET /api/v1/events` to receive real-time resource detection
4. User browses target site with proxy set to `127.0.0.1:8899`
5. AI receives `resource.new` events, filters by type
6. AI calls `POST /api/v1/resources/{id}/download` or `POST /api/v1/download` to download
7. AI monitors `download.progress` / `download.done` events
8. Retrieve downloaded files from save directory

## Notes

- The proxy and API share the same port (default 8899); requests to `/api/v1/*` go to API, others go to proxy
- Resources are deduplicated by MD5(URL)
- Multi-threaded download uses Range requests; falls back to single-thread if server doesn't support Range
- Default MITM rule is `*` (intercept all); configure via `PUT /api/v1/config` with `mitm_rule` field
- v0.1 does not include: CDN cache, webhook, OpenAPI docs, user auth, AI summarization
