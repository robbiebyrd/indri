package melody

import (
	"github.com/olahol/melody"
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

	m.Config = &melody.Config{
		WriteWait:                 10 * time.Second,
		PongWait:                  60 * time.Second,
		PingPeriod:                54 * time.Second,
		ConcurrentMessageHandling: true,
		MaxMessageSize:            int64(32 * 1024),
		MessageBufferSize:         1024,
	}

	return m
}
