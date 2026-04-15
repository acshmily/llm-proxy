# WebSocket 隧道客户端设计文档

> **日期：** 2026-04-15  
> **状态：** 已完成  
> **类型：** 本地 HTTP 代理服务器

---

## 目标

实现一个本地 HTTP 代理服务器，使应用程序无需修改代码即可通过 WebSocket 隧道与服务端通信。配置代理后，所有 HTTP 请求自动封装到 WebSocket 消息中发送到服务端。

---

## 架构设计

```
┌──────────────┐     HTTP      ┌─────────────────────────────────┐
│  应用程序     │ ───────────→ │  WSTunnel Client                │
│  (任意语言)   │  proxy :8081  │  本地 HTTP 代理服务器              │
└──────────────┘               └─────────────────────────────────┘
                                      │
                                      │ WebSocket 消息封装
                                      ↓
                               ┌─────────────────────────────────┐
                               │  WebSocket Tunnel               │
                               │  单长连接 + 心跳保持               │
                               └─────────────────────────────────┘
                                      │
                                      │ ws://server:8080/ws-tunnel
                                      ↓
┌──────────────┐               ┌─────────────────────────────────┐
│  服务端代理   │ ←──────────── │  LLM Proxy Server               │
│  (已启用 WS)  │   WebSocket   │  (处理 /ws-tunnel 端点)           │
└──────────────┘               └─────────────────────────────────┘
```

**设计决策：**
- **单连接架构** - 维护单一 WebSocket 长连接，简单可靠，低延迟
- **串行处理** - 请求按序通过单一连接发送，避免并发竞争
- **断线重连** - 指数退避策略自动重连，故障恢复

---

## 目录结构

```
cmd/ws-client/
  main.go              # 程序入口，信号处理
  config.go            # 配置结构 + 加载逻辑
  
internal/wsclient/
  proxy.go             # HTTP 代理服务器（Listener + Handler）
  tunnel.go            # WebSocket 隧道管理（连接 + 重连 + 心跳）
  protocol.go          # 消息协议（封装/解封装）
  
configs/
  client-config.example.yaml  # 配置示例文件
  
docs/
  ws-client-guide.md   # 客户端使用文档
```

**设计决策：**
- `cmd/ws-client/` - 与服务端 `cmd/proxy/` 保持一致
- `internal/wsclient/` - 核心逻辑封装，可复用
- `configs/` - 配置示例独立存放

---

## 配置设计

### 配置文件 (`client-config.yaml`)

```yaml
server:
  address: "ws://localhost:8080/ws-tunnel"  # 服务端 WebSocket 地址
  ping_interval_ms: 30000                    # 心跳间隔

listen:
  address: ":8081"                           # 本地监听地址

logging:
  format: "json"                             # json 或 text
  level: "info"                              # debug, info, warn, error

health:
  enabled: true
  address: ":8082"                           # 健康检查端点监听
```

### 命令行参数

```bash
ws-client --config client-config.yaml
ws-client --server ws://localhost:8080/ws-tunnel --listen :8081
ws-client --server ws://...  # 命令行覆盖配置文件
```

### 环境变量

```bash
export WS_TUNNEL_SERVER="ws://localhost:8080/ws-tunnel"
export WS_TUNNEL_LISTEN=":8081"
```

### 配置优先级

1. 命令行参数（最高）
2. 环境变量
3. 配置文件
4. 默认值（最低）

---

## 核心组件

### 4.1 HTTP 代理服务器 (`proxy.go`)

**职责：**
- 监听本地端口（默认 8081）
- 接收 HTTP 请求，转发到 WebSocket 隧道
- 将 WebSocket 响应写回 HTTP 客户端
- 支持 HTTP/1.1，Keep-Alive

**接口：**
```go
type ProxyServer struct {
    tunnel *Tunnel
    listen string
}

func NewProxyServer(tunnel *Tunnel, listen string) *ProxyServer
func (p *ProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request)
func (p *ProxyServer) Start() error
```

