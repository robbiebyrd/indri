package http

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/robbiebyrd/indri/internal/injector"
)

type GameDataKeys map[string]interface{}

func Serve(ctx context.Context, i *injector.Injector) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		if err := i.MelodyClient.HandleRequestWithKeys(w, r, GameDataKeys{}); err != nil {
			log.Println(err)
		}
	})

	server := &http.Server{
		Addr:              i.EnvVars.ListenAddress + ":" + strconv.Itoa(i.EnvVars.ListenPort),
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// On shutdown, close the websocket hub first so the hijacked connection
	// handlers return, then let the HTTP server drain and stop accepting.
	go func() {
		<-ctx.Done()

		if !i.MelodyClient.IsClosed() {
			if err := i.MelodyClient.Close(); err != nil {
				log.Printf("error closing websocket hub during shutdown: %v", err)
			}
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("error shutting down http server: %v", err)
		}
	}()

	log.Println("Starting web server at address " + server.Addr)

	err := server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}
