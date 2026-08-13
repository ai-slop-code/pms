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
