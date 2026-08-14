// Package pms21cleanup implements the gated PMS 21 destructive cleanup audit
// and apply workflow. Reports contain checksums and operational references,
// never business row values.
package pms21cleanup

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"pms/backend/internal/dbconn"
	"pms/backend/internal/migrate"
)

const reportFormat = "pms21-cleanup/v2"

var immutableDigestPattern = regexp.MustCompile(`^(?:[^@\s]+@)?sha256:[0-9a-fA-F]{64}$`)

type Options struct {
	DBPath                      string
	DataRoot                    string
	ImageDigest                 string
	Commit                      string
	FrontendBuild               string
	Operator                    string
	ExceptionRegisterReference  string
	ApprovedExceptionsReference string
	AnalyticsParityReference    string
	RemoteVerificationReference string
	CallerInventoryReference    string
	PreReport                   string
	Confirm                     bool
	ConfirmRepair               bool

	beforeWritableOpen func()
}

type Fingerprint struct {
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size_bytes"`
	WALAbsent bool   `json:"wal_absent"`
	SHMAbsent bool   `json:"shm_absent"`
}

type Check struct {
	ID     string `json:"id"`
	Passed bool   `json:"passed"`
	Count  int64  `json:"violation_count"`
	Error  string `json:"error,omitempty"`
}

type Counts struct {
	Before map[string]int64 `json:"before"`
	After  map[string]int64 `json:"after"`
}

type Aggregates struct {
	Before map[string]string `json:"before"`
	After  map[string]string `json:"after"`
}

type SchemaObject struct {
	Type             string `json:"type"`
	Name             string `json:"name"`
	Table            string `json:"table"`
	DefinitionSHA256 string `json:"definition_sha256"`
}

type FileChecksum struct {
	InvoiceFileID int64  `json:"invoice_file_id"`
	Accessible    bool   `json:"accessible"`
	Size          int64  `json:"size_bytes,omitempty"`
	SHA256        string `json:"sha256,omitempty"`
	Failure       string `json:"failure,omitempty"`
}

type FileChecksums struct {
	Before []FileChecksum `json:"before"`
	After  []FileChecksum `json:"after"`
}

type RepairCounts struct {
	CancellationTimestamps int64 `json:"cancellation_timestamps"`
	ExactSourceMappings    int64 `json:"exact_source_mappings"`
	LegacyCleaningPointers int64 `json:"legacy_cleaning_pointers"`
	CleaningKinds          int64 `json:"cleaning_kinds"`
}

type ExternalEvidence struct {
	AnalyticsParity    string `json:"analytics_parity_reference"`
	RemoteVerification string `json:"remote_verification_reference"`
	CallerInventory    string `json:"caller_inventory_reference"`
}

type Report struct {
	Format                      string           `json:"format"`
	Mode                        string           `json:"mode"`
	Passed                      bool             `json:"passed"`
	StartedAt                   string           `json:"started_at_utc"`
	CompletedAt                 string           `json:"completed_at_utc"`
	ImageDigest                 string           `json:"image_digest"`
	Commit                      string           `json:"commit"`
	FrontendBuild               string           `json:"frontend_build"`
	Operator                    string           `json:"operator"`
	CommandMode                 string           `json:"command_mode"`
	CommandArguments            []string         `json:"command_arguments"`
	EffectiveFlags              string           `json:"effective_pms21_flags"`
	MigrationToolSHA256         string           `json:"migration_tool_sha256"`
	ExceptionRegisterReference  string           `json:"exception_register_reference"`
	ApprovedExceptionsReference string           `json:"approved_exceptions_reference"`
	ExternalEvidence            ExternalEvidence `json:"approved_external_evidence"`
	DatabaseBefore              Fingerprint      `json:"database_before"`
	DatabaseAfter               Fingerprint      `json:"database_after,omitempty"`
	SchemaBefore                []string         `json:"schema_versions_before"`
	SchemaAfter                 []string         `json:"schema_versions_after"`
	SchemaObjectsBefore         []SchemaObject   `json:"schema_index_constraint_hashes_before"`
	SchemaObjectsAfter          []SchemaObject   `json:"schema_index_constraint_hashes_after"`
	SQLiteSequenceBefore        map[string]int64 `json:"sqlite_sequence_before"`
	SQLiteSequenceAfter         map[string]int64 `json:"sqlite_sequence_after"`
	InvoiceFileChecksums        FileChecksums    `json:"invoice_file_content_checksums"`
	RepairCounts                RepairCounts     `json:"repair_counts,omitempty"`
	RemainingReadinessChecks    []Check          `json:"remaining_readiness_checks,omitempty"`
	Checks                      []Check          `json:"checks"`
	TableCounts                 Counts           `json:"table_counts"`
	CriticalAggregates          Aggregates       `json:"critical_aggregate_sha256"`
	Errors                      []string         `json:"errors,omitempty"`
	ReportChecksum              string           `json:"report_checksum,omitempty"`
}

type readinessCheck struct {
	id    string
	query string
}

const sourceLinkReadinessQuery = `WITH health AS (
	SELECT ns.id AS stay_id, ns.property_id,
		CASE
			WHEN COUNT(DISTINCT rb.id) = 0 OR COUNT(DISTINCT CASE WHEN rbn.active = 1 AND rbn.local_night_date >= ns.check_in_date AND rbn.local_night_date < ns.check_out_date THEN rbn.local_night_date END) = 0 THEN 'source_deleted'
			WHEN COUNT(DISTINCT CASE WHEN rbn.active = 1 AND rbn.local_night_date >= ns.check_in_date AND rbn.local_night_date < ns.check_out_date THEN rbn.local_night_date END) <> CAST(julianday(ns.check_out_date) - julianday(ns.check_in_date) AS INTEGER) THEN 'conflict'
			ELSE 'active'
		END AS expected_status
	FROM named_stays ns
	JOIN stay_source_links l ON l.named_stay_id = ns.id AND l.property_id = ns.property_id AND l.link_status <> 'manual_unlinked'
	LEFT JOIN raw_booking_blocks rb ON rb.id = l.raw_booking_block_id AND rb.property_id = l.property_id AND rb.status = 'active'
	LEFT JOIN raw_booking_block_nights rbn ON rbn.raw_booking_block_id = rb.id AND rbn.property_id = l.property_id
	WHERE ns.status = 'active'
	GROUP BY ns.id, ns.property_id
)
SELECT count(*)
FROM stay_source_links l
LEFT JOIN named_stays s ON s.id = l.named_stay_id
LEFT JOIN raw_booking_blocks b ON b.id = l.raw_booking_block_id
LEFT JOIN health h ON h.stay_id = l.named_stay_id AND h.property_id = l.property_id
WHERE s.id IS NULL OR s.property_id <> l.property_id
	OR date(l.linked_check_in_date) <> l.linked_check_in_date
	OR date(l.linked_check_out_date) <> l.linked_check_out_date
	OR l.linked_check_out_date <= l.linked_check_in_date
	OR l.linked_check_in_date <> s.check_in_date OR l.linked_check_out_date <> s.check_out_date
	OR (l.raw_booking_block_id IS NOT NULL AND (b.id IS NULL OR b.property_id <> l.property_id OR l.source_type IS NOT b.source_type OR l.source_event_uid IS NOT b.source_event_uid))
	OR (l.link_status = 'manual_unlinked' AND l.raw_booking_block_id IS NOT NULL)
	OR (l.link_status IN ('active', 'conflict') AND l.raw_booking_block_id IS NULL)
	OR l.link_status NOT IN ('active', 'source_deleted', 'conflict', 'manual_unlinked')
	OR (h.expected_status IS NOT NULL AND l.link_status <> h.expected_status)`

