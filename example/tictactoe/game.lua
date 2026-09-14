-- Tic-tac-toe: the whole game, as one script.
--
-- The board is stage.scenes.<currentScene>.data.board, a list of rows of cell
-- strings. Its size is read from the board itself rather than assumed, so a
-- config declaring a three-row, two-column board plays a three-row, two-column
-- game -- which is also what makes the bounds check something a test can drive
-- from a fixture instead of from a constant.
--
-- CELL COORDINATES ARE ZERO-INDEXED AND LUA IS ONE-INDEXED. A player sends
-- "r,c" counted from zero and the cell is board[r + 1][c + 1]. Get the +1 wrong
-- in one place only and the board transposes silently: it still looks like a
-- game, it just plays a different one. The win checks below keep the same
-- convention for the same reason -- see diagonal_win.

local game = require("indri.game")

-- The cell value meaning "nobody has played here".
local EMPTY = ""

-- What winningTeam holds when the board filled with no line. Everything else it
-- holds is a team id, which is how a client tells the two apart.
local DRAW = "draw"

-- reject ends the move with message.
--
-- Raising rather than returning is what makes a refusal safe. An error inside
-- the indri.mutate callback unwinds the store without saving, so a refused move
-- writes nothing and publishes no delta; Invoke then packs the message into the
-- frame its caller is answered with. Level 0 keeps this file and line out of
-- that frame: "it is not your turn" is a player's mistake, not the author's.
local function reject(message)
    error(message, 0)
end

-- board_size returns the board's rows and columns.
--
-- The column count comes from the first row alone, exactly as the Go handler
-- this replaced measured it. A ragged board is therefore measured by its first
-- row and indexed past the end of a shorter one, which surfaces as a script
-- error rather than as a silently different game.
local function board_size(board)
    if #board == 0 then
        return 0, 0
    end

    return #board, #board[1]
end

-- to_index parses one coordinate, or returns nil when it is not an integer.
--
-- The pattern is stricter than tonumber on purpose. tonumber accepts "0x2",
-- "1e3", "2.0" and " 2 "; the Go strconv.Atoi this replaces accepts none of
-- them, and a coordinate that quietly parses as something else is a move landing
-- on a cell the player did not name.
local function to_index(text)
    if string.match(text, "^[-+]?%d+$") == nil then
        return nil
    end

    return tonumber(text)
end

