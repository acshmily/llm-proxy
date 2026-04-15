# 防护机制文档

本文档详细说明 llm-proxy 的防护机制，用于避免 GFW 和网络行为特征扫描。

## 防护原理

GFW 和检测系统主要通过以下方式识别代理流量：

1. **TLS 指纹识别** - 通过 JA3/JA4 指纹识别客户端库（如 Go 的默认 TLS 实现）
2. **HTTP 头部分析** - 检测缺少浏览器特征头部（User-Agent、Accept 等）
3. **行为模式分析** - 识别机器流量的规律性（固定延迟、连接复用模式）
4. **流量特征分析** - 识别固定大小的请求包、无填充数据

本代理提供三层防护机制应对上述检测：

```
┌─────────────────────────────────────────────────────────┐
│                    检测方视角                            │
├─────────────────────────────────────────────────────────┤
│  TLS 指纹 → 模拟浏览器 TLS 握手                            │
│  HTTP 头部 → 添加完整浏览器头部                           │
│  请求延迟 → 随机抖动 50-200ms                            │
│  连接复用 → 70% 复用率，模拟真实行为                      │
│  请求大小 → 随机填充 10-100 字节                          │
└─────────────────────────────────────────────────────────┘
```

## 第一层：流量伪装（Traffic Camouflage）

### TLS 指纹模拟

**作用：** 修改 TLS 握手时的 ClientHello，模拟浏览器的 TLS 指纹，避免被 JA3/JA4 指纹识别。

**配置：**
```yaml
protection:
  traffic_camouflage:
    tls_fingerprint:
      enabled: true
      mode: chrome  # chrome, firefox, safari, random
```

**模式说明：**
- `chrome` - 模拟 Chrome 120 的 TLS 指纹
- `firefox` - 模拟 Firefox 121 的 TLS 指纹
- `safari` - 模拟 Safari 17 的 TLS 指纹
- `random` - 每次请求随机选择一种浏览器指纹

**注意：** TLS 指纹模拟需要 `utls` 库支持。如果使用 Nginx 反向代理，应在 Nginx 层配置 TLS，代理本身可以关闭此功能。

### 浏览器头部模拟

**作用：** 添加完整的浏览器 HTTP 请求头部，使流量特征与真实浏览器一致。

**配置：**
```yaml
protection:
  traffic_camouflage:
    browser_headers:
      enabled: true
      mode: random  # random, chrome, firefox, safari
      custom_user_agent: ""  # 可选：自定义 User-Agent
```

**模拟的头部包括：**
| 头部 | 说明 |
|------|------|
| `User-Agent` | 浏览器标识 |
| `Accept` | 可接受的内容类型 |
| `Accept-Language` | 语言偏好 |
| `Accept-Encoding` | 压缩算法支持 |
| `Sec-Ch-Ua` | 客户端提示信息（Chrome） |
| `Sec-Ch-Ua-Mobile` | 是否移动端 |
| `Sec-Ch-Ua-Platform` | 操作系统 |

**浏览器头部示例（Chrome）：**
```
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8
Accept-Language: en-US,en;q=0.5
Accept-Encoding: gzip, deflate, br
Sec-Ch-Ua: "Not_A Brand";v="8", "Chromium";v="120"
Sec-Ch-Ua-Mobile: ?0
Sec-Ch-Ua-Platform: "macOS"
```

### 内容编码混淆

**作用：** 随机使用不同的内容编码算法，增加流量特征的多样性。

**配置：**
```yaml
protection:
  traffic_camouflage:
    content_encoding:
      enabled: false
      algorithms: ["gzip", "deflate", "br"]
```

## 第二层：行为打散（Behavior Jitter）

### 请求延迟抖动

**作用：** 在每次请求前添加随机延迟，避免规律的请求间隔被识别。

**配置：**
```yaml
protection:
  behavior_jitter:
    request_delay:
      enabled: true
      min_ms: 50
      max_ms: 200
      distribution: exponential  # uniform, exponential
```

**分布模式：**
- `uniform` - 均匀分布，延迟在 min-ms 范围内均匀随机
- `exponential` - 指数分布，延迟更倾向于接近最小值

**推荐值：**
- 低延迟场景：50-100ms
- 常规场景：50-200ms
- 高安全场景：100-500ms

### 连接复用打散

**作用：** 随机决定是否复用 HTTP 连接，避免连接模式过于规律。

**配置：**
```yaml
protection:
  behavior_jitter:
    connection_reuse:
      enabled: true
      reuse_rate: 0.7  # 0.0-1.0
```

**复用率说明：**
- `0.0` - 从不复用连接（每次请求都新建连接）
- `1.0` - 总是复用连接（默认 HTTP 行为）
- `0.7` - 70% 概率复用连接（推荐）

**推荐值：**
- 低延迟需求：0.8-0.9
- 平衡场景：0.6-0.7
- 高隐蔽性：0.3-0.5

### 请求体填充

**作用：** 在请求中添加随机大小的填充数据，改变请求包大小特征。

**配置：**
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

**填充模式：**
- `random` - 在 min_bytes 到 max_bytes 之间随机选择填充大小
- `fixed` - 使用固定的填充大小（fixed_size）

