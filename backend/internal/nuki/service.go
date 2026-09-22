package nuki

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"pms/backend/internal/store"
)

type Service struct {
	Store  *store.Store
	Client Client
}

const syncRunRetention = 50
const cleanerLogLookbackDays = 45

type ReconcileStats struct {
	FetchedEvents     int
	AuthMatchedEvents int
	EntryLikeEvents   int
	UpsertedDays      int
	FallbackAnyEvent  bool
	CleanerAliasCount int
	RequestedSinceUTC string
}

type runStats struct {
	createdN  int
	updatedN  int
	revokedN  int
	failedN   int
	processed int
}

func (s *Service) finishRun(ctx context.Context, propertyID, runID int64, status string, errMsg *string, st runStats) error {
	if err := s.Store.FinishNukiSyncRun(ctx, runID, status, errMsg, st.processed, st.createdN, st.updatedN, st.revokedN, st.failedN); err != nil {
		return err
	}
	_ = s.Store.PruneNukiSyncRuns(ctx, propertyID, syncRunRetention)
	return nil
}

func (s *Service) SyncProperty(ctx context.Context, propertyID int64, trigger string) error {
	if s.Client == nil {
		s.Client = NewClient(Config{})
	}
	runID, err := s.Store.StartNukiSyncRun(ctx, propertyID, trigger)
	if err != nil {
		return err
	}
	stats := runStats{}

	_, _, cred, _, _, _, _, _, err := s.loadNukiSyncContext(ctx, propertyID, false)
	if err != nil {
		msg := truncateErr(err.Error())
		_ = s.finishRun(ctx, propertyID, runID, "failure", &msg, stats)
		return err
	}
	codes, err := s.Client.ListKeypadCodes(ctx, cred)
	if err != nil {
		msg := truncateErr(err.Error())
		_ = s.finishRun(ctx, propertyID, runID, "failure", &msg, stats)
		return err
	}
	stats.processed = len(codes)
	keep := make([]string, 0, len(codes))
	for _, row := range codes {
		keep = append(keep, row.ExternalID)
		record := &store.NukiKeypadCode{
			PropertyID:       propertyID,
			ExternalNukiID:   strings.TrimSpace(row.ExternalID),
			Name:             sql.NullString{String: strings.TrimSpace(row.Name), Valid: strings.TrimSpace(row.Name) != ""},
			AccessCodeMasked: maskCode(row.AccessCodeMasked),
			Enabled:          row.Enabled,
			RawJSON:          sql.NullString{String: row.PayloadJSON, Valid: strings.TrimSpace(row.PayloadJSON) != ""},
			LastSeenAt:       time.Now().UTC(),
		}
		if row.ValidFrom != nil {
			record.ValidFrom = sql.NullTime{Time: row.ValidFrom.UTC(), Valid: true}
		}
		if row.ValidUntil != nil {
			record.ValidUntil = sql.NullTime{Time: row.ValidUntil.UTC(), Valid: true}
		}
		if err := s.Store.UpsertNukiKeypadCode(ctx, record); err != nil {
			stats.failedN++
			continue
		}
		stats.updatedN++
	}
	_ = s.Store.NormalizeNukiKeypadWindows(ctx, propertyID)
	destructiveReconcile := trigger != "after_generate_refresh"
	if destructiveReconcile {
		if err := s.Store.DeleteMissingNukiKeypadCodes(ctx, propertyID, keep); err != nil {
			stats.failedN++
		}
	}
	if keypadRows, err := s.Store.ListNukiKeypadCodes(ctx, propertyID); err == nil {
		stats.updatedN += s.bindAccessCodesToKeypadRows(ctx, propertyID, runID, keypadRows)
	} else {
		stats.failedN++
	}
	if destructiveReconcile {
		if err := s.Store.ReconcileNukiAccessCodesWithKeypad(ctx, propertyID, keep); err != nil {
			stats.failedN++
		}
	}
	status := "success"
	var msg *string
	if stats.failedN > 0 {
		status = "partial"
		m := fmt.Sprintf("%d keypad row(s) failed to store", stats.failedN)
		msg = &m
	}
	return s.finishRun(ctx, propertyID, runID, status, msg, stats)
}

func (s *Service) GenerateCodeForNamedStay(ctx context.Context, propertyID, stayID int64, trigger string, pinName string) error {
	if stayID <= 0 {
		return errors.New("stay_id required")
	}
	if strings.TrimSpace(pinName) == "" {
		return errors.New("pin_name required")
	}
	return s.generateCodesInternal(ctx, propertyID, trigger, &stayID, &pinName)
}

