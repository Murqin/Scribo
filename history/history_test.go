package history

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "history.jsonl"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if s == nil {
		t.Fatal("Open returned a nil store for a non-empty path")
	}
	return s
}

func TestAppendThenLastSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")

	first, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := first.Append(Entry{At: time.Now(), ChatID: 7, Mode: "tldr", Text: "eski"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := first.Append(Entry{At: time.Now(), ChatID: 7, Mode: "trans", Text: "yeni"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// A second Open stands in for a bot restart: nothing may be held in memory.
	second, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	entry, found, err := second.Last(7)
	if err != nil {
		t.Fatalf("Last: %v", err)
	}
	if !found {
		t.Fatal("entry written before the reopen was not found afterwards")
	}
	if entry.Text != "yeni" || entry.Mode != "trans" {
		t.Errorf("Last returned %q/%q, want the most recent entry (yeni/trans)", entry.Text, entry.Mode)
	}
}

func TestLastIsPerChat(t *testing.T) {
	s := tempStore(t)
	s.Append(Entry{ChatID: 1, Text: "bir"})
	s.Append(Entry{ChatID: 2, Text: "iki"})
	s.Append(Entry{ChatID: 1, Text: "bir-yeni"})

	entry, found, err := s.Last(1)
	if err != nil || !found {
		t.Fatalf("Last(1) = %v, %v", found, err)
	}
	if entry.Text != "bir-yeni" {
		t.Errorf("Last(1) = %q, want the newest entry of chat 1, not another chat's", entry.Text)
	}

	if _, found, _ := s.Last(99); found {
		t.Error("Last reported an entry for a chat that has none")
	}
}

func TestLastOnMissingFileIsEmptyNotError(t *testing.T) {
	// Open creates the file, so remove it to reach the "never written" state.
	path := filepath.Join(t.TempDir(), "history.jsonl")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, found, err := s.Last(1); err != nil || found {
		t.Errorf("missing file gave found=%v err=%v, want false/nil", found, err)
	}
}

func TestCorruptLineIsSkippedNotFatal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.Append(Entry{ChatID: 5, Text: "saglam"})

	// A crash mid-append leaves a truncated line; the entries after it must
	// still be readable.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open for corruption: %v", err)
	}
	f.WriteString(`{"chat_id":5,"text":"yarim`)
	f.WriteString("\n")
	f.Close()

	s.Append(Entry{ChatID: 5, Text: "sonraki"})

	entry, found, err := s.Last(5)
	if err != nil {
		t.Fatalf("a truncated line made the whole history unreadable: %v", err)
	}
	if !found || entry.Text != "sonraki" {
		t.Errorf("Last after corruption = %q (found=%v), want %q", entry.Text, found, "sonraki")
	}
}

func TestLongTranscriptSurvivesRoundTrip(t *testing.T) {
	// bufio.Scanner caps lines at 64 KB; a transcript can exceed that, and the
	// reader must not choke on it or on the entries behind it.
	long := strings.Repeat("uzun döküm satırı. ", 20000)

	s := tempStore(t)
	s.Append(Entry{ChatID: 3, Text: long})
	s.Append(Entry{ChatID: 3, Text: "arkadaki"})

	entry, found, err := s.Last(3)
	if err != nil {
		t.Fatalf("Last after a >64 KB line: %v", err)
	}
	if !found || entry.Text != "arkadaki" {
		t.Errorf("entry after a very long line was lost: found=%v text=%q", found, entry.Text)
	}
}

func TestSpendCountsOnlyCurrentWindows(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.Local)

	s := tempStore(t)
	s.Append(Entry{At: now, ChatID: 1, Cost: 0.02})                   // today
	s.Append(Entry{At: now.AddDate(0, 0, -1), ChatID: 1, Cost: 0.03}) // earlier this month
	s.Append(Entry{At: now.AddDate(0, -1, 0), ChatID: 1, Cost: 5.00}) // last month
	s.Append(Entry{At: now, ChatID: 1, Cost: 0})                      // free Google call
	s.Append(Entry{At: now.AddDate(0, 0, 1), ChatID: 1, Cost: 0.07})  // tomorrow

	daily, monthly, err := s.Spend(now)
	if err != nil {
		t.Fatalf("Spend: %v", err)
	}
	if !approx(daily, 0.02) {
		t.Errorf("daily = %v, want 0.02 (only today's paid calls)", daily)
	}
	if !approx(monthly, 0.12) {
		t.Errorf("monthly = %v, want 0.12 (this calendar month, both directions)", monthly)
	}
}

