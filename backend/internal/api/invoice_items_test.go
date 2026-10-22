package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"pms/backend/internal/store"
)

func TestInvoiceItemsLifecycle(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	owner, err := st.CreateUser(ctx, "invoice-items@example.com", testPasswordHash(t, "secret123"), "owner")
	if err != nil {
		t.Fatal(err)
	}
	prop, err := st.CreateProperty(ctx, owner.ID, "Itemized", "Europe/Bratislava", "en")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdatePropertyProfile(ctx, prop.ID, map[string]interface{}{
		"billing_name": "Owner", "billing_address": "Main 1", "dic": "1234567890",
	}); err != nil {
		t.Fatal(err)
	}
	stay, err := st.CreateNamedStayRecord(ctx, store.NamedStayCreateInput{
		PropertyID: prop.ID, DisplayName: "Guest", StayType: store.StayTypeBookingCom,
		CheckInDate: "2026-11-01", CheckOutDate: "2026-11-03",
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: st, SessionTTL: time.Hour, DataDir: t.TempDir()}
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()
	cookies := loginCookies(t, ts.URL, "invoice-items@example.com", "secret123")
	base := fmt.Sprintf("%s/api/properties/%d/invoices", ts.URL, prop.ID)
	request := func(method, url string, body interface{}, want int) invoiceRow {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		var response invoiceResponse
		status := doAuthedJSONRequest(t, &http.Client{}, method, url, cookies, bytes.NewReader(raw), &response)
		if status != want {
			t.Fatalf("%s %s status=%d want=%d body=%s", method, url, status, want, raw)
		}
		return response.Invoice
	}
	items := []map[string]interface{}{
		{"description": " Accommodation ", "quantity": "1", "unit_price_cents": 100000},
		{"description": "Čistenie / Cleaning", "quantity": "1.500000", "unit_price_cents": 101, "line_total_cents": 99999},
	}
	for _, documentType := range []string{"proforma", "invoice"} {
		t.Run(documentType, func(t *testing.T) {
			body := map[string]interface{}{
				"document_type": documentType, "named_stay_id": stay.ID, "language": "en",
				"issue_date": "2026-10-22", "stay_start_date": "2026-11-01", "stay_end_date": "2026-11-03",
				"booking_reference": "TEST-REF", "guest_name": "Guest", "supplier_vat_status": "non_vat_payer",
				"customer": map[string]string{"company_name": "Company", "address_line_1": "Street 1", "city": "Sofia", "postal_code": "1000", "country": "BG"},
				"items":    items,
			}
			body["amount_total_cents"] = 1
			request(http.MethodPost, base, body, http.StatusBadRequest)
			delete(body, "amount_total_cents")
			created := request(http.MethodPost, base, body, http.StatusCreated)
			if created.AmountTotalCents != 100152 || len(created.Items) != 2 || created.Items[1].Quantity != "1.5" || created.Items[1].LineTotalCents != 152 {
				t.Fatalf("created items/total=%+v / %d", created.Items, created.AmountTotalCents)
			}
			request(http.MethodPost, base, body, http.StatusConflict)
			url := fmt.Sprintf("%s/%d", base, created.ID)
			for _, patch := range []map[string]interface{}{
				{"items": []interface{}{}},
				{"amount_total_cents": 12},
				{"items": []map[string]interface{}{{"description": "Cleaning", "quantity": "1"}}},
				{"items": []map[string]interface{}{{"description": "Cleaning", "quantity": "0", "unit_price_cents": 1}}},
				{"items": []map[string]interface{}{{"description": "Cleaning", "quantity": 1.5, "unit_price_cents": 1}}},
				{"items": []map[string]interface{}{{"description": "Cleaning", "quantity": "1.5", "unit_price_cents": 1.1}}},
			} {
				request(http.MethodPatch, url, patch, http.StatusBadRequest)
			}
			unchanged := request(http.MethodGet, url, nil, http.StatusOK)
			if unchanged.Version != 1 || !reflect.DeepEqual(unchanged.Items, created.Items) {
				t.Fatalf("invalid patch changed document: %+v", unchanged)
			}
			preserved := request(http.MethodPatch, url, map[string]interface{}{"items": nil, "price_details": "Includes cleaning"}, http.StatusOK)
			if !reflect.DeepEqual(preserved.Items, created.Items) {
				t.Fatal("null items did not preserve snapshot")
			}
			updated := request(http.MethodPatch, url, map[string]interface{}{"items": []map[string]interface{}{
				{"description": "Cleaning", "quantity": "2.5", "unit_price_cents": 101},
				{"description": "Accommodation", "quantity": "1", "unit_price_cents": 100000},
			}}, http.StatusOK)
			if updated.AmountTotalCents != 100253 || updated.Items[0].Description != "Cleaning" {
				t.Fatalf("updated=%+v", updated)
			}
			regen := request(http.MethodPost, url+"/regenerate", nil, http.StatusOK)
			if regen.AmountTotalCents != updated.AmountTotalCents || !reflect.DeepEqual(regen.Items, updated.Items) || regen.InvoiceNumber != created.InvoiceNumber {
				t.Fatalf("regeneration changed saved data: %+v", regen)
			}
			detail := request(http.MethodGet, url, nil, http.StatusOK)
			if detail.Files == nil || len(*detail.Files) != 4 {
				t.Fatalf("expected four PDF versions: %+v", detail.Files)
			}
		})
	}
	var documents, financeRows int
	if err := st.DB.QueryRow(`SELECT count(*) FROM invoices`).Scan(&documents); err != nil {
		t.Fatal(err)
	}
	if err := st.DB.QueryRow(`SELECT count(*) FROM finance_bookings`).Scan(&financeRows); err != nil {
		t.Fatal(err)
	}
	if documents != 2 || financeRows != 0 {
		t.Fatalf("documents=%d financeRows=%d want 2 and 0", documents, financeRows)
	}
}
