package main

import (
	"flag"
	"fmt"
	"os"
	"log"

	"github.com/claude-projetc/proxy-gemini-go/internal/config"
	"github.com/claude-projetc/proxy-gemini-go/internal/logger"
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

	// TODO: Initialize router and start server
	fmt.Println("Proxy initialized successfully")
	os.Exit(0)
}
