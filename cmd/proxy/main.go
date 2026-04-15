package main

import (
	"flag"
	"os"
	"log"
	"net/http"
	"github.com/claude-projetc/llm-proxy/internal/config"
	"github.com/claude-projetc/llm-proxy/internal/logger"
	"github.com/claude-projetc/llm-proxy/internal/router"
	"github.com/claude-projetc/llm-proxy/internal/server"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	logFormat := logger.TEXT
	if cfg.Logging.Format == "json" {
		logFormat = logger.JSON
	}

	logLevel := logger.INFO
	switch cfg.Logging.Level {
	case "debug":
		logLevel = logger.DEBUG
	case "warn":
		logLevel = logger.WARN
	case "error":
		logLevel = logger.ERROR
	}

	log := logger.New(logFormat, logLevel)
	log.Info("Starting Anthropic Protocol Proxy",
		logger.LogField{Key: "listen", Value: cfg.Server.Listen})

	// 初始化路由和服务器
	r := router.New(cfg.Routes)
	srv := server.New(cfg, r, log)

	log.Info("Listening", logger.LogField{Key: "address", Value: cfg.Server.Listen})
	if err := http.ListenAndServe(cfg.Server.Listen, srv); err != nil {
		log.Error("Server failed", logger.LogField{Key: "error", Value: err.Error()})
		os.Exit(1)
	}
}