// MaintainNamedStay updates an existing usable credential or revokes it when
// the stay is no longer eligible. It never creates or recreates access.
func (s *Service) MaintainNamedStay(ctx context.Context, propertyID, stayID int64, trigger string) error {
	stay, err := s.Store.GetNamedStay(ctx, propertyID, stayID)
	if err != nil {
		return err
	}
	reviewStatus := "confirmed"
	if stay.ReviewStatus.Valid && strings.TrimSpace(stay.ReviewStatus.String) != "" {
		reviewStatus = strings.TrimSpace(stay.ReviewStatus.String)
	}
	eligible := stay.Status == store.NamedStayStatusActive &&
		store.NamedStayNukiEligible(stay.StayType, reviewStatus) &&
		(!stay.StayOutcome.Valid || (stay.StayOutcome.String != store.StayOutcomeCancelledNonRefundable && stay.StayOutcome.String != store.StayOutcomeNoShow))
	code, err := s.Store.GetNukiCodeByNamedStayID(ctx, propertyID, stayID)
	if err != nil || code == nil || code.Status == "revoked" {
		return err
	}
	if !eligible {
		return s.revokeNamedStayCode(ctx, propertyID, code, trigger)
	}
	if code.Status != "generated" || !code.ExternalNukiID.Valid || strings.TrimSpace(code.ExternalNukiID.String) == "" {
		err := errors.New("nuki_existing_credential_requires_manual_recovery")
		_ = s.Store.MarkNamedStayNukiGeneration(ctx, propertyID, stayID, store.NukiGenerationError, err.Error())
		return err
	}
	if s.Client == nil {
		s.Client = NewClient(Config{})
	}
	_, _, cred, loc, inH, inM, outH, outM, err := s.loadNukiSyncContext(ctx, propertyID, true)
	if err != nil {
		return err
	}
	nukiStay := store.NukiStay{NamedStayID: stay.ID, PropertyID: propertyID, DisplayName: stay.DisplayName, CheckInDate: stay.CheckInDate, CheckOutDate: stay.CheckOutDate}
	from, until := namedStayWindow(nukiStay, loc, inH, inM, outH, outM)
	res, err := s.Client.UpdateAccess(ctx, cred, code.ExternalNukiID.String, UpsertAccessRequest{Label: buildGuestCodeLabelFromName(stay.DisplayName), ValidFrom: from, ValidUntil: until})
	if err != nil {
		code.ErrorMessage = sql.NullString{String: truncateErr(err.Error()), Valid: true}
		_ = s.Store.UpsertNukiCode(ctx, code)
		_ = s.Store.MarkNamedStayNukiGeneration(ctx, propertyID, stayID, store.NukiGenerationError, err.Error())
		return err
	}
	code.CodeLabel = buildGuestCodeLabelFromName(stay.DisplayName)
	code.ValidFrom, code.ValidUntil = from, until
	code.ErrorMessage = sql.NullString{}
	if res != nil && strings.TrimSpace(res.ExternalID) != "" && strings.TrimSpace(res.ExternalID) != code.ExternalNukiID.String {
		err := errors.New("nuki_update_changed_credential_identity")
		_ = s.Store.MarkNamedStayNukiGeneration(ctx, propertyID, stayID, store.NukiGenerationError, err.Error())
		return err
	}
	if err := s.Store.UpsertNukiCode(ctx, code); err != nil {
		_ = s.Store.MarkNamedStayNukiGeneration(ctx, propertyID, stayID, store.NukiGenerationError, err.Error())
		return err
	}
	_ = s.Store.MarkNamedStayNukiGeneration(ctx, propertyID, stayID, store.NukiGenerationGenerated, "")
	return nil
}

func (s *Service) revokeNamedStayCode(ctx context.Context, propertyID int64, code *store.NukiAccessCode, reason string) error {
	if code.ExternalNukiID.Valid && strings.TrimSpace(code.ExternalNukiID.String) != "" {
		if s.Client == nil {
			s.Client = NewClient(Config{})
		}
		sec, err := s.Store.GetPropertySecrets(ctx, propertyID)
		if err != nil {
			return err
		}
		cred := Credentials{APIToken: strings.TrimSpace(sec.NukiAPIToken.String), SmartLockID: strings.TrimSpace(sec.NukiSmartlockID.String)}
		if err := s.Client.RevokeAccess(ctx, cred, code.ExternalNukiID.String); err != nil {
			return err
		}
	}
	code.Status = "revoked"
	code.ErrorMessage = sql.NullString{}
	code.RevokedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
	return s.Store.UpsertNukiCode(ctx, code)
}

