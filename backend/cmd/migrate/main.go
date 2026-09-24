// pms-migrate runs explicit operator-only schema transitions.
package main

import (
	"fmt"
	"io"
	"os"

	"pms/backend/internal/config"
	"pms/backend/internal/dbconn"
	"pms/backend/internal/migrate"
)

const usage = "usage: pms-migrate cleaning-calendar\nRun offline after cleaning-calendar-cleanup prepare and run have completed.\nUses DATABASE_PATH and the backend environment; applies only migration 000040."

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, err := fmt.Fprintln(out, usage)
		return err
	}
	if len(args) != 1 || args[0] != "cleaning-calendar" {
		return fmt.Errorf("%s", usage)
	}
	path := os.Getenv("DATABASE_PATH")
	if path == "" {
		return fmt.Errorf("DATABASE_PATH must identify the existing production database")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("check existing database: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("DATABASE_PATH must identify an existing regular file")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := dbconn.Open(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := migrate.UpCleaningCalendar(db); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, "cleaning-calendar migration 000040 is complete")
	return err
}
