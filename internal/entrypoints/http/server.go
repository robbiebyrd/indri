package http

import (
	"github.com/olahol/melody"
	"github.com/robbiebyrd/indri/internal/repo/env"
	"log"
	"net/http"
	"strconv"
	"time"
)

type GameDataKeys map[string]interface{}

func Serve(m *melody.Melody) error {

	envVars := env.GetEnv()

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		err := m.HandleRequestWithKeys(w, r, GameDataKeys{})
		if err != nil {
			log.Println(err)
		}
	})

	server := &http.Server{
		Addr:              envVars.ListenAddress + ":" + strconv.Itoa(envVars.ListenPort),
		ReadHeaderTimeout: 3 * time.Second,
	}

	log.Println("Starting web server at address " + envVars.ListenAddress +
		":" + strconv.Itoa(envVars.ListenPort))

	err := server.ListenAndServe()
	if err != nil {
		return err
	}

	return nil
}
