package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"pms/backend/internal/store"
)

func TestFinanceImportRejectsUnmatchedEvidenceBeforeCanonicalWrite(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash := testPasswordHash(t, "secret123")
	owner, err := st.CreateUser(ctx, "finance-unmatched@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	property, err := st.CreateProperty(ctx, owner.ID, "Unmatched Import", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cookies := loginCookies(t, ts.URL, "finance-unmatched@example.com", "secret123")

	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	part, err := writer.CreateFormFile("file", "payout.csv")
	if err != nil {
		t.Fatal(err)
	}
	const payout = `"Reference number","Type","Guest name","Check-in","Checkout","Amount","Commission","Payments service fee","Net","Currency","Payout date","Payout ID","Payment status","Reservation status"
"UNMATCHED-1","reservation","No Stay","1 Sept 2026","3 Sept 2026","250.00","-37.50","-3.50","209.00","EUR","5 Sept 2026","PO-123","paid","OK"`
	if _, err := io.WriteString(part, payout); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/properties/"+strconv.FormatInt(property.ID, 10)+"/finance/imports/preview", &form)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-PMS-Client", "test")
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("preview status=%d body=%s", res.StatusCode, raw)
	}
	var preview financeImportPreviewResponse
	if err := json.NewDecoder(res.Body).Decode(&preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Inserts) != 0 || len(preview.Rejected) != 1 || preview.PreviewToken == "" {
		t.Fatalf("unexpected preview: %+v", preview)
	}

	commitBody, _ := json.Marshal(financeImportCommitRequest{PreviewToken: preview.PreviewToken})
	var commit financeImportCommitResponse
	status := doAuthedJSONRequest(t, http.DefaultClient, http.MethodPost,
		ts.URL+"/api/properties/"+strconv.FormatInt(property.ID, 10)+"/finance/imports/commit",
		cookies, bytes.NewReader(commitBody), &commit)
	if status != http.StatusOK || commit.RowCountRejected != 1 || commit.RowCountInserted != 0 {
		t.Fatalf("status=%d commit=%+v", status, commit)
	}
	var bookings, transactions int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM finance_bookings WHERE property_id = ?`, property.ID).Scan(&bookings); err != nil {
		t.Fatal(err)
	}
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM finance_transactions WHERE property_id = ?`, property.ID).Scan(&transactions); err != nil {
		t.Fatal(err)
	}
	if bookings != 0 || transactions != 0 {
		t.Fatalf("bookings=%d transactions=%d, want no canonical writes", bookings, transactions)
	}
}

