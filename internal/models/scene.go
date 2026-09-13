package models

// Scene is the visual unit the client renders.
//
// PrivateData is tagged bson "private_data", while the same store on Game,
// Stage, Team and Player is tagged "privateData". It is the only field in this
// package whose bson name differs from its json name, apart from "_id" and the
// fields json hides. That divergence is a defect. The fix is blocked on a
// migration decision, which is not an agent's to take, so the record below is
// what was checked — the next reader does not have to check it again.
//
// # Why it is wrong
//
// A dotted path is used twice, unchanged: gameRepo.UpdateField gives the key to
// MongoDB as the update path, and publishes the same string as the path of the
// delta every player receives. This works only while a field's bson name and
// its json name agree, because MongoDB resolves the path by bson tag while the
// client and events.SanitizeDelta read it by json tag. This field is the one
// place they disagree, so the two uses cannot both be correct:
//
//   - internal/services/stage composes the path from DataStorePrivate
//     ("privateData"), so UpdateScene and LoadSceneFromScript write
//     stage.scenes.<id>.privateData — a field this struct does not declare. It
//     is stored, and no read gives it back.
//   - Composing that path from this bson tag instead would store the data where
//     the struct can read it, but the published delta would then end in a
//     "private_data" segment, which SanitizeDelta does not recognise as
//     private. Scene private data would go out to every player. That is worse.
//
// # Which direction
//
// Change this tag to "privateData". Respelling DataStorePrivate as
// "private_data" is not available: that constant is the shared name of the
// private store for the game, stage, team and player as well, all of which
// persist it as "privateData"; it is the segment SanitizeDelta matches on; and
// it is the json name on the wire that the client and the Lua host API use.
//
// # What it costs
//
// Changing the tag orphans data that is already stored, so it needs a migration
// or a read-both/write-new shim. That is the open decision. Checked, not
// assumed:
//
//   - The field is live, not theoretical. config.json declares
//     "privateData": {} on the board scene, json decodes that to a non-nil
//     empty map, and gameRepo.New stamps the whole script stage into the new
//     document. So stage.scenes.board.private_data exists on every game ever
//     created.
//   - Lua game scripts write it. The host API in internal/services/lua applies
//     a script's table over the game through its json view, where the field is
//     already "privateData" (applyLua, called from host_mutate.go), and Mutate
//     saves the result whole, which bson-marshals it back to "private_data".
//   - Every save rewrites it. saveVersioned $sets the whole "stage" subtree, so
//     the first save after a tag change replaces that subtree without
//     "private_data", and the old value is gone silently.
//   - Those three are the only reachable writers. The broken "privateData" path
//     has no writers at all: only stage.Service.UpdateScene and
//     LoadSceneFromScript compose it, and nothing in this repository imports
//     internal/services/stage.
//   - A deployed database cannot be inspected from here, so it cannot be shown
//     that a deployment's scenes hold nothing worth keeping. This repository's
//     config.json holds an empty map, but a deployment's config.json and its
//     Lua scripts are not visible here.
//
// # Why this is not an incident
//
// Neither spelling leaks. Data at "privateData" is never decoded into this
// struct, so no keyframe can carry it. Data at "private_data" is decoded, and
// GameService.Sanitize nils this field on every scene before a keyframe goes
// out. Delta paths name "privateData" either way, and SanitizeDelta drops any
// path holding that segment.
type Scene struct {
	PublicData  *map[string]interface{}            `bson:"data"         json:"data,omitempty"`
	PrivateData *map[string]interface{}            `bson:"private_data" json:"privateData,omitempty"`
	PlayerData  *map[string]map[string]interface{} `bson:"playerData"   json:"playerData,omitempty"`
}
