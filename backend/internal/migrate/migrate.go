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
	"000039_legacy_occupancy_removal":          {},
	"000040_named_stay_only_cleaning_calendar": {},
}

func Up(db *sql.DB) error {
	return up(db, false, "")
}

// UpAutomatic applies ordinary migrations while leaving explicitly manual or
// destructive migrations pending for an operator-controlled command.
func UpAutomatic(db *sql.DB) error {
	return up(db, true, "")
}

// UpCleaningCalendar explicitly completes the PMS-22 schema transition after
// offline cleanup. It cannot apply PMS-21 or unrelated pending migrations.
func UpCleaningCalendar(db *sql.DB) error {
	var installed int
	if err := db.QueryRow(`SELECT count(*) FROM schema_migrations WHERE version = '000039_legacy_occupancy_removal'`).Scan(&installed); err != nil {
		return fmt.Errorf("check PMS-21 migration prerequisite: %w", err)
	}
	if installed != 1 {
		return fmt.Errorf("migration 000039_legacy_occupancy_removal must be completed through the PMS-21 procedure first")
	}
	return up(db, false, "000040_named_stay_only_cleaning_calendar")
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

func up(db *sql.DB, automatic bool, onlyVersion string) error {
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
		if onlyVersion != "" && version != onlyVersion {
			continue
		}
		if automatic {
			if _, manual := manualMigrations[version]; manual {
				continue
			}
		}
		var exists int
		if err := db.QueryRow(`SELECT 1 FROM schema_migrations WHERE version = ?`, version).Scan(&exists); err == nil {
			continue
		} else if err != sql.ErrNoRows {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		body, err := embeddedMigrations.ReadFile(path.Join(".", name))
		if err != nil {
			return err
		}
		if version == "000040_named_stay_only_cleaning_calendar" {
			if err := ensurePMS22CleanupReady(db); err != nil {
				return err
			}
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		// PMS-22 rebuilds both the event parent and its log children. Enforce
		// their foreign keys at commit, once both replacement tables exist.
		if version == "000040_named_stay_only_cleaning_calendar" {
			if _, err := tx.Exec(`PRAGMA defer_foreign_keys = ON`); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("defer PMS-22 rebuild foreign keys: %w", err)
			}
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

func ensurePMS22CleanupReady(db *sql.DB) error {
	var events int
	if err := db.QueryRow(`SELECT count(*) FROM cleaning_calendar_events`).Scan(&events); err != nil {
		return nil
	}
	if events == 0 {
		return nil
	}
	var tables int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE type='table' AND name='cleaning_calendar_cleanup_state'`).Scan(&tables); err != nil || tables == 0 {
		return fmt.Errorf("PMS-22 cleanup is incomplete; run cleaning-calendar-cleanup prepare and run")
	}
	var incomplete int
	err := db.QueryRow(`SELECT count(*) FROM properties p WHERE EXISTS (SELECT 1 FROM cleaning_calendar_events e WHERE e.property_id=p.id) AND NOT EXISTS (SELECT 1 FROM cleaning_calendar_cleanup_state s WHERE s.property_id=p.id AND s.phase='complete' AND coalesce(trim(s.calendar_id),'')=coalesce((SELECT trim(g.calendar_id) FROM property_google_cleaning_settings g WHERE g.property_id=p.id),'') AND NOT EXISTS (SELECT 1 FROM cleaning_calendar_cleanup_items i WHERE i.property_id=p.id))`).Scan(&incomplete)
	if err != nil || incomplete > 0 {
		return fmt.Errorf("PMS-22 cleanup is incomplete; run cleaning-calendar-cleanup prepare and run")
	}
	return nil
}
