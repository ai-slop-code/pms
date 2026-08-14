package pms21cleanup

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pms/backend/internal/dbconn"
	"pms/backend/internal/migrate"
)

func TestAuditDoesNotMutateSource(t *testing.T) {
	path := automaticDatabase(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	report := Audit(context.Background(), testOptions(path))
	if !report.Passed {
		t.Fatalf("audit failed: errors=%v checks=%+v", report.Errors, failedChecks(report.Checks))
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("audit changed source database bytes")
	}
	if !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
		t.Fatal("audit changed source database modification time")
	}
	assertMigrationState(t, path, false)
	assertNoSidecars(t, path)
	if report.DatabaseBefore.SHA256 == "" || report.DatabaseBefore.Size == 0 {
		t.Fatal("audit omitted database fingerprint")
	}
	if len(report.TableCounts.Before) == 0 || len(report.TableCounts.After) == 0 {
		t.Fatal("audit omitted before/after table counts")
	}
	if err := VerifyChecksum(report); err != nil {
		t.Fatalf("audit checksum: %v", err)
	}
}

func TestAuditRejectsSQLiteSidecars(t *testing.T) {
	for _, test := range []struct {
		suffix  string
		checkID string
	}{
		{suffix: "-wal", checkID: "gate.sqlite_wal_absent"},
		{suffix: "-shm", checkID: "gate.sqlite_shm_absent"},
	} {
		t.Run(test.suffix, func(t *testing.T) {
			path := automaticDatabase(t)
			if err := os.WriteFile(path+test.suffix, []byte("sidecar"), 0o600); err != nil {
				t.Fatal(err)
			}
			report := Audit(context.Background(), testOptions(path))
			if report.Passed || checkByID(t, report.Checks, test.checkID).Passed {
				t.Fatalf("audit accepted %s sidecar", test.suffix)
			}
			if err := VerifyChecksum(report); err != nil {
				t.Fatalf("failed sidecar report checksum: %v", err)
			}
		})
	}
}

func TestImageDigestMustBeImmutable(t *testing.T) {
	path := automaticDatabase(t)
	for _, digest := range []string{
		"ghcr.io/example/pms:latest",
		"ghcr.io/example/pms:release-c",
		"sha256:short",
		"ghcr.io/example/pms@sha256:" + strings.Repeat("g", 64),
	} {
		t.Run(digest, func(t *testing.T) {
			opts := testOptions(path)
			opts.ImageDigest = digest
			report := Audit(context.Background(), opts)
			if report.Passed || !strings.Contains(strings.Join(report.Errors, " "), "64 hex") {
				t.Fatalf("accepted mutable or malformed digest %q: %+v", digest, report)
			}
		})
	}
}

func TestAuditRequiresOperatorAndEvidenceReferences(t *testing.T) {
	path := automaticDatabase(t)
	for _, test := range []struct {
		name    string
		checkID string
		clear   func(*Options)
	}{
		{name: "operator", checkID: "gate.operator", clear: func(opts *Options) { opts.Operator = "" }},
		{name: "exception register", checkID: "gate.exception_references", clear: func(opts *Options) { opts.ExceptionRegisterReference = "" }},
		{name: "approved exceptions", checkID: "gate.exception_references", clear: func(opts *Options) { opts.ApprovedExceptionsReference = "" }},
		{name: "external evidence", checkID: "gate.external_evidence_references", clear: func(opts *Options) { opts.AnalyticsParityReference = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			opts := testOptions(path)
			test.clear(&opts)
			report := Audit(context.Background(), opts)
			if report.Passed || checkByID(t, report.Checks, test.checkID).Passed {
				t.Fatalf("audit accepted missing %s", test.name)
			}
		})
	}
}

