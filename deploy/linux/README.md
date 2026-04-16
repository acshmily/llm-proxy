# LLM Proxy Linux 服务安装指南

本文档介绍如何在 Linux 服务器上将 LLM Proxy 安装为 systemd 服务，实现后台运行、开机自启、日志管理等功能。

## 目录

- [快速开始](#快速开始)
- [安装方法](#安装方法)
- [服务管理](#服务管理)
- [日志查看](#日志查看)
- [配置修改](#配置修改)
- [卸载方法](#卸载方法)
- [故障排查](#故障排查)

---

## 快速开始

**一键安装（推荐）：**

```bash
curl -fsSL https://raw.githubusercontent.com/acshmily/llm-proxy/main/deploy/linux/install.sh | sudo bash
```

安装完成后：
1. 编辑配置文件 `/opt/llm-proxy/config.yaml`
2. 填入你的 API 密钥
3. 重启服务：`sudo systemctl restart llm-proxy`

---

## 安装方法

### 方法一：一键安装（推荐）

自动下载最新版本并安装为服务：

```bash
curl -fsSL https://raw.githubusercontent.com/acshmily/llm-proxy/main/deploy/linux/install.sh | sudo bash
```

### 方法二：手动安装

1. **下载安装脚本：**

   ```bash
   wget https://raw.githubusercontent.com/acshmily/llm-proxy/main/deploy/linux/install.sh
   ```

2. **准备二进制文件（可选）：**

   如果已编译好二进制文件，将其命名为 `proxy` 并与 `install.sh` 放在同一目录。

   否则安装脚本会自动从 GitHub 下载最新版本。

3. **运行安装脚本：**

   ```bash
   sudo bash install.sh
   ```

### 方法三：完全手动安装

适合需要自定义安装路径的场景。

1. **创建用户和组：**

   ```bash
   sudo groupadd -r llm-proxy
   sudo useradd -r -g llm-proxy -d /opt/llm-proxy -s /sbin/nologin llm-proxy
   ```

2. **创建安装目录：**

   ```bash
   sudo mkdir -p /opt/llm-proxy
   sudo mkdir -p /opt/llm-proxy/logs
   ```

3. **安装二进制文件：**

   ```bash
   # 从 Release 下载
   wget https://github.com/acshmily/llm-proxy/releases/latest/download/proxy-linux-amd64 -O proxy
   chmod +x proxy
   sudo mv proxy /opt/llm-proxy/proxy
   ```

4. **复制配置文件：**

   ```bash
   sudo cp config.example.yaml /opt/llm-proxy/config.yaml
   sudo chown llm-proxy:llm-proxy /opt/llm-proxy/config.yaml
   ```

5. **安装 systemd 服务：**

   ```bash
   sudo cp deploy/linux/llm-proxy.service /etc/systemd/system/llm-proxy.service
   sudo systemctl daemon-reload
   ```

6. **启用并启动服务：**

   ```bash
   sudo systemctl enable llm-proxy
   sudo systemctl start llm-proxy
   ```

---

## 安装目录结构

```
/opt/llm-proxy/
├── proxy              # 二进制文件
├── config.yaml        # 配置文件
└── logs/              # 日志目录（可选）
```

### 文件权限

| 文件 | 权限 | 所有者 |
|------|------|--------|
| proxy | 755 (可执行) | llm-proxy:llm-proxy |
| config.yaml | 644 (只读) | llm-proxy:llm-proxy |
| logs/ | 755 | llm-proxy:llm-proxy |

---

## 服务管理

### 查看服务状态

```bash
sudo systemctl status llm-proxy
```

输出示例：
```
● llm-proxy.service - LLM Proxy Service
     Loaded: loaded (/etc/systemd/system/llm-proxy.service; enabled)
     Active: active (running) since Thu 2026-04-16 10:00:00 CST
```

### 启动/停止/重启

```bash
# 启动服务
sudo systemctl start llm-proxy

# 停止服务
sudo systemctl stop llm-proxy

# 重启服务
sudo systemctl restart llm-proxy

# 重新加载配置（不断开连接）
sudo systemctl reload llm-proxy
```

### 开机自启

```bash
# 启用开机自启
sudo systemctl enable llm-proxy

# 禁用开机自启
sudo systemctl disable llm-proxy

# 检查自启状态
sudo systemctl is-enabled llm-proxy
```

---

## 日志查看

### 实时日志

```bash
sudo journalctl -u llm-proxy -f
```

### 查看最近 100 行

```bash
sudo journalctl -u llm-proxy -n 100
```

### 查看今日日志

```bash
sudo journalctl -u llm-proxy --since today
```

### 查看特定时间段

```bash
# 从指定时间开始
sudo journalctl -u llm-proxy --since "2026-04-16 09:00:00"

# 到指定时间结束
sudo journalctl -u llm-proxy --until "2026-04-16 10:00:00"
```

### 按日志级别过滤

```bash
# 仅查看错误日志
sudo journalctl -u llm-proxy -p err

# 查看警告及以上
sudo journalctl -u llm-proxy -p warning
```

### JSON 格式输出（适合机器解析）

```bash
sudo journalctl -u llm-proxy -o json
```

---

## 配置修改

1. **编辑配置文件：**

   ```bash
   sudo nano /opt/llm-proxy/config.yaml
   ```

2. **验证配置语法（可选）：**

   重启服务前可先验证配置是否正确。

3. **重启服务使配置生效：**

   ```bash
   sudo systemctl restart llm-proxy
   ```

4. **检查服务状态：**

   ```bash
   sudo systemctl status llm-proxy
   ```

---

## 卸载方法

### 一键卸载

```bash
# 下载安装脚本
wget https://raw.githubusercontent.com/acshmily/llm-proxy/main/deploy/linux/uninstall.sh

# 运行卸载
sudo bash uninstall.sh
```

### 手动卸载

1. **停止服务：**

   ```bash
   sudo systemctl stop llm-proxy
   sudo systemctl disable llm-proxy
   ```

2. **移除服务文件：**

   ```bash
   sudo rm /etc/systemd/system/llm-proxy.service
   sudo systemctl daemon-reload
   ```

3. **删除安装目录（可选保留配置）：**

   ```bash
   # 完全删除
   sudo rm -rf /opt/llm-proxy

   # 或仅删除二进制文件，保留配置
   sudo rm /opt/llm-proxy/proxy
   ```

4. **删除用户和组（可选）：**

   ```bash
   sudo userdel -r llm-proxy
   sudo groupdel llm-proxy
   ```

---

## 故障排查

### 服务无法启动

**症状：** `systemctl start llm-proxy` 失败

**排查步骤：**

1. 查看详细错误日志：
   ```bash
   sudo journalctl -u llm-proxy -n 50 --no-pager
   ```

2. 检查配置文件语法：
   ```bash
   cat /opt/llm-proxy/config.yaml
   ```

3. 检查端口是否被占用：
   ```bash
   sudo lsof -i :8080
   ```

4. 手动运行测试：
   ```bash
   sudo -u llm-proxy /opt/llm-proxy/proxy -config /opt/llm-proxy/config.yaml
   ```

### 服务启动后立即退出

**可能原因：**
- 配置文件路径错误
- API 密钥格式错误
- 端口被占用
- 权限不足

**解决方法：**
```bash
# 查看完整日志
sudo journalctl -u llm-proxy -n 100 --no-pager

# 检查文件权限
ls -la /opt/llm-proxy/

# 修复权限
sudo chown -R llm-proxy:llm-proxy /opt/llm-proxy
```

### 无法访问服务

**症状：** 无法访问 `http://localhost:8080`

**排查步骤：**

1. 检查服务状态：
   ```bash
   sudo systemctl is-active llm-proxy
   ```

2. 检查防火墙：
   ```bash
   sudo firewall-cmd --list-ports
   sudo ufw status
   ```

3. 开放端口（如需要）：
   ```bash
   # Ubuntu/Debian
   sudo ufw allow 8080/tcp

   # CentOS/RHEL
   sudo firewall-cmd --permanent --add-port=8080/tcp
   sudo firewall-cmd --reload
   ```

### 日志级别调整

如需更详细的日志，修改配置文件：

```yaml
logging:
  format: json
  level: debug  # 改为 debug 获取详细日志
```

然后重启服务：

```bash
sudo systemctl restart llm-proxy
```

---

## 系统要求

| 要求 | 说明 |
|------|------|
| **systemd** | v215+ |
| **操作系统** | Ubuntu 16.04+, Debian 8+, CentOS 7+, Rocky Linux, AlmaLinux, Fedora |
| **架构** | linux/amd64 (x86_64), linux/arm64 (aarch64) |
| **内存** | 最低 128MB，推荐 512MB+ |
| **磁盘** | 最低 50MB，推荐 100MB+ |

---

## 相关文档

- [主项目 README](https://github.com/acshmily/llm-proxy)
- [Docker 部署指南](../docker/README.md)
- [Nginx 反向代理配置](../nginx/README-SSL.md)
- [WebSocket 隧道客户端](../../docs/ws-client-guide.md)

---

## 支持

遇到问题？

1. 查看 [GitHub Issues](https://github.com/acshmily/llm-proxy/issues)
2. 查看 [故障排查](#故障排查) 章节
3. 提交新 Issue 并附上日志输出
