package nuki

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"pms/backend/internal/store"
	"pms/backend/internal/testutil"
)

type fakeClient struct {
	createCalls int
	updateCalls int
	revokeCalls int
	listCalls   int
	logCalls    int
	setCalls    int
	failCreate  bool
	failUpdate  bool
	failRevoke  bool
	listCodes   []KeypadAccessCode
	logEvents   []SmartlockEvent
	createID    string
	createCode  string
}

func (f *fakeClient) ListKeypadCodes(ctx context.Context, cred Credentials) ([]KeypadAccessCode, error) {
	f.listCalls++
	if len(f.listCodes) > 0 {
		return f.listCodes, nil
	}
	return []KeypadAccessCode{
		{ExternalID: "1", Name: "A", AccessCodeMasked: "111111", Enabled: true, PayloadJSON: `{"id":"1"}`},
		{ExternalID: "2", Name: "B", AccessCodeMasked: "222222", Enabled: true, PayloadJSON: `{"id":"2"}`},
	}, nil
}

func (f *fakeClient) CreateAccess(ctx context.Context, cred Credentials, req UpsertAccessRequest) (*UpsertAccessResponse, error) {
	f.createCalls++
	if f.failCreate {
		return nil, sql.ErrConnDone
	}
	externalID := f.createID
	if externalID == "" {
		externalID = "ext-created"
	}
	code := f.createCode
	if code == "" {
		code = "123456"
	}
	return &UpsertAccessResponse{ExternalID: externalID, AccessCode: code}, nil
}

func (f *fakeClient) ListSmartlockEvents(ctx context.Context, cred Credentials, since time.Time, authID string) ([]SmartlockEvent, error) {
	f.logCalls++
	return f.logEvents, nil
}

func (f *fakeClient) UpdateAccess(ctx context.Context, cred Credentials, externalID string, req UpsertAccessRequest) (*UpsertAccessResponse, error) {
	f.updateCalls++
	if f.failUpdate {
		return nil, sql.ErrConnDone
	}
	return &UpsertAccessResponse{ExternalID: externalID, AccessCode: "654321"}, nil
}

func (f *fakeClient) SetAccessEnabled(ctx context.Context, cred Credentials, externalID string, payload map[string]interface{}) error {
	f.setCalls++
	return nil
}

func (f *fakeClient) RevokeAccess(ctx context.Context, cred Credentials, externalID string) error {
	f.revokeCalls++
	if f.failRevoke {
		return sql.ErrConnDone
	}
	return nil
}

func newTestStore(t *testing.T) *store.Store {
	return &store.Store{DB: testutil.OpenTestDB(t)}
}