func TestAuditReportContainsOperationalEvidence(t *testing.T) {
	path := automaticDatabase(t)
	opts := testOptions(path)
	report := Audit(context.Background(), opts)
	if !report.Passed {
		t.Fatalf("audit failed: errors=%v checks=%+v", report.Errors, failedChecks(report.Checks))
	}
	if report.CommandMode != "audit" || len(report.CommandArguments) == 0 || report.EffectiveFlags != "none" {
		t.Fatalf("command evidence missing: mode=%q args=%v flags=%q", report.CommandMode, report.CommandArguments, report.EffectiveFlags)
	}
	if report.Operator != opts.Operator || report.ExceptionRegisterReference != opts.ExceptionRegisterReference || report.ApprovedExceptionsReference != opts.ApprovedExceptionsReference {
		t.Fatal("operator or exception references missing from report")
	}
	if report.ExternalEvidence.AnalyticsParity != opts.AnalyticsParityReference || report.ExternalEvidence.RemoteVerification != opts.RemoteVerificationReference || report.ExternalEvidence.CallerInventory != opts.CallerInventoryReference {
		t.Fatal("approved external evidence references missing from report")
	}
	if len(report.MigrationToolSHA256) != 64 {
		t.Fatalf("migration/tool checksum=%q", report.MigrationToolSHA256)
	}
	if !report.DatabaseBefore.WALAbsent || !report.DatabaseBefore.SHMAbsent {
		t.Fatalf("database fingerprint did not prove sidecars absent: %+v", report.DatabaseBefore)
	}
	if len(report.SchemaObjectsBefore) == 0 || len(report.SchemaObjectsAfter) == 0 || report.SQLiteSequenceBefore == nil || report.SQLiteSequenceAfter == nil {
		t.Fatal("schema/index/constraint or sqlite_sequence evidence missing")
	}
	for _, forbidden := range []string{"--password", "--token", "--secret"} {
		if strings.Contains(strings.Join(report.CommandArguments, " "), forbidden) {
			t.Fatalf("command arguments contain secret flag %q", forbidden)
		}
	}
}

func TestAuditReturnsChecksummedFailureForInvalidGate(t *testing.T) {
	path := automaticDatabase(t)
	db, err := dbconn.Open("sqlite://" + path)
	if err != nil {
		t.Fatal(err)
	}
	now := "2026-08-11T00:00:00Z"
	mustExec(t, db, `INSERT INTO users (id, email, password_hash, role, created_at, updated_at) VALUES (1, 'gate@test.invalid', 'hash', 'owner', ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO properties (id, name, timezone, owner_user_id, created_at, updated_at) VALUES (1, 'Gate', 'UTC', 1, ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO named_stays (id, property_id, display_name, stay_type, check_in_date, check_out_date, status, cleaning_required, review_status, first_known_at, created_at, updated_at) VALUES (1, 1, 'Sensitive Guest Name', 'external', '2026-08-11', '2026-08-12', 'archived', 0, 'needs_review', ?, ?, ?)`, now, now, now)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	report := Audit(context.Background(), testOptions(path))
	if report.Passed {
		t.Fatal("invalid readiness gate passed")
	}
	if checkByID(t, report.Checks, "stable.named_stays_review_resolved").Count != 1 {
		t.Fatal("review readiness violation was not counted")
	}
	if err := VerifyChecksum(report); err != nil {
		t.Fatalf("failed audit checksum: %v", err)
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Sensitive Guest Name") {
		t.Fatal("report emitted a guest name")
	}
}

func TestAuditBlocksUnsupportedMigratedLifecycleTimestamps(t *testing.T) {
	path := automaticDatabase(t)
	db, err := dbconn.Open("sqlite://" + path)
	if err != nil {
		t.Fatal(err)
	}
	now := "2026-08-11T00:00:00Z"
	mustExec(t, db, `INSERT INTO users (id, email, password_hash, role, created_at, updated_at) VALUES (1, 'lifecycle@test.invalid', 'hash', 'owner', ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO properties (id, name, timezone, owner_user_id, created_at, updated_at) VALUES (1, 'Lifecycle', 'UTC', 1, ?, ?)`, now, now)
	mustExec(t, db, `
		INSERT INTO named_stays (
			id, property_id, display_name, stay_type, check_in_date, check_out_date,
			status, cleaning_required, source_channel, review_status, review_resolution,
			created_at, updated_at
		) VALUES (1, 1, 'Unsupported migrated stay', 'booking_com', '2026-08-11',
			'2026-08-12', 'archived', 0, 'booking_com', 'confirmed', 'confirmed', ?, ?)`, now, now)
	mustExec(t, db, `
		INSERT INTO named_stays (
			id, property_id, display_name, stay_type, check_in_date, check_out_date,
			status, cleaning_required, source_channel, review_status, review_resolution,
			first_known_at, created_at, updated_at
		) VALUES (2, 1, 'Unsupported migrated cancellation', 'booking_com', '2026-08-13',
			'2026-08-14', 'cancelled', 0, 'booking_com', 'confirmed', 'confirmed', ?, ?, ?)`, now, now, now)
	mustExec(t, db, `
		INSERT INTO finance_bookings (
			property_id, reference_number, check_in_date, check_out_date, guest_name,
			net_cents, payout_date, named_stay_id, created_at, updated_at
		) VALUES (1, 'NO-LIFECYCLE-EVIDENCE', '2026-08-11', '2026-08-12',
			'Unsupported migrated stay', 0, '2026-08-13', 1, ?, ?)`, now, now)
	mustExec(t, db, `
		INSERT INTO finance_bookings (
			property_id, reference_number, check_in_date, check_out_date, guest_name,
			net_cents, payout_date, named_stay_id, booked_on, created_at, updated_at
		) VALUES (1, 'NO-CANCELLATION-EVIDENCE', '2026-08-13', '2026-08-14',
			'Unsupported migrated cancellation', 0, '2026-08-15', 2, ?, ?, ?)`, now, now, now)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	report := Audit(context.Background(), testOptions(path))
	check := checkByID(t, report.Checks, "stable.named_stays_lifecycle_complete")
	if report.Passed || check.Passed || check.Count != 2 {
		t.Fatalf("missing lifecycle evidence was not blocked: %+v", check)
	}
}

