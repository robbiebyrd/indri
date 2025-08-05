package http

import (
	"github.com/olahol/melody"
	envVars "github.com/robbiebyrd/indri/internal/repo/env"
	"log"
	"net/http"
	"strconv"
	"time"
)

type GameDataKeys map[string]interface{}

func Serve(m *melody.Melody) error {

	vars := envVars.GetEnv()

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		err := m.HandleRequestWithKeys(w, r, GameDataKeys{})
		if err != nil {
			log.Println(err)
		}
	})

	server := &http.Server{
		Addr:              vars.ListenAddress + ":" + strconv.Itoa(vars.ListenPort),
		ReadHeaderTimeout: 3 * time.Second,
	}

	log.Println("Starting web server at address " + vars.ListenAddress +
		":" + strconv.Itoa(vars.ListenPort))

	err := server.ListenAndServe()
	if err != nil {
		return err
	}

	return nil
}