var readinessChecks = []readinessCheck{
	{"schema.000038_applied", `SELECT CASE WHEN EXISTS (SELECT 1 FROM schema_migrations WHERE version = '000038_named_stay_lifecycle_metadata') THEN 0 ELSE 1 END`},
	{"schema.000039_pending", `SELECT CASE WHEN EXISTS (SELECT 1 FROM schema_migrations WHERE version = '000039_legacy_occupancy_removal') THEN 1 ELSE 0 END`},
	{"database.foreign_key_check", `SELECT count(*) FROM pragma_foreign_key_check`},
	{"database.integrity_check", `SELECT count(*) FROM pragma_integrity_check WHERE integrity_check <> 'ok'`},
	{"stable.named_stays_review_resolved", `SELECT count(*) FROM named_stays WHERE review_resolution IS NULL`},
	{"stable.named_stays_lifecycle_complete", `SELECT count(*) FROM named_stays WHERE first_known_at IS NULL OR trim(first_known_at) = '' OR julianday(first_known_at) IS NULL OR (status = 'cancelled' AND (cancellation_effective_at IS NULL OR trim(cancellation_effective_at) = '' OR julianday(cancellation_effective_at) IS NULL))`},
	{"stable.legacy_api_tokens_retired", `SELECT count(*) FROM occupancy_api_tokens`},
	{"stable.legacy_occupancies_mapped", `SELECT count(*) FROM occupancies o LEFT JOIN occupancy_stay_migration_map m ON m.old_occupancy_id = o.id WHERE m.old_occupancy_id IS NULL`},
	{"stable.legacy_mapping_targets", `SELECT count(*) FROM occupancy_stay_migration_map m LEFT JOIN occupancies o ON o.id = m.old_occupancy_id LEFT JOIN raw_booking_blocks r ON r.id = m.raw_booking_block_id LEFT JOIN named_stays s ON s.id = m.named_stay_id LEFT JOIN property_availability_blocks a ON a.id = m.availability_block_id WHERE m.migration_kind = 'unmapped' OR o.id IS NULL OR o.property_id <> m.property_id OR (m.raw_booking_block_id IS NOT NULL AND (r.id IS NULL OR r.property_id <> m.property_id)) OR (m.named_stay_id IS NOT NULL AND (s.id IS NULL OR s.property_id <> m.property_id)) OR (m.availability_block_id IS NOT NULL AND (a.id IS NULL OR a.property_id <> m.property_id)) OR ((m.raw_booking_block_id IS NOT NULL) + (m.named_stay_id IS NOT NULL) + (m.availability_block_id IS NOT NULL)) <> 1 OR (m.migration_kind = 'raw_block' AND m.raw_booking_block_id IS NULL) OR (m.migration_kind IN ('named_stay', 'synthetic_finance') AND m.named_stay_id IS NULL) OR (m.migration_kind IN ('availability_block', 'closure') AND m.availability_block_id IS NULL)`},
	{"stable.nuki_access_code_owners", `SELECT count(*) FROM nuki_access_codes c LEFT JOIN named_stays s ON s.id = c.named_stay_id WHERE c.named_stay_id IS NULL OR s.id IS NULL OR s.property_id <> c.property_id`},
	{"stable.nuki_guest_entry_owners", `SELECT count(*) FROM nuki_guest_daily_entries e LEFT JOIN named_stays s ON s.id = e.named_stay_id WHERE e.named_stay_id IS NULL OR s.id IS NULL OR s.property_id <> e.property_id`},
	{"stable.finance_booking_owners", `SELECT count(*) FROM finance_bookings b LEFT JOIN named_stays s ON s.id = b.named_stay_id WHERE b.named_stay_id IS NULL OR s.id IS NULL OR s.property_id <> b.property_id`},
	{"stable.invoice_owners", `SELECT count(*) FROM invoices i LEFT JOIN named_stays s ON s.id = i.named_stay_id WHERE i.named_stay_id IS NULL OR s.id IS NULL OR s.property_id <> i.property_id`},
	{"stable.invoice_finance_owners", `SELECT count(*) FROM invoices i JOIN finance_bookings b ON b.id = i.finance_booking_payout_id WHERE b.property_id <> i.property_id OR b.named_stay_id <> i.named_stay_id`},
	{"stable.cleaning_event_owners", `SELECT count(*) FROM cleaning_calendar_events e LEFT JOIN named_stays s ON s.id = e.named_stay_id LEFT JOIN raw_booking_blocks b ON b.id = e.raw_booking_block_id WHERE (e.named_stay_id IS NULL) = (e.raw_booking_block_id IS NULL) OR (e.named_stay_id IS NOT NULL AND (s.id IS NULL OR s.property_id <> e.property_id)) OR (e.raw_booking_block_id IS NOT NULL AND (b.id IS NULL OR b.property_id <> e.property_id))`},
	{"stable.cleaning_kind_complete", `SELECT count(*) FROM cleaning_calendar_events WHERE (cleaning_kind = 'named_stay' AND (named_stay_id IS NULL OR raw_booking_block_id IS NOT NULL)) OR (cleaning_kind = 'provisional_block' AND (raw_booking_block_id IS NULL OR named_stay_id IS NOT NULL)) OR cleaning_kind NOT IN ('named_stay', 'provisional_block') OR next_occupancy_id IS NOT NULL`},
	{"stable.named_stay_night_owners", `SELECT count(*) FROM named_stay_nights n LEFT JOIN named_stays s ON s.id = n.named_stay_id WHERE s.id IS NULL OR s.property_id <> n.property_id`},
	{"stable.raw_block_night_owners", `SELECT count(*) FROM raw_booking_block_nights n LEFT JOIN raw_booking_blocks b ON b.id = n.raw_booking_block_id WHERE b.id IS NULL OR b.property_id <> n.property_id`},
	{"stable.named_stay_active_nights_exact", `WITH RECURSIVE expected(stay_id, property_id, night_date, end_date) AS (SELECT id, property_id, check_in_date, check_out_date FROM named_stays WHERE status = 'active' UNION ALL SELECT stay_id, property_id, date(night_date, '+1 day'), end_date FROM expected WHERE date(night_date, '+1 day') < end_date), missing AS (SELECT stay_id, property_id, night_date FROM expected EXCEPT SELECT named_stay_id, property_id, local_night_date FROM named_stay_nights WHERE active = 1), unexpected AS (SELECT named_stay_id, property_id, local_night_date FROM named_stay_nights WHERE active = 1 EXCEPT SELECT stay_id, property_id, night_date FROM expected) SELECT (SELECT count(*) FROM missing) + (SELECT count(*) FROM unexpected)`},
	{"stable.raw_block_active_nights_exact", `WITH RECURSIVE expected(block_id, property_id, night_date, end_date) AS (SELECT id, property_id, check_in_date, check_out_date FROM raw_booking_blocks WHERE status = 'active' UNION ALL SELECT block_id, property_id, date(night_date, '+1 day'), end_date FROM expected WHERE date(night_date, '+1 day') < end_date), missing AS (SELECT block_id, property_id, night_date FROM expected EXCEPT SELECT raw_booking_block_id, property_id, local_night_date FROM raw_booking_block_nights WHERE active = 1), unexpected AS (SELECT raw_booking_block_id, property_id, local_night_date FROM raw_booking_block_nights WHERE active = 1 EXCEPT SELECT block_id, property_id, night_date FROM expected) SELECT (SELECT count(*) FROM missing) + (SELECT count(*) FROM unexpected)`},
	{"stable.availability_named_stay_overlap", `SELECT count(*) FROM property_availability_blocks a JOIN named_stay_nights n ON n.property_id = a.property_id AND n.active = 1 AND n.local_night_date >= a.start_date AND n.local_night_date < a.end_date WHERE a.status = 'active'`},
	{"stable.source_link_status_complete", sourceLinkReadinessQuery},
	{"stable.child_reference_owners", `SELECT count(*) FROM (SELECT l.id FROM nuki_event_logs l JOIN nuki_access_codes c ON c.id = l.nuki_access_code_id WHERE c.property_id <> l.property_id UNION ALL SELECT l.id FROM cleaning_calendar_event_logs l JOIN cleaning_calendar_events e ON e.id = l.cleaning_calendar_event_id WHERE e.property_id <> l.property_id UNION ALL SELECT m.id FROM finance_booking_merges m JOIN finance_bookings b ON b.id = m.booking_id JOIN finance_imports i ON i.id = m.import_id WHERE b.property_id <> i.property_id UNION ALL SELECT f.id FROM invoice_files f LEFT JOIN invoices i ON i.id = f.invoice_id WHERE i.id IS NULL)`},
	{"stable.critical_fields_complete", `SELECT count(*) FROM (SELECT id FROM named_stays WHERE trim(display_name) = '' OR date(check_in_date) <> check_in_date OR date(check_out_date) <> check_out_date OR check_out_date <= check_in_date OR first_known_at IS NULL OR trim(first_known_at) = '' OR julianday(first_known_at) IS NULL OR (status = 'cancelled' AND (cancellation_effective_at IS NULL OR trim(cancellation_effective_at) = '' OR julianday(cancellation_effective_at) IS NULL)) UNION ALL SELECT id FROM raw_booking_blocks WHERE trim(source_type) = '' OR trim(source_event_uid) = '' OR trim(content_hash) = '' OR date(check_in_date) <> check_in_date OR date(check_out_date) <> check_out_date OR check_out_date <= check_in_date OR trim(imported_at) = '' OR trim(last_synced_at) = '' UNION ALL SELECT id FROM property_availability_blocks WHERE date(start_date) <> start_date OR date(end_date) <> end_date OR end_date <= start_date UNION ALL SELECT id FROM finance_bookings WHERE trim(reference_number) = '' OR trim(payout_date) = '' OR trim(created_at) = '' OR trim(updated_at) = '' UNION ALL SELECT id FROM invoices WHERE trim(invoice_number) = '' OR trim(supplier_snapshot_json) = '' OR trim(customer_snapshot_json) = '' OR trim(currency) = '' OR sequence_value <= 0 OR version <= 0 UNION ALL SELECT id FROM invoice_files WHERE trim(file_path) = '' OR file_size_bytes < 0 OR version <= 0 UNION ALL SELECT id FROM named_stay_nights WHERE active NOT IN (0, 1) OR date(local_night_date) <> local_night_date UNION ALL SELECT id FROM raw_booking_block_nights WHERE active NOT IN (0, 1) OR date(local_night_date) <> local_night_date)`},
	{"stable.nuki_access_code_uniqueness", `SELECT count(*) FROM (SELECT 1 FROM nuki_access_codes WHERE named_stay_id IS NOT NULL GROUP BY property_id, named_stay_id HAVING count(*) > 1)`},
	{"stable.nuki_guest_entry_uniqueness", `SELECT count(*) FROM (SELECT 1 FROM nuki_guest_daily_entries WHERE named_stay_id IS NOT NULL GROUP BY property_id, named_stay_id, day_date HAVING count(*) > 1)`},
	{"stable.invoice_named_stay_uniqueness", `SELECT count(*) FROM (SELECT 1 FROM invoices WHERE named_stay_id IS NOT NULL GROUP BY property_id, named_stay_id HAVING count(*) > 1)`},
}

