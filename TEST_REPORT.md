# res-sniffer v0.1.0 测试报告

**测试日期**: 2026-09-13
**测试环境**: Linux, Go 1.23.2
**测试结果**: ✅ 全部通过

## 1. 编译验证

| 检查项 | 结果 |
|--------|------|
| `go mod tidy` | ✅ 通过 |
| `go build -o res-sniffer ./cmd/res-sniffer/` | ✅ 通过 (13MB 二进制) |
| `go vet ./...` | ✅ 无错误 |

## 2. CLI 命令测试

| 命令 | 结果 |
|------|------|
| `res-sniffer version` | ✅ 输出 v0.1.0 |
| `res-sniffer --help` | ✅ 显示 8 个子命令 |
| `res-sniffer cert export --output ca.crt` | ✅ 导出 1159 字节 PEM 证书 |
| `res-sniffer config show` | ✅ 显示配置 (59 条 MIME 映射) |
| `res-sniffer cert --help` | ✅ 显示 export/install 子命令 |

## 3. HTTP API 测试 (7 项)

| 端点 | 方法 | 结果 |
|------|------|------|
| `/api/v1/health` | GET | ✅ 返回版本 v0.1.0 和运行状态 |
| `/api/v1/config` | GET | ✅ 返回完整配置 (59 MIME 映射) |
| `/api/v1/cert` | GET | ✅ 下载 1159 字节 CA 证书 |
| `/api/v1/resources` | GET | ✅ 返回资源列表 |
| `/api/v1/downloads` | GET | ✅ 返回下载任务列表 |
| `/api/v1/proxy/status` | GET | ✅ 返回代理运行状态 |
| `/api/v1/download` | POST | ✅ 创建下载任务并返回 task_id |
| `/api/v1/proxy/start` | POST | ✅ 启动代理 |

## 4. 代理嗅探测试

通过 MITM 代理请求本地测试服务器，验证资源识别：

| 资源 | Content-Type | 识别分类 | 文件后缀 | 结果 |
|------|-------------|---------|---------|------|
| test.mp4 | video/mp4 | video | .mp4 | ✅ |
| test.mp3 | audio/mpeg | audio | .mp3 | ✅ |
| test.jpg | image/jpeg | image | .jpeg | ✅ |
| test.m3u8 | application/vnd.apple.mpegurl | m3u8 | .m3u8 | ✅ |

**去重验证**: 重复请求同一 URL 不会重复添加资源 ✅

## 5. 下载功能测试

| 测试项 | 结果 |
|--------|------|
| 下载已嗅探资源 (`POST /resources/{id}/download`) | ✅ 文件成功保存 |
| 直接下载 URL (`POST /download`) | ✅ 文件内容正确 |
| 下载进度回调 | ✅ progress=100, total_size 正确 |
| 多线程 Range 下载 | ✅ 支持 (服务端支持 Range 时) |
| 下载任务状态查询 | ✅ 可通过 `/downloads/{id}` 查询 |

## 6. CA 证书测试

| 测试项 | 结果 |
|--------|------|
| 首次运行自动生成 RSA 2048 证书 | ✅ |
| 证书持久化到配置目录 | ✅ |
| 通过 API 下载证书 | ✅ |
| 通过 CLI 导出证书 | ✅ |

## 7. 已知限制 (v0.1)

- 下载任务完成后状态可能短暂显示 "running" 而非 "done" (进度已 100%)
- SSE 事件流需在无代理干扰的环境下验证
- 微信视频号插件为简化版，未实现 JS 注入
- 未实现: CDN缓存、Webhook、版本历史、E2E加密、OpenAPI文档、用户系统、AI摘要
