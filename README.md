# Anthropic Protocol Proxy

将 Anthropic API 协议转写为多种后端协议（OpenAI、Claude、Gemini）的网络代理。

## 快速开始

### 安装

```bash
go build -o proxy ./cmd/proxy
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

```bash
./proxy -config config.yaml
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

- **OpenAI** - `/chat/completions` 端点
- **Claude** - Anthropic 原生 API
- **Gemini** - Google AI `/generateContent` 端点

## 特性

- **路由表认证** - 不同客户端 Key 路由到不同后端
- **流式转发** - SSE 低延迟转发
- **连接池** - HTTP 连接复用
- **结构化日志** - JSON 格式，包含延迟、状态码、Token 数
- **自动重试** - 对 429/503/504 错误自动重试

## 许可证

MIT
