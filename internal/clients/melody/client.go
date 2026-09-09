package melody

import (
	"net/http"
	"strings"
	"time"

	"github.com/olahol/melody"

	envVars "github.com/robbiebyrd/indri/internal/repo/env"
)

var melodyClient *melody.Melody

func New() *melody.Melody {
	if melodyClient != nil {
		return melodyClient
	}

	vars := envVars.GetEnv()

	melodyClient = melody.New()

	melodyClient.Config = &melody.Config{
		WriteWait:                 time.Duration(vars.WSWriteTimeout) * time.Second,
		PongWait:                  time.Duration(vars.WSPongTimeoutSeconds) * time.Second,
		PingPeriod:                time.Duration(vars.WSPingPeriodSeconds) * time.Second,
		ConcurrentMessageHandling: false,
		MaxMessageSize:            int64(vars.WSMaxMessageSizeBytes),
		MessageBufferSize:         vars.WSMessageBufferSize,
	}

	melodyClient.Upgrader.CheckOrigin = originChecker(vars.AllowedOrigins)

	return melodyClient
}

// originChecker guards the WebSocket upgrade against Cross-Site WebSocket
// Hijacking. Requests without an Origin header (native apps, CLI tools,
// server-to-server) are allowed, since CSWSH is a browser-only attack.
// Browser requests are allowed only if their Origin is in the configured
// allowlist; an empty allowlist therefore rejects all cross-origin browsers.
func originChecker(allowedOrigins string) func(*http.Request) bool {
	allowed := make(map[string]struct{})

	for _, o := range strings.Split(allowedOrigins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = struct{}{}
		}
	}

	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}

		_, ok := allowed[origin]

		return ok
	}
}
