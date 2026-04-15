# 贡献指南

感谢你考虑为 LLM Proxy 项目做出贡献！

## 目录

- [行为准则](#行为准则)
- [开发环境设置](#开发环境设置)
- [开发流程](#开发流程)
- [代码规范](#代码规范)
- [测试要求](#测试要求)
- [提交规范](#提交规范)
- [代码审查](#代码审查)

## 行为准则

本项目采用 [Contributor Covenant](https://www.contributor-covenant.org/) 行为准则。
请尊重每一位贡献者，营造友好、包容的社区环境。

## 开发环境设置

### 前置要求

- Go 1.21 或更高版本
- Docker 20.10+（可选，用于容器化测试）
- Git

### 安装依赖

```bash
git clone https://github.com/acshmily/llm-proxy.git
cd llm-proxy
go mod download
```

### 运行测试

```bash
# 运行所有测试
go test -v ./...

# 运行测试并生成覆盖率报告
go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
go tool cover -html=coverage.out

# 运行特定包的测试
go test -v ./internal/wsclient/...
```

## 开发流程

### 1. Fork 项目

```bash
# 在 GitHub 上 fork 项目
# 然后克隆到你的本地
git clone https://github.com/YOUR_USERNAME/llm-proxy.git
cd llm-proxy
```

### 2. 创建分支

```bash
# 基于 main 分支创建功能分支
git checkout -b feature/your-feature-name

# 或修复 bug
git checkout -b fix/issue-123
```

### 3. 开发并提交

```bash
# 编写代码和测试
# 确保测试通过
go test -v ./...

# 提交更改
git add .
git commit -m "feat: add your feature description"
```

### 4. 推送到远程

```bash
git push origin feature/your-feature-name
```

### 5. 创建 Pull Request

在 GitHub 上创建 Pull Request，填写清晰的描述和测试说明。

## 代码规范

### Go 代码风格

遵循 [Effective Go](https://golang.org/doc/effective_go) 和 [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)。

**关键要点：**

```go
// ✅ 好的：有意义的变量名
var connectionString string

// ❌ 差的：缩写无意义
var cs string

// ✅ 好的：错误处理
if err != nil {
    return fmt.Errorf("failed to connect: %w", err)
}

// ✅ 好的：注释解释为什么，而不是做什么
// 使用指数退避避免服务端限流
delay := time.Duration(math.Pow(2, float64(attempt))) * time.Second
```

### 目录结构

```
cmd/
  proxy/           # 主程序入口
  ws-client/       # WebSocket 隧道客户端
internal/
  config/          # 配置加载和验证
  middleware/      # 中间件（混淆、限流等）
  protocol/        # 协议转换（OpenAI, Gemini, Claude）
  router/          # 路由匹配
  server/          # HTTP 服务器
  wsclient/        # WebSocket 客户端
pkg/               # 可复用的公共包
test/              # 集成测试和 mock
```

### 错误处理

```go
// ✅ 好的：错误包装和上下文
if err := json.Unmarshal(data, &req); err != nil {
    return nil, fmt.Errorf("invalid JSON format: %w", err)
}

// ✅ 好的：定义 sentinel error
var ErrTunnelDisconnected = errors.New("tunnel is disconnected")

// 使用 errors.Is 判断
if errors.Is(err, ErrTunnelDisconnected) {
    return nil, http.StatusServiceUnavailable
}
```

## 测试要求

### TDD 原则

**所有新功能必须遵循 TDD（测试驱动开发）：**

1. 编写失败的测试（Red）
2. 编写最小化代码使测试通过（Green）
3. 重构代码保持测试通过（Refactor）

### 测试覆盖要求

**最低覆盖率要求：**
- 新功能：≥ 80%
- 核心模块（protocol, wsclient）：≥ 90%

**测试类型：**

```go
// 1. 单元测试 - 测试单个函数
func TestEncodeRequest(t *testing.T) {
    t.Run("GET request", func(t *testing.T) {
        // ...
    })
}

// 2. 边界测试 - 空输入、错误输入
func TestDecodeResponse_EdgeCases(t *testing.T) {
    t.Run("invalid JSON", func(t *testing.T) {
        // ...
    })
}

// 3. 集成测试 - 端到端流程
func TestProxyEndToEnd(t *testing.T) {
    // ...
}
```

### 运行测试

```bash
# 提交前必须运行
go test -v -race ./...

# 检查覆盖率
go test -cover ./...
```

## 提交规范

### Conventional Commits

遵循 [Conventional Commits](https://www.conventionalcommits.org/) 规范：

```
<type>(<scope>): <description>

[optional body]
```

### 类型说明

| 类型 | 说明 | 示例 |
|------|------|------|
| `feat` | 新功能 | `feat(ws-client): add heartbeat mechanism` |
| `fix` | Bug 修复 | `fix(protocol): prevent panic on empty choices` |
| `test` | 测试相关 | `test(protocol): add invalid JSON tests` |
| `docs` | 文档更新 | `docs: update README with WebSocket guide` |
| `refactor` | 重构（无功能变更） | `refactor: extract helper function` |
| `chore` | 构建/工具配置 | `chore: add GitHub Actions workflow` |

### 提交示例

```bash
# 新功能
git commit -m "feat(ws-client): add WebSocket tunnel client

- HTTP proxy server with configurable listen address
- Single long-lived WebSocket connection
- Automatic reconnection with exponential backoff
- Health check endpoint"

# Bug 修复
git commit -m "fix(protocol): prevent panic on empty choices array

Add bounds check before accessing resp.Choices[0] to prevent
index out of range panic when API returns empty choices."

# 测试
git commit -m "test(protocol): add invalid JSON error handling tests

Add test cases for invalid JSON input in OpenAI, Gemini,
and Claude protocol converters."
```

## 代码审查

### 审查清单

提交 PR 前，请确保：

- [ ] 代码通过所有测试
- [ ] 添加了适当的单元测试
- [ ] 遵循 Go 代码规范
- [ ] 提交了清晰的提交信息
- [ ] 更新了相关文档（如需要）

### 审查流程

1. **自动化检查** - GitHub Actions 运行测试和构建
2. **贡献者审查** - 至少一位维护者审查代码
3. **合并** - 审查通过后合并到 main 分支

### 审查标准

**必须修复的问题：**
- 测试失败
- 竞态检测失败（`-race`）
- 安全漏洞
- 明显的逻辑错误

**建议改进的问题：**
- 代码可读性
- 性能优化建议
- 测试覆盖率提升

---

## 提问和讨论

- **GitHub Issues** - Bug 报告和功能请求
- **GitHub Discussions** - 一般性问题讨论

感谢你的贡献！