**设计决策：**
- 实现 `http.Handler` 接口，便于测试
- 使用 `httputil.ReverseProxy` 风格，但自定义传输逻辑

---

### 4.2 WebSocket 隧道管理 (`tunnel.go`)

**职责：**
- 建立并维护单一 WebSocket 长连接
- 心跳循环（可配置间隔）
- 断线自动重连（指数退避）
- 并发安全（互斥锁保护连接）

**接口：**
```go
type Tunnel struct {
    mu           sync.Mutex
    conn         *websocket.Conn
    server       string
    pingInterval time.Duration
    done         chan struct{}
}

func NewTunnel(server string, pingInterval time.Duration) *Tunnel
func (t *Tunnel) Connect() error
func (t *Tunnel) SendRequest(req *http.Request) (*http.Response, error)
func (t *Tunnel) Close() error
func (t *Tunnel) IsConnected() bool
```

**重连策略：**
- 初始延迟：1 秒
- 最大延迟：30 秒
- 增长因子：2x（指数退避）

**设计决策：**
- 单一连接简化并发控制
- 互斥锁保护连接访问
- 重连后台进行，不阻塞调用方

---

### 4.3 消息协议 (`protocol.go`)

**请求封装（HTTP → WebSocket）：**
```json
{
  "type": "request",
  "data": {
    "method": "POST",
    "path": "/v1/messages",
    "headers": {
      "Content-Type": "application/json"
    },
    "body": "base64-encoded-body"
  }
}
```

**响应解封装（WebSocket → HTTP）：**
```json
{
  "type": "response",
  "data": {
    "status": 200,
    "headers": {
      "Content-Type": "application/json"
    },
    "body": "base64-encoded-response-body"
  }
}
```

**接口：**
```go
type WSRequest struct {
    Type   string     `json:"type"`
    Data   WSReqData  `json:"data"`
}

type WSReqData struct {
    Method  string            `json:"method"`
    Path    string            `json:"path"`
    Headers map[string]string `json:"headers"`
    Body    string            `json:"body,omitempty"`
}

type WSResponse struct {
    Type   string      `json:"type"`
    Data   WSRespData  `json:"data"`
}

type WSRespData struct {
    Status  int               `json:"status"`
    Headers map[string]string `json:"headers"`
    Body    string            `json:"body,omitempty"`
}

func EncodeRequest(req *http.Request) ([]byte, error)
func DecodeResponse(data []byte) (*http.Response, error)
```

**设计决策：**
- Base64 编码保证二进制安全
- 头部扁平化（多值头部取第一个）
- 错误响应使用 `type: "error"`

---

## 数据流

```
HTTP 请求 → 读取 Body → 封装 JSON → WebSocket Send → 等待响应
                                                      ↓
HTTP 响应 ← 构建 Response ← 解析 JSON ← WebSocket Recv ←
```

**详细流程：**

1. **请求到达**
   - `ProxyServer.ServeHTTP()` 接收 HTTP 请求
   - 读取请求体到内存

2. **封装发送**
   - `EncodeRequest()` 构建 WebSocket JSON 消息
   - `Tunnel.SendRequest()` 获取连接并发送

3. **等待响应**
   - 同步读取 WebSocket 响应（阻塞）
   - 超时时间：60 秒（可配置）

4. **解封装返回**
   - `DecodeResponse()` 解析 JSON
   - 构建 `http.Response` 写回客户端

---

## 错误处理

| 场景 | HTTP 响应 | 说明 |
|------|-----------|------|
| WebSocket 未连接 | 503 Service Unavailable | 正在重连或初始化中 |
| 发送失败 | 502 Bad Gateway | WebSocket 写入错误 |
| 响应解析失败 | 502 Bad Gateway | JSON 解析或 Base64 解码失败 |
| 请求超时 | 504 Gateway Timeout | 超过 60 秒未收到响应 |
| 服务端错误 | 5xx | 服务端返回的错误状态码 |