func (s *Service) generateCodesInternal(ctx context.Context, propertyID int64, trigger string, onlyStayID *int64, pinName *string) error {
	if onlyStayID == nil || *onlyStayID <= 0 || pinName == nil || strings.TrimSpace(*pinName) == "" {
		return errors.New("single_stay_generation_required")
	}
	if s.Client == nil {
		s.Client = NewClient(Config{})
	}
	runID, err := s.Store.StartNukiSyncRun(ctx, propertyID, trigger)
	if err != nil {
		return err
	}
	stats := runStats{}
	var selectedErr error

	_, _, cred, loc, inH, inM, outH, outM, err := s.loadNukiSyncContext(ctx, propertyID, true)
	if err != nil {
		msg := truncateErr(err.Error())
		_ = s.finishRun(ctx, propertyID, runID, "failure", &msg, stats)
		return err
	}
	accountUserID := s.discoverAccountUserID(ctx, propertyID)
	keypadCodes, _ := s.Store.ListNukiKeypadCodes(ctx, propertyID)

	// Process only the explicitly selected stay. No bulk generation or
	// property-wide revocation is part of this command.
	stays, err := s.Store.ListNamedStaysForNukiSync(ctx, propertyID)
	if err != nil {
		msg := "list_sync_named_stays_failed"
		_ = s.finishRun(ctx, propertyID, runID, "partial", &msg, stats)
		return err
	}
	selected := false
	for _, stay := range stays {
		if stay.NamedStayID != *onlyStayID {
			continue
		}
		selected = true
		from, until := namedStayWindow(stay, loc, inH, inM, outH, outM)
		label := buildGuestCodeLabelFromName(stay.DisplayName)
		if pinName != nil && onlyStayID != nil && stay.NamedStayID == *onlyStayID {
			label = buildGuestCodeLabelFromName(*pinName)
		}
		code, err := s.Store.GetNukiCodeByNamedStayID(ctx, propertyID, stay.NamedStayID)
		if err != nil {
			stats.failedN++
			stats.processed++
			selectedErr = err
			continue
		}
		stats.processed++
		if code == nil {
			if ext, masked := findMatchingKeypadEntry(label, from, until, keypadCodes); ext != "" {
				linked := &store.NukiAccessCode{
					PropertyID:       propertyID,
					NamedStayID:      sql.NullInt64{Int64: stay.NamedStayID, Valid: true},
					CodeLabel:        label,
					AccessCodeMasked: maskCode(masked),
					ExternalNukiID:   sql.NullString{String: ext, Valid: true},
					ValidFrom:        from,
					ValidUntil:       until,
					Status:           "generated",
					LastSyncRunID:    sql.NullInt64{Int64: runID, Valid: true},
				}
				if err := s.Store.UpsertNukiCode(ctx, linked); err == nil {
					_ = s.Store.MarkNamedStayNukiGeneration(ctx, propertyID, stay.NamedStayID, store.NukiGenerationGenerated, "")
					stats.updatedN++
					continue
				}
			}
			// new
			req := UpsertAccessRequest{
				Label:         label,
				ValidFrom:     from,
				ValidUntil:    until,
				AccountUserID: accountUserID,
				AccessCode:    randomNumericCode(6),
			}
			res, err := s.Client.CreateAccess(ctx, cred, req)
			if err != nil {
				stats.failedN++
				selectedErr = err
				_ = s.upsertFailure(ctx, propertyID, stay.NamedStayID, runID, label, from, until, nil, err)
				continue
			}
			pinForMask := strings.TrimSpace(res.AccessCode)
			if pinForMask == "" {
				pinForMask = req.AccessCode
			}
			newCode := &store.NukiAccessCode{
				PropertyID:        propertyID,
				NamedStayID:       sql.NullInt64{Int64: stay.NamedStayID, Valid: true},
				CodeLabel:         label,
				AccessCodeMasked:  maskCode(pinForMask),
				GeneratedPINPlain: sql.NullString{String: pinForMask, Valid: strings.TrimSpace(pinForMask) != ""},
				ExternalNukiID:    sql.NullString{String: strings.TrimSpace(res.ExternalID), Valid: strings.TrimSpace(res.ExternalID) != ""},
				ValidFrom:         from,
				ValidUntil:        until,
				Status:            "generated",
				LastSyncRunID:     sql.NullInt64{Int64: runID, Valid: true},
			}
			if err := s.Store.UpsertNukiCode(ctx, newCode); err != nil {
				stats.failedN++
				selectedErr = err
				continue
			}
			_ = s.Store.MarkNamedStayNukiGeneration(ctx, propertyID, stay.NamedStayID, store.NukiGenerationGenerated, "")
			stats.createdN++
			continue
		}
		code.NamedStayID = sql.NullInt64{Int64: stay.NamedStayID, Valid: true}
		if !code.ExternalNukiID.Valid || strings.TrimSpace(code.ExternalNukiID.String) == "" {
			if ext, masked := findMatchingKeypadEntry(label, from, until, keypadCodes); ext != "" {
				code.ExternalNukiID = sql.NullString{String: ext, Valid: true}
				if !code.AccessCodeMasked.Valid || strings.TrimSpace(code.AccessCodeMasked.String) == "" {
					code.AccessCodeMasked = maskCode(masked)
				}
				code.Status = "generated"
				code.ErrorMessage = sql.NullString{}
				code.LastSyncRunID = sql.NullInt64{Int64: runID, Valid: true}
				if err := s.Store.UpsertNukiCode(ctx, code); err == nil {
					_ = s.Store.MarkNamedStayNukiGeneration(ctx, propertyID, stay.NamedStayID, store.NukiGenerationGenerated, "")
					stats.updatedN++
					continue
				}
			}
		}
		if len(keypadCodes) > 0 && code.ExternalNukiID.Valid && strings.TrimSpace(code.ExternalNukiID.String) != "" {
			if !isExternalLinkUsable(code.ExternalNukiID.String, label, from, until, keypadCodes) {
				// stale/ambiguous link (typically name-only match) must not block fresh PIN creation
				code.ExternalNukiID = sql.NullString{}
			}
		}
		needsUpdate := code.ValidFrom.UTC().Format(time.RFC3339) != from.UTC().Format(time.RFC3339) ||
			code.ValidUntil.UTC().Format(time.RFC3339) != until.UTC().Format(time.RFC3339) ||
			code.Status != "generated" ||
			(!code.ExternalNukiID.Valid || strings.TrimSpace(code.ExternalNukiID.String) == "") ||
			code.CodeLabel != label
		if !needsUpdate {
			code.Status = "generated"
			code.LastSyncRunID = sql.NullInt64{Int64: runID, Valid: true}
			code.ErrorMessage = sql.NullString{}
			_ = s.Store.UpsertNukiCode(ctx, code)
			_ = s.Store.MarkNamedStayNukiGeneration(ctx, propertyID, stay.NamedStayID, store.NukiGenerationGenerated, "")
			continue
		}

		req := UpsertAccessRequest{Label: label, ValidFrom: from, ValidUntil: until, AccountUserID: accountUserID}
		if !code.ExternalNukiID.Valid || strings.TrimSpace(code.ExternalNukiID.String) == "" || code.Status == "revoked" {
			req.AccessCode = randomNumericCode(6)
		}
		var res *UpsertAccessResponse
		updating := code.ExternalNukiID.Valid && strings.TrimSpace(code.ExternalNukiID.String) != "" && code.Status != "revoked"
		if updating {
			res, err = s.Client.UpdateAccess(ctx, cred, code.ExternalNukiID.String, req)
		} else {
			res, err = s.Client.CreateAccess(ctx, cred, req)
		}
		if err != nil {
			stats.failedN++
			selectedErr = err
			var existing *store.NukiAccessCode
			if updating {
				existing = code
			}
			_ = s.upsertFailure(ctx, propertyID, stay.NamedStayID, runID, label, from, until, existing, err)
			continue
		}
		code.CodeLabel = label
		code.ValidFrom = from
		code.ValidUntil = until
		if strings.TrimSpace(res.ExternalID) != "" {
			code.ExternalNukiID = sql.NullString{String: strings.TrimSpace(res.ExternalID), Valid: true}
		}
		if strings.TrimSpace(res.AccessCode) != "" {
			code.AccessCodeMasked = maskCode(res.AccessCode)
			code.GeneratedPINPlain = sql.NullString{String: strings.TrimSpace(res.AccessCode), Valid: true}
		} else if strings.TrimSpace(req.AccessCode) != "" {
			code.AccessCodeMasked = maskCode(req.AccessCode)
			code.GeneratedPINPlain = sql.NullString{String: strings.TrimSpace(req.AccessCode), Valid: true}
		}
		code.Status = "generated"
		code.ErrorMessage = sql.NullString{}
		code.LastSyncRunID = sql.NullInt64{Int64: runID, Valid: true}
		code.RevokedAt = sql.NullTime{}
		if err := s.Store.UpsertNukiCode(ctx, code); err != nil {
			stats.failedN++
			selectedErr = err
			continue
		}
		_ = s.Store.MarkNamedStayNukiGeneration(ctx, propertyID, stay.NamedStayID, store.NukiGenerationGenerated, "")
		stats.updatedN++
	}
	if !selected {
		msg := "named_stay_not_found_or_not_upcoming"
		_ = s.finishRun(ctx, propertyID, runID, "failure", &msg, stats)
		return errors.New(msg)
	}

	status := "success"
	var msg *string
	if stats.failedN > 0 {
		status = "partial"
		m := fmt.Sprintf("%d operation(s) failed", stats.failedN)
		msg = &m
	}
	finishErr := s.finishRun(ctx, propertyID, runID, status, msg, stats)
	if onlyStayID != nil && selectedErr != nil {
		return selectedErr
	}
	return finishErr
}

