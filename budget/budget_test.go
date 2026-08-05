package budget

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func at(t *Tracker, ts time.Time) {
	t.now = func() time.Time { return ts }
}

func TestCheck_NoLimitAllowsEverything(t *testing.T) {
	tr := New(0, 0)
	tr.Record(1000)

	if err := tr.Check(); err != nil {
		t.Errorf("unlimited tracker refused a call: %v", err)
	}
}

func TestCheck_NilTrackerAllowsEverything(t *testing.T) {
	var tr *Tracker

	if err := tr.Check(); err != nil {
		t.Errorf("nil tracker refused a call: %v", err)
	}
	tr.Record(5) // must not panic
	if s := tr.Snapshot(); s.Enabled() {
		t.Errorf("nil tracker reported an enabled budget: %+v", s)
	}
}

func TestCheck_DailyLimit(t *testing.T) {
	tr := New(0.10, 0)

	tr.Record(0.09)
	if err := tr.Check(); err != nil {
		t.Fatalf("call under the daily limit was refused: %v", err)
	}

	tr.Record(0.02)

	var limitErr *LimitError
	if err := tr.Check(); !errors.As(err, &limitErr) {
		t.Fatalf("expected *LimitError once the daily limit was reached, got %v", err)
	}
	if limitErr.Window != WindowDaily {
		t.Errorf("expected window %q, got %q", WindowDaily, limitErr.Window)
	}
	if limitErr.Limit != 0.10 {
		t.Errorf("expected limit 0.10 in the error, got %v", limitErr.Limit)
	}
	if limitErr.Spent < 0.10 {
		t.Errorf("expected spend of at least the limit, got %v", limitErr.Spent)
	}
}

func TestCheck_MonthlyLimitWithoutDailyLimit(t *testing.T) {
	tr := New(0, 1.0)
	tr.Record(1.5)

	var limitErr *LimitError
	if err := tr.Check(); !errors.As(err, &limitErr) {
		t.Fatalf("expected *LimitError for the monthly window, got %v", err)
	}
	if limitErr.Window != WindowMonthly {
		t.Errorf("expected window %q, got %q", WindowMonthly, limitErr.Window)
	}
}

func TestRollover_NewDayClearsDailyButKeepsMonthly(t *testing.T) {
	tr := New(0.10, 5.0)
	at(tr, time.Date(2026, 8, 6, 23, 0, 0, 0, time.Local))
	tr.Record(0.20)

	if tr.Check() == nil {
		t.Fatal("expected the daily limit to be reached before rollover")
	}

	at(tr, time.Date(2026, 8, 7, 1, 0, 0, 0, time.Local))
	if err := tr.Check(); err != nil {
		t.Errorf("a new day must clear the daily counter, got %v", err)
	}

	s := tr.Snapshot()
	if s.DailySpent != 0 {
		t.Errorf("expected daily spend to reset, got %v", s.DailySpent)
	}
	if s.MonthlySpent != 0.20 {
		t.Errorf("expected monthly spend to survive a day change, got %v", s.MonthlySpent)
	}
}

func TestRollover_NewMonthClearsBoth(t *testing.T) {
	tr := New(0, 1.0)
	at(tr, time.Date(2026, 8, 31, 12, 0, 0, 0, time.Local))
	tr.Record(2.0)

	if tr.Check() == nil {
		t.Fatal("expected the monthly limit to be reached before rollover")
	}

	at(tr, time.Date(2026, 9, 1, 0, 30, 0, 0, time.Local))
	if err := tr.Check(); err != nil {
		t.Errorf("a new month must clear the monthly counter, got %v", err)
	}
	if s := tr.Snapshot(); s.MonthlySpent != 0 || s.DailySpent != 0 {
		t.Errorf("expected both counters to reset, got %+v", s)
	}
}

func TestRecord_IgnoresNonPositiveCost(t *testing.T) {
	tr := New(1.0, 1.0)
	tr.Record(0)
	tr.Record(-5)

	if s := tr.Snapshot(); s.DailySpent != 0 || s.MonthlySpent != 0 {
		t.Errorf("expected non-positive costs to be ignored, got %+v", s)
	}
}

