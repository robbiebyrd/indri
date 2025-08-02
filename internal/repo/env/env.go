package env

import (
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"log"
)

type Vars struct {
	ListenAddress         string `default:"localhost" envconfig:"LISTEN_ADDRESS"`
	ListenPort            int    `default:"5002"      envconfig:"LISTEN_PORT"`
	RedisHost             string `default:"localhost" envconfig:"REDIS_HOST"`
	RedisPort             int    `default:"6379"      envconfig:"REDIS_PORT"`
	RedisPassword         string `default:""          envconfig:"REDIS_PASSWORD"`
	RedisDatabase         int    `default:"0"         envconfig:"REDIS_DATABASE"`
	MongoURI              string `default:"localhost" envconfig:"MONGO_URI"`
	MongoDatabase         string `default:"indri"     envconfig:"MONGO_DATABASE"`
	MongoAuthDatabase     string `default:"admin"     envconfig:"MONGO_AUTH_DATABASE"`
	WSWriteTimeout        int    `default:"10"        envconfig:"WS_WRITE_TIMEOUT"`
	WSPingPeriodSeconds   int    `default:"54"        envconfig:"WS_PING_PERIOD"`
	WSPongTimeoutSeconds  int    `default:"60"        envconfig:"WS_PONG_TIMEOUT"`
	WSMaxMessageSizeBytes int    `default:"32768"     envconfig:"WS_MAX_MESSAGE_SIZE"`
	WSMessageBufferSize   int    `default:"1024"      envconfig:"WS_MESSAGE_BUFFER_SIZE"`
}

var globalClient *Vars

func GetEnv() *Vars {
	if globalClient != nil {
		return globalClient
	}

	var v Vars

	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file, continuing anyway with environment variables.")
		log.Println(err)
	}

	err = envconfig.Process("indri", &v)
	if err != nil {
		log.Println("Error parsing environment variables, continuing anyway with defaults.")
		log.Println(err)
	}

	globalClient = &v

	return &v
}
