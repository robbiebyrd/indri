package restart

import "testing"

func TestBuildEmptyBoard_3x3(t *testing.T) {
	board := buildEmptyBoard(3, 3)
	if len(board) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(board))
	}
	for i, row := range board {
		if len(row) != 3 {
			t.Fatalf("expected 3 cols in row %d, got %d", i, len(row))
		}
		for j, cell := range row {
			if cell != "" {
				t.Errorf("expected empty cell at [%d][%d], got %q", i, j, cell)
			}
		}
	}
}

func TestBuildEmptyBoard_2x4(t *testing.T) {
	board := buildEmptyBoard(2, 4)
	if len(board) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(board))
	}
	for i, row := range board {
		if len(row) != 4 {
			t.Fatalf("expected 4 cols in row %d, got %d", i, len(row))
		}
	}
}

func TestBuildEmptyBoard_ZeroRows(t *testing.T) {
	board := buildEmptyBoard(0, 3)
	if len(board) != 0 {
		t.Errorf("expected 0 rows, got %d", len(board))
	}
}