func TestStatus_NearLimit(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{"no limit configured", Status{DailySpent: 100}, false},
		{"well under the ceiling", Status{DailySpent: 0.5, DailyLimit: 1}, false},
		{"exactly at the warn ratio", Status{DailySpent: 0.8, DailyLimit: 1}, true},
		{"monthly window warns too", Status{MonthlySpent: 9, MonthlyLimit: 10}, true},
		{"daily quiet, monthly loud", Status{DailySpent: 0.1, DailyLimit: 1, MonthlySpent: 9.9, MonthlyLimit: 10}, true},
	}

	for _, tt := range tests {
		if got := tt.status.NearLimit(); got != tt.want {
			t.Errorf("%s: NearLimit() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestConcurrentRecordAndCheck(t *testing.T) {
	tr := New(0, 0)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.Record(0.01)
			tr.Check()
			tr.Snapshot()
		}()
	}
	wg.Wait()

	if s := tr.Snapshot(); s.DailySpent < 0.49 || s.DailySpent > 0.51 {
		t.Errorf("expected ~0.50 recorded, got %v", s.DailySpent)
	}
}

func TestSeed_RestoresSpendAcrossRestart(t *testing.T) {
	tr := New(0.10, 5.0)
	at(tr, time.Date(2026, 8, 6, 12, 0, 0, 0, time.Local))
	tr.Seed(0.12, 0.30)

	if tr.Check() == nil {
		t.Error("a seeded daily spend above the limit must refuse the next paid call")
	}
	s := tr.Snapshot()
	if s.DailySpent != 0.12 || s.MonthlySpent != 0.30 {
		t.Errorf("Snapshot after Seed = %+v, want daily 0.12 / monthly 0.30", s)
	}
}

func TestSeed_SurvivesTheFirstRollover(t *testing.T) {
	// Seed runs before any other call, so it is the first thing to stamp the
	// window keys. If it did not, the next Check would treat the seeded value as
	// belonging to a stale window and silently hand back a full allowance.
	tr := New(0.10, 0)
	at(tr, time.Date(2026, 8, 6, 12, 0, 0, 0, time.Local))
	tr.Seed(0.50, 0)

	if err := tr.Check(); err == nil {
		t.Fatal("seeded spend was cleared by the first rollover")
	}
	if s := tr.Snapshot(); s.DailySpent != 0.50 {
		t.Errorf("daily spend = %v after Seed, want 0.50", s.DailySpent)
	}
}

func TestSeed_OverwritesRatherThanAccumulates(t *testing.T) {
	tr := New(1.0, 1.0)
	at(tr, time.Date(2026, 8, 6, 12, 0, 0, 0, time.Local))
	tr.Seed(0.20, 0.20)
	tr.Seed(0.20, 0.20)

	if s := tr.Snapshot(); s.DailySpent != 0.20 {
		t.Errorf("seeding twice gave %v, want 0.20 — Seed must replace, not add", s.DailySpent)
	}
}

func TestSeed_StillExpiresOnAWindowChange(t *testing.T) {
	tr := New(0.10, 5.0)
	at(tr, time.Date(2026, 8, 6, 23, 0, 0, 0, time.Local))
	tr.Seed(0.50, 0.50)

	at(tr, time.Date(2026, 8, 7, 1, 0, 0, 0, time.Local))
	if err := tr.Check(); err != nil {
		t.Errorf("seeded spend must still expire when the day rolls over, got %v", err)
	}
	if s := tr.Snapshot(); s.MonthlySpent != 0.50 {
		t.Errorf("monthly seeded spend = %v, want it to survive a day change", s.MonthlySpent)
	}
}

func TestSeed_NilTrackerDoesNotPanic(t *testing.T) {
	var tr *Tracker
	tr.Seed(1.0, 1.0)
}

func TestDayAndMonthKeys(t *testing.T) {
	at := time.Date(2026, 8, 6, 23, 59, 0, 0, time.Local)
	if got := DayKey(at); got != "2026-08-06" {
		t.Errorf("DayKey = %q, want 2026-08-06", got)
	}
	if got := MonthKey(at); got != "2026-08" {
		t.Errorf("MonthKey = %q, want 2026-08", got)
	}
	if DayKey(at) == DayKey(at.AddDate(0, 0, 1)) {
		t.Error("DayKey must differ across a day boundary")
	}
	if MonthKey(at) == MonthKey(at.AddDate(0, 1, 0)) {
		t.Error("MonthKey must differ across a month boundary")
	}
}
