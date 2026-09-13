package events_test

import (
	"bytes"
	"encoding/json"
	"log"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/events"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
)

// The secrets below must never reach a player. They are looked for in the
// marshalled delta rather than at a known field, so a leak is caught wherever
// in the value it surfaces.
const (
	stageSecret = "stage-answer-must-not-leak"
	sceneSecret = "scene-answer-must-not-leak"
	scenePublic = "the prompt every player may see"
)

// stageWithSecrets builds a real models.Stage carrying private data at both
// levels — on the stage and on the scene inside it — because that is what the
// callers of gameRepo.UpdateField actually pass: internal/services/stage
// publishes a whole script stage (LoadFromScript) and a whole scene
// (AddScene). Hand-rolling a struct here would keep passing if models.Stage
// grew a second private store or renamed its json tag; a real one cannot.
//
// Every call returns an independent value: the maps inside a Stage are shared
// by any copy of it, so two tests holding "the same" stage would otherwise see
// each other's edits.
func stageWithSecrets() models.Stage {
	scenePrivate := map[string]interface{}{"answer": sceneSecret}
	sceneData := map[string]interface{}{"prompt": scenePublic}

	return models.Stage{
		CurrentScene: "board",
		SceneOrder:   []string{"board"},
		Scenes: map[string]models.Scene{
			"board": {PublicData: &sceneData, PrivateData: &scenePrivate},
		},
		PublicData:  map[string]interface{}{"round": 2},
		PrivateData: map[string]interface{}{"answer": stageSecret},
	}
}

// A delta value is not always the generic JSON view of a document.
// core.UpdateField publishes the raw Go value it was handed, so a struct
// reaches SanitizeDelta with its PrivateData intact. A sanitizer that
// understands only maps and slices returns such a value untouched, and the
// broadcast then shows every player what the keyframe hides.
func TestSanitizeDelta_StripsPrivateDataFromStructValues(t *testing.T) {
	scene := stageWithSecrets().Scenes["board"]

	tests := []struct {
		name  string
		value interface{}
	}{
		{name: "whole stage, as LoadFromScript publishes it", value: stageWithSecrets()},
		{name: "pointer to a stage", value: pointerTo(stageWithSecrets())},
		{name: "one scene, as AddScene publishes it", value: scene},
		{name: "pointer to a scene", value: &scene},
		{name: "map of scenes", value: stageWithSecrets().Scenes},
		{name: "slice of scenes", value: []models.Scene{scene}},
		{name: "array of scenes", value: [1]models.Scene{scene}},
		{name: "struct behind a map value", value: map[string]interface{}{"stage": stageWithSecrets()}},
		{name: "struct behind a slice element", value: []interface{}{stageWithSecrets()}},
		{name: "struct nested two levels down", value: map[string]interface{}{
			"a": map[string]interface{}{"b": []interface{}{pointerTo(stageWithSecrets())}},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated, _ := events.SanitizeDelta(map[string]interface{}{"stage": tt.value}, nil)

			got, ok := updated["stage"]
			if !ok {
				t.Fatalf("the update was dropped entirely, got %v", updated)
			}

			published := mustMarshal(t, got)

			for _, secret := range []string{stageSecret, sceneSecret} {
				if bytes.Contains(published, []byte(secret)) {
					t.Errorf("private data reached the delta: %s", published)
				}
			}

			if paths := privateDataPaths(t, got); len(paths) != 0 {
				t.Errorf("privateData survives at %v: %s", paths, published)
			}

			// The other half of the contract: sanitizing must not silence the
			// update it is sanitizing.
			if !bytes.Contains(published, []byte(scenePublic)) {
				t.Errorf("public data was stripped along with the private data: %s", published)
			}
		})
	}
}

// A sanitizer must not edit what it sanitizes. UpdateField stores the value
// first and publishes second, handing the very same Go value to both, so a
// SanitizeDelta that stripped in place would reach back into the caller's
// stage — and, on the memory store, into the stored game itself.
func TestSanitizeDelta_DoesNotModifyTheValueItWasGiven(t *testing.T) {
	stage := stageWithSecrets()
	updated := map[string]interface{}{"stage": stage}

	events.SanitizeDelta(updated, nil)

	if stage.PrivateData["answer"] != stageSecret {
		t.Errorf("the caller's stage lost its private data, got %v", stage.PrivateData)
	}

	scene := stage.Scenes["board"]
	if scene.PrivateData == nil || (*scene.PrivateData)["answer"] != sceneSecret {
		t.Errorf("the caller's scene lost its private data, got %v", scene.PrivateData)
	}

	if _, ok := updated["stage"]; !ok {
		t.Error("SanitizeDelta modified the map it was given")
	}
}

// CLAUDE.md requires GameService.Sanitize and events.SanitizeDelta to hide the
// same things: a keyframe and a delta describing the same stage must not
// disagree about what a player may see. Sanitize drops the private store on
// the stage and on every scene, so the published delta must be exactly the
// sanitized stage — no more hidden, and nothing extra shown.
func TestSanitizeDelta_AgreesWithKeyframeSanitizeForStructValues(t *testing.T) {
	// Sanitize edits the game in place and shares the scene map with any copy
	// of the stage, so the keyframe and the delta each get their own fixture.
	keyframe := (&gameService.Service{}).Sanitize(&models.Game{Stage: stageWithSecrets()})

	updated, _ := events.SanitizeDelta(map[string]interface{}{"stage": stageWithSecrets()}, nil)

	got, ok := updated["stage"]
	if !ok {
		t.Fatalf("the stage update was dropped entirely, got %v", updated)
	}

	// Compared through JSON on both sides: that is the only form in which a
	// keyframe and a delta ever meet, and it is the form the client parses.
	if want := jsonView(t, keyframe.Stage); !reflect.DeepEqual(jsonView(t, got), want) {
		t.Errorf("delta and keyframe disagree\n delta:    %s\n keyframe: %s",
			mustMarshal(t, got), mustMarshal(t, keyframe.Stage))
	}
}

