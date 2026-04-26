package nuki

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"pms/backend/internal/auth"
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
	return &UpsertAccessResponse{ExternalID: externalID, AccessCode: "654321"}, nil
}

func (f *fakeClient) SetAccessEnabled(ctx context.Context, cred Credentials, externalID string, payload map[string]interface{}) error {
	f.setCalls++
	return nil
}

func (f *fakeClient) RevokeAccess(ctx context.Context, cred Credentials, externalID string) error {
	f.revokeCalls++
	return nil
}

func newTestStore(t *testing.T) *store.Store {
	return &store.Store{DB: testutil.OpenTestDB(t)}
}

func setupPropertyForNuki(t *testing.T, st *store.Store) int64 {
	t.Helper()
	ctx := context.Background()
	hash, _ := auth.HashPassword("secret123")
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

func upsertOcc(t *testing.T, st *store.Store, pid int64, uid, status string, start, end time.Time) {
	t.Helper()
	runID, err := st.StartOccupancySyncRun(context.Background(), pid, "test")
	if err != nil {
		t.Fatal(err)
	}
	err = st.UpsertOccupancy(context.Background(), &store.Occupancy{
		PropertyID:     pid,
		SourceType:     "ics_booking",
		SourceEventUID: uid,
		StartAt:        start,
		EndAt:          end,
		Status:         status,
		ContentHash:    uid + "-" + status,
	}, runID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGenerateCodes_CreatesAndUpdatesWithoutDuplicates(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{}
	svc := &Service{Store: st, Client: fc}
	now := time.Now().UTC().Add(48 * time.Hour)

	upsertOcc(t, st, pid, "uid-1", "active", now, now.Add(48*time.Hour))
	if err := svc.GenerateCodes(context.Background(), pid, "manual"); err != nil {
		t.Fatal(err)
	}
	if fc.createCalls != 1 {
		t.Fatalf("createCalls=%d want 1", fc.createCalls)
	}
	// Change occupancy dates => update call, not second create.
	upsertOcc(t, st, pid, "uid-1", "updated", now.Add(24*time.Hour), now.Add(72*time.Hour))
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

func TestGenerateCodes_FailureMarksCodeNotGenerated(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{failCreate: true}
	svc := &Service{Store: st, Client: fc}
	now := time.Now().UTC().Add(48 * time.Hour)
	upsertOcc(t, st, pid, "uid-fail", "active", now, now.Add(24*time.Hour))

	if err := svc.GenerateCodes(context.Background(), pid, "manual"); err != nil {
		t.Fatal(err)
	}
	occ, err := st.GetOccupancyBySourceEventUID(context.Background(), pid, "uid-fail")
	if err != nil || occ == nil {
		t.Fatalf("occ err=%v nil=%v", err, occ == nil)
	}
	code, err := st.GetNukiCodeByOccupancyID(context.Background(), pid, occ.ID)
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
	upsertOcc(t, st, pid, "uid-exp", "active", now.Add(24*time.Hour), now.Add(72*time.Hour))
	if err := svc.GenerateCodes(context.Background(), pid, "manual"); err != nil {
		t.Fatal(err)
	}
	occ, err := st.GetOccupancyBySourceEventUID(context.Background(), pid, "uid-exp")
	if err != nil || occ == nil {
		t.Fatalf("occ err=%v nil=%v", err, occ == nil)
	}
	code, err := st.GetNukiCodeByOccupancyID(context.Background(), pid, occ.ID)
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
	upsertOcc(t, st, pid, "uid-transition", "active", now, now.Add(24*time.Hour))

	if err := svc.GenerateCodes(context.Background(), pid, "manual"); err != nil {
		t.Fatal(err)
	}
	occ, err := st.GetOccupancyBySourceEventUID(context.Background(), pid, "uid-transition")
	if err != nil || occ == nil {
		t.Fatalf("occ err=%v nil=%v", err, occ == nil)
	}
	code, err := st.GetNukiCodeByOccupancyID(context.Background(), pid, occ.ID)
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
	code, err = st.GetNukiCodeByOccupancyID(context.Background(), pid, occ.ID)
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
	code, err = st.GetNukiCodeByOccupancyID(context.Background(), pid, occ.ID)
	if err != nil || code == nil {
		t.Fatalf("code err=%v nil=%v", err, code == nil)
	}
	if code.Status != "revoked" {
		t.Fatalf("status=%s want revoked", code.Status)
	}
}

func TestGenerateCodes_ReconcilesCancelledOccupancyByRevokingCode(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{}
	svc := &Service{Store: st, Client: fc}
	now := time.Now().UTC().Add(24 * time.Hour)
	upsertOcc(t, st, pid, "uid-can", "active", now, now.Add(48*time.Hour))
	if err := svc.GenerateCodes(context.Background(), pid, "manual"); err != nil {
		t.Fatal(err)
	}
	upsertOcc(t, st, pid, "uid-can", "cancelled", now, now.Add(48*time.Hour))
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

func TestGenerateCodeForOccupancy_UsesBookingPrefixLabel(t *testing.T) {
	st := newTestStore(t)
	pid := setupPropertyForNuki(t, st)
	fc := &fakeClient{}
	svc := &Service{Store: st, Client: fc}
	now := time.Now().UTC().Add(48 * time.Hour)
	upsertOcc(t, st, pid, "uid-prefix", "active", now, now.Add(24*time.Hour))
	occ, err := st.GetOccupancyBySourceEventUID(context.Background(), pid, "uid-prefix")
	if err != nil || occ == nil {
		t.Fatalf("occ err=%v nil=%v", err, occ == nil)
	}

	if err := svc.GenerateCodeForOccupancy(context.Background(), pid, occ.ID, "manual", "Martina Novak"); err != nil {
		t.Fatal(err)
	}
	code, err := st.GetNukiCodeByOccupancyID(context.Background(), pid, occ.ID)
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
	upsertOcc(t, st, pid, "uid-link", "active", now, now.Add(24*time.Hour))
	occ, err := st.GetOccupancyBySourceEventUID(context.Background(), pid, "uid-link")
	if err != nil || occ == nil {
		t.Fatalf("occ err=%v nil=%v", err, occ == nil)
	}

	if err := svc.GenerateCodeForOccupancy(context.Background(), pid, occ.ID, "manual", "Link Guest"); err != nil {
		t.Fatal(err)
	}
	code, err := st.GetNukiCodeByOccupancyID(context.Background(), pid, occ.ID)
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
	updatedCode, err := st.GetNukiCodeByOccupancyID(context.Background(), pid, occ.ID)
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
	upsertOcc(t, st, pid, "uid-refresh", "active", now, now.Add(24*time.Hour))
	occ, err := st.GetOccupancyBySourceEventUID(context.Background(), pid, "uid-refresh")
	if err != nil || occ == nil {
		t.Fatalf("occ err=%v nil=%v", err, occ == nil)
	}
	if err := svc.GenerateCodeForOccupancy(context.Background(), pid, occ.ID, "manual", "Maros"); err != nil {
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
	code, err := st.GetNukiCodeByOccupancyID(context.Background(), pid, occ.ID)
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