func TestAuditRequiresExactActiveNightParity(t *testing.T) {
	path := automaticDatabase(t)
	db, err := dbconn.Open("sqlite://" + path)
	if err != nil {
		t.Fatal(err)
	}
	now := "2026-08-11T00:00:00Z"
	mustExec(t, db, `INSERT INTO users (id, email, password_hash, role, created_at, updated_at) VALUES (1, 'night@test.invalid', 'hash', 'owner', ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO properties (id, name, timezone, owner_user_id, created_at, updated_at) VALUES (1, 'Night', 'UTC', 1, ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO named_stays (id, property_id, display_name, stay_type, check_in_date, check_out_date, status, cleaning_required, review_status, review_resolution, first_known_at, created_at, updated_at) VALUES (1, 1, 'Missing night', 'external', '2026-08-11', '2026-08-12', 'active', 0, 'confirmed', 'confirmed', ?, ?, ?)`, now, now, now)
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	report := Audit(context.Background(), testOptions(path))
	check := checkByID(t, report.Checks, "stable.named_stay_active_nights_exact")
	if report.Passed || check.Passed || check.Count != 1 {
		t.Fatalf("missing active night was not rejected: %+v", check)
	}
}

func TestAuditAcceptsResolvedRejectedReview(t *testing.T) {
	path := automaticDatabase(t)
	db, err := dbconn.Open("sqlite://" + path)
	if err != nil {
		t.Fatal(err)
	}
	now := "2026-08-11T00:00:00Z"
	mustExec(t, db, `INSERT INTO users (id, email, password_hash, role, created_at, updated_at) VALUES (1, 'rejected@test.invalid', 'hash', 'owner', ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO properties (id, name, timezone, owner_user_id, created_at, updated_at) VALUES (1, 'Rejected', 'UTC', 1, ?, ?)`, now, now)
	mustExec(t, db, `
		INSERT INTO named_stays (
			id, property_id, display_name, stay_type, check_in_date, check_out_date,
			status, cleaning_required, review_status, review_resolution, review_reason,
			review_actor_user_id, reviewed_at, first_known_at, created_at, updated_at
		) VALUES (1, 1, 'Resolved rejection', 'external', '2026-08-11', '2026-08-12',
			'archived', 0, 'needs_review', 'rejected', 'duplicate', 1, ?, ?, ?, ?)`, now, now, now, now)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	report := Audit(context.Background(), testOptions(path))
	if !report.Passed {
		t.Fatalf("resolved rejection failed cleanup audit: errors=%v checks=%+v", report.Errors, failedChecks(report.Checks))
	}
	if !checkByID(t, report.Checks, "stable.named_stays_review_resolved").Passed {
		t.Fatal("resolved rejection was counted as unresolved")
	}
}

