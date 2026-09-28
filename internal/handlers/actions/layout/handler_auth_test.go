package layout_test

import (
	"testing"

	"github.com/robbiebyrd/indri/internal/handlers/actions/layout"
	ht "github.com/robbiebyrd/indri/internal/handlers/handlertest"
)

func setGrid() map[string]interface{} {
	return map[string]interface{}{
		"code": "LAYOUT",
		"op":   "setGrid",
		"grid": map[string]interface{}{"cols": float64(10), "rows": float64(10)},
	}
}

// Players are keyed by slot, so the host must be found by the caller's slot.
func TestLayout_TheHostCanEditTheLayout(t *testing.T) {
	stores := ht.NewStores(t)
	g := stores.Game("LAYOUT", 2)
	_, hostSession := stores.Seat(g.ID, "host")
	conn := ht.NewConn(hostSession)
	i := stores.Injector(t, ht.NewInstance(conn), nil)

	if err := layout.New(i).Handle(conn, setGrid()); err != nil {
		t.Fatalf("host's layout edit refused: %v", err)
	}

	after := ht.Must(stores.Games.Get(g.ID))
	if _, ok := after.PublicData["layout"].(map[string]interface{})["grid"]; !ok {
		t.Fatalf("layout not written: %v", after.PublicData["layout"])
	}
}

func TestLayout_OnlyTheHostCanEditTheLayout(t *testing.T) {
	stores := ht.NewStores(t)
	g := stores.Game("LAYOUT", 2)
	stores.Seat(g.ID, "host")
	_, guestSession := stores.Seat(g.ID, "guest")
	conn := ht.NewConn(guestSession)
	i := stores.Injector(t, ht.NewInstance(conn), nil)

	if err := layout.New(i).Handle(conn, setGrid()); err == nil {
		t.Fatal("a player who isn't the host edited the layout")
	}
}
