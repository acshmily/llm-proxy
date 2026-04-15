# Changelog

所有重要的项目变更都将记录在此文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)，
项目遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [未发布]

### 新增
- WebSocket 隧道客户端实现 (#39)
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
