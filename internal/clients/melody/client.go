package melody

import (
	"github.com/olahol/melody"
)

var globalClient *melody.Melody

func New() (*melody.Melody, error) {
	if globalClient != nil {
		return globalClient, nil
	}

	m := melody.New()

	m.Config = &melody.Config{
		ConcurrentMessageHandling: true,
		MaxMessageSize:             int64(32 * 1024),
		MessageBufferSize:          1024,
	}

	globalClient = m

	return globalClient, nil
}

