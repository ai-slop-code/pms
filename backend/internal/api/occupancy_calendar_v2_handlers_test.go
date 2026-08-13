package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"pms/backend/internal/ctxuser"
	"pms/backend/internal/permissions"
	"pms/backend/internal/store"
)

func availabilityBlockPatchRequest(actor *store.User, propertyID, blockID int64, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPatch, "/api/properties/1/availability-blocks/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", formatInt64(propertyID))
	routeContext.URLParams.Add("blockId", formatInt64(blockID))
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeContext)
	return req.WithContext(ctxuser.WithUser(ctx, actor))
}

func TestPatchAvailabilityBlockStatusPermissionsAndPropertyScope(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	owner, err := st.CreateUser(ctx, "block-owner@test.local", "hash", "owner")
	if err != nil {
		t.Fatal(err)
	}
	reader, err := st.CreateUser(ctx, "block-reader@test.local", "hash", "read_only")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := st.CreateUser(ctx, "block-admin@test.local", "hash", "super_admin")
	if err != nil {
		t.Fatal(err)
	}
	property, err := st.CreateProperty(ctx, owner.ID, "Blocks", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateProperty(ctx, owner.ID, "Other blocks", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertPropertyPermission(ctx, reader.ID, property.ID, permissions.Occupancy, permissions.LevelRead); err != nil {
		t.Fatal(err)
	}
	block, err := st.CreateAvailabilityBlock(ctx, property.ID, store.AvailabilityBlockInput{
		BlockType: "closed", StartDate: "2026-11-01", EndDate: "2026-11-03",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"block_type":"closed","start_date":"2026-11-01","end_date":"2026-11-03","status":"archived"}`
	server := &Server{Store: st}

	for _, test := range []struct {
		name       string
		actor      *store.User
		propertyID int64
		wantStatus int
	}{
		{name: "read permission cannot mutate", actor: reader, propertyID: property.ID, wantStatus: http.StatusForbidden},
		{name: "block cannot cross property scope", actor: admin, propertyID: other.ID, wantStatus: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			server.patchAvailabilityBlock(recorder, availabilityBlockPatchRequest(test.actor, test.propertyID, block.ID, body))
			if recorder.Code != test.wantStatus {
				t.Fatalf("status=%d body=%s want %d", recorder.Code, recorder.Body.String(), test.wantStatus)
			}
		})
	}

	recorder := httptest.NewRecorder()
	server.patchAvailabilityBlock(recorder, availabilityBlockPatchRequest(owner, property.ID, block.ID, body))
	if recorder.Code != http.StatusOK {
		t.Fatalf("archive status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	updated, err := st.GetAvailabilityBlock(ctx, property.ID, block.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "archived" {
		t.Fatalf("status=%q want archived", updated.Status)
	}
}
