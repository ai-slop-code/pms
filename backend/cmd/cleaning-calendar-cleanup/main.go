package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"pms/backend/internal/cleaningcalendar"
	"pms/backend/internal/cleaningcalendarcleanup"
	"pms/backend/internal/config"
	"pms/backend/internal/dbconn"
	"pms/backend/internal/store"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "prepare" && os.Args[1] != "run" && os.Args[1] != "status") {
		fmt.Fprintln(os.Stderr, "usage: cleaning-calendar-cleanup prepare|run|status")
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		fatal(err)
	}
	db, err := dbconn.Open(cfg.DatabaseURL)
	if err != nil {
		fatal(err)
	}
	defer db.Close()
	st := &store.Store{DB: db}
	owner := fmt.Sprintf("cleanup-%d", os.Getpid())
	acquired, err := st.TryAcquireJobLease(context.Background(), "cleaning_calendar_cleanup", owner, 30*time.Minute)
	if err != nil {
		fatal(err)
	}
	if !acquired {
		fatal(fmt.Errorf("another cleanup command holds the lease"))
	}
	defer st.ReleaseJobLease(context.Background(), "cleaning_calendar_cleanup", owner)
	client := loadClient()
	runner := &cleaningcalendarcleanup.Runner{DB: db, Store: st, Client: client}
	ctx := context.Background()
	switch os.Args[1] {
	case "prepare":
		err = runner.Prepare(ctx)
	case "run":
		err = runner.Run(ctx)
	case "status":
		var lines []string
		lines, err = runner.Status(ctx)
		for _, line := range lines {
			fmt.Println(line)
		}
	}
	if err != nil {
		fatal(err)
	}
}

func loadClient() *cleaningcalendar.ServiceAccountClient {
	raw := os.Getenv("PMS_GOOGLE_SERVICE_ACCOUNT_JSON")
	if raw == "" {
		if path := os.Getenv("PMS_GOOGLE_SERVICE_ACCOUNT_FILE"); path != "" {
			b, err := os.ReadFile(path)
			if err == nil {
				raw = string(b)
			}
		}
	}
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	client, _ := cleaningcalendar.NewServiceAccountClient([]byte(raw), nil)
	return client
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
