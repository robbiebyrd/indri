package http

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/transport"
)

func Serve(ctx context.Context, i *injector.Injector) error {
	mux := http.NewServeMux()
	i.Transport.Register(mux)

	// One origin allowlist for every route: the WebSocket upgrade, the GraphQL
	// endpoint, the SSE stream and the REST actions. Browsers need the CORS
	// response headers to use any of the HTTP transports cross-origin, and a
	// rejected origin is refused here rather than separately by each transport.
	handler := transport.OriginPolicyFromEnv().Middleware(mux)

	server := &http.Server{
		Addr:              i.EnvVars.ListenAddress + ":" + strconv.Itoa(i.EnvVars.ListenPort),
		Handler:           handler,
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// On shutdown, close the websocket hub first so the hijacked connection
	// handlers return, then let the HTTP server drain and stop accepting.
	go func() {
		<-ctx.Done()

		if !i.Transport.IsClosed() {
			if err := i.Transport.Close(); err != nil {
				log.Printf("error closing transport during shutdown: %v", err)
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
