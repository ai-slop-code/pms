package nuki

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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

func (s *Service) GenerateCodes(ctx context.Context, propertyID int64, trigger string) error {
	return s.generateCodesInternal(ctx, propertyID, trigger, nil, nil)
}

func (s *Service) GenerateCodeForOccupancy(ctx context.Context, propertyID, occupancyID int64, trigger string, pinName string) error {
	return s.generateCodesInternal(ctx, propertyID, trigger, &occupancyID, &pinName)
}

func (s *Service) generateCodesInternal(ctx context.Context, propertyID int64, trigger string, onlyOccupancyID *int64, pinName *string) error {
	if s.Client == nil {
		s.Client = NewClient(Config{})
	}
	runID, err := s.Store.StartNukiSyncRun(ctx, propertyID, trigger)
	if err != nil {
		return err
	}
	stats := runStats{}

	_, _, cred, loc, inH, inM, outH, outM, err := s.loadNukiSyncContext(ctx, propertyID, true)
	if err != nil {
		msg := truncateErr(err.Error())
		_ = s.finishRun(ctx, propertyID, runID, "failure", &msg, stats)
		return err
	}
	accountUserID := s.discoverAccountUserID(ctx, propertyID)
	keypadCodes, _ := s.Store.ListNukiKeypadCodes(ctx, propertyID)

	// 1) Revoke codes for cancelled/deleted occupancies.
	revokeOccs, err := s.Store.ListOccupanciesForNukiRevocation(ctx, propertyID)
	if err != nil {
		msg := "list_revoke_occupancies_failed"
		_ = s.finishRun(ctx, propertyID, runID, "partial", &msg, stats)
		return err
	}
	for _, o := range revokeOccs {
		code, err := s.Store.GetNukiCodeByOccupancyID(ctx, propertyID, o.ID)
		if err != nil || code == nil {
			continue
		}
		if code.Status == "revoked" {
			continue
		}
		stats.processed++
		if err := s.revokeCode(ctx, cred, runID, code, "occupancy_"+o.Status); err != nil {
			stats.failedN++
			continue
		}
		stats.revokedN++
	}

	// 2) Create/update codes for active/updated occupancies.
	occs, err := s.Store.ListOccupanciesForNukiSync(ctx, propertyID)
	if err != nil {
		msg := "list_sync_occupancies_failed"
		_ = s.finishRun(ctx, propertyID, runID, "partial", &msg, stats)
		return err
	}
	for _, o := range occs {
		if onlyOccupancyID != nil && o.ID != *onlyOccupancyID {
			continue
		}
		from, until := occupancyWindow(o, loc, inH, inM, outH, outM)
		label := buildGuestCodeLabel(o)
		if pinName != nil && onlyOccupancyID != nil && o.ID == *onlyOccupancyID {
			label = buildGuestCodeLabelFromName(*pinName)
		}
		code, err := s.Store.GetNukiCodeByOccupancyID(ctx, propertyID, o.ID)
		if err != nil {
			stats.failedN++
			stats.processed++
			continue
		}
		stats.processed++
		if code == nil {
			if ext, masked := findMatchingKeypadEntry(label, from, until, keypadCodes); ext != "" {
				linked := &store.NukiAccessCode{
					PropertyID:       propertyID,
					OccupancyID:      o.ID,
					CodeLabel:        label,
					AccessCodeMasked: maskCode(masked),
					ExternalNukiID:   sql.NullString{String: ext, Valid: true},
					ValidFrom:        from,
					ValidUntil:       until,
					Status:           "generated",
					LastSyncRunID:    sql.NullInt64{Int64: runID, Valid: true},
				}
				if err := s.Store.UpsertNukiCode(ctx, linked); err == nil {
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
				_ = s.upsertFailure(ctx, propertyID, o.ID, runID, label, from, until, err)
				continue
			}
			pinForMask := strings.TrimSpace(res.AccessCode)
			if pinForMask == "" {
				pinForMask = req.AccessCode
			}
			newCode := &store.NukiAccessCode{
				PropertyID:        propertyID,
				OccupancyID:       o.ID,
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
				continue
			}
			stats.createdN++
			continue
		}
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
			continue
		}

		req := UpsertAccessRequest{Label: label, ValidFrom: from, ValidUntil: until, AccountUserID: accountUserID}
		if !code.ExternalNukiID.Valid || strings.TrimSpace(code.ExternalNukiID.String) == "" || code.Status == "not_generated" || code.Status == "revoked" {
			req.AccessCode = randomNumericCode(6)
		}
		var res *UpsertAccessResponse
		if code.ExternalNukiID.Valid && strings.TrimSpace(code.ExternalNukiID.String) != "" && code.Status != "revoked" {
			res, err = s.Client.UpdateAccess(ctx, cred, code.ExternalNukiID.String, req)
		} else {
			res, err = s.Client.CreateAccess(ctx, cred, req)
		}
		if err != nil {
			stats.failedN++
			_ = s.upsertFailure(ctx, propertyID, o.ID, runID, label, from, until, err)
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
			continue
		}
		stats.updatedN++
	}
	if onlyOccupancyID != nil && stats.processed == 0 {
		msg := "occupancy_not_found_or_not_upcoming"
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
	return s.finishRun(ctx, propertyID, runID, status, msg, stats)
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
	// Keep occupancy-linked generated PIN state in sync with manual deletions.
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

func (s *Service) upsertFailure(ctx context.Context, propertyID, occupancyID, runID int64, label string, from, until time.Time, err error) error {
	m := truncateErr(err.Error())
	c := &store.NukiAccessCode{
		PropertyID:    propertyID,
		OccupancyID:   occupancyID,
		CodeLabel:     label,
		ValidFrom:     from,
		ValidUntil:    until,
		Status:        "not_generated",
		ErrorMessage:  sql.NullString{String: m, Valid: true},
		LastSyncRunID: sql.NullInt64{Int64: runID, Valid: true},
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

func occupancyWindow(o store.Occupancy, loc *time.Location, inH, inM, outH, outM int) (time.Time, time.Time) {
	startLocal := o.StartAt.In(loc)
	endLocal := o.EndAt.In(loc)
	validFrom := time.Date(startLocal.Year(), startLocal.Month(), startLocal.Day(), inH, inM, 0, 0, loc).UTC()
	validUntil := time.Date(endLocal.Year(), endLocal.Month(), endLocal.Day(), outH, outM, 0, 0, loc).UTC()
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

func buildGuestCodeLabel(o store.Occupancy) string {
	name := "Guest"
	if o.GuestDisplayName.Valid && strings.TrimSpace(o.GuestDisplayName.String) != "" {
		name = strings.TrimSpace(o.GuestDisplayName.String)
	} else if o.RawSummary.Valid && strings.TrimSpace(o.RawSummary.String) != "" {
		name = normalizeGuestName(strings.TrimSpace(o.RawSummary.String))
	}
	return canonicalBookingLabel(name)
}

func buildGuestCodeLabelFromName(name string) string {
	return canonicalBookingLabel(name)
}

var nonWord = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func normalizeGuestName(s string) string {
	if i := strings.Index(s, "-"); i >= 0 && i+1 < len(s) {
		s = strings.TrimSpace(s[i+1:])
	}
	s = nonWord.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return "Guest"
	}
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return "Guest"
	}
	return parts[0]
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
