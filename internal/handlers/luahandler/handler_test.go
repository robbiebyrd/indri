package luahandler

import (
	"testing"

	lua "github.com/yuin/gopher-lua"
)

// fakeConn implements transport.Conn for tests that need a connection
// but don't exercise the real transport.
type fakeConn struct {
	keys    map[string]any
	written [][]byte
	closed  bool
}

func newFakeConn() *fakeConn { return &fakeConn{keys: make(map[string]any)} }

func (f *fakeConn) Get(key string) (any, bool)  { v, ok := f.keys[key]; return v, ok }
func (f *fakeConn) Set(key string, value any)   { f.keys[key] = value }
func (f *fakeConn) UnSet(key string)            { delete(f.keys, key) }
func (f *fakeConn) Write(msg []byte) error      { f.written = append(f.written, msg); return nil }
func (f *fakeConn) Close() error                { f.closed = true; return nil }
func (f *fakeConn) IsClosed() bool              { return f.closed }

func TestBuildCallerTable_AllFieldsPresent(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tbl := buildCallerTable(L, "game-1", "team-2", "user-3")

	cases := map[string]string{
		"gameId": "game-1",
		"teamId": "team-2",
		"userId": "user-3",
	}
	for field, want := range cases {
		if got := tbl.RawGetString(field).String(); got != want {
			t.Errorf("%s: want %q, got %q", field, want, got)
		}
	}
}