-- decode_move parses the payload's "move" into zero-based row and column, and
-- rejects anything outside the board.
--
-- Bounds are checked here, before the cell is read, because an out-of-range
-- index in Lua is nil rather than an error: without this, "5,5" would reach the
-- occupied-cell test as a nil cell and be reported as a taken spot.
local function decode_move(payload, rows, columns)
    local move = payload.move

    if move == nil then
        reject("move is nil")
    end

    if type(move) ~= "string" then
        reject("invalid move: " .. tostring(move))
    end

    -- The trailing comma makes the last field match the same pattern as the
    -- others, so "1,2" yields two fields and "1" yields one -- the split Go's
    -- strings.Split performs, rather than gmatch's usual habit of dropping an
    -- empty final field.
    local fields = {}

    for field in string.gmatch(move .. ",", "([^,]*),") do
        fields[#fields + 1] = field
    end

    local coordinates = {}

    for i, field in ipairs(fields) do
        local index = to_index(field)

        -- Refused before the assignment, because storing a nil would leave a
        -- hole in the list and make the length check below read the wrong count.
        if index == nil then
            reject("error converting string '" .. field .. "' to int")
        end

        coordinates[i] = index
    end

    if #coordinates ~= 2 then
        reject("invalid move: " .. move)
    end

    local row, column = coordinates[1], coordinates[2]

    if row < 0 or row >= rows or column < 0 or column >= columns then
        reject("move out of bounds: " .. move)
    end

    return row, column
end

-- markers_on returns every distinct non-empty cell value, sorted.
--
-- Sorted so that a board somehow holding two winning lines names the same winner
-- on every call. The Go handler walked a map here and was free to answer either
-- way.
local function markers_on(board, rows, columns)
    local seen, markers = {}, {}

    for row = 1, rows do
        for column = 1, columns do
            local cell = board[row][column]

            if cell ~= EMPTY and not seen[cell] then
                seen[cell] = true
                markers[#markers + 1] = cell
            end
        end
    end

    table.sort(markers)

    return markers
end

-- has_empty_cell reports whether anyone can still play.
local function has_empty_cell(board, rows, columns)
    for row = 1, rows do
        for column = 1, columns do
            if board[row][column] == EMPTY then
                return true
            end
        end
    end

    return false
end

-- straight_across_win reports whether marker fills a whole row or a whole
-- column.
local function straight_across_win(board, rows, columns, marker)
    for row = 1, rows do
        local win = true

        for column = 1, columns do
            if board[row][column] ~= marker then
                win = false
                break
            end
        end

        if win then
            return true
        end
    end

    for column = 1, columns do
        local win = true

        for row = 1, rows do
            if board[row][column] ~= marker then
                win = false
                break
            end
        end

        if win then
            return true
        end
    end

    return false
end

-- diagonal_win reports whether marker fills a diagonal running either way.
--
-- A diagonal holds at most one cell per row, so only one spanning every row can
-- be complete, and each is named by the column it starts from on the top row.
-- That is why the two loops run over starting columns rather than over every
-- offset: a shorter diagonal cannot decide the game, and enumerating it anyway
-- would leave the loop bounds saying nothing about which lines matter. A board
-- with more columns than rows therefore has several of each -- the
-- generalisation the Go handler this replaced had, kept because the size is read
-- from the board and a non-square one is reachable from config alone.
--
-- Coordinates stay zero-based and the one is added only at the index, the same
-- convention the placement uses: "down and right" is row + 1, column + 1 in the
-- player's frame, and mixing the two frames is how a board transposes.
local function diagonal_win(board, rows, columns, marker)
    -- Down and to the right, from every start that still fits.
    for start = 0, columns - rows do
        local win = true

        for row = 0, rows - 1 do
            if board[row + 1][start + row + 1] ~= marker then
                win = false
                break
            end
        end

        if win then
            return true
        end
    end

    -- Down and to the left, which needs enough columns to its left to fit.
    for start = rows - 1, columns - 1 do
        local win = true

        for row = 0, rows - 1 do
            if board[row + 1][start - row + 1] ~= marker then
                win = false
                break
            end
        end

        if win then
            return true
        end
    end

    return false
end

-- find_winner returns the winning marker, DRAW, or nil while the game is live.
--
-- The two-marker gate is not an optimisation. A board holding only one player's
-- marks cannot have been played by both, so declaring a win on it would end a
-- game that a config seeding the board had merely started part-way through.
local function find_winner(board, rows, columns)
    local markers = markers_on(board, rows, columns)

    if #markers < 2 then
        return nil
    end

    for _, marker in ipairs(markers) do
        if straight_across_win(board, rows, columns, marker)
            or diagonal_win(board, rows, columns, marker) then
            return marker
        end
    end

    if has_empty_cell(board, rows, columns) then
        return nil
    end

    return DRAW
end

-- team_with_marker returns the id of the team playing marker.
local function team_with_marker(state, marker)
    for id, team in game.each_team(state) do
        if (team.data or {}).marker == marker then
            return id
        end
    end

    reject("no team found with marker " .. marker)
end

-- pass_turn gives the turn to everyone except the team that just played.
--
-- Written as "not the mover" rather than as a swap so that a config with more
-- than two teams still leaves somebody able to play, which is what the Go
-- handler did.
local function pass_turn(state, mover_id)
    for id, team in game.each_team(state) do
        team.data = team.data or {}
        team.data.turn = id ~= mover_id
    end
end

indri.on("move", function(req)
    local session = req.session

    -- Authority comes from the session the transport authenticated, never from
    -- the payload: a player may only ever move as the team they joined as.
    if session == nil then
        reject("not authenticated")
    end

    if session.gameId == nil or session.teamId == nil then
        reject("session is not in a game/team")
    end

    -- One indri.mutate covers validation, the placement, the win check and the
    -- turn pass together, so two racing moves cannot leave the board advanced
    -- with the turn unpassed. The callback re-runs on a lost version fence, so
    -- everything it reads must be read from the state it was handed.
    indri.mutate(function(state)
        local team = (state.teams or {})[session.teamId]

        if team == nil then
            reject("your team is not in this game")
        end

        local team_data = team.data or {}
        local marker = team_data.marker

        if marker == nil then
            reject("marker is nil")
        end

        if team_data.turn == nil then
            reject("turn is nil")
        end

        if not team_data.turn then
            reject("it is not your turn")
        end

        local scene_id = game.current_scene(state)

        if scene_id == nil then
            reject("the stage has no current scene")
        end

        -- The scene's own data table, so writing the cell below edits the state
        -- that is about to be returned.
        local scene_data = game.scene_data(state)
        local board = scene_data.board

        if board == nil then
            reject("the current scene has no board")
        end

        local rows, columns = board_size(board)
        local row, column = decode_move(req.payload or {}, rows, columns)

        if board[row + 1][column + 1] ~= EMPTY then
            reject("spot is taken")
        end

        board[row + 1][column + 1] = marker

        local winner = find_winner(board, rows, columns)

        if winner == DRAW then
            scene_data.winningTeam = DRAW
        elseif winner ~= nil then
            scene_data.winningTeam = team_with_marker(state, winner)
        end

        pass_turn(state, session.teamId)

        return state
    end)
end)
