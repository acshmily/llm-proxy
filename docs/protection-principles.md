# 防护机制原理

本文档详细说明 llm-proxy 防护机制的工作原理和技术实现。

## 一、威胁模型

### 1.1 GFW 和检测系统的识别方法

**1. TLS 指纹识别（JA3/JA4）**

TLS 握手时，客户端发送的 ClientHello 包含以下特征：
- TLS 版本支持列表
- 密码套件列表和顺序
- TLS 扩展列表和顺序
- 椭圆曲线参数

这些特征组合形成"指纹"，不同客户端（Go、Python、浏览器）的指纹显著不同。

```
Go 默认 TLS 指纹：
  TLS Versions: [1.2, 1.3]
  Cipher Suites: [TLS_AES_128_GCM_SHA256, TLS_AES_256_GCM_SHA384, TLS_CHACHA20_POLY1305_SHA256]
  Extensions: [server_name, status_request, supported_curves, ...]

Chrome 120 TLS 指纹：
  TLS Versions: [1.2, 1.3]
  Cipher Suites: [TLS_AES_128_GCM_SHA256, TLS_AES_256_GCM_SHA384, TLS_CHACHA20_POLY1305_SHA256, TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, ...]
  Extensions: [server_name, status_request, supported_curves, session_ticket, application_layer_protocol_negotiation, ...]

差异：密码套件数量、扩展顺序、GREASE 值使用
```

**2. HTTP 头部分析**

机器流量通常缺少浏览器特征头部：

| 头部 | Go 默认 | 真实浏览器 | 检测风险 |
|------|--------|-----------|---------|
| User-Agent | 无/Go-http-client | 完整浏览器标识 | 高 |
| Accept | 无 | text/html,application/... | 中 |
| Accept-Language | 无 | en-US,en;q=0.9 | 中 |
| Accept-Encoding | gzip | gzip, deflate, br | 低 |
| Sec-Ch-Ua | 无 | 有（现代浏览器） | 中 |

**3. 行为模式分析**

机器流量表现出规律性：
- 固定时间间隔发送请求
- 总是复用或从不复用连接
- 请求包大小高度一致
- 无人类操作的随机性

```
机器流量特征：
  请求间隔：1000ms, 1000ms, 1000ms, ...（标准差 = 0）
  连接复用：100% 或 0%
  请求大小：256, 256, 256, ...（字节）

人类流量特征：
  请求间隔：523ms, 1847ms, 912ms, ...（标准差大）
  连接复用：~70%
  请求大小：243, 312, 267, ...（字节）
```

**4. 流量特征分析**

- 固定大小的 TCP 包
- 无填充数据
- 固定的内容编码
- 可预测的请求/响应模式

### 1.2 检测系统的应对策略

检测系统使用多层分析：

```
┌─────────────────────────────────────────────────────────┐
│ Layer 1: 包级别分析                                      │
│ - TLS 指纹匹配已知代理库                                 │
│ - TCP/IP 栈指纹分析                                     │
├─────────────────────────────────────────────────────────┤
│ Layer 2: 会话级别分析                                    │
│ - HTTP 头部完整性检查                                   │
│ - User-Agent 数据库匹配                                  │
│ - 请求/响应时序分析                                     │
├─────────────────────────────────────────────────────────┤
│ Layer 3: 行为级别分析                                    │
│ - 长期流量模式识别                                      │
│ - 请求间隔统计分析                                      │
│ - 连接行为聚类分析                                      │
└─────────────────────────────────────────────────────────┘
```

## 二、防护机制设计

### 2.1 三层防护架构

```
┌─────────────────────────────────────────────────────────┐
│ Layer 1: Traffic Camouflage（流量伪装）                  │
│ 目标：使流量特征与真实浏览器一致                         │
│ 方法：TLS 指纹模拟、HTTP 头部注入、内容编码               │
├─────────────────────────────────────────────────────────┤
│ Layer 2: Behavior Jitter（行为打散）                     │
│ 目标：引入人类行为的随机性                              │
│ 方法：延迟抖动、连接复用打散、请求填充                   │
├─────────────────────────────────────────────────────────┤
│ Layer 3: Traffic Obfuscation（流量混淆）                 │
│ 目标：改变流量传输形态                                  │
│ 方法：WebSocket 隧道、请求分片                            │
└─────────────────────────────────────────────────────────┘
```

### 2.2 防护效果评估

