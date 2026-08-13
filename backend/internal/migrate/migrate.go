package migrate

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

//go:embed *.sql
var embeddedMigrations embed.FS

// manualMigrations are intentionally never applied during application startup.
// They remain part of Up for explicit operational execution and fresh-schema
// tests. Entries are exact versions so later ordinary migrations are not held
// behind a manual migration.
var manualMigrations = map[string]struct{}{
	"000039_legacy_occupancy_removal": {},
}

func Up(db *sql.DB) error {
	return up(db, false)
}

// UpAutomatic applies ordinary migrations while leaving explicitly manual or
// destructive migrations pending for an operator-controlled command.
func UpAutomatic(db *sql.DB) error {
	return up(db, true)
}

// UpStartup applies the final schema for a new database, while existing
// installations leave destructive migrations for the operator-controlled
// cleanup command.
func UpStartup(db *sql.DB) error {
	var initialized int
	if err := db.QueryRow(`
		SELECT count(*) FROM sqlite_schema
		WHERE type = 'table' AND name = 'schema_migrations'`).Scan(&initialized); err != nil {
		return err
	}
	if initialized == 0 {
		return Up(db)
	}
	return UpAutomatic(db)
}

func up(db *sql.DB, automatic bool) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(embeddedMigrations, ".")
	if err != nil {
		return err
	}
	var ups []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".up.sql") {
			ups = append(ups, e.Name())
		}
	}
	sort.Strings(ups)
	for _, name := range ups {
		version := strings.TrimSuffix(name, ".up.sql")
		if automatic {
			if _, manual := manualMigrations[version]; manual {
				continue
			}
		}
		var exists int
		if err := db.QueryRow(`SELECT 1 FROM schema_migrations WHERE version = ?`, version).Scan(&exists); err == nil {
			continue
		}
		body, err := embeddedMigrations.ReadFile(path.Join(".", name))
		if err != nil {
			return err
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migrate %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
