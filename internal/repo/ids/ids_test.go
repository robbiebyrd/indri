package ids

import (
	"regexp"
	"testing"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNew_ReturnsUUIDv4(t *testing.T) {
	id := New()
	if !uuidPattern.MatchString(id) {
		t.Fatalf("New() = %q, does not match UUIDv4 pattern", id)
	}
}

func TestNew_IsUnique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id := New()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate UUID %q at iteration %d", id, i)
		}
		seen[id] = struct{}{}
	}
}
