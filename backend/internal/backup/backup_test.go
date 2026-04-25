package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pms/backend/internal/testutil"
)

func TestSnapshot_CreatesReadableCopy(t *testing.T) {
	db := testutil.OpenTestDB(t)
	dir := t.TempDir()
	path, err := Snapshot(context.Background(), db, dir, time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "pms-20250102T030405Z.db") {
		t.Fatalf("unexpected name: %s", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("backup file is empty")
	}
}

func TestPrune_KeepsHourlyAndDailyWindow(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2025, 1, 10, 12, 0, 0, 0, time.UTC)

	// Build synthetic snapshots across the last 10 days at 3 times per day.
	for d := 0; d < 10; d++ {
		for _, h := range []int{1, 12, 23} {
			ts := now.Add(-time.Duration(d) * 24 * time.Hour).Add(time.Duration(h-12) * time.Hour)
			name := "pms-" + ts.UTC().Format("20060102T150405Z") + ".db"
			if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	removed, err := Prune(dir, now, 5, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) == 0 {
		t.Fatal("expected some files pruned")
	}
	entries, _ := os.ReadDir(dir)
	// Left-over count should be bounded by hourly (≤5) + daily (≤7) − overlap.
	if len(entries) > 12 {
		t.Fatalf("retention exceeded: %d files remain", len(entries))
	}
	// A file from 9 days ago should definitely be gone.
	oldStamp := now.Add(-9 * 24 * time.Hour).Format("20060102T150405Z")
	for _, e := range entries {
		if strings.Contains(e.Name(), oldStamp) {
			t.Fatalf("expected old snapshot pruned: %s", e.Name())
		}
	}
}

func TestPrune_IgnoresForeignFiles(t *testing.T) {
	dir := t.TempDir()
	foreign := filepath.Join(dir, "README.txt")
	if err := os.WriteFile(foreign, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Prune(dir, time.Now().UTC(), 5, 7); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("foreign file removed: %v", err)
	}
}
