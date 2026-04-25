package api

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pms/backend/internal/ctxuser"
)

// getAdminBackup streams a gzipped tar archive containing a consistent
// snapshot of the SQLite database plus the on-disk data directory (invoices,
// attachments). The snapshot is produced via `VACUUM INTO`, which is WAL-safe
// and does not block concurrent writers.
//
// Access is restricted to super_admin. The archive is produced on the fly and
// never staged in full on disk — only the temporary SQLite snapshot file
// exists briefly before it is streamed and removed.
func (s *Server) getAdminBackup(w http.ResponseWriter, r *http.Request) {
	actor := ctxuser.From(r.Context())
	if actor == nil || actor.Role != "super_admin" {
		WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	if s.AdminBackupLimiter != nil {
		key := "admin_backup"
		if actor != nil {
			key = fmt.Sprintf("admin_backup:%d", actor.ID)
		}
		if !s.AdminBackupLimiter.Allow(key) {
			w.Header().Set("Retry-After", "30")
			WriteError(w, http.StatusTooManyRequests, "backup already in progress; try again shortly")
			return
		}
	}

	baseDir := strings.TrimSpace(s.DataDir)
	if baseDir == "" {
		baseDir = "./data"
	}
	tmpDir, err := os.MkdirTemp("", "pms-backup-*")
	if err != nil {
		log.Printf("admin_backup: mktemp: %v", err)
		WriteError(w, http.StatusInternalServerError, "backup failed")
		return
	}
	defer os.RemoveAll(tmpDir)

	snapshotPath := filepath.Join(tmpDir, "pms.db")
	if err := sqliteSnapshot(s.Store.DB, snapshotPath); err != nil {
		// Full error stays in server logs; the client gets an opaque message
		// so we don't leak filesystem paths or SQLite internals. See PMS_11/T2.12.
		log.Printf("admin_backup: snapshot: %v", err)
		WriteError(w, http.StatusInternalServerError, "backup failed")
		return
	}

	ts := time.Now().UTC().Format("20060102T150405Z")
	filename := fmt.Sprintf("pms-backup-%s.tar.gz", ts)
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")

	gz := gzip.NewWriter(w)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	if err := addFileToTar(tw, snapshotPath, "pms.db"); err != nil {
		s.audit(r, actor, "admin_backup", "backup", "", "failure")
		return
	}
	// Include the persistent data subdirectories. Missing directories are fine
	// — a fresh deployment may not have invoices or attachments yet.
	for _, rel := range []string{"invoices", "attachments"} {
		src := filepath.Join(baseDir, rel)
		info, statErr := os.Stat(src)
		if statErr != nil || !info.IsDir() {
			continue
		}
		if err := addDirToTar(tw, src, rel); err != nil {
			s.audit(r, actor, "admin_backup", "backup", rel, "failure")
			return
		}
	}
	s.audit(r, actor, "admin_backup", "backup", "", "success")
}

// sqliteSnapshot writes a consistent copy of the running database to dst via
// `VACUUM INTO`. This is SQLite's documented online-backup path and is safe
// while the primary is accepting writes.
func sqliteSnapshot(db *sql.DB, dst string) error {
	// Ensure the target path does not already exist — VACUUM INTO fails when
	// the destination file is present.
	if _, err := os.Stat(dst); err == nil {
		if err := os.Remove(dst); err != nil {
			return fmt.Errorf("remove stale snapshot: %w", err)
		}
	}
	if _, err := db.Exec(`VACUUM INTO ?`, dst); err != nil {
		return err
	}
	return nil
}

func addFileToTar(tw *tar.Writer, src, nameInArchive string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	hdr.Name = nameInArchive
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}

func addDirToTar(tw *tar.Writer, root, archivePrefix string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join(archivePrefix, rel))
		if info.IsDir() {
			if rel == "." {
				return nil
			}
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			hdr.Name = name + "/"
			return tw.WriteHeader(hdr)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return addFileToTar(tw, path, name)
	})
}
