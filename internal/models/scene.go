package models

// Scene is the visual unit the client renders.
//
// PrivateData was tagged bson "private_data" until this tag was flipped to
// "privateData", matching the same store on Game, Stage, Team and Player. The
// old spelling was a defect: a dotted path is used twice, unchanged —
// gameRepo.UpdateField gives the key to MongoDB as the update path and
// publishes the same string as the path of the delta every player receives —
// and that works only while a field's bson name and its json name agree,
// because MongoDB resolves the path by bson tag while the client and
// events.SanitizeDelta read it by json tag. internal/services/stage composes
// the path from DataStorePrivate ("privateData"), so UpdateScene and
// LoadSceneFromScript wrote stage.scenes.<id>.privateData — a field the struct
// did not declare. It was stored, and no read gave it back.
//
// # Why the tag was the side that had to change
//
// The two alternatives were both worse, and that analysis is why this
// direction is the right one:
//
//   - Respelling DataStorePrivate as "private_data" is not available. That
//     constant is the shared name of the private store for the game, stage,
//     team and player as well, all of which persist it as "privateData"; it is
//     the segment SanitizeDelta matches on; and it is the json name on the wire
//     that the client and the Lua host API use.
//   - Composing the scene path from the bson tag instead would have stored the
//     data where the struct can read it, but the published delta would then end
//     in a "private_data" segment, which SanitizeDelta does not recognise as
//     private. Scene private data would go out to every player.
//
// # The accepted loss
//
// Flipping the tag orphans scene private data already stored under
// "private_data": saveVersioned $sets the whole "stage" subtree, so the first
// save after this change replaced that subtree without the old field rather
// than leaving it to be migrated. config.json declares "privateData": {} on the
// board scene and gameRepo.New stamps the script stage into every new document,
// so stage.scenes.board.private_data existed on every game ever created, and
// Lua scripts could write it live through their json view.
//
// Robbie decided to accept that one-time loss rather than write a migration or
// a read-both/write-new shim: no deployment holds scene private data worth
// keeping. The alternative was carrying a compatibility shim indefinitely for
// data known to be empty.
//
// # Why this was never an incident
//
// Neither spelling leaked. Data at "privateData" was never decoded into this
// struct, so no keyframe could carry it. Data at "private_data" was decoded,
// and GameService.Sanitize nils this field on every scene before a keyframe
// goes out. Delta paths named "privateData" either way, and SanitizeDelta drops
// any path holding that segment.
type Scene struct {
	PublicData  *map[string]interface{}            `bson:"data"         json:"data,omitempty"`
	PrivateData *map[string]interface{}            `bson:"privateData"  json:"privateData,omitempty"`
	PlayerData  *map[string]map[string]interface{} `bson:"playerData"   json:"playerData,omitempty"`
}
