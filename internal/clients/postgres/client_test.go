package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/robbiebyrd/indri/internal/clients/postgres"
)

func TestOpen_EmptyURI_ReturnsError(t *testing.T) {
	if _, err := postgres.Open(context.Background(), ""); err == nil {
		t.Fatal("Open with empty URI: want error, got nil")
	}
}

func TestOpen_CapsOpenConnections(t *testing.T) {
	uri := os.Getenv("INDRI_TEST_POSTGRES_URI")
	if uri == "" {
		t.Skip("INDRI_TEST_POSTGRES_URI not set; skipping Postgres integration test")
	}

	db, err := postgres.Open(context.Background(), uri)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if got := db.Stats().MaxOpenConnections; got != postgres.MaxOpenConns {
		t.Errorf("MaxOpenConnections: want %d, got %d", postgres.MaxOpenConns, got)
	}
}