| 检测维度 | 无防护 | 仅 L1 | L1+L2 | L1+L2+L3 |
|---------|--------|-------|-------|----------|
| TLS 指纹识别 | 高 | 低 | 低 | 低 |
| HTTP 头部检测 | 高 | 低 | 低 | 低 |
| 行为分析 | 高 | 高 | 中 | 低 |
| 流量特征分析 | 高 | 高 | 中 | 低 |
| 深度包检测 | 高 | 中 | 中 | 低 |

## 三、技术实现

### 3.1 TLS 指纹模拟

**实现原理：**

使用 `utls` 库（uTLS - unbreakable TLS）修改 TLS 握手的 ClientHello 结构：

```go
// utls 示例（概念代码）
config := utls.Config{
    ServerName: "api.anthropic.com",
}
conn := utls.Dial("tcp", "api.anthropic.com:443", config)

// 模拟 Chrome 指纹
helloID := utls.HelloChrome_120
conn.ApplyPreset(&helloID)
```

**模拟的浏览器指纹：**

| 浏览器 | 版本 | JA3 指纹 | 特征 |
|--------|------|---------|------|
| Chrome | 120 | 771,4865-... | GREASE 支持、特定扩展顺序 |
| Firefox | 121 | 771,4865-... | 不同的密码套件顺序 |
| Safari | 17 | 771,4865-... | 基于 Chrome 但有差异 |

**GREASE 值：**

浏览器为兼容未来协议，在 TLS 握手中随机插入 GREASE（Generate Random Extensions And Sustain Extensibility）值：

```
GREASE 位置示例：
  Cipher Suites: [..., 0x0A0A (GREASE), ...]
  Extensions: [..., 0xFAFA (GREASE), ...]
```

Go 默认实现不使用 GREASE，这是重要识别特征。

### 3.2 浏览器头部注入

**实现原理：**

中间件层拦截 HTTP 请求，注入完整的浏览器头部：

```go
func (m *ProtectionMiddleware) ApplyBrowserHeaders(req *http.Header) {
    // 根据配置模式获取预定义的浏览器头部
    headers := m.getBrowserHeaders()
    
    // 注入头部
    req.Set("User-Agent", headers.UserAgent)
    req.Set("Accept", headers.Accept)
    req.Set("Accept-Language", headers.AcceptLanguage)
    req.Set("Accept-Encoding", headers.AcceptEncoding)
    
    // 注入 Sec-* 头部（现代浏览器特征）
    if headers.SecChUa != "" {
        req.Set("Sec-Ch-Ua", headers.SecChUa)
        req.Set("Sec-Ch-Ua-Mobile", headers.SecChUaMobile)
        req.Set("Sec-Ch-Ua-Platform", headers.SecChUaPlatform)
    }
}
```

**头部数据来源：**

预定义真实浏览器的头部模板：

```go
var browserHeadersMap = map[string]BrowserHeaders{
    BrowserModeChrome: {
        UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36...",
        Accept: "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
        AcceptLanguage: "en-US,en;q=0.5",
        AcceptEncoding: "gzip, deflate, br",
        SecChUa: `"Not_A Brand";v="8", "Chromium";v="120"`,
        SecChUaMobile: "?0",
        SecChUaPlatform: `"macOS"`,
    },
    // ... Firefox, Safari
}
```

### 3.3 随机数生成

**为什么使用 crypto/rand？**

Go 的 `math/rand` 存在两个问题：
1. **不是线程安全的** - 并发访问需要加锁
2. **可预测** - 基于确定性算法，知道种子即可预测输出

防护场景需要：
- **不可预测** - 避免检测系统通过观察流量推测延迟模式
- **线程安全** - 服务天然并发处理多个请求

**实现对比：**

```go
// math/rand - 可预测，线程不安全
src := rand.NewSource(time.Now().UnixNano())
r := rand.New(src)
delay := r.Intn(100)  // 可预测序列

// crypto/rand - 不可预测，线程安全
n, _ := crypto_rand.Int(crypto_rand.Reader, big.NewInt(100))
delay := int(n.Int64())  // 加密安全的随机性
```

### 3.4 请求延迟抖动

**均匀分布实现：**

```go
// 在 [minMs, maxMs) 范围内均匀随机
n, _ := crypto_rand.Int(crypto_rand.Reader, big.NewInt(int64(maxMs-minMs)))
return minMs + int(n.Int64())
```

**指数分布实现：**

指数分布使延迟更倾向于接近最小值，模拟人类"尽快但偶尔延迟"的行为：

