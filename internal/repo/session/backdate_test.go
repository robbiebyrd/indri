package session

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// backdate makes a stored session look created at createdAt, the way time
// passing would. Each backend keeps the creation time differently, so each
// needs its own hook.
func backdate(t *testing.T, store Storer, id string, createdAt time.Time) {
	t.Helper()

	switch s := store.(type) {
	case *MemoryStore:
		s.mu.Lock()
		s.byID[id].CreatedAt = createdAt
		s.mu.Unlock()
	case *SQLStore:
		res, err := s.db.Exec(s.dialect.Rebind(`UPDATE sessions SET created_at = ? WHERE id = ?`), createdAt.UnixNano(), id)
		if err != nil {
			t.Fatalf("backdating session %s: %v", id, err)
		}

		if n, _ := res.RowsAffected(); n != 1 {
			t.Fatalf("backdating session %s touched %d rows, want 1", id, n)
		}
	case *Store:
		res, err := s.collection.Collection().UpdateOne(context.Background(),
			bson.D{{Key: "_id", Value: id}},
			bson.D{{Key: "$set", Value: bson.D{{Key: "createdAt", Value: createdAt}}}})
		if err != nil {
			t.Fatalf("backdating session %s: %v", id, err)
		}

		if res.MatchedCount != 1 {
			t.Fatalf("backdating session %s matched %d documents, want 1", id, res.MatchedCount)
		}
	default:
		t.Fatalf("no backdating hook for %T", store)
	}
}