func TestFinanceStatementRetainsUnmatchedCancellationEvidence(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash := testPasswordHash(t, "secret123")
	owner, err := st.CreateUser(ctx, "statement-cancel@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	property, err := st.CreateProperty(ctx, owner.ID, "Statement Cancellation", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cookies := loginCookies(t, ts.URL, "statement-cancel@example.com", "secret123")
	const csvBody = `"Reservation number","Invoice number","Booked on","Arrival","Departure","Booker name","Guest name","Rooms","Persons","Room nights","Commission %","Original amount","Final amount","Commission amount","Payment fee","Status","Guest request","Currency","Hotel id","Property name","City","Country"
"CANCEL-1","INV-1","2026-08-14T11:45:20","2026-08-20","2026-08-21","Booker","Guest","1","1","0","0","0","0","0","","CANCELLED","","EUR","H","P","C","SK"`
	var form bytes.Buffer
	w := multipart.NewWriter(&form)
	p, err := w.CreateFormFile("file", "statement.csv")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(p, csvBody)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/properties/"+strconv.FormatInt(property.ID, 10)+"/finance/imports/preview", &form)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-PMS-Client", "test")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var preview financeImportPreviewResponse
	if res.StatusCode != http.StatusOK {
		t.Fatalf("preview status=%d", res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(&preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.SkippedCancellations) != 1 || len(preview.Rejected) != 0 {
		t.Fatalf("preview=%+v", preview)
	}
	body, _ := json.Marshal(financeImportCommitRequest{PreviewToken: preview.PreviewToken})
	var commit financeImportCommitResponse
	status := doAuthedJSONRequest(t, http.DefaultClient, http.MethodPost, ts.URL+"/api/properties/"+strconv.FormatInt(property.ID, 10)+"/finance/imports/commit", cookies, bytes.NewReader(body), &commit)
	if status != http.StatusOK || commit.RowCountSkippedCancellations != 1 || commit.RowCountTotal != 1 {
		t.Fatalf("status=%d commit=%+v", status, commit)
	}
	var evidence, bookings int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM finance_statement_evidence WHERE property_id = ?`, property.ID).Scan(&evidence); err != nil {
		t.Fatal(err)
	}
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM finance_bookings WHERE property_id = ?`, property.ID).Scan(&bookings); err != nil {
		t.Fatal(err)
	}
	if evidence != 1 || bookings != 0 {
		t.Fatalf("evidence=%d bookings=%d", evidence, bookings)
	}
}

func TestFinanceImportAutomaticallyLinksAndRenamesUniqueExactDateStay(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	hash := testPasswordHash(t, "secret123")
	owner, err := st.CreateUser(ctx, "finance-auto@example.com", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	property, err := st.CreateProperty(ctx, owner.ID, "Automatic Import", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	stay, err := st.CreateNamedStayRecord(ctx, store.NamedStayCreateInput{
		PropertyID: property.ID, DisplayName: "Martin", StayType: store.StayTypeBookingCom,
		CheckInDate: "2026-08-18", CheckOutDate: "2026-08-19", CreatedByUserID: owner.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	srv := &Server{Store: st, SessionTTL: time.Hour}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	cookies := loginCookies(t, ts.URL, "finance-auto@example.com", "secret123")
	const payout = `"Reference number","Type","Guest name","Check-in","Checkout","Amount","Commission","Payments service fee","Net","Currency","Payout date","Payout ID","Payment status","Reservation status"
"AUTO-1","reservation","Martin Schneider","18 Aug 2026","19 Aug 2026","68.00","-10.00","-0.84","57.16","EUR","20 Aug 2026","PO-AUTO","by_booking","OK"`

	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	part, err := writer.CreateFormFile("file", "payout.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, payout); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	previewReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/properties/"+strconv.FormatInt(property.ID, 10)+"/finance/imports/preview", &form)
	if err != nil {
		t.Fatal(err)
	}
	previewReq.Header.Set("Content-Type", writer.FormDataContentType())
	previewReq.Header.Set("X-PMS-Client", "test")
	for _, cookie := range cookies {
		previewReq.AddCookie(cookie)
	}
	previewRes, err := http.DefaultClient.Do(previewReq)
	if err != nil {
		t.Fatal(err)
	}
	defer previewRes.Body.Close()
	if previewRes.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(previewRes.Body)
		t.Fatalf("preview status=%d body=%s", previewRes.StatusCode, raw)
	}
	var preview financeImportPreviewResponse
	if err := json.NewDecoder(previewRes.Body).Decode(&preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Inserts) != 1 || len(preview.StayNameChanges) != 1 || len(preview.NeedsStaySelection) != 0 || len(preview.Rejected) != 0 {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	if preview.Inserts[0].Line != 2 || preview.Inserts[0].NamedStayID != stay.ID || preview.StayNameChanges[0].PayoutGuestName != "Martin Schneider" {
		t.Fatalf("unexpected match metadata: %+v / %+v", preview.Inserts[0], preview.StayNameChanges[0])
	}

	commitBody, _ := json.Marshal(financeImportCommitRequest{PreviewToken: preview.PreviewToken})
	var commit financeImportCommitResponse
	status := doAuthedJSONRequest(t, http.DefaultClient, http.MethodPost,
		ts.URL+"/api/properties/"+strconv.FormatInt(property.ID, 10)+"/finance/imports/commit",
		cookies, bytes.NewReader(commitBody), &commit)
	if status != http.StatusOK || commit.RowCountTotal != 1 || commit.RowCountInserted != 1 || commit.RowCountRejected != 0 {
		t.Fatalf("status=%d commit=%+v", status, commit)
	}
	var bookingStayID int64
	if err := st.DB.QueryRowContext(ctx, `SELECT named_stay_id FROM finance_bookings WHERE property_id = ? AND reference_number = ?`, property.ID, "AUTO-1").Scan(&bookingStayID); err != nil {
		t.Fatal(err)
	}
	if bookingStayID != stay.ID {
		t.Fatalf("booking linked to stay %d, want %d", bookingStayID, stay.ID)
	}
	updatedStay, err := st.GetNamedStay(ctx, property.ID, stay.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedStay.DisplayName != "Martin Schneider" {
		t.Fatalf("stay name=%q", updatedStay.DisplayName)
	}
	var transactionCount, transactionAmount int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(amount_cents), 0) FROM finance_transactions WHERE property_id = ? AND source_type = 'booking_payout'`, property.ID).Scan(&transactionCount, &transactionAmount); err != nil {
		t.Fatal(err)
	}
	if transactionCount != 1 || transactionAmount != 5716 {
		t.Fatalf("transactions=%d amount=%d", transactionCount, transactionAmount)
	}
}
