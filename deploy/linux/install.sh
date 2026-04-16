#!/bin/bash
#
# LLM Proxy 安装脚本
# 用途：一键安装 llm-proxy 为 systemd 服务
#
# 使用方法:
#   curl -fsSL https://raw.githubusercontent.com/acshmily/llm-proxy/main/deploy/linux/install.sh | sudo bash
#
# 或者:
#   wget https://raw.githubusercontent.com/acshmily/llm-proxy/main/deploy/linux/install.sh
#   sudo bash install.sh
#

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查是否以 root 运行
if [ "$EUID" -ne 0 ]; then
    log_error "请使用 sudo 运行此脚本：sudo $0"
    exit 1
fi

# 检查 systemd 是否可用
if ! command -v systemctl &> /dev/null; then
    log_error "未检测到 systemd，此脚本仅支持使用 systemd 的 Linux 发行版"
    exit 1
fi

log_info "开始安装 LLM Proxy..."

# ======================
# 配置变量
# ======================
INSTALL_DIR="/opt/llm-proxy"
SERVICE_NAME="llm-proxy"
SERVICE_USER="llm-proxy"
SERVICE_GROUP="llm-proxy"
REPO_URL="https://github.com/acshmily/llm-proxy"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ======================
# 检测系统架构
# ======================
ARCH=$(uname -m)
case $ARCH in
    x86_64)
        BINARY_ARCH="linux-amd64"
        ;;
    aarch64)
        BINARY_ARCH="linux-arm64"
        ;;
    *)
        log_error "不支持的架构：$ARCH"
        exit 1
        ;;
esac

log_info "检测到系统架构：$ARCH ($BINARY_ARCH)"

# ======================
# 创建用户和组
# ======================
if ! getent group "$SERVICE_GROUP" > /dev/null 2>&1; then
    log_info "创建用户组：$SERVICE_GROUP"
    groupadd -r "$SERVICE_GROUP"
else
    log_info "用户组已存在：$SERVICE_GROUP"
fi

if ! id "$SERVICE_USER" > /dev/null 2>&1; then
    log_info "创建用户：$SERVICE_USER"
    useradd -r -g "$SERVICE_GROUP" -d "$INSTALL_DIR" -s /sbin/nologin "$SERVICE_USER"
else
    log_info "用户已存在：$SERVICE_USER"
fi

# ======================
# 创建安装目录
# ======================
log_info "创建安装目录：$INSTALL_DIR"
mkdir -p "$INSTALL_DIR"
mkdir -p "$INSTALL_DIR/logs"

# ======================
# 安装二进制文件
# ======================
if [ -f "$SCRIPT_DIR/proxy" ]; then
    log_info "复制本地二进制文件..."
    cp "$SCRIPT_DIR/proxy" "$INSTALL_DIR/proxy"
    chmod +x "$INSTALL_DIR/proxy"
elif [ -f "$SCRIPT_DIR/proxy-$BINARY_ARCH" ]; then
    log_info "复制本地二进制文件（$BINARY_ARCH）..."
    cp "$SCRIPT_DIR/proxy-$BINARY_ARCH" "$INSTALL_DIR/proxy"
    chmod +x "$INSTALL_DIR/proxy"
else
    # 从 GitHub 下载最新版本
    log_info "从 GitHub 下载最新版本的二进制文件..."
    LATEST_RELEASE=$(curl -s https://api.github.com/repos/acshmily/llm-proxy/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    if [ -z "$LATEST_RELEASE" ]; then
        log_warn "无法获取最新版本信息，尝试从 main 分支下载"
        BINARY_URL="$REPO_URL/releases/latest/download/proxy-$BINARY_ARCH"
    else
        BINARY_URL="$REPO_URL/releases/download/$LATEST_RELEASE/proxy-$BINARY_ARCH"
    fi

    log_info "下载地址：$BINARY_URL"

    if command -v wget &> /dev/null; then
        wget -q -O "$INSTALL_DIR/proxy" "$BINARY_URL" || {
            log_error "下载二进制文件失败，请手动下载并放置到 $INSTALL_DIR/proxy"
            exit 1
        }
    elif command -v curl &> /dev/null; then
        curl -sL -o "$INSTALL_DIR/proxy" "$BINARY_URL" || {
            log_error "下载二进制文件失败，请手动下载并放置到 $INSTALL_DIR/proxy"
            exit 1
        }
    else
        log_error "未找到 wget 或 curl，请手动下载二进制文件"
        exit 1
    fi

    chmod +x "$INSTALL_DIR/proxy"
fi

log_info "二进制文件已安装：$INSTALL_DIR/proxy"
log_info "版本：$($INSTALL_DIR/proxy -version)"

# ======================
# 安装配置文件
# ======================
if [ ! -f "$INSTALL_DIR/config.yaml" ]; then
    log_info "复制配置文件示例..."
    if [ -f "$SCRIPT_DIR/config.example.yaml" ]; then
        cp "$SCRIPT_DIR/config.example.yaml" "$INSTALL_DIR/config.yaml"
    else
        # 从仓库下载
        curl -sL -o "$INSTALL_DIR/config.yaml" "$REPO_URL/raw/main/config.example.yaml"
    fi
    log_warn "请编辑 $INSTALL_DIR/config.yaml 填入你的 API 密钥"
else
    log_info "配置文件已存在，跳过"
fi

# ======================
# 安装 systemd 服务
# ======================
log_info "安装 systemd 服务..."

if [ -f "$SCRIPT_DIR/llm-proxy.service" ]; then
    cp "$SCRIPT_DIR/llm-proxy.service" "/etc/systemd/system/$SERVICE_NAME.service"
else
    # 从仓库下载
    curl -sL -o "/etc/systemd/system/$SERVICE_NAME.service" "$REPO_URL/raw/main/deploy/linux/llm-proxy.service"
fi

# 重新加载 systemd 配置
systemctl daemon-reload

# 设置目录权限
chown -R "$SERVICE_USER:$SERVICE_GROUP" "$INSTALL_DIR"
chmod 755 "$INSTALL_DIR"
chmod 644 "$INSTALL_DIR/config.yaml"

# ======================
# 启用并启动服务
# ======================
log_info "启用服务（开机自启）..."
systemctl enable "$SERVICE_NAME"

log_info "启动服务..."
systemctl start "$SERVICE_NAME"

# ======================
# 验证服务状态
# ======================
sleep 2
if systemctl is-active --quiet "$SERVICE_NAME"; then
    log_info "✓ LLM Proxy 安装成功！"
    echo ""
    echo "服务名称：$SERVICE_NAME"
    echo "安装目录：$INSTALL_DIR"
    echo "配置文件：$INSTALL_DIR/config.yaml"
    echo "日志目录：$INSTALL_DIR/logs"
    echo ""
    echo "常用命令:"
    echo "  查看状态：sudo systemctl status $SERVICE_NAME"
    echo "  查看日志：sudo journalctl -u $SERVICE_NAME -f"
    echo "  重启服务：sudo systemctl restart $SERVICE_NAME"
    echo "  停止服务：sudo systemctl stop $SERVICE_NAME"
    echo ""
    log_warn "请记得编辑配置文件：$INSTALL_DIR/config.yaml"
else
    log_error "服务启动失败，请检查日志：sudo journalctl -u $SERVICE_NAME -n 50"
    exit 1
fi
