package stage

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/robbiebyrd/indri/internal/models"
)

// recordingStore is a sceneStore that writes nowhere and remembers the keys it
// was handed, so a test can assert both the path a write composes and that a
// rejected call never reached the store at all.
type recordingStore struct {
	game    *models.Game
	written []string
	deleted []string
}

func (r *recordingStore) Get(string) (*models.Game, error) {
	if r.game == nil {
		return nil, errors.New("no game")
	}

	return r.game, nil
}

func (r *recordingStore) UpdateField(_ string, key string, _ interface{}) error {
	r.written = append(r.written, key)

	return nil
}

func (r *recordingStore) DeleteField(_ string, key string) error {
	r.deleted = append(r.deleted, key)

	return nil
}

func (r *recordingStore) touched() []string {
	return append(append([]string{}, r.written...), r.deleted...)
}

const sceneId = "round1"

func newService(store *recordingStore) *Service {
	store.game = &models.Game{
		ID: bson.NewObjectID(),
		Stage: models.Stage{
			Scenes: map[string]models.Scene{sceneId: {}},
		},
	}

	return &Service{gameRepo: store}
}

// Every path this service builds is concatenated from a caller-supplied scene
// id and handed to UpdateField/DeleteField, which use that string twice: as the
// MongoDB update path and as the path of the delta broadcast to every player.
// A scene id holding a "." would therefore forge a segment in both — an id of
// "foo.privateData" ends the delta path in a segment literally equal to
// "privateData", which SanitizeDelta drops, so every update to that scene would
// silently disappear from the broadcast. Escaping is not an option on this
// route, because MongoDB would read the escape characters as part of the field
// name, so the id must be rejected before it is concatenated.
func TestSceneIdThatWouldForgeAPathSegmentIsRejectedBeforeAnyWrite(t *testing.T) {
	entryPoints := map[string]func(ss *Service, id string) error{
		"AddScene": func(ss *Service, id string) error {
			return ss.AddScene("game", id, &models.Scene{})
		},
		"DeleteScene": func(ss *Service, id string) error {
			return ss.DeleteScene("game", id)
		},
		"SetCurrentScene": func(ss *Service, id string) error {
			return ss.SetCurrentScene("game", id)
		},
		"UpdateScene": func(ss *Service, id string) error {
			return ss.UpdateScene("game", id, models.DataStorePublic, nil, "value")
		},
		"LoadSceneFromScript": func(ss *Service, id string) error {
			return ss.LoadSceneFromScript("game", id, nil)
		},
	}

	tests := []struct {
		name    string
		sceneId string
	}{
		{name: "a dot forges a trailing privateData segment", sceneId: "foo.privateData"},
		{name: "a dot forges a segment anywhere in the id", sceneId: "a.b"},
		{name: "a leading dot forges an empty segment", sceneId: ".privateData"},
		{name: "a trailing dot forges an empty segment", sceneId: "scene."},
		{name: "a dollar is not a legal mongo field name", sceneId: "sc$ene"},
		{name: "a leading dollar is read as an update operator", sceneId: "$set"},
		{name: "an empty id collapses a segment", sceneId: ""},
	}

	for entryPoint, call := range entryPoints {
		for _, tt := range tests {
			t.Run(entryPoint+"/"+tt.name, func(t *testing.T) {
				store := &recordingStore{}

				err := call(newService(store), tt.sceneId)
				if !errors.Is(err, ErrInvalidPathSegment) {
					t.Errorf("%s(%q) returned %v, want an error wrapping %v: a scene id"+
						" becomes one segment of the mongo update path and of the"+
						" published delta, so it may not contain a %q or a %q",
						entryPoint, tt.sceneId, err, ErrInvalidPathSegment, pathSeparator, mongoOperator)
				}

				if touched := store.touched(); len(touched) > 0 {
					t.Errorf("%s(%q) wrote %q; a rejected scene id must never reach the store",
						entryPoint, tt.sceneId, touched)
				}
			})
		}
	}
}

