package stage

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/robbiebyrd/indri/internal/models"
	gameService "github.com/robbiebyrd/indri/internal/services/game"
)

// recordingStore is a sceneStore that writes nowhere and remembers the keys and
// values it was handed, so a test can assert the path a write composes, what it
// would have stored there, and that a rejected call never reached the store at
// all.
type recordingStore struct {
	game    *models.Game
	written []string
	values  []interface{}
	deleted []string
}

func (r *recordingStore) Get(string) (*models.Game, error) {
	if r.game == nil {
		return nil, errors.New("no game")
	}

	return r.game, nil
}

func (r *recordingStore) UpdateField(_ string, key string, value interface{}) error {
	r.written = append(r.written, key)
	r.values = append(r.values, value)

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

// scriptScene is the scene the booted script declares, and the value
// LoadSceneFromScript copies into the game.
var scriptScene = models.Scene{
	PublicData:  &map[string]interface{}{"prompt": "who goes first?"},
	PrivateData: &map[string]interface{}{"answer": "the host"},
}

func newService(store *recordingStore) *Service {
	store.game = &models.Game{
		ID: bson.NewObjectID(),
		Stage: models.Stage{
			Scenes: map[string]models.Scene{sceneId: {}},
		},
	}

	return &Service{
		gameRepo: store,
		gameService: &gameService.Service{
			Script: &models.Script{
				Stage: models.Stage{Scenes: map[string]models.Scene{sceneId: scriptScene}},
			},
		},
	}
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

// A write is only worth anything if the value can be read back. The store takes
// the key it is given verbatim, so a key naming a field the model does not
// declare succeeds, publishes a delta, and stores something no read will ever
// see again — LoadSceneFromScript wrote "stage.scene.<id>" (singular) where the
// bson tag is "scenes", so every scene it loaded vanished on the next read.
// Applying the write the way MongoDB would and decoding the result back into
// models.Game is what tells the two apart.
func TestASceneLoadedFromTheScriptIsReadableBackThroughTheModel(t *testing.T) {
	public := models.DataStorePublic

	tests := []struct {
		name     string
		dataType *models.DataStoreType
		wantPath string
	}{
		{
			name:     "the whole scene",
			wantPath: "stage.scenes.round1",
		},
		{
			name:     "only the public data store",
			dataType: &public,
			wantPath: "stage.scenes.round1.data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &recordingStore{}

			if err := newService(store).LoadSceneFromScript("game", sceneId, tt.dataType); err != nil {
				t.Fatalf("LoadSceneFromScript returned %v, want nil", err)
			}

			if len(store.written) != 1 {
				t.Fatalf("wrote %q, want exactly one write", store.written)
			}

			if store.written[0] != tt.wantPath {
				t.Errorf("wrote to %q, want the %q path every other method in the file builds",
					store.written[0], tt.wantPath)
			}

			g := applyUpdate(t, store.written[0], store.values[0])

			scene, ok := g.Stage.Scenes[sceneId]
			if !ok {
				t.Fatalf("after writing %q the game has scenes %v; the loaded scene is not readable"+
					" back through models.Stage, so the path names a field the model does not declare",
					store.written[0], g.Stage.Scenes)
			}

			if scene.PublicData == nil {
				t.Fatalf("scene %q read back without its public data", sceneId)
			}

			if got := (*scene.PublicData)["prompt"]; got != (*scriptScene.PublicData)["prompt"] {
				t.Errorf("read back public data %v, want %v", *scene.PublicData, *scriptScene.PublicData)
			}
		})
	}
}

// applyUpdate does to an empty game document what MongoDB does with the key
// UpdateField is handed: it sets a dotted path, creating the documents along
// the way. Decoding the result into models.Game is the assertion that matters —
// bson stores a field the model does not declare just as happily, and drops it
// on the way back.
func applyUpdate(t *testing.T, path string, value interface{}) *models.Game {
	t.Helper()

	segments := strings.Split(path, pathSeparator)

	doc := bson.M{}
	at := doc

	for _, segment := range segments[:len(segments)-1] {
		next := bson.M{}
		at[segment] = next
		at = next
	}

	at[segments[len(segments)-1]] = value

	raw, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("marshalling %v: %v", doc, err)
	}

	g := &models.Game{}
	if err := bson.Unmarshal(raw, g); err != nil {
		t.Fatalf("unmarshalling %v: %v", doc, err)
	}

	return g
}