func (s *Service) discoverAccountUserID(ctx context.Context, propertyID int64) string {
	rows, err := s.Store.ListNukiKeypadCodes(ctx, propertyID)
	if err != nil {
		return ""
	}
	for _, row := range rows {
		if !row.RawJSON.Valid || strings.TrimSpace(row.RawJSON.String) == "" {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(row.RawJSON.String), &m); err != nil {
			continue
		}
		if v, ok := m["accountUserId"]; ok && v != nil {
			switch vv := v.(type) {
			case string:
				if strings.TrimSpace(vv) != "" {
					return strings.TrimSpace(vv)
				}
			case float64:
				return strconv.FormatInt(int64(vv), 10)
			}
		}
	}
	return ""
}

func (s *Service) loadNukiSyncContext(ctx context.Context, propertyID int64, includeProfile bool) (*store.Property, *store.PropertyProfile, Credentials, *time.Location, int, int, int, int, error) {
	prop, err := s.Store.GetProperty(ctx, propertyID)
	if err != nil {
		return nil, nil, Credentials{}, nil, 0, 0, 0, 0, fmt.Errorf("property_not_found")
	}
	if !prop.Active {
		return nil, nil, Credentials{}, nil, 0, 0, 0, 0, fmt.Errorf("property_inactive")
	}
	sec, err := s.Store.GetPropertySecrets(ctx, propertyID)
	if err != nil {
		return nil, nil, Credentials{}, nil, 0, 0, 0, 0, fmt.Errorf("property_secrets_missing")
	}
	if !sec.NukiAPIToken.Valid || strings.TrimSpace(sec.NukiAPIToken.String) == "" || !sec.NukiSmartlockID.Valid || strings.TrimSpace(sec.NukiSmartlockID.String) == "" {
		return nil, nil, Credentials{}, nil, 0, 0, 0, 0, fmt.Errorf("nuki_credentials_not_configured")
	}
	loc, err := time.LoadLocation(prop.Timezone)
	if err != nil {
		loc = time.UTC
	}
	inH, inM := 14, 0
	outH, outM := 10, 0
	var profile *store.PropertyProfile
	if includeProfile {
		profile, err = s.Store.GetPropertyProfile(ctx, propertyID)
		if err != nil {
			return nil, nil, Credentials{}, nil, 0, 0, 0, 0, fmt.Errorf("property_profile_missing")
		}
		inH, inM = parseHM(profile.DefaultCheckInTime, 14, 0)
		outH, outM = parseHM(profile.DefaultCheckOutTime, 10, 0)
	}
	cred := Credentials{
		APIToken:    strings.TrimSpace(sec.NukiAPIToken.String),
		SmartLockID: strings.TrimSpace(sec.NukiSmartlockID.String),
	}
	if _, err := strconv.ParseInt(cred.SmartLockID, 10, 64); err != nil {
		return nil, nil, Credentials{}, nil, 0, 0, 0, 0, fmt.Errorf("nuki_smartlock_id_must_be_numeric")
	}
	return prop, profile, cred, loc, inH, inM, outH, outM, nil
}

func (s *Service) CleanupExpiredCodes(ctx context.Context, propertyID int64) error {
	if s.Client == nil {
		s.Client = NewClient(Config{})
	}
	sec, err := s.Store.GetPropertySecrets(ctx, propertyID)
	if err != nil {
		return err
	}
	if !sec.NukiAPIToken.Valid || !sec.NukiSmartlockID.Valid {
		return nil
	}
	cred := Credentials{
		APIToken:    strings.TrimSpace(sec.NukiAPIToken.String),
		SmartLockID: strings.TrimSpace(sec.NukiSmartlockID.String),
	}
	codes, err := s.Store.ListNukiCodesForCleanup(ctx, propertyID, time.Now().UTC())
	if err != nil {
		return err
	}
	for i := range codes {
		code := &codes[i]
		if code.ExternalNukiID.Valid && strings.TrimSpace(code.ExternalNukiID.String) != "" {
			_ = s.Client.RevokeAccess(ctx, cred, code.ExternalNukiID.String)
		}
		code.Status = "revoked"
		code.AccessCodeMasked = sql.NullString{}
		code.GeneratedPINPlain = sql.NullString{}
		code.ExternalNukiID = sql.NullString{}
		code.ErrorMessage = sql.NullString{}
		now := time.Now().UTC()
		code.RevokedAt = sql.NullTime{Time: now, Valid: true}
		_ = s.Store.UpsertNukiCode(ctx, code)
		cid := code.ID
		_ = s.Store.InsertNukiEventLog(ctx, propertyID, &cid, nil, "cleanup_expired", "expired code moved to historical", "")
	}
	return nil
}

func (s *Service) ReconcileCleanerDailyLogs(ctx context.Context, propertyID int64) (*ReconcileStats, error) {
	since := time.Now().UTC().AddDate(0, 0, -cleanerLogLookbackDays)
	return s.ReconcileCleanerDailyLogsSince(ctx, propertyID, since)
}

