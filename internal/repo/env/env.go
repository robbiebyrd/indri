package env

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"reflect"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Vars struct {
	ListenAddress         string `default:"localhost" envconfig:"LISTEN_ADDRESS"        json:"listenAddress"        flag:"listen-address"`
	ListenPort            int    `default:"5002"      envconfig:"LISTEN_PORT"           json:"listenPort"           flag:"listen-port"`
	AllowedOrigins        string `default:""          envconfig:"ALLOWED_ORIGINS"       json:"allowedOrigins"       flag:"allowed-origins"`
	RedisHost             string `default:"localhost" envconfig:"REDIS_HOST"            json:"redisHost"            flag:"redis-host"`
	RedisPort             int    `default:"6379"      envconfig:"REDIS_PORT"            json:"redisPort"            flag:"redis-port"`
	RedisPassword         string `default:""          envconfig:"REDIS_PASSWORD"        json:"redisPassword"        flag:"redis-password"`
	RedisDatabase         int    `default:"0"         envconfig:"REDIS_DATABASE"        json:"redisDatabase"        flag:"redis-database"`
	LockBackend           string `default:"inprocess" envconfig:"LOCK_BACKEND"          json:"lockBackend"          flag:"lock-backend"`
	MongoURI              string `default:"localhost" envconfig:"MONGO_URI"             json:"mongoUri"             flag:"mongo-uri"`
	MongoDatabase         string `default:"indri"     envconfig:"MONGO_DATABASE"        json:"mongoDatabase"        flag:"mongo-database"`
	MongoAuthDatabase     string `default:"admin"     envconfig:"MONGO_AUTH_DATABASE"   json:"mongoAuthDatabase"    flag:"mongo-auth-database"`
	WSWriteTimeout        int    `default:"10"        envconfig:"WS_WRITE_TIMEOUT"      json:"wsWriteTimeout"       flag:"ws-write-timeout"`
	WSPingPeriodSeconds   int    `default:"54"        envconfig:"WS_PING_PERIOD"        json:"wsPingPeriod"         flag:"ws-ping-period"`
	WSPongTimeoutSeconds  int    `default:"60"        envconfig:"WS_PONG_TIMEOUT"       json:"wsPongTimeout"        flag:"ws-pong-timeout"`
	WSMaxMessageSizeBytes int    `default:"32768"     envconfig:"WS_MAX_MESSAGE_SIZE"   json:"wsMaxMessageSize"     flag:"ws-max-message-size"`
	WSMessageBufferSize   int    `default:"1024"      envconfig:"WS_MESSAGE_BUFFER_SIZE" json:"wsMessageBufferSize" flag:"ws-message-buffer-size"`
}

var globalClient *Vars

// GetEnv returns the cached config, initializing it from env vars on first call.
func GetEnv() *Vars {
	if globalClient != nil {
		return globalClient
	}
	v, err := load("", nil)
	if err != nil {
		log.Printf("config load error: %v", err)
	}
	return v
}

// Load initializes the global config from three sources in ascending priority:
// JSON config file < environment variables (+ .env file) < cliOverrides.
// Must be called before the first GetEnv() to take effect.
func Load(jsonConfigPath string, cliOverrides map[string]string) (*Vars, error) {
	return load(jsonConfigPath, cliOverrides)
}

func load(jsonConfigPath string, cliOverrides map[string]string) (*Vars, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, continuing with environment variables.")
	}

	if jsonConfigPath != "" {
		if err := applyJSONConfigToEnv(jsonConfigPath); err != nil {
			return nil, err
		}
	}

	var v Vars
	if err := envconfig.Process("indri", &v); err != nil {
		log.Printf("error parsing environment variables: %v", err)
	}

	applyFlagOverrides(&v, cliOverrides)

	globalClient = &v
	return &v, nil
}

// applyJSONConfigToEnv reads path and sets env vars for any key present in the
// JSON that is not already set in the environment. ENV vars already set win.
func applyJSONConfigToEnv(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config file %q: %w", path, err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parsing config file %q: %w", path, err)
	}

	t := reflect.TypeFor[Vars]()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		jsonKey := field.Tag.Get("json")
		envconfigSuffix := field.Tag.Get("envconfig")
		if jsonKey == "" || envconfigSuffix == "" {
			continue
		}
		envconfigKey := "INDRI_" + envconfigSuffix

		val, ok := raw[jsonKey]
		if !ok {
			continue
		}
		if _, alreadySet := os.LookupEnv(envconfigKey); alreadySet {
			continue
		}
		os.Setenv(envconfigKey, jsonValueToString(val))
	}
	return nil
}

func jsonValueToString(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// applyFlagOverrides sets Vars fields for entries in overrides, matched by the
// "flag" struct tag. Only flags explicitly passed on the command line should be
// in overrides (use flag.Visit to collect them).
func applyFlagOverrides(v *Vars, overrides map[string]string) {
	if len(overrides) == 0 {
		return
	}
	rv := reflect.ValueOf(v).Elem()
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		flagName := rt.Field(i).Tag.Get("flag")
		strVal, ok := overrides[flagName]
		if !ok || flagName == "" {
			continue
		}
		field := rv.Field(i)
		switch field.Kind() {
		case reflect.String:
			field.SetString(strVal)
		case reflect.Int:
			if n, err := strconv.ParseInt(strVal, 10, 64); err == nil {
				field.SetInt(n)
			}
		}
	}
}
