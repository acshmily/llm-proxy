# Anthropic 协议代理设计文档

## 概述

构建一个 Go 语言网络代理，将 Anthropic API 协议转写为多种后端协议（OpenAI、Claude、Gemini）。

## 架构设计

```
┌──────────────┐     ┌─────────────────────────────────────┐     ┌──────────────┐
│   Client     │────▶│         Proxy (统一中间格式)         │────▶│   Backend    │
│  (Anthropic  │     │                                     │     │  (OpenAI/    │
│   Protocol)  │     │  ┌─────────┐  ┌──────────────────┐   │     │   Claude/    │
│              │◀────│  │ Anthropic│  │  Protocol        │   │◀────│   Gemini)    │
└──────────────┘     │  │ Parser  │  │  Converters      │   │     └──────────────┘
                     │  └─────────┘  │  - OpenAI        │   │
                     │  ┌─────────┐  │  - Claude        │   │
                     │  │ Router  │  │  - Gemini        │   │
                     │  │ + Auth  │  │                  │   │
                     │  └─────────┘  └──────────────────┘   │
                     └─────────────────────────────────────┘
```

## 核心组件

| 组件 | 职责 |
|------|------|
| **HTTP Server** | 监听 Anthropic API 端点 (`/v1/messages`, `/v1/models` 等) |
| **Auth Router** | 解析客户端 API Key，查路由表，选择后端和认证信息 |
| **Anthropic Parser** | 解析 Anthropic 请求格式，转为内部统一格式 |
| **Protocol Converters** | 统一格式 → 目标后端协议（OpenAI/Claude/Gemini） |
| **Stream Handler** | 处理 SSE 流式请求和响应转发 |
| **Connection Pool** | 复用后端 HTTP 连接 |
| **Logger** | 结构化日志（元数据：延迟、状态码、Token 数） |

## 配置结构 (config.yaml)

```yaml
server:
  listen: :8080

logging:
  format: json  # 或 text
  level: info

routes:
  - api_key: "sk-client-1"
    backend: "openai"
    api_key: "sk-openai-xxx"
    timeout: 60s
    
  - api_key: "sk-client-2"
    backend: "claude"
    api_key: "sk-claude-xxx"
    timeout: 120s
    
  - api_key: "sk-client-3"
    backend: "gemini"
    api_key: "AIzaSyD-xxx"
    timeout: 90s

backends:
  openai:
    base_url: "https://api.openai.com/v1"
    
  claude:
    base_url: "https://api.anthropic.com/v1"
    
  gemini:
    base_url: "https://generativelanguage.googleapis.com/v1beta"

retry:
  max_attempts: 3
  retry_errors: [429, 503, 504]
```

## 接口支持

| 端点 | 支持 |
|------|------|
| `POST /v1/messages` | ✅ (核心) |
| `POST /v1/messages (stream=true)` | ✅ (SSE) |
| `GET /v1/models` | ✅ |
| `POST /v1/messages/count_tokens` | ✅ |
| `GET /v1/billing/usage` | ✅ |

## 项目结构

```
proxy-gemini-go/
├── cmd/
│   └── proxy/
│       └── main.go
├── internal/
│   ├── config/        # 配置加载和验证
│   ├── router/        # 认证和路由
│   ├── protocol/
│   │   ├── anthropic/ # Anthropic 解析
│   │   ├── openai/    # OpenAI 转换
│   │   ├── claude/    # Claude 转换
│   │   └── gemini/    # Gemini 转换
│   ├── pool/          # 连接池
│   ├── stream/        # SSE 处理
│   └── logger/        # 日志
├── pkg/
│   └── types/         # 公共类型定义
├── test/
│   ├── mock/          # Mock 后端
│   └── integration/   # 集成测试
├── config.yaml
└── go.mod
```

## 设计决策

### 1. 统一中间格式架构
- 选择统一中间格式而非直接映射
- 原因：便于未来扩展其他后端协议，核心逻辑复用

### 2. 认证模式
- 路由表模式：不同客户端 Key 路由到不同后端
- 配置文件定义路由规则

### 3. 日志策略
- 支持 JSON 和文本两种格式
- 只记录元数据（延迟、状态码、Token 数）
- 不记录完整请求/响应内容

### 4. 并发模型
- 使用 HTTP 连接池复用连接
- 超时可配置（不同后端不同值）
- 流式转发（低延迟优先）

### 5. 错误处理
- 统一映射为 Anthropic 错误格式
- 对可重试错误（429、503）自动重试
- 不需要降级策略

## 测试策略

- **单元测试**：每个转换函数单独测试
- **集成测试**：Mock 后端 + 真实 API 两种
- **覆盖率**：关注核心逻辑覆盖
