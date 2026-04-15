# SSL 证书配置说明

## 方式一：Let's Encrypt（推荐）

使用 Certbot 自动获取和更新证书：

```bash
# 安装 Certbot
sudo apt-get install certbot python3-certbot-nginx

# 获取证书
sudo certbot --nginx -d your-domain.com

# 自动续期（添加到 crontab）
0 3 * * * certbot renew --quiet
```

证书文件位置：
- `/etc/letsencrypt/live/your-domain.com/fullchain.pem`
- `/etc/letsencrypt/live/your-domain.com/privkey.pem`

修改 nginx.conf 中的路径：
```nginx
ssl_certificate /etc/letsencrypt/live/your-domain.com/fullchain.pem;
ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;
```

## 方式二：自签名证书（测试用）

```bash
# 生成自签名证书
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout privkey.pem \
  -out fullchain.pem \
  -subj "/C=CN/ST=State/L=City/O=Organization/CN=your-domain.com"

# 复制到 Nginx 目录
sudo cp fullchain.pem /etc/nginx/ssl/
sudo cp privkey.pem /etc/nginx/ssl/
```

## 方式三：云服务商证书

从阿里云、腾讯云、AWS ACM 等下载证书，转换为 PEM 格式：

```bash
# 阿里云证书转换
cat your_domain_public_key.pem > fullchain.pem
cat your_domain_private_key.pem > privkey.pem
```

## 文件权限

```bash
sudo chmod 600 /etc/nginx/ssl/privkey.pem
sudo chmod 644 /etc/nginx/ssl/fullchain.pem
sudo chown root:root /etc/nginx/ssl/*.pem
```

## 验证配置

```bash
# 测试 Nginx 配置
sudo nginx -t

# 重新加载 Nginx
sudo systemctl reload nginx
```

## 验证证书

```bash
# 检查证书
openssl s_client -connect your-domain.com:443 -servername your-domain.com

# 在线验证
# https://www.ssllabs.com/ssltest/
```
