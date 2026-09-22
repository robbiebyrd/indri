package luahandler

import (
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func newStateWithStdlib(t *testing.T) *lua.LState {
	t.Helper()
	L := lua.NewState()
	if err := L.DoString(stdlib); err != nil {
		t.Fatalf("loading stdlib: %v", err)
	}
	return L
}

// setBoardGlobal pushes a 2D string slice as a 1-indexed Lua table into L.
func setBoardGlobal(L *lua.LState, name string, rows [][]string) {
	outer := L.NewTable()
	for r, row := range rows {
		inner := L.NewTable()
		for c, cell := range row {
			inner.RawSetInt(c+1, lua.LString(cell))
		}
		outer.RawSetInt(r+1, inner)
	}
	L.SetGlobal(name, outer)
}

func TestBoardWinner_RowWin(t *testing.T) {
	L := newStateWithStdlib(t)
	defer L.Close()
	setBoardGlobal(L, "b", [][]string{{"X", "X", "X"}, {"O", "", "O"}, {"", "", ""}})
	if err := L.DoString(`result = boardWinner(b)`); err != nil {
		t.Fatal(err)
	}
	if got := L.GetGlobal("result").String(); got != "X" {
		t.Errorf("expected X, got %q", got)
	}
}

func TestBoardWinner_ColumnWin(t *testing.T) {
	L := newStateWithStdlib(t)
	defer L.Close()
	setBoardGlobal(L, "b", [][]string{{"O", "X", ""}, {"O", "X", ""}, {"O", "", ""}})
	if err := L.DoString(`result = boardWinner(b)`); err != nil {
		t.Fatal(err)
	}
	if got := L.GetGlobal("result").String(); got != "O" {
		t.Errorf("expected O, got %q", got)
	}
}

func TestBoardWinner_MainDiagonalWin(t *testing.T) {
	L := newStateWithStdlib(t)
	defer L.Close()
	setBoardGlobal(L, "b", [][]string{{"X", "O", ""}, {"O", "X", ""}, {"", "", "X"}})
	if err := L.DoString(`result = boardWinner(b)`); err != nil {
		t.Fatal(err)
	}
	if got := L.GetGlobal("result").String(); got != "X" {
		t.Errorf("expected X, got %q", got)
	}
}

func TestBoardWinner_AntiDiagonalWin(t *testing.T) {
	L := newStateWithStdlib(t)
	defer L.Close()
	setBoardGlobal(L, "b", [][]string{{"", "O", "X"}, {"O", "X", ""}, {"X", "", "O"}})
	if err := L.DoString(`result = boardWinner(b)`); err != nil {
		t.Fatal(err)
	}
	if got := L.GetGlobal("result").String(); got != "X" {
		t.Errorf("expected X, got %q", got)
	}
}

func TestBoardWinner_NoWinner(t *testing.T) {
	L := newStateWithStdlib(t)
	defer L.Close()
	setBoardGlobal(L, "b", [][]string{{"X", "O", "X"}, {"O", "X", "O"}, {"O", "X", "O"}})
	if err := L.DoString(`result = boardWinner(b)`); err != nil {
		t.Fatal(err)
	}
	if got := L.GetGlobal("result"); got != lua.LNil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestBoardWinner_EmptyBoard(t *testing.T) {
	L := newStateWithStdlib(t)
	defer L.Close()
	setBoardGlobal(L, "b", [][]string{{"", "", ""}, {"", "", ""}, {"", "", ""}})
	if err := L.DoString(`result = boardWinner(b)`); err != nil {
		t.Fatal(err)
	}
	if got := L.GetGlobal("result"); got != lua.LNil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestBoardFull_Full(t *testing.T) {
	L := newStateWithStdlib(t)
	defer L.Close()
	setBoardGlobal(L, "b", [][]string{{"X", "O", "X"}, {"O", "X", "O"}, {"O", "X", "O"}})
	if err := L.DoString(`result = boardFull(b)`); err != nil {
		t.Fatal(err)
	}
	if got := L.GetGlobal("result"); got != lua.LTrue {
		t.Errorf("expected true, got %v", got)
	}
}

func TestBoardFull_NotFull(t *testing.T) {
	L := newStateWithStdlib(t)
	defer L.Close()
	setBoardGlobal(L, "b", [][]string{{"X", "", "X"}, {"O", "X", "O"}, {"O", "X", "O"}})
	if err := L.DoString(`result = boardFull(b)`); err != nil {
		t.Fatal(err)
	}
	if got := L.GetGlobal("result"); got != lua.LFalse {
		t.Errorf("expected false, got %v", got)
	}
}

func TestParseMove_Valid(t *testing.T) {
	L := newStateWithStdlib(t)
	defer L.Close()
	if err := L.DoString(`m = parseMove("1,2")`); err != nil {
		t.Fatal(err)
	}
	tbl := L.GetGlobal("m").(*lua.LTable)
	row := int(tbl.RawGetString("row").(lua.LNumber))
	col := int(tbl.RawGetString("col").(lua.LNumber))
	if row != 2 || col != 3 {
		t.Errorf("expected row=2 col=3 (1-indexed), got row=%d col=%d", row, col)
	}
}

func TestParseMove_ZeroZero(t *testing.T) {
	L := newStateWithStdlib(t)
	defer L.Close()
	if err := L.DoString(`m = parseMove("0,0")`); err != nil {
		t.Fatal(err)
	}
	tbl := L.GetGlobal("m").(*lua.LTable)
	row := int(tbl.RawGetString("row").(lua.LNumber))
	col := int(tbl.RawGetString("col").(lua.LNumber))
	if row != 1 || col != 1 {
		t.Errorf("expected row=1 col=1, got row=%d col=%d", row, col)
	}
}

func TestParseMove_Invalid(t *testing.T) {
	L := newStateWithStdlib(t)
	defer L.Close()
	err := L.DoString(`m = parseMove("abc")`)
	if err == nil {
		t.Error("expected error for invalid move, got nil")
	}
}
