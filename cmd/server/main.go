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

	registerSettingsFlags(flag.CommandLine)

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

// registerSettingsFlags declares one flag per env.Vars field with a flag tag.
// Values given on the command line override env vars and the JSON config.
func registerSettingsFlags(fs *flag.FlagSet) {
	fs.String("listen-address", "", "Address to listen on.")
	fs.Int("listen-port", 0, "Port to listen on.")
	fs.String("allowed-origins", "", "Comma-separated browser origins allowed to connect.")
	fs.String("redis-host", "", "Redis server host.")
	fs.Int("redis-port", 0, "Redis server port.")
	fs.String("redis-password", "", "Redis server password.")
	fs.Int("redis-database", 0, "Redis database number.")
	fs.String("lock-backend", "", "Lock backend: inprocess or redis.")
	fs.String("db-backend", "", "Database backend (default mongodb).")
	fs.String("postgres-uri", "", "PostgreSQL connection URI.")
	fs.String("sqlite-path", "", "Path to SQLite database file (default ./indri.db).")
	fs.String("mongo-uri", "", "MongoDB URI.")
	fs.String("mongo-database", "", "MongoDB database name.")
	fs.String("mongo-auth-database", "", "MongoDB auth database.")
	fs.Int("ws-write-timeout", 0, "WebSocket write timeout in seconds.")
	fs.Int("ws-ping-period", 0, "Ping period in seconds (WebSocket, GraphQL, SSE keepalive).")
	fs.Int("ws-pong-timeout", 0, "Pong timeout in seconds (WebSocket, GraphQL).")
	fs.Int("ws-max-message-size", 0, "Max client message size in bytes, on every transport.")
	fs.Int("ws-message-buffer-size", 0, "Outbound messages queued per connection, on every transport.")
	fs.String("transports", "", "Comma-separated client transports: ws, sse, graphqlws, webrtc.")
	fs.String("webrtc-ice-servers", "", "Comma-separated STUN/TURN URLs for WebRTC.")
	fs.Int("webrtc-max-peers", 0, "Cap on concurrent WebRTC peer connections.")
	fs.String("webrtc-nat-1to1-ips", "", "Comma-separated public IPs to advertise for WebRTC behind 1:1 NAT.")
	fs.Int("webrtc-udp-port-min", 0, "Lowest UDP port for WebRTC (0 = any).")
	fs.Int("webrtc-udp-port-max", 0, "Highest UDP port for WebRTC (0 = any).")
	fs.Int("webrtc-gather-timeout", 0, "Seconds to gather ICE candidates when answering an offer; keep under 10.")
	fs.Int("webrtc-open-timeout", 0, "Seconds a WebRTC peer has to open its data channel.")
	fs.Int("graphql-init-timeout", 0, "Seconds a GraphQL client has to send connection_init.")
}
