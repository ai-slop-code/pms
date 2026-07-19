package dbconn

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenReadOnlyReadsWithoutAllowingMutationOrCreatingSidecars(t *testing.T) {
	path := filepath.Join(t.TempDir(), "read-only.db")
	writer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT); INSERT INTO items (name) VALUES ('one')`); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := OpenReadOnly("sqlite://" + path)
	if err != nil {
		t.Fatal(err)
	}
	var count, queryOnly int
	if err := db.QueryRow(`SELECT COUNT(*) FROM items`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("item count = %d, want 1", count)
	}
	if err := db.QueryRow(`PRAGMA query_only`).Scan(&queryOnly); err != nil {
		t.Fatal(err)
	}
	if queryOnly != 1 {
		t.Fatalf("query_only = %d, want 1", queryOnly)
	}
	if _, err := db.Exec(`INSERT INTO items (name) VALUES ('two')`); err == nil {
		t.Fatal("insert through read-only connection unexpectedly succeeded")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	for _, suffix := range []string{"-journal", "-shm", "-wal"} {
		if _, err := os.Stat(path + suffix); !os.IsNotExist(err) {
			t.Fatalf("SQLite sidecar %s was created (stat error: %v)", suffix, err)
		}
	}
}

func TestOpenReadOnlyDoesNotCreateMissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	if db, err := OpenReadOnly("sqlite://" + path); err == nil {
		_ = db.Close()
		t.Fatal("opening a missing database read-only unexpectedly succeeded")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("missing database was created (stat error: %v)", err)
	}
}
