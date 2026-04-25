package occupancy

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"pms/backend/internal/otelx"
	"pms/backend/internal/store"
)

const (
	defaultSyncHTTPTimeout = 60 * time.Second
	maxICSBodyBytes        = 20 << 20
	maxParseErrorsShown    = 3
	maxSyncErrorLength     = 900
)

// Service runs ICS fetch and occupancy normalization.
type Service struct {
	Store *store.Store
	HTTP  *http.Client
}

// SyncProperty fetches the property ICS URL and updates occupancies. Idempotent per UID.
func (s *Service) SyncProperty(ctx context.Context, propertyID int64, trigger string) error {
	if s.HTTP == nil {
		s.HTTP = &http.Client{Timeout: defaultSyncHTTPTimeout, Transport: otelx.HTTPTransport(nil)}
	}
	prop, err := s.Store.GetProperty(ctx, propertyID)
	if err != nil {
		return err
	}
	if !prop.Active {
		return fmt.Errorf("property_inactive")
	}
	sec, err := s.Store.GetPropertySecrets(ctx, propertyID)
	if err != nil {
		return err
	}
	src, err := s.Store.GetOccupancySource(ctx, propertyID)
	if err != nil {
		return err
	}
	if !src.Active || !sec.BookingICSURL.Valid || strings.TrimSpace(sec.BookingICSURL.String) == "" {
		runID, _ := s.Store.StartOccupancySyncRun(ctx, propertyID, trigger)
		msg := "ics_url_not_configured_or_source_inactive"
		_ = s.Store.FinishOccupancySyncRun(ctx, runID, "failure", &msg, nil, 0, 0)
		return fmt.Errorf("%s", msg)
	}
	url := strings.TrimSpace(sec.BookingICSURL.String)

	runID, err := s.Store.StartOccupancySyncRun(ctx, propertyID, trigger)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return s.failRun(ctx, runID, err.Error(), nil, 0, 0)
	}
	req.Header.Set("User-Agent", "PMS-OccupancySync/1.0")
	res, err := s.HTTP.Do(req)
	if err != nil {
		return s.failRun(ctx, runID, err.Error(), nil, 0, 0)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxICSBodyBytes))
	if err != nil {
		return s.failRun(ctx, runID, err.Error(), &res.StatusCode, 0, 0)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := fmt.Sprintf("http_%d", res.StatusCode)
		return s.failRun(ctx, runID, msg, &res.StatusCode, 0, 0)
	}

	parsedRes, err := ParseICalendarDetailed(body)
	if err != nil {
		return s.failRun(ctx, runID, "parse_ics: "+err.Error(), &res.StatusCode, 0, 0)
	}
	parsed := parsedRes.Events

	seen := make([]string, 0, len(parsed))
	upserted := 0
	sourceType := src.SourceType
	for _, pe := range parsed {
		seen = append(seen, pe.UID)
		existing, err := s.Store.GetOccupancyBySourceEventUID(ctx, propertyID, pe.UID)
		if err != nil {
			_ = s.Store.FinishOccupancySyncRun(ctx, runID, "partial", ptrStr(err.Error()), &res.StatusCode, parsedRes.SeenEvents, upserted)
			return err
		}
		st := "active"
		if pe.Cancelled {
			st = "cancelled"
		} else if existing != nil {
			// Make changes auditable in downstream modules.
			if existing.ContentHash != pe.ContentHash || existing.Status == "cancelled" || existing.Status == "deleted_from_source" {
				st = "updated"
			}
		}
		rs := sql.NullString{String: pe.Summary, Valid: pe.Summary != ""}
		occ := &store.Occupancy{
			PropertyID:     propertyID,
			SourceType:     sourceType,
			SourceEventUID: pe.UID,
			StartAt:        pe.StartUTC,
			EndAt:          pe.EndUTC,
			Status:         st,
			RawSummary:     rs,
			ContentHash:    pe.ContentHash,
		}
		if err := s.Store.InsertOccupancyRawEvent(ctx, propertyID, runID, pe.UID, pe.RawICS, pe.Summary,
			pe.StartUTC.Format(time.RFC3339), pe.EndUTC.Format(time.RFC3339), pe.Sequence, pe.ICalStatus, pe.ContentHash); err != nil {
			_ = s.Store.FinishOccupancySyncRun(ctx, runID, "partial", ptrStr(err.Error()), &res.StatusCode, parsedRes.SeenEvents, upserted)
			return err
		}
		if err := s.Store.UpsertOccupancy(ctx, occ, runID); err != nil {
			_ = s.Store.FinishOccupancySyncRun(ctx, runID, "partial", ptrStr(err.Error()), &res.StatusCode, parsedRes.SeenEvents, upserted)
			return err
		}
		upserted++
	}

	if err := s.Store.MarkOccupanciesDeletedFromSource(ctx, propertyID, sourceType, seen); err != nil {
		_ = s.Store.FinishOccupancySyncRun(ctx, runID, "partial", ptrStr(err.Error()), &res.StatusCode, parsedRes.SeenEvents, upserted)
		return err
	}
	status := "success"
	var msg *string
	if len(parsedRes.ParseErrors) > 0 {
		status = "partial"
		s := summarizeParseErrors(len(parsedRes.Events), parsedRes.SeenEvents, parsedRes.ParseErrors)
		msg = &s
	}
	return s.Store.FinishOccupancySyncRun(ctx, runID, status, msg, &res.StatusCode, parsedRes.SeenEvents, upserted)
}

func (s *Service) failRun(ctx context.Context, runID int64, msg string, httpStatus *int, seen, upserted int) error {
	_ = s.Store.FinishOccupancySyncRun(ctx, runID, "failure", &msg, httpStatus, seen, upserted)
	return fmt.Errorf("%s", msg)
}

func ptrStr(s string) *string { return &s }

func summarizeParseErrors(parsedCount, seenCount int, errs []string) string {
	shown := errs
	if len(shown) > maxParseErrorsShown {
		shown = shown[:maxParseErrorsShown]
	}
	msg := fmt.Sprintf("parsed %d/%d event(s); %d skipped: %s", parsedCount, seenCount, len(errs), strings.Join(shown, " | "))
	if len(errs) > maxParseErrorsShown {
		msg += " | ..."
	}
	if len(msg) > maxSyncErrorLength {
		msg = msg[:maxSyncErrorLength]
	}
	return msg
}