func (s *Service) ReconcileCleanerDailyLogsSince(ctx context.Context, propertyID int64, since time.Time) (*ReconcileStats, error) {
	stats := &ReconcileStats{}
	if s.Client == nil {
		s.Client = NewClient(Config{})
	}
	_, profile, cred, loc, _, _, _, _, err := s.loadNukiSyncContext(ctx, propertyID, true)
	if err != nil {
		return stats, err
	}
	if profile == nil || !profile.CleanerNukiAuthID.Valid || strings.TrimSpace(profile.CleanerNukiAuthID.String) == "" {
		return stats, nil
	}
	cleanerAuth := strings.TrimSpace(profile.CleanerNukiAuthID.String)
	reconcileSince := since.UTC()
	if reconcileSince.IsZero() {
		reconcileSince = time.Now().In(loc).AddDate(0, 0, -cleanerLogLookbackDays).UTC()
	}
	stats.RequestedSinceUTC = reconcileSince.Format(time.RFC3339)
	aliases := s.cleanerAuthAliases(ctx, propertyID, cleanerAuth)
	stats.CleanerAliasCount = len(aliases)
	events, err := s.Client.ListSmartlockEvents(ctx, cred, reconcileSince, cleanerAuth)
	if err != nil {
		return stats, err
	}
	stats.FetchedEvents = len(events)
	firstByDay := map[string]SmartlockEvent{}
	anyByDay := map[string]SmartlockEvent{}
	for _, ev := range events {
		if !matchesAnyCleanerAuthID(ev.AuthID, ev.PayloadJSON, aliases) {
			continue
		}
		stats.AuthMatchedEvents++
		day := ev.OccurredAt.In(loc).Format("2006-01-02")
		anyExisting, anyOK := anyByDay[day]
		if !anyOK || ev.OccurredAt.Before(anyExisting.OccurredAt) {
			anyByDay[day] = ev
		}
		if !ev.IsEntryLike {
			continue
		}
		stats.EntryLikeEvents++
		existing, ok := firstByDay[day]
		if !ok || ev.OccurredAt.Before(existing.OccurredAt) {
			firstByDay[day] = ev
		}
	}
	// Fallback for integrations where Nuki log type labels differ:
	// if no entry-like events were recognized for the selected period,
	// use first auth-matched event per day.
	if len(firstByDay) == 0 && len(anyByDay) > 0 {
		firstByDay = anyByDay
		stats.FallbackAnyEvent = true
	}
	for day, ev := range firstByDay {
		if err := s.Store.UpsertCleaningDailyLog(ctx, &store.CleaningDailyLog{
			PropertyID:         propertyID,
			DayDate:            day,
			FirstEntryAt:       sql.NullTime{Time: ev.OccurredAt.UTC(), Valid: true},
			NukiEventReference: sql.NullString{String: strings.TrimSpace(ev.ExternalID), Valid: strings.TrimSpace(ev.ExternalID) != ""},
			CountedForSalary:   true,
		}); err != nil {
			return stats, err
		}
		stats.UpsertedDays++
	}
	return stats, nil
}

// GuestReconcileStats summarises the work done by ReconcileGuestDailyEntries.
// AuthMatchedEvents counts unlocks that resolved to a known guest stay
// (after cleaner-alias filtering); UpsertedDays counts the unique
// (named stay, day) pairs persisted in the latest reconcile pass.
type GuestReconcileStats struct {
	FetchedEvents     int
	CleanerSkipped    int
	AuthMatchedEvents int
	EntryLikeEvents   int
	UpsertedDays      int
	FallbackAnyEvent  bool
	NamedStayKeyCount int
	CleanerAliasCount int
	RequestedSinceUTC string
}

// ReconcileGuestDailyEntries reconciles guest unlock events for the
// configured lookback window. Results power the Analytics → Performance
// guest check-in heatmap (PMS_12 task 3).
func (s *Service) ReconcileGuestDailyEntries(ctx context.Context, propertyID int64) (*GuestReconcileStats, error) {
	since := time.Now().UTC().AddDate(0, 0, -cleanerLogLookbackDays)
	return s.ReconcileGuestDailyEntriesSince(ctx, propertyID, since)
}

