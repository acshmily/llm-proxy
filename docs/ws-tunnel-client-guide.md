# WebSocket 隧道客户端使用示例

本文档说明如何使用 WebSocket 隧道客户端代理 HTTP 请求。

## 服务端配置

首先在服务端配置中启用 WebSocket 隧道：

```yaml
protection:
  enabled: true
  traffic_obfuscation:
    websocket_tunnel:
      enabled: true
      path: "/ws-tunnel"
      ping_interval_ms: 30000
```

## Go 客户端示例

### 方法 1：使用标准库 + gorilla/websocket（推荐）

```go
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// WSTunnelTransport 实现 http.RoundTripper
type WSTunnelTransport struct {
	conn *websocket.Conn
}

func NewWSTunnelTransport(serverURL string) (*WSTunnelTransport, error) {
	dialer := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	
	conn, _, err := dialer.Dial(serverURL, nil)
	if err != nil {
		return nil, err
	}
	
	return &WSTunnelTransport{conn: conn}, nil
}

func (t *WSTunnelTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 读取请求体
	bodyBytes, _ := io.ReadAll(req.Body)
	req.Body.Close()
	
	// 构建 WebSocket 消息
	msg := map[string]interface{}{
		"type": "request",
		"data": map[string]interface{}{
			"method":  req.Method,
			"path":    req.URL.RequestURI(),
			"headers": req.Header,
			"body":    base64.StdEncoding.EncodeToString(bodyBytes),
		},
	}
	
	data, _ := json.Marshal(msg)
	t.conn.WriteMessage(websocket.TextMessage, data)
	
	// 等待响应
	_, respData, err := t.conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	
	// 解析响应
	var response struct {
		Type string `json:"type"`
		Data struct {
			Status  int    `json:"status"`
			Body    string `json:"body"`
			Headers map[string]string `json:"headers"`
		} `json:"data"`
	}
	json.Unmarshal(respData, &response)
	
	// 构建 HTTP 响应
	bodyBytes, _ = base64.StdEncoding.DecodeString(response.Data.Body)
	
	return &http.Response{
		StatusCode: response.Data.Status,
		Header: http.Header(response.Data.Headers),
		Body: io.NopCloser(bytes.NewReader(bodyBytes)),
	}, nil
}

// 使用示例
func main() {
	transport, err := NewWSTunnelTransport("ws://localhost:8080/ws-tunnel")
	if err != nil {
		log.Fatal(err)
	}
	defer transport.conn.Close()
	
	client := &http.Client{Transport: transport}
	
	resp, err := client.Get("http://backend-api/v1/messages")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	
	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))
}
```

### 方法 2：使用标准库 net 包（无依赖）

