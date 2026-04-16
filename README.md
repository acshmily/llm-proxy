# Anthropic Protocol Proxy

将 Anthropic API 协议转写为多种后端协议（OpenAI、Claude、Gemini）的网络代理。

[![Go](https://github.com/acshmily/llm-proxy/actions/workflows/go.yml/badge.svg)](https://github.com/acshmily/llm-proxy/actions/workflows/go.yml)
[![Codecov](https://codecov.io/gh/acshmily/llm-proxy/branch/main/graph/badge.svg)](https://codecov.io/gh/acshmily/llm-proxy)
[![Go Report Card](https://goreportcard.com/badge/github.com/acshmily/llm-proxy)](https://goreportcard.com/report/github.com/acshmily/llm-proxy)
[![Docker Pulls](https://img.shields.io/docker/pulls/acshmily/llm-proxy)](https://hub.docker.com/repository/docker/acshmily/llm-proxy/)
[![Docker Image Size](https://img.shields.io/docker/image-size/acshmily/llm-proxy/latest)](https://hub.docker.com/repository/docker/acshmily/llm-proxy/)
[![License](https://img.shields.io/github/license/acshmily/llm-proxy)](LICENSE)
[![Release](https://img.shields.io/github/v/release/acshmily/llm-proxy)](https://github.com/acshmily/llm-proxy/releases)

**Docker 镜像：** [`acshmily/llm-proxy`](https://hub.docker.com/repository/docker/acshmily/llm-proxy/)

```bash
docker pull acshmily/llm-proxy:latest
```

## 快速开始

### 安装

**本地编译：**

```bash
go build -o proxy ./cmd/proxy
```

**Docker 构建：**

```bash
# 单架构构建
docker build -t llm-proxy:latest .

# 多架构构建（AMD64 + ARM64）
docker buildx build --platform linux/amd64,linux/arm64 -t llm-proxy:latest .

# 推送多架构镜像到仓库
docker buildx build --platform linux/amd64,linux/arm64 \
  -t your-registry/llm-proxy:latest \
  --push .
```

**使用 Makefile：**

```bash
make build            # 本地编译
make docker-build     # Docker 构建（当前架构）
make docker-multiarch # Docker 多架构构建
make docker-push      # Docker 推送多架构镜像
```

### 配置

**注意：** `config.yaml` 包含敏感信息，已被 `.gitignore` 排除。

复制示例配置文件：

```bash
cp config.example.yaml config.yaml
```

然后编辑 `config.yaml` 填入你的 API 密钥：

```yaml
server:
  listen: :8080

logging:
  format: json
  level: info

routes:
  - api_key: "sk-client-1"
    backend: "openai"
    backend_api_key: "sk-openai-xxx"
    timeout: 60s

backends:
  openai:
    base_url: "https://api.openai.com/v1"
```

### 运行

**本地运行：**

```bash
./proxy -config config.yaml
```

**Docker 运行：**

```bash
# 从 Docker Hub 拉取镜像
docker pull acshmily/llm-proxy:latest

# 先复制配置文件
cp config.example.yaml config.yaml

# 编辑配置文件填入 API 密钥
# 然后运行
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  --name llm-proxy \
  acshmily/llm-proxy:latest
```

**注意：** 使用 `:ro` 标志只读挂载配置文件，提高安全性。

**Docker Compose：**

```yaml
version: '3.8'
services:
  proxy:
    image: acshmily/llm-proxy:latest
    ports:
      - "8080:8080"
    volumes:
      - ./config.yaml:/app/config.yaml
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
```

**健康检查端点：**

```bash
curl http://localhost:8080/health
# 返回：{"status":"healthy","time":"2026-04-15T12:00:00Z"}
```

### 使用示例

```bash
curl http://localhost:8080/v1/messages \
  -H "Authorization: Bearer sk-client-1" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3",
    "messages": [{"role":"user","content":[{"type":"text","text":"Hello"}]}]
  }'
```

## 支持的端点

| 端点 | 描述 |
|------|------|
| `POST /v1/messages` | 聊天完成（支持流式） |
| `GET /v1/models` | 获取模型列表 |
| `POST /v1/messages/count_tokens` | Token 计数 |

## 支持的后端

| 后端名称 | 配置值 | API 端点 | 说明 |
|----------|--------|----------|------|
| **OpenAI** | `openai` | `/chat/completions` | 兼容 OpenAI 格式的 API |
| **Anthropic** | `anthropic` | `/v1/messages` | Anthropic 原生 API（Claude 模型） |
| **Gemini** | `gemini` | `/generateContent` | Google AI Gemini API |

## 路由配置示例

```yaml
routes:
  # 转发到 OpenAI
  - api_key: "sk-client-1"
    backend: "openai"
    backend_api_key: "sk-openai-xxx"
    timeout: 60s

  # 转发到 Anthropic (Claude)
  - api_key: "sk-client-2"
    backend: "anthropic"
    backend_api_key: "sk-ant-xxx"
    timeout: 120s

  # 转发到 Gemini
  # 注意：Gemini API Key 格式为 AIzaSyD-xxx，无需 Bearer 前缀
  # 获取方式：https://makersuite.google.com/app/apikey
  - api_key: "sk-client-3"
    backend: "gemini"
    backend_api_key: "AIzaSyD-xxx"
    timeout: 90s

backends:
  # OpenAI 配置
  openai:
    base_url: "https://api.openai.com/v1"

  # Anthropic (Claude) 配置
  anthropic:
    base_url: "https://api.anthropic.com"

  # Gemini 配置
  gemini:
    base_url: "https://generativelanguage.googleapis.com/v1beta"
```

## 特性

- **路由表认证** - 不同客户端 Key 路由到不同后端
- **流式转发** - SSE 低延迟转发
- **连接池** - HTTP 连接复用
- **结构化日志** - JSON 格式，包含延迟、状态码、Token 数
- **自动重试** - 对 429/503/504 错误自动重试
- **多平台支持** - Linux、macOS、Windows，AMD64/ARM64 架构
- **防护机制** - 三层防护避免 GFW 和网络行为特征扫描
  - 流量伪装：TLS 指纹模拟、浏览器头部模拟
  - 行为打散：请求延迟、连接复用打散、请求填充
  - 流量混淆：WebSocket 隧道、请求分片（已实现）

## 防护机制

为防止 GFW 和网络行为特征扫描，代理提供三层防护机制，所有功能均可通过配置开启/关闭。

### 1. 流量伪装（Traffic Camouflage）

**TLS 指纹模拟：**
```yaml
protection:
  traffic_camouflage:
    tls_fingerprint:
      enabled: true
      mode: chrome  # chrome, firefox, safari, random
```

**浏览器头部模拟：**
```yaml
protection:
  traffic_camouflage:
    browser_headers:
      enabled: true
      mode: chrome  # chrome, firefox, safari, random
      custom_user_agent: ""  # 可选：自定义 UA
```

### 2. 行为打散（Behavior Jitter）

**请求延迟：**
```yaml
protection:
  behavior_jitter:
    request_delay:
      enabled: true
      min_ms: 50
      max_ms: 200
      distribution: exponential  # uniform, exponential
```

**连接复用打散：**
```yaml
protection:
  behavior_jitter:
    connection_reuse:
      enabled: true
      reuse_rate: 0.7  # 0.0-1.0，70% 复用率
```

**请求填充：**
```yaml
protection:
  behavior_jitter:
    request_padding:
      enabled: true
      min_bytes: 10
      max_bytes: 100
      mode: random  # random, fixed
      fixed_size: 64  # mode=fixed 时使用
```

### 3. 流量混淆（Traffic Obfuscation）

> **状态：** 已实现（v0.2.0+）

**WebSocket 隧道：**
```yaml
protection:
  traffic_obfuscation:
    websocket_tunnel:
      enabled: true
      path: "/ws-tunnel"
      ping_interval_ms: 30000
```

**请求分片：**
```yaml
protection:
  traffic_obfuscation:
    request_sharding:
      enabled: true
      max_chunk_size: 1024  # 最大分片大小（字节）
```

### 完整防护配置示例

```yaml
protection:
  enabled: true
  
  # 流量伪装
  traffic_camouflage:
    tls_fingerprint:
      enabled: true
      mode: chrome
    browser_headers:
      enabled: true
      mode: chrome
  
  # 行为打散
  behavior_jitter:
    request_delay:
      enabled: true
      min_ms: 50
      max_ms: 200
      distribution: exponential
    connection_reuse:
      enabled: true
      reuse_rate: 0.7
    request_padding:
      enabled: true
      min_bytes: 10
      max_bytes: 100
      mode: random
  
  # 流量混淆
  traffic_obfuscation:
    websocket_tunnel:
      enabled: false
    request_sharding:
      enabled: false
```

## Linux 系统服务部署

将 LLM Proxy 安装为 systemd 服务，实现后台运行、开机自启、日志管理。

**一键安装：**

```bash
curl -fsSL https://raw.githubusercontent.com/acshmily/llm-proxy/main/deploy/linux/install.sh | sudo bash
```

**服务管理：**

```bash
sudo systemctl start llm-proxy      # 启动
sudo systemctl stop llm-proxy       # 停止
sudo systemctl restart llm-proxy    # 重启
sudo systemctl status llm-proxy     # 状态
sudo systemctl enable llm-proxy     # 开机自启
```

**日志查看：**

```bash
sudo journalctl -u llm-proxy -f     # 实时日志
sudo journalctl -u llm-proxy -n 100 # 最近 100 行
```

详细文档：[deploy/linux/README.md](deploy/linux/README.md)

---

## Nginx 反向代理部署

生产环境建议使用 Nginx 作为反向代理，提供 TLS 终止、限流、防护功能。

### 部署架构

```
Internet → Nginx (443/80) → llm-proxy (8080)
```

### Docker Compose 部署

```bash
cd deploy/docker
docker compose up -d
```

### 准备工作

**1. 准备 SSL 证书：**
```bash
mkdir -p deploy/nginx/ssl
# 将证书文件放入：
# - deploy/nginx/ssl/fullchain.pem  (证书链)
# - deploy/nginx/ssl/privkey.pem    (私钥)
```

**2. 修改 Nginx 配置：**

编辑 `deploy/nginx/nginx.conf`，替换：
- `your-domain.com` → 你的域名

**3. 准备配置文件：**
```bash
cp config.example.yaml deploy/docker/config.yaml
# 编辑 config.yaml 填入 API 密钥
```

### Nginx 功能

- **TLS 终止** - TLSv1.2/TLSv1.3，OCSP Stapling 优化
- **限流防护** - IP 限流 10r/s，全局限流 100r/s
- **安全头部** - HSTS、X-Frame-Options、XSS 保护
- **WebSocket 支持** - `/ws` 路径长连接支持
- **健康检查** - `/health` 端点公开访问
- **伪装路径** - `/static/`、`/about`、`/robots.txt`

详细配置见 [deploy/nginx/README-SSL.md](deploy/nginx/README-SSL.md)

## 多架构 Docker 镜像

**GitHub Actions 自动构建：**
- `linux/amd64` - Intel/AMD 服务器
- `linux/arm64` - ARM 服务器、Apple Silicon

**本地构建：**
```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  -t llm-proxy:latest \
  --load .
```

**推送多架构镜像：**
```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  -t your-registry/llm-proxy:latest \
  --push .
```

查看本地版本：

```bash
./proxy -version
```

## 文档

- **[贡献指南](CONTRIBUTING.md)** - 开发规范、测试要求、提交流程
- **[变更日志](CHANGELOG.md)** - 版本历史、升级指南
- **[防护机制说明](docs/protection.md)** - 三层防护详细配置
- **[WebSocket 隧道指南](docs/ws-client-guide.md)** - 客户端使用文档

## 相关项目

- [Superpowers](https://github.com/garretth/superpowers) - AI 驱动开发技能扩展

## 许可证

MIT License - 详见 [LICENSE](LICENSE) 文件

## 贡献

欢迎贡献代码、报告问题或提出建议！

1. Fork 本项目
2. 创建功能分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'feat: add amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 创建 Pull Request

详见 [贡献指南](CONTRIBUTING.md)。
