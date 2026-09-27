package boot

import (
	"testing"

	sqliteClient "github.com/robbiebyrd/indri/internal/clients/sqlite"
	"github.com/robbiebyrd/indri/internal/injector"
)

func TestCloseResources_ClosesSQLDB(t *testing.T) {
	db, err := sqliteClient.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite Open: %v", err)
	}

	closeResources(&injector.Injector{
		ReposInjector:   &injector.ReposInjector{SQLDB: db},
		ClientsInjector: &injector.ClientsInjector{},
	})

	if err := db.Ping(); err == nil {
		t.Fatal("Ping after closeResources: want a closed-database error, got nil")
	}
}
