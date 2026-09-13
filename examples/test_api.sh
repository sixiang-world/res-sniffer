#!/bin/bash
# res-sniffer API 测试脚本
# 用法: ./examples/test_api.sh [host] [port]

HOST="${1:-127.0.0.1}"
PORT="${2:-8899}"
BASE="http://${HOST}:${PORT}/api/v1"

echo "========================================="
echo "  res-sniffer API 测试"
echo "  目标: ${BASE}"
echo "========================================="
echo ""

# 1. 健康检查
echo "[1] 健康检查 GET /health"
curl -s "${BASE}/health" | python3 -m json.tool 2>/dev/null || curl -s "${BASE}/health"
echo -e "\n"

# 2. 获取配置
echo "[2] 获取配置 GET /config"
curl -s "${BASE}/config" | python3 -m json.tool 2>/dev/null || curl -s "${BASE}/config"
echo -e "\n"

# 3. 下载 CA 证书
echo "[3] 下载 CA 证书 GET /cert"
curl -s -o /tmp/res-sniffer-test-ca.crt "${BASE}/cert"
if [ -f /tmp/res-sniffer-test-ca.crt ]; then
    echo "证书已保存到 /tmp/res-sniffer-test-ca.crt"
    head -1 /tmp/res-sniffer-test-ca.crt
    wc -c /tmp/res-sniffer-test-ca.crt
else
    echo "下载失败"
fi
echo ""

# 4. 列出资源
echo "[4] 列出资源 GET /resources?limit=5"
curl -s "${BASE}/resources?limit=5" | python3 -m json.tool 2>/dev/null || curl -s "${BASE}/resources?limit=5"
echo -e "\n"

# 5. 列出下载任务
echo "[5] 列出下载任务 GET /downloads"
curl -s "${BASE}/downloads" | python3 -m json.tool 2>/dev/null || curl -s "${BASE}/downloads"
echo -e "\n"

# 6. 代理状态
echo "[6] 代理状态 GET /proxy/status"
curl -s "${BASE}/proxy/status" | python3 -m json.tool 2>/dev/null || curl -s "${BASE}/proxy/status"
echo -e "\n"

# 7. 直接下载测试
echo "[7] 直接下载 POST /download"
curl -s -X POST "${BASE}/download" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://www.example.com/test.mp4","save_path":""}' | python3 -m json.tool 2>/dev/null || \
  curl -s -X POST "${BASE}/download" -H "Content-Type: application/json" -d '{"url":"https://www.example.com/test.mp4"}'
echo -e "\n"

echo "========================================="
echo "  测试完成"
echo "========================================="
