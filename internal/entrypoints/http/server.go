package http

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/robbiebyrd/indri/internal/injector"
)

type GameDataKeys map[string]interface{}

func Serve(i *injector.Injector) error {
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		err := i.MelodyClient.HandleRequestWithKeys(w, r, GameDataKeys{})
		if err != nil {
			log.Println(err)
		}
	})

	server := &http.Server{
		Addr:              i.EnvVars.ListenAddress + ":" + strconv.Itoa(i.EnvVars.ListenPort),
		ReadHeaderTimeout: 3 * time.Second,
	}

	log.Println("Starting web server at address " + i.EnvVars.ListenAddress +
		":" + strconv.Itoa(i.EnvVars.ListenPort))

	err := server.ListenAndServe()
	if err != nil {
		return err
	}

	return nil
}
