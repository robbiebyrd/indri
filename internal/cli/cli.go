// Package cli parses the command line shared by every Indri server binary:
// -script, -config, and one flag per configuration setting.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	envVars "github.com/robbiebyrd/indri/internal/repo/env"
)

// Parse reads args into fs and loads the configuration. Settings come from, in
// rising priority: the JSON config file (-config, or server.json beside the
// script), environment variables, then flags given on the command line.
// defaultScript is used when -script is absent; if it is empty, -script is
// required. Parse returns the script path and the loaded settings.
func Parse(fs *flag.FlagSet, args []string, defaultScript string) (string, *envVars.Vars, error) {
	scriptPath := fs.String("script", defaultScript, "A JSON file containing the default game script.")
	configPath := fs.String("config", "", "A JSON file containing server configuration (default: server.json beside the script).")

	registerSettingsFlags(fs)

	if err := fs.Parse(args); err != nil {
		return "", nil, err
	}

	if *scriptPath == "" {
		return "", nil, errors.New("a script file is required: use -script <path>")
	}

	if _, err := os.Stat(*scriptPath); err != nil {
		return "", nil, fmt.Errorf("script file not found: %w", err)
	}

	if *configPath == "" {
		defaultConfig := filepath.Join(filepath.Dir(*scriptPath), "server.json")
		if _, err := os.Stat(defaultConfig); err == nil {
			*configPath = defaultConfig
		}
	}

	// Only flags given on the command line override; -script and -config are
	// not settings.
	overrides := map[string]string{}
	fs.Visit(func(f *flag.Flag) {
		if f.Name != "script" && f.Name != "config" {
			overrides[f.Name] = f.Value.String()
		}
	})

	vars, err := envVars.Load(*configPath, overrides)
	if err != nil {
		return "", nil, fmt.Errorf("loading configuration: %w", err)
	}

	return *scriptPath, vars, nil
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
