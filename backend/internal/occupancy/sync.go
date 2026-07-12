package occupancy

import (
	"context"
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
	propertySyncLeaseTTL   = 5 * time.Minute

	// StatusPartialNoMutation makes it explicit that a partial parse applied
	// zero occupancy mutations (PMS_19 §7.2).
	StatusPartialNoMutation = "partial_no_mutation"
)

// Service runs ICS fetch and occupancy normalization.
type Service struct {
	Store *store.Store
	HTTP  *http.Client
	Now   func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// SyncProperty fetches the property ICS URL and reconciles occupancies. A
// successful full sync is authoritative for current/future Booking.com events;
// failed or partial syncs never mutate occupancies (PMS_19 §7).
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
	leaseName := fmt.Sprintf("occupancy_sync_property_%d", propertyID)
	leaseOwner := fmt.Sprintf("run-%d", runID)
	leaseOK, err := s.Store.TryAcquireJobLease(ctx, leaseName, leaseOwner, propertySyncLeaseTTL)
	if err != nil {
		return s.failRun(ctx, runID, "sync_lease: "+err.Error(), nil)
	}
	if !leaseOK {
		msg := "sync_already_running"
		_ = s.Store.FinishOccupancySyncRun(ctx, runID, "skipped", &msg, nil, 0, 0)
		return fmt.Errorf("%s", msg)
	}
	defer func() { _ = s.Store.ReleaseJobLease(context.Background(), leaseName, leaseOwner) }()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return s.failRun(ctx, runID, err.Error(), nil)
	}
	req.Header.Set("User-Agent", "PMS-OccupancySync/1.0")
	res, err := s.HTTP.Do(req)
	if err != nil {
		return s.failRun(ctx, runID, err.Error(), nil)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxICSBodyBytes))
	if err != nil {
		return s.failRun(ctx, runID, err.Error(), &res.StatusCode)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := fmt.Sprintf("http_%d", res.StatusCode)
		return s.failRun(ctx, runID, msg, &res.StatusCode)
	}

	parsedRes, err := ParseICalendarDetailed(body)
	if err != nil {
		return s.failRun(ctx, runID, "parse_ics: "+err.Error(), &res.StatusCode)
	}

	counters := &store.SyncCounters{
		UpstreamEventsSeen:   parsedRes.SeenEvents,
		UpstreamEventsParsed: len(parsedRes.Events),
		ParseErrors:          len(parsedRes.ParseErrors),
		DeletionEnabled:      true,
	}

	// PMS_19 §7.2: any event-level parse failure aborts mutation entirely so a
	// skipped event can never be mistaken for a disappeared one.
	if len(parsedRes.ParseErrors) > 0 {
		counters.DeletionEnabled = false
		msg := summarizeParseErrors(len(parsedRes.Events), parsedRes.SeenEvents, parsedRes.ParseErrors)
		_ = s.Store.FinishOccupancySyncRunDetailed(ctx, runID, StatusPartialNoMutation, &msg, &res.StatusCode, counters)
		return nil
	}

	// Store raw upstream snapshots before expansion (§7.1 step 6).
	for _, pe := range parsedRes.Events {
		dtstamp := ""
		if !pe.DTStampUTC.IsZero() {
			dtstamp = pe.DTStampUTC.Format(time.RFC3339)
		}
		if err := s.Store.InsertOccupancyRawEventDetailed(ctx, propertyID, runID, src.SourceType, pe.UID, pe.RawICS, pe.Summary,
			pe.StartUTC.Format(time.RFC3339), pe.EndUTC.Format(time.RFC3339), pe.Sequence, pe.ICalStatus, dtstamp, pe.ContentHash); err != nil {
			return s.failRunDetailed(ctx, runID, err.Error(), &res.StatusCode, counters)
		}
	}

	blocks := make([]store.DesiredBlock, 0, len(parsedRes.Events))
	for _, pe := range parsedRes.Events {
		blocks = append(blocks, store.DesiredBlock{
			UID:         pe.UID,
			Start:       pe.StartUTC,
			End:         pe.EndUTC,
			Summary:     pe.Summary,
			ContentHash: pe.ContentHash,
			Cancelled:   pe.Cancelled,
			DTStamp:     pe.DTStampUTC,
		})
	}

	if err := s.Store.ReconcileBookingICSSync(ctx, propertyID, src.SourceType, blocks, s.now(), counters); err != nil {
		return s.failRunDetailed(ctx, runID, err.Error(), &res.StatusCode, counters)
	}

	// PMS_19 §11B: sync-driven deletions are attributable to the sync run.
	if counters.RepresentationsDeletedFromSource > 0 {
		_ = s.Store.InsertAuditLog(ctx, nil, "occupancy_ics_row_deleted_from_source", "occupancy_sync_run",
			fmt.Sprintf("%d", runID), fmt.Sprintf("deleted=%d named=%d", counters.RepresentationsDeletedFromSource, counters.NamedStaysDeletedFromSource), "sync", url)
	}

	return s.Store.FinishOccupancySyncRunDetailed(ctx, runID, "success", nil, &res.StatusCode, counters)
}

func (s *Service) failRun(ctx context.Context, runID int64, msg string, httpStatus *int) error {
	_ = s.Store.FinishOccupancySyncRun(ctx, runID, "failure", &msg, httpStatus, 0, 0)
	return fmt.Errorf("%s", msg)
}

func (s *Service) failRunDetailed(ctx context.Context, runID int64, msg string, httpStatus *int, c *store.SyncCounters) error {
	_ = s.Store.FinishOccupancySyncRunDetailed(ctx, runID, "failure", &msg, httpStatus, c)
	return fmt.Errorf("%s", msg)
}

func summarizeParseErrors(parsedCount, seenCount int, errs []string) string {
	shown := errs
	if len(shown) > maxParseErrorsShown {
		shown = shown[:maxParseErrorsShown]
	}
	msg := fmt.Sprintf("parsed %d/%d event(s); %d skipped (no mutation applied): %s", parsedCount, seenCount, len(errs), strings.Join(shown, " | "))
	if len(errs) > maxParseErrorsShown {
		msg += " | ..."
	}
	if len(msg) > maxSyncErrorLength {
		msg = msg[:maxSyncErrorLength]
	}
	return msg
}
