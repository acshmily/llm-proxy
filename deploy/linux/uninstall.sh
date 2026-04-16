#!/bin/bash
#
# LLM Proxy 卸载脚本
# 用途：卸载 llm-proxy systemd 服务
#
# 使用方法:
#   sudo bash uninstall.sh
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

# ======================
# 配置变量
# ======================
INSTALL_DIR="/opt/llm-proxy"
SERVICE_NAME="llm-proxy"
SERVICE_USER="llm-proxy"
SERVICE_GROUP="llm-proxy"

echo ""
echo "=========================================="
echo "  LLM Proxy 卸载程序"
echo "=========================================="
echo ""

# ======================
# 确认卸载
# ======================
echo -n "此操作将卸载 LLM Proxy 服务，是否继续？[y/N]: "
read -r CONFIRM
if [[ ! $CONFIRM =~ ^[Yy]$ ]]; then
    log_info "已取消卸载"
    exit 0
fi

echo ""
echo -n "是否保留配置文件和数据？[Y/n]: "
read -r KEEP_CONFIG
if [[ $KEEP_CONFIG =~ ^[Nn]$ ]]; then
    KEEP_CONFIG=false
else
    KEEP_CONFIG=true
fi

# ======================
# 停止服务
# ======================
log_info "停止服务..."
if systemctl is-active --quiet "$SERVICE_NAME"; then
    systemctl stop "$SERVICE_NAME"
    log_info "服务已停止"
else
    log_warn "服务未运行，跳过停止"
fi

# ======================
# 禁用服务
# ======================
log_info "禁用服务（取消开机自启）..."
systemctl disable "$SERVICE_NAME" 2>/dev/null || true

# ======================
# 移除 systemd 服务
# ======================
log_info "移除 systemd 服务配置..."
rm -f "/etc/systemd/system/$SERVICE_NAME.service"
rm -f "/etc/systemd/system/multi-user.target.wants/$SERVICE_NAME.service"
systemctl daemon-reload

# ======================
# 删除安装目录
# ======================
if [ "$KEEP_CONFIG" = false ]; then
    log_info "删除安装目录：$INSTALL_DIR"
    rm -rf "$INSTALL_DIR"
else
    log_info "保留配置文件和数据"
    log_info "配置目录：$INSTALL_DIR"
fi

# ======================
# 删除用户和组（可选）
# ======================
echo ""
echo -n "是否删除服务用户 $SERVICE_USER？[y/N]: "
read -r DELETE_USER
if [[ $DELETE_USER =~ ^[Yy]$ ]]; then
    log_info "删除用户：$SERVICE_USER"
    userdel -r "$SERVICE_USER" 2>/dev/null || userdel "$SERVICE_USER" 2>/dev/null || true

    log_info "删除用户组：$SERVICE_GROUP"
    groupdel "$SERVICE_GROUP" 2>/dev/null || true
else
    log_info "保留用户和组"
fi

# ======================
# 完成
# ======================
echo ""
log_info "✓ LLM Proxy 已卸载完成！"

if [ "$KEEP_CONFIG" = true ]; then
    echo ""
    log_warn "配置文件保留在：$INSTALL_DIR/config.yaml"
    log_warn "如需完全清理，可手动删除：rm -rf $INSTALL_DIR"
fi

echo ""