// ReconcileGuestDailyEntriesSince fetches Smartlock events since the given
// instant, partitions guest unlocks from cleaner unlocks, and persists the
// earliest entry per (named stay, day) into nuki_guest_daily_entries.
//
// The implementation deliberately mirrors ReconcileCleanerDailyLogsSince:
// same lookback default, same TZ-aware bucketing, same fallback to "any
// matched event" when the upstream Nuki log doesn't classify entry-like
// events. Cleaner aliases are loaded with cleanerAuthAliases and applied
// as an exclusion list so a stay isn't credited with the cleaner's unlock.
func (s *Service) ReconcileGuestDailyEntriesSince(ctx context.Context, propertyID int64, since time.Time) (*GuestReconcileStats, error) {
	stats := &GuestReconcileStats{}
	if s.Client == nil {
		s.Client = NewClient(Config{})
	}
	_, profile, cred, loc, _, _, _, _, err := s.loadNukiSyncContext(ctx, propertyID, true)
	if err != nil {
		return stats, err
	}
	stayByAuth, err := s.Store.ListGeneratedNukiAccessCodesByExternalID(ctx, propertyID)
	if err != nil {
		return stats, err
	}
	stats.NamedStayKeyCount = len(stayByAuth)
	if len(stayByAuth) == 0 {
		return stats, nil
	}
	cleanerAuth := ""
	if profile != nil && profile.CleanerNukiAuthID.Valid {
		cleanerAuth = strings.TrimSpace(profile.CleanerNukiAuthID.String)
	}
	aliases := s.cleanerAuthAliases(ctx, propertyID, cleanerAuth)
	stats.CleanerAliasCount = len(aliases)

	reconcileSince := since.UTC()
	if reconcileSince.IsZero() {
		reconcileSince = time.Now().In(loc).AddDate(0, 0, -cleanerLogLookbackDays).UTC()
	}
	stats.RequestedSinceUTC = reconcileSince.Format(time.RFC3339)
	events, err := s.Client.ListSmartlockEvents(ctx, cred, reconcileSince, "")
	if err != nil {
		return stats, err
	}
	stats.FetchedEvents = len(events)

	type bucketKey struct {
		namedStayID int64
		day         string
	}
	firstByKey := map[bucketKey]SmartlockEvent{}
	anyByKey := map[bucketKey]SmartlockEvent{}
	for _, ev := range events {
		if cleanerAuth != "" && matchesAnyCleanerAuthID(ev.AuthID, ev.PayloadJSON, aliases) {
			stats.CleanerSkipped++
			continue
		}
		authID := strings.TrimSpace(ev.AuthID)
		if authID == "" {
			continue
		}
		ident, ok := stayByAuth[authID]
		if !ok {
			continue
		}
		if ident.NamedStayID <= 0 {
			continue
		}
		stats.AuthMatchedEvents++
		key := bucketKey{namedStayID: ident.NamedStayID, day: ev.OccurredAt.In(loc).Format("2006-01-02")}
		if anyExisting, anyOK := anyByKey[key]; !anyOK || ev.OccurredAt.Before(anyExisting.OccurredAt) {
			anyByKey[key] = ev
		}
		if !ev.IsEntryLike {
			continue
		}
		stats.EntryLikeEvents++
		if existing, ok := firstByKey[key]; !ok || ev.OccurredAt.Before(existing.OccurredAt) {
			firstByKey[key] = ev
		}
	}
	if len(firstByKey) == 0 && len(anyByKey) > 0 {
		firstByKey = anyByKey
		stats.FallbackAnyEvent = true
	}
	for key, ev := range firstByKey {
		ref := strings.TrimSpace(ev.ExternalID)
		if err := s.Store.UpsertNukiGuestDailyEntry(ctx, &store.NukiGuestDailyEntry{
			PropertyID:         propertyID,
			NamedStayID:        sql.NullInt64{Int64: key.namedStayID, Valid: true},
			DayDate:            key.day,
			FirstEntryAt:       ev.OccurredAt.UTC(),
			NukiEventReference: sql.NullString{String: ref, Valid: ref != ""},
		}); err != nil {
			return stats, err
		}
		stats.UpsertedDays++
	}
	return stats, nil
}

func (s *Service) cleanerAuthAliases(ctx context.Context, propertyID int64, configured string) map[string]struct{} {
	out := map[string]struct{}{}
	add := func(v string) {
		v = strings.TrimSpace(strings.ToLower(v))
		if v != "" {
			out[v] = struct{}{}
		}
	}
	add(configured)
	rows, err := s.Store.ListNukiKeypadCodes(ctx, propertyID)
	if err != nil {
		return out
	}
	for _, row := range rows {
		if strings.EqualFold(strings.TrimSpace(row.ExternalNukiID), strings.TrimSpace(configured)) {
			add(row.ExternalNukiID)
			if row.Name.Valid {
				add(row.Name.String)
			}
			if !row.RawJSON.Valid || strings.TrimSpace(row.RawJSON.String) == "" {
				continue
			}
			var m map[string]interface{}
			if err := json.Unmarshal([]byte(row.RawJSON.String), &m); err != nil {
				continue
			}
			add(anyToIDString(m["accountUserId"]))
			add(anyToIDString(m["authId"]))
			add(anyToIDString(m["id"]))
			if authObj, ok := m["auth"].(map[string]interface{}); ok {
				add(anyToIDString(authObj["id"]))
				add(anyToIDString(authObj["authId"]))
				add(anyToIDString(authObj["accountUserId"]))
				add(anyToIDString(authObj["userId"]))
				add(anyToIDString(authObj["name"]))
			}
			if accObj, ok := m["accountUser"].(map[string]interface{}); ok {
				add(anyToIDString(accObj["id"]))
				add(anyToIDString(accObj["accountUserId"]))
				add(anyToIDString(accObj["name"]))
			}
		}
	}
	return out
}

func anyToIDString(v interface{}) string {
	switch vv := v.(type) {
	case string:
		return strings.TrimSpace(vv)
	case float64:
		return strconv.FormatInt(int64(vv), 10)
	case int64:
		return strconv.FormatInt(vv, 10)
	case int:
		return strconv.Itoa(vv)
	default:
		return ""
	}
}

func (s *Service) RevokeCode(ctx context.Context, propertyID, codeID int64, reason string) error {
	if s.Client == nil {
		s.Client = NewClient(Config{})
	}
	code, err := s.Store.MustGetNukiCodeByID(ctx, propertyID, codeID)
	if err != nil {
		return err
	}
	sec, err := s.Store.GetPropertySecrets(ctx, propertyID)
	if err != nil {
		return err
	}
	cred := Credentials{
		APIToken:    strings.TrimSpace(sec.NukiAPIToken.String),
		SmartLockID: strings.TrimSpace(sec.NukiSmartlockID.String),
	}
	return s.revokeCode(ctx, cred, 0, code, reason)
}

func (s *Service) DeleteKeypadCode(ctx context.Context, propertyID int64, externalID, reason string) error {
	if s.Client == nil {
		s.Client = NewClient(Config{})
	}
	_, _, cred, _, _, _, _, _, err := s.loadNukiSyncContext(ctx, propertyID, false)
	if err != nil {
		return err
	}
	if strings.TrimSpace(externalID) == "" {
		return fmt.Errorf("external_id_required")
	}
	if err := s.Client.RevokeAccess(ctx, cred, externalID); err != nil {
		return err
	}
	if err := s.Store.DeleteNukiKeypadCodeByExternalID(ctx, propertyID, externalID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	// Keep generated PIN state in sync with manual deletions.
	if err := s.Store.MarkNukiAccessCodesDeletedByExternalID(ctx, propertyID, externalID); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{"external_id": externalID, "reason": reason})
	_ = s.Store.InsertNukiEventLog(ctx, propertyID, nil, nil, "keypad_deleted", "keypad entry deleted", string(payload))
	return nil
}

