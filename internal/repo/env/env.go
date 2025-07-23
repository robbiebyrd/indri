package env

import (
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"log"
)

type Vars struct {
	ListenAddress     string `envconfig:"LISTEN_ADDRESS" default:"localhost"`
	ListenPort        int    `envconfig:"LISTEN_PORT" default:"5002"`
	RedisHost         string `envconfig:"REDIS_HOST" default:"localhost"`
	RedisPort         int    `envconfig:"REDIS_PORT" default:"6379"`
	RedisPassword     string `envconfig:"REDIS_PASSWORD" default:""`
	RedisDatabase     int    `envconfig:"REDIS_DATABASE" default:"0"`
	MongoHost         string `envconfig:"MONGO_HOST" default:"localhost"`
	MongoPort         int    `envconfig:"MONGO_PORT" default:"27017"`
	MongoUsername     string `envconfig:"MONGO_USERNAME" default:""`
	MongoPassword     string `envconfig:"MONGO_PASSWORD" default:""`
	MongoDatabase     string `envconfig:"MONGO_DATABASE" default:"indri"`
	MongoAuthDatabase string `envconfig:"MONGO_AUTH_DATABASE" default:"admin"`
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
