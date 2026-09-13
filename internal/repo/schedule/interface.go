package schedule

import "time"

// Storer is the contract a schedule store must satisfy. The assertions below
// keep it in step with both backends: change one without the other and the
// build fails.
var (
	_ Storer = (*Store)(nil)
	_ Storer = (*MemoryStore)(nil)
)

type Storer interface {
	InstanceID() string
	Schedule(create CreateEntry) (*Entry, error)
	Get(id string) (*Entry, error)
	ClaimDue(now time.Time) (*Entry, error)
	RenewLease(id string) (*Entry, error)
	Complete(id string) error
	Fail(id string, reason string) (*Entry, error)
	MarkDead(id string, reason string) error
	Cancel(id string) error
	CancelForGame(gameID string) (int64, error)
}
