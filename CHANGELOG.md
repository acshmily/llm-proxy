# Changelog

所有重要的项目变更都将记录在此文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，
项目遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [0.7.9] - 2026-04-22

### 新增
- **后端请求/响应调试日志**: 开启 `debug_requests` 后，记录发送到后端的实际请求体和从后端接收的响应体
  - 使用 `debug_max_body` 控制日志最大字节数（默认 1024）
  - 方便排查 400 等错误时的实际 payload

---

## [0.7.8] - 2026-04-22

### 修复
- **tool 角色映射**: `tool` 角色未映射为 `user`，Gemini API 不识别 `tool` 角色
  - 根据官方文档，函数响应必须使用 `user` 角色
  - 此修复是 400 错误的另一层原因

---

## [0.7.7] - 2026-04-22

### 修复
- **functionResponse 格式**: `tool` 角色返回的 `functionResponse.response` 字段格式
  不符合 Gemini API 规范
  - 之前: `{"name": "func_name", "content": "{\"data\": ...}"}`
  - 现在: 直接解析为 JSON 对象 `{"data": ...}`，或包装为 `{"output": "..."}`
  - Gemini API 要求 `response` 字段必须是 `google.protobuf.Struct`（JSON 对象）

---

## [0.7.6] - 2026-04-22

### 修复
- **OpenAI→Gemini 角色交替违规**: 当 `system`/`developer` 角色映射为 `user` 时，
  连续的同角色消息导致 Gemini API 返回 400 错误
  - 添加 `mergeConsecutiveSameRoles()` 在转换前合并连续相同 Gemini 角色的消息
  - OpenClaw embedded agent 发送 `system` + `user` 开头消息时触发此问题

---

## [0.7.5] - 2026-04-22

### 修复
- **Completions 端点工具调用**: `/v1/completions` 未解析 `tools`、`tool_calls`、`tool_call_id` 字段
  - OpenClaw 使用 completions 端点 + `messages` 格式发送工具调用请求
  - 之前 `tools` 被忽略导致 Gemini 返回 400 错误
  - 现在正确解析工具定义和工具调用历史，与 `/v1/chat/completions` 行为一致

---

## [0.7.4] - 2026-04-22

### 修复
- **Gemini 原生端点认证**: 后端请求遗漏附加 API Key，导致 403 PERMISSION_DENIED
  - 将 `route.BackendKey` 作为 `?key=` 查询参数附加到后端 URL
  - 兼容客户端已有查询参数时用 `&key=` 拼接

---

## [0.7.3] - 2026-04-22

### 修复
- **Gemini 原生端点安全**: Header 转发策略从黑名单改为白名单，防止敏感信息泄露
  - 只转发 `Content-Type`, `User-Agent`, `Accept`, `Accept-Encoding`, `Accept-Language`
  - 不再转发 `Cookie`, `Connection`, `Transfer-Encoding` 等跳步 headers
- **Gemini 原生端点**: BaseURL 尾部斜杠导致的路径拼接错误

---

## [0.7.2] - 2026-04-22

### 新增
- **Gemini 原生端点**: `/v1beta/models/:model` 支持 Gemini 协议客户端直接访问
  - 透传请求体到 Gemini 后端，无需协议转换
  - 支持流式 SSE 响应直接转发
  - 自动剥离客户端认证 headers（Authorization / X-Api-Key）
  - 仅路由到 Gemini 后端，其他后端返回 400

### 修复
- **工具参数 `strict` 字段**: 自动从工具定义中剥离 `strict` 字段，防止 OpenClaw 工具调用返回空响应

---

## [0.7.1] - 2026-04-22

### 修复
- **Gemini 工具定义兼容性**: 清理工具参数中 Gemini 不支持的 JSON Schema 字段
  - 自动剥离 `additionalProperties`、`$ref`、`$schema`、`$id`、`default`、`deprecated`、`patternProperties`、`propertyNames`
  - 递归处理嵌套 properties 和 `anyOf`/`oneOf` 数组中的 schema
  - 修复 nil Parameters 导致输出 `null` 的问题
  - 解决 OpenClaw 大量 MCP 工具定义触发 400 Bad Request 的问题

---

## [0.7.0] - 2026-04-22

### 新增
- **Gemini Function Calling 支持**: 完整支持 Gemini 工具调用协议
  - OpenAI `tools`/`tool_calls`/`tool` 角色自动映射到 Gemini `functionDeclarations`/`functionCall`/`functionResponse`
  - 支持多 tool_calls 同时发送（并行工具调用）
  - 支持 assistant 消息混合 text + tool_calls 内容
  - 流式响应中 functionCall 自动转换为 OpenAI `tool_calls` delta 格式
  - 非流式响应正确提取 functionCall 到 `ToolCalls` 字段
  - `tool_call_id` 自动解析为函数名（通过回溯消息历史）
- **SSE 转换器单元测试**: 覆盖文本、functionCall、finish_reason 等场景

### 改进
- ToolCall ID 使用唯一标识符（`call_<name>_<index>`），避免多工具调用时 ID 冲突

---

## [0.6.2] - 2026-04-22

### 新增
- **调试日志**: 可配置的请求/响应体调试日志
  - 通过配置项开启请求和响应体的日志输出
  - 支持限制日志长度，避免大请求体刷屏
  - 便于排查协议转换和兼容性问题

### 测试
- 新增 `extractContent` 辅助函数单元测试

---

## [0.6.1] - 2026-04-21