func TestAuditAcceptsNamedStaySubrangesCoveredByOneRawBlock(t *testing.T) {
	path := automaticDatabase(t)
	db, err := dbconn.Open("sqlite://" + path)
	if err != nil {
		t.Fatal(err)
	}
	now := "2026-08-11T00:00:00Z"
	mustExec(t, db, `INSERT INTO users (id, email, password_hash, role, created_at, updated_at) VALUES (1, 'coverage@test.invalid', 'hash', 'owner', ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO properties (id, name, timezone, owner_user_id, created_at, updated_at) VALUES (1, 'Coverage', 'UTC', 1, ?, ?)`, now, now)
	mustExec(t, db, `
		INSERT INTO raw_booking_blocks (
			id, property_id, source_type, source_event_uid, check_in_date, check_out_date,
			status, content_hash, imported_at, last_synced_at, created_at, updated_at
		) VALUES (10, 1, 'booking_ics', 'shared-block', '2026-08-10', '2026-08-14',
			'active', 'hash', ?, ?, ?, ?)`, now, now, now, now)
	for id, day := range []string{"2026-08-10", "2026-08-11", "2026-08-12", "2026-08-13"} {
		mustExec(t, db, `INSERT INTO raw_booking_block_nights (id, property_id, raw_booking_block_id, local_night_date, active, created_at, updated_at) VALUES (?, 1, 10, ?, 1, ?, ?)`, id+1, day, now, now)
	}
	for _, stay := range []struct {
		id       int
		checkIn  string
		checkOut string
	}{
		{id: 20, checkIn: "2026-08-10", checkOut: "2026-08-12"},
		{id: 21, checkIn: "2026-08-12", checkOut: "2026-08-14"},
	} {
		mustExec(t, db, `
			INSERT INTO named_stays (
				id, property_id, display_name, stay_type, check_in_date, check_out_date,
				status, cleaning_required, review_status, review_resolution, first_known_at,
				nuki_generation_status, created_at, updated_at
			) VALUES (?, 1, 'Synthetic stay', 'booking_com', ?, ?, 'active', 1,
				'confirmed', 'confirmed', ?, 'pending', ?, ?)`, stay.id, stay.checkIn, stay.checkOut, now, now, now)
		for night, day := range []string{stay.checkIn, timeDayAfter(stay.checkIn)} {
			if day >= stay.checkOut {
				continue
			}
			mustExec(t, db, `INSERT INTO named_stay_nights (id, property_id, named_stay_id, local_night_date, active, created_at) VALUES (?, 1, ?, ?, 1, ?)`, stay.id*10+night, stay.id, day, now)
		}
		mustExec(t, db, `
			INSERT INTO stay_source_links (
				property_id, named_stay_id, raw_booking_block_id, source_type, source_event_uid,
				linked_check_in_date, linked_check_out_date, link_status, created_at, updated_at
			) VALUES (1, ?, 10, 'booking_ics', 'shared-block', ?, ?, 'active', ?, ?)`, stay.id, stay.checkIn, stay.checkOut, now, now)
	}
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	report := Audit(context.Background(), testOptions(path))
	if !report.Passed {
		t.Fatalf("valid union coverage failed audit: errors=%v checks=%+v", report.Errors, failedChecks(report.Checks))
	}
	if !checkByID(t, report.Checks, "stable.source_link_status_complete").Passed {
		t.Fatal("valid named-stay subranges were rejected")
	}
}