func setupPropertyForNuki(t *testing.T, st *store.Store) int64 {
	t.Helper()
	ctx := context.Background()
	hash := testutil.FastPasswordHash(t, "secret123")
	u, err := st.CreateUser(ctx, "owner@nuki.test", hash, "owner")
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProperty(ctx, u.ID, "NukiTest", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	token := "tok-test"
	lock := "111222"
	if err := st.UpdatePropertySecrets(ctx, p.ID, nil, &token, &lock); err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func upsertNukiStay(t *testing.T, st *store.Store, pid int64, uid, status string, start, end time.Time) int64 {
	t.Helper()
	ctx := context.Background()
	stayStatus := store.NamedStayStatusActive
	if status == "cancelled" {
		stayStatus = store.NamedStayStatusCancelled
	} else if status == "deleted_from_source" {
		stayStatus = store.NamedStayStatusArchived
	}
	now := time.Now().UTC().Format(time.RFC3339)
	checkIn := start.UTC().Format("2006-01-02")
	checkOut := end.UTC().Format("2006-01-02")
	var stayID int64
	err := st.DB.QueryRowContext(ctx, `SELECT id FROM named_stays WHERE property_id = ? AND source_reference = ? LIMIT 1`, pid, uid).Scan(&stayID)
	if err == sql.ErrNoRows {
		res, insertErr := st.DB.ExecContext(ctx, `
			INSERT INTO named_stays (property_id, display_name, stay_type, check_in_date, check_out_date, status, cleaning_required, source_channel, source_reference, review_status, nuki_generation_status, created_at, updated_at)
			VALUES (?, ?, 'booking_com', ?, ?, ?, 1, 'booking_ics', ?, 'confirmed', 'pending', ?, ?)`,
			pid, "Guest "+uid, checkIn, checkOut, stayStatus, uid, now, now)
		if insertErr != nil {
			t.Fatal(insertErr)
		}
		stayID, err = res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
	} else if err == nil {
		if _, err := st.DB.ExecContext(ctx, `
			UPDATE named_stays
			SET check_in_date = ?, check_out_date = ?, status = ?, updated_at = ?
			WHERE property_id = ? AND id = ?`, checkIn, checkOut, stayStatus, now, pid, stayID); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Fatal(err)
	}
	return stayID
}

func TestGenerateCodes_CreatesAndUpdatesWithoutDuplicates(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{}
	svc := &Service{Store: st, Client: fc}
	now := time.Now().UTC().Add(48 * time.Hour)

	stayID := upsertNukiStay(t, st, pid, "uid-1", "active", now, now.Add(48*time.Hour))
	if err := svc.GenerateCodes(context.Background(), pid, "manual"); err != nil {
		t.Fatal(err)
	}
	if fc.createCalls != 1 {
		t.Fatalf("createCalls=%d want 1", fc.createCalls)
	}
	before, err := st.GetNukiCodeByNamedStayID(context.Background(), pid, stayID)
	if err != nil || before == nil {
		t.Fatalf("initial code err=%v code=%+v", err, before)
	}
	if err := svc.GenerateCodes(context.Background(), pid, "manual"); err != nil {
		t.Fatal(err)
	}
	after, err := st.GetNukiCodeByNamedStayID(context.Background(), pid, stayID)
	if err != nil || after == nil {
		t.Fatalf("idempotent code err=%v code=%+v", err, after)
	}
	if after.ID != before.ID || after.GeneratedPINPlain != before.GeneratedPINPlain || after.ExternalNukiID != before.ExternalNukiID ||
		!after.ValidFrom.Equal(before.ValidFrom) || !after.ValidUntil.Equal(before.ValidUntil) || after.Status != before.Status || after.RevokedAt != before.RevokedAt {
		t.Fatalf("idempotent generation changed code: before=%+v after=%+v", before, after)
	}
	// Change stay dates: update the existing code rather than creating a duplicate.
	upsertNukiStay(t, st, pid, "uid-1", "updated", now.Add(24*time.Hour), now.Add(72*time.Hour))
	if err := svc.GenerateCodes(context.Background(), pid, "manual"); err != nil {
		t.Fatal(err)
	}
	if fc.createCalls != 1 {
		t.Fatalf("createCalls=%d want still 1", fc.createCalls)
	}
	if fc.updateCalls < 1 {
		t.Fatalf("updateCalls=%d want >=1", fc.updateCalls)
	}
	rows, err := st.ListNukiCodes(context.Background(), pid, "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("codes=%d want 1", len(rows))
	}
}

func TestUpsertNukiCode_RequiresNamedStayID(t *testing.T) {
	st := newTestStore(t)
	err := st.UpsertNukiCode(context.Background(), &store.NukiAccessCode{
		PropertyID: 1,
		CodeLabel:  "Booking-No owner",
		ValidFrom:  time.Now().UTC(),
		ValidUntil: time.Now().UTC().Add(time.Hour),
		Status:     "not_generated",
	})
	if err == nil {
		t.Fatal("expected missing named_stay_id to be rejected")
	}
}

func TestGenerateCodes_NamedStayWithoutLegacyOccupancy(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{createID: "named-only-external", createCode: "987654"}
	svc := &Service{Store: st, Client: fc}
	start := time.Now().UTC().Add(48 * time.Hour).Format("2006-01-02")
	end := time.Now().UTC().Add(96 * time.Hour).Format("2006-01-02")
	stay, err := st.CreateNamedStayRecord(context.Background(), store.NamedStayCreateInput{
		PropertyID: pid, DisplayName: "Named Only", StayType: store.StayTypeBookingCom,
		CheckInDate: start, CheckOutDate: end, ReviewStatus: "confirmed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.GenerateCodeForNamedStay(context.Background(), pid, stay.ID, "test", "Named Only"); err != nil {
		t.Fatal(err)
	}
	code, err := st.GetNukiCodeByNamedStayID(context.Background(), pid, stay.ID)
	if err != nil {
		t.Fatal(err)
	}
	if code == nil || !code.NamedStayID.Valid || code.NamedStayID.Int64 != stay.ID {
		t.Fatalf("named-stay-primary code: %+v", code)
	}
	if code.GeneratedPINPlain.String != "987654" || code.ExternalNukiID.String != "named-only-external" {
		t.Fatalf("generated values not preserved: %+v", code)
	}
}

func TestReconcileNamedStay_UpdatesAndRevokesAcrossLifecycle(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{createID: "lifecycle-external", createCode: "987654"}
	svc := &Service{Store: st, Client: fc}
	start := time.Now().UTC().Add(48 * time.Hour).Format("2006-01-02")
	end := time.Now().UTC().Add(96 * time.Hour).Format("2006-01-02")
	stay, err := st.CreateNamedStayRecord(context.Background(), store.NamedStayCreateInput{
		PropertyID: pid, DisplayName: "Lifecycle", StayType: store.StayTypeBookingCom,
		CheckInDate: start, CheckOutDate: end, ReviewStatus: "confirmed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ReconcileNamedStay(context.Background(), pid, stay.ID, "create"); err != nil {
		t.Fatal(err)
	}
	newName := "Lifecycle Updated"
	newEnd := time.Now().UTC().Add(120 * time.Hour).Format("2006-01-02")
	if _, err := st.UpdateNamedStayRecord(context.Background(), pid, stay.ID, store.NamedStayUpdateInput{DisplayName: &newName, CheckOutDate: &newEnd}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReconcileNamedStay(context.Background(), pid, stay.ID, "patch"); err != nil {
		t.Fatal(err)
	}
	if fc.updateCalls != 1 {
		t.Fatalf("update calls=%d", fc.updateCalls)
	}
	code, err := st.GetNukiCodeByNamedStayID(context.Background(), pid, stay.ID)
	if err != nil {
		t.Fatal(err)
	}
	if code.CodeLabel != "Booking-Lifecycle Updated" {
		t.Fatalf("label=%q", code.CodeLabel)
	}
	if _, err := st.UpdateNamedStayStatus(context.Background(), pid, stay.ID, store.NamedStayStatusCancelled, 0); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReconcileNamedStay(context.Background(), pid, stay.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	code, _ = st.GetNukiCodeByNamedStayID(context.Background(), pid, stay.ID)
	if fc.revokeCalls != 1 || code.Status != "revoked" {
		t.Fatalf("revoke calls=%d code=%+v", fc.revokeCalls, code)
	}
	if _, err := st.UpdateNamedStayStatus(context.Background(), pid, stay.ID, store.NamedStayStatusActive, 0); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReconcileNamedStay(context.Background(), pid, stay.ID, "reactivate"); err != nil {
		t.Fatal(err)
	}
	if fc.createCalls != 2 {
		t.Fatalf("create calls after reactivation=%d", fc.createCalls)
	}
}

func TestReconcileNamedStay_UpdateFailurePreservesCredentialAndReturnsError(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{createID: "update-failure-external", createCode: "987654"}
	svc := &Service{Store: st, Client: fc}
	ctx := context.Background()
	start := time.Now().UTC().Add(48 * time.Hour).Format("2006-01-02")
	end := time.Now().UTC().Add(96 * time.Hour).Format("2006-01-02")
	stay, err := st.CreateNamedStayRecord(ctx, store.NamedStayCreateInput{
		PropertyID: pid, DisplayName: "Update Failure", StayType: store.StayTypeBookingCom,
		CheckInDate: start, CheckOutDate: end, ReviewStatus: "confirmed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ReconcileNamedStay(ctx, pid, stay.ID, "create"); err != nil {
		t.Fatal(err)
	}
	before, err := st.GetNukiCodeByNamedStayID(ctx, pid, stay.ID)
	if err != nil || before == nil {
		t.Fatalf("initial code err=%v code=%+v", err, before)
	}

	newName := "Update Failure Renamed"
	if _, err := st.UpdateNamedStayRecord(ctx, pid, stay.ID, store.NamedStayUpdateInput{DisplayName: &newName}); err != nil {
		t.Fatal(err)
	}
	fc.failUpdate = true
	if err := svc.ReconcileNamedStay(ctx, pid, stay.ID, "patch"); !errors.Is(err, sql.ErrConnDone) {
		t.Fatalf("reconcile error=%v want %v", err, sql.ErrConnDone)
	}

	after, err := st.GetNukiCodeByNamedStayID(ctx, pid, stay.ID)
	if err != nil || after == nil {
		t.Fatalf("failed code err=%v code=%+v", err, after)
	}
	if after.ID != before.ID || after.GeneratedPINPlain != before.GeneratedPINPlain || after.ExternalNukiID != before.ExternalNukiID {
		t.Fatalf("credential identity changed: before=%+v after=%+v", before, after)
	}
	if after.Status != "not_generated" || !after.ErrorMessage.Valid {
		t.Fatalf("failure state not persisted: %+v", after)
	}
	refreshed, err := st.GetNamedStay(ctx, pid, stay.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.NukiGenerationStatus.String != store.NukiGenerationError || !refreshed.NukiGenerationError.Valid {
		t.Fatalf("named stay failure state cleared: %+v", refreshed)
	}
}

func TestGenerateCodes_FailureMarksCodeNotGenerated(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{failCreate: true}
	svc := &Service{Store: st, Client: fc}
	now := time.Now().UTC().Add(48 * time.Hour)
	stayID := upsertNukiStay(t, st, pid, "uid-fail", "active", now, now.Add(24*time.Hour))

	if err := svc.GenerateCodes(context.Background(), pid, "manual"); err != nil {
		t.Fatal(err)
	}
	code, err := st.GetNukiCodeByNamedStayID(context.Background(), pid, stayID)
	if err != nil || code == nil {
		t.Fatalf("code err=%v nil=%v", err, code == nil)
	}
	if code.Status != "not_generated" {
		t.Fatalf("status=%s want not_generated", code.Status)
	}
}

func TestCleanupExpiredCodes_MovesToRevoked(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{}
	svc := &Service{Store: st, Client: fc}
	now := time.Now().UTC()
	stayID := upsertNukiStay(t, st, pid, "uid-exp", "active", now.Add(24*time.Hour), now.Add(72*time.Hour))
	if err := svc.GenerateCodes(context.Background(), pid, "manual"); err != nil {
		t.Fatal(err)
	}
	code, err := st.GetNukiCodeByNamedStayID(context.Background(), pid, stayID)
	if err != nil || code == nil {
		t.Fatalf("code err=%v nil=%v", err, code == nil)
	}
	code.ValidUntil = now.Add(-2 * time.Hour)
	code.Status = "generated"
	if err := st.UpsertNukiCode(context.Background(), code); err != nil {
		t.Fatal(err)
	}

	if err := svc.CleanupExpiredCodes(context.Background(), pid); err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListNukiCodes(context.Background(), pid, "historical")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("historical=%d want 1", len(rows))
	}
	if rows[0].Code.Status != "revoked" {
		t.Fatalf("status=%s want revoked", rows[0].Code.Status)
	}
}

func TestGenerateCodes_StatusTransition_NotGeneratedToGeneratedToRevoked(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{failCreate: true}
	svc := &Service{Store: st, Client: fc}
	now := time.Now().UTC().Add(48 * time.Hour)
	stayID := upsertNukiStay(t, st, pid, "uid-transition", "active", now, now.Add(24*time.Hour))

	if err := svc.GenerateCodes(context.Background(), pid, "manual"); err != nil {
		t.Fatal(err)
	}
	code, err := st.GetNukiCodeByNamedStayID(context.Background(), pid, stayID)
	if err != nil || code == nil {
		t.Fatalf("code err=%v nil=%v", err, code == nil)
	}
	if code.Status != "not_generated" {
		t.Fatalf("status=%s want not_generated", code.Status)
	}

	fc.failCreate = false
	if err := svc.GenerateCodes(context.Background(), pid, "manual"); err != nil {
		t.Fatal(err)
	}
	code, err = st.GetNukiCodeByNamedStayID(context.Background(), pid, stayID)
	if err != nil || code == nil {
		t.Fatalf("code err=%v nil=%v", err, code == nil)
	}
	if code.Status != "generated" {
		t.Fatalf("status=%s want generated", code.Status)
	}
	if !code.ExternalNukiID.Valid || code.ExternalNukiID.String == "" {
		t.Fatalf("external id missing after generation")
	}

	if err := svc.DeleteKeypadCode(context.Background(), pid, code.ExternalNukiID.String, "test"); err != nil {
		t.Fatal(err)
	}
	code, err = st.GetNukiCodeByNamedStayID(context.Background(), pid, stayID)
	if err != nil || code == nil {
		t.Fatalf("code err=%v nil=%v", err, code == nil)
	}
	if code.Status != "revoked" {
		t.Fatalf("status=%s want revoked", code.Status)
	}
}

func TestGenerateCodes_ReconcilesCancelledNamedStayByRevokingCode(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{}
	svc := &Service{Store: st, Client: fc}
	now := time.Now().UTC().Add(24 * time.Hour)
	upsertNukiStay(t, st, pid, "uid-can", "active", now, now.Add(48*time.Hour))
	if err := svc.GenerateCodes(context.Background(), pid, "manual"); err != nil {
		t.Fatal(err)
	}
	upsertNukiStay(t, st, pid, "uid-can", "cancelled", now, now.Add(48*time.Hour))
	if err := svc.GenerateCodes(context.Background(), pid, "manual"); err != nil {
		t.Fatal(err)
	}
	if fc.revokeCalls < 1 {
		t.Fatalf("revokeCalls=%d want >=1", fc.revokeCalls)
	}
	rows, err := st.ListNukiCodes(context.Background(), pid, "historical")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Code.Status != "revoked" {
		t.Fatalf("rows=%d status=%s", len(rows), func() string {
			if len(rows) == 0 {
				return ""
			}
			return rows[0].Code.Status
		}())
	}
}

func TestSyncProperty_FetchesAndStoresKeypadCodes(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{}
	svc := &Service{Store: st, Client: fc}
	if err := svc.SyncProperty(context.Background(), pid, "manual"); err != nil {
		t.Fatal(err)
	}
	if fc.listCalls != 1 {
		t.Fatalf("listCalls=%d want 1", fc.listCalls)
	}
	runs, err := st.ListNukiSyncRuns(context.Background(), pid, 1, 0)
	if err != nil || len(runs) == 0 {
		t.Fatalf("runs err=%v len=%d", err, len(runs))
	}
	if runs[0].ProcessedCount != 2 {
		t.Fatalf("processed=%d want 2", runs[0].ProcessedCount)
	}
	codes, err := st.ListNukiKeypadCodes(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 2 {
		t.Fatalf("codes len=%d want 2", len(codes))
	}
}

func TestDeleteKeypadCode_RemovesRemoteAndLocalEntry(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{}
	svc := &Service{Store: st, Client: fc}
	if err := st.UpsertNukiKeypadCode(context.Background(), &store.NukiKeypadCode{
		PropertyID:       pid,
		ExternalNukiID:   "to-delete",
		Name:             sql.NullString{String: "Delete me", Valid: true},
		AccessCodeMasked: sql.NullString{String: "***56", Valid: true},
		Enabled:          true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteKeypadCode(context.Background(), pid, "to-delete", "test"); err != nil {
		t.Fatal(err)
	}
	if fc.revokeCalls != 1 {
		t.Fatalf("revokeCalls=%d want 1", fc.revokeCalls)
	}
	list, err := st.ListNukiKeypadCodes(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("remaining=%d want 0", len(list))
	}
}

func TestSetKeypadCodeEnabled_UpdatesRemoteAndLocalState(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{}
	svc := &Service{Store: st, Client: fc}
	if err := st.UpsertNukiKeypadCode(context.Background(), &store.NukiKeypadCode{
		PropertyID:       pid,
		ExternalNukiID:   "toggle-me",
		Name:             sql.NullString{String: "Toggle me", Valid: true},
		AccessCodeMasked: sql.NullString{String: "***12", Valid: true},
		Enabled:          true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetKeypadCodeEnabled(context.Background(), pid, "toggle-me", false, "test"); err != nil {
		t.Fatal(err)
	}
	if fc.setCalls != 1 {
		t.Fatalf("setCalls=%d want 1", fc.setCalls)
	}
	rows, err := st.ListNukiKeypadCodes(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1", len(rows))
	}
	if rows[0].Enabled {
		t.Fatalf("enabled=true want false")
	}
}

func TestSyncRuns_ArePrunedToRetentionLimit(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{}
	svc := &Service{Store: st, Client: fc}

	for i := 0; i < syncRunRetention+7; i++ {
		if err := svc.SyncProperty(context.Background(), pid, "manual"); err != nil {
			t.Fatal(err)
		}
	}
	runs, err := st.ListNukiSyncRuns(context.Background(), pid, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != syncRunRetention {
		t.Fatalf("runs=%d want %d", len(runs), syncRunRetention)
	}
}

func TestGenerateCodeForNamedStay_UsesBookingPrefixLabel(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{}
	svc := &Service{Store: st, Client: fc}
	now := time.Now().UTC().Add(48 * time.Hour)
	stayID := upsertNukiStay(t, st, pid, "uid-prefix", "active", now, now.Add(24*time.Hour))

	if err := svc.GenerateCodeForNamedStay(context.Background(), pid, stayID, "manual", "Martina Novak"); err != nil {
		t.Fatal(err)
	}
	code, err := st.GetNukiCodeByNamedStayID(context.Background(), pid, stayID)
	if err != nil || code == nil {
		t.Fatalf("code err=%v nil=%v", err, code == nil)
	}
	if code.CodeLabel != "Booking-Martina Novak" {
		t.Fatalf("label=%q want %q", code.CodeLabel, "Booking-Martina Novak")
	}
}

func TestSyncProperty_PMSLinkSurvivesExternalIDDiff(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{createID: "created-id", createCode: "654321"}
	svc := &Service{Store: st, Client: fc}
	now := time.Now().UTC().Add(48 * time.Hour)
	stayID := upsertNukiStay(t, st, pid, "uid-link", "active", now, now.Add(24*time.Hour))

	if err := svc.GenerateCodeForNamedStay(context.Background(), pid, stayID, "manual", "Link Guest"); err != nil {
		t.Fatal(err)
	}
	code, err := st.GetNukiCodeByNamedStayID(context.Background(), pid, stayID)
	if err != nil || code == nil {
		t.Fatalf("code err=%v nil=%v", err, code == nil)
	}
	fc.listCodes = []KeypadAccessCode{
		{
			ExternalID:       "listed-id",
			Name:             "Booking-Link Guest",
			AccessCodeMasked: "654321",
			Enabled:          true,
			ValidFrom:        timePtr(code.ValidFrom.Add(-45 * time.Minute)),
			ValidUntil:       timePtr(code.ValidUntil.Add(-45 * time.Minute)),
			PayloadJSON:      `{"id":"listed-id"}`,
		},
	}

	if err := svc.SyncProperty(context.Background(), pid, "manual"); err != nil {
		t.Fatal(err)
	}
	updatedCode, err := st.GetNukiCodeByNamedStayID(context.Background(), pid, stayID)
	if err != nil || updatedCode == nil {
		t.Fatalf("updated code err=%v nil=%v", err, updatedCode == nil)
	}
	if !updatedCode.ExternalNukiID.Valid || updatedCode.ExternalNukiID.String != "listed-id" {
		t.Fatalf("external=%v want listed-id", updatedCode.ExternalNukiID)
	}
	rows, err := st.ListNukiKeypadCodes(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1", len(rows))
	}
	if !rows[0].PMSLinked {
		t.Fatalf("expected keypad row to be PMS-linked")
	}
}

func TestSyncProperty_AfterGenerateRefreshDoesNotRevokeFreshCode(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{createID: "created-id", createCode: "456789"}
	svc := &Service{Store: st, Client: fc}
	now := time.Now().UTC().Add(48 * time.Hour)
	stayID := upsertNukiStay(t, st, pid, "uid-refresh", "active", now, now.Add(24*time.Hour))
	if err := svc.GenerateCodeForNamedStay(context.Background(), pid, stayID, "manual", "Maros"); err != nil {
		t.Fatal(err)
	}

	// Simulate listing drift: refresh does not include the just-created code yet.
	fc.listCodes = []KeypadAccessCode{
		{
			ExternalID:       "other-id",
			Name:             "Booking-Someone Else",
			AccessCodeMasked: "111111",
			Enabled:          true,
			PayloadJSON:      `{"id":"other-id"}`,
		},
	}
	if err := svc.SyncProperty(context.Background(), pid, "after_generate_refresh"); err != nil {
		t.Fatal(err)
	}
	code, err := st.GetNukiCodeByNamedStayID(context.Background(), pid, stayID)
	if err != nil || code == nil {
		t.Fatalf("code err=%v nil=%v", err, code == nil)
	}
	if code.Status != "generated" {
		t.Fatalf("status=%s want generated", code.Status)
	}
	if !code.ExternalNukiID.Valid || code.ExternalNukiID.String != "created-id" {
		t.Fatalf("external=%v want created-id", code.ExternalNukiID)
	}
}

func TestListNukiKeypadCodes_NonPMSRemainsUnlinked(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	if err := st.UpsertNukiKeypadCode(context.Background(), &store.NukiKeypadCode{
		PropertyID:       pid,
		ExternalNukiID:   "external-only",
		Name:             sql.NullString{String: "Booking-External Guest", Valid: true},
		AccessCodeMasked: sql.NullString{String: "***99", Valid: true},
		Enabled:          true,
		ValidFrom:        sql.NullTime{Time: time.Now().UTC().Add(24 * time.Hour), Valid: true},
		ValidUntil:       sql.NullTime{Time: time.Now().UTC().Add(48 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListNukiKeypadCodes(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1", len(rows))
	}
	if rows[0].PMSLinked {
		t.Fatalf("expected non-PMS row to stay unlinked")
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func TestReconcileCleanerDailyLogs_UsesFirstEntryPerDayForCleaner(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	// Enable cleaner auth id in property profile.
	if err := st.UpdatePropertyProfile(context.Background(), pid, map[string]interface{}{"cleaner_nuki_auth_id": "cleaner-1"}); err != nil {
		t.Fatal(err)
	}
	// Anchor `now` at noon UTC so the test stays day-stable when run in
	// the early UTC morning. Otherwise `now.Add(-6h)` straddles midnight
	// and ends up indexed under yesterday's day, while the assertion
	// below still looks under today's.
	today := time.Now().UTC()
	now := time.Date(today.Year(), today.Month(), today.Day(), 12, 0, 0, 0, time.UTC)
	fc := &fakeClient{
		logEvents: []SmartlockEvent{
			{ExternalID: "e1", OccurredAt: now.Add(-6 * time.Hour), AuthID: "cleaner-1", IsEntryLike: true},
			{ExternalID: "e2", OccurredAt: now.Add(-2 * time.Hour), AuthID: "cleaner-1", IsEntryLike: true}, // same day, later
			{ExternalID: "e3", OccurredAt: now.Add(-26 * time.Hour), AuthID: "cleaner-1", IsEntryLike: true},
			{ExternalID: "e4", OccurredAt: now.Add(-5 * time.Hour), AuthID: "other-user", IsEntryLike: true}, // ignored auth
			{ExternalID: "e5", OccurredAt: now.Add(-4 * time.Hour), AuthID: "cleaner-1", IsEntryLike: false}, // ignored non-entry
		},
	}
	svc := &Service{Store: st, Client: fc}
	if _, err := svc.ReconcileCleanerDailyLogs(context.Background(), pid); err != nil {
		t.Fatal(err)
	}
	month := now.Format("2006-01")
	logs, err := st.ListCleaningDailyLogsForMonth(context.Background(), pid, month)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) == 0 {
		t.Fatalf("expected cleaning logs to be created")
	}
	day := now.Format("2006-01-02")
	var dayLog *store.CleaningDailyLog
	for i := range logs {
		if logs[i].DayDate == day {
			dayLog = &logs[i]
			break
		}
	}
	if dayLog == nil {
		t.Fatalf("expected log for day %s", day)
	}
	if !dayLog.FirstEntryAt.Valid {
		t.Fatalf("expected first entry timestamp")
	}
	// earliest entry for same day should win (e1 before e2)
	want := now.Add(-6 * time.Hour).UTC().Truncate(time.Second)
	got := dayLog.FirstEntryAt.Time.UTC().Truncate(time.Second)
	if !got.Equal(want) {
		t.Fatalf("first entry=%s want %s", got, want)
	}
}

func TestReconcileCleanerDailyLogsSince_MatchesAliasFromExternalID(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	// Set configured value to external_nuki_id style identifier.
	if err := st.UpdatePropertyProfile(context.Background(), pid, map[string]interface{}{"cleaner_nuki_auth_id": "ext-cleaner-1"}); err != nil {
		t.Fatal(err)
	}
	// Cache a keypad row mapping external id -> accountUserId used in event logs.
	if err := st.UpsertNukiKeypadCode(context.Background(), &store.NukiKeypadCode{
		PropertyID:     pid,
		ExternalNukiID: "ext-cleaner-1",
		RawJSON:        sql.NullString{String: `{"id":"ext-cleaner-1","accountUserId":"1387687933","authId":"1387687933"}`, Valid: true},
		Enabled:        true,
	}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().AddDate(0, 0, -70)
	fc := &fakeClient{
		logEvents: []SmartlockEvent{
			{ExternalID: "old-e1", OccurredAt: old, AuthID: "1387687933", IsEntryLike: true},
		},
	}
	svc := &Service{Store: st, Client: fc}
	if _, err := svc.ReconcileCleanerDailyLogsSince(context.Background(), pid, old.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	month := old.Format("2006-01")
	logs, err := st.ListCleaningDailyLogsForMonth(context.Background(), pid, month)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) == 0 {
		t.Fatalf("expected historical log to be created via alias matching")
	}
}

func TestReconcileGuestDailyEntries_PartitionsCleanerVsGuest(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	ctx := context.Background()

	// Cleaner alias is configured.
	if err := st.UpdatePropertyProfile(ctx, pid, map[string]interface{}{"cleaner_nuki_auth_id": "cleaner-1"}); err != nil {
		t.Fatal(err)
	}

	// Two stays mapped to two distinct guest authIDs.
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC)
	stayA := upsertNukiStay(t, st, pid, "uid-A", "active", today.Add(-48*time.Hour), today.Add(24*time.Hour))
	stayB := upsertNukiStay(t, st, pid, "uid-B", "active", today.Add(-24*time.Hour), today.Add(48*time.Hour))

	// Guest access codes map each authID directly to a named stay.
	if err := st.UpsertNukiCode(ctx, &store.NukiAccessCode{
		PropertyID:     pid,
		NamedStayID:    sql.NullInt64{Int64: stayA, Valid: true},
		CodeLabel:      "uid-A",
		ExternalNukiID: sql.NullString{String: "guest-A", Valid: true},
		ValidFrom:      today.Add(-72 * time.Hour),
		ValidUntil:     today.Add(72 * time.Hour),
		Status:         "generated",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertNukiCode(ctx, &store.NukiAccessCode{
		PropertyID:     pid,
		NamedStayID:    sql.NullInt64{Int64: stayB, Valid: true},
		CodeLabel:      "uid-B",
		ExternalNukiID: sql.NullString{String: "guest-B", Valid: true},
		ValidFrom:      today.Add(-72 * time.Hour),
		ValidUntil:     today.Add(72 * time.Hour),
		Status:         "generated",
	}); err != nil {
		t.Fatal(err)
	}

	fc := &fakeClient{
		logEvents: []SmartlockEvent{
			// Guest A: two unlocks same day, the earlier wins.
			{ExternalID: "ga-1", OccurredAt: today.Add(-3 * time.Hour), AuthID: "guest-A", IsEntryLike: true},
			{ExternalID: "ga-2", OccurredAt: today.Add(-1 * time.Hour), AuthID: "guest-A", IsEntryLike: true},
			// Guest B: one unlock today.
			{ExternalID: "gb-1", OccurredAt: today.Add(-2 * time.Hour), AuthID: "guest-B", IsEntryLike: true},
			// Cleaner unlock: must be excluded even if mapped via aliases.
			{ExternalID: "cl-1", OccurredAt: today.Add(-4 * time.Hour), AuthID: "cleaner-1", IsEntryLike: true},
			// Unknown authID: ignored.
			{ExternalID: "ux-1", OccurredAt: today.Add(-5 * time.Hour), AuthID: "stranger", IsEntryLike: true},
			// Non-entry-like guest event: still counted via fallback only when no entry-like exist.
			{ExternalID: "ga-3", OccurredAt: today.Add(-30 * time.Minute), AuthID: "guest-A", IsEntryLike: false},
		},
	}
	svc := &Service{Store: st, Client: fc}
	stats, err := svc.ReconcileGuestDailyEntries(ctx, pid)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if stats.NamedStayKeyCount != 2 {
		t.Fatalf("NamedStayKeyCount=%d want 2", stats.NamedStayKeyCount)
	}
	if stats.UpsertedDays != 2 {
		t.Fatalf("UpsertedDays=%d want 2 (one per stay)", stats.UpsertedDays)
	}
	if stats.CleanerSkipped == 0 {
		t.Fatalf("CleanerSkipped=0 want >0 (cleaner unlock must be filtered)")
	}

	day := today.Format("2006-01-02")
	rows, err := st.ListNukiGuestDailyEntriesInRange(ctx, pid, day, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d want 2", len(rows))
	}
	byStay := map[int64]store.NukiGuestDailyEntry{}
	for _, r := range rows {
		byStay[r.NamedStayID.Int64] = r
	}
	if got := byStay[stayA].FirstEntryAt.UTC().Truncate(time.Second); !got.Equal(today.Add(-3 * time.Hour).UTC().Truncate(time.Second)) {
		t.Fatalf("guest A first entry=%s want earlier of two unlocks", got)
	}
	if got := byStay[stayB].FirstEntryAt.UTC().Truncate(time.Second); !got.Equal(today.Add(-2 * time.Hour).UTC().Truncate(time.Second)) {
		t.Fatalf("guest B first entry=%s", got)
	}
}

func TestReconcileGuestDailyEntries_NoNamedStayCodesNoOp(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{logEvents: []SmartlockEvent{
		{ExternalID: "x", OccurredAt: time.Now().UTC(), AuthID: "any", IsEntryLike: true},
	}}
	svc := &Service{Store: st, Client: fc}
	stats, err := svc.ReconcileGuestDailyEntries(context.Background(), pid)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if stats.UpsertedDays != 0 || stats.NamedStayKeyCount != 0 {
		t.Fatalf("expected no-op, got stats=%+v", stats)
	}
}
