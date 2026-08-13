package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"pms/backend/internal/finance/statements"
	"pms/backend/internal/testutil"
)

func setupFinanceProperty(t *testing.T, st *Store) int64 {
	t.Helper()
	ctx := context.Background()
	hash := testutil.FastPasswordHash(t, "secret123")
	u, err := st.CreateUser(ctx, "owner@finance.test", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProperty(ctx, u.ID, "FinanceTest", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func categoryIDByCode(t *testing.T, st *Store, pid int64, code string) int64 {
	t.Helper()
	rows, err := st.ListFinanceCategories(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range rows {
		if c.Code == code {
			return c.ID
		}
	}
	t.Fatalf("category %q not found", code)
	return 0
}

func createFinanceNamedStay(t *testing.T, st *Store, propertyID int64, reference, checkIn, checkOut string) *NamedStay {
	t.Helper()
	stay, err := st.CreateNamedStayRecord(context.Background(), NamedStayCreateInput{
		PropertyID:      propertyID,
		DisplayName:     reference + " Guest",
		StayType:        StayTypeBookingCom,
		CheckInDate:     checkIn,
		CheckOutDate:    checkOut,
		SourceChannel:   "booking_com",
		SourceReference: reference,
	})
	if err != nil {
		t.Fatal(err)
	}
	return stay
}

func TestLinkBookingToNamedStayConfirmsOnlyMigrationReviewWithEligibleNukiState(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	now := time.Now().UTC().Format(time.RFC3339)

	createStay := func(name, checkIn, checkOut, reason string) *NamedStay {
		t.Helper()
		stay, err := st.CreateNamedStayRecord(ctx, NamedStayCreateInput{
			PropertyID: pid, DisplayName: name, StayType: StayTypeBookingCom,
			CheckInDate: checkIn, CheckOutDate: checkOut,
			ReviewStatus: "needs_review", ReviewReason: reason,
		})
		if err != nil {
			t.Fatal(err)
		}
		return stay
	}
	insertBooking := func(reference, status string, stayID int64) int64 {
		t.Helper()
		res, err := st.DB.Exec(`INSERT INTO finance_bookings (property_id, named_stay_id, reference_number, check_in_date, check_out_date, guest_name, net_cents, payout_date, created_at, updated_at, source_channel, has_payout_data, has_statement_data, status) VALUES (?, ?, ?, '2099-01-01', '2099-01-02', ?, 10000, '2099-01-03', ?, ?, 'booking_com', 1, 1, ?)`, pid, stayID, reference, reference, now, now, status)
		if err != nil {
			t.Fatal(err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	assertState := func(stayID int64, wantReview, wantReason, wantNuki string) {
		t.Helper()
		var review, reason, nuki string
		var resolution sql.NullString
		if err := st.DB.QueryRow(`SELECT review_status, review_resolution, COALESCE(review_reason, ''), nuki_generation_status FROM named_stays WHERE id = ?`, stayID).Scan(&review, &resolution, &reason, &nuki); err != nil {
			t.Fatal(err)
		}
		if review != wantReview || reason != wantReason || nuki != wantNuki {
			t.Fatalf("stay %d state=%q/%q/%q want %q/%q/%q", stayID, review, reason, nuki, wantReview, wantReason, wantNuki)
		}
		if (wantReview == "confirmed" && resolution.String != "confirmed") || (wantReview == "needs_review" && resolution.Valid) {
			t.Fatalf("stay %d resolution=%v for review_status=%q", stayID, resolution, wantReview)
		}
	}

	confirmed := createStay("Confirmed", "2099-01-01", "2099-01-02", "legacy_non_reservation_stay")
	if err := st.LinkBookingToNamedStay(ctx, pid, insertBooking("CONFIRMED", "OK", confirmed.ID), confirmed.ID); err != nil {
		t.Fatal(err)
	}
	assertState(confirmed.ID, "confirmed", "", "pending")

	cancelReview := createStay("Cancellation review", "2099-02-01", "2099-02-02", "finance_status_cancelled")
	if err := st.LinkBookingToNamedStay(ctx, pid, insertBooking("REVIEW", "OK", cancelReview.ID), cancelReview.ID); err != nil {
		t.Fatal(err)
	}
	assertState(cancelReview.ID, "needs_review", "finance_status_cancelled", "not_applicable")

	cancelledBooking := createStay("Cancelled booking", "2099-03-01", "2099-03-02", "legacy_non_reservation_stay")
	if err := st.LinkBookingToNamedStay(ctx, pid, insertBooking("CANCELLED", "CANCELLED", cancelledBooking.ID), cancelledBooking.ID); err != nil {
		t.Fatal(err)
	}
	assertState(cancelledBooking.ID, "needs_review", "legacy_non_reservation_stay", "not_applicable")

	historical := createStay("Historical", "2020-01-01", "2020-01-02", "legacy_non_reservation_stay")
	if err := st.LinkBookingToNamedStay(ctx, pid, insertBooking("HISTORICAL", "OK", historical.ID), historical.ID); err != nil {
		t.Fatal(err)
	}
	assertState(historical.ID, "confirmed", "", "not_applicable")

	rejected := createStay("Rejected", "2099-04-01", "2099-04-02", "legacy_non_reservation_stay")
	if _, err := st.UpdateNamedStayReview(ctx, pid, rejected.ID, 1, "rejected", "legacy_non_reservation_stay"); err != nil {
		t.Fatal(err)
	}
	if err := st.LinkBookingToNamedStay(ctx, pid, insertBooking("REJECTED", "OK", rejected.ID), rejected.ID); err != nil {
		t.Fatal(err)
	}
	var resolution string
	if err := st.DB.QueryRow(`SELECT review_resolution FROM named_stays WHERE id = ?`, rejected.ID).Scan(&resolution); err != nil {
		t.Fatal(err)
	}
	if resolution != "rejected" {
		t.Fatalf("finance evidence changed rejected resolution to %q", resolution)
	}
}

func TestLinkBookingToNamedStayLowersFirstKnownAtFromEarlierFinanceEvidence(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	stay := createFinanceNamedStay(t, st, pid, "LATE-LINK", "2026-06-01", "2026-06-02")
	now := time.Now().UTC().Format(time.RFC3339)
	bookedOn := "2020-02-03T04:05:06Z"
	res, err := st.DB.Exec(`
		INSERT INTO finance_bookings (
			property_id, named_stay_id, reference_number, net_cents, payout_date,
			booked_on, created_at, updated_at
		) VALUES (?, ?, 'LATE-LINK', 10000, '2026-06-03', ?, ?, ?)`, pid, stay.ID, bookedOn, now, now)
	if err != nil {
		t.Fatal(err)
	}
	bookingID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.LinkBookingToNamedStay(ctx, pid, bookingID, stay.ID); err != nil {
		t.Fatal(err)
	}
	updated, err := st.GetNamedStay(ctx, pid, stay.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := updated.FirstKnownAt.Format(time.RFC3339); got != bookedOn {
		t.Fatalf("first_known_at=%q want %q", got, bookedOn)
	}
}

func TestCanonicalFinanceEvidenceUpdateConfirmsAlreadyLinkedMigrationStay(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	stay, err := st.CreateNamedStayRecord(ctx, NamedStayCreateInput{
		PropertyID: pid, DisplayName: "Canonical update", StayType: StayTypeBookingCom,
		CheckInDate: "2099-05-01", CheckOutDate: "2099-05-02",
		ReviewStatus: "needs_review", ReviewReason: "legacy_non_reservation_stay",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := st.DB.Exec(`INSERT INTO finance_bookings (property_id, reference_number, net_cents, payout_date, named_stay_id, created_at, updated_at, source_channel) VALUES (?, 'CANONICAL', 10000, '2099-05-03', ?, ?, ?, 'booking_com')`, pid, stay.ID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	bookingID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	status := "OK"
	bookedOn := "2020-05-01T00:00:00Z"
	if _, err := st.UpsertFinanceBookingFromCanonical(ctx, pid, bookingID, 0, statements.CanonicalBooking{
		ReferenceNumber: "CANONICAL", SourceChannel: "booking_com", HasStatementData: true, Status: &status, BookedOn: &bookedOn,
	}); err != nil {
		t.Fatal(err)
	}
	var reviewStatus, nukiStatus string
	if err := st.DB.QueryRow(`SELECT review_status, nuki_generation_status FROM named_stays WHERE id = ?`, stay.ID).Scan(&reviewStatus, &nukiStatus); err != nil {
		t.Fatal(err)
	}
	if reviewStatus != "confirmed" || nukiStatus != "pending" {
		t.Fatalf("status=%q nuki=%q", reviewStatus, nukiStatus)
	}
	updated, err := st.GetNamedStay(ctx, pid, stay.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := updated.FirstKnownAt.Format(time.RFC3339); got != bookedOn {
		t.Fatalf("first_known_at=%q want imported booked_on %q", got, bookedOn)
	}
}

func TestNewFinanceBookingsRequireNamedStayBeforeAnyWrite(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	ref := "UNMATCHED"
	net := 1000
	payoutDate := time.Now().UTC().Format(time.RFC3339)

	if _, err := st.UpsertFinanceBookingFromCanonical(ctx, pid, 0, 0, statements.CanonicalBooking{
		ReferenceNumber: ref,
		SourceChannel:   "booking_com",
		NetCents:        &net,
		PayoutDate:      &payoutDate,
	}); err == nil {
		t.Fatal("expected unmatched canonical insert to fail")
	}
	if _, err := st.ImportBookingPayoutRow(ctx, &FinanceTransaction{
		PropertyID: pid, TransactionDate: time.Now().UTC(), Direction: "incoming", AmountCents: net,
		SourceType: "booking_payout", SourceReference: sql.NullString{String: ref, Valid: true},
	}, &FinanceBookingPayout{
		PropertyID: pid, ReferenceNumber: ref, NetCents: net, PayoutDate: time.Now().UTC(),
	}, 0); err == nil {
		t.Fatal("expected unmatched payout insert to fail")
	}
	var bookings, transactions int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM finance_bookings WHERE property_id = ?`, pid).Scan(&bookings); err != nil {
		t.Fatal(err)
	}
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM finance_transactions WHERE property_id = ?`, pid).Scan(&transactions); err != nil {
		t.Fatal(err)
	}
	if bookings != 0 || transactions != 0 {
		t.Fatalf("bookings=%d transactions=%d, want no writes", bookings, transactions)
	}
	stay := createFinanceNamedStay(t, st, pid, "MATCHED", "2026-07-01", "2026-07-02")
	matchedRef := "MATCHED"
	if _, err := st.UpsertFinanceBookingFromCanonical(ctx, pid, 0, stay.ID, statements.CanonicalBooking{
		ReferenceNumber: matchedRef,
		SourceChannel:   "booking_com",
		NetCents:        &net,
		PayoutDate:      &payoutDate,
	}); err != nil {
		t.Fatalf("matched canonical insert: %v", err)
	}
	var savedStayID int64
	if err := st.DB.QueryRow(`SELECT named_stay_id FROM finance_bookings WHERE property_id = ? AND reference_number = ?`, pid, matchedRef).Scan(&savedStayID); err != nil {
		t.Fatal(err)
	}
	if savedStayID != stay.ID {
		t.Fatalf("named_stay_id=%d want %d", savedStayID, stay.ID)
	}
}

func TestOpenFinanceMonth_IsIdempotentForRecurringRules(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	loc := time.UTC
	month := time.Now().UTC().Format("2006-01")

	rule, err := st.CreateFinanceRecurringRule(context.Background(), &FinanceRecurringRule{
		PropertyID:    pid,
		Title:         "Internet monthly",
		CategoryID:    sql.NullInt64{},
		AmountCents:   2500,
		Direction:     "outgoing",
		Frequency:     "monthly",
		StartMonth:    month,
		EffectiveFrom: time.Now().UTC().Add(-24 * time.Hour),
		Active:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rule.ID == 0 {
		t.Fatalf("rule id missing")
	}

	if _, err := st.OpenFinanceMonth(context.Background(), pid, month, nil, loc); err != nil {
		t.Fatal(err)
	}
	if _, err := st.OpenFinanceMonth(context.Background(), pid, month, nil, loc); err != nil {
		t.Fatal(err)
	}

	txs, err := st.ListFinanceTransactions(context.Background(), pid, month, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, tx := range txs {
		if tx.SourceType == "recurring_rule" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("recurring transactions = %d, want 1", count)
	}
}

func TestResetFinanceRecords_DeletesFinanceDataAndPreservesCleaningSalary(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	ctx := context.Background()
	prop, err := st.GetProperty(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	actorID := prop.OwnerUserID
	now := time.Now().UTC().Format(time.RFC3339)
	month := "2026-04"

	bookingCat := categoryIDByCode(t, st, pid, "booking_income")
	cleaningCat := categoryIDByCode(t, st, pid, "cleaning_salary")
	manualTx, err := st.CreateFinanceTransaction(ctx, &FinanceTransaction{
		PropertyID:      pid,
		TransactionDate: time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC),
		Direction:       "incoming",
		AmountCents:     10000,
		CategoryID:      sql.NullInt64{Int64: bookingCat, Valid: true},
		SourceType:      "manual",
		AttachmentPath:  sql.NullString{String: "attachments/test/manual.pdf", Valid: true},
	})
	if err != nil || manualTx.ID == 0 {
		t.Fatalf("create manual tx: %v", err)
	}
	bookingTx, err := st.CreateFinanceTransaction(ctx, &FinanceTransaction{
		PropertyID:      pid,
		TransactionDate: time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC),
		Direction:       "incoming",
		AmountCents:     50000,
		CategoryID:      sql.NullInt64{Int64: bookingCat, Valid: true},
		SourceType:      "booking_payout",
		SourceReference: sql.NullString{String: "REF-1", Valid: true},
		IsAutoGenerated: true,
	})
	if err != nil {
		t.Fatalf("create booking tx: %v", err)
	}
	if _, err := st.CreateFinanceRecurringRule(ctx, &FinanceRecurringRule{
		PropertyID:    pid,
		Title:         "Internet",
		AmountCents:   2500,
		Direction:     "outgoing",
		Frequency:     "monthly",
		StartMonth:    month,
		EffectiveFrom: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		Active:        true,
	}); err != nil {
		t.Fatalf("create recurring rule: %v", err)
	}
	if _, err := st.CreateFinanceTransaction(ctx, &FinanceTransaction{
		PropertyID:      pid,
		TransactionDate: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		Direction:       "outgoing",
		AmountCents:     2500,
		SourceType:      "recurring_rule",
		SourceReference: sql.NullString{String: "1:2026-04", Valid: true},
		IsAutoGenerated: true,
	}); err != nil {
		t.Fatalf("create recurring tx: %v", err)
	}
	if _, err := st.CreateFinanceTransaction(ctx, &FinanceTransaction{
		PropertyID:      pid,
		TransactionDate: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		Direction:       "outgoing",
		AmountCents:     1,
		CategoryID:      sql.NullInt64{Int64: cleaningCat, Valid: true},
		SourceType:      "cleaning_salary",
		SourceReference: sql.NullString{String: month, Valid: true},
		IsAutoGenerated: true,
	}); err != nil {
		t.Fatalf("create cleaning salary tx: %v", err)
	}
	if _, err := st.CreateFinanceTransaction(ctx, &FinanceTransaction{
		PropertyID:      pid,
		TransactionDate: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		Direction:       "outgoing",
		AmountCents:     999,
		CategoryID:      sql.NullInt64{Int64: cleaningCat, Valid: true},
		SourceType:      "cleaning_salary",
		SourceReference: sql.NullString{String: "2026-05", Valid: true},
		IsAutoGenerated: true,
	}); err != nil {
		t.Fatalf("create stale cleaning salary tx: %v", err)
	}

	if err := st.CreateCleanerFeeHistoryRow(ctx, pid, 4000, 1000, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), &actorID); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCleaningDailyLog(ctx, &CleaningDailyLog{
		PropertyID:       pid,
		DayDate:          "2026-04-10",
		FirstEntryAt:     sql.NullTime{Time: time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC), Valid: true},
		CountedForSalary: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateCleaningAdjustment(ctx, pid, 2026, 4, 500, "bonus", &actorID); err != nil {
		t.Fatal(err)
	}

	importID, err := st.CreateFinanceImport(ctx, &FinanceImport{PropertyID: pid, SourceType: "booking_payout", SourceChannel: "booking_com", UploadedAt: time.Now().UTC(), RowCountTotal: 1})
	if err != nil {
		t.Fatal(err)
	}
	stay := createFinanceNamedStay(t, st, pid, "REF-1", "2026-04-03", "2026-04-04")
	bookingRes, err := st.DB.ExecContext(ctx, `
		INSERT INTO finance_bookings (property_id, named_stay_id, reference_number, net_cents, payout_date, transaction_id, created_at, updated_at, has_payout_data)
		VALUES (?, ?, 'REF-1', 50000, ?, ?, ?, ?, 1)`, pid, stay.ID, time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC).Format(time.RFC3339), bookingTx.ID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	bookingID, _ := bookingRes.LastInsertId()
	if _, err := st.DB.ExecContext(ctx, `INSERT INTO finance_booking_merges (booking_id, import_id, source_type, changed_fields_json, occurred_at) VALUES (?, ?, 'booking_payout', '[]', ?)`, bookingID, importID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `INSERT INTO invoice_sequences (property_id, sequence_year, current_value, created_at, updated_at) VALUES (?, 2026, 7, ?, ?)`, pid, now, now); err != nil {
		t.Fatal(err)
	}
	invoiceRes, err := st.DB.ExecContext(ctx, `
		INSERT INTO invoices (
			property_id, named_stay_id, finance_booking_payout_id, invoice_number, sequence_year, sequence_value, language,
			issue_date, taxable_supply_date, due_date, stay_start_date, stay_end_date,
			supplier_snapshot_json, customer_snapshot_json, amount_total_cents, currency, payment_status,
			payment_note, version, created_at, updated_at
		) VALUES (?, ?, ?, 'T/2026/0007', 2026, 7, 'en', ?, ?, ?, ?, ?, '{}', '{}', 50000, 'EUR', 'paid', 'paid', 1, ?, ?)`,
		pid, stay.ID, bookingID, now, now, now, now, now, now, now)
	if err != nil {
		t.Fatal(err)
	}
	invoiceID, _ := invoiceRes.LastInsertId()
	if _, err := st.DB.ExecContext(ctx, `INSERT INTO invoice_files (invoice_id, version, file_path, file_size_bytes, created_at) VALUES (?, 1, 'invoices/test/invoice.pdf', 10, ?)`, invoiceID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `
		INSERT INTO finance_month_states (property_id, month, opened_at, opened_by, last_synced_at, last_synced_by, last_synced_reason)
		VALUES (?, '2026-03', ?, ?, ?, ?, 'test')`, pid, now, actorID, now, actorID); err != nil {
		t.Fatal(err)
	}

	preview, err := st.PreviewFinanceReset(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if preview.WouldDelete.FinanceTransactions != 3 || preview.WouldDelete.Invoices != 1 || preview.WouldDelete.InvoiceFiles != 1 {
		t.Fatalf("unexpected preview delete counts: %+v", preview.WouldDelete)
	}
	if preview.WouldPreserve.CleaningSalaryTransactions != 2 || preview.WouldPreserve.CleaningDailyLogs != 1 {
		t.Fatalf("unexpected preview preserve counts: %+v", preview.WouldPreserve)
	}

	result, files, err := st.ResetFinanceRecords(ctx, pid, &actorID, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if result.ResetRunID == 0 {
		t.Fatal("reset_run_id missing")
	}
	if result.Deleted.FinanceTransactions != 3 || result.Deleted.FinanceBookings != 1 || result.Deleted.Invoices != 1 || result.Deleted.InvoiceFiles != 1 {
		t.Fatalf("unexpected delete counts: %+v", result.Deleted)
	}
	if result.Regenerated.CleaningSalaryUpdated != 1 || result.Regenerated.CleaningSalaryInserted != 0 {
		t.Fatalf("unexpected regenerated counts: %+v", result.Regenerated)
	}
	if len(files) != 2 {
		t.Fatalf("file paths=%v, want 2 paths", files)
	}

	txs, err := st.ListFinanceTransactions(ctx, pid, "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) != 1 {
		t.Fatalf("transactions after reset=%d want 1: %+v", len(txs), txs)
	}
	if txs[0].SourceType != "cleaning_salary" || txs[0].SourceReference.String != month || txs[0].AmountCents != 5500 {
		t.Fatalf("unexpected cleaning salary tx after reset: %+v", txs[0])
	}
	for _, table := range []string{"finance_bookings", "finance_imports", "finance_booking_merges", "finance_recurring_rules", "invoices", "invoice_files"} {
		var n int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
		if err := st.DB.QueryRowContext(ctx, query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s rows=%d want 0", table, n)
		}
	}
	var seq int
	if err := st.DB.QueryRowContext(ctx, `SELECT current_value FROM invoice_sequences WHERE property_id = ? AND sequence_year = 2026`, pid).Scan(&seq); err != nil {
		t.Fatal(err)
	}
	if seq != 7 {
		t.Fatalf("invoice sequence=%d want 7", seq)
	}
	var reason string
	if err := st.DB.QueryRowContext(ctx, `SELECT last_synced_reason FROM finance_month_states WHERE property_id = ? AND month = ?`, pid, month).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != "finance_reset_preserve_cleaning_salary" {
		t.Fatalf("month state reason=%q", reason)
	}
	var deletedJSON string
	if err := st.DB.QueryRowContext(ctx, `SELECT deleted_counts_json FROM finance_reset_runs WHERE id = ?`, result.ResetRunID).Scan(&deletedJSON); err != nil {
		t.Fatal(err)
	}
	var deletedCounts FinanceResetDeleteCounts
	if err := json.Unmarshal([]byte(deletedJSON), &deletedCounts); err != nil {
		t.Fatal(err)
	}
	if deletedCounts.Invoices != 1 || deletedCounts.FinanceTransactions != 3 {
		t.Fatalf("reset run deleted json=%+v", deletedCounts)
	}

	second, _, err := st.ResetFinanceRecords(ctx, pid, &actorID, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if second.Deleted.FinanceTransactions != 0 || second.Deleted.Invoices != 0 || second.Deleted.FinanceBookings != 0 || second.Deleted.FinanceMonthStates != 0 {
		t.Fatalf("second reset should be idempotent, deleted=%+v", second.Deleted)
	}
}

func TestSyncFinanceGeneratedEntries_TracksMetadataAndProtectsManualRows(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	ctx := context.Background()
	month := "2026-04"
	prop, err := st.GetProperty(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	actorID := prop.OwnerUserID

	if _, err := st.CreateFinanceRecurringRule(ctx, &FinanceRecurringRule{
		PropertyID:    pid,
		Title:         "Internet",
		AmountCents:   2500,
		Direction:     "outgoing",
		Frequency:     "monthly",
		StartMonth:    "2026-01",
		EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Active:        true,
	}); err != nil {
		t.Fatal(err)
	}
	manual, err := st.CreateFinanceTransaction(ctx, &FinanceTransaction{
		PropertyID:      pid,
		TransactionDate: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
		Direction:       "incoming",
		AmountCents:     12345,
		Note:            sql.NullString{String: "manual income", Valid: true},
		SourceType:      "manual",
	})
	if err != nil {
		t.Fatal(err)
	}

	sync, changes, err := st.SyncFinanceGeneratedEntriesForMonth(ctx, pid, month, &actorID, time.UTC, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if sync.Status != "synced" || !sync.LastSyncedAt.Valid || sync.LastSyncedReason.String != "manual" {
		t.Fatalf("sync metadata = %+v", sync)
	}
	if sync.LastSyncedBy.Int64 != actorID {
		t.Fatalf("last_synced_by = %d, want %d", sync.LastSyncedBy.Int64, actorID)
	}
	if changes.RecurringInserted != 1 {
		t.Fatalf("recurring inserted = %d, want 1", changes.RecurringInserted)
	}

	if _, _, err := st.SyncFinanceGeneratedEntriesForMonth(ctx, pid, month, &actorID, time.UTC, "recurring_rule_update"); err != nil {
		t.Fatal(err)
	}
	sync, err = st.GetFinanceGeneratedEntrySync(ctx, pid, month)
	if err != nil {
		t.Fatal(err)
	}
	if sync.LastSyncedReason.String != "recurring_rule_update" {
		t.Fatalf("last_synced_reason = %q, want recurring_rule_update", sync.LastSyncedReason.String)
	}

	txs, err := st.ListFinanceTransactions(ctx, pid, month, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	manualCount := 0
	for _, tx := range txs {
		if tx.ID == manual.ID {
			manualCount++
			if tx.Note.String != "manual income" || tx.AmountCents != 12345 || tx.SourceType != "manual" {
				t.Fatalf("manual transaction was mutated: %+v", tx)
			}
		}
	}
	if manualCount != 1 {
		t.Fatalf("manual transaction count = %d, want 1", manualCount)
	}
}

func TestComputeFinanceSummary_CalculatesCleanerMargin(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	month := "2026-04"
	tDate := time.Date(2026, 4, 5, 9, 0, 0, 0, time.UTC)

	bookingCat := categoryIDByCode(t, st, pid, "booking_income")
	cleaningCat := categoryIDByCode(t, st, pid, "cleaning_salary")

	_, err := st.CreateFinanceTransaction(context.Background(), &FinanceTransaction{
		PropertyID:      pid,
		TransactionDate: tDate,
		Direction:       "incoming",
		AmountCents:     100000,
		CategoryID:      sql.NullInt64{Int64: bookingCat, Valid: true},
		SourceType:      "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateFinanceTransaction(context.Background(), &FinanceTransaction{
		PropertyID:      pid,
		TransactionDate: tDate,
		Direction:       "outgoing",
		AmountCents:     40000,
		CategoryID:      sql.NullInt64{Int64: cleaningCat, Valid: true},
		SourceType:      "cleaning_salary",
		SourceReference: sql.NullString{String: month, Valid: true},
		IsAutoGenerated: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	sum, err := st.ComputeFinanceSummary(context.Background(), pid, month)
	if err != nil {
		t.Fatal(err)
	}
	if sum.MonthlyPropertyIncomeCents != 100000 {
		t.Fatalf("monthly property income = %d, want 100000", sum.MonthlyPropertyIncomeCents)
	}
	if sum.CleanerExpenseCents != 40000 {
		t.Fatalf("cleaner expense = %d, want 40000", sum.CleanerExpenseCents)
	}
	if sum.CleanerMargin < 0.399 || sum.CleanerMargin > 0.401 {
		t.Fatalf("cleaner margin = %f, want ~0.4", sum.CleanerMargin)
	}
}

func TestUpsertBookingFinanceTransaction_UsesCanonicalDirections(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	ctx := context.Background()
	pid := setupFinanceProperty(t, st)
	catID := categoryIDByCode(t, st, pid, "booking_income")
	payoutDate := time.Date(2026, 4, 7, 10, 0, 0, 0, time.UTC)
	stay := createFinanceNamedStay(t, st, pid, "REF-CANONICAL-DIR", "2026-04-05", "2026-04-07")

	payout := &FinanceBookingPayout{
		PropertyID:      pid,
		ReferenceNumber: "REF-CANONICAL-DIR",
		NetCents:        12345,
		PayoutDate:      payoutDate,
		NamedStayID:     sql.NullInt64{Int64: stay.ID, Valid: true},
	}
	if err := st.CreateBookingPayout(ctx, payout); err != nil {
		t.Fatal(err)
	}
	created, err := st.GetBookingPayoutByReference(ctx, pid, payout.ReferenceNumber)
	if err != nil {
		t.Fatal(err)
	}

	if err := st.UpsertBookingFinanceTransaction(ctx, pid, created.ID, created.ReferenceNumber, created.NetCents, payoutDate, catID, "PAYOUT-1"); err != nil {
		t.Fatal(err)
	}

	var direction string
	if err := st.DB.QueryRowContext(ctx, `SELECT direction FROM finance_transactions WHERE property_id = ? AND source_type = 'booking_payout' AND source_reference_id = ?`, pid, created.ReferenceNumber).Scan(&direction); err != nil {
		t.Fatal(err)
	}
	if direction != "incoming" {
		t.Fatalf("direction = %q, want incoming", direction)
	}

	summary, err := st.ComputeFinanceSummary(ctx, pid, "2026-04")
	if err != nil {
		t.Fatal(err)
	}
	if summary.MonthlyIncomingCents != 12345 || summary.MonthlyPropertyIncomeCents != 12345 {
		t.Fatalf("summary incoming=%d property_income=%d, want 12345", summary.MonthlyIncomingCents, summary.MonthlyPropertyIncomeCents)
	}
}

func TestOpenFinanceMonth_PositiveTimezoneKeepsTargetMonth(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	loc := time.FixedZone("UTC+2", 2*60*60)
	month := "2026-04"

	rule, err := st.CreateFinanceRecurringRule(context.Background(), &FinanceRecurringRule{
		PropertyID:    pid,
		Title:         "TZ recurring",
		CategoryID:    sql.NullInt64{},
		AmountCents:   1234,
		Direction:     "outgoing",
		Frequency:     "monthly",
		StartMonth:    month,
		EffectiveFrom: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
		Active:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rule.ID == 0 {
		t.Fatalf("rule id missing")
	}

	if _, err := st.OpenFinanceMonth(context.Background(), pid, month, nil, loc); err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListFinanceTransactions(context.Background(), pid, month, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatalf("expected recurring row in month %s", month)
	}
}

// TestOpenFinanceMonth_PurgesOrphanRecurringTransactions reproduces the bug
// where deactivating a recurring rule (and replacing it with a new one) left
// the old auto-generated finance_transaction in place across re-opens. After
// the fix, OpenFinanceMonth deletes any recurring-rule transactions whose
// source rule is no longer active or no longer covers the month.
func TestOpenFinanceMonth_PurgesOrphanRecurringTransactions(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	loc := time.UTC
	month := "2026-04"
	ctx := context.Background()

	old, err := st.CreateFinanceRecurringRule(ctx, &FinanceRecurringRule{
		PropertyID:    pid,
		Title:         "Mortgage old",
		AmountCents:   30076,
		Direction:     "outgoing",
		Frequency:     "monthly",
		StartMonth:    "2025-01",
		EffectiveFrom: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Active:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.OpenFinanceMonth(ctx, pid, month, nil, loc); err != nil {
		t.Fatal(err)
	}

	// Deactivate the old rule and add a replacement at a new amount.
	deactivated := false
	if _, err := st.UpdateFinanceRecurringRule(ctx, pid, old.ID, nil, nil, nil, nil, nil, nil, nil, nil, &deactivated); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateFinanceRecurringRule(ctx, &FinanceRecurringRule{
		PropertyID:    pid,
		Title:         "Mortgage new",
		AmountCents:   39470,
		Direction:     "outgoing",
		Frequency:     "monthly",
		StartMonth:    "2026-04",
		EffectiveFrom: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		Active:        true,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := st.OpenFinanceMonth(ctx, pid, month, nil, loc); err != nil {
		t.Fatal(err)
	}

	txs, err := st.ListFinanceTransactions(ctx, pid, month, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var total int
	for _, tx := range txs {
		if tx.SourceType == "recurring_rule" {
			total += tx.AmountCents
		}
	}
	if total != 39470 {
		t.Fatalf("recurring total cents = %d, want 39470 (only the active replacement rule should remain)", total)
	}
}

// TestDeleteFinanceRecurringRule_CascadesTransactions verifies that deleting
// a recurring rule also removes every auto-generated finance_transaction
// produced by it, regardless of source_reference_id format.
func TestDeleteFinanceRecurringRule_CascadesTransactions(t *testing.T) {
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	ctx := context.Background()

	rule, err := st.CreateFinanceRecurringRule(ctx, &FinanceRecurringRule{
		PropertyID:    pid,
		Title:         "Internet",
		AmountCents:   2500,
		Direction:     "outgoing",
		Frequency:     "monthly",
		StartMonth:    "2026-01",
		EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Active:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"2026-01", "2026-02", "2026-03"} {
		if _, err := st.OpenFinanceMonth(ctx, pid, m, nil, time.UTC); err != nil {
			t.Fatal(err)
		}
	}

	if err := st.DeleteFinanceRecurringRule(ctx, pid, rule.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := st.GetFinanceRecurringRuleByID(ctx, pid, rule.ID); err == nil {
		t.Fatal("rule still present after delete")
	}
	for _, m := range []string{"2026-01", "2026-02", "2026-03"} {
		txs, err := st.ListFinanceTransactions(ctx, pid, m, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, tx := range txs {
			if tx.SourceType == "recurring_rule" {
				t.Fatalf("month %s still has recurring_rule transaction id=%d", m, tx.ID)
			}
		}
	}
}

func TestOpenFinanceMonth_PositiveTimezoneKeepsTargetMonth_AfterPurge(t *testing.T) {
	// Smoke test: ensure existing positive-timezone test still passes after
	// the orphan purge is added (regression guard).
	st := &Store{DB: testutil.OpenTestDB(t)}
	pid := setupFinanceProperty(t, st)
	loc := time.FixedZone("UTC+2", 2*60*60)
	month := "2026-04"
	rule, err := st.CreateFinanceRecurringRule(context.Background(), &FinanceRecurringRule{
		PropertyID:    pid,
		Title:         "TZ recurring",
		AmountCents:   1234,
		Direction:     "outgoing",
		Frequency:     "monthly",
		StartMonth:    month,
		EffectiveFrom: time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC),
		Active:        true,
	})
	if err != nil || rule.ID == 0 {
		t.Fatalf("rule create failed: %v", err)
	}
	if _, err := st.OpenFinanceMonth(context.Background(), pid, month, nil, loc); err != nil {
		t.Fatal(err)
	}
}
