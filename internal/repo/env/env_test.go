package env

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setEnvA(t *testing.T) {
	t.Helper()
	t.Setenv("INDRI_LISTEN_ADDRESS", "0.0.0.0")
	t.Setenv("INDRI_LISTEN_PORT", "6001")
	t.Setenv("INDRI_REDIS_HOST", "redis-host-A")
	t.Setenv("INDRI_REDIS_PORT", "6380")
	t.Setenv("INDRI_REDIS_PASSWORD", "secretA")
	t.Setenv("INDRI_REDIS_DATABASE", "2")
	t.Setenv("INDRI_MONGO_URI", "mongodb://mongo-a:27017")
	t.Setenv("INDRI_MONGO_DATABASE", "dbA")
	t.Setenv("INDRI_MONGO_AUTH_DATABASE", "adminA")
	t.Setenv("INDRI_WS_WRITE_TIMEOUT", "15")
	t.Setenv("INDRI_WS_PING_PERIOD", "30")
	t.Setenv("INDRI_WS_PONG_TIMEOUT", "40")
	t.Setenv("INDRI_WS_MAX_MESSAGE_SIZE", "65536")
	t.Setenv("INDRI_WS_MESSAGE_BUFFER_SIZE", "2048")
}

func setEnvB(t *testing.T) {
	t.Helper()
	t.Setenv("INDRI_LISTEN_ADDRESS", "127.0.0.1")
	t.Setenv("INDRI_LISTEN_PORT", "7002")
	t.Setenv("INDRI_REDIS_HOST", "redis-host-B")
	t.Setenv("INDRI_REDIS_PORT", "6390")
	t.Setenv("INDRI_REDIS_PASSWORD", "secretB")
	t.Setenv("INDRI_REDIS_DATABASE", "5")
	t.Setenv("INDRI_MONGO_URI", "mongodb://mongo-b:27017")
	t.Setenv("INDRI_MONGO_DATABASE", "dbB")
	t.Setenv("INDRI_MONGO_AUTH_DATABASE", "adminB")
	t.Setenv("INDRI_WS_WRITE_TIMEOUT", "20")
	t.Setenv("INDRI_WS_PING_PERIOD", "35")
	t.Setenv("INDRI_WS_PONG_TIMEOUT", "50")
	t.Setenv("INDRI_WS_MAX_MESSAGE_SIZE", "131072")
	t.Setenv("INDRI_WS_MESSAGE_BUFFER_SIZE", "4096")
}

func assertVarsEqual(t *testing.T, v *Vars, want Vars) {
	t.Helper()
	if v.ListenAddress != want.ListenAddress {
		t.Fatalf("ListenAddress = %q, want %q", v.ListenAddress, want.ListenAddress)
	}
	if v.ListenPort != want.ListenPort {
		t.Fatalf("ListenPort = %d, want %d", v.ListenPort, want.ListenPort)
	}
	if v.RedisHost != want.RedisHost {
		t.Fatalf("RedisHost = %q, want %q", v.RedisHost, want.RedisHost)
	}
	if v.RedisPort != want.RedisPort {
		t.Fatalf("RedisPort = %d, want %d", v.RedisPort, want.RedisPort)
	}
	if v.RedisPassword != want.RedisPassword {
		t.Fatalf("RedisPassword = %q, want %q", v.RedisPassword, want.RedisPassword)
	}
	if v.RedisDatabase != want.RedisDatabase {
		t.Fatalf("RedisDatabase = %d, want %d", v.RedisDatabase, want.RedisDatabase)
	}
	if v.MongoURI != want.MongoURI {
		t.Fatalf("MongoURI = %q, want %q", v.MongoURI, want.MongoURI)
	}
	if v.MongoDatabase != want.MongoDatabase {
		t.Fatalf("MongoDatabase = %q, want %q", v.MongoDatabase, want.MongoDatabase)
	}
	if v.MongoAuthDatabase != want.MongoAuthDatabase {
		t.Fatalf("MongoAuthDatabase = %q, want %q", v.MongoAuthDatabase, want.MongoAuthDatabase)
	}
	if v.WSWriteTimeout != want.WSWriteTimeout {
		t.Fatalf("WSWriteTimeout = %d, want %d", v.WSWriteTimeout, want.WSWriteTimeout)
	}
	if v.WSPingPeriodSeconds != want.WSPingPeriodSeconds {
		t.Fatalf("WSPingPeriodSeconds = %d, want %d", v.WSPingPeriodSeconds, want.WSPingPeriodSeconds)
	}
	if v.WSPongTimeoutSeconds != want.WSPongTimeoutSeconds {
		t.Fatalf("WSPongTimeoutSeconds = %d, want %d", v.WSPongTimeoutSeconds, want.WSPongTimeoutSeconds)
	}
	if v.WSMaxMessageSizeBytes != want.WSMaxMessageSizeBytes {
		t.Fatalf("WSMaxMessageSizeBytes = %d, want %d", v.WSMaxMessageSizeBytes, want.WSMaxMessageSizeBytes)
	}
	if v.WSMessageBufferSize != want.WSMessageBufferSize {
		t.Fatalf("WSMessageBufferSize = %d, want %d", v.WSMessageBufferSize, want.WSMessageBufferSize)
	}
}