### 修复
- **兼容性**: `/v1/completions` 端点新增 `messages` 数组格式支持
  - OpenClaw `openai-completions` 模式发送 `messages` 数组而非 `prompt` 字段
  - 同时解析 `prompt` 和 `messages` 两种输入格式
  - `prompt` 存在时优先使用，否则使用 `messages` 数组
- **流式响应**: Gemini 后端流式请求返回空数据
  - `streamGenerateContent` 默认返回 NDJSON 格式（无 `data:` 前缀）
  - 添加 `?alt=sse` 参数请求标准 SSE 格式
  - 修复 `/v1/messages`、`/v1/completions` 和 `/v1/chat/completions` 三个端点
  - 解决 OpenClaw 默认流式调用无数据的问题
- **健壮性**: 空输入校验
  - 当 `prompt` 和 `messages` 均为空时返回 400 错误（此前透传后端 502）
  - 更新 `EmptyPrompt` 测试预期行为

### 测试
- 新增 7 个测试用例覆盖双格式解析、优先级、输入校验和流式场景

---

## [0.6.0] - 2026-04-21

### 新增
- **OpenAI Completions API 支持**
  - 新增 `POST /v1/completions` 端点
  - 兼容旧版 OpenAI Completions 格式（`prompt` 字段替代 `messages`）
  - 响应转换为 `choices[].text` 格式（非 `choices[].message.content`）
  - 支持 OpenAI/Anthropic/Gemini 三种后端
  - 支持流式响应（SSE 格式转换）

---

## [0.5.3] - 2026-04-21

### 修复
- **健壮性**: `/v1/models` 端点 JSON 解析失败时静默丢弃结果
  - 增加错误日志记录便于排查
- **一致性**: `owned_by` 字段统一使用后端类型名，不再依赖后端响应

---

## [0.5.2] - 2026-04-21

### 修复
- **兼容性**: `/v1/models` 端点查询 OpenAI/Anthropic 后端时返回 401
  - OpenAI 后端添加 `Authorization: Bearer` Header
  - Anthropic 后端添加 `x-api-key` Header
  - 新增认证 Header 测试用例

---

## [0.5.1] - 2026-04-21

### 修复
- **兼容性**: OpenAI `system`/`developer`/`tool` 角色不被 Gemini 支持
  - 新增 `mapToGeminiRole` 函数自动映射角色
  - `system`/`developer`/`tool` → `user`
  - `assistant` → `model`
  - 解决 OpenClaw 发送 system prompt 时返回 "Role 'system' is not supported" 错误

---

## [0.5.0] - 2026-04-21

### 新增
- **模型列表端点**
  - 新增 `GET /v1/models` 端点（OpenAI 兼容格式）
  - 并发查询所有已配置后端（OpenAI/Anthropic/Gemini）
  - 合并返回统一模型列表，部分后端不可达时不影响其他结果
  - OpenClaw 可通过此端点正常获取可用模型

---

## [0.4.2] - 2026-04-21

### 修复
- **缺陷**: Gemini `ParseResponse` 硬编码模型名为 `"gemini-pro"`
  - 客户端请求 `gemini-2.5-flash` 时返回错误模型名
  - 改为接收 `model` 参数透传，确保响应中的模型名与请求一致
- **内部接口**: `gemini.ParseResponse` 签名变更（`data []byte` → `data []byte, model string`）

---

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

### 从 v0.7.x 升级到 v0.7.5

**无破坏性变更。**

**修复内容：**
- Completions 端点工具调用支持（OpenClaw embedded agent 400 错误修复）
- Gemini 原生端点后端认证（403 错误修复）
- Header 转发安全加固（白名单策略）
- BaseURL 尾部斜杠路径拼接修复

---

### 从 v0.7.1 升级到 v0.7.2

**无破坏性变更。**

**新增功能：**
- `/v1beta/models/:model` Gemini 原生端点
  - 支持 `generateContent`、`streamGenerateContent`、`countTokens` 等完整 Gemini API
  - 请求/响应直接透传，不做协议转换

**修复：**
- 工具参数 `strict` 字段导致 OpenClaw 工具调用空响应

**升级步骤：**
1. 停止旧版本服务
2. 部署 v0.7.2 二进制文件或 Docker 镜像
3. 重启服务（`config.yaml` 无需修改）

---

### 从 v0.7.0 升级到 v0.7.1

**无破坏性变更。**

**修复：**
- Gemini 工具定义兼容性：自动清理不支持的 JSON Schema 字段
  - 剥离 `additionalProperties`、`$ref`、`$schema`、`$id`、`default`、`deprecated`、`patternProperties`、`propertyNames`
  - 解决 OpenClaw 大量 MCP 工具定义触发 400 的问题

**升级步骤：**
1. 停止旧版本服务
2. 部署 v0.7.1 二进制文件或 Docker 镜像
3. 重启服务（`config.yaml` 无需修改）

---

### 从 v0.6.x 升级到 v0.7.0

**无破坏性变更。**

**新增功能：**
- **Gemini Function Calling 支持**
  - OpenAI `tools`/`tool_calls`/`tool` 角色自动映射到 Gemini `functionDeclarations`/`functionCall`/`functionResponse`
  - 支持多 tool_calls 同时发送（并行工具调用）
  - 流式响应中 functionCall 自动转换为 OpenAI `tool_calls` delta 格式

**升级步骤：**
1. 停止旧版本服务
2. 部署 v0.7.0 二进制文件或 Docker 镜像
3. 重启服务（`config.yaml` 无需修改）

---

### 从 v0.6.x 升级到 v0.6.2

**新增功能：**
- 可配置的请求/响应体调试日志，便于排查问题

**升级步骤：**
1. 部署新版本
2. （可选）在 `config.yaml` 中添加调试日志配置

---

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