// The sub-path of UpdateScene is a dotted path by design — it addresses a
// nested field inside a data store — so its dots are separators, not forged
// segments. Each of its segments must still be a key MongoDB accepts.
func TestUpdateSceneRejectsAPathSegmentMongoCannotStore(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "a dollar is not a legal mongo field name", path: "widgets.sc$ore"},
		{name: "a leading dollar is read as an update operator", path: "$set"},
		{name: "an empty segment", path: "widgets..score"},
		{name: "a trailing separator leaves an empty segment", path: "widgets."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &recordingStore{}

			err := newService(store).UpdateScene("game", sceneId, models.DataStorePublic, &tt.path, "value")
			if !errors.Is(err, ErrInvalidPathSegment) {
				t.Errorf("UpdateScene(path %q) returned %v, want an error wrapping %v",
					tt.path, err, ErrInvalidPathSegment)
			}

			if touched := store.touched(); len(touched) > 0 {
				t.Errorf("UpdateScene(path %q) wrote %q; a rejected path must never reach the store",
					tt.path, touched)
			}
		})
	}
}

// The rejection must name the value it refused, so an operator reading the log
// can tell which id was at fault.
func TestRejectionNamesTheOffendingValue(t *testing.T) {
	store := &recordingStore{}

	err := newService(store).DeleteScene("game", "foo.privateData")
	if err == nil {
		t.Fatal("DeleteScene accepted a dotted scene id")
	}

	if want := `"foo.privateData"`; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not name the rejected value %s", err, want)
	}
}

// A scene id and a sub-path that cannot forge anything must compose exactly the
// path they composed before the validation was added.
func TestValidIdsComposeTheSamePathsAsBefore(t *testing.T) {
	widgetPath := "widgets.score"

	tests := []struct {
		name    string
		call    func(ss *Service) error
		written []string
		deleted []string
	}{
		{
			name:    "UpdateScene without a sub-path writes the whole data store",
			call:    func(ss *Service) error { return ss.UpdateScene("game", sceneId, models.DataStorePublic, nil, 1) },
			written: []string{"stage.scenes.round1.data"},
		},
		{
			name: "UpdateScene with a sub-path writes the nested field",
			call: func(ss *Service) error {
				return ss.UpdateScene("game", sceneId, models.DataStorePrivate, &widgetPath, 1)
			},
			written: []string{"stage.scenes.round1.privateData.widgets.score"},
		},
		{
			name:    "SetCurrentScene writes the current scene",
			call:    func(ss *Service) error { return ss.SetCurrentScene("game", sceneId) },
			written: []string{"stage.currentScene"},
		},
		{
			name:    "DeleteScene removes the scene",
			call:    func(ss *Service) error { return ss.DeleteScene("game", sceneId) },
			deleted: []string{"stage.scenes.round1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &recordingStore{}

			if err := tt.call(newService(store)); err != nil {
				t.Fatalf("call returned %v, want nil", err)
			}

			if got := fmt.Sprint(store.written); got != fmt.Sprint(tt.written) {
				t.Errorf("updated %v, want %v", store.written, tt.written)
			}

			if got := fmt.Sprint(store.deleted); got != fmt.Sprint(tt.deleted) {
				t.Errorf("deleted %v, want %v", store.deleted, tt.deleted)
			}
		})
	}
}

func TestValidateSegment(t *testing.T) {
	tests := []struct {
		name    string
		segment string
		wantErr bool
	}{
		{name: "a plain key", segment: "round1"},
		{name: "an object id", segment: "65f1c2a4b8d3e9f0a1b2c3d4"},
		{name: "a key named after a data store", segment: "privateData"},
		{name: "a key with a hyphen and an underscore", segment: "scene_1-a"},
		{name: "a key with a dot", segment: "foo.privateData", wantErr: true},
		{name: "a key with a dollar", segment: "fo$o", wantErr: true},
		{name: "a key starting with a dollar", segment: "$set", wantErr: true},
		{name: "an empty key", segment: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSegment("scene id", tt.segment)
			if gotErr := errors.Is(err, ErrInvalidPathSegment); gotErr != tt.wantErr {
				t.Errorf("validateSegment(%q) = %v, want error: %v", tt.segment, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "a single segment", path: "score"},
		{name: "a nested path", path: "widgets.score.value"},
		{name: "a dollar in a later segment", path: "widgets.$set", wantErr: true},
		{name: "an empty segment", path: "widgets..score", wantErr: true},
		{name: "an empty path", path: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePath("scene data path", tt.path)
			if gotErr := errors.Is(err, ErrInvalidPathSegment); gotErr != tt.wantErr {
				t.Errorf("validatePath(%q) = %v, want error: %v", tt.path, err, tt.wantErr)
			}
		})
	}
}