// Normalizing a struct must not quietly rewrite it. The published value is
// re-marshalled on its way to the client, so it has to produce the same JSON
// the raw value would have produced — including integers a float64 cannot
// hold, which is what a nanosecond timestamp or a snowflake id looks like.
func TestSanitizeDelta_PreservesTheValueItPublishes(t *testing.T) {
	// Embedded, renamed, omitted and hidden fields are all in here on purpose:
	// they are the encoding/json rules a reflective sanitizer would have had to
	// reimplement, and any one of them is a place the two could have drifted.
	type promoted struct {
		Promoted string `json:"promoted"`
	}

	type fixture struct {
		promoted

		Big     int64             `json:"big"`
		Small   float64           `json:"small"`
		Bytes   []byte            `json:"bytes"`
		Labels  []string          `json:"labels"`
		Omitted string            `json:"omitted,omitempty"`
		Renamed string            `json:"renamed"`
		Hidden  string            `json:"-"`
		Stage   models.Stage      `json:"stage"`
		Numbers map[string]int    `json:"numbers"`
		Strings map[string]string `json:"strings"`
	}

	value := fixture{
		promoted: promoted{Promoted: "promoted"},
		Big:      math.MaxInt64,
		Small:    1.5,
		Bytes:    []byte("bytes"),
		Labels:   []string{"a", "b"},
		Renamed:  "renamed",
		Hidden:   "hidden",
		Stage:    stageWithSecrets(),
		Numbers:  map[string]int{"n": 7},
		Strings:  map[string]string{"s": "v"},
	}

	// What the raw value would have marshalled to, minus the private data the
	// sanitizer is expected to remove.
	want := jsonView(t, value)
	stage, _ := want.(map[string]interface{})["stage"].(map[string]interface{})
	delete(stage, "privateData")
	delete(stage["scenes"].(map[string]interface{})["board"].(map[string]interface{}), "privateData")

	updated, _ := events.SanitizeDelta(map[string]interface{}{"doc": value}, nil)

	got, ok := updated["doc"]
	if !ok {
		t.Fatalf("the update was dropped entirely, got %v", updated)
	}

	if !reflect.DeepEqual(jsonView(t, got), want) {
		t.Errorf("publishing rewrote the value\n got:  %s\n want: %s", mustMarshal(t, got), mustMarshal(t, want))
	}

	// jsonView compares through float64, which cannot tell MaxInt64 from
	// MaxInt64-1. The literal is the thing that must survive.
	if published := mustMarshal(t, got); !bytes.Contains(published, []byte("9223372036854775807")) {
		t.Errorf("a large integer was rounded on its way out: %s", published)
	}
}

// A value that cannot be rendered as JSON cannot be shown to be free of
// private data, so it is dropped rather than broadcast unexamined. It loses
// nothing: the delta is marshalled again on its way to the client, so such a
// value would never have reached one.
func TestSanitizeDelta_DropsAValueItCannotRead(t *testing.T) {
	type unmarshalable struct {
		Broken float64 `json:"broken"`
	}

	// The drop is reported, so the log is captured and asserted on rather than
	// left to appear as noise in the test output.
	var logged bytes.Buffer

	log.SetOutput(&logged)

	defer log.SetOutput(os.Stderr)

	updated, _ := events.SanitizeDelta(map[string]interface{}{
		"stage.data.score":   unmarshalable{Broken: math.Inf(1)},
		"stage.currentScene": "board",
	}, nil)

	if _, ok := updated["stage.data.score"]; ok {
		t.Errorf("an unreadable value was published anyway: %v", updated)
	}

	if _, ok := updated["stage.currentScene"]; !ok {
		t.Errorf("the other updates in the delta were dropped with it: %v", updated)
	}

	if !strings.Contains(logged.String(), "stage.data.score") {
		t.Errorf("the dropped update was not reported, log said %q", logged.String())
	}
}

func pointerTo[T any](v T) *T {
	return &v
}

// jsonView renders a value the way the client receives it, so two values built
// differently in Go can be compared on what they actually publish.
func jsonView(t *testing.T, value interface{}) interface{} {
	t.Helper()

	var decoded interface{}
	if err := json.Unmarshal(mustMarshal(t, value), &decoded); err != nil {
		t.Fatalf("decoding %v: %v", value, err)
	}

	return decoded
}

func mustMarshal(t *testing.T, value interface{}) []byte {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshalling %v: %v", value, err)
	}

	return raw
}

// privateDataPaths reports where a privateData key survives in a value, so a
// failure names the leak instead of only proving one exists.
func privateDataPaths(t *testing.T, value interface{}) []string {
	t.Helper()

	var (
		found []string
		walk  func(prefix string, node interface{})
	)

	walk = func(prefix string, node interface{}) {
		switch n := node.(type) {
		case map[string]interface{}:
			for key, child := range n {
				path := key
				if prefix != "" {
					path = prefix + "." + key
				}

				if key == "privateData" {
					found = append(found, path)
				}

				walk(path, child)
			}
		case []interface{}:
			for _, child := range n {
				walk(prefix, child)
			}
		}
	}

	walk("", jsonView(t, value))

	return found
}
