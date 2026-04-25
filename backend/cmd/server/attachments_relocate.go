package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"pms/backend/internal/metrics"
)

// relocateLegacyFinanceAttachments migrates finance transaction attachments
// from the legacy `attachments/<propertyID>/<timestamp>_<file>` layout to the
// architecture-specified `attachments/<propertyID>/<transactionID>/<file>`
// layout. It is idempotent: rows already matching the new layout are skipped.
// Files that cannot be located on disk are left untouched (the DB path is
// rewritten to the new canonical form so future regenerations store under the
// expected directory, but we never discard work in progress).
func relocateLegacyFinanceAttachments(db *sql.DB, baseDir string) error {
	if baseDir == "" {
		baseDir = "./data"
	}
	ctx := context.Background()
	rows, err := db.QueryContext(ctx, `
		SELECT id, property_id, attachment_path
		FROM finance_transactions
		WHERE attachment_path IS NOT NULL AND attachment_path <> ''`)
	if err != nil {
		return fmt.Errorf("list finance attachments: %w", err)
	}
	type item struct {
		id       int64
		property int64
		current  string
	}
	var todo []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.property, &it.current); err != nil {
			rows.Close()
			return fmt.Errorf("scan finance attachment row: %w", err)
		}
		todo = append(todo, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate finance attachments: %w", err)
	}

	moved := 0
	skipped := 0
	for _, it := range todo {
		newRel, changed, err := migrateAttachmentPath(baseDir, it.property, it.id, it.current)
		if err != nil {
			log.Printf("attachment migration: tx %d: %v", it.id, err)
			continue
		}
		if !changed {
			skipped++
			continue
		}
		if _, err := db.ExecContext(ctx, `
			UPDATE finance_transactions
			SET attachment_path = ?, updated_at = ?
			WHERE id = ?`, newRel, time.Now().UTC().Format(time.RFC3339), it.id); err != nil {
			log.Printf("attachment migration: update tx %d: %v", it.id, err)
			continue
		}
		moved++
	}
	if moved > 0 || skipped > 0 {
		log.Printf("attachment migration: relocated=%d already_ok=%d", moved, skipped)
	}
	metrics.RecordAttachmentRelocation("relocated", moved)
	metrics.RecordAttachmentRelocation("already_ok", skipped)
	return nil
}

// migrateAttachmentPath computes the canonical storage path for an attachment
// and, when the current path differs, moves the file on disk. Returns the new
// relative path (stored in the DB) and whether a change was applied.
func migrateAttachmentPath(baseDir string, propertyID, transactionID int64, current string) (string, bool, error) {
	current = strings.TrimSpace(current)
	if current == "" {
		return "", false, nil
	}
	if strings.Contains(current, "..") {
		return "", false, fmt.Errorf("refusing path with parent traversal: %q", current)
	}
	clean := path.Clean(strings.ReplaceAll(current, "\\", "/"))
	parts := strings.Split(clean, "/")
	// Strip a leading "attachments/<propertyID>" prefix if present.
	if len(parts) >= 2 && parts[0] == "attachments" {
		parts = parts[1:]
		if len(parts) >= 1 && parts[0] == fmt.Sprintf("%d", propertyID) {
			parts = parts[1:]
		}
	}
	if len(parts) == 0 {
		return "", false, fmt.Errorf("attachment path empty after normalization: %q", current)
	}
	// Already in the new layout: <transactionID>/<file>.
	if len(parts) >= 2 && parts[0] == fmt.Sprintf("%d", transactionID) {
		return "", false, nil
	}
	// Legacy layout: the last segment is the filename. Strip any `<timestamp>_` prefix.
	filename := parts[len(parts)-1]
	if idx := strings.Index(filename, "_"); idx > 0 {
		if _, err := parseLegacyTimestamp(filename[:idx]); err == nil {
			filename = filename[idx+1:]
		}
	}
	if filename == "" || filename == "." || strings.Contains(filename, "/") {
		return "", false, fmt.Errorf("cannot derive filename from %q", current)
	}
	newRel := filepath.ToSlash(filepath.Join("attachments", fmt.Sprintf("%d", propertyID), fmt.Sprintf("%d", transactionID), filename))
	if newRel == clean || newRel == current {
		return "", false, nil
	}
	oldFull := filepath.Join(baseDir, filepath.FromSlash(clean))
	newFull := filepath.Join(baseDir, filepath.FromSlash(newRel))
	if err := os.MkdirAll(filepath.Dir(newFull), 0o755); err != nil {
		return "", false, fmt.Errorf("mkdir %s: %w", filepath.Dir(newFull), err)
	}
	if _, err := os.Stat(oldFull); err == nil {
		if err := os.Rename(oldFull, newFull); err != nil {
			// Fall back to copy when rename across devices fails.
			if copyErr := copyFile(oldFull, newFull); copyErr != nil {
				return "", false, fmt.Errorf("move %s -> %s: %v (copy fallback: %v)", oldFull, newFull, err, copyErr)
			}
			_ = os.Remove(oldFull)
		}
	} else if !os.IsNotExist(err) {
		return "", false, fmt.Errorf("stat %s: %w", oldFull, err)
	}
	return newRel, true, nil
}

func parseLegacyTimestamp(s string) (time.Time, error) {
	// legacy layout used `<UnixNano>_<filename>`; accept decimal digits only.
	if s == "" {
		return time.Time{}, fmt.Errorf("empty")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return time.Time{}, fmt.Errorf("not a timestamp")
		}
	}
	return time.Unix(0, 0), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
