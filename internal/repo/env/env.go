package env

import (
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"log"
)

type Vars struct {
	ListenAddress     string `envconfig:"LISTENADDRESS" default:"localhost"`
	ListenPort        int    `envconfig:"LISTENPORT" default:"5002"`
	RedisHost         string `envconfig:"REDISHOST" default:"localhost"`
	RedisPort         int    `envconfig:"REDISPORT" default:"6379"`
	RedisPassword     string `envconfig:"REDISPASSWORD" default:""`
	RedisDatabase     int    `envconfig:"REDISDATABASE" default:"0"`
	MongoHost         string `envconfig:"MONGOHOST" default:"localhost"`
	MongoPort         int    `envconfig:"MONGOPORT" default:"27017"`
	MongoUsername     string `envconfig:"MONGOUSERNAME" default:""`
	MongoPassword     string `envconfig:"MONGOPASSWORD" default:""`
	MongoDatabase     string `envconfig:"MONGODATABASE" default:"indri"`
	MongoAuthDatabase string `envconfig:"MONGOAUTHDATABASE" default:"admin"`
}

var loadedVars *Vars

func GetEnv() *Vars {
	if loadedVars != nil {
		return loadedVars
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

	loadedVars = &v

	return &v
}