func TestGetEnv_UsesEnvironmentVariables(t *testing.T) {
	// Ensure clean cache
	globalClient = nil

	setEnvA(t)

	got := GetEnv()
	want := Vars{
		ListenAddress:         "0.0.0.0",
		ListenPort:            6001,
		RedisHost:             "redis-host-A",
		RedisPort:             6380,
		RedisPassword:         "secretA",
		RedisDatabase:         2,
		MongoURI:              "mongodb://mongo-a:27017",
		MongoDatabase:         "dbA",
		MongoAuthDatabase:     "adminA",
		WSWriteTimeout:        15,
		WSPingPeriodSeconds:   30,
		WSPongTimeoutSeconds:  40,
		WSMaxMessageSizeBytes: 65536,
		WSMessageBufferSize:   2048,
	}
	assertVarsEqual(t, got, want)
}

func TestGetEnv_CachesResult(t *testing.T) {
	// Ensure clean cache
	globalClient = nil

	setEnvA(t)
	first := GetEnv()

	// Change environment; cached result should remain the same
	setEnvB(t)
	second := GetEnv()

	if first != second {
		t.Fatalf("expected cached pointer to be reused; got different pointers")
	}

	// Ensure values did not change after environment changed
	want := Vars{
		ListenAddress:         "0.0.0.0",
		ListenPort:            6001,
		RedisHost:             "redis-host-A",
		RedisPort:             6380,
		RedisPassword:         "secretA",
		RedisDatabase:         2,
		MongoURI:              "mongodb://mongo-a:27017",
		MongoDatabase:         "dbA",
		MongoAuthDatabase:     "adminA",
		WSWriteTimeout:        15,
		WSPingPeriodSeconds:   30,
		WSPongTimeoutSeconds:  40,
		WSMaxMessageSizeBytes: 65536,
		WSMessageBufferSize:   2048,
	}
	assertVarsEqual(t, second, want)
}

// clearEnvVars unsets all INDRI_ env vars for the duration of the test and
// restores them on cleanup.
func clearEnvVars(t *testing.T) {
	t.Helper()
	keys := []string{
		"INDRI_LISTEN_ADDRESS", "INDRI_LISTEN_PORT",
		"INDRI_ALLOWED_ORIGINS",
		"INDRI_REDIS_HOST", "INDRI_REDIS_PORT",
		"INDRI_REDIS_PASSWORD", "INDRI_REDIS_DATABASE",
		"INDRI_LOCK_BACKEND",
		"INDRI_MONGO_URI", "INDRI_MONGO_DATABASE", "INDRI_MONGO_AUTH_DATABASE",
		"INDRI_WS_WRITE_TIMEOUT", "INDRI_WS_PING_PERIOD",
		"INDRI_WS_PONG_TIMEOUT", "INDRI_WS_MAX_MESSAGE_SIZE",
		"INDRI_WS_MESSAGE_BUFFER_SIZE",
		"INDRI_TRANSPORTS", "INDRI_WEBRTC_ICE_SERVERS", "INDRI_WEBRTC_MAX_PEERS",
		"INDRI_WEBRTC_NAT_1TO1_IPS", "INDRI_WEBRTC_UDP_PORT_MIN", "INDRI_WEBRTC_UDP_PORT_MAX",
		"INDRI_WEBRTC_GATHER_TIMEOUT", "INDRI_WEBRTC_OPEN_TIMEOUT", "INDRI_GRAPHQL_INIT_TIMEOUT",
	}
	saved := make(map[string]string, len(keys))
	wasSet := make(map[string]bool, len(keys))
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			saved[k] = v
			wasSet[k] = true
		}
		os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for _, k := range keys {
			if wasSet[k] {
				os.Setenv(k, saved[k])
			} else {
				os.Unsetenv(k)
			}
		}
	})
}

