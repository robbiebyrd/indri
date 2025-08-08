package env

import (
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