var criticalTables = map[string][]string{
	"named_stays":                  {"id", "property_id", "display_name", "stay_type", "check_in_date", "check_out_date", "status", "cleaning_required", "cleaning_override_reason", "source_channel", "source_reference", "manual_revenue_cents", "manual_revenue_currency", "manual_revenue_note", "review_status", "review_resolution", "review_reason", "stay_outcome", "stay_outcome_reason", "stay_outcome_marked_at", "nuki_generation_status", "nuki_generation_error", "nuki_generation_updated_at", "created_by_user_id", "updated_by_user_id", "created_at", "updated_at", "first_known_at", "cancellation_effective_at", "stay_outcome_actor_user_id", "review_actor_user_id", "reviewed_at"},
	"named_stay_nights":            {"id", "property_id", "named_stay_id", "local_night_date", "active", "created_at"},
	"raw_booking_blocks":           {"id", "property_id", "source_type", "source_event_uid", "check_in_date", "check_out_date", "status", "raw_summary", "content_hash", "source_dtstamp", "first_seen_sync_run_id", "last_sync_run_id", "imported_at", "last_synced_at", "deleted_from_source_at", "conflict_reason", "created_at", "updated_at"},
	"raw_booking_block_nights":     {"id", "property_id", "raw_booking_block_id", "local_night_date", "active", "created_at", "updated_at"},
	"stay_source_links":            {"id", "property_id", "named_stay_id", "raw_booking_block_id", "source_type", "source_event_uid", "linked_check_in_date", "linked_check_out_date", "link_status", "created_at", "updated_at"},
	"property_availability_blocks": {"id", "property_id", "block_type", "start_date", "end_date", "reason", "status", "created_by_user_id", "updated_by_user_id", "created_at", "updated_at"},
	"nuki_access_codes":            {"id", "property_id", "named_stay_id", "code_label", "access_code_masked", "generated_pin_plain", "external_nuki_id", "valid_from", "valid_until", "status", "error_message", "last_sync_run_id", "created_at", "updated_at", "revoked_at"},
	"nuki_event_logs":              {"id", "property_id", "nuki_access_code_id", "sync_run_id", "event_type", "message", "payload_json", "created_at"},
	"nuki_guest_daily_entries":     {"id", "property_id", "named_stay_id", "day_date", "first_entry_at", "nuki_event_reference", "created_at"},
	"cleaning_calendar_events":     {"id", "property_id", "upstream_event_uid", "checkout_date", "cleaning_kind", "google_calendar_id", "google_event_id", "cleaning_date", "starts_at", "ends_at", "same_day_arrival", "title", "status", "warning_message", "error_message", "last_synced_at", "created_at", "updated_at", "named_stay_id", "raw_booking_block_id", "cleaning_identity", "desired_hash", "last_google_seen_at"},
	"cleaning_calendar_event_logs": {"id", "property_id", "cleaning_calendar_event_id", "sync_run_id", "action", "message", "created_at"},
	"finance_bookings":             {"id", "property_id", "reference_number", "payout_id", "row_type", "check_in_date", "check_out_date", "guest_name", "reservation_status", "currency", "payment_status", "amount_cents", "commission_cents", "payment_service_fee_cents", "net_cents", "payout_date", "transaction_id", "raw_payout_row_json", "created_at", "updated_at", "booked_on", "original_amount_cents", "commission_pct", "persons", "rooms", "room_nights", "booker_name", "guest_request", "invoice_number", "hotel_id", "property_label", "country", "source_channel", "has_payout_data", "has_statement_data", "raw_statement_row_json", "status", "outcome_override", "outcome_override_marked_at", "named_stay_id"},
	"finance_booking_merges":       {"id", "booking_id", "import_id", "source_type", "changed_fields_json", "occurred_at"},
	"invoices":                     {"id", "property_id", "invoice_number", "sequence_year", "sequence_value", "language", "issue_date", "taxable_supply_date", "due_date", "stay_start_date", "stay_end_date", "supplier_snapshot_json", "customer_snapshot_json", "amount_total_cents", "currency", "payment_status", "payment_note", "version", "created_by", "created_at", "updated_at", "finance_booking_payout_id", "named_stay_id"},
	"invoice_files":                {"id", "invoice_id", "version", "file_path", "file_size_bytes", "created_at"},
}

var preservedSequenceTables = map[string]struct{}{
	"nuki_access_codes": {}, "nuki_event_logs": {}, "nuki_guest_daily_entries": {},
	"cleaning_calendar_events": {}, "cleaning_calendar_event_logs": {},
	"finance_bookings": {}, "finance_booking_merges": {}, "invoices": {}, "invoice_files": {},
	"property_availability_blocks": {}, "named_stay_nights": {},
	"raw_booking_block_nights": {}, "stay_source_links": {},
}

