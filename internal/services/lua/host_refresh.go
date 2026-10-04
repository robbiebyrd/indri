package lua

import (
	"context"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/handlers/actions"
)

// Keyframes renders and sends whole-game keyframes: the sanitized game, as a
// client rebuilds its state from.
//
// It is narrow, and set on the engine at boot, for the same reason Dispatch is:
// the game and broadcast services are what implement it, and the lua package
// does not import them. A nil one makes indri.refresh and indri.refreshSelf
// refuse rather than silently send nothing.
type Keyframes interface {
	// Keyframe renders game gameID as the caller would receive it.
	Keyframe(gameID string) ([]byte, error)

	// BroadcastKeyframe sends that keyframe to every player in the game.
	BroadcastKeyframe(gameID string) error
}

// refreshEffect is one indri.refresh (everyone) or indri.refreshSelf (the
// caller only).
//
// It is an effect like any other, so it is delivered after the write the
// handler made has committed and only once per commit: the keyframe it sends is
// rendered then, from the stored game, and shows the write. A refresh asked for
// in a write that failed is dropped with it.
type refreshEffect struct {
	gameID    string
	everyone  bool
	keyframes Keyframes
}

func (r refreshEffect) deliver(_ context.Context, res *actions.Result) error {
	if r.everyone {
		return r.keyframes.BroadcastKeyframe(r.gameID)
	}

	frame, err := r.keyframes.Keyframe(r.gameID)
	if err != nil {
		return err
	}

	res.Responses = append(res.Responses, frame)

	return nil
}

// hostRefresh is indri.refresh(): send every player in the game a full
// keyframe, once the handler's write has committed.
func hostRefresh(L *lua.LState) int {
	queueRefresh(L, "indri.refresh", true)

	return 0
}

// hostRefreshSelf is indri.refreshSelf(): answer the caller alone with a full
// keyframe, once the handler's write has committed.
func hostRefreshSelf(L *lua.LState) int {
	queueRefresh(L, "indri.refreshSelf", false)

	return 0
}

// queueRefresh queues a refresh, once: a handler that asks twice — in a helper
// and its caller, or on every retry of a mutate — sends one keyframe, since a
// second would only repeat the first.
func queueRefresh(L *lua.LState, name string, everyone bool) {
	inv, err := currentInvocation(L)
	if err != nil {
		L.RaiseError("%s: %s", name, err.Error())
	}

	if inv.gameID == "" {
		L.RaiseError("%s: this call has no game to refresh", name)
	}

	if inv.keyframes == nil {
		L.RaiseError("%s: this engine was built without a way to send keyframes", name)
	}

	alreadyQueued := inv.effects.count(func(e effect) bool {
		r, ok := e.(refreshEffect)

		return ok && r.everyone == everyone
	})
	if alreadyQueued > 0 {
		return
	}

	if err := queueEffect(L, refreshEffect{gameID: inv.gameID, everyone: everyone, keyframes: inv.keyframes}); err != nil {
		L.RaiseError("%s: %s", name, err.Error())
	}
}
