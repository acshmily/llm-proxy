# llm-proxy 部署说明

## 快速部署（Docker Compose）

### 1. 准备配置文件

```bash
# 复制示例配置
cp config.example.yaml config.yaml

# 编辑配置填入 API 密钥
vim config.yaml
```

### 2. 准备 SSL 证书

**方式 A：使用 Let's Encrypt（推荐）**

```bash
# 安装 Certbot
sudo apt-get install certbot

# 获取证书
sudo certbot certonly --standalone -d your-domain.com

# 复制证书到部署目录
sudo cp /etc/letsencrypt/live/your-domain.com/fullchain.pem deploy/nginx/ssl/
sudo cp /etc/letsencrypt/live/your-domain.com/privkey.pem deploy/nginx/ssl/
```

**方式 B：自签名证书（测试用）**

```bash
cd deploy/nginx/ssl
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout privkey.pem \
  -out fullchain.pem \
  -subj "/CN=your-domain.com"
```

### 3. 修改 Nginx 配置

编辑 `deploy/nginx/nginx.conf`：
- 将 `your-domain.com` 替换为你的域名
- 确认 SSL 证书路径正确

### 4. 启动服务

```bash
cd deploy/docker

# 启动所有服务
docker-compose up -d

# 查看日志
docker-compose logs -f

# 检查状态
docker-compose ps
```

### 5. 验证部署

```bash
# 测试健康检查
curl https://your-domain.com/health

# 预期输出：{"status":"healthy","time":"2026-04-15T12:00:00Z"}
```

---

## 其他部署方式

### Linux systemd 服务

将 llm-proxy 安装为系统服务（无需 Docker）：

**一键安装：**
```bash
curl -fsSL https://raw.githubusercontent.com/acshmily/llm-proxy/main/deploy/linux/install.sh | sudo bash
```

详细文档：[../linux/README.md](../linux/README.md)

---

## 手动部署（systemd）

### 1. 安装依赖

```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install -y nginx openssl

# CentOS/RHEL
sudo yum install -y nginx openssl
```

### 2. 配置 Nginx

```bash
# 复制配置文件
sudo cp deploy/nginx/nginx.conf /etc/nginx/nginx.conf

# 创建 SSL 目录
sudo mkdir -p /etc/nginx/ssl

# 复制证书
sudo cp deploy/nginx/ssl/*.pem /etc/nginx/ssl/

# 测试配置
sudo nginx -t

# 启动 Nginx
sudo systemctl enable nginx
sudo systemctl start nginx
```

### 3. 运行 llm-proxy

```bash
# 下载二进制
wget https://github.com/acshmily/llm-proxy/releases/latest/download/proxy-linux-amd64.tar.gz
tar -xzf proxy-linux-amd64.tar.gz
cd proxy-*-linux-amd64

# 复制配置
cp ../config.example.yaml config.yaml
vim config.yaml  # 编辑 API 密钥

# 后台运行
nohup ./proxy -config config.yaml > proxy.log 2>&1 &

# 或使用 systemd
sudo cp deploy/systemd/llm-proxy.service /etc/systemd/system/
sudo systemctl enable llm-proxy
sudo systemctl start llm-proxy
```

---

## 配置说明

### llm-proxy 配置（config.yaml）

```yaml
server:
  listen: 0.0.0.0:8080  # Docker Compose 中改为 127.0.0.1:8080

protection:
  enabled: true  # 启用防护
  
  # Nginx 已处理 TLS，这里可以关闭
  traffic_camouflage:
    tls_fingerprint:
      enabled: false
    
    # 后端请求仍需伪装
    browser_headers:
      enabled: true
      mode: "random"
  
  # 行为打散
  behavior_jitter:
    request_delay:
      enabled: true
      min_ms: 50
      max_ms: 200
    connection_reuse:
      enabled: true
      reuse_rate: 0.8
    request_padding:
      enabled: true
      max_bytes: 256
```

### Nginx 关键配置

| 配置项 | 说明 | 推荐值 |
|--------|------|--------|
| `limit_req_zone` | IP 限流 | 10r/s |
| `server_tokens` | 隐藏版本 | off |
| `ssl_protocols` | TLS 版本 | TLSv1.2 TLSv1.3 |
| `proxy_read_timeout` | 读取超时 | 60s (WebSocket: 86400s) |

---

## 常见问题

### 1. 证书权限问题

```bash
sudo chmod 600 /etc/nginx/ssl/privkey.pem
sudo chmod 644 /etc/nginx/ssl/fullchain.pem
sudo chown root:root /etc/nginx/ssl/*.pem
```

### 2. Nginx 启动失败

```bash
# 查看详细错误
sudo nginx -t
sudo journalctl -u nginx -f

# 检查端口占用
sudo lsof -i :80
sudo lsof -i :443
```

### 3. llm-proxy 连接失败

```bash
# 检查服务状态
docker-compose ps
docker-compose logs llm-proxy

# 测试本地访问
curl http://localhost:8080/health
```

### 4. Let's Encrypt 续期

```bash
# 手动续期
sudo certbot renew

# 自动续期（crontab）
0 3 * * * certbot renew --quiet
```

---

## 监控和维护

### 查看日志

```bash
# Nginx 访问日志
tail -f deploy/nginx/logs/access.log

# Nginx 错误日志
tail -f deploy/nginx/logs/error.log

# llm-proxy 日志
docker-compose logs -f llm-proxy
```

### 性能监控

```bash
# 查看连接数
docker-compose exec nginx nginx -T | grep limit_req

# 查看请求速率
watch -n 1 'docker-compose exec llm-proxy wget -q -O - http://localhost:8080/health'
```

### 备份配置

```bash
# 备份重要配置
tar -czf llm-proxy-backup-$(date +%Y%m%d).tar.gz \
  config.yaml \
  deploy/nginx/nginx.conf \
  deploy/nginx/ssl/
```
