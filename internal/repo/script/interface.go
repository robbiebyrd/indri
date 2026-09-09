package script

import "github.com/robbiebyrd/indri/internal/models"

// Storer is the contract a script store must satisfy. The assertion below
// keeps it in step with *Store: change one without the other and the build
// fails.
var _ Storer = (*Store)(nil)

type Storer interface {
	Get() *models.Script
}
