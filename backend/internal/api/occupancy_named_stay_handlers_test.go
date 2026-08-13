package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"pms/backend/internal/ctxuser"
	"pms/backend/internal/store"
)

func namedStayPatchRequest(t *testing.T, actor *store.User, propertyID, stayID int64, path, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", formatInt64(propertyID))
	routeContext.URLParams.Add("stayId", formatInt64(stayID))
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeContext)
	return req.WithContext(ctxuser.WithUser(ctx, actor))
}

func namedStayPostRequest(t *testing.T, actor *store.User, propertyID int64, path, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", formatInt64(propertyID))
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeContext)
	return req.WithContext(ctxuser.WithUser(ctx, actor))
}

func TestNamedStayRequestsRejectGuestDisplayNameAlias(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	actor, err := st.CreateUser(ctx, "strict-named-stay@test.local", "hash", "super_admin")
	if err != nil {
		t.Fatal(err)
	}
	property, err := st.CreateProperty(ctx, actor.ID, "Strict named stays", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	stay, err := st.CreateNamedStayRecord(ctx, store.NamedStayCreateInput{
		PropertyID: property.ID, DisplayName: "Canonical name", StayType: store.StayTypeExternal,
		CheckInDate: "2026-11-01", CheckOutDate: "2026-11-03", CreatedByUserID: actor.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{Store: st}

	t.Run("create", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		request := namedStayPostRequest(t, actor, property.ID, "/api/properties/1/stays",
			`{"guest_display_name":"Legacy name","stay_type":"external","check_in":"2026-11-10","check_out":"2026-11-12"}`)
		server.postStay(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("patch", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		request := namedStayPatchRequest(t, actor, property.ID, stay.ID,
			"/api/properties/1/stays/1", `{"guest_display_name":"Legacy name"}`)
		server.patchStay(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		unchanged, err := st.GetNamedStay(ctx, property.ID, stay.ID)
		if err != nil {
			t.Fatal(err)
		}
		if unchanged.DisplayName != "Canonical name" {
			t.Fatalf("display_name=%q", unchanged.DisplayName)
		}
	})
}

func TestPatchStayOutcomePersistsCanonicalMetadata(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	actor, err := st.CreateUser(ctx, "outcome-handler@test.local", "hash", "super_admin")
	if err != nil {
		t.Fatal(err)
	}
	property, err := st.CreateProperty(ctx, actor.ID, "Outcome", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	stay, err := st.CreateNamedStayRecord(ctx, store.NamedStayCreateInput{
		PropertyID: property.ID, DisplayName: "Outcome Guest", StayType: store.StayTypeExternal,
		CheckInDate: "2026-12-01", CheckOutDate: "2026-12-03", CreatedByUserID: actor.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{Store: st}
	recorder := httptest.NewRecorder()
	request := namedStayPatchRequest(t, actor, property.ID, stay.ID,
		"/api/properties/1/stays/1/outcome", `{"outcome":"no_show","reason":"  no arrival  "}`)
	server.patchStayOutcome(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	updated, err := st.GetNamedStay(ctx, property.ID, stay.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.StayOutcome.String != store.StayOutcomeNoShow || updated.StayOutcomeReason.String != "no arrival" ||
		updated.StayOutcomeActorUserID.Int64 != actor.ID || !updated.StayOutcomeMarkedAt.Valid {
		t.Fatalf("outcome metadata=%+v", updated)
	}

	recorder = httptest.NewRecorder()
	request = namedStayPatchRequest(t, actor, property.ID, stay.ID,
		"/api/properties/1/stays/1/outcome", `{"outcome":null}`)
	server.patchStayOutcome(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("clear status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	updated, err = st.GetNamedStay(ctx, property.ID, stay.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.StayOutcome.Valid || updated.StayOutcomeReason.Valid || updated.StayOutcomeActorUserID.Valid || updated.StayOutcomeMarkedAt.Valid {
		t.Fatalf("outcome clear metadata=%+v", updated)
	}
}

func TestPatchStayReviewPersistsDecisionAttribution(t *testing.T) {
	st := testDB(t)
	ctx := context.Background()
	actor, err := st.CreateUser(ctx, "review-handler@test.local", "hash", "super_admin")
	if err != nil {
		t.Fatal(err)
	}
	property, err := st.CreateProperty(ctx, actor.ID, "Review", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	stay, err := st.CreateNamedStayRecord(ctx, store.NamedStayCreateInput{
		PropertyID: property.ID, DisplayName: "Review Guest", StayType: store.StayTypeBookingCom,
		CheckInDate: "2026-12-10", CheckOutDate: "2026-12-12", CreatedByUserID: actor.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := namedStayPatchRequest(t, actor, property.ID, stay.ID,
		"/api/properties/1/stays/1/review", `{"review_status":"rejected","reason":"  duplicate  "}`)
	(&Server{Store: st}).patchStayReview(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	updated, err := st.GetNamedStay(ctx, property.ID, stay.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ReviewStatus.String != "rejected" || updated.ReviewReason.String != "duplicate" ||
		updated.ReviewActorUserID.Int64 != actor.ID || !updated.ReviewedAt.Valid {
		t.Fatalf("review metadata=%+v", updated)
	}
}

func formatInt64(value int64) string {
	return strconv.FormatInt(value, 10)
}
