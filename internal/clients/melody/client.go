package melody

import (
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

	melodyClient := melody.New()

	melodyClient.Config = &melody.Config{
		WriteWait:                 time.Duration(vars.WSWriteTimeout) * time.Second,
		PongWait:                  time.Duration(vars.WSPongTimeoutSeconds) * time.Second,
		PingPeriod:                time.Duration(vars.WSPingPeriodSeconds) * time.Second,
		ConcurrentMessageHandling: false,
		MaxMessageSize:            int64(vars.WSMessageBufferSize),
		MessageBufferSize:         vars.WSMessageBufferSize,
	}

	return melodyClient
}
