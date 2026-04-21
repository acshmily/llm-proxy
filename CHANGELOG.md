# Changelog

所有重要的项目变更都将记录在此文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，
项目遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [0.4.1] - 2026-04-21

### 修复
- **安全**: SSE 流式转换中的 JSON 注入漏洞
  - 使用 `json.Marshal` 替代 `fmt.Sprintf` 构造 SSE 响应
  - 防止 AI 输出中的引号、换行等字符破坏 JSON 格式
- **安全**: 协议转换错误被静默丢弃
  - 转换失败时返回客户端明确的错误响应并记录日志
- **兼容**: 使用 `http.Request.Context()` 替代已弃用的 `http.CloseNotifier`
- **健壮性**: 未映射 HTTP 状态码产生空错误字段的问题

---

## [0.4.0] - 2026-04-16

### 新增
- **OpenAI 协议支持**
  - 新增 `/v1/chat/completions` 端点，支持 OpenAI Chat Completion API
  - 支持流式和非流式响应
  - 智能协议转换：OpenAI 请求 → OpenAI/Anthropic/Gemini 后端

- **协议转换器**
  - `internal/protocol/openai` 模块实现
  - OpenAI 请求解析 (`ParseRequest`)
  - OpenAI 响应构建 (`BuildResponse`)
  - `finish_reason` 映射支持（stop/length/content_filter）

- **双协议架构**
  - 保留原有 Anthropic 协议 (`/v1/messages`)
  - 新增 OpenAI 协议 (`/v1/chat/completions`)
  - 两种协议可混合使用，路由配置无需修改

### 改进
- **finish_reason 映射**
  - Anthropic: `end_turn`/`stop_sequence` → `stop`, `max_tokens` → `length`
  - Gemini: `STOP` → `stop`, `MAX_TOKENS` → `length`, `SAFETY`/`RECITATION` → `content_filter`
  - OpenAI: 直接使用后端返回的标准值

- **错误处理**
  - 统一错误格式为 OpenAI 兼容结构
  - 后端错误消息提取和转换

- **客户端兼容性**
  - 支持 OpenAI SDK (Python/Node.js/Go 等)
  - 支持 Anthropic SDK
  - 支持任意 OpenAI 兼容客户端

### 测试
- 6 个 OpenAI 协议集成测试（OpenAI/Anthropic/Gemini 后端）
- 9 个协议转换器单元测试
- 流式响应协议转换测试

### 文档
- README.md 添加 OpenAI 协议使用示例
- config.example.yaml 添加协议说明
- OpenAI SDK 调用示例（Python/curl）

---

## [0.3.2] - 2026-04-16

### 修复
- **重要**: Gemini API 认证方式错误（401 UNAUTHENTICATED）
  - Gemini API 使用 URL 参数 `?key=` 而非 `Authorization: Bearer` Header
  - 修复后 Gemini 后端可正常使用

### 文档
- 添加 Gemini API Key 格式说明（AIzaSyD-xxx，无需 Bearer 前缀）
- 添加 Gemini API Key 获取链接：https://makersuite.google.com/app/apikey

---

## [0.3.1] - 2026-04-15

### 修复
- 文档：修正 ws-client 使用示例，强调直接访问模式（无需配置代理）

### 说明
- v0.3.0 的文档修复版本，功能无变更

---

## [0.3.0] - 2026-04-15

### 新增
- WebSocket 隧道客户端实现
  - 本地 HTTP 代理服务器，无需修改应用代码即可通过 WebSocket 隧道通信
  - 单连接架构，维护单一 WebSocket 长连接
  - 指数退避自动重连策略
  - 心跳保持机制（可配置间隔）
  - 健康检查端点（独立端口）
  - JSON/Text 可配置日志格式
- 协议转换器单元测试
  - OpenAI/Gemini/Claude 三个模块的完整测试覆盖
  - 边界情况测试（空数组、无效 JSON 等）
  - Round-trip 往返测试
- 文档完善
  - 贡献指南（CONTRIBUTING.md）
  - 变更日志（CHANGELOG.md）
  - 代码审查指南（.github/CODE_REVIEW.md）
  - Pull Request 模板

### 修复
- OpenAI `ParseResponse` panic 风险 - 当 `choices` 为空数组时
- 协议转换器缺少无效 JSON 输入的错误处理测试

### 改进
- GitHub Actions CI 流程
  - Go 版本矩阵测试 (1.21, 1.22)
  - 代码覆盖率报告（Codecov）
  - 多平台构建测试（Linux, macOS, Windows）
  - Docker 多架构镜像自动构建和推送

