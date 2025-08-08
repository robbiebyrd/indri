package script

import "github.com/robbiebyrd/indri/internal/models"

type Storer interface {
	Get() *models.Script
}
