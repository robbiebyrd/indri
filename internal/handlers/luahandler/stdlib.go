package luahandler

// stdlib is pre-loaded into every handler's Lua state before the script runs.
// It provides helpers that hide indexing differences and common game logic.
const stdlib = `
-- parseMove: converts a "row,col" string (0-indexed, as sent by clients) into
-- a table {row, col} with 1-indexed values suitable for Lua table access.
function parseMove(str)
  if str == nil then error("move is nil") end
  local r, c = string.match(tostring(str), "^(%d+),(%d+)$")
  if not r then error("invalid move: " .. tostring(str)) end
  return {row = tonumber(r) + 1, col = tonumber(c) + 1}
end

-- boardWinner: scan rows, columns, and diagonals for a winning marker.
-- Returns the winning marker string, or nil if there is no winner yet.
function boardWinner(board)
  local rows = #board
  if rows == 0 then return nil end
  local cols = #board[1]

  for r = 1, rows do
    local m = board[r][1]
    if m ~= "" then
      local win = true
      for c = 2, cols do
        if board[r][c] ~= m then win = false; break end
      end
      if win then return m end
    end
  end

  for c = 1, cols do
    local m = board[1][c]
    if m ~= "" then
      local win = true
      for r = 2, rows do
        if board[r][c] ~= m then win = false; break end
      end
      if win then return m end
    end
  end

  if rows == cols then
    local m = board[1][1]
    if m ~= "" then
      local win = true
      for i = 2, rows do
        if board[i][i] ~= m then win = false; break end
      end
      if win then return m end
    end

    m = board[1][rows]
    if m ~= "" then
      local win = true
      for i = 2, rows do
        if board[i][rows - i + 1] ~= m then win = false; break end
      end
      if win then return m end
    end
  end

  return nil
end

-- boardFull: returns true when every cell is non-empty.
function boardFull(board)
  for r = 1, #board do
    for c = 1, #board[r] do
      if board[r][c] == "" then return false end
    end
  end
  return true
end
`
