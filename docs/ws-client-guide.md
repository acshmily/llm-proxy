# WebSocket 隧道客户端使用指南

## 简介

WebSocket 隧道客户端是一个本地 HTTP 代理服务器，使应用程序无需修改代码即可通过 WebSocket 隧道与服务端通信。配置代理后，所有 HTTP 请求自动封装到 WebSocket 消息中发送到服务端。

## 安装方法

### 从源码编译

```bash
cd /Users/r2d2/Documents/claude-projetc/proxy-gemini-go
go build -o ws-client ./cmd/ws-client
```

编译后的二进制文件 `ws-client` 即为客户端程序。

## 快速开始

### 1. 配置服务端

确保服务端已启用 WebSocket 隧道功能，在 `config.yaml` 中配置：

```yaml
protection:
  enabled: true
  traffic_obfuscation:
    websocket_tunnel:
      enabled: true
      path: "/ws-tunnel"
      ping_interval_ms: 30000
```

启动服务端：
```bash
./proxy --config config.yaml
```

### 2. 启动客户端

复制配置示例文件并修改：

```bash
cp configs/client-config.example.yaml configs/client-config.yaml
```

编辑 `configs/client-config.yaml`，修改服务端地址：

```yaml
server:
  address: "ws://your-server:8080/ws-tunnel"
  ping_interval_ms: 30000

listen:
  address: ":8081"

logging:
  format: "json"
  level: "info"

health:
  enabled: true
  address: ":8082"
```

启动客户端：

```bash
./ws-client --config configs/client-config.yaml
```

### 3. 配置应用程序代理

在应用程序中配置 HTTP 代理为 `http://localhost:8081`：

**Python 示例：**
```python
import requests

proxies = {
    "http": "http://localhost:8081",
    "https": "http://localhost:8081",
}

response = requests.get("https://api.openai.com/v1/models", proxies=proxies)
```

**cURL 示例：**
```bash
curl -x http://localhost:8081 https://api.openai.com/v1/models
```

### 4. 验证连接

检查健康状态：

```bash
curl http://localhost:8082/health
```

响应示例：
```json
{
  "status": "connected",
  "server": "ws://localhost:8080/ws-tunnel",
  "uptime_seconds": 3600,
  "requests_total": 1234
}
```

## 配置选项

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `server.address` | string | - | 服务端 WebSocket 地址（必填） |
| `server.ping_interval_ms` | int | 30000 | 心跳间隔（毫秒） |
| `listen.address` | string | :8081 | 本地 HTTP 代理监听地址 |
| `logging.format` | string | json | 日志格式：`json` 或 `text` |
| `logging.level` | string | info | 日志级别：`debug`, `info`, `warn`, `error` |
| `health.enabled` | bool | true | 是否启用健康检查端点 |
| `health.address` | string | :8082 | 健康检查端点监听地址 |

## 命令行参数

| 参数 | 说明 |
|------|------|
| `--config <path>` | 指定配置文件路径 |
| `--server <url>` | 覆盖配置文件中的服务端地址 |
| `--listen <addr>` | 覆盖配置文件中的监听地址 |

## 环境变量

| 变量名 | 说明 |
|--------|------|
| `WS_TUNNEL_SERVER` | 服务端 WebSocket 地址 |
| `WS_TUNNEL_LISTEN` | 本地监听地址 |

**配置优先级：** 命令行参数 > 环境变量 > 配置文件 > 默认值

## 健康检查

健康检查端点提供以下信息：

**请求：**
```bash
GET http://localhost:8082/health
```

**响应：**
```json
{
  "status": "connected",
  "server": "ws://localhost:8080/ws-tunnel",
  "uptime_seconds": 3600,
  "requests_total": 1234
}
```

**状态值说明：**
- `connected` - WebSocket 已连接
- `disconnected` - WebSocket 未连接
- `connecting` - 正在重连中

## 日志示例

### JSON 格式（生产环境）

```json
{"time":"2026-04-15T12:00:00Z","level":"info","event":"request_start","method":"POST","path":"/v1/messages","client_ip":"127.0.0.1"}
{"time":"2026-04-15T12:00:00Z","level":"info","event":"tunnel_send","msg_id":"abc123"}
{"time":"2026-04-15T12:00:01Z","level":"info","event":"tunnel_recv","status":200,"latency_ms":125}
```

### 文本格式（开发环境）

配置文件中设置 `format: "text"`：

```
INFO request_start POST /v1/messages from 127.0.0.1
INFO tunnel_send msg_id=abc123
INFO tunnel_recv status=200 latency_ms=125
```

## 故障排查

### 客户端无法连接服务端

**症状：** 健康检查返回 `disconnected` 状态

**检查步骤：**
1. 确认服务端已启动并监听正确端口
2. 检查服务端地址配置是否正确
3. 检查防火墙是否允许 WebSocket 连接
4. 查看客户端日志中的错误信息

### 请求通过客户端后超时

**症状：** 应用程序收到 504 Gateway Timeout

**可能原因：**
- 服务端处理超时（默认 60 秒）
- 网络延迟过高
- 服务端负载过高

**解决方法：**
1. 检查服务端日志
2. 增加服务端处理超时时间
3. 检查网络连接质量

### 客户端启动失败

**症状：** 客户端启动后立即退出

**检查步骤：**
1. 检查配置文件语法是否正确（YAML 格式）
2. 检查端口是否被占用（`lsof -i :8081`）
3. 查看启动错误日志

### 日志级别调整

遇到可疑问题时，可将日志级别调整为 `debug` 获取详细信息：

```yaml
logging:
  format: "text"
  level: "debug"
```

## 架构说明

```
应用程序 → HTTP 代理 (:8081) → WebSocket 隧道 → 服务端代理
```

- **HTTP 代理**：接收应用程序的 HTTP 请求
- **WebSocket 隧道**：将 HTTP 请求封装到 WebSocket 消息
- **服务端代理**：解封装并转发到真实后端 API

详细说明参考设计文档：`docs/superpowers/specs/ws-tunnel-client-design.md`