**重试策略：**
- 客户端不自动重试 HTTP 请求
- 由上游应用程序决定重试
- 避免放大效应

---

## 日志设计

**JSON 格式（生产环境）：**
```json
{"time":"2026-04-15T12:00:00Z","level":"info","event":"request_start","method":"POST","path":"/v1/messages","client_ip":"127.0.0.1"}
{"time":"2026-04-15T12:00:00Z","level":"info","event":"tunnel_send","msg_id":"abc123"}
{"time":"2026-04-15T12:00:01Z","level":"info","event":"tunnel_recv","status":200,"latency_ms":125}
```

**文本格式（开发环境）：**
```
INFO request_start POST /v1/messages from 127.0.0.1
INFO tunnel_send msg_id=abc123
INFO tunnel_recv status=200 latency_ms=125
```

**日志级别：**
- `debug` - 详细调试信息（消息内容）
- `info` - 请求/响应摘要
- `warn` - 可恢复错误（重连、超时）
- `error` - 不可恢复错误

---

## 健康检查

**端点：** `GET /health`（独立端口 8082）

**响应格式：**
```json
{
  "status": "connected",
  "server": "ws://localhost:8080/ws-tunnel",
  "uptime_seconds": 3600,
  "requests_total": 1234
}
```

**状态值：**
- `connected` - WebSocket 已连接
- `disconnected` - WebSocket 未连接
- `connecting` - 正在重连中

**设计决策：**
- 独立端口避免与代理端口冲突
- 健康检查不依赖 WebSocket 连接
- 包含运行统计便于监控

---

## 测试设计

### 单元测试

| 测试项 | 文件 | 说明 |
|--------|------|------|
| `TestEncodeRequest` | protocol_test.go | 请求封装正确性 |
| `TestDecodeResponse` | protocol_test.go | 响应解封装正确性 |
| `TestRoundTrip` | protocol_test.go | 往返一致性 |
| `TestTunnel_IsConnected` | tunnel_test.go | 连接状态 |
| `TestProxyServer_ServeHTTP` | proxy_test.go | 代理服务器 |

### 集成测试

- Mock WebSocket 服务端
- 完整请求/响应流程
- 并发请求测试
- 断线重连测试

### 端到端测试

- 真实服务端 + 客户端
- 使用 `httptest.Server` 模拟后端
- 验证完整链路

---

## 成功标准

- ✅ **A**: 应用程序配置代理 `http://localhost:8081` 后，请求自动通过 WebSocket 隧道到达服务端，无需修改应用代码
- ✅ **B**: 客户端显示请求/响应日志，便于调试
- ✅ **C**: 支持并发请求（多个请求同时通过 WebSocket 隧道，串行处理）
- ✅ **D**: 断线自动重连（指数退避策略）

---

## YAGNI 裁剪（本次不做）

- ❌ 连接池（单连接足够，复杂度不匹配）
- ❌ 请求优先级（无此需求）
- ❌ 指标导出（Prometheus，后续可扩展）
- ❌ Web 管理界面（日志 + 健康检查足够）
- ❌ 请求缓存（无状态代理）

---

## 实现计划预览

1. **消息协议** - 封装/解封装（无依赖，可独立测试）
2. **WebSocket 隧道** - 连接管理、重连、心跳
3. **HTTP 代理服务器** - Listener、Handler
4. **配置管理** - 配置文件、命令行、环境变量
5. **健康检查** - 独立端点、状态报告
6. **日志系统** - JSON/Text 格式、级别控制
7. **集成测试** - 端到端验证
8. **文档** - 使用指南、配置说明

---

## 参考文档

- 服务端实现：`internal/server/ws_tunnel.go`
- 服务端测试：`internal/server/ws_tunnel_test.go`
- 客户端示例：`internal/client-examples/ws_tunnel_client.go`
- 服务端文档：`docs/ws-tunnel-client-guide.md`
