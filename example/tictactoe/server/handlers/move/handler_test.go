package move

import (
	"testing"
)

func TestCheckStraightAcrossWin_RowWin(t *testing.T) {
	board := [][]string{
		{"X", "X", "X"},
		{"O", "", "O"},
		{"", "", ""},
	}
	if !checkStraightAcrossWin(board, "X") {
		t.Errorf("Expected row win for 'X'")
	}
}

func TestCheckStraightAcrossWin_ColumnWin(t *testing.T) {
	board := [][]string{
		{"O", "X", ""},
		{"O", "X", ""},
		{"O", "", ""},
	}
	if !checkStraightAcrossWin(board, "O") {
		t.Errorf("Expected column win for 'O'")
	}
}

func TestCheckStraightAcrossWin_NoWin(t *testing.T) {
	board := [][]string{
		{"X", "O", "X"},
		{"O", "X", "O"},
		{"O", "X", "O"},
	}
	if checkStraightAcrossWin(board, "X") {
		t.Errorf("Expected no win for 'X'")
	}
	if checkStraightAcrossWin(board, "O") {
		t.Errorf("Expected no win for 'O'")
	}
}

func TestCheckStraightAcrossWin_EmptyBoard(t *testing.T) {
	board := [][]string{
		{"", "", ""},
		{"", "", ""},
		{"", "", ""},
	}
	if checkStraightAcrossWin(board, "X") {
		t.Errorf("Expected no win for 'X' on empty board")
	}
	if checkStraightAcrossWin(board, "O") {
		t.Errorf("Expected no win for 'O' on empty board")
	}
}

func TestCheckStraightAcrossWin_PartialRow(t *testing.T) {
	board := [][]string{
		{"X", "X", ""},
		{"O", "", "O"},
		{"", "", ""},
	}
	if checkStraightAcrossWin(board, "X") {
		t.Errorf("Expected no win for 'X' with partial row")
	}
}
func TestCheckDiagonalWin_MainDiagonalWin(t *testing.T) {
	board := [][]string{
		{"X", "O", ""},
		{"O", "X", ""},
		{"", "", "X"},
	}
	if !checkDiagonalWin(board, "X") {
		t.Errorf("Expected main diagonal win for 'X'")
	}
}

func TestCheckDiagonalWin_AntiDiagonalWin(t *testing.T) {
	board := [][]string{
		{"", "O", "X"},
		{"O", "X", ""},
		{"X", "", ""},
	}
	if !checkDiagonalWin(board, "X") {
		t.Errorf("Expected anti-diagonal win for 'X'")
	}
}

func TestCheckDiagonalWin_NoWin(t *testing.T) {
	board := [][]string{
		{"X", "O", "X"},
		{"O", "O", "X"},
		{"X", "X", "O"},
	}
	if checkDiagonalWin(board, "X") {
		t.Errorf("Expected no diagonal win for 'X'")
	}
	if checkDiagonalWin(board, "O") {
		t.Errorf("Expected no diagonal win for 'O'")
	}
}

func TestCheckDiagonalWin_EmptyBoard(t *testing.T) {
	board := [][]string{
		{"", "", ""},
		{"", "", ""},
		{"", "", ""},
	}
	if checkDiagonalWin(board, "X") {
		t.Errorf("Expected no diagonal win for 'X' on empty board")
	}
	if checkDiagonalWin(board, "O") {
		t.Errorf("Expected no diagonal win for 'O' on empty board")
	}
}

