package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/claude-projetc/llm-proxy/internal/wsclient"
)

func main() {
	// 命令行参数解析
	var (
		configPath string
		server     string
		listen     string
		showHelp   bool
	)

	flag.StringVar(&configPath, "config", "", "配置文件路径")
	flag.StringVar(&server, "server", "", "WebSocket 服务器地址 (覆盖配置文件)")
	flag.StringVar(&listen, "listen", "", "HTTP 代理监听地址 (覆盖配置文件)")
	flag.BoolVar(&showHelp, "help", false, "显示帮助信息")
	flag.BoolVar(&showHelp, "h", false, "显示帮助信息")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "WebSocket 隧道客户端 - 本地 HTTP 代理服务器\n\n")
		fmt.Fprintf(os.Stderr, "使用方法:\n")
		fmt.Fprintf(os.Stderr, "  ws-client --config client-config.yaml\n")
		fmt.Fprintf(os.Stderr, "  ws-client --server ws://localhost:8080/ws-tunnel --listen :8081\n\n")
		fmt.Fprintf(os.Stderr, "选项:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n环境变量:\n")
		fmt.Fprintf(os.Stderr, "  WS_TUNNEL_SERVER       WebSocket 服务器地址\n")
		fmt.Fprintf(os.Stderr, "  WS_TUNNEL_LISTEN       HTTP 代理监听地址\n")
		fmt.Fprintf(os.Stderr, "  WS_TUNNEL_PING_INTERVAL_MS  心跳间隔 (毫秒)\n")
		fmt.Fprintf(os.Stderr, "  WS_TUNNEL_HEALTH_ENABLED    健康检查开关\n")
		fmt.Fprintf(os.Stderr, "  WS_TUNNEL_HEALTH_ADDRESS    健康检查监听地址\n")
	}

	flag.Parse()

	if showHelp {
		flag.Usage()
		os.Exit(0)
	}

	// 加载配置
	cfg, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("加载配置失败：%v", err)
	}

	// 命令行参数覆盖配置文件
	if server != "" {
		cfg.Server.Address = server
	}
	if listen != "" {
		cfg.Listen.Address = listen
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		log.Fatalf("配置验证失败：%v", err)
	}

	// 创建隧道
	pingInterval := cfg.Server.PingInterval()
	tunnel := wsclient.NewTunnel(cfg.Server.Address, pingInterval)

	// 创建代理服务器
	proxy := wsclient.NewProxyServer(tunnel)

	// 创建 HTTP 服务器
	httpServer := &http.Server{
		Addr:    cfg.Listen.Address,
		Handler: proxy,
	}

	// 启动健康检查服务器
	var healthServer *http.Server
	if cfg.Health.Enabled {
		checker := wsclient.NewHealthChecker(tunnel, cfg.Server.Address)
		healthServer = &http.Server{
			Addr:    cfg.Health.Address,
			Handler: checker,
		}

		go func() {
			log.Printf("健康检查端点监听 %s", cfg.Health.Address)
			if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("健康检查服务器错误：%v", err)
			}
		}()
	}

	// 优雅关闭
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动 HTTP 代理服务器
	go func() {
		log.Printf("HTTP 代理服务器监听 %s", cfg.Listen.Address)
		log.Printf("WebSocket 隧道连接 %s", cfg.Server.Address)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务器错误：%v", err)
		}
	}()

	// 等待关闭信号
	sig := <-sigChan
	log.Printf("收到信号 %v，开始关闭...", sig)

	// 创建关闭上下文
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 关闭 HTTP 服务器
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("HTTP 服务器关闭错误：%v", err)
	}

	// 关闭健康检查服务器
	if healthServer != nil {
		if err := healthServer.Shutdown(ctx); err != nil {
			log.Printf("健康检查服务器关闭错误：%v", err)
		}
	}

	// 关闭隧道
	tunnel.Close()

	log.Println("WebSocket 隧道客户端已关闭")
}
