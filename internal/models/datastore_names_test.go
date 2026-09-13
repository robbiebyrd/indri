package models_test

import (
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/robbiebyrd/indri/internal/models"
)

// knownDivergences lists every field in the game document whose bson name is
// allowed to differ from its json name, keyed "Type.Field". An entry is a debt,
// not a licence: the test below fails if an entry stops diverging, so a field
// that is fixed must have its entry deleted with the fix.
var knownDivergences = map[string]string{
	"Game.ID": "MongoDB owns the name \"_id\", and the id is never addressed by a" +
		" dotted path: repo/game strips _id out of every update it sends.",
	"Scene.PrivateData": "The defect this list exists to make visible. Fixing it orphans" +
		" scene private data that is already stored under \"private_data\", so it needs a" +
		" migration decision — see the record on models.Scene. Delete this entry with the fix.",
}

// A dotted path into the game document is used twice, unchanged: repo/game
// hands it to MongoDB as an update path, where it is resolved by bson tag, and
// publishes the same string as the path of the delta, which the client and
// events.SanitizeDelta read by json tag. The two uses agree only while a field
// spells its bson name and its json name the same way, and nothing else in the
// repository enforces that.
//
// Three instances of the breach have shipped already — "stage.scene.<id>",
// "stage.scriptId" and this package's "private_data" — so the rule is checked
// against the models themselves rather than trusted to review.
func TestBsonAndJsonNamesAgreeAcrossTheGameDocument(t *testing.T) {
	seen := map[string]bool{}

	for _, field := range fieldsReachableFrom(reflect.TypeOf(models.Game{}), map[reflect.Type]bool{}) {
		t.Run(field.key, func(t *testing.T) {
			seen[field.key] = true

			reason, known := knownDivergences[field.key]

			if field.bson == field.json {
				// The debt was paid. The entry must go with it, or it stays as
				// cover for the next field that takes the same name.
				if known {
					t.Errorf("knownDivergences lists %s, but its bson and json names now"+
						" agree on %q; delete the entry", field.key, field.bson)
				}

				return
			}

			if !known {
				t.Errorf("%s is bson %q but json %q; a dotted path naming it addresses the"+
					" field in MongoDB or in the delta, never both, so it stores data no read"+
					" returns or tells every client to apply a change to a field it does not have",
					field.key, field.bson, field.json)

				return
			}

			t.Logf("known divergence, bson %q vs json %q: %s", field.bson, field.json, reason)
		})
	}

	// An exception that has quietly become untrue is worse than no exception:
	// it hides the next breach behind a name that already looks handled.
	for key := range knownDivergences {
		if !seen[key] {
			t.Errorf("knownDivergences lists %q, which the game document no longer reaches;"+
				" delete the entry", key)
		}
	}
}

type taggedField struct {
	key  string
	bson string
	json string
}

// fieldsReachableFrom collects every struct field the game document can reach,
// descending through pointers, maps and slices the way the bson and json codecs
// do. Fields json hides are skipped: they never appear in a delta, so they
// cannot make a path mean two things.
func fieldsReachableFrom(typ reflect.Type, visited map[reflect.Type]bool) []taggedField {
	typ = deref(typ)
	if typ.Kind() != reflect.Struct || visited[typ] {
		return nil
	}

	visited[typ] = true

	var fields []taggedField

	for i := range typ.NumField() {
		field := typ.Field(i)

		// Neither codec stores an unexported field, so it cannot appear in a
		// path. Skipping it also stops the walk at the leaves the codecs treat
		// as opaque values, such as time.Time.
		if !field.IsExported() {
			continue
		}

		jsonName := tagName(field, "json")
		if jsonName == "-" {
			continue
		}

		fields = append(fields, taggedField{
			key:  typ.Name() + "." + field.Name,
			bson: tagName(field, "bson"),
			json: jsonName,
		})

		fields = append(fields, fieldsReachableFrom(elem(field.Type), visited)...)
	}

	return fields
}

// elem unwraps the container kinds a document nests through, so a
// map[string]Scene is followed to Scene.
func elem(typ reflect.Type) reflect.Type {
	for {
		switch typ = deref(typ); typ.Kind() {
		case reflect.Map, reflect.Slice, reflect.Array:
			typ = typ.Elem()
		default:
			return typ
		}
	}
}

func deref(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	return typ
}

// tagName returns the name a codec stores a field under, ignoring the options
// after the first comma. An untagged field falls back to its Go name, which is
// what both codecs do.
func tagName(field reflect.StructField, codec string) string {
	name, _, _ := strings.Cut(field.Tag.Get(codec), ",")
	if name == "" {
		return field.Name
	}

	return name
}

// The name a scene stores its private data under is a persisted format, not an
// implementation detail: documents written by every release so far hold it, and
// saveVersioned $sets the whole "stage" subtree, so the first save after a
// rename drops the old field rather than leaving it to be migrated later.
//
// This pins the spelling so the rename cannot happen as an incidental edit. If
// this test fails, the change is a migration — read the record on models.Scene
// before changing the expectation.
func TestScenePrivateDataPersistsUnderItsRecordedName(t *testing.T) {
	raw, err := bson.Marshal(models.Scene{
		PrivateData: &map[string]interface{}{"solution": "the host"},
	})
	if err != nil {
		t.Fatalf("marshalling a scene: %v", err)
	}

	var stored bson.M
	if err := bson.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("unmarshalling a scene: %v", err)
	}

	const persistedName = "private_data"

	if _, ok := stored[persistedName]; !ok {
		t.Fatalf("a scene stored its private data under %v, not %q; stored documents use %q,"+
			" and changing it orphans them — see the record on models.Scene",
			keysOf(stored), persistedName, persistedName)
	}

	// The same value must come back, or the field is write-only.
	var readBack models.Scene
	if err := bson.Unmarshal(raw, &readBack); err != nil {
		t.Fatalf("unmarshalling into a scene: %v", err)
	}

	if readBack.PrivateData == nil {
		t.Fatal("a scene's private data did not survive a bson round trip")
	}

	if got := (*readBack.PrivateData)["solution"]; got != "the host" {
		t.Errorf("read back %v, want the value that was stored", *readBack.PrivateData)
	}
}

func keysOf(doc bson.M) []string {
	keys := make([]string, 0, len(doc))
	for key := range doc {
		keys = append(keys, key)
	}

	return keys
}
