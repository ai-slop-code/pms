// Package backup takes consistent point-in-time snapshots of the PMS SQLite
// database and manages a small local retention window. Off-host copy is the
// operator's responsibility (documented in docs/deployment/backup-runbook.md);
// this package only owns the atomic local snapshot + retention pruning.
package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Config controls snapshot location and retention. Defaults are resolved by
// the caller; the scheduler only runs when Interval > 0.
type Config struct {
	Dir             string
	Interval        time.Duration
	KeepHourly      int
	KeepDaily       int
	SnapshotTimeout time.Duration
}

// Snapshot writes a hot copy of the live database using SQLite's
// `VACUUM INTO` and returns the resulting absolute path. `VACUUM INTO` is
// safe to run concurrently with writers — it acquires only a reader lock
// and produces a self-consistent file without the WAL hazard that a raw
// file copy would have.
func Snapshot(ctx context.Context, db *sql.DB, dir string, now time.Time) (string, error) {
	if db == nil {
		return "", errors.New("backup: nil db")
	}
	if dir == "" {
		return "", errors.New("backup: empty dir")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("backup: mkdir: %w", err)
	}
	name := fmt.Sprintf("pms-%s.db", now.UTC().Format("20060102T150405Z"))
	final := filepath.Join(dir, name)
	tmp := final + ".part"
	// Defensive: remove any stale partial from a previous crash.
	_ = os.Remove(tmp)
	// SQLite requires a literal path in VACUUM INTO — parameter binds are
	// rejected. The path comes from server config (never user input), so
	// string interpolation here is acceptable; still escape single quotes.
	safe := strings.ReplaceAll(tmp, "'", "''")
	if _, err := db.ExecContext(ctx, "VACUUM INTO '"+safe+"'"); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("backup: vacuum into: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("backup: rename: %w", err)
	}
	return final, nil
}

// Prune deletes snapshots beyond the hourly/daily retention window.
// Hourly window keeps the most recent `keepHourly` files from the last 24h;
// daily window keeps one snapshot per day (the oldest in each UTC day) for
// the past `keepDaily` days. Everything else is removed.
func Prune(dir string, now time.Time, keepHourly, keepDaily int) (removed []string, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	type snap struct {
		path string
		ts   time.Time
	}
	var snaps []snap
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "pms-") || !strings.HasSuffix(name, ".db") {
			continue
		}
		stamp := strings.TrimSuffix(strings.TrimPrefix(name, "pms-"), ".db")
		t, parseErr := time.Parse("20060102T150405Z", stamp)
		if parseErr != nil {
			continue
		}
		snaps = append(snaps, snap{filepath.Join(dir, name), t.UTC()})
	}
	// Newest first.
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].ts.After(snaps[j].ts) })

	keep := make(map[string]bool, len(snaps))
	// Hourly bucket: newest N within the last 24h.
	hourCutoff := now.Add(-24 * time.Hour)
	kept := 0
	for _, s := range snaps {
		if kept >= keepHourly {
			break
		}
		if s.ts.After(hourCutoff) {
			keep[s.path] = true
			kept++
		}
	}
	// Daily bucket: one per day (newest of the day) for the last keepDaily days.
	dayCutoff := now.Add(-time.Duration(keepDaily) * 24 * time.Hour)
	seenDay := make(map[string]bool)
	for _, s := range snaps {
		if s.ts.Before(dayCutoff) {
			break
		}
		day := s.ts.Format("2006-01-02")
		if seenDay[day] {
			continue
		}
		seenDay[day] = true
		keep[s.path] = true
	}

	for _, s := range snaps {
		if keep[s.path] {
			continue
		}
		if rmErr := os.Remove(s.path); rmErr != nil {
			return removed, rmErr
		}
		removed = append(removed, s.path)
	}
	return removed, nil
}
