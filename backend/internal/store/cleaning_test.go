package store

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestComputeCleaningMonthlySummary_AppliesFeeHistoryAndAdjustments(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, "owner-cleaning@test.local", "hash", "owner")
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProperty(ctx, u.ID, "P-cleaning", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}

	if err := st.CreateCleanerFeeHistoryRow(ctx, p.ID, 1000, 200, time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), &u.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateCleanerFeeHistoryRow(ctx, p.ID, 1200, 300, time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC), &u.ID); err != nil {
		t.Fatal(err)
	}

	if err := st.UpsertCleaningDailyLog(ctx, &CleaningDailyLog{
		PropertyID:       p.ID,
		DayDate:          "2026-04-10",
		FirstEntryAt:     sql.NullTime{Time: time.Date(2026, 4, 10, 9, 5, 0, 0, time.UTC), Valid: true},
		CountedForSalary: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCleaningDailyLog(ctx, &CleaningDailyLog{
		PropertyID:       p.ID,
		DayDate:          "2026-04-20",
		FirstEntryAt:     sql.NullTime{Time: time.Date(2026, 4, 20, 8, 45, 0, 0, time.UTC), Valid: true},
		CountedForSalary: true,
	}); err != nil {
		t.Fatal(err)
	}
	// Should not count.
	if err := st.UpsertCleaningDailyLog(ctx, &CleaningDailyLog{
		PropertyID:       p.ID,
		DayDate:          "2026-04-21",
		FirstEntryAt:     sql.NullTime{Time: time.Date(2026, 4, 21, 10, 15, 0, 0, time.UTC), Valid: true},
		CountedForSalary: false,
	}); err != nil {
		t.Fatal(err)
	}

	if err := st.CreateCleaningAdjustment(ctx, p.ID, 2026, 4, 500, "bonus", &u.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateCleaningAdjustment(ctx, p.ID, 2026, 4, -200, "deduction", &u.ID); err != nil {
		t.Fatal(err)
	}

	summary, err := st.ComputeCleaningMonthlySummary(ctx, p.ID, 2026, 4, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if summary.CountedDays != 2 {
		t.Fatalf("countedDays=%d want 2", summary.CountedDays)
	}
	// 10th => 1200, 20th => 1500, base=2700
	if summary.BaseSalaryCents != 2700 {
		t.Fatalf("base=%d want 2700", summary.BaseSalaryCents)
	}
	if summary.AdjustmentsTotalCents != 300 {
		t.Fatalf("adjustments=%d want 300", summary.AdjustmentsTotalCents)
	}
	if summary.FinalSalaryCents != 3000 {
		t.Fatalf("final=%d want 3000", summary.FinalSalaryCents)
	}
}