---

## [0.2.0] - 2026-04-15

### 新增
- 流量混淆功能
  - WebSocket 隧道服务端实现
  - 请求分片（Request Sharding）
  - 配置化开关控制
- 防护机制配置
  - 流量伪装（TLS 指纹、浏览器头部）
  - 行为打散（请求延迟、连接复用、请求填充）
  - 流量混淆（WebSocket 隧道、请求分片）

### 文档
- WebSocket 隧道客户端设计文档
- WebSocket 隧道使用指南
- 防护机制详细说明文档

---

## [0.1.0] - 2026-04-15

### 新增
- 初始版本
- Anthropic 协议代理服务
  - 支持 OpenAI 后端转换
  - 支持 Anthropic 后端转换
  - 支持 Gemini 后端转换
- 路由表认证机制
- 流式响应转发（SSE）
- HTTP 连接池复用
- 结构化日志（JSON 格式）
- 自动重试机制（429/503/504）
- 健康检查端点 `/health`
- Docker 多架构镜像（AMD64 + ARM64）

### 支持的平台
- Linux (AMD64, ARM64)
- macOS (AMD64, ARM64)
- Windows (AMD64)

### 支持的 API 端点
- `POST /v1/messages` - 聊天完成（支持流式）
- `GET /v1/models` - 获取模型列表
- `POST /v1/messages/count_tokens` - Token 计数

---

## 版本说明

### 语义化版本格式
- **MAJOR.MINOR.PATCH** (主版本号。次版本号。修订号)
- **MAJOR** - 不兼容的 API 变更
- **MINOR** - 向后兼容的功能新增
- **PATCH** - 向后兼容的问题修复

### 发布类型
- **未发布** - 已合并但未发布的功能
- ** vX.Y.Z** - 已发布的正式版本

---

## 升级指南

### 从 v0.3.x 升级到 v0.4.0

**无破坏性变更** - 配置完全向后兼容。

**新增功能：**
- `/v1/chat/completions` 端点（OpenAI 协议）
- 支持 OpenAI SDK、Anthropic SDK 任意调用
- 智能协议转换（OpenAI → OpenAI/Anthropic/Gemini 后端）
- `finish_reason` 映射支持

**升级步骤：**
1. 停止旧版本服务
2. 部署 v0.4.0 二进制文件或 Docker 镜像
3. 重启服务（`config.yaml` 无需修改）

**使用新协议（可选）：**

```python
# Python OpenAI SDK
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8848",  # 代理地址
    api_key="sk-client-1"              # 配置的客户端 Key
)

response = client.chat.completions.create(
    model="claude-3-opus",
    messages=[{"role": "user", "content": "Hello!"}]
)
```

```bash
# curl 调用
curl http://localhost:8848/v1/chat/completions \
  -H "Authorization: Bearer sk-client-1" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-3-opus","messages":[{"role":"user","content":"Hello!"}]}'
```

---

### 从 v0.2.x 升级到 v0.3.0

**新增功能：**
- WebSocket 隧道客户端 (`ws-client`)
- 协议转换器完整测试覆盖（OpenAI/Gemini/Claude）
- GitHub Actions CI 改进（Go 版本矩阵、Codecov）

**升级步骤：**
1. 下载 v0.3.0 二进制文件（包含 `proxy` 和 `ws-client`）
2. 部署新版本
3. （可选）如需使用 WebSocket 隧道，启动 `ws-client`：

```bash
./ws-client --server ws://your-server:8080/ws-tunnel --listen :8081

# 将应用代理地址指向 ws-client
export HTTP_PROXY=http://localhost:8081
```

---

### 从 v0.1.x 升级到 v0.2.0

**新增配置项：**

```yaml
# 在 config.yaml 中添加以下配置
protection:
  enabled: true
  
  # WebSocket 隧道（可选）
  traffic_obfuscation:
    websocket_tunnel:
      enabled: false  # 启用时需要运行 ws-client
    request_sharding:
      enabled: false
```

**客户端升级（如需 WebSocket 隧道）：**

```bash
# 构建 WebSocket 隧道客户端
go build -o ws-client ./cmd/ws-client

# 运行客户端
./ws-client --server ws://your-server:8080/ws-tunnel --listen :8081
```

**应用配置变更：**

将应用的代理地址修改为本地 ws-client 监听地址：
```
HTTP_PROXY=http://localhost:8081
```

---

## 开发记录

详细的开发和实现计划请查看：
- [设计文档](docs/superpowers/specs/)
- [实现计划](docs/superpowers/plans/)
