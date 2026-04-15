.PHONY: build build-amd64 build-arm64 build-multiarch docker-build docker-push test clean

# 二进制名称
BINARY_NAME=proxy
IMAGE_NAME=proxy-gemini-go
IMAGE_TAG=latest

# 本地编译
build:
	@echo "==> 编译本地二进制..."
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY_NAME) ./cmd/proxy

# 编译 AMD64 架构
build-amd64:
	@echo "==> 编译 linux/amd64 架构..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY_NAME)-amd64 ./cmd/proxy

# 编译 ARM64 架构
build-arm64:
	@echo "==> 编译 linux/arm64 架构..."
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY_NAME)-arm64 ./cmd/proxy

# 多架构编译（静态链接）
build-multiarch: build-amd64 build-arm64
	@echo "==> 多架构编译完成"

# Docker 构建（单架构）
docker-build:
	@echo "==> 构建 Docker 镜像 (当前架构)..."
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

# Docker 多架构构建（需要 buildx）
docker-multiarch:
	@echo "==> 构建多架构 Docker 镜像..."
	docker buildx build --platform linux/amd64,linux/arm64 \
		-t $(IMAGE_NAME):$(IMAGE_TAG) \
		-t $(IMAGE_NAME):latest \
		--load \
		.

# Docker 多架构构建并推送到仓库
docker-push:
	@echo "==> 构建并推送多架构镜像到仓库..."
	docker buildx build --platform linux/amd64,linux/arm64 \
		-t $(IMAGE_NAME):$(IMAGE_TAG) \
		-t $(IMAGE_NAME):latest \
		--push \
		.

# 运行测试
test:
	@echo "==> 运行测试..."
	go test -v ./...

# 清理
clean:
	@echo "==> 清理构建产物..."
	rm -f $(BINARY_NAME) $(BINARY_NAME)-amd64 $(BINARY_NAME)-arm64
	@echo "==> 清理完成"

# 帮助信息
help:
	@echo "可用命令:"
	@echo "  make build           - 编译本地二进制"
	@echo "  make build-amd64     - 编译 linux/amd64 架构"
	@echo "  make build-arm64     - 编译 linux/arm64 架构"
	@echo "  make build-multiarch - 多架构编译"
	@echo "  make docker-build    - 构建 Docker 镜像 (当前架构)"
	@echo "  make docker-multiarch- 构建多架构 Docker 镜像"
	@echo "  make docker-push     - 推送多架构镜像到仓库"
	@echo "  make test            - 运行测试"
	@echo "  make clean           - 清理构建产物"