func TestCheckDiagonalWin_PartialDiagonal(t *testing.T) {
	board := [][]string{
		{"X", "", ""},
		{"", "X", ""},
		{"", "", ""},
	}
	if checkDiagonalWin(board, "X") {
		t.Errorf("Expected no win for 'X' with partial diagonal")
	}
}
func TestHandler_getUniqueStrings_ExcludeEmpty(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	board := [][]string{
		{"X", "O", ""},
		{"O", "X", ""},
		{"", "", "X"},
	}
	result := h.getUniqueStrings(board, false)
	expected := []string{"X", "O"}
	if len(result) != len(expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
	for _, v := range expected {
		found := false
		for _, r := range result {
			if r == v {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected %v in result, got %v", v, result)
		}
	}
}

func TestHandler_getUniqueStrings_IncludeEmpty(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	board := [][]string{
		{"X", "O", ""},
		{"O", "X", ""},
		{"", "", "X"},
	}
	result := h.getUniqueStrings(board, true)
	expected := []string{"X", "O", ""}
	if len(result) != len(expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
	for _, v := range expected {
		found := false
		for _, r := range result {
			if r == v {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected %v in result, got %v", v, result)
		}
	}
}

func TestHandler_getUniqueStrings_AllEmpty(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	board := [][]string{
		{"", "", ""},
		{"", "", ""},
		{"", "", ""},
	}
	result := h.getUniqueStrings(board, false)
	if len(result) != 0 {
		t.Errorf("Expected empty slice, got %v", result)
	}
	resultWithEmpty := h.getUniqueStrings(board, true)
	if len(resultWithEmpty) != 1 || resultWithEmpty[0] != "" {
		t.Errorf("Expected slice with one empty string, got %v", resultWithEmpty)
	}
}

func TestHandler_getUniqueStrings_NoDuplicates(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	board := [][]string{
		{"X", "X", "O"},
		{"O", "X", "O"},
		{"O", "X", "O"},
	}
	result := h.getUniqueStrings(board, false)
	expected := []string{"X", "O"}
	if len(result) != len(expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
	for _, v := range expected {
		found := false
		for _, r := range result {
			if r == v {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected %v in result, got %v", v, result)
		}
	}
}
func TestHandler_findWinner_RowWin(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	board := [][]string{
		{"X", "X", "X"},
		{"O", "", "O"},
		{"", "", ""},
	}
	winner, won := h.findWinner(board)
	if !won || winner != "X" {
		t.Errorf("Expected winner 'X', got '%v', won: %v", winner, won)
	}
}

func TestHandler_findWinner_ColumnWin(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	board := [][]string{
		{"O", "X", ""},
		{"O", "X", ""},
		{"O", "", ""},
	}
	winner, won := h.findWinner(board)
	if !won || winner != "O" {
		t.Errorf("Expected winner 'O', got '%v', won: %v", winner, won)
	}
}

func TestHandler_findWinner_DiagonalWin(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	board := [][]string{
		{"X", "O", ""},
		{"O", "X", ""},
		{"", "", "X"},
	}
	winner, won := h.findWinner(board)
	if !won || winner != "X" {
		t.Errorf("Expected winner 'X' (diagonal), got '%v', won: %v", winner, won)
	}
}

func TestHandler_findWinner_AntiDiagonalWin(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	board := [][]string{
		{"", "O", "X"},
		{"O", "X", ""},
		{"X", "", ""},
	}
	winner, won := h.findWinner(board)
	if !won || winner != "X" {
		t.Errorf("Expected winner 'X' (anti-diagonal), got '%v', won: %v", winner, won)
	}
}

func TestHandler_findWinner_Draw(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	board := [][]string{
		{"X", "O", "X"},
		{"O", "X", "O"},
		{"O", "X", "O"},
	}
	winner, won := h.findWinner(board)
	if !won || winner != "draw" {
		t.Errorf("Expected draw, got '%v', won: %v", winner, won)
	}
}

func TestHandler_findWinner_NoWin_NotEnoughMarkers(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	board := [][]string{
		{"X", "", ""},
		{"", "", ""},
		{"", "", ""},
	}
	winner, won := h.findWinner(board)
	if won || winner != "" {
		t.Errorf("Expected no winner, got '%v', won: %v", winner, won)
	}
}

func TestHandler_findWinner_NoWin_EmptyBoard(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	board := [][]string{
		{"", "", ""},
		{"", "", ""},
		{"", "", ""},
	}
	winner, won := h.findWinner(board)
	if won || winner != "" {
		t.Errorf("Expected no winner on empty board, got '%v', won: %v", winner, won)
	}
}

func TestHandler_findWinner_NoWin_PartialRow(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	board := [][]string{
		{"X", "X", ""},
		{"O", "", "O"},
		{"", "", ""},
	}
	winner, won := h.findWinner(board)
	if won || winner != "" {
		t.Errorf("Expected no winner with partial row, got '%v', won: %v", winner, won)
	}
}
func TestHandler_decodeMove_ValidMove(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	input := map[string]interface{}{"move": "1,2"}
	move, err := h.decodeMove(input, 3, 3)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if move == nil || len(*move) != 2 || (*move)[0] != 1 || (*move)[1] != 2 {
		t.Errorf("Expected move [1,2], got %v", move)
	}
}

func TestHandler_decodeMove_RectangularBoardBounds(t *testing.T) {
	h := &TicTacToeMoveHandler{}

	// On a 3-row, 2-column board, row 2 is valid but column 2 is out of bounds.
	if move, err := h.decodeMove(map[string]interface{}{"move": "2,1"}, 2, 3); err != nil {
		t.Errorf("Expected (2,1) valid on a 3x2 board, got err: %v", err)
	} else if move == nil || (*move)[0] != 2 || (*move)[1] != 1 {
		t.Errorf("Expected move [2,1], got %v", move)
	}

	if move, err := h.decodeMove(map[string]interface{}{"move": "1,2"}, 2, 3); err == nil || move != nil {
		t.Errorf("Expected (1,2) out of bounds on a 3x2 board, got move: %v, err: %v", move, err)
	}
}

func TestHandler_decodeMove_MissingMoveKey(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	input := map[string]interface{}{}
	move, err := h.decodeMove(input, 3, 3)
	if err == nil || move != nil {
		t.Errorf("Expected error for missing move key, got move: %v, err: %v", move, err)
	}
}

func TestHandler_decodeMove_InvalidFormat(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	input := map[string]interface{}{"move": "1"}
	move, err := h.decodeMove(input, 3, 3)
	if err == nil || move != nil {
		t.Errorf("Expected error for invalid format, got move: %v, err: %v", move, err)
	}
}

func TestHandler_decodeMove_NonInteger(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	input := map[string]interface{}{"move": "a,2"}
	move, err := h.decodeMove(input, 3, 3)
	if err == nil || move != nil {
		t.Errorf("Expected error for non-integer value, got move: %v, err: %v", move, err)
	}
}

func TestHandler_decodeMove_OutOfBounds_Negative(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	input := map[string]interface{}{"move": "-1,2"}
	move, err := h.decodeMove(input, 3, 3)
	if err == nil || move != nil {
		t.Errorf("Expected error for out-of-bounds negative value, got move: %v, err: %v", move, err)
	}
}

func TestHandler_decodeMove_OutOfBounds_Positive(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	input := map[string]interface{}{"move": "1,3"}
	move, err := h.decodeMove(input, 3, 3)
	if err == nil || move != nil {
		t.Errorf("Expected error for out-of-bounds positive value, got move: %v, err: %v", move, err)
	}
}

func TestHandler_decodeMove_ExtraValues(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	input := map[string]interface{}{"move": "1,2,0"}
	move, err := h.decodeMove(input, 3, 3)
	if err == nil || move != nil {
		t.Errorf("Expected error for extra values, got move: %v, err: %v", move, err)
	}
}

func TestHandler_decodeMove_EmptyString(t *testing.T) {
	h := &TicTacToeMoveHandler{}
	input := map[string]interface{}{"move": ""}
	move, err := h.decodeMove(input, 3, 3)
	if err == nil || move != nil {
		t.Errorf("Expected error for empty string, got move: %v, err: %v", move, err)
	}
}
func TestGetBoardSize_StandardBoard(t *testing.T) {
	board := [][]string{
		{"X", "O", ""},
		{"O", "X", ""},
		{"", "", "X"},
	}
	rows, cols := getBoardSize(board)
	if rows != 3 || cols != 3 {
		t.Errorf("Expected (3,3), got (%d,%d)", rows, cols)
	}
}

func TestGetBoardSize_RectangularBoard(t *testing.T) {
	board := [][]string{
		{"X", "O"},
		{"O", "X"},
		{"", "X"},
	}
	rows, cols := getBoardSize(board)
	if rows != 3 || cols != 2 {
		t.Errorf("Expected (3,2), got (%d,%d)", rows, cols)
	}
}

func TestGetBoardSize_OneRowBoard(t *testing.T) {
	board := [][]string{
		{"X", "O", "X"},
	}
	rows, cols := getBoardSize(board)
	if rows != 1 || cols != 3 {
		t.Errorf("Expected (1,3), got (%d,%d)", rows, cols)
	}
}

func TestGetBoardSize_OneColumnBoard(t *testing.T) {
	board := [][]string{
		{"X"},
		{"O"},
		{"X"},
	}
	rows, cols := getBoardSize(board)
	if rows != 3 || cols != 1 {
		t.Errorf("Expected (3,1), got (%d,%d)", rows, cols)
	}
}
