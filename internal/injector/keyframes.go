package injector

import (
	"fmt"

	broadcastService "github.com/robbiebyrd/indri/internal/services/broadcast"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
)

// keyframeSender is the script engine's Keyframes over the game and broadcast
// services: the sanitized game, for the caller or for everyone in it.
type keyframeSender struct {
	games     *gameService.Service
	broadcast *broadcastService.Service
}

func (k keyframeSender) Keyframe(gameID string) ([]byte, error) {
	frame, err := k.games.GetJSONBytes(gameID)
	if err != nil {
		return nil, fmt.Errorf("rendering a keyframe of game %q: %w", gameID, err)
	}

	return *frame, nil
}

func (k keyframeSender) BroadcastKeyframe(gameID string) error {
	g, err := k.games.Get(gameID)
	if err != nil {
		return err
	}

	return k.broadcast.Broadcast(&gameID, nil, k.games.Sanitize(g))
}
