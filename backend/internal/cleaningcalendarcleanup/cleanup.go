package cleaningcalendarcleanup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"pms/backend/internal/cleaningcalendar"
	"pms/backend/internal/store"
)

const (
	stateTable = "cleaning_calendar_cleanup_state"
	itemTable  = "cleaning_calendar_cleanup_items"
)

type Runner struct {
	DB     *sql.DB
	Store  *store.Store
	Client interface {
		ListEvents(context.Context, string, time.Time, time.Time) ([]cleaningcalendar.GoogleCalendarEvent, error)
		DeleteEvent(context.Context, string, string) error
		Configured() bool
	}
	Now func() time.Time
}

func (r *Runner) Prepare(ctx context.Context) error {
	if err := createTables(ctx, r.DB); err != nil {
		return err
	}
	rows, err := r.DB.QueryContext(ctx, `
		SELECT p.id, p.timezone, trim(coalesce(g.calendar_id, ''))
		FROM properties p LEFT JOIN property_google_cleaning_settings g ON g.property_id = p.id
		WHERE trim(coalesce(g.calendar_id, '')) <> '' OR EXISTS
		  (SELECT 1 FROM cleaning_calendar_events e WHERE e.property_id = p.id AND e.raw_booking_block_id IS NOT NULL)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	now := r.now().UTC().Format(time.RFC3339)
	for rows.Next() {
		var id int64
		var zone, calendar string
		if err := rows.Scan(&id, &zone, &calendar); err != nil {
			return err
		}
		loc, err := time.LoadLocation(zone)
		if err != nil {
			return fmt.Errorf("property %d: invalid timezone %q", id, zone)
		}
		cutoff := r.now().In(loc).Format("2006-01-02")
		_, err = r.DB.ExecContext(ctx, `INSERT INTO cleaning_calendar_cleanup_state
			(property_id, cutoff_date, timezone, calendar_id, phase, created_at, updated_at)
			VALUES (?, ?, ?, NULLIF(?, ''), 'prepared', ?, ?)
			ON CONFLICT(property_id) DO UPDATE SET calendar_id = excluded.calendar_id, updated_at = excluded.updated_at`, id, cutoff, zone, calendar, now, now)
		if err != nil {
			return err
		}
	}
	return rows.Err()
}

func (r *Runner) Run(ctx context.Context) error {
	if r.Client == nil || !r.Client.Configured() {
		return errors.New("Google Calendar service account is not configured")
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT property_id, cutoff_date, timezone, coalesce(calendar_id, '') FROM `+stateTable+` WHERE phase <> 'complete' ORDER BY property_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var propertyID int64
		var cutoff, zone, calendar string
		if err := rows.Scan(&propertyID, &cutoff, &zone, &calendar); err != nil {
			return err
		}
		if strings.TrimSpace(calendar) == "" {
			if err := r.completeNoCalendar(ctx, propertyID); err != nil {
				return err
			}
			continue
		}
		loc, err := time.LoadLocation(zone)
		if err != nil {
			return fmt.Errorf("property %d: invalid timezone %q", propertyID, zone)
		}
		var currentCalendar string
		if err := r.DB.QueryRowContext(ctx, `SELECT trim(coalesce(calendar_id, '')) FROM property_google_cleaning_settings WHERE property_id=?`, propertyID).Scan(&currentCalendar); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if currentCalendar != calendar {
			if _, err := r.DB.ExecContext(ctx, `DELETE FROM `+itemTable+` WHERE property_id=?`, propertyID); err != nil {
				return err
			}
			calendar = currentCalendar
			if _, err := r.DB.ExecContext(ctx, `UPDATE `+stateTable+` SET calendar_id=NULLIF(?, ''), phase='prepared', updated_at=? WHERE property_id=?`, calendar, r.now().UTC().Format(time.RFC3339), propertyID); err != nil {
				return err
			}
			if calendar == "" {
				if err := r.completeNoCalendar(ctx, propertyID); err != nil {
					return err
				}
				continue
			}
		}
		start, _ := time.ParseInLocation("2006-01-02", cutoff, loc)
		end := time.Date(9999, 12, 31, 0, 0, 0, 0, loc)
		events, err := r.Client.ListEvents(ctx, calendar, start, end)
		if err != nil {
			return r.recordStateError(ctx, propertyID, err)
		}
		if err := r.discover(ctx, propertyID, calendar, cutoff, events); err != nil {
			return r.recordStateError(ctx, propertyID, err)
		}
		if _, err := r.DB.ExecContext(ctx, `UPDATE `+stateTable+` SET phase='discovered', last_error=NULL, updated_at=? WHERE property_id=?`, r.now().UTC().Format(time.RFC3339), propertyID); err != nil {
			return err
		}
		if err := r.deleteQueued(ctx, propertyID, calendar, cutoff, loc); err != nil {
			return r.recordStateError(ctx, propertyID, err)
		}
		if _, err := r.DB.ExecContext(ctx, `UPDATE `+stateTable+` SET phase='complete', last_error=NULL, updated_at=? WHERE property_id=? AND NOT EXISTS (SELECT 1 FROM `+itemTable+` i WHERE i.property_id=?)`, r.now().UTC().Format(time.RFC3339), propertyID, propertyID); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (r *Runner) Status(ctx context.Context) ([]string, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT property_id, phase, cutoff_date, coalesce(calendar_id, ''), (SELECT count(*) FROM `+itemTable+` i WHERE i.property_id=s.property_id), coalesce(last_error, '') FROM `+stateTable+` s ORDER BY property_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id, phase, cutoff, calendar, errText string
		var pending int
		if err := rows.Scan(&id, &phase, &cutoff, &calendar, &pending, &errText); err != nil {
			return nil, err
		}
		out = append(out, fmt.Sprintf("property=%s phase=%s pending=%d cutoff=%s calendar=%s error=%s", id, phase, pending, cutoff, calendar, errText))
	}
	return out, rows.Err()
}

func (r *Runner) discover(ctx context.Context, propertyID int64, calendar, cutoff string, events []cleaningcalendar.GoogleCalendarEvent) error {
	var zone string
	if err := r.DB.QueryRowContext(ctx, `SELECT timezone FROM `+stateTable+` WHERE property_id=?`, propertyID).Scan(&zone); err != nil {
		return err
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.Status == "cancelled" || strings.TrimSpace(event.ID) == "" || event.Start.IsZero() || event.Start.In(loc).Format("2006-01-02") < cutoff {
			continue
		}
		priv := event.PrivateProperties
		metadataRaw := priv != nil && priv["pms_property_id"] == fmt.Sprintf("%d", propertyID) && strings.TrimSpace(priv["pms_raw_booking_block_id"]) != ""
		metadataNamed := priv != nil && strings.TrimSpace(priv["pms_named_stay_id"]) != ""
		var localRaw, localNamed int
		if err := r.DB.QueryRowContext(ctx, `SELECT count(*) FROM cleaning_calendar_events WHERE property_id=? AND google_calendar_id=? AND google_event_id=? AND raw_booking_block_id IS NOT NULL`, propertyID, calendar, event.ID).Scan(&localRaw); err != nil {
			return err
		}
		if err := r.DB.QueryRowContext(ctx, `SELECT count(*) FROM cleaning_calendar_events WHERE property_id=? AND google_calendar_id=? AND google_event_id=? AND named_stay_id IS NOT NULL`, propertyID, calendar, event.ID).Scan(&localNamed); err != nil {
			return err
		}
		if (metadataNamed || localNamed > 0) && (metadataRaw || localRaw > 0) {
			return errors.New("ambiguous named/raw cleaning ownership")
		}
		if !metadataRaw && localRaw == 0 {
			continue
		}
		_, err := r.DB.ExecContext(ctx, `INSERT OR IGNORE INTO `+itemTable+` (property_id, calendar_id, google_event_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, propertyID, calendar, event.ID, r.now().UTC().Format(time.RFC3339), r.now().UTC().Format(time.RFC3339))
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) deleteQueued(ctx context.Context, propertyID int64, calendar, cutoff string, loc *time.Location) error {
	rows, err := r.DB.QueryContext(ctx, `SELECT id, google_event_id FROM `+itemTable+` WHERE property_id=? AND calendar_id=? ORDER BY id`, propertyID, calendar)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var googleID string
		if err := rows.Scan(&id, &googleID); err != nil {
			return err
		}
		var currentCalendar string
		if err := r.DB.QueryRowContext(ctx, `SELECT trim(coalesce(calendar_id, '')) FROM property_google_cleaning_settings WHERE property_id=?`, propertyID).Scan(&currentCalendar); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if currentCalendar != calendar {
			return errors.New("configured cleaning calendar changed; rerun cleanup")
		}
		start, _ := time.ParseInLocation("2006-01-02", cutoff, loc)
		remote, err := r.Client.ListEvents(ctx, calendar, start, time.Date(9999, 12, 31, 0, 0, 0, 0, loc))
		if err != nil {
			return err
		}
		var found *cleaningcalendar.GoogleCalendarEvent
		for i := range remote {
			if remote[i].ID == googleID {
				found = &remote[i]
				break
			}
		}
		if found == nil {
			if _, err := r.DB.ExecContext(ctx, `DELETE FROM `+itemTable+` WHERE id=?`, id); err != nil {
				return err
			}
			continue
		}
		if found.Start.In(loc).Format("2006-01-02") < cutoff {
			continue
		}
		private := found.PrivateProperties
		if private == nil || private["pms_property_id"] != fmt.Sprintf("%d", propertyID) || strings.TrimSpace(private["pms_raw_booking_block_id"]) == "" || strings.TrimSpace(private["pms_named_stay_id"]) != "" {
			return errors.New("remote cleaning ownership changed; refusing deletion")
		}
		if err := r.Client.DeleteEvent(ctx, calendar, googleID); err != nil {
			_, _ = r.DB.ExecContext(ctx, `UPDATE `+itemTable+` SET attempts=attempts+1, last_error=?, updated_at=? WHERE id=?`, err.Error(), r.now().UTC().Format(time.RFC3339), id)
			return err
		}
		if _, err := r.DB.ExecContext(ctx, `DELETE FROM `+itemTable+` WHERE id=?`, id); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (r *Runner) completeNoCalendar(ctx context.Context, propertyID int64) error {
	_, err := r.DB.ExecContext(ctx, `DELETE FROM `+itemTable+` WHERE property_id=?`, propertyID)
	if err != nil {
		return err
	}
	_, err = r.DB.ExecContext(ctx, `UPDATE `+stateTable+` SET phase='complete', last_error='no current calendar configured', updated_at=? WHERE property_id=?`, r.now().UTC().Format(time.RFC3339), propertyID)
	return err
}
func (r *Runner) recordStateError(ctx context.Context, propertyID int64, cause error) error {
	_, _ = r.DB.ExecContext(ctx, `UPDATE `+stateTable+` SET last_error=?, updated_at=? WHERE property_id=?`, cause.Error(), r.now().UTC().Format(time.RFC3339), propertyID)
	return cause
}
func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func createTables(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS cleaning_calendar_cleanup_state (property_id INTEGER PRIMARY KEY REFERENCES properties(id) ON DELETE CASCADE, cutoff_date TEXT NOT NULL, timezone TEXT NOT NULL, calendar_id TEXT, phase TEXT NOT NULL CHECK (phase IN ('prepared','discovered','complete')), last_error TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL); CREATE TABLE IF NOT EXISTS cleaning_calendar_cleanup_items (id INTEGER PRIMARY KEY AUTOINCREMENT, property_id INTEGER NOT NULL REFERENCES cleaning_calendar_cleanup_state(property_id) ON DELETE CASCADE, calendar_id TEXT NOT NULL, google_event_id TEXT NOT NULL CHECK (length(trim(google_event_id)) > 0), attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0), last_error TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, UNIQUE(property_id, calendar_id, google_event_id));`)
	return err
}