```go
package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// SimpleWSClient 简化的 WebSocket 客户端
type SimpleWSClient struct {
	conn net.Conn
}

func NewSimpleWSClient(serverURL string) (*SimpleWSClient, error) {
	// 解析 URL
	host := "localhost:8080"
	path := "/ws-tunnel"
	
	// 建立 TCP 连接
	conn, err := net.Dial("tcp", host)
	if err != nil {
		return nil, err
	}
	
	// 发送 WebSocket 握手
	key := "dGhlIHNhbXBsZSBub25jZQ=="
	request := fmt.Sprintf(
		"GET %s HTTP/1.1\r\n"+
		"Host: %s\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Key: %s\r\n"+
		"Sec-WebSocket-Version: 13\r\n"+
		"\r\n",
		path, host, key,
	)
	
	conn.Write([]byte(request))
	
	// 读取响应
	reader := bufio.NewReader(conn)
	statusLine, _ := reader.ReadString('\n')
	if !strings.Contains(statusLine, "101") {
		return nil, fmt.Errorf("handshake failed: %s", statusLine)
	}
	
	// 读取 headers
	for {
		line, _ := reader.ReadString('\n')
		if line == "\r\n" {
			break
		}
	}
	
	return &SimpleWSClient{conn: conn}, nil
}

func (c *SimpleWSClient) SendRequest(req *http.Request) (*http.Response, error) {
	// 读取请求体
	bodyBytes, _ := io.ReadAll(req.Body)
	req.Body.Close()
	
	// 构建消息
	msg := map[string]interface{}{
		"type": "request",
		"data": map[string]interface{}{
			"method":  req.Method,
			"path":    req.URL.RequestURI(),
			"headers": req.Header,
			"body":    base64.StdEncoding.EncodeToString(bodyBytes),
		},
	}
	
	data, _ := json.Marshal(msg)
	
	// 发送 WebSocket 帧
	c.sendFrame(data)
	
	// 接收响应
	respData := c.readFrame()
	
	// 解析响应
	var response map[string]interface{}
	json.Unmarshal(respData, &response)
	
	data := response["data"].(map[string]interface{})
	status := int(data["status"].(float64))
	bodyStr := data["body"].(string)
	
	bodyBytes, _ = base64.StdEncoding.DecodeString(bodyStr)
	
	return &http.Response{
		StatusCode: status,
		Body: io.NopCloser(bytes.NewReader(bodyBytes)),
	}, nil
}

func (c *SimpleWSClient) sendFrame(data []byte) {
	// 文本帧，客户端需要 mask
	frame := make([]byte, 0, 10+len(data))
	frame = append(frame, 0x81) // FIN=1, opcode=1
	
	// Mask + length
	maskKey := []byte{0x12, 0x34, 0x56, 0x78}
	frame = append(frame, 0x80|byte(len(data)))
	frame = append(frame, maskKey...)
	
	// Mask data
	for i := range data {
		data[i] ^= maskKey[i%4]
	}
	frame = append(frame, data...)
	
	c.conn.Write(frame)
}

func (c *SimpleWSClient) readFrame() []byte {
	header := make([]byte, 2)
	c.conn.Read(header)
	
	payloadLen := int(header[1] & 0x7F)
	if payloadLen == 126 {
		extLen := make([]byte, 2)
		c.conn.Read(extLen)
		payloadLen = int(binary.BigEndian.Uint16(extLen))
	}
	
	payload := make([]byte, payloadLen)
	c.conn.Read(payload)
	
	return payload
}

func (c *SimpleWSClient) Close() error {
	return c.conn.Close()
}

// 使用示例
func main() {
	client, err := NewSimpleWSClient("ws://localhost:8080/ws-tunnel")
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	
	req, _ := http.NewRequest("GET", "http://backend-api/v1/messages", nil)
	resp, err := client.SendRequest(req)
	if err != nil {
		log.Fatal(err)
	}
	
	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))
}
```

## JavaScript/Node.js 客户端示例

```javascript
const WebSocket = require('ws');

class WSTunnelClient {
	constructor(serverUrl) {
		this.ws = new WebSocket(serverUrl);
		this.requestId = 0;
		this.pendingRequests = new Map();
		
		this.ws.on('message', (data) => {
			const response = JSON.parse(data);
			const pending = this.pendingRequests.get(response.id);
			if (pending) {
				pending.resolve(response);
				this.pendingRequests.delete(response.id);
			}
		});
	}
	
	async request(options) {
		return new Promise((resolve, reject) => {
			const id = ++this.requestId;
			
			this.ws.send(JSON.stringify({
				id,
				type: 'request',
				data: {
					method: options.method || 'GET',
					path: options.path,
					headers: options.headers || {},
					body: options.body ? Buffer.from(options.body).toString('base64') : ''
				}
			}));
			
			this.pendingRequests.set(id, { resolve, reject });
			
			// 超时处理
			setTimeout(() => {
				if (this.pendingRequests.has(id)) {
					reject(new Error('Request timeout'));
					this.pendingRequests.delete(id);
				}
			}, 30000);
		});
	}
	
	async get(path, headers) {
		return this.request({ method: 'GET', path, headers });
	}
	
	async post(path, body, headers) {
		return this.request({ method: 'POST', path, body, headers });
	}
	
	close() {
		this.ws.close();
	}
}

// 使用示例
async function main() {
	const client = new WSTunnelClient('ws://localhost:8080/ws-tunnel');
	
	try {
		const response = await client.post('/v1/messages', {
			model: 'claude-3',
			messages: [{ role: 'user', content: 'Hello' }]
		});
		
		console.log('Status:', response.data.status);
		console.log('Body:', Buffer.from(response.data.body, 'base64').toString());
	} catch (err) {
		console.error('Request failed:', err);
	} finally {
		client.close();
	}
}

main();
```

