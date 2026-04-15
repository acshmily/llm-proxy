# 多阶段构建 - 支持多架构编译
# 使用 docker buildx 构建：
#   docker buildx build --platform linux/amd64,linux/arm64 -t llm-proxy:latest .

# ========== 构建阶段 ==========
FROM golang:1.21-alpine AS builder

# 安装必要的工具
RUN apk add --no-cache git

# 设置工作目录
WORKDIR /build

# 设置 GOARCH 和 GOOS（由 buildx 自动设置）
ARG TARGETOS
ARG TARGETARCH

# 启用 CGO 交叉编译支持（ARM64 需要）
ENV CGO_ENABLED=0
ENV GOOS=${TARGETOS:-linux}
ENV GOARCH=${TARGETARCH}

# 启用 Go 模块缓存加速
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go version

# 复制 go.mod 和 go.sum（优先缓存依赖）
COPY go.mod go.sum ./

# 下载依赖
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# 复制源代码
COPY . .

# 编译二进制文件（静态链接）
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o proxy ./cmd/proxy

# ========== 运行阶段 ==========
FROM alpine:3.19 AS runner

# 安装必要的运行时依赖
RUN apk add --no-cache ca-certificates tzdata

# 创建非 root 用户
RUN addgroup -g 1000 proxy && \
    adduser -D -u 1000 -G proxy proxy

# 设置工作目录
WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /build/proxy .

# 复制示例配置文件（实际使用时挂载外部 config.yaml）
COPY config.example.yaml /app/config.example.yaml

# 设置文件权限
RUN chown -R proxy:proxy /app && \
    chmod +x proxy

# 切换到非 root 用户
USER proxy

# 暴露端口
EXPOSE 8080

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/health || exit 1

# 启动命令（默认使用 config.yaml，需外部挂载）
ENTRYPOINT ["./proxy"]
CMD ["-config", "config.yaml"]
