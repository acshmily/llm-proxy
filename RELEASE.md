# 发布指南

## 创建新版本

1. 更新版本号（如需要）

2. 创建 git tag：
   ```bash
   git tag -a v0.1.0 -m "Release v0.1.0"
   git push origin v0.1.0
   ```

3. GitHub Actions 会自动：
   - 构建多平台二进制文件（Linux/macOS/Windows × AMD64/ARM64）
   - 构建并推送 Docker 多架构镜像
   - 创建 GitHub Release 并上传所有构建产物

## 发布产物

### 二进制文件
- `proxy-v0.1.0-linux-amd64.tar.gz` - Linux AMD64
- `proxy-v0.1.0-linux-arm64.tar.gz` - Linux ARM64
- `proxy-v0.1.0-darwin-amd64.tar.gz` - macOS AMD64
- `proxy-v0.1.0-darwin-arm64.tar.gz` - macOS ARM64 (M1/M2)
- `proxy-v0.1.0-windows-amd64.zip` - Windows AMD64

### Docker 镜像
- `acshmily/llm-proxy:v0.1.0` - 版本标签
- `acshmily/llm-proxy:latest` - 最新稳定版

每个 Release 包含：
- 对应平台的二进制文件
- `config.example.yaml` 配置示例
- `README.md` 使用文档

## 语义化版本

遵循 [Semantic Versioning](https://semver.org/)：

- **MAJOR** (v1.0.0) - 不兼容的 API 变更
- **MINOR** (v0.2.0) - 向后兼容的功能新增
- **PATCH** (v0.1.1) - 向后兼容的问题修复

## 预发布版本

创建预发布版本：
```bash
git tag -a v0.1.0-rc.1 -m "Release candidate 1"
git push origin v0.1.0-rc.1
```

预发布版本会在 Release 页面标记为 "Pre-release"。