## Python 客户端示例

```python
import websocket
import json
import base64
import threading
import time

class WSTunnelClient:
    def __init__(self, server_url):
        self.ws = websocket.create_connection(server_url)
        self.responses = {}
        self.lock = threading.Lock()
        
        # 启动接收线程
        threading.Thread(target=self._receive_loop, daemon=True).start()
    
    def _receive_loop(self):
        while True:
            try:
                data = self.ws.recv()
                response = json.loads(data)
                req_id = response.get('id')
                if req_id:
                    with self.lock:
                        self.responses[req_id] = response
            except:
                break
    
    def request(self, method, path, headers=None, body=None):
        import uuid
        req_id = str(uuid.uuid4())
        
        message = {
            'id': req_id,
            'type': 'request',
            'data': {
                'method': method,
                'path': path,
                'headers': headers or {},
                'body': base64.b64encode(body).decode() if body else ''
            }
        }
        
        self.ws.send(json.dumps(message))
        
        # 等待响应（带超时）
        for _ in range(300):  # 30 秒超时
            with self.lock:
                if req_id in self.responses:
                    response = self.responses.pop(req_id)
                    return response
            time.sleep(0.1)
        
        raise TimeoutError('Request timeout')
    
    def get(self, path, headers=None):
        return self.request('GET', path, headers)
    
    def post(self, path, body, headers=None):
        return self.request('POST', path, headers, body)
    
    def close(self):
        self.ws.close()

# 使用示例
if __name__ == '__main__':
    client = WSTunnelClient('ws://localhost:8080/ws-tunnel')
    
    try:
        response = client.post(
            '/v1/messages',
            body=json.dumps({
                'model': 'claude-3',
                'messages': [{'role': 'user', 'content': 'Hello'}]
            }).encode()
        )
        
        print(f"Status: {response['data']['status']}")
        body = base64.b64decode(response['data']['body'])
        print(f"Body: {body.decode()}")
    finally:
        client.close()
```

## 集成到现有 HTTP 客户端

### Go 集成

```go
// 创建 WebSocket 隧道传输
transport, err := NewWSTunnelTransport("ws://localhost:8080/ws-tunnel")
if err != nil {
    log.Fatal(err)
}

// 创建 HTTP 客户端
client := &http.Client{
    Transport: transport,
    Timeout:   60 * time.Second,
}

// 正常使用
resp, err := client.Get("http://your-backend-api/v1/messages")
```

### 注意事项

1. **连接管理**：WebSocket 连接是长连接，需要妥善管理重连
2. **心跳**：配置 ping_interval_ms 保持连接活跃
3. **并发**：单个 WebSocket 连接是串行的，高并发需要多个连接
4. **错误处理**：WebSocket 断开时需要重新建立连接

## 完整工作流程

```
客户端                      WebSocket 隧道                    后端服务
  |                            |                                |
  |-- WebSocket 连接 ---------->|                                |
  |                            |                                |
  |-- HTTP 请求 (封装) -------->|                                |
  |                            |-- 解封装 ---------------------->|
  |                            |                                |
  |                            |<-- HTTP 响应 ------------------|
  |<-- HTTP 响应 (封装) --------|                                |
  |                            |                                |
```

## 安全建议

1. 使用 WSS（WebSocket Secure）代替 WS
2. 验证服务器证书
3. 添加认证令牌到 WebSocket 握手头部
4. 限制 WebSocket 连接的来源
