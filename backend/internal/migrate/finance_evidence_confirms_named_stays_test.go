package migrate

import (
	"testing"

	"pms/backend/internal/dbconn"
)

func TestFinanceEvidenceConfirmsNamedStaysMigration(t *testing.T) {
	db, err := dbconn.Open("sqlite://" + t.TempDir() + "/migration.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	applyMigrationsThrough(t, db, "000036_nuki_named_stay_primary.up.sql")

	now := "2026-01-01T00:00:00Z"
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash, role, created_at, updated_at) VALUES (1, 'finance-evidence@test.local', 'hash', 'owner', ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO properties (id, name, timezone, owner_user_id, created_at, updated_at) VALUES (1, 'Migration', 'UTC', 1, ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		id     int
		name   string
		source string
	}{
		{id: 10, name: "Finance confirmed", source: "booking_com"},
		{id: 11, name: "No finance evidence", source: ""},
		{id: 12, name: "Unverified source", source: "external"},
	} {
		if _, err := db.Exec(`INSERT INTO named_stays (id, property_id, display_name, stay_type, check_in_date, check_out_date, status, cleaning_required, review_status, review_reason, nuki_generation_status, created_at, updated_at) VALUES (?, 1, ?, 'booking_com', '2099-02-01', '2099-02-02', 'active', 1, 'needs_review', 'legacy_non_reservation_stay', 'not_applicable', ?, ?)`, row.id, row.name, now, now); err != nil {
			t.Fatal(err)
		}
		if row.source != "" {
			if _, err := db.Exec(`INSERT INTO finance_bookings (property_id, reference_number, check_in_date, check_out_date, guest_name, net_cents, payout_date, named_stay_id, created_at, updated_at, source_channel, has_payout_data, has_statement_data) VALUES (1, ?, '2099-02-01', '2099-02-02', ?, 10000, '2099-02-03', ?, ?, ?, ?, 1, 1)`, row.name, row.name, row.id, now, now, row.source); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := db.Exec(`INSERT INTO named_stays (id, property_id, display_name, stay_type, check_in_date, check_out_date, status, cleaning_required, review_status, review_reason, nuki_generation_status, created_at, updated_at) VALUES (13, 1, 'Cancellation review', 'booking_com', '2099-03-01', '2099-03-02', 'active', 1, 'needs_review', 'finance_status_cancelled', 'not_applicable', ?, ?), (14, 1, 'Cancelled finance row', 'booking_com', '2099-04-01', '2099-04-02', 'active', 1, 'needs_review', 'legacy_non_reservation_stay', 'not_applicable', ?, ?)`, now, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO finance_bookings (property_id, reference_number, check_in_date, check_out_date, guest_name, net_cents, payout_date, named_stay_id, created_at, updated_at, source_channel, has_payout_data, status) VALUES (1, 'CANCEL-REVIEW', '2099-03-01', '2099-03-02', 'Cancellation review', 10000, '2099-03-03', 13, ?, ?, 'booking_com', 1, 'OK'), (1, 'CANCELLED-ROW', '2099-04-01', '2099-04-02', 'Cancelled finance row', 10000, '2099-04-03', 14, ?, ?, 'booking_com', 1, 'CANCELLED')`, now, now, now, now); err != nil {
		t.Fatal(err)
	}

	body, err := embeddedMigrations.ReadFile("000037_finance_evidence_confirms_named_stays.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(body)); err != nil {
		t.Fatal(err)
	}

	var reviewStatus, reviewReason, nukiStatus string
	if err := db.QueryRow(`SELECT review_status, COALESCE(review_reason, ''), nuki_generation_status FROM named_stays WHERE id = 10`).Scan(&reviewStatus, &reviewReason, &nukiStatus); err != nil {
		t.Fatal(err)
	}
	if reviewStatus != "confirmed" || reviewReason != "" || nukiStatus != "pending" {
		t.Fatalf("finance-backed stay status=%q reason=%q nuki=%q", reviewStatus, reviewReason, nukiStatus)
	}
	for _, id := range []int{11, 12, 13, 14} {
		if err := db.QueryRow(`SELECT review_status, review_reason, nuki_generation_status FROM named_stays WHERE id = ?`, id).Scan(&reviewStatus, &reviewReason, &nukiStatus); err != nil {
			t.Fatal(err)
		}
		wantReason := "legacy_non_reservation_stay"
		if id == 13 {
			wantReason = "finance_status_cancelled"
		}
		if reviewStatus != "needs_review" || reviewReason != wantReason || nukiStatus != "not_applicable" {
			t.Fatalf("unverified stay %d changed: status=%q reason=%q nuki=%q", id, reviewStatus, reviewReason, nukiStatus)
		}
	}

	if _, err := db.Exec(string(body)); err != nil {
		t.Fatalf("migration is not idempotent: %v", err)
	}
}