// Every key this package composes is used twice: as the MongoDB update path and
// as the path of the published delta. A key that names no field on the model is
// wrong in both directions at once — it stores data no read returns, and it
// tells every client to apply a change to a field it does not have. Two such
// keys shipped here, "stage.scene.<id>" and "stage.scriptId", and neither could
// fail a test that only compared strings, so the rule is checked against the
// models themselves.
//
// The scenes' private data store is not exercised: models.Scene tags it
// "private_data" while models.DataStoreType spells it "privateData", a third
// instance of this same defect that is outside this change and filed on its own.
func TestEveryPathThisPackageWritesNamesAFieldTheModelsDeclare(t *testing.T) {
	public := models.DataStorePublic
	widgetPath := "widgets.score"

	calls := map[string]func(ss *Service) error{
		"AddScene": func(ss *Service) error {
			return ss.AddScene("game", "round2", &models.Scene{})
		},
		"AddScenes": func(ss *Service) error {
			return ss.AddScenes("game", map[string]models.Scene{"round2": {}})
		},
		"DeleteScene": func(ss *Service) error {
			return ss.DeleteScene("game", sceneId)
		},
		"SetCurrentScene": func(ss *Service) error {
			return ss.SetCurrentScene("game", sceneId)
		},
		"SetSceneOrder": func(ss *Service) error {
			return ss.SetSceneOrder("game", []string{sceneId})
		},
		"UpdateScene": func(ss *Service) error {
			return ss.UpdateScene("game", sceneId, public, nil, 1)
		},
		"UpdateScene with a sub-path": func(ss *Service) error {
			return ss.UpdateScene("game", sceneId, public, &widgetPath, 1)
		},
		"LoadFromScript": func(ss *Service) error {
			return ss.LoadFromScript("game", "script")
		},
		"LoadSceneFromScript": func(ss *Service) error {
			return ss.LoadSceneFromScript("game", sceneId, nil)
		},
		"LoadSceneFromScript of one data store": func(ss *Service) error {
			return ss.LoadSceneFromScript("game", sceneId, &public)
		},
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			store := &recordingStore{}

			if err := call(newService(store)); err != nil {
				t.Fatalf("%s returned %v, want nil", name, err)
			}

			touched := store.touched()
			if len(touched) == 0 {
				t.Fatalf("%s wrote nothing", name)
			}

			for _, path := range touched {
				if !declaresPath(reflect.TypeOf(models.Game{}), path) {
					t.Errorf("%s addresses %q, which names no field on the models;"+
						" a write there stores data nothing can read back and publishes"+
						" a delta no client can apply", name, path)
				}
			}
		})
	}
}

// declaresPath reports whether a dotted MongoDB path addresses a field the
// models declare. It resolves each segment the way the bson codec does: against
// a struct the segment must be a declared bson tag; against a map it is a key,
// so it is free and resolution continues against the value type; anything
// deeper than that — inside a data store's interface{}, or inside a slice — is
// opaque to the model and accepted.
func declaresPath(typ reflect.Type, path string) bool {
	for _, segment := range strings.Split(path, pathSeparator) {
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}

		switch typ.Kind() {
		case reflect.Struct:
			field, ok := bsonField(typ, segment)
			if !ok {
				return false
			}

			typ = field
		case reflect.Map:
			typ = typ.Elem()
		default:
			return true
		}
	}

	return true
}

// bsonField returns the type of the field typ stores under the given bson name.
func bsonField(typ reflect.Type, name string) (reflect.Type, bool) {
	for i := range typ.NumField() {
		field := typ.Field(i)
		if tag, _, _ := strings.Cut(field.Tag.Get("bson"), ","); tag == name {
			return field.Type, true
		}
	}

	return nil, false
}

// declaresPath is the oracle the test above judges every write by, so it is
// worth nothing unless it can say no. Both paths this change removed are here
// as the cases that must be rejected.
func TestDeclaresPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "the whole stage", path: "stage", want: true},
		{name: "a scene", path: "stage.scenes.round1", want: true},
		{name: "a scene's public data", path: "stage.scenes.round1.data", want: true},
		{name: "a field inside a data store", path: "stage.scenes.round1.data.widgets.score", want: true},
		{name: "the current scene", path: "stage.currentScene", want: true},
		{name: "the scene order", path: "stage.sceneOrder", want: true},
		{name: "a player", path: "players.p1.host", want: true},
		{name: "a scene under the singular field name", path: "stage.scene.round1"},
		{name: "a data store under the singular field name", path: "stage.scene.round1.data"},
		{name: "a script id on the stage", path: "stage.scriptId"},
		{name: "a field no model declares", path: "stage.currentScence"},
		{name: "a stage field addressed at the top level", path: "scenes.round1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := declaresPath(reflect.TypeOf(models.Game{}), tt.path); got != tt.want {
				t.Errorf("declaresPath(%q) = %v, want %v", tt.path, got, tt.want)
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
