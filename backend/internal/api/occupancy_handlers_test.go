package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"pms/backend/internal/store"
)

func TestListOccupancySyncRunsReturnsRawBlockCounters(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	password := "secret123"
	owner, err := st.CreateUser(ctx, "occupancy-sync-runs@test.local", testPasswordHash(t, password), "owner")
	if err != nil {
		t.Fatal(err)
	}
	property, err := st.CreateProperty(ctx, owner.ID, "Sync counters", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	runID, err := st.StartOccupancySyncRun(ctx, property.ID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	counters := &store.SyncCounters{
		UpstreamEventsSeen:               8,
		RepresentationsInserted:          40,
		RepresentationsUpdated:           2,
		RepresentationsDeletedFromSource: 3,
		RawBlocksInserted:                1,
		RawBlocksUpdated:                 2,
		RawBlocksUnchanged:               3,
		RawBlocksDeletedFromSource:       4,
		RawBlockConflicts:                5,
	}
	if err := st.FinishOccupancySyncRunDetailed(ctx, runID, "success", nil, nil, counters); err != nil {
		t.Fatal(err)
	}
	var legacyUpserted int
	if err := st.DB.QueryRowContext(ctx, `SELECT occupancies_upserted FROM occupancy_sync_runs WHERE id = ?`, runID).Scan(&legacyUpserted); err != nil {
		t.Fatal(err)
	}
	if legacyUpserted != 0 {
		t.Fatalf("legacy occupancies_upserted=%d want 0", legacyUpserted)
	}

	server := httptest.NewServer((&Server{Store: st, SessionTTL: time.Hour}).Routes())
	t.Cleanup(server.Close)
	cookies := loginCookies(t, server.URL, owner.Email, password)
	var response struct {
		Runs []map[string]json.RawMessage `json:"runs"`
	}
	path := server.URL + "/api/properties/" + strconv.FormatInt(property.ID, 10) + "/occupancy-sync/runs"
	if status := doAuthedJSONRequest(t, http.DefaultClient, http.MethodGet, path, cookies, nil, &response); status != http.StatusOK {
		t.Fatalf("status=%d want %d", status, http.StatusOK)
	}
	if len(response.Runs) != 1 {
		t.Fatalf("runs=%d want 1", len(response.Runs))
	}
	run := response.Runs[0]
	for field, want := range map[string]string{
		"events_seen":                    "8",
		"raw_blocks_inserted":            "1",
		"raw_blocks_updated":             "2",
		"raw_blocks_unchanged":           "3",
		"raw_blocks_deleted_from_source": "4",
		"raw_block_conflicts":            "5",
	} {
		if got := string(run[field]); got != want {
			t.Errorf("%s=%s want %s", field, got, want)
		}
	}
	for _, field := range []string{
		"occupancies_upserted",
		"deletion_enabled",
		"representations_deleted_from_source",
		"duplicate_nights_resolved",
		"named_stays_deleted_from_source",
	} {
		if _, ok := run[field]; ok {
			t.Errorf("obsolete field %q is present", field)
		}
	}
}

func TestRemovedOccupancyCompatibilityRoutesReturn404(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	password := "secret123"
	owner, err := st.CreateUser(ctx, "removed-occupancy-routes@test.local", testPasswordHash(t, password), "owner")
	if err != nil {
		t.Fatal(err)
	}
	property, err := st.CreateProperty(ctx, owner.ID, "Removed routes", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer((&Server{Store: st, SessionTTL: time.Hour}).Routes())
	t.Cleanup(server.Close)
	cookies := loginCookies(t, server.URL, owner.Email, password)
	propertyPath := "/api/properties/" + strconv.FormatInt(property.ID, 10)

	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, propertyPath + "/occupancy-export"},
		{http.MethodGet, propertyPath + "/occupancies"},
		{http.MethodGet, propertyPath + "/occupancies/calendar"},
		{http.MethodPost, propertyPath + "/occupancies/1/close"},
		{http.MethodPost, propertyPath + "/occupancies/1/external-sale"},
		{http.MethodPost, propertyPath + "/occupancies/1/split-nights"},
		{http.MethodPost, propertyPath + "/occupancies/1/reopen"},
		{http.MethodPost, propertyPath + "/occupancies/1/outcome/cancelled-non-refundable"},
		{http.MethodPost, propertyPath + "/occupancies/1/outcome/no-show"},
		{http.MethodPost, propertyPath + "/occupancies/1/outcome/clear"},
		{http.MethodPost, propertyPath + "/occupancies/1/cleaning-calendar/exclude"},
		{http.MethodPost, propertyPath + "/occupancies/1/cleaning-calendar/include"},
		{http.MethodPost, propertyPath + "/occupancy-blocks/uid/named-stays"},
		{http.MethodPatch, propertyPath + "/occupancies/1/named-stay"},
		{http.MethodDelete, propertyPath + "/occupancies/1/named-stay"},
		{http.MethodPost, propertyPath + "/occupancy-repair/ics-reconciliation/dry-run"},
		{http.MethodPost, propertyPath + "/occupancy-repair/ics-reconciliation/apply"},
		{http.MethodPost, propertyPath + "/occupancy-api-tokens"},
		{http.MethodGet, propertyPath + "/occupancy-api-tokens"},
		{http.MethodDelete, propertyPath + "/occupancy-api-tokens/1"},
		{http.MethodGet, propertyPath + "/invoices/occupancy-candidates"},
	}
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			request, err := http.NewRequest(test.method, server.URL+test.path, strings.NewReader(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-PMS-Client", "test")
			for _, cookie := range cookies {
				request.AddCookie(cookie)
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusNotFound {
				t.Fatalf("status=%d want 404", response.StatusCode)
			}
		})
	}
}