func (s *Service) SetKeypadCodeEnabled(ctx context.Context, propertyID int64, externalID string, enabled bool, reason string) error {
	if s.Client == nil {
		s.Client = NewClient(Config{})
	}
	_, _, cred, _, _, _, _, _, err := s.loadNukiSyncContext(ctx, propertyID, false)
	if err != nil {
		return err
	}
	if strings.TrimSpace(externalID) == "" {
		return fmt.Errorf("external_id_required")
	}
	payload := map[string]interface{}{
		"enabled": enabled,
	}
	localRow, rowErr := s.Store.GetNukiKeypadCodeByExternalID(ctx, propertyID, externalID)
	if rowErr == nil && localRow != nil && localRow.RawJSON.Valid && strings.TrimSpace(localRow.RawJSON.String) != "" {
		if p := buildTogglePayloadFromRaw(localRow.RawJSON.String, enabled); len(p) > 0 {
			payload = p
		}
	}
	if err := s.Client.SetAccessEnabled(ctx, cred, externalID, payload); err != nil {
		return err
	}
	if err := s.Store.UpdateNukiKeypadCodeEnabled(ctx, propertyID, externalID, enabled); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		// Local cache can be stale/missing; keep remote success and create a minimal local row.
		if err := s.Store.UpsertNukiKeypadCode(ctx, &store.NukiKeypadCode{
			PropertyID:       propertyID,
			ExternalNukiID:   externalID,
			Enabled:          enabled,
			AccessCodeMasked: sql.NullString{},
			Name:             sql.NullString{},
			LastSeenAt:       time.Now().UTC(),
		}); err != nil {
			return err
		}
	}
	logPayload, _ := json.Marshal(map[string]interface{}{"external_id": externalID, "enabled": enabled, "reason": reason})
	_ = s.Store.InsertNukiEventLog(ctx, propertyID, nil, nil, "keypad_enabled_state_changed", "keypad enabled state changed", string(logPayload))
	return nil
}

func buildTogglePayloadFromRaw(raw string, enabled bool) map[string]interface{} {
	var src map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &src); err != nil {
		return nil
	}
	out := map[string]interface{}{"enabled": enabled}
	copyIfExists := func(key string) {
		if v, ok := src[key]; ok {
			if v == nil {
				return
			}
			out[key] = v
		}
	}
	copyIfExists("name")
	copyIfExists("allowedFromDate")
	copyIfExists("allowedUntilDate")
	copyIfExists("remoteAllowed")
	copyIfExists("smartActionsEnabled")
	copyIfExists("type")
	copyIfExists("accountUserId")
	return out
}

func (s *Service) revokeCode(ctx context.Context, cred Credentials, runID int64, code *store.NukiAccessCode, reason string) error {
	if code.ExternalNukiID.Valid && strings.TrimSpace(code.ExternalNukiID.String) != "" {
		if err := s.Client.RevokeAccess(ctx, cred, code.ExternalNukiID.String); err != nil {
			code.Status = "generated"
			code.ErrorMessage = sql.NullString{String: truncateErr(err.Error()), Valid: true}
			if runID > 0 {
				code.LastSyncRunID = sql.NullInt64{Int64: runID, Valid: true}
			}
			_ = s.Store.UpsertNukiCode(ctx, code)
			cid := code.ID
			_ = s.Store.InsertNukiEventLog(ctx, code.PropertyID, &cid, nil, "revoke_failed", code.ErrorMessage.String, "")
			return err
		}
	}
	code.Status = "revoked"
	code.ErrorMessage = sql.NullString{}
	now := time.Now().UTC()
	code.RevokedAt = sql.NullTime{Time: now, Valid: true}
	if runID > 0 {
		code.LastSyncRunID = sql.NullInt64{Int64: runID, Valid: true}
	}
	if err := s.Store.UpsertNukiCode(ctx, code); err != nil {
		return err
	}
	cid := code.ID
	var rid *int64
	if runID > 0 {
		rid = &runID
	}
	payload, _ := json.Marshal(map[string]string{"reason": reason})
	_ = s.Store.InsertNukiEventLog(ctx, code.PropertyID, &cid, rid, "revoked", "code revoked", string(payload))
	return nil
}

func (s *Service) upsertFailure(ctx context.Context, propertyID, stayID, runID int64, label string, from, until time.Time, existing *store.NukiAccessCode, err error) error {
	m := truncateErr(err.Error())
	c := existing
	if c == nil {
		c = &store.NukiAccessCode{}
	}
	c.PropertyID = propertyID
	c.NamedStayID = sql.NullInt64{Int64: stayID, Valid: stayID > 0}
	c.CodeLabel = label
	c.ValidFrom = from
	c.ValidUntil = until
	c.Status = "not_generated"
	c.ErrorMessage = sql.NullString{String: m, Valid: true}
	c.LastSyncRunID = sql.NullInt64{Int64: runID, Valid: true}
	if stayID > 0 {
		_ = s.Store.MarkNamedStayNukiGeneration(ctx, propertyID, stayID, store.NukiGenerationError, m)
	}
	return s.Store.UpsertNukiCode(ctx, c)
}

func (s *Service) bindAccessCodesToKeypadRows(ctx context.Context, propertyID, runID int64, rows []store.NukiKeypadCode) int {
	codes, err := s.Store.ListNukiCodes(ctx, propertyID, "all")
	if err != nil {
		return 0
	}
	updated := 0
	for _, row := range codes {
		code := row.Code
		if code.Status == "revoked" {
			continue
		}
		if code.ExternalNukiID.Valid && strings.TrimSpace(code.ExternalNukiID.String) != "" &&
			isExternalLinkUsable(code.ExternalNukiID.String, code.CodeLabel, code.ValidFrom, code.ValidUntil, rows) {
			continue
		}
		ext, masked := findMatchingKeypadEntry(code.CodeLabel, code.ValidFrom, code.ValidUntil, rows)
		if ext == "" {
			continue
		}
		code.ExternalNukiID = sql.NullString{String: ext, Valid: true}
		if !code.AccessCodeMasked.Valid || strings.TrimSpace(code.AccessCodeMasked.String) == "" {
			code.AccessCodeMasked = maskCode(masked)
		}
		code.Status = "generated"
		code.ErrorMessage = sql.NullString{}
		code.LastSyncRunID = sql.NullInt64{Int64: runID, Valid: true}
		code.RevokedAt = sql.NullTime{}
		if err := s.Store.UpsertNukiCode(ctx, &code); err == nil {
			updated++
		}
	}
	return updated
}