// writeJSONConfig writes a JSON file to a temp dir and returns its path.
func writeJSONConfig(t *testing.T, data map[string]any) string {
	t.Helper()
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal JSON config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatalf("write JSON config: %v", err)
	}
	return path
}

func TestLoad_JSONFallback(t *testing.T) {
	globalClient = nil
	clearEnvVars(t)

	configPath := writeJSONConfig(t, map[string]any{
		"listenAddress": "1.2.3.4",
		"listenPort":    9000,
		"mongoUri":      "mongodb://json-host:27017",
	})

	got, err := Load(configPath, nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.ListenAddress != "1.2.3.4" {
		t.Errorf("ListenAddress = %q, want %q", got.ListenAddress, "1.2.3.4")
	}
	if got.ListenPort != 9000 {
		t.Errorf("ListenPort = %d, want %d", got.ListenPort, 9000)
	}
	if got.MongoURI != "mongodb://json-host:27017" {
		t.Errorf("MongoURI = %q, want %q", got.MongoURI, "mongodb://json-host:27017")
	}
}

func TestLoad_TransportSettings(t *testing.T) {
	t.Run("defaults keep today's WebSocket-only behavior", func(t *testing.T) {
		globalClient = nil
		clearEnvVars(t)

		got, err := Load("", nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.Transports != "ws" {
			t.Errorf("Transports = %q, want ws", got.Transports)
		}
		if got.WebRTCICEServers != "" || got.WebRTCNAT1To1IPs != "" {
			t.Errorf("ICE servers %q / NAT IPs %q, want none by default", got.WebRTCICEServers, got.WebRTCNAT1To1IPs)
		}
		if got.WebRTCMaxPeers != 256 {
			t.Errorf("WebRTCMaxPeers = %d, want 256", got.WebRTCMaxPeers)
		}
		if got.WebRTCGatherTimeoutSeconds != 5 || got.WebRTCOpenTimeoutSeconds != 15 || got.GraphQLInitTimeoutSeconds != 10 {
			t.Errorf("timeouts gather=%d open=%d graphqlInit=%d, want 5/15/10",
				got.WebRTCGatherTimeoutSeconds, got.WebRTCOpenTimeoutSeconds, got.GraphQLInitTimeoutSeconds)
		}
	})

	t.Run("config file", func(t *testing.T) {
		globalClient = nil
		clearEnvVars(t)

		path := writeJSONConfig(t, map[string]any{
			"transports":          "ws,sse",
			"webrtcIceServers":    "stun:stun.example:3478",
			"webrtcMaxPeers":      8,
			"webrtcNat1To1Ips":    "203.0.113.7",
			"webrtcUdpPortMin":    50000,
			"webrtcUdpPortMax":    50100,
			"webrtcGatherTimeout": 3,
			"webrtcOpenTimeout":   20,
			"graphqlInitTimeout":  7,
		})

		got, err := Load(path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.Transports != "ws,sse" || got.WebRTCICEServers != "stun:stun.example:3478" ||
			got.WebRTCMaxPeers != 8 || got.WebRTCNAT1To1IPs != "203.0.113.7" ||
			got.WebRTCUDPPortMin != 50000 || got.WebRTCUDPPortMax != 50100 ||
			got.WebRTCGatherTimeoutSeconds != 3 || got.WebRTCOpenTimeoutSeconds != 20 || got.GraphQLInitTimeoutSeconds != 7 {
			t.Errorf("config file not applied: %+v", got)
		}
	})

	t.Run("environment", func(t *testing.T) {
		globalClient = nil
		clearEnvVars(t)
		t.Setenv("INDRI_TRANSPORTS", "graphqlws,webrtc")
		t.Setenv("INDRI_WEBRTC_MAX_PEERS", "64")
		t.Setenv("INDRI_WEBRTC_GATHER_TIMEOUT", "4")
		t.Setenv("INDRI_WEBRTC_OPEN_TIMEOUT", "30")
		t.Setenv("INDRI_GRAPHQL_INIT_TIMEOUT", "6")

		got, err := Load("", nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.Transports != "graphqlws,webrtc" || got.WebRTCMaxPeers != 64 ||
			got.WebRTCGatherTimeoutSeconds != 4 || got.WebRTCOpenTimeoutSeconds != 30 || got.GraphQLInitTimeoutSeconds != 6 {
			t.Errorf("environment not applied: %+v", got)
		}
	})

	t.Run("command-line flags", func(t *testing.T) {
		globalClient = nil
		clearEnvVars(t)

		got, err := Load("", map[string]string{
			"transports":            "sse",
			"webrtc-max-peers":      "32",
			"webrtc-gather-timeout": "2",
			"webrtc-open-timeout":   "25",
			"graphql-init-timeout":  "8",
		})
		if err != nil {
			t.Fatal(err)
		}
		if got.Transports != "sse" || got.WebRTCMaxPeers != 32 ||
			got.WebRTCGatherTimeoutSeconds != 2 || got.WebRTCOpenTimeoutSeconds != 25 || got.GraphQLInitTimeoutSeconds != 8 {
			t.Errorf("flags not applied: %+v", got)
		}
	})
}

func TestLoad_EnvOverridesJSON(t *testing.T) {
	globalClient = nil
	clearEnvVars(t)
	t.Setenv("INDRI_LISTEN_ADDRESS", "env-host")

	configPath := writeJSONConfig(t, map[string]any{
		"listenAddress": "json-host",
	})

	got, err := Load(configPath, nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.ListenAddress != "env-host" {
		t.Errorf("ListenAddress = %q, want env-host (env should win over JSON)", got.ListenAddress)
	}
}

func TestLoad_CLIOverridesEnv(t *testing.T) {
	globalClient = nil
	clearEnvVars(t)
	t.Setenv("INDRI_LISTEN_ADDRESS", "env-host")

	got, err := Load("", map[string]string{"listen-address": "cli-host"})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.ListenAddress != "cli-host" {
		t.Errorf("ListenAddress = %q, want cli-host (CLI should win over env)", got.ListenAddress)
	}
}

func TestLoad_CLIOverridesJSON(t *testing.T) {
	globalClient = nil
	clearEnvVars(t)

	configPath := writeJSONConfig(t, map[string]any{
		"listenAddress": "json-host",
	})

	got, err := Load(configPath, map[string]string{"listen-address": "cli-host"})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.ListenAddress != "cli-host" {
		t.Errorf("ListenAddress = %q, want cli-host (CLI should win over JSON)", got.ListenAddress)
	}
}

func TestLoad_MissingJSONFile(t *testing.T) {
	globalClient = nil

	_, err := Load("/nonexistent/path/config.json", nil)
	if err == nil {
		t.Fatal("expected error for missing JSON config file, got nil")
	}
}

func TestGetEnv_ResetCache_PicksUpNewEnvironment(t *testing.T) {
	// Load with initial environment
	globalClient = nil
	setEnvA(t)
	_ = GetEnv()

	// Reset cache and change environment
	globalClient = nil
	setEnvB(t)
	got := GetEnv()

	want := Vars{
		ListenAddress:         "127.0.0.1",
		ListenPort:            7002,
		RedisHost:             "redis-host-B",
		RedisPort:             6390,
		RedisPassword:         "secretB",
		RedisDatabase:         5,
		MongoURI:              "mongodb://mongo-b:27017",
		MongoDatabase:         "dbB",
		MongoAuthDatabase:     "adminB",
		WSWriteTimeout:        20,
		WSPingPeriodSeconds:   35,
		WSPongTimeoutSeconds:  50,
		WSMaxMessageSizeBytes: 131072,
		WSMessageBufferSize:   4096,
	}
	assertVarsEqual(t, got, want)
}
