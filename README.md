# Anthropic Protocol Proxy

将 Anthropic API 协议转写为多种后端协议（OpenAI、Claude、Gemini）的网络代理。

## 快速开始

### 安装

**本地编译：**

```bash
go build -o proxy ./cmd/proxy
```

**Docker 构建：**

```bash
# 单架构构建
docker build -t proxy-gemini-go:latest .

# 多架构构建（AMD64 + ARM64）
docker buildx build --platform linux/amd64,linux/arm64 -t proxy-gemini-go:latest .

# 推送多架构镜像到仓库
docker buildx build --platform linux/amd64,linux/arm64 \
  -t your-registry/proxy-gemini-go:latest \
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

编辑 `config.yaml`：

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
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  --name proxy-gemini-go \
  proxy-gemini-go:latest
```

**Docker Compose：**

```yaml
version: '3.8'
services:
  proxy:
    image: proxy-gemini-go:latest
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

## 许可证

MIT