func namedStayWindow(stay store.NukiStay, loc *time.Location, inH, inM, outH, outM int) (time.Time, time.Time) {
	ci, err := time.ParseInLocation("2006-01-02", stay.CheckInDate, loc)
	if err != nil {
		ci = time.Now().In(loc)
	}
	co, err := time.ParseInLocation("2006-01-02", stay.CheckOutDate, loc)
	if err != nil {
		co = ci.AddDate(0, 0, 1)
	}
	validFrom := time.Date(ci.Year(), ci.Month(), ci.Day(), inH, inM, 0, 0, loc).UTC()
	validUntil := time.Date(co.Year(), co.Month(), co.Day(), outH, outM, 0, 0, loc).UTC()
	if !validUntil.After(validFrom) {
		validUntil = validFrom.Add(2 * time.Hour)
	}
	return validFrom, validUntil
}

func parseHM(v string, defH, defM int) (int, int) {
	parts := strings.Split(strings.TrimSpace(v), ":")
	if len(parts) != 2 {
		return defH, defM
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return defH, defM
	}
	return h, m
}

func buildGuestCodeLabelFromName(name string) string {
	return canonicalBookingLabel(name)
}

func canonicalBookingLabel(raw string) string {
	name := strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(name), "booking-") {
		name = strings.TrimSpace(name[8:])
	}
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		name = "Guest"
	}
	return "Booking-" + name
}

func labelKey(v string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(v)), " "))
}

func maskCode(raw string) sql.NullString {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return sql.NullString{}
	}
	r := []rune(raw)
	if len(r) <= 2 {
		return sql.NullString{String: "***", Valid: true}
	}
	return sql.NullString{String: strings.Repeat("*", len(r)-2) + string(r[len(r)-2:]), Valid: true}
}

func truncateErr(s string) string {
	if len(s) > 900 {
		return s[:900]
	}
	return s
}

func randomNumericCode(n int) string {
	if n <= 0 {
		n = 6
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "123456"
	}
	for i := range buf {
		// Nuki guest keypad PIN policy for this app: digits 1-9 only, no zero.
		buf[i] = '1' + (buf[i] % 9)
	}
	return string(buf)
}

func findMatchingKeypadEntry(label string, from, until time.Time, rows []store.NukiKeypadCode) (externalID, masked string) {
	l := labelKey(label)
	nameOnlyMatches := 0
	var nameOnly store.NukiKeypadCode
	for _, row := range rows {
		if strings.TrimSpace(row.ExternalNukiID) == "" {
			continue
		}
		if !row.Name.Valid || labelKey(row.Name.String) != l {
			continue
		}
		if !row.ValidFrom.Valid || !row.ValidUntil.Valid {
			if strings.HasPrefix(l, "booking-") {
				nameOnlyMatches++
				nameOnly = row
			}
			continue
		}
		if !windowsLikelySame(from, until, row.ValidFrom.Time.UTC(), row.ValidUntil.Time.UTC()) {
			continue
		}
		m := ""
		if row.AccessCodeMasked.Valid {
			m = row.AccessCodeMasked.String
		}
		return row.ExternalNukiID, m
	}
	if strings.HasPrefix(l, "booking-") && nameOnlyMatches == 1 {
		m := ""
		if nameOnly.AccessCodeMasked.Valid {
			m = nameOnly.AccessCodeMasked.String
		}
		return nameOnly.ExternalNukiID, m
	}
	return "", ""
}

func isExternalLinkUsable(externalID, label string, from, until time.Time, rows []store.NukiKeypadCode) bool {
	if strings.TrimSpace(externalID) == "" {
		return false
	}
	l := labelKey(label)
	for _, row := range rows {
		if strings.TrimSpace(row.ExternalNukiID) != strings.TrimSpace(externalID) {
			continue
		}
		if !row.Name.Valid || labelKey(row.Name.String) != l {
			return false
		}
		if !row.ValidFrom.Valid || !row.ValidUntil.Valid {
			return strings.HasPrefix(l, "booking-")
		}
		return windowsLikelySame(from, until, row.ValidFrom.Time.UTC(), row.ValidUntil.Time.UTC())
	}
	return false
}

func windowsLikelySame(aFrom, aUntil, bFrom, bUntil time.Time) bool {
	aFrom = aFrom.UTC()
	aUntil = aUntil.UTC()
	bFrom = bFrom.UTC()
	bUntil = bUntil.UTC()
	if aFrom.Before(bUntil) && aUntil.After(bFrom) {
		return true
	}
	return sameUTCDate(aFrom, bFrom) && sameUTCDate(aUntil, bUntil)
}

func sameUTCDate(a, b time.Time) bool {
	a = a.UTC()
	b = b.UTC()
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func matchesCleanerAuthID(eventAuthID, cleanerAuthID string) bool {
	a := strings.TrimSpace(strings.ToLower(eventAuthID))
	b := strings.TrimSpace(strings.ToLower(cleanerAuthID))
	if a == "" || b == "" {
		return false
	}
	return a == b
}

func matchesAnyCleanerAuthID(eventAuthID, payloadJSON string, aliases map[string]struct{}) bool {
	key := strings.TrimSpace(strings.ToLower(eventAuthID))
	if len(aliases) == 0 {
		return false
	}
	if key != "" {
		if _, ok := aliases[key]; ok {
			return true
		}
	}
	raw := strings.ToLower(strings.TrimSpace(payloadJSON))
	if raw == "" {
		return false
	}
	for alias := range aliases {
		if alias == "" {
			continue
		}
		if strings.Contains(raw, alias) {
			return true
		}
	}
	return false
}
