package melody

import (
	"github.com/olahol/melody"
	"github.com/robbiebyrd/indri/internal/repo/env"
	"time"
)

/*
New returns a singleton instance of a configured Melody client.
It ensures only one Melody client is created and reused throughout the application.

Returns:

	*melody.Melody: A pointer to the singleton Melody client instance.
*/
func New() *melody.Melody {
	m := melody.New()

	envVars := env.GetEnv()

	m.Config = &melody.Config{
		WriteWait:                 time.Duration(envVars.WSWriteTimeout) * time.Second,
		PongWait:                  time.Duration(envVars.WSPongTimeoutSeconds) * time.Second,
		PingPeriod:                time.Duration(envVars.WSPingPeriodSeconds) * time.Second,
		ConcurrentMessageHandling: true,
		MaxMessageSize:            int64(envVars.WSMessageBufferSize),
		MessageBufferSize:         envVars.WSMessageBufferSize,
	}

	return m
}
