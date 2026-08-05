// Package budget caps what the bot is allowed to spend on paid provider calls.
//
// Counters live in memory, but they are not the record: Seed reloads them at
// startup from the costs the history layer already persists, so a restart no
// longer hands the user a fresh allowance. This package stays free of any
// storage dependency — the caller does the reloading.
package budget

import (
	"fmt"
	"sync"
	"time"
)

const (
	WindowDaily   = "daily"
	WindowMonthly = "monthly"

	// warnRatio is the share of a limit past which the usage summary starts
	// warning. Warning from the first cent would make the notice noise.
	warnRatio = 0.8
)

// LimitError reports a paid call refused because a ceiling was reached. It
// carries the numbers so the caller can explain the refusal in its own wording
// and language.
type LimitError struct {
	Window string
	Spent  float64
	Limit  float64
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("%s spending limit reached: $%.5f of $%.5f", e.Window, e.Spent, e.Limit)
}

// Status is a point-in-time view of both windows. A zero limit means that
// window is unlimited.
type Status struct {
	DailySpent   float64
	DailyLimit   float64
	MonthlySpent float64
	MonthlyLimit float64
}

// Enabled reports whether any ceiling is configured at all.
func (s Status) Enabled() bool {
	return s.DailyLimit > 0 || s.MonthlyLimit > 0
}

// NearLimit reports whether a configured window has passed warnRatio of its
// ceiling without reaching it yet.
func (s Status) NearLimit() bool {
	if s.DailyLimit > 0 && s.DailySpent >= s.DailyLimit*warnRatio {
		return true
	}
	return s.MonthlyLimit > 0 && s.MonthlySpent >= s.MonthlyLimit*warnRatio
}

type Tracker struct {
	mu sync.Mutex

	dailyLimit   float64
	monthlyLimit float64

	daily   float64
	monthly float64

	dayKey   string
	monthKey string

	// now is swappable so window rollover can be tested without waiting for
	// midnight or the first of the month.
	now func() time.Time
}

// New builds a tracker. A limit of zero or less means that window is unlimited.
func New(dailyLimit, monthlyLimit float64) *Tracker {
	return &Tracker{
		dailyLimit:   dailyLimit,
		monthlyLimit: monthlyLimit,
		now:          time.Now,
	}
}

// Check reports whether another paid call is allowed. The cost of a call is
// only known after it returns, so the ceiling means "stop once the limit is
// reached", not "stop before crossing it" — a single call may overshoot. For
// the same reason the boundary is approximate: accumulated float64 costs can
// land a fraction below a limit they were meant to hit exactly, which is
// irrelevant next to the size of one call.
//
// A nil tracker allows everything, which keeps callers that never configured a
// budget free of nil checks.
func (t *Tracker) Check() error {
	if t == nil {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.rollover()

	if t.dailyLimit > 0 && t.daily >= t.dailyLimit {
		return &LimitError{Window: WindowDaily, Spent: t.daily, Limit: t.dailyLimit}
	}
	if t.monthlyLimit > 0 && t.monthly >= t.monthlyLimit {
		return &LimitError{Window: WindowMonthly, Spent: t.monthly, Limit: t.monthlyLimit}
	}
	return nil
}

// Seed replaces the counters with spend already recorded for the current
// windows, which is how a restart picks up where it left off. It is meant to run
// once before the tracker is shared, and it overwrites rather than adds so that
// seeding twice cannot double-count the same history.
func (t *Tracker) Seed(daily, monthly float64) {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	// rollover first: it stamps the window keys, and without them the next call
	// would treat this seed as belonging to a stale window and zero it.
	t.rollover()

	t.daily = daily
	t.monthly = monthly
}

// Record adds the cost of a completed paid call to both windows.
func (t *Tracker) Record(cost float64) {
	if t == nil || cost <= 0 {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.rollover()

	t.daily += cost
	t.monthly += cost
}

func (t *Tracker) Snapshot() Status {
	if t == nil {
		return Status{}
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.rollover()

	return Status{
		DailySpent:   t.daily,
		DailyLimit:   t.dailyLimit,
		MonthlySpent: t.monthly,
		MonthlyLimit: t.monthlyLimit,
	}
}

// DayKey and MonthKey name the window an instant belongs to. They are exported
// because whoever persists costs has to bucket them the same way the tracker
// does; two independent date formats would drift apart at a month boundary and
// resurrect spend that should have expired.
func DayKey(t time.Time) string   { return t.Format("2006-01-02") }
func MonthKey(t time.Time) string { return t.Format("2006-01") }

// rollover zeroes counters whose window has elapsed. Windows are keyed by
// formatted local date rather than by an expiry timestamp so that "daily" means
// the calendar day the user lives in, not a rolling 24 hours. The caller holds
// the lock.
func (t *Tracker) rollover() {
	now := t.now()
	if day := DayKey(now); day != t.dayKey {
		t.dayKey = day
		t.daily = 0
	}
	if month := MonthKey(now); month != t.monthKey {
		t.monthKey = month
		t.monthly = 0
	}
}