func TestRepairAppliesOnlyDeterministicReadinessFixes(t *testing.T) {
	path := automaticDatabase(t)
	db, err := dbconn.Open("sqlite://" + path)
	if err != nil {
		t.Fatal(err)
	}
	now := "2026-08-11T00:00:00Z"
	mustExec(t, db, `INSERT INTO users (id, email, password_hash, role, created_at, updated_at) VALUES (1, 'repair@test.invalid', 'hash', 'owner', ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO properties (id, name, timezone, owner_user_id, created_at, updated_at) VALUES (1, 'Repair', 'UTC', 1, ?, ?)`, now, now)
	mustExec(t, db, `
		INSERT INTO occupancies (
			id, property_id, source_type, source_event_uid, start_at, end_at, status,
			content_hash, imported_at, last_synced_at
		) VALUES (10, 1, 'booking_ics', 'repair-source', '2026-08-11T00:00:00Z',
			'2026-08-12T00:00:00Z', 'active', 'legacy-hash', ?, ?)`, now, now)
	mustExec(t, db, `
		INSERT INTO raw_booking_blocks (
			id, property_id, source_type, source_event_uid, check_in_date, check_out_date,
			status, content_hash, imported_at, last_synced_at, created_at, updated_at
		) VALUES (20, 1, 'booking_ics', 'repair-source', '2026-08-11', '2026-08-12',
			'active', 'raw-hash', ?, ?, ?, ?)`, now, now, now, now)
	mustExec(t, db, `INSERT INTO raw_booking_block_nights (id, property_id, raw_booking_block_id, local_night_date, active, created_at, updated_at) VALUES (21, 1, 20, '2026-08-11', 1, ?, ?)`, now, now)
	mustExec(t, db, `
		INSERT INTO named_stays (
			id, property_id, display_name, stay_type, check_in_date, check_out_date,
			status, cleaning_required, review_status, review_resolution, first_known_at,
			nuki_generation_status, created_at, updated_at
		) VALUES (30, 1, 'Cancelled stay', 'booking_com', '2026-08-01', '2026-08-02',
			'cancelled', 0, 'confirmed', 'confirmed', ?, 'not_applicable', ?, ?)`, now, now, now)
	mustExec(t, db, `
		INSERT INTO finance_bookings (
			id, property_id, reference_number, check_in_date, check_out_date, net_cents,
			payout_date, status, named_stay_id, created_at, updated_at
		) VALUES (40, 1, 'REPAIR-40', '2026-08-01', '2026-08-02', 0,
			'2026-08-03', 'CANCELLED', 30, ?, ?)`, now, now)
	mustExec(t, db, `
		INSERT INTO cleaning_calendar_events (
			id, property_id, occupancy_id, next_occupancy_id, raw_booking_block_id,
			checkout_date, cleaning_kind, google_calendar_id, cleaning_date, starts_at,
			ends_at, same_day_arrival, title, status, created_at, updated_at
		) VALUES (50, 1, 10, 10, 20, '2026-08-12', 'named_stay', 'calendar',
			'2026-08-12', '2026-08-12T10:00:00Z', '2026-08-12T12:00:00Z', 0,
			'Repair cleaning', 'removed', ?, ?)`, now, now)
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	opts := testOptions(path)
	withoutConfirmation := Repair(context.Background(), opts)
	if withoutConfirmation.Passed {
		t.Fatal("repair passed without confirmation")
	}
	opts.ConfirmRepair = true
	report := Repair(context.Background(), opts)
	if !report.Passed {
		t.Fatalf("repair failed: errors=%v checks=%+v", report.Errors, failedChecks(report.Checks))
	}
	if report.RepairCounts != (RepairCounts{CancellationTimestamps: 1, ExactSourceMappings: 1, LegacyCleaningPointers: 1, CleaningKinds: 1}) {
		t.Fatalf("repair counts=%+v", report.RepairCounts)
	}
	if !checksPassed(report.RemainingReadinessChecks) {
		t.Fatalf("readiness remained blocked: %+v", failedChecks(report.RemainingReadinessChecks))
	}

	second := Repair(context.Background(), opts)
	if !second.Passed || second.RepairCounts != (RepairCounts{}) {
		t.Fatalf("repair was not idempotent: passed=%t counts=%+v errors=%v", second.Passed, second.RepairCounts, second.Errors)
	}
}

func TestRepairDoesNotInferCancellationAfterStatusChange(t *testing.T) {
	path := automaticDatabase(t)
	db, err := dbconn.Open("sqlite://" + path)
	if err != nil {
		t.Fatal(err)
	}
	now := "2026-08-11T00:00:00Z"
	mustExec(t, db, `INSERT INTO users (id, email, password_hash, role, created_at, updated_at) VALUES (1, 'status-change@test.invalid', 'hash', 'owner', ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO properties (id, name, timezone, owner_user_id, created_at, updated_at) VALUES (1, 'Status change', 'UTC', 1, ?, ?)`, now, now)
	mustExec(t, db, `
		INSERT INTO named_stays (
			id, property_id, display_name, stay_type, check_in_date, check_out_date,
			status, cleaning_required, review_status, review_resolution, first_known_at,
			nuki_generation_status, created_at, updated_at
		) VALUES (1, 1, 'Changed cancellation', 'booking_com', '2026-08-01',
			'2026-08-02', 'cancelled', 0, 'confirmed', 'confirmed', ?,
			'not_applicable', ?, ?)`, now, now, now)
	mustExec(t, db, `INSERT INTO finance_imports (id, property_id, source_type, source_channel, uploaded_at, file_sha256) VALUES (1, 1, 'statement', 'booking_com', ?, 'status-change')`, now)
	mustExec(t, db, `
		INSERT INTO finance_bookings (
			id, property_id, reference_number, check_in_date, check_out_date, net_cents,
			payout_date, status, named_stay_id, created_at, updated_at
		) VALUES (1, 1, 'STATUS-CHANGE', '2026-08-01', '2026-08-02', 0,
			'2026-08-03', 'CANCELLED', 1, ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO finance_booking_merges (booking_id, import_id, source_type, changed_fields_json, occurred_at) VALUES (1, 1, 'statement', '["status"]', ?)`, now)
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	opts := testOptions(path)
	opts.ConfirmRepair = true
	report := Repair(context.Background(), opts)
	if !report.Passed || report.RepairCounts.CancellationTimestamps != 0 {
		t.Fatalf("ambiguous cancellation was repaired: passed=%t counts=%+v errors=%v", report.Passed, report.RepairCounts, report.Errors)
	}
	check := checkByID(t, report.RemainingReadinessChecks, "stable.named_stays_lifecycle_complete")
	if check.Passed || check.Count != 1 {
		t.Fatalf("ambiguous cancellation did not remain blocked: %+v", check)
	}
}