func Audit(ctx context.Context, opts Options) Report {
	r := newReport("audit", opts)
	path, validationID, err := validateOptions(opts)
	if err != nil {
		return finishWithGate(r, validationID, err)
	}
	sidecarChecks, err := sqliteSidecarChecks(path, "gate.sqlite")
	r.Checks = append(r.Checks, sidecarChecks...)
	if err != nil {
		return finish(r, err)
	}
	r.DatabaseBefore, err = fingerprint(path)
	if err != nil {
		return finish(r, fmt.Errorf("fingerprint database: %w", err))
	}

	db, err := dbconn.OpenReadOnly("sqlite://" + path)
	if err != nil {
		return finish(r, fmt.Errorf("open immutable database: %w", err))
	}
	r.SchemaBefore = schemaVersions(ctx, db, &r)
	r.SchemaObjectsBefore = schemaObjects(ctx, db, &r, "before")
	r.SQLiteSequenceBefore = sqliteSequences(ctx, db, &r, "before")
	r.TableCounts.Before = tableCounts(ctx, db, &r)
	r.CriticalAggregates.Before = criticalAggregates(ctx, db, &r, "before")
	r.Checks = append(r.Checks, runReadinessChecks(ctx, db)...)
	var fileCheck Check
	r.InvoiceFileChecksums.Before, fileCheck = invoiceFileChecksums(ctx, db, opts.DataRoot, "files.invoice_content_checksums")
	r.Checks = append(r.Checks, fileCheck)
	if err := db.Close(); err != nil {
		r.Errors = append(r.Errors, "close immutable database: "+err.Error())
	}

	tmp, err := copyToTemporary(path)
	if err != nil {
		return finish(r, fmt.Errorf("copy database: %w", err))
	}
	defer removeSQLiteFiles(tmp)
	recheck, err := sqliteSidecarChecks(path, "gate.audit_recheck_sqlite")
	r.Checks = append(r.Checks, recheck...)
	if err != nil {
		return finish(r, err)
	}
	stableFingerprint, err := fingerprint(path)
	if err != nil {
		return finishWithGate(r, "gate.audit_source_stable", err)
	}
	copyFingerprint, err := fingerprint(tmp)
	if err != nil {
		return finishWithGate(r, "gate.audit_copy_fingerprint", err)
	}
	if stableFingerprint != r.DatabaseBefore {
		return finishWithGate(r, "gate.audit_source_stable", errors.New("database changed while audit snapshot was being captured"))
	}
	r.Checks = append(r.Checks, Check{ID: "gate.audit_source_stable", Passed: true})
	if copyFingerprint != r.DatabaseBefore {
		return finishWithGate(r, "gate.audit_copy_fingerprint", errors.New("temporary copy does not match audited database fingerprint"))
	}
	r.Checks = append(r.Checks, Check{ID: "gate.audit_copy_fingerprint", Passed: true})
	copyDB, err := dbconn.Open("sqlite://" + tmp)
	if err != nil {
		return finish(r, fmt.Errorf("open temporary copy: %w", err))
	}
	migrationErr := migrate.Up(copyDB)
	r.Checks = append(r.Checks, errorCheck("migration.copy_all", migrationErr))
	if migrationErr == nil {
		r.SchemaAfter = schemaVersions(ctx, copyDB, &r)
		r.SchemaObjectsAfter = schemaObjects(ctx, copyDB, &r, "after")
		r.SQLiteSequenceAfter = sqliteSequences(ctx, copyDB, &r, "after")
		r.TableCounts.After = tableCounts(ctx, copyDB, &r)
		r.CriticalAggregates.After = criticalAggregates(ctx, copyDB, &r, "after")
		r.InvoiceFileChecksums.After, fileCheck = invoiceFileChecksums(ctx, copyDB, opts.DataRoot, "post.files.invoice_content_checksums")
		r.Checks = append(r.Checks, fileCheck)
		r.Checks = append(r.Checks, runPostChecks(ctx, copyDB)...)
		r.Checks = append(r.Checks, aggregateCheck(r.CriticalAggregates.Before, r.CriticalAggregates.After))
		r.Checks = append(r.Checks, sequenceCheck(r.SQLiteSequenceBefore, r.SQLiteSequenceAfter))
		r.Checks = append(r.Checks, fileChecksumCheck(r.InvoiceFileChecksums.Before, r.InvoiceFileChecksums.After, "post.invoice_file_checksums_preserved"))
	}
	if err := copyDB.Close(); err != nil {
		r.Errors = append(r.Errors, "close temporary database: "+err.Error())
	}
	return finish(r, nil)
}

