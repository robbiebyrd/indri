-- indri.game -- read helpers over a game state.
--
-- The state every helper here takes is the JSON view of models.Game: the same
-- shape the client already holds, keyed by json tag names. A helper therefore
-- reads state.stage.currentScene, team.playerIds and player.score, never the Go
-- field names.
--
-- Two rules hold throughout.
--
-- Every table is optional. The Go model tags teams, scenes and the three data
-- stores omitempty, so a game with no teams arrives with no `teams` key at all.
-- A helper substitutes an empty table rather than raising on a nil index, and
-- reports "nothing found" by returning nil.
--
-- Every walk over a map is ordered. pairs() order is unspecified in Lua, so a
-- helper that picked the first match out of pairs() would be free to answer two
-- different ways for the same game -- on two servers, or on two invocations of
-- the same one. Each helper that walks a map walks it in sorted key order
-- instead, which is what makes leader_by_score's tie-break and each_team's
-- iteration order something a game can rely on.

local M = {}

-- sorted_ids returns the keys of t, sorted as strings.
--
-- The comparator goes through tostring rather than comparing the keys directly:
-- table.sort raises when it has to compare a string to a number, and while ids
-- decoded from JSON are always strings, nothing stops a caller from handing in a
-- state assembled in Lua with numeric ones.
local function sorted_ids(t)
    local ids = {}

    for id in pairs(t or {}) do
        ids[#ids + 1] = id
    end

    table.sort(ids, function(a, b) return tostring(a) < tostring(b) end)

    return ids
end

-- current_scene returns the id of the stage's current scene and the scene
-- table it names.
--
-- The two results are independent. An id with no matching entry in
-- stage.scenes yields the id and nil, because a script that wants to branch on
-- which scene is current should not have to care whether the scene carries any
-- data. A stage with no current scene at all -- the zero value, whose
-- currentScene is the empty string -- yields nothing.
function M.current_scene(state)
    local stage = (state or {}).stage or {}
    local id = stage.currentScene

    if id == nil or id == "" then
        return nil
    end

    return id, (stage.scenes or {})[id]
end

-- scene_data returns the public data of the current scene, or an empty table
-- when there is no current scene or it carries none.
--
-- It is the state's own table, not a copy, so writing to it changes what the
-- rest of this invocation sees and nothing more: the state a script is handed is
-- a value, and only what the script returns is ever persisted.
function M.scene_data(state)
    local _, scene = M.current_scene(state)

    return (scene or {}).data or {}
end

-- team_of returns the id of the team player_id belongs to, and the team table.
--
-- A player is expected to be in at most one team. If one is somehow listed in
-- two, the team whose id sorts first wins, so the answer is at least the same
-- one every time.
function M.team_of(state, player_id)
    if player_id == nil then
        return nil
    end

    local teams = (state or {}).teams or {}

    for _, id in ipairs(sorted_ids(teams)) do
        for _, pid in ipairs((teams[id] or {}).playerIds or {}) do
            if pid == player_id then
                return id, teams[id]
            end
        end
    end

    return nil
end

-- players_in_team returns the ids of the team's members, in the order the team
-- lists them.
--
-- The result is a fresh array rather than the team's own playerIds, so a caller
-- sorting or truncating it cannot edit the state by accident. An unknown team is
-- an empty array, not nil, so the result is always safe to pass to ipairs.
function M.players_in_team(state, team_id)
    local ids = {}

    if team_id == nil then
        return ids
    end

    local team = ((state or {}).teams or {})[team_id] or {}

    for i, pid in ipairs(team.playerIds or {}) do
        ids[i] = pid
    end

    return ids
end

-- leader_by_score returns the id of the highest-scoring player and the player
-- table, or nothing when the game has no players.
--
-- Ties are broken by the lowest player id, compared as a string. That rule is
-- part of the contract and not an accident of the implementation: a game that
-- awards something to the leader has to award it to the same player every time
-- it asks, and "whoever pairs() happened to reach first" is not a rule anyone
-- can reason about. Players are therefore visited in ascending id order and the
-- leader is replaced only on a strictly higher score, which leaves the first --
-- lowest -- id holding the tie.
--
-- A player with no score, or one whose score is not a number, counts as zero.
function M.leader_by_score(state)
    local players = (state or {}).players or {}

    local leader_id, leader, best

    for _, id in ipairs(sorted_ids(players)) do
        local player = players[id] or {}
        local score = tonumber(player.score) or 0

        if best == nil or score > best then
            leader_id, leader, best = id, player, score
        end
    end

    if leader_id == nil then
        return nil
    end

    return leader_id, leader
end

-- each_team returns an iterator over the game's teams, in id order:
--
--     for id, team in indri_game.each_team(state) do ... end
--
-- The team ids are read once, when the iterator is built, so adding a team
-- during the loop does not change what the loop visits.
function M.each_team(state)
    local teams = (state or {}).teams or {}
    local ids = sorted_ids(teams)
    local i = 0

    return function()
        i = i + 1

        local id = ids[i]

        if id == nil then
            return nil
        end

        return id, teams[id]
    end
end

return M
