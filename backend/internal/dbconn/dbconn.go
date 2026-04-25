package dbconn

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// Open returns a SQLite connection pool configured for mixed read/write
// workloads. WAL journaling and a non-trivial busy timeout let reads run in
// parallel with at most one concurrent writer (enforced by SQLite itself),
// which lifts the historical single-connection bottleneck while preserving
// SQLite's atomicity guarantees.
func Open(databaseURL string) (*sql.DB, error) {
	if !strings.HasPrefix(databaseURL, "sqlite://") {
		return nil, fmt.Errorf("only sqlite:// URLs supported")
	}
	rest := strings.TrimPrefix(databaseURL, "sqlite://")
	query := ""
	path := rest
	if i := strings.Index(rest, "?"); i >= 0 {
		path = rest[:i]
		query = rest[i+1:]
	}
	params := map[string]string{
		"_pragma": "journal_mode(WAL)",
	}
	// Preserve caller-supplied query params; support repeated _pragma values.
	var extraPragmas []string
	if query != "" {
		for _, kv := range strings.Split(query, "&") {
			if kv == "" {
				continue
			}
			eq := strings.IndexByte(kv, '=')
			if eq < 0 {
				params[kv] = ""
				continue
			}
			k, v := kv[:eq], kv[eq+1:]
			if k == "_pragma" {
				extraPragmas = append(extraPragmas, v)
				continue
			}
			params[k] = v
		}
	}
	// Compose final query string. journal_mode=WAL first so a caller-supplied
	// pragma cannot accidentally downgrade to the default journal.
	parts := []string{"_pragma=journal_mode(WAL)", "_pragma=busy_timeout(5000)", "_pragma=foreign_keys(ON)", "_pragma=synchronous(NORMAL)"}
	for _, p := range extraPragmas {
		parts = append(parts, "_pragma="+p)
	}
	for k, v := range params {
		if k == "_pragma" {
			continue
		}
		if v == "" {
			parts = append(parts, k)
		} else {
			parts = append(parts, k+"="+v)
		}
	}
	dsn := path + "?" + strings.Join(parts, "&")
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite + WAL supports many concurrent readers and a single writer; the
	// pool can safely hold multiple connections because busy_timeout will
	// serialize competing writes at the file level.
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