// Repair applies only deterministic, evidence-backed readiness repairs. It
// deliberately leaves ambiguous ownership and review decisions untouched.
func Repair(ctx context.Context, opts Options) Report {
	r := newReport("repair", opts)
	path, validationID, err := validateOptions(opts)
	if err != nil {
		return finishWithGate(r, validationID, err)
	}
	if !opts.ConfirmRepair {
		return finishWithGate(r, "gate.confirm_readiness_repair", errors.New("--confirm-readiness-repair is required with --repair"))
	}
	sidecarChecks, err := sqliteSidecarChecks(path, "gate.sqlite")
	r.Checks = append(r.Checks, sidecarChecks...)
	if err != nil {
		return finish(r, err)
	}
	r.DatabaseBefore, err = fingerprint(path)
	if err != nil {
		return finish(r, fmt.Errorf("fingerprint database: %w", err))
	}

	db, err := dbconn.OpenReadOnly("sqlite://" + path)
	if err != nil {
		return finish(r, fmt.Errorf("open immutable database: %w", err))
	}
	r.SchemaBefore = schemaVersions(ctx, db, &r)
	r.SchemaObjectsBefore = schemaObjects(ctx, db, &r, "before")
	r.SQLiteSequenceBefore = sqliteSequences(ctx, db, &r, "before")
	r.TableCounts.Before = tableCounts(ctx, db, &r)
	r.CriticalAggregates.Before = criticalAggregates(ctx, db, &r, "before")
	var schema38, schema39 int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE version = '000038_named_stay_lifecycle_metadata'`).Scan(&schema38); err != nil {
		_ = db.Close()
		return finishWithGate(r, "gate.repair_schema", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE version = '000039_legacy_occupancy_removal'`).Scan(&schema39); err != nil {
		_ = db.Close()
		return finishWithGate(r, "gate.repair_schema", err)
	}
	if err := db.Close(); err != nil {
		return finish(r, fmt.Errorf("close immutable database before repair: %w", err))
	}
	if schema38 != 1 || schema39 != 0 {
		return finishWithGate(r, "gate.repair_schema", fmt.Errorf("repair requires migration 000038 applied and 000039 pending"))
	}

	if opts.beforeWritableOpen != nil {
		opts.beforeWritableOpen()
	}
	recheck, err := sqliteSidecarChecks(path, "gate.pre_repair_sqlite")
	r.Checks = append(r.Checks, recheck...)
	if err != nil {
		return finish(r, err)
	}
	currentFingerprint, err := fingerprint(path)
	if err != nil {
		return finishWithGate(r, "gate.pre_repair_database_fingerprint", err)
	}
	if currentFingerprint != r.DatabaseBefore {
		return finishWithGate(r, "gate.pre_repair_database_fingerprint", errors.New("database changed immediately before writable repair"))
	}
	r.Checks = append(r.Checks, Check{ID: "gate.confirm_readiness_repair", Passed: true}, Check{ID: "gate.pre_repair_database_fingerprint", Passed: true})

	db, err = dbconn.Open("sqlite://" + path)
	if err != nil {
		return finish(r, fmt.Errorf("open database for repair: %w", err))
	}
	closed := false
	defer func() {
		if !closed {
			_ = db.Close()
		}
	}()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return finish(r, fmt.Errorf("begin readiness repair: %w", err))
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()

	if r.RepairCounts.CancellationTimestamps, err = execRepair(ctx, tx, `
		UPDATE named_stays
		SET cancellation_effective_at = (
			SELECT MIN(fb.created_at)
			FROM finance_bookings fb
			WHERE fb.named_stay_id = named_stays.id
			  AND fb.property_id = named_stays.property_id
			  AND upper(trim(COALESCE(fb.status, ''))) = 'CANCELLED'
			  AND julianday(fb.created_at) IS NOT NULL
			  AND NOT EXISTS (
				SELECT 1
				FROM finance_booking_merges m,
				     json_each(CASE WHEN json_valid(m.changed_fields_json) THEN m.changed_fields_json ELSE '[]' END) changed
				WHERE m.booking_id = fb.id AND lower(trim(CAST(changed.value AS TEXT))) = 'status'
			  )
		)
		WHERE status = 'cancelled'
		  AND cancellation_effective_at IS NULL
		  AND EXISTS (
			SELECT 1 FROM finance_bookings fb
			WHERE fb.named_stay_id = named_stays.id
			  AND fb.property_id = named_stays.property_id
			  AND upper(trim(COALESCE(fb.status, ''))) = 'CANCELLED'
			  AND julianday(fb.created_at) IS NOT NULL
			  AND NOT EXISTS (
				SELECT 1
				FROM finance_booking_merges m,
				     json_each(CASE WHEN json_valid(m.changed_fields_json) THEN m.changed_fields_json ELSE '[]' END) changed
				WHERE m.booking_id = fb.id AND lower(trim(CAST(changed.value AS TEXT))) = 'status'
			  )
		)`); err != nil {
		return finish(r, fmt.Errorf("repair cancellation timestamps: %w", err))
	}
	if r.RepairCounts.ExactSourceMappings, err = execRepair(ctx, tx, `
		INSERT INTO occupancy_stay_migration_map (
			old_occupancy_id, property_id, raw_booking_block_id, migration_kind, notes, created_at
		)
		SELECT o.id, o.property_id, r.id, 'raw_block', 'pms21_exact_source_repair',
			strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		FROM occupancies o
		JOIN raw_booking_blocks r
		  ON r.property_id = o.property_id
		 AND r.source_type = o.source_type
		 AND r.source_event_uid = o.source_event_uid
		LEFT JOIN occupancy_stay_migration_map m ON m.old_occupancy_id = o.id
		WHERE m.old_occupancy_id IS NULL`); err != nil {
		return finish(r, fmt.Errorf("repair exact source mappings: %w", err))
	}
	if r.RepairCounts.LegacyCleaningPointers, err = execRepair(ctx, tx, `
		UPDATE cleaning_calendar_events
		SET next_occupancy_id = NULL
		WHERE next_occupancy_id IS NOT NULL
		  AND (
			(named_stay_id IS NOT NULL AND raw_booking_block_id IS NULL AND EXISTS (
				SELECT 1 FROM named_stays s WHERE s.id = cleaning_calendar_events.named_stay_id AND s.property_id = cleaning_calendar_events.property_id
			))
			OR
			(raw_booking_block_id IS NOT NULL AND named_stay_id IS NULL AND EXISTS (
				SELECT 1 FROM raw_booking_blocks b WHERE b.id = cleaning_calendar_events.raw_booking_block_id AND b.property_id = cleaning_calendar_events.property_id
			))
		  )`); err != nil {
		return finish(r, fmt.Errorf("repair legacy cleaning pointers: %w", err))
	}
	if r.RepairCounts.CleaningKinds, err = execRepair(ctx, tx, `
		UPDATE cleaning_calendar_events
		SET cleaning_kind = 'provisional_block'
		WHERE raw_booking_block_id IS NOT NULL
		  AND named_stay_id IS NULL
		  AND cleaning_kind <> 'provisional_block'
		  AND EXISTS (
			SELECT 1 FROM raw_booking_blocks b WHERE b.id = cleaning_calendar_events.raw_booking_block_id AND b.property_id = cleaning_calendar_events.property_id
		  )`); err != nil {
		return finish(r, fmt.Errorf("repair cleaning kinds: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return finish(r, fmt.Errorf("commit readiness repair: %w", err))
	}
	rollback = false
	r.Checks = append(r.Checks, Check{ID: "repair.deterministic", Passed: true})

	r.SchemaAfter = schemaVersions(ctx, db, &r)
	r.SchemaObjectsAfter = schemaObjects(ctx, db, &r, "after")
	r.SQLiteSequenceAfter = sqliteSequences(ctx, db, &r, "after")
	r.TableCounts.After = tableCounts(ctx, db, &r)
	r.CriticalAggregates.After = criticalAggregates(ctx, db, &r, "after")
	r.RemainingReadinessChecks = runReadinessChecks(ctx, db)
	if _, err := db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return finish(r, fmt.Errorf("checkpoint repaired database: %w", err))
	}
	if err := db.Close(); err != nil {
		return finish(r, fmt.Errorf("close repaired database: %w", err))
	}
	closed = true
	postSidecars, sidecarErr := sqliteSidecarChecks(path, "post.sqlite")
	r.Checks = append(r.Checks, postSidecars...)
	if sidecarErr != nil {
		return finish(r, sidecarErr)
	}
	r.DatabaseAfter, err = fingerprint(path)
	if err != nil {
		return finish(r, fmt.Errorf("fingerprint repaired database: %w", err))
	}
	return finish(r, nil)
}

