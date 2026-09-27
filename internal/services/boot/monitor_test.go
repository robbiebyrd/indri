package boot

import (
	"errors"
	"reflect"
	"testing"

	"github.com/robbiebyrd/indri/internal/services/events"
)

// payloads is a game service stand-in that counts what it loads.
type payloads struct {
	t         *testing.T
	keyframes int
	layouts   int
	err       error
}

func (p *payloads) Keyframe(id string) (events.KeyframeWrapper, error) {
	p.keyframes++
	if id != "g1" {
		p.t.Errorf("loaded keyframe for %q, want g1", id)
	}
	return events.KeyframeWrapper{SV: "v1", Game: map[string]interface{}{"code": "C1"}}, p.err
}

func (p *payloads) LayoutFrame(id string) (events.LayoutFrame, error) {
	p.layouts++
	if id != "g1" {
		p.t.Errorf("loaded layout for %q, want g1", id)
	}
	return events.LayoutFrame{O: events.OpLayout, V: "lv1", Data: map[string]interface{}{"grid": 4}}, p.err
}

func TestBroadcastPayload(t *testing.T) {
	load := &payloads{t: t}

	delta := events.ChangeEvent{ID: "g1", OperationType: events.OpUpdate}
	if got, err := broadcastPayload(delta, load); err != nil || !reflect.DeepEqual(got, any(delta)) || load.keyframes+load.layouts != 0 {
		t.Fatalf("delta: got %v, %v; want the event itself, nothing loaded", got, err)
	}

	got, err := broadcastPayload(events.ChangeEvent{ID: "g1", OperationType: events.OpKeyframe}, load)
	if err != nil || load.keyframes != 1 {
		t.Fatalf("keyframe request: err %v after %d loads", err, load.keyframes)
	}
	if kf, ok := got.(events.KeyframeWrapper); !ok || kf.SV != "v1" {
		t.Fatalf("keyframe request broadcast %#v, want the fresh keyframe", got)
	}

	got, err = broadcastPayload(events.ChangeEvent{ID: "g1", OperationType: events.OpLayout}, load)
	if err != nil || load.layouts != 1 {
		t.Fatalf("layout event: err %v after %d loads", err, load.layouts)
	}
	if lf, ok := got.(events.LayoutFrame); !ok || lf.V != "lv1" {
		t.Fatalf("layout event broadcast %#v, want the game's layout frame", got)
	}

	failing := &payloads{t: t, err: errors.New("gone")}
	for _, op := range []events.OpCode{events.OpKeyframe, events.OpLayout} {
		if _, err := broadcastPayload(events.ChangeEvent{ID: "g1", OperationType: op}, failing); err == nil {
			t.Fatalf("a failed load for op %d was not reported", op)
		}
	}
}
