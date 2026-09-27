package boot

import (
	"errors"
	"reflect"
	"testing"

	"github.com/robbiebyrd/indri/internal/services/events"
)

func TestBroadcastPayload(t *testing.T) {
	keyframe := events.KeyframeWrapper{SV: "v1", Game: map[string]interface{}{"code": "C1"}}
	loads := 0
	load := func(id string) (events.KeyframeWrapper, error) {
		loads++
		if id != "g1" {
			t.Errorf("loaded keyframe for %q, want g1", id)
		}
		return keyframe, nil
	}

	delta := events.ChangeEvent{ID: "g1", OperationType: events.OpUpdate}
	if got, err := broadcastPayload(delta, load); err != nil || !reflect.DeepEqual(got, any(delta)) || loads != 0 {
		t.Fatalf("delta: got %v, %v after %d loads; want the event itself, unloaded", got, err, loads)
	}

	got, err := broadcastPayload(events.ChangeEvent{ID: "g1", OperationType: events.OpKeyframe}, load)
	if err != nil || loads != 1 {
		t.Fatalf("keyframe request: err %v after %d loads", err, loads)
	}
	if kf, ok := got.(events.KeyframeWrapper); !ok || kf.SV != "v1" {
		t.Fatalf("keyframe request broadcast %#v, want the fresh keyframe", got)
	}

	failing := func(string) (events.KeyframeWrapper, error) { return events.KeyframeWrapper{}, errors.New("gone") }
	if _, err := broadcastPayload(events.ChangeEvent{ID: "g1", OperationType: events.OpKeyframe}, failing); err == nil {
		t.Fatal("a failed keyframe load was not reported")
	}
}