func execRepair(ctx context.Context, tx *sql.Tx, query string) (int64, error) {
	result, err := tx.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func Apply(ctx context.Context, opts Options) Report {
	r := newReport("apply", opts)
	path, validationID, err := validateOptions(opts)
	if err != nil {
		return finishWithGate(r, validationID, err)
	}
	if !opts.Confirm {
		return finishWithGate(r, "gate.confirm_destructive_cleanup", errors.New("--confirm-destructive-cleanup is required with --apply"))
	}
	if opts.PreReport == "" {
		return finishWithGate(r, "gate.pre_report", errors.New("--pre-report is required with --apply"))
	}
	if !filepath.IsAbs(opts.PreReport) {
		return finishWithGate(r, "gate.pre_report", errors.New("--pre-report must be an absolute path"))
	}
	sidecarChecks, err := sqliteSidecarChecks(path, "gate.sqlite")
	r.Checks = append(r.Checks, sidecarChecks...)
	if err != nil {
		return finish(r, err)
	}
	pre, err := ReadReport(opts.PreReport)
	if err != nil {
		return finishWithGate(r, "gate.pre_report", err)
	}
	if err := VerifyChecksum(pre); err != nil {
		return finishWithGate(r, "gate.pre_report_checksum", err)
	}
	if pre.Format != reportFormat || pre.Mode != "audit" || !pre.Passed {
		return finishWithGate(r, "gate.pre_report_passed", errors.New("pre-report is not a passing PMS 21 cleanup audit"))
	}
	if pre.ImageDigest != opts.ImageDigest || pre.Commit != opts.Commit || pre.FrontendBuild != opts.FrontendBuild || pre.MigrationToolSHA256 != r.MigrationToolSHA256 {
		return finishWithGate(r, "gate.release_identity", errors.New("pre-report release identity does not match apply flags"))
	}
	if pre.Operator != opts.Operator || pre.ExceptionRegisterReference != opts.ExceptionRegisterReference || pre.ApprovedExceptionsReference != opts.ApprovedExceptionsReference || pre.ExternalEvidence != r.ExternalEvidence {
		return finishWithGate(r, "gate.evidence_identity", errors.New("pre-report operator or approved evidence references do not match apply flags"))
	}
	if pre.CommandMode != "audit" || pre.EffectiveFlags != "none" || !equalStrings(pre.CommandArguments, commandArguments("audit", opts)) {
		return finishWithGate(r, "gate.command_identity", errors.New("pre-report command arguments or effective flags do not match apply flags"))
	}
	r.DatabaseBefore, err = fingerprint(path)
	if err != nil {
		return finishWithGate(r, "gate.database_fingerprint", err)
	}
	if r.DatabaseBefore != pre.DatabaseBefore {
		return finishWithGate(r, "gate.database_fingerprint", errors.New("database fingerprint does not match pre-report"))
	}
	r.Checks = append(r.Checks, Check{ID: "gate.pre_report", Passed: true}, Check{ID: "gate.pre_report_checksum", Passed: true}, Check{ID: "gate.release_identity", Passed: true}, Check{ID: "gate.command_identity", Passed: true}, Check{ID: "gate.evidence_identity", Passed: true}, Check{ID: "gate.database_fingerprint", Passed: true})

	db, err := dbconn.OpenReadOnly("sqlite://" + path)
	if err != nil {
		return finish(r, fmt.Errorf("open immutable database for apply checks: %w", err))
	}
	r.SchemaBefore = schemaVersions(ctx, db, &r)
	r.SchemaObjectsBefore = schemaObjects(ctx, db, &r, "before")
	r.SQLiteSequenceBefore = sqliteSequences(ctx, db, &r, "before")
	r.TableCounts.Before = tableCounts(ctx, db, &r)
	r.CriticalAggregates.Before = criticalAggregates(ctx, db, &r, "before")
	preChecks := runReadinessChecks(ctx, db)
	r.Checks = append(r.Checks, preChecks...)
	var fileCheck Check
	r.InvoiceFileChecksums.Before, fileCheck = invoiceFileChecksums(ctx, db, opts.DataRoot, "files.invoice_content_checksums")
	r.Checks = append(r.Checks, fileCheck)
	if err := db.Close(); err != nil {
		return finish(r, fmt.Errorf("close immutable database before apply: %w", err))
	}
	if !checksPassed(preChecks) {
		return finish(r, errors.New("current database failed readiness checks"))
	}
	if !fileCheck.Passed {
		return finish(r, errors.New("current invoice files failed checksum checks"))
	}
	if check := fileChecksumCheck(pre.InvoiceFileChecksums.Before, r.InvoiceFileChecksums.Before, "gate.invoice_file_checksums"); !check.Passed {
		r.Checks = append(r.Checks, check)
		return finish(r, errors.New("invoice file checksums do not match pre-report"))
	} else {
		r.Checks = append(r.Checks, check)
	}

	if opts.beforeWritableOpen != nil {
		opts.beforeWritableOpen()
	}
	recheck, err := sqliteSidecarChecks(path, "gate.pre_apply_sqlite")
	r.Checks = append(r.Checks, recheck...)
	if err != nil {
		return finish(r, err)
	}
	currentFingerprint, err := fingerprint(path)
	if err != nil {
		return finishWithGate(r, "gate.pre_apply_database_fingerprint", err)
	}
	if currentFingerprint != pre.DatabaseBefore || currentFingerprint != r.DatabaseBefore {
		return finishWithGate(r, "gate.pre_apply_database_fingerprint", errors.New("database changed immediately before writable apply"))
	}
	r.Checks = append(r.Checks, Check{ID: "gate.pre_apply_database_fingerprint", Passed: true})

	db, err = dbconn.Open("sqlite://" + path)
	if err != nil {
		return finish(r, fmt.Errorf("open database for apply: %w", err))
	}
	closed := false
	defer func() {
		if !closed {
			_ = db.Close()
		}
	}()

	err = migrate.Up(db)
	r.Checks = append(r.Checks, errorCheck("migration.apply_all", err))
	if err != nil {
		return finish(r, fmt.Errorf("apply migrations: %w", err))
	}
	r.SchemaAfter = schemaVersions(ctx, db, &r)
	r.SchemaObjectsAfter = schemaObjects(ctx, db, &r, "after")
	r.SQLiteSequenceAfter = sqliteSequences(ctx, db, &r, "after")
	r.TableCounts.After = tableCounts(ctx, db, &r)
	r.CriticalAggregates.After = criticalAggregates(ctx, db, &r, "after")
	r.InvoiceFileChecksums.After, fileCheck = invoiceFileChecksums(ctx, db, opts.DataRoot, "post.files.invoice_content_checksums")
	r.Checks = append(r.Checks, fileCheck)
	r.Checks = append(r.Checks, runPostChecks(ctx, db)...)
	r.Checks = append(r.Checks, aggregateCheck(r.CriticalAggregates.Before, r.CriticalAggregates.After))
	r.Checks = append(r.Checks, sequenceCheck(r.SQLiteSequenceBefore, r.SQLiteSequenceAfter))
	r.Checks = append(r.Checks, fileChecksumCheck(r.InvoiceFileChecksums.Before, r.InvoiceFileChecksums.After, "post.invoice_file_checksums_preserved"))
	if _, err := db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		r.Checks = append(r.Checks, errorCheck("database.checkpoint", err))
	} else {
		r.Checks = append(r.Checks, Check{ID: "database.checkpoint", Passed: true})
	}
	if err := db.Close(); err != nil {
		return finish(r, fmt.Errorf("close cleaned database: %w", err))
	}
	closed = true
	postSidecars, sidecarErr := sqliteSidecarChecks(path, "post.sqlite")
	r.Checks = append(r.Checks, postSidecars...)
	if sidecarErr != nil {
		return finish(r, sidecarErr)
	}
	r.DatabaseAfter, err = fingerprint(path)
	if err != nil {
		r.Errors = append(r.Errors, "fingerprint cleaned database: "+err.Error())
	}
	return finish(r, nil)
}

func ReadReport(path string) (Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return Report{}, fmt.Errorf("read pre-report: %w", err)
	}
	defer f.Close()
	var r Report
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return Report{}, fmt.Errorf("decode pre-report: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return Report{}, errors.New("decode pre-report: trailing JSON data")
	}
	return r, nil
}

func SetChecksum(r *Report) error {
	r.ReportChecksum = ""
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	r.ReportChecksum = hex.EncodeToString(sum[:])
	return nil
}

func VerifyChecksum(r Report) error {
	want := r.ReportChecksum
	if want == "" {
		return errors.New("pre-report has no report checksum")
	}
	if err := SetChecksum(&r); err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(want), []byte(r.ReportChecksum)) != 1 {
		return errors.New("pre-report checksum is invalid")
	}
	return nil
}

func newReport(mode string, opts Options) Report {
	r := Report{
		Format: reportFormat, Mode: mode, StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
		ImageDigest: opts.ImageDigest, Commit: opts.Commit, FrontendBuild: opts.FrontendBuild, Operator: opts.Operator,
		CommandMode: mode, CommandArguments: commandArguments(mode, opts), EffectiveFlags: "none",
		ExceptionRegisterReference: opts.ExceptionRegisterReference, ApprovedExceptionsReference: opts.ApprovedExceptionsReference,
		ExternalEvidence: ExternalEvidence{AnalyticsParity: opts.AnalyticsParityReference, RemoteVerification: opts.RemoteVerificationReference, CallerInventory: opts.CallerInventoryReference},
		Checks:           []Check{}, TableCounts: Counts{Before: map[string]int64{}, After: map[string]int64{}},
		CriticalAggregates:  Aggregates{Before: map[string]string{}, After: map[string]string{}},
		SchemaObjectsBefore: []SchemaObject{}, SchemaObjectsAfter: []SchemaObject{},
		SQLiteSequenceBefore: map[string]int64{}, SQLiteSequenceAfter: map[string]int64{},
		InvoiceFileChecksums: FileChecksums{Before: []FileChecksum{}, After: []FileChecksum{}},
	}
	checksum, err := migrationToolChecksum()
	if err != nil {
		r.Errors = append(r.Errors, "checksum migration tool: "+err.Error())
	} else {
		r.MigrationToolSHA256 = checksum
	}
	return r
}

func finish(r Report, err error) Report {
	if err != nil {
		r.Errors = append(r.Errors, err.Error())
	}
	r.Passed = err == nil && len(r.Errors) == 0 && checksPassed(r.Checks)
	r.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if checksumErr := SetChecksum(&r); checksumErr != nil {
		r.Passed = false
		r.Errors = append(r.Errors, "report checksum: "+checksumErr.Error())
	}
	return r
}

func finishWithGate(r Report, id string, err error) Report {
	r.Checks = append(r.Checks, Check{ID: id, Passed: false, Count: 1, Error: err.Error()})
	return finish(r, err)
}

func validateOptions(opts Options) (string, string, error) {
	if opts.DBPath == "" || !filepath.IsAbs(opts.DBPath) || strings.HasPrefix(opts.DBPath, "sqlite:") {
		return "", "gate.database_path", errors.New("--db requires an explicit absolute filesystem path")
	}
	if !immutableDigestPattern.MatchString(strings.TrimSpace(opts.ImageDigest)) {
		return "", "gate.image_digest_immutable", errors.New("--image-digest must be sha256:<64 hex> or an image reference containing @sha256:<64 hex>; tags are not accepted")
	}
	if strings.TrimSpace(opts.Commit) == "" || strings.TrimSpace(opts.FrontendBuild) == "" {
		return "", "gate.release_identity", errors.New("--commit and --frontend-build are required")
	}
	if strings.TrimSpace(opts.Operator) == "" {
		return "", "gate.operator", errors.New("--operator is required")
	}
	if strings.TrimSpace(opts.ExceptionRegisterReference) == "" {
		return "", "gate.exception_references", errors.New("--exception-register-reference is required")
	}
	if strings.TrimSpace(opts.ApprovedExceptionsReference) == "" {
		return "", "gate.exception_references", errors.New("--approved-exceptions-reference is required")
	}
	if strings.TrimSpace(opts.AnalyticsParityReference) == "" || strings.TrimSpace(opts.RemoteVerificationReference) == "" || strings.TrimSpace(opts.CallerInventoryReference) == "" {
		return "", "gate.external_evidence_references", errors.New("--analytics-parity-reference, --remote-verification-reference, and --caller-inventory-reference are required")
	}
	if opts.DataRoot != "" && !filepath.IsAbs(opts.DataRoot) {
		return "", "gate.data_root", errors.New("--data-root must be an absolute path when provided")
	}
	return filepath.Clean(opts.DBPath), "", nil
}

