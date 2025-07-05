package melody

import (
	"github.com/olahol/melody"
	"time"
)

var globalClient *melody.Melody

func New() (*melody.Melody, error) {
	if globalClient != nil {
		return globalClient, nil
	}

	m := melody.New()

	m.Config = &melody.Config{
		WriteWait:                 10 * time.Second,
		PongWait:                  60 * time.Second,
		PingPeriod:                54 * time.Second,
		ConcurrentMessageHandling: true,
		MaxMessageSize:            int64(32 * 1024),
		MessageBufferSize:         1024,
	}

	globalClient = m

	return globalClient, nil
}
