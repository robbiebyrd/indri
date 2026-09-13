package boot

import (
	"strings"
	"testing"
	"time"

	"github.com/robbiebyrd/indri/internal/handlers/actions/script"
	"github.com/robbiebyrd/indri/internal/services/lock"
)

// TestCheckScriptDeadline_AcceptsTheShippedConstants is the assertion that
// actually matters: the pair of values this server is built with must be a
// legal pair. It is what turns a future edit to either constant into a failing
// test rather than a production hazard nobody is looking for.
func TestCheckScriptDeadline_AcceptsTheShippedConstants(t *testing.T) {
	if err := checkScriptDeadline(script.InvocationTimeout, lock.DefaultRedisLeaseTTL); err != nil {
		t.Fatalf("the shipped script deadline and lock lease are not a legal pair: %v", err)
	}
}

// A deadline that crowds the lease must be refused, and the refusal has to
// explain the coupling — an operator who raised the deadline needs to know the
// lease is why they cannot.
func TestCheckScriptDeadline_RefusesTooLittleHeadroom(t *testing.T) {
	tests := map[string]struct {
		deadline time.Duration
		lease    time.Duration
	}{
		"deadline larger than the lease":  {deadline: 20 * time.Second, lease: 10 * time.Second},
		"deadline equal to the lease":     {deadline: 10 * time.Second, lease: 10 * time.Second},
		"headroom just under the minimum": {deadline: time.Second + 1, lease: minLeaseHeadroom * time.Second},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := checkScriptDeadline(test.deadline, test.lease)
			if err == nil {
				t.Fatalf("a %v deadline under a %v lease was accepted", test.deadline, test.lease)
			}

			for _, want := range []string{"deadline", "lease"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the error %q does not mention the %s", err.Error(), want)
				}
			}
		})
	}
}

// Exactly the minimum headroom is legal. Without this the boundary could be
// off by one in the refusing direction and every test above would still pass.
func TestCheckScriptDeadline_AcceptsExactlyTheMinimumHeadroom(t *testing.T) {
	if err := checkScriptDeadline(time.Second, minLeaseHeadroom*time.Second); err != nil {
		t.Fatalf("exactly %dx headroom was refused: %v", minLeaseHeadroom, err)
	}
}

// A non-positive deadline would make the ratio check vacuously true, so it is
// rejected on its own terms rather than slipping through.
func TestCheckScriptDeadline_RefusesANonPositiveDeadline(t *testing.T) {
	for _, deadline := range []time.Duration{0, -time.Second} {
		if err := checkScriptDeadline(deadline, lock.DefaultRedisLeaseTTL); err == nil {
			t.Errorf("a %v deadline was accepted", deadline)
		}
	}
}
