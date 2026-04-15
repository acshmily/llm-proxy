# 流量混淆功能实现计划

**Goal:** 实现 WebSocket 隧道和请求分片功能，完成防护机制的第三层流量混淆

**Architecture:** 
- 请求分片：在客户端将大请求分片，服务端自动重组
- WebSocket 隧道：建立 WebSocket 连接，通过 WS 隧道传输 HTTP 请求

**Tech Stack:**
- Go net/http 包
- Gorilla WebSocket (需添加依赖)
- Base64 编码分片数据

---

### Task 1: 请求分片功能实现

**Files:**
- Create: `internal/middleware/obfuscation.go`
- Test: `internal/middleware/obfuscation_test.go`

- [ ] **Step 1: 编写分片功能测试**

```go
func TestTrafficObfuscationMiddleware_ShouldShardRequest(t *testing.T) {
    // 测试是否应该分片
}

func TestTrafficObfuscationMiddleware_ShardRequest(t *testing.T) {
    // 测试分片功能
}

func TestTrafficObfuscationMiddleware_ReassembleChunks(t *testing.T) {
    // 测试重组功能
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/middleware -run TestTrafficObfuscation -v
```

Expected: FAIL (functions not defined)

- [ ] **Step 3: 实现分片功能**

```go
// 实现 ShouldShardRequest, ShardRequest, ReassembleChunks 等方法
```

- [ ] **Step 4: 运行测试确认通过**

- [ ] **Step 5: 提交**

```bash
git add internal/middleware/obfuscation.go internal/middleware/obfuscation_test.go
git commit -m "feat: implement request sharding for traffic obfuscation"
```

### Task 2: WebSocket 隧道服务端实现

**Files:**
- Create: `internal/server/ws_tunnel.go`
- Test: `internal/server/ws_tunnel_test.go`

- [ ] **Step 1: 编写 WebSocket 隧道测试**

```go
func TestWSTunnel_HandleWebSocket(t *testing.T) {
    // 测试 WS 隧道握手
}

func TestWSTunnel_EncodeHTTPRequest(t *testing.T) {
    // 测试 HTTP 请求编码
}

func TestWSTunnel_DecodeHTTPRequest(t *testing.T) {
    // 测试 HTTP 请求解码
}
```

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 实现 WebSocket 隧道**

```go
// 实现 WS 隧道 handler
```

- [ ] **Step 4: 运行测试确认通过**

- [ ] **Step 5: 提交**

```bash
git add internal/server/ws_tunnel.go internal/server/ws_tunnel_test.go
git commit -m "feat: implement WebSocket tunnel server"
```

### Task 3: 添加 Gorilla WebSocket 依赖

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: 添加依赖**

```bash
go get github.com/gorilla/websocket
```

- [ ] **Step 2: 提交**

```bash
git add go.mod go.sum
git commit -m "chore: add gorilla/websocket dependency"
```

### Task 4: 集成到 HTTP 服务器

**Files:**
- Modify: `internal/server/server.go`

- [ ] **Step 1: 添加 WebSocket 路由**

```go
// 在 server.go 中添加 /ws-tunnel 路由
```

- [ ] **Step 2: 添加分片请求处理**

```go
// 在 ServeHTTP 中检测并处理分片请求
```

- [ ] **Step 3: 提交**

```bash
git add internal/server/server.go
git commit -m "feat: integrate traffic obfuscation into server"
```

### Task 5: 更新配置和文档

**Files:**
- Modify: `docs/protection.md`
- Modify: `README.md`

- [ ] **Step 1: 更新文档移除"计划中"标注**

- [ ] **Step 2: 提交**

```bash
git add docs/protection.md README.md
git commit -m "docs: update protection docs with implemented features"
```
