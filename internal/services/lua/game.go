package lua

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	lua "github.com/yuin/gopher-lua"

	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/events"
)

// gameToLua renders a game as the Lua table a script edits.
//
// It goes through events.ToMap so a script sees exactly the document the delta
// path compares and the client already holds: json tag names, numbers as
// float64, timestamps as RFC3339 strings. Nothing is hidden — Lua is
// authoritative, and sanitization stays on the outbound broadcast path.
func gameToLua(L *lua.LState, g *models.Game) (lua.LValue, error) {
	m, err := events.ToMap(g)
	if err != nil {
		return nil, fmt.Errorf("rendering game as a document: %w", err)
	}

	return toLua(L, m)
}

// applyLua writes the table a script returned back over g.
//
// The decoded document is unmarshalled into a *zero* models.Game rather than
// into g. That is what makes deletion work: encoding/json merges into an
// existing map instead of replacing it, so unmarshalling over g would leave
// every key the script removed in place.
//
// Unmarshalling into a zero value means the server-owned fields have to be put
// back by hand. Version is json:"-" and would otherwise reset to 0, silently
// destroying the CAS fence that mutation.Run relies on; ID, CreatedAt and
// DeletedAt are restored for the same reason. The script's own "id" key is
// dropped before decoding, so neither a forged ObjectID nor a malformed one
// reaches the field.
//
// g is left untouched unless the whole apply succeeds, so a rejected mutation
// never half-writes.
func applyLua(g *models.Game, v lua.LValue, b *budget) error {
	decoded, err := fromLua(v, b)
	if err != nil {
		return err
	}

	doc, ok := decoded.(map[string]interface{})
	if !ok {
		return fmt.Errorf("game state must be a table, got %T", decoded)
	}

	// Server-owned. Removing the key is kinder than letting json.Unmarshal
	// reject "id": "nonsense" with an error about ObjectID hex digits.
	delete(doc, "id")

	raw, err := json.Marshal(coerceEmpty(doc, reflect.TypeOf(models.Game{})))
	if err != nil {
		return fmt.Errorf("encoding game state: %w", err)
	}

	var next models.Game

	if err := json.Unmarshal(raw, &next); err != nil {
		return fmt.Errorf("decoding game state: %w", err)
	}

	next.ID, next.Version = g.ID, g.Version
	next.CreatedAt, next.DeletedAt = g.CreatedAt, g.DeletedAt

	if err := checkInvariants(g, &next); err != nil {
		return err
	}

	*g = next

	return nil
}

// coerceEmpty reconciles the one thing Lua cannot express: the difference
// between an empty list and an empty object.
//
// A Lua table has no element type, so an empty Team.PlayerIDs and an empty
// Game.Players come back from fromLua as the same shape and one of them is
// always wrong for its Go field. Neither field carries omitempty, so ordinary
// state hits this:
//
//	json: cannot unmarshal object into Go struct field team.playerIds of type []string
//	json: cannot unmarshal array into Go struct field game.players of type map[string]int
//
// No global rule in the marshaller can fix it — a game may legitimately have
// both empty at once. The Go type is the missing information, so this walks
// models.Game's reflect type alongside the decoded document and swaps an empty
// map for an empty slice, or the reverse, wherever the target field is
// concrete. Inside a *Data map the target is interface{}, which accepts either
// shape, so nothing there is touched and nothing there can break.
//
// The document is edited in place; it was built by fromLua for this call and
// has no other owner.
func coerceEmpty(v any, t reflect.Type) any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Struct:
		return coerceStruct(v, t)
	case reflect.Slice:
		return coerceSlice(v, t)
	case reflect.Map:
		return coerceMap(v, t)
	default:
		// Scalars, and interface{} targets that accept either shape.
		return v
	}
}

func coerceStruct(v any, t reflect.Type) any {
	m, ok := v.(map[string]interface{})
	if !ok {
		return v
	}

	for i := range t.NumField() {
		field := t.Field(i)

		name, ok := jsonFieldName(field)
		if !ok {
			continue
		}

		if val, present := m[name]; present {
			m[name] = coerceEmpty(val, field.Type)
		}
	}

	return m
}

func coerceSlice(v any, t reflect.Type) any {
	if m, ok := v.(map[string]interface{}); ok && len(m) == 0 {
		return []interface{}{}
	}

	items, ok := v.([]interface{})
	if !ok {
		return v
	}

	for i, item := range items {
		items[i] = coerceEmpty(item, t.Elem())
	}

	return items
}