func timeDayAfter(day string) string {
	parsed, err := time.Parse("2006-01-02", day)
	if err != nil {
		panic(err)
	}
	return parsed.AddDate(0, 0, 1).Format("2006-01-02")
}

func TestApplyRefusesMissingBadAndStalePreReport(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		path := automaticDatabase(t)
		opts := testOptions(path)
		opts.Confirm = true
		report := Apply(context.Background(), opts)
		if report.Passed || checkByID(t, report.Checks, "gate.pre_report").Passed {
			t.Fatal("apply accepted missing pre-report")
		}
		assertMigrationState(t, path, false)
	})

	t.Run("bad checksum", func(t *testing.T) {
		path := automaticDatabase(t)
		audit := Audit(context.Background(), testOptions(path))
		if !audit.Passed {
			t.Fatalf("audit failed: %v", audit.Errors)
		}
		audit.Commit = "tampered"
		prePath := writeReport(t, audit)
		opts := testOptions(path)
		opts.Confirm = true
		opts.PreReport = prePath
		report := Apply(context.Background(), opts)
		if report.Passed || checkByID(t, report.Checks, "gate.pre_report_checksum").Passed {
			t.Fatal("apply accepted bad pre-report checksum")
		}
		assertMigrationState(t, path, false)
	})

	t.Run("failed gate", func(t *testing.T) {
		path := automaticDatabase(t)
		audit := Audit(context.Background(), testOptions(path))
		audit.Passed = false
		if err := SetChecksum(&audit); err != nil {
			t.Fatal(err)
		}
		opts := testOptions(path)
		opts.Confirm = true
		opts.PreReport = writeReport(t, audit)
		report := Apply(context.Background(), opts)
		if report.Passed || checkByID(t, report.Checks, "gate.pre_report_passed").Passed {
			t.Fatal("apply accepted failed readiness report")
		}
		assertMigrationState(t, path, false)
	})

	t.Run("stale fingerprint", func(t *testing.T) {
		path := automaticDatabase(t)
		audit := Audit(context.Background(), testOptions(path))
		if !audit.Passed {
			t.Fatalf("audit failed: %v", audit.Errors)
		}
		prePath := writeReport(t, audit)
		db, err := dbconn.Open("sqlite://" + path)
		if err != nil {
			t.Fatal(err)
		}
		mustExec(t, db, `CREATE TABLE post_audit_change (id INTEGER)`)
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		opts := testOptions(path)
		opts.Confirm = true
		opts.PreReport = prePath
		report := Apply(context.Background(), opts)
		if report.Passed || checkByID(t, report.Checks, "gate.database_fingerprint").Passed {
			t.Fatal("apply accepted stale database fingerprint")
		}
		assertMigrationState(t, path, false)
	})
}

