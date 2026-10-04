package env

import (
	"fmt"
	"log"
	"reflect"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Vars struct {
	ListenAddress         string `default:"localhost" envconfig:"LISTEN_ADDRESS"`
	ListenPort            int    `default:"5002"      envconfig:"LISTEN_PORT"`
	AllowedOrigins        string `default:""          envconfig:"ALLOWED_ORIGINS"`
	RedisHost             string `default:"localhost" envconfig:"REDIS_HOST"`
	RedisPort             int    `default:"6379"      envconfig:"REDIS_PORT"`
	RedisPassword         string `default:""          envconfig:"REDIS_PASSWORD"`
	RedisDatabase         int    `default:"0"         envconfig:"REDIS_DATABASE"`
	LockBackend           string `default:"inprocess" envconfig:"LOCK_BACKEND"`
	DBBackend             string `default:"mongodb"   envconfig:"DB_BACKEND"`
	SQLitePath            string `default:"./indri.db" envconfig:"SQLITE_PATH"`
	PostgresURI           string `default:""          envconfig:"POSTGRES_URI"`
	MongoURI              string `default:"localhost" envconfig:"MONGO_URI"`
	MongoDatabase         string `default:"indri"     envconfig:"MONGO_DATABASE"`
	MongoAuthDatabase     string `default:"admin"     envconfig:"MONGO_AUTH_DATABASE"`
	WSWriteTimeout        int    `default:"10"        envconfig:"WS_WRITE_TIMEOUT"`
	WSPingPeriodSeconds   int    `default:"54"        envconfig:"WS_PING_PERIOD"`
	WSPongTimeoutSeconds  int    `default:"60"        envconfig:"WS_PONG_TIMEOUT"`
	WSMaxMessageSizeBytes int    `default:"32768"     envconfig:"WS_MAX_MESSAGE_SIZE"`
	WSMessageBufferSize   int    `default:"1024"      envconfig:"WS_MESSAGE_BUFFER_SIZE"`
	RTCUDPPort            int    `default:"8443"      envconfig:"RTC_UDP_PORT"`
	RTCNAT1To1IPs         string `default:""          envconfig:"RTC_NAT_1TO1_IPS"`
	RTCMaxPeers           int    `default:"1000"      envconfig:"RTC_MAX_PEERS"`
	RTCPendingTTLSeconds  int    `default:"30"        envconfig:"RTC_PENDING_TTL"`
}

var globalClient *Vars

// GetEnv returns the settings, loading them from the environment (and a .env
// file) on first use. A server that takes flags calls Load first.
func GetEnv() *Vars {
	if globalClient != nil {
		return globalClient
	}

	v, err := Load(nil)
	if err != nil {
		log.Printf("Error applying settings, continuing with what loaded: %v", err)
	}

	return v
}

// Load reads the settings from, in rising priority: their defaults, the
// environment (and a .env file), then overrides — command-line values keyed
// by flag name. It replaces the settings GetEnv returns.
func Load(overrides map[string]string) (*Vars, error) {
	var v Vars

	if err := godotenv.Load(); err != nil {
		log.Println("Error loading .env file, continuing anyway with environment variables.")
		log.Println(err)
	}

	if err := envconfig.Process("indri", &v); err != nil {
		log.Println("Error parsing environment variables, continuing anyway with defaults.")
		log.Println(err)
	}

	globalClient = &v

	return &v, applyOverrides(&v, overrides)
}

// Setting is one configurable value: its environment variable, the flag that
// overrides it, and its default.
type Setting struct {
	EnvVar  string
	Flag    string
	Default string
	Kind    reflect.Kind

	field int
}

// Settings lists every setting. A flag's name is derived from its environment
// variable — INDRI_LISTEN_PORT is -listen-port — so a new setting has a flag
// without anyone remembering to add one.
func Settings() []Setting {
	t := reflect.TypeFor[Vars]()
	settings := make([]Setting, 0, t.NumField())

	for i := range t.NumField() {
		field := t.Field(i)

		name := field.Tag.Get("envconfig")
		if name == "" {
			continue
		}

		settings = append(settings, Setting{
			EnvVar:  "INDRI_" + name,
			Flag:    strings.ReplaceAll(strings.ToLower(name), "_", "-"),
			Default: field.Tag.Get("default"),
			Kind:    field.Type.Kind(),
			field:   i,
		})
	}

	return settings
}

// applyOverrides sets each overridden setting on v from its string form.
func applyOverrides(v *Vars, overrides map[string]string) error {
	if len(overrides) == 0 {
		return nil
	}

	byFlag := map[string]Setting{}
	for _, s := range Settings() {
		byFlag[s.Flag] = s
	}

	target := reflect.ValueOf(v).Elem()

	for flag, value := range overrides {
		s, ok := byFlag[flag]
		if !ok {
			return fmt.Errorf("no setting has the flag -%s", flag)
		}

		field := target.Field(s.field)

		switch s.Kind {
		case reflect.String:
			field.SetString(value)
		case reflect.Int:
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("-%s must be a whole number, got %q", flag, value)
			}

			field.SetInt(int64(n))
		case reflect.Bool:
			b, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("-%s must be true or false, got %q", flag, value)
			}

			field.SetBool(b)
		default:
			return fmt.Errorf("-%s has a type flags cannot set", flag)
		}
	}

	return nil
}
