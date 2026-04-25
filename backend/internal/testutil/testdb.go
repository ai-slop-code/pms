package testutil

import (
	"database/sql"
	"path/filepath"
	"testing"

	"pms/backend/internal/dbconn"
	"pms/backend/internal/migrate"
)

func OpenTestDB(t *testing.T) *sql.DB {
	t.Helper()
	p, err := filepath.Abs(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := dbconn.Open("sqlite://" + p + "?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := migrate.Up(db); err != nil {
		t.Fatal(err)
	}
	return db
}