func fingerprint(path string) (Fingerprint, error) {
	if err := requireSidecarsAbsent(path); err != nil {
		return Fingerprint{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return Fingerprint{}, err
	}
	if !info.Mode().IsRegular() {
		return Fingerprint{}, errors.New("database path must be a regular file, not a symlink or special file")
	}
	f, err := os.Open(path)
	if err != nil {
		return Fingerprint{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return Fingerprint{}, err
	}
	if err := requireSidecarsAbsent(path); err != nil {
		return Fingerprint{}, err
	}
	return Fingerprint{SHA256: hex.EncodeToString(h.Sum(nil)), Size: n, WALAbsent: true, SHMAbsent: true}, nil
}

func sqliteSidecarChecks(path, idPrefix string) ([]Check, error) {
	checks := make([]Check, 0, 2)
	var failures []string
	for _, sidecar := range []struct {
		suffix string
		name   string
	}{
		{suffix: "-wal", name: "wal_absent"},
		{suffix: "-shm", name: "shm_absent"},
	} {
		_, err := os.Lstat(path + sidecar.suffix)
		check := Check{ID: idPrefix + "_" + sidecar.name, Passed: os.IsNotExist(err)}
		if err == nil {
			check.Count = 1
			check.Error = "SQLite sidecar exists; stop the API and remove it only through a verified checkpoint/clean shutdown"
			failures = append(failures, sidecar.suffix+" exists")
		} else if !os.IsNotExist(err) {
			check.Count = 1
			check.Error = "unable to verify SQLite sidecar absence"
			failures = append(failures, sidecar.suffix+" could not be checked")
		}
		checks = append(checks, check)
	}
	if len(failures) > 0 {
		return checks, errors.New("quiesced SQLite sidecar gate failed: " + strings.Join(failures, ", "))
	}
	return checks, nil
}

func requireSidecarsAbsent(path string) error {
	_, err := sqliteSidecarChecks(path, "unused")
	return err
}

func commandArguments(mode string, opts Options) []string {
	args := []string{"--" + mode, "--db", opts.DBPath, "--image-digest", opts.ImageDigest, "--commit", opts.Commit, "--frontend-build", opts.FrontendBuild, "--operator", opts.Operator, "--exception-register-reference", opts.ExceptionRegisterReference, "--approved-exceptions-reference", opts.ApprovedExceptionsReference, "--analytics-parity-reference", opts.AnalyticsParityReference, "--remote-verification-reference", opts.RemoteVerificationReference, "--caller-inventory-reference", opts.CallerInventoryReference}
	if opts.DataRoot != "" {
		args = append(args, "--data-root", opts.DataRoot)
	}
	if mode == "apply" {
		if opts.Confirm {
			args = append(args, "--confirm-destructive-cleanup")
		}
		if opts.PreReport != "" {
			args = append(args, "--pre-report", opts.PreReport)
		}
	} else if mode == "repair" && opts.ConfirmRepair {
		args = append(args, "--confirm-readiness-repair")
	}
	return args
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

var (
	toolChecksumOnce sync.Once
	toolChecksum     string
	toolChecksumErr  error
)

func migrationToolChecksum() (string, error) {
	toolChecksumOnce.Do(func() {
		path, err := os.Executable()
		if err != nil {
			toolChecksumErr = err
			return
		}
		f, err := os.Open(path)
		if err != nil {
			toolChecksumErr = err
			return
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			toolChecksumErr = err
			return
		}
		toolChecksum = hex.EncodeToString(h.Sum(nil))
	})
	return toolChecksum, toolChecksumErr
}

func copyToTemporary(source string) (string, error) {
	f, err := os.CreateTemp("", "pms21-cleanup-*.db")
	if err != nil {
		return "", err
	}
	path := f.Name()
	defer f.Close()
	src, err := os.Open(source)
	if err != nil {
		os.Remove(path)
		return "", err
	}
	defer src.Close()
	if _, err := io.Copy(f, src); err != nil {
		os.Remove(path)
		return "", err
	}
	if err := f.Sync(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

func removeSQLiteFiles(path string) {
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		_ = os.Remove(path + suffix)
	}
}

func schemaVersions(ctx context.Context, db *sql.DB, r *Report) []string {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		r.Errors = append(r.Errors, "read schema versions: "+err.Error())
		return []string{}
	}
	defer rows.Close()
	versions := []string{}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			r.Errors = append(r.Errors, "scan schema versions: "+err.Error())
			return versions
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		r.Errors = append(r.Errors, "read schema versions: "+err.Error())
	}
	return versions
}

func schemaObjects(ctx context.Context, db *sql.DB, r *Report, phase string) []SchemaObject {
	rows, err := db.QueryContext(ctx, `SELECT type, name, tbl_name, coalesce(sql, '') FROM sqlite_schema WHERE type IN ('table', 'index', 'trigger') AND name NOT LIKE 'sqlite_%' ORDER BY type, name`)
	if err != nil {
		r.Errors = append(r.Errors, phase+" schema/index/constraint evidence: "+err.Error())
		return []SchemaObject{}
	}
	defer rows.Close()
	objects := []SchemaObject{}
	for rows.Next() {
		var object SchemaObject
		var definition string
		if err := rows.Scan(&object.Type, &object.Name, &object.Table, &definition); err != nil {
			r.Errors = append(r.Errors, phase+" schema/index/constraint evidence: "+err.Error())
			return objects
		}
		sum := sha256.Sum256([]byte(definition))
		object.DefinitionSHA256 = hex.EncodeToString(sum[:])
		objects = append(objects, object)
	}
	if err := rows.Err(); err != nil {
		r.Errors = append(r.Errors, phase+" schema/index/constraint evidence: "+err.Error())
	}
	return objects
}

func sqliteSequences(ctx context.Context, db *sql.DB, r *Report, phase string) map[string]int64 {
	rows, err := db.QueryContext(ctx, `SELECT name, seq FROM sqlite_sequence ORDER BY name`)
	if err != nil {
		r.Errors = append(r.Errors, phase+" sqlite_sequence baseline: "+err.Error())
		return map[string]int64{}
	}
	defer rows.Close()
	sequences := map[string]int64{}
	for rows.Next() {
		var name string
		var sequence int64
		if err := rows.Scan(&name, &sequence); err != nil {
			r.Errors = append(r.Errors, phase+" sqlite_sequence baseline: "+err.Error())
			return sequences
		}
		sequences[name] = sequence
	}
	if err := rows.Err(); err != nil {
		r.Errors = append(r.Errors, phase+" sqlite_sequence baseline: "+err.Error())
	}
	return sequences
}

func invoiceFileChecksums(ctx context.Context, db *sql.DB, dataRoot, checkID string) ([]FileChecksum, Check) {
	rows, err := db.QueryContext(ctx, `SELECT id, file_path, file_size_bytes FROM invoice_files ORDER BY id`)
	if err != nil {
		return []FileChecksum{}, errorCheck(checkID, err)
	}
	defer rows.Close()
	checksums := []FileChecksum{}
	var failures int64
	for rows.Next() {
		var id, expectedSize int64
		var storedPath string
		if err := rows.Scan(&id, &storedPath, &expectedSize); err != nil {
			return checksums, errorCheck(checkID, err)
		}
		evidence := FileChecksum{InvoiceFileID: id}
		fullPath, failure := resolveDataPath(dataRoot, storedPath)
		if failure != "" {
			evidence.Failure = failure
			failures++
			checksums = append(checksums, evidence)
			continue
		}
		f, err := os.Open(fullPath)
		if err != nil {
			evidence.Failure = "missing_or_unreadable"
			failures++
			checksums = append(checksums, evidence)
			continue
		}
		h := sha256.New()
		size, readErr := io.Copy(h, f)
		closeErr := f.Close()
		if readErr != nil || closeErr != nil {
			evidence.Failure = "read_failed"
			failures++
			checksums = append(checksums, evidence)
			continue
		}
		evidence.Accessible = true
		evidence.Size = size
		evidence.SHA256 = hex.EncodeToString(h.Sum(nil))
		if size != expectedSize {
			evidence.Accessible = false
			evidence.Failure = "size_mismatch"
			failures++
		}
		checksums = append(checksums, evidence)
	}
	if err := rows.Err(); err != nil {
		return checksums, errorCheck(checkID, err)
	}
	check := Check{ID: checkID, Passed: failures == 0, Count: failures}
	if failures > 0 {
		check.Error = strconv.FormatInt(failures, 10) + " invoice files were missing, unreadable, outside --data-root, or had a size mismatch"
	}
	return checksums, check
}

func resolveDataPath(dataRoot, storedPath string) (string, string) {
	if strings.TrimSpace(dataRoot) == "" {
		return "", "data_root_required"
	}
	root := filepath.Clean(dataRoot)
	relative := filepath.Clean(filepath.FromSlash(storedPath))
	relative = strings.TrimLeft(relative, string(os.PathSeparator))
	if relative == "." || relative == "" {
		return "", "invalid_path"
	}
	fullPath := filepath.Join(root, relative)
	rel, err := filepath.Rel(root, fullPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", "outside_data_root"
	}
	return fullPath, ""
}

func tableCounts(ctx context.Context, db *sql.DB, r *Report) map[string]int64 {
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		r.Errors = append(r.Errors, "list tables: "+err.Error())
		return map[string]int64{}
	}
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			rows.Close()
			r.Errors = append(r.Errors, "scan table: "+err.Error())
			return map[string]int64{}
		}
		tables = append(tables, table)
	}
	if err := rows.Close(); err != nil {
		r.Errors = append(r.Errors, "close table list: "+err.Error())
	}
	counts := make(map[string]int64, len(tables))
	for _, table := range tables {
		var count int64
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM `+quoteIdentifier(table)).Scan(&count); err != nil {
			r.Errors = append(r.Errors, "count table "+table+": "+err.Error())
			continue
		}
		counts[table] = count
	}
	return counts
}

func criticalAggregates(ctx context.Context, db *sql.DB, r *Report, phase string) map[string]string {
	tables := make([]string, 0, len(criticalTables))
	for table := range criticalTables {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	result := make(map[string]string, len(tables))
	for _, table := range tables {
		digest, err := aggregateTable(ctx, db, table, criticalTables[table])
		if err != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("%s aggregate %s: %v", phase, table, err))
			continue
		}
		result[table] = digest
	}
	return result
}

func aggregateTable(ctx context.Context, db *sql.DB, table string, columns []string) (string, error) {
	quoted := make([]string, len(columns))
	for i, column := range columns {
		quoted[i] = quoteIdentifier(column)
	}
	rows, err := db.QueryContext(ctx, `SELECT `+strings.Join(quoted, ",")+` FROM `+quoteIdentifier(table)+` ORDER BY id`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	h := sha256.New()
	for _, column := range columns {
		writeHashValue(h, column)
	}
	for rows.Next() {
		values := make([]any, len(columns))
		ptrs := make([]any, len(columns))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return "", err
		}
		for _, value := range values {
			switch v := value.(type) {
			case nil:
				writeHashValue(h, "null")
			case []byte:
				writeHashValue(h, "bytes:"+hex.EncodeToString(v))
			default:
				writeHashValue(h, fmt.Sprintf("%T:%v", v, v))
			}
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeHashValue(w io.Writer, value string) {
	_ = binary.Write(w, binary.BigEndian, uint64(len(value)))
	_, _ = io.WriteString(w, value)
}

func runReadinessChecks(ctx context.Context, db *sql.DB) []Check {
	checks := make([]Check, 0, len(readinessChecks))
	for _, definition := range readinessChecks {
		var count int64
		err := db.QueryRowContext(ctx, definition.query).Scan(&count)
		check := Check{ID: definition.id, Count: count, Passed: err == nil && count == 0}
		if err != nil {
			check.Count = 1
			check.Error = err.Error()
		}
		checks = append(checks, check)
	}
	return checks
}

func runPostChecks(ctx context.Context, db *sql.DB) []Check {
	definitions := []readinessCheck{
		{"post.schema.000039_applied", `SELECT CASE WHEN EXISTS (SELECT 1 FROM schema_migrations WHERE version = '000039_legacy_occupancy_removal') THEN 0 ELSE 1 END`},
		{"post.database.foreign_key_check", `SELECT count(*) FROM pragma_foreign_key_check`},
		{"post.database.integrity_check", `SELECT count(*) FROM pragma_integrity_check WHERE integrity_check <> 'ok'`},
		{"post.schema.legacy_tables_absent", `SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name IN ('occupancies', 'occupancy_nights', 'occupancy_api_tokens', 'occupancy_stay_migration_map')`},
		{"post.schema.legacy_columns_absent", `SELECT count(*) FROM sqlite_schema s, pragma_table_info(s.name) c WHERE s.type = 'table' AND c.name IN ('occupancy_id', 'next_occupancy_id', 'source_occupancy_id', 'old_occupancy_id')`},
		{"post.schema.legacy_references_absent", `SELECT count(*) FROM sqlite_schema WHERE lower(coalesce(sql, '')) LIKE '%references occupancies%'`},
		{"post.schema.transitional_tables_absent", `SELECT count(*) FROM sqlite_schema WHERE name LIKE '%_v2' OR name LIKE '%_v3' OR name LIKE 'pms21_%'`},
	}
	checks := make([]Check, 0, len(definitions))
	for _, definition := range definitions {
		var count int64
		err := db.QueryRowContext(ctx, definition.query).Scan(&count)
		check := Check{ID: definition.id, Count: count, Passed: err == nil && count == 0}
		if err != nil {
			check.Count = 1
			check.Error = err.Error()
		}
		checks = append(checks, check)
	}
	return checks
}

func aggregateCheck(before, after map[string]string) Check {
	passed := len(before) == len(after)
	var differences int64
	for table, digest := range before {
		if after[table] != digest {
			passed = false
			differences++
		}
	}
	if len(after) > len(before) {
		differences += int64(len(after) - len(before))
	}
	return Check{ID: "post.critical_aggregates_preserved", Passed: passed, Count: differences}
}

func sequenceCheck(before, after map[string]int64) Check {
	var differences int64
	for table := range preservedSequenceTables {
		beforeSequence, hadBefore := before[table]
		afterSequence, hadAfter := after[table]
		if hadBefore && (!hadAfter || afterSequence != beforeSequence) {
			differences++
		}
		if !hadBefore && hadAfter && afterSequence != 0 {
			differences++
		}
	}
	return Check{ID: "post.sqlite_sequences_preserved", Passed: differences == 0, Count: differences}
}

func fileChecksumCheck(before, after []FileChecksum, id string) Check {
	var differences int64
	if len(before) != len(after) {
		differences = int64(abs(len(before) - len(after)))
	}
	limit := min(len(before), len(after))
	for i := 0; i < limit; i++ {
		if before[i] != after[i] {
			differences++
		}
	}
	return Check{ID: id, Passed: differences == 0, Count: differences}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func errorCheck(id string, err error) Check {
	if err == nil {
		return Check{ID: id, Passed: true}
	}
	return Check{ID: id, Passed: false, Count: 1, Error: err.Error()}
}

func checksPassed(checks []Check) bool {
	for _, check := range checks {
		if !check.Passed {
			return false
		}
	}
	return true
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