```go
const exponentialFactor = 3.0

// ExpFloat64() 返回均值为 1 的指数分布随机数
expValue := m.rng.ExpFloat64() / exponentialFactor
return minMs + int(expValue*float64(maxMs-minMs))
```

**分布对比：**

```
均匀分布（Uniform）:
  min_ms=50, max_ms=200
  P(delay) = 1/150 对于所有 delay ∈ [50, 200)
  期望值：E[delay] = (50+200)/2 = 125ms

指数分布（Exponential）:
  min_ms=50, max_ms=200
  P(delay) ∝ e^(-λ·delay), λ = 3.0
  大部分延迟接近 50ms，少数延迟接近 200ms
  期望值：E[delay] ≈ 50 + (200-50)/3 ≈ 100ms
```

### 3.5 连接复用打散

**实现原理：**

```go
func (m *ProtectionMiddleware) ShouldReuseConnection() bool {
    // 默认行为：总是复用
    if !m.cfg.BehaviorJitter.ConnectionReuse.Enabled {
        return true
    }
    
    // 根据复用率随机决定
    reuseRate := m.cfg.BehaviorJitter.ConnectionReuse.ReuseRate
    n, _ := crypto_rand.Int(crypto_rand.Reader, big.NewInt(1000))
    return float64(n.Int64())/1000 < reuseRate
}
```

**HTTP 连接复用：**

```
不复用（Connection: close）:
  Client                Server
    |  Request 1  |
    |------------>|
    |  Response 1 |
    |<------------|
    |  (TCP 断开)  |
    |  Request 2  |
    |------------>|
    |  Response 2 |
    |<------------|

复用（Keep-Alive）:
  Client                Server
    |  Request 1  |
    |------------>|
    |  Response 1 |
    |<------------|
    |  Request 2  |  ← 同一连接
    |------------>|
    |  Response 2 |
    |<------------|
```

**检测风险：**
- 100% 复用：机器流量特征（性能优化）
- 0% 复用：机器流量特征（避免追踪）
- ~70% 复用：模拟浏览器行为

### 3.6 请求填充

**实现原理：**

```go
func (m *ProtectionMiddleware) GeneratePadding(size int) []byte {
    padding := make([]byte, size)
    _, err := crypto_rand.Read(padding)
    if err != nil {
        // 降级：使用可预测但确定性的填充
        for i := range padding {
            padding[i] = byte(32 + (i % 95))  // ASCII 可打印字符
        }
    }
    return padding
}
```

**填充数据格式：**

填充以 Base64 编码添加到请求体中：

```json
// 原始请求
{
  "model": "claude-3",
  "messages": [...]
}

// 添加填充后
{
  "model": "claude-3",
  "messages": [...],
  "_padding": "aGVsbG8gd29ybGQh..."  // Base64 编码的随机字节
}
```

**填充效果：**

```
无填充:
  请求 1: 256 字节
  请求 2: 256 字节
  请求 3: 256 字节

有填充（10-100 字节随机）:
  请求 1: 287 字节
  请求 2: 341 字节
  请求 3: 269 字节
```

## 四、并发安全设计

### 4.1 问题分析

`math/rand.Rand` 不是线程安全的：

```go
// 错误示例：并发访问导致 panic
r := rand.New(rand.NewSource(time.Now().UnixNano()))
go func() { r.Intn(100) }()  // goroutine 1
go func() { r.Intn(100) }()  // goroutine 2 - 可能 panic!
```

### 4.2 解决方案

**方案一：互斥锁保护**

```go
type ProtectionMiddleware struct {
    cfg *config.ProtectionConfig
    mu  sync.Mutex
    rng *math_rand.Rand
}

func (m *ProtectionMiddleware) GetRequestDelay() int {
    m.mu.Lock()
    defer m.mu.Unlock()
    // 安全使用 m.rng
}
```

**方案二：crypto/rand**

```go
// crypto/rand 是线程安全的，无需加锁
n, _ := crypto_rand.Int(crypto_rand.Reader, big.NewInt(100))
```

**混合方案（本实现）：**

- 临界区使用 `crypto/rand`（无需持锁）
- 非临界区使用 `math/rand`（如指数分布）
- 使用 `sync.Mutex` 保护共享状态

