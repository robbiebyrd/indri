package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	"github.com/robbiebyrd/indri/internal/services/boot"
)

func main() {
	scriptFilePath := flag.String("script", "", "A JSON file containing the default game script.")
	configFilePath := flag.String("config", "", "A JSON file containing server configuration.")

	// Server settings — override env vars and JSON config when explicitly passed.
	flag.String("listen-address", "", "Address to listen on.")
	flag.Int("listen-port", 0, "Port to listen on.")
	flag.String("allowed-origins", "", "Comma-separated allowed WebSocket origins.")
	flag.String("redis-host", "", "Redis server host.")
	flag.Int("redis-port", 0, "Redis server port.")
	flag.String("redis-password", "", "Redis server password.")
	flag.Int("redis-database", 0, "Redis database number.")
	flag.String("lock-backend", "", "Lock backend: inprocess or redis.")
	flag.String("mongo-uri", "", "MongoDB URI.")
	flag.String("mongo-database", "", "MongoDB database name.")
	flag.String("mongo-auth-database", "", "MongoDB auth database.")
	flag.Int("ws-write-timeout", 0, "WebSocket write timeout in seconds.")
	flag.Int("ws-ping-period", 0, "WebSocket ping period in seconds.")
	flag.Int("ws-pong-timeout", 0, "WebSocket pong timeout in seconds.")
	flag.Int("ws-max-message-size", 0, "Max WebSocket message size in bytes.")
	flag.Int("ws-message-buffer-size", 0, "WebSocket message buffer size.")

	flag.Parse()

	if *scriptFilePath == "" {
		log.Fatal("a script file is required: use -script <path>")
	}

	if _, err := os.Stat(*scriptFilePath); err != nil {
		log.Fatalf("script file not found: %v", err)
	}

	configExplicit := *configFilePath != ""
	if !configExplicit {
		defaultConfig := filepath.Join(filepath.Dir(*scriptFilePath), "server.json")
		if _, err := os.Stat(defaultConfig); err == nil {
			*configFilePath = defaultConfig
		}
	}

	// Collect only flags explicitly set on the command line, skipping the
	// two non-settings flags.
	overrides := map[string]string{}
	flag.Visit(func(f *flag.Flag) {
		if f.Name != "script" && f.Name != "config" {
			overrides[f.Name] = f.Value.String()
		}
	})

	if _, err := envVars.Load(*configFilePath, overrides); err != nil {
		log.Fatalf("could not load configuration: %v", err)
	}

	// Cancel the root context on SIGINT/SIGTERM so the server can drain and
	// shut down cleanly instead of being killed mid-write.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	i, err := boot.Boot(ctx, scriptFilePath)
	if err != nil {
		log.Fatalf("could not bootstrap: %v", err)
	}

	if err := boot.Serve(i); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}