func TestSpendIgnoresOtherChats(t *testing.T) {
	// Spend backs a global ceiling, so it must not filter by chat.
	now := time.Now()
	s := tempStore(t)
	s.Append(Entry{At: now, ChatID: 1, Cost: 0.01})
	s.Append(Entry{At: now, ChatID: 2, Cost: 0.01})

	daily, _, err := s.Spend(now)
	if err != nil {
		t.Fatalf("Spend: %v", err)
	}
	if !approx(daily, 0.02) {
		t.Errorf("daily = %v, want 0.02 summed across chats", daily)
	}
}

func TestNilStoreIsAWorkingNoOp(t *testing.T) {
	var s *Store

	if s.Path() != "" {
		t.Errorf("nil store Path = %q, want empty", s.Path())
	}
	if err := s.Append(Entry{Text: "x"}); err != nil {
		t.Errorf("nil store Append = %v, want nil", err)
	}
	if _, found, err := s.Last(1); found || err != nil {
		t.Errorf("nil store Last = %v, %v; want false, nil", found, err)
	}
	if d, m, err := s.Spend(time.Now()); d != 0 || m != 0 || err != nil {
		t.Errorf("nil store Spend = %v, %v, %v; want 0, 0, nil", d, m, err)
	}
}

func TestOpenEmptyPathDisablesPersistence(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatalf("Open(\"\") = %v, want nil error", err)
	}
	if s != nil {
		t.Error("Open(\"\") returned a store; an empty path must disable persistence")
	}
}

func TestOpenCreatesMissingParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "veri", "alt", "history.jsonl")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open with a missing parent directory: %v", err)
	}
	if err := s.Append(Entry{ChatID: 1, Text: "x"}); err != nil {
		t.Errorf("Append into a created directory: %v", err)
	}
}

func TestOpenUnwritablePathIsAnError(t *testing.T) {
	// A configured but unusable history must fail at startup, not at the end of
	// the first transcript the user waited for.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "engel"), []byte("x"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if _, err := Open(filepath.Join(dir, "engel", "history.jsonl")); err == nil {
		t.Error("Open under a regular file succeeded; want an error")
	}
}

func TestConcurrentAppendsKeepEveryLineIntact(t *testing.T) {
	s := tempStore(t)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.Append(Entry{At: time.Now(), ChatID: 1, Cost: 0.01, Text: strings.Repeat("a", 500)})
		}(i)
	}
	wg.Wait()

	// Interleaved writes would corrupt lines, which Spend would silently
	// under-count rather than report.
	daily, _, err := s.Spend(time.Now())
	if err != nil {
		t.Fatalf("Spend: %v", err)
	}
	if !approx(daily, 0.20) {
		t.Errorf("daily = %v after 20 concurrent appends, want 0.20", daily)
	}
}

func approx(got, want float64) bool {
	d := got - want
	return d < 1e-9 && d > -1e-9
}

func TestPartiallyDecodedLineIsNotHandedOn(t *testing.T) {
	// A line can be valid JSON and still fail to decode — a wrong field type is
	// the common case. encoding/json populates the fields it managed to read
	// before returning that error, so an entry rejected here would otherwise be
	// handed on half-filled and outrank the last good one.
	path := filepath.Join(t.TempDir(), "history.jsonl")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.Append(Entry{ChatID: 5, Text: "saglam", Cost: 0.01, At: time.Now()})

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open for corruption: %v", err)
	}
	f.WriteString(`{"chat_id":5,"text":"bozuk","cost":"cok"}` + "\n")
	f.Close()

	entry, found, err := s.Last(5)
	if err != nil {
		t.Fatalf("Last: %v", err)
	}
	if !found || entry.Text != "saglam" {
		t.Errorf("Last = %q (found=%v), want the last fully decoded entry %q", entry.Text, found, "saglam")
	}

	daily, _, err := s.Spend(time.Now())
	if err != nil {
		t.Fatalf("Spend: %v", err)
	}
	if !approx(daily, 0.01) {
		t.Errorf("daily = %v, want 0.01 — a rejected line must not contribute", daily)
	}
}