**填充实现：**
- 使用 crypto/rand 生成加密安全的随机字节
- 填充数据以 Base64 编码添加到请求中
- 服务端自动忽略填充数据

## 第三层：流量混淆（Traffic Obfuscation）

### WebSocket 隧道

**作用：** 将 HTTP 请求封装在 WebSocket 消息中传输，绕过基于 HTTP 特征的检测。

**配置：**
```yaml
protection:
  traffic_obfuscation:
    websocket_tunnel:
      enabled: false
      path: "/ws-tunnel"
      ping_interval_ms: 30000
```

**工作原理：**
1. 客户端建立 WebSocket 连接到指定路径
2. HTTP 请求被封装在 WebSocket 消息中发送
3. 服务端从 WebSocket 消息中解包 HTTP 请求
4. 响应同样通过 WebSocket 消息返回

**注意：** WebSocket 隧道需要客户端配合实现。

### 请求分片

**作用：** 将大请求分割成多个小片段发送，避免大包特征。

**配置：**
```yaml
protection:
  traffic_obfuscation:
    request_sharding:
      enabled: false
      max_chunk_size: 1024  # 字节
```

**工作原理：**
1. 当请求体超过 max_chunk_size 时自动分片
2. 每个分片作为独立请求发送
3. 服务端重组分片后处理

## 配置推荐

### 基础防护配置

适用于大多数场景，平衡安全性和性能：

```yaml
protection:
  enabled: true
  
  traffic_camouflage:
    browser_headers:
      enabled: true
      mode: random
  
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
```

### 高安全配置

适用于高检测风险场景：

```yaml
protection:
  enabled: true
  
  traffic_camouflage:
    tls_fingerprint:
      enabled: true
      mode: random
    browser_headers:
      enabled: true
      mode: random
  
  behavior_jitter:
    request_delay:
      enabled: true
      min_ms: 100
      max_ms: 500
      distribution: exponential
    connection_reuse:
      enabled: true
      reuse_rate: 0.5
    request_padding:
      enabled: true
      min_bytes: 50
      max_bytes: 500
      mode: random
  
  traffic_obfuscation:
    websocket_tunnel:
      enabled: true
      path: "/ws-tunnel"
      ping_interval_ms: 30000
    request_sharding:
      enabled: true
      max_chunk_size: 512
```

### Nginx 反向代理配置

当使用 Nginx 作为反向代理时，TLS 终止在 Nginx 层处理：

```yaml
protection:
  enabled: true
  
  traffic_camouflage:
    tls_fingerprint:
      enabled: false  # Nginx 处理 TLS
    browser_headers:
      enabled: true
      mode: random
  
  behavior_jitter:
    request_delay:
      enabled: true
      min_ms: 50
      max_ms: 200
    connection_reuse:
      enabled: true
      reuse_rate: 0.7
    request_padding:
      enabled: true
      min_bytes: 10
      max_bytes: 100
```

## 防护效果验证

### 检查浏览器头部

```bash
curl -v http://localhost:8080/v1/messages \
  -H "Authorization: Bearer sk-client-1" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-3","messages":[]}' 2>&1 | grep -E "User-Agent|Accept|Sec-"
```

### 检查请求延迟

在日志中查看请求处理时间，应该观察到 50-200ms 的随机延迟。

### 检查连接复用

通过抓包工具（如 Wireshark）观察 TCP 连接模式，应该看到部分连接被复用。

## 中间件实现

防护中间件位于 `internal/middleware/protection.go`，提供以下公开方法：

| 方法 | 说明 |
|------|------|
| `IsEnabled()` | 检查防护是否启用 |
| `ApplyBrowserHeaders(req *http.Header)` | 应用浏览器头部 |
| `GetRequestDelay()` int | 获取请求延迟（毫秒） |
| `ShouldReuseConnection()` bool | 是否复用连接 |
| `GetPaddingSize()` int | 获取填充大小 |
| `GeneratePadding(size int) []byte` | 生成填充数据 |
| `GetBrowserUserAgent()` string | 获取浏览器 UA |

所有方法都是并发安全的，可在多个 goroutine 中调用。

## 注意事项

1. **性能影响** - 防护机制会引入额外延迟（50-500ms）和少量 CPU 开销
2. **日志安全** - 建议关闭 `log_jitter_applied` 避免防护参数暴露在日志中
3. **Nginx 部署** - 使用 Nginx 时关闭 TLS 指纹模拟，在 Nginx 层配置
4. **WebSocket 隧道** - 需要客户端配合实现，目前仅支持服务端配置

## 故障排查

**问题：防护未生效**

检查配置：
```yaml
protection:
  enabled: true  # 确保总开关开启
```

**问题：请求失败**

检查防护配置范围：
- `reuse_rate` 必须在 0.0-1.0 之间
- `min_ms` 必须 <= `max_ms`
- `min_bytes` 必须 <= `max_bytes`

**问题：死锁**

早期版本存在死锁问题（在持有锁时调用 `IsEnabled()`），已在最新版本修复。确保使用最新代码。