func coerceMap(v any, t reflect.Type) any {
	if items, ok := v.([]interface{}); ok && len(items) == 0 {
		return map[string]interface{}{}
	}

	m, ok := v.(map[string]interface{})
	if !ok {
		return v
	}

	for key, val := range m {
		m[key] = coerceEmpty(val, t.Elem())
	}

	return m
}

// jsonFieldName reports the document key a struct field decodes from, and
// whether the field participates in JSON at all.
func jsonFieldName(field reflect.StructField) (string, bool) {
	if !field.IsExported() {
		return "", false
	}

	tag, ok := field.Tag.Lookup("json")
	if !ok {
		return field.Name, true
	}

	name, _, _ := strings.Cut(tag, ",")

	switch name {
	case "-":
		return "", false
	case "":
		return field.Name, true
	default:
		return name, true
	}
}

// checkInvariants refuses a script edit that rewrites membership or authority.
//
// Handing a script the whole document is what makes state.players[id].score = n
// ergonomic, but it also lets the script rewrite who is in the game, who is on
// which team and who is host. models.Session stores GameID, TeamID and UserID
// independently of the game document, so a membership edit made here does not
// desynchronise one document — it desynchronises two, and nothing reconciles
// them afterwards. Host is worse still: a script could promote its own caller.
//
// Membership therefore stays in Go, reached only through the built-in actions
// (join, leave, kick, ...) that also maintain the session. What a script owns
// is the game's content: Player.Score, every *Data map, and the whole Stage.
func checkInvariants(before, after *models.Game) error {
	if before.Code != after.Code {
		return fmt.Errorf("script changed the game code from %q to %q", before.Code, after.Code)
	}

	if before.Private != after.Private {
		return fmt.Errorf("script changed the game privacy from %t to %t", before.Private, after.Private)
	}

	if err := checkPlayers(before.Players, after.Players); err != nil {
		return err
	}

	return checkTeams(before.Teams, after.Teams)
}

func checkPlayers(before, after map[string]models.Player) error {
	added, removed := setDiff(slices.Collect(maps.Keys(before)), slices.Collect(maps.Keys(after)))
	if len(added) > 0 || len(removed) > 0 {
		return fmt.Errorf("script changed the player set (added %v, removed %v)", added, removed)
	}

	for _, id := range slices.Sorted(maps.Keys(before)) {
		was, is := before[id], after[id]

		if was.Host != is.Host {
			return fmt.Errorf("script changed the host flag of player %q from %t to %t", id, was.Host, is.Host)
		}

		if was.Connected != is.Connected {
			return fmt.Errorf("script changed the connected flag of player %q from %t to %t", id, was.Connected, is.Connected)
		}
	}

	return nil
}

func checkTeams(before, after map[string]models.Team) error {
	ids := slices.Sorted(maps.Keys(before))

	for _, id := range slices.Sorted(maps.Keys(after)) {
		if _, ok := before[id]; !ok {
			ids = append(ids, id)
		}
	}

	for _, id := range ids {
		// A team missing from either side contributes no members, so dropping a
		// team that still holds players is a membership change like any other,
		// while adding or removing an empty team is not.
		added, removed := setDiff(before[id].PlayerIDs, after[id].PlayerIDs)
		if len(added) > 0 || len(removed) > 0 {
			return fmt.Errorf("script changed the membership of team %q (added %v, removed %v)", id, added, removed)
		}
	}

	return nil
}

// setDiff reports which members are only in after and which are only in before,
// sorted so an error message reads the same way on every run.
func setDiff(before, after []string) (added, removed []string) {
	inBefore := make(map[string]struct{}, len(before))
	for _, id := range before {
		inBefore[id] = struct{}{}
	}

	inAfter := make(map[string]struct{}, len(after))
	for _, id := range after {
		inAfter[id] = struct{}{}
	}

	for id := range inAfter {
		if _, ok := inBefore[id]; !ok {
			added = append(added, id)
		}
	}

	for id := range inBefore {
		if _, ok := inAfter[id]; !ok {
			removed = append(removed, id)
		}
	}

	slices.Sort(added)
	slices.Sort(removed)

	return added, removed
}