func TestApplyRejectsSidecarCreatedAfterAudit(t *testing.T) {
	path := automaticDatabase(t)
	audit := Audit(context.Background(), testOptions(path))
	if !audit.Passed {
		t.Fatalf("audit failed: %v", audit.Errors)
	}
	for _, test := range []struct {
		suffix  string
		checkID string
	}{
		{suffix: "-wal", checkID: "gate.sqlite_wal_absent"},
		{suffix: "-shm", checkID: "gate.sqlite_shm_absent"},
	} {
		t.Run(test.suffix, func(t *testing.T) {
			if err := os.WriteFile(path+test.suffix, []byte("late sidecar"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Remove(path + test.suffix) })
			opts := testOptions(path)
			opts.Confirm = true
			opts.PreReport = writeReport(t, audit)
			report := Apply(context.Background(), opts)
			if report.Passed || checkByID(t, report.Checks, test.checkID).Passed {
				t.Fatalf("apply accepted %s created after audit", test.suffix)
			}
			if err := os.Remove(path + test.suffix); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestApplyRechecksSidecarsAndFingerprintImmediatelyBeforeWritableOpen(t *testing.T) {
	t.Run("sidecar", func(t *testing.T) {
		path := automaticDatabase(t)
		audit := Audit(context.Background(), testOptions(path))
		if !audit.Passed {
			t.Fatalf("audit failed: %v", audit.Errors)
		}
		opts := testOptions(path)
		opts.Confirm = true
		opts.PreReport = writeReport(t, audit)
		opts.beforeWritableOpen = func() {
			if err := os.WriteFile(path+"-shm", []byte("raced"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		report := Apply(context.Background(), opts)
		if report.Passed || checkByID(t, report.Checks, "gate.pre_apply_sqlite_shm_absent").Passed {
			t.Fatal("apply accepted sidecar created immediately before writable open")
		}
		_ = os.Remove(path + "-shm")
		assertMigrationState(t, path, false)
	})

	t.Run("fingerprint", func(t *testing.T) {
		path := automaticDatabase(t)
		audit := Audit(context.Background(), testOptions(path))
		if !audit.Passed {
			t.Fatalf("audit failed: %v", audit.Errors)
		}
		opts := testOptions(path)
		opts.Confirm = true
		opts.PreReport = writeReport(t, audit)
		opts.beforeWritableOpen = func() {
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.Write([]byte{0}); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
		}
		report := Apply(context.Background(), opts)
		if report.Passed || checkByID(t, report.Checks, "gate.pre_apply_database_fingerprint").Passed {
			t.Fatal("apply accepted database changed immediately before writable open")
		}
		assertMigrationState(t, path, false)
	})
}

func TestApplyRejectsChangedEvidenceIdentity(t *testing.T) {
	path := automaticDatabase(t)
	audit := Audit(context.Background(), testOptions(path))
	if !audit.Passed {
		t.Fatalf("audit failed: %v", audit.Errors)
	}
	opts := testOptions(path)
	opts.Confirm = true
	opts.PreReport = writeReport(t, audit)
	opts.AnalyticsParityReference = "restricted://different-approved-artifact#sha256=" + strings.Repeat("1", 64)
	report := Apply(context.Background(), opts)
	if report.Passed || checkByID(t, report.Checks, "gate.evidence_identity").Passed {
		t.Fatal("apply accepted changed approved external evidence identity")
	}
	assertMigrationState(t, path, false)
}

func TestAuditReportsMissingInvoiceFileAsFailure(t *testing.T) {
	path := automaticDatabase(t)
	db, err := dbconn.Open("sqlite://" + path)
	if err != nil {
		t.Fatal(err)
	}
	now := "2026-08-11T00:00:00Z"
	mustExec(t, db, `INSERT INTO users (id, email, password_hash, role, created_at, updated_at) VALUES (1, 'invoice@test.invalid', 'hash', 'owner', ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO properties (id, name, timezone, owner_user_id, created_at, updated_at) VALUES (1, 'Invoice', 'UTC', 1, ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO named_stays (id, property_id, display_name, stay_type, check_in_date, check_out_date, status, cleaning_required, review_status, review_resolution, first_known_at, created_at, updated_at) VALUES (1, 1, 'Invoice stay', 'external', '2026-08-11', '2026-08-12', 'archived', 0, 'confirmed', 'confirmed', ?, ?, ?)`, now, now, now)
	mustExec(t, db, `INSERT INTO invoices (id, property_id, invoice_number, sequence_year, sequence_value, language, issue_date, taxable_supply_date, due_date, stay_start_date, stay_end_date, supplier_snapshot_json, customer_snapshot_json, amount_total_cents, currency, payment_status, payment_note, version, created_at, updated_at, named_stay_id) VALUES (1, 1, 'INV-1', 2026, 1, 'en', '2026-08-11', '2026-08-11', '2026-08-12', '2026-08-11', '2026-08-12', '{}', '{}', 100, 'EUR', 'paid', 'paid', 1, ?, ?, 1)`, now, now)
	mustExec(t, db, `INSERT INTO invoice_files (id, invoice_id, version, file_path, file_size_bytes, created_at) VALUES (1, 1, 1, 'invoices/missing.pdf', 10, ?)`, now)
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	opts := testOptions(path)
	opts.DataRoot = t.TempDir()
	report := Audit(context.Background(), opts)
	check := checkByID(t, report.Checks, "files.invoice_content_checksums")
	if report.Passed || check.Passed || check.Count != 1 {
		t.Fatalf("missing invoice file was not a failing check: %+v", check)
	}
	if len(report.InvoiceFileChecksums.Before) != 1 || report.InvoiceFileChecksums.Before[0].Failure != "missing_or_unreadable" {
		t.Fatalf("missing invoice evidence=%+v", report.InvoiceFileChecksums.Before)
	}
}

func TestApplySuccessfullyRunsDestructiveMigration(t *testing.T) {
	path := automaticDatabase(t)
	audit := Audit(context.Background(), testOptions(path))
	if !audit.Passed {
		t.Fatalf("audit failed: errors=%v checks=%+v", audit.Errors, failedChecks(audit.Checks))
	}
	opts := testOptions(path)
	opts.Confirm = true
	opts.PreReport = writeReport(t, audit)

	report := Apply(context.Background(), opts)
	if !report.Passed {
		t.Fatalf("apply failed: errors=%v checks=%+v", report.Errors, failedChecks(report.Checks))
	}
	if err := VerifyChecksum(report); err != nil {
		t.Fatalf("apply checksum: %v", err)
	}
	assertMigrationState(t, path, true)
	if report.DatabaseAfter.SHA256 == "" || report.DatabaseAfter == report.DatabaseBefore {
		t.Fatal("apply omitted distinct post-cleanup fingerprint")
	}
	if !checkByID(t, report.Checks, "post.critical_aggregates_preserved").Passed {
		t.Fatal("critical aggregates changed")
	}
}

func TestReportChecksumExcludesOnlyChecksumField(t *testing.T) {
	r := newReport("audit", testOptions(filepath.Join(t.TempDir(), "db")))
	r.Passed = true
	r.CompletedAt = "2026-08-11T00:00:00Z"
	if err := SetChecksum(&r); err != nil {
		t.Fatal(err)
	}
	first := r.ReportChecksum
	if err := SetChecksum(&r); err != nil {
		t.Fatal(err)
	}
	if r.ReportChecksum != first {
		t.Fatal("checksum included its own field")
	}
	if err := VerifyChecksum(r); err != nil {
		t.Fatal(err)
	}
	r.Commit = "changed"
	if err := VerifyChecksum(r); err == nil {
		t.Fatal("checksum did not cover report fields")
	}
}

func automaticDatabase(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pms.db")
	db, err := dbconn.Open("sqlite://" + path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.UpAutomatic(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	assertNoSidecars(t, path)
	return path
}

func testOptions(path string) Options {
	return Options{
		DBPath:                      path,
		ImageDigest:                 "ghcr.io/example/pms@sha256:" + strings.Repeat("a", 64),
		Commit:                      "test-commit",
		FrontendBuild:               "test-frontend",
		Operator:                    "test-operator",
		ExceptionRegisterReference:  "restricted://exception-register#sha256=" + strings.Repeat("b", 64),
		ApprovedExceptionsReference: "restricted://exception-approval#sha256=" + strings.Repeat("c", 64),
		AnalyticsParityReference:    "restricted://analytics-parity#sha256=" + strings.Repeat("d", 64),
		RemoteVerificationReference: "restricted://remote-verification#sha256=" + strings.Repeat("e", 64),
		CallerInventoryReference:    "restricted://caller-inventory#sha256=" + strings.Repeat("f", 64),
	}
}

func writeReport(t *testing.T, report Report) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "audit.json")
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertMigrationState(t *testing.T, path string, cleaned bool) {
	t.Helper()
	db, err := dbconn.OpenReadOnly("sqlite://" + path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var applied, legacy int
	if err := db.QueryRow(`SELECT count(*) FROM schema_migrations WHERE version = '000039_legacy_occupancy_removal'`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = 'occupancies'`).Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	if cleaned && (applied != 1 || legacy != 0) {
		t.Fatalf("cleaned state applied=%d legacy=%d", applied, legacy)
	}
	if !cleaned && (applied != 0 || legacy != 1) {
		t.Fatalf("pending state applied=%d legacy=%d", applied, legacy)
	}
}

func assertNoSidecars(t *testing.T, path string) {
	t.Helper()
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Stat(path + suffix); !os.IsNotExist(err) {
			t.Fatalf("unexpected sidecar %s: %v", suffix, err)
		}
	}
}

func mustExec(t *testing.T, db interface {
	Exec(string, ...any) (sql.Result, error)
}, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func checkByID(t *testing.T, checks []Check, id string) Check {
	t.Helper()
	for _, check := range checks {
		if check.ID == id {
			return check
		}
	}
	t.Fatalf("check %q not found", id)
	return Check{}
}

func failedChecks(checks []Check) []Check {
	var failed []Check
	for _, check := range checks {
		if !check.Passed {
			failed = append(failed, check)
		}
	}
	return failed
}