```go
func (m *ProtectionMiddleware) GetRequestDelay() int {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    // 检查配置（临界区）
    if !m.cfg.Enabled {
        return 0
    }
    
    // crypto/rand 无需持锁即可调用
    m.mu.Unlock()
    n, _ := crypto_rand.Int(crypto_rand.Reader, big.NewInt(100))
    m.mu.Lock()
    
    return int(n.Int64())
}
```

### 4.3 死锁修复

早期版本存在的问题：

```go
// 错误：在持有锁时调用 IsEnabled()，后者也尝试获取锁
func (m *ProtectionMiddleware) GetRequestDelay() int {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    if !m.IsEnabled() {  // IsEnabled() 也调用 m.mu.Lock() ← 死锁！
        return 0
    }
    // ...
}
```

修复方案：直接检查配置，避免嵌套锁调用：

```go
func (m *ProtectionMiddleware) GetRequestDelay() int {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    // 直接检查，不调用 IsEnabled()
    if m.cfg == nil || !m.cfg.Enabled {
        return 0
    }
    // ...
}
```

## 五、性能影响

### 5.1 延迟开销

| 防护功能 | 平均开销 | 最大开销 |
|---------|---------|---------|
| 浏览器头部注入 | <0.1ms | <0.5ms |
| 请求延迟抖动 | 50-200ms | 200ms |
| 连接复用打散 | <0.1ms | <0.5ms |
| 请求填充生成 | <0.5ms | <1ms |
| TLS 指纹模拟 | ~10ms | ~20ms |

**总开销：**
- 基础防护（L1+L2）：50-200ms（主要是延迟抖动）
- 完整防护（L1+L2+L3）：50-250ms

### 5.2 CPU 开销

| 操作 | CPU 周期 | 说明 |
|------|---------|------|
| crypto/rand 调用 | ~1000 | 系统调用开销 |
| Base64 编码 | ~500/KB | 线性复杂度 |
| TLS 握手（utls） | ~50000 | 仅在连接建立时 |

**并发能力：**
- 单核：~1000 请求/秒（基础防护）
- 4 核：~4000 请求/秒（基础防护）

## 六、最佳实践

### 6.1 配置建议

**生产环境推荐：**

```yaml
protection:
  enabled: true
  
  # 必须：浏览器头部伪装
  traffic_camouflage:
    browser_headers:
      enabled: true
      mode: random
  
  # 推荐：行为打散
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
  
  # 可选：根据风险等级启用
  traffic_obfuscation:
    websocket_tunnel:
      enabled: false  # 高风险场景开启
```

### 6.2 监控建议

**监控指标：**

```yaml
# 防护日志
logging:
  log_protected_requests: true   # 记录防护的请求
  log_jitter_applied: false      # 避免暴露防护参数
```

**关键指标：**
- 防护触发次数
- 延迟分布直方图
- 连接复用率
- 请求大小分布

### 6.3 部署建议

**Nginx 反向代理：**

```
客户端 → Nginx (TLS 终止 + 限流) → llm-proxy (行为打散)
```

**配置调整：**

```yaml
protection:
  traffic_camouflage:
    tls_fingerprint:
      enabled: false  # Nginx 处理 TLS
    browser_headers:
      enabled: true   # 代理层仍需注入
```

## 七、局限性

### 7.1 防护边界

**能防护的：**
- TLS 指纹识别
- HTTP 头部分析
- 基础行为分析
- 流量大小分析

**不能防护的：**
- 深度包检测（DPI）分析应用层协议
- 长期流量模式分析（需要额外措施）
- IP 地址层面的封锁
- DNS 污染

### 7.2 应对策略

**多层防护：**

```
┌─────────────────────────────────────────────────────────┐
│ Layer 0: 基础设施层                                      │
│ - CDN / 中转服务器                                       │
│ - 多 IP 轮换                                             │
├─────────────────────────────────────────────────────────┤
│ Layer 1: 传输层（本代理防护）                            │
│ - TLS 指纹模拟                                          │
│ - HTTP 头部注入                                         │
├─────────────────────────────────────────────────────────┤
│ Layer 2: 应用层                                         │
│ - 请求频率限制                                          │
│ - 用户行为模拟                                          │
└─────────────────────────────────────────────────────────┘
```

## 八、参考资料

- [JA3 TLS Fingerprinting](https://engineering.salesforce.com/ja3-and-ja4-tls-fingerprinting-explained-7f7523388d6a)
- [uTLS Library](https://github.com/refraction-networking/utls)
- [Go crypto/rand](https://pkg.go.dev/crypto/rand)
- [HTTP Header Best Practices](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers)
