// Package history keeps finished transcripts on disk so they survive a restart.
//
// The format is JSONL — one JSON object per line, appended and never rewritten.
// SQLite was rejected in K-1: it would break the project's single-dependency and
// static-binary stance for a bot that serves one user. Every read is a full file
// scan, which is affordable at that scale; if it stops being affordable, this
// package is the only place a switch to an indexed store has to touch.
//
// The file is also where the spending ceiling gets its memory: every paid call
// writes its cost here, so budget.Tracker can be seeded from Spend at startup
// instead of forgetting the day's spend on every restart.
package history

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"scribo/budget"
)

// Entry is one completed run. Fields are recorded even when they are only
// interesting later — a transcript that cost nothing still needs its cost field
// present so Spend can sum a mixed file without special cases.
type Entry struct {
	At               time.Time `json:"at"`
	ChatID           int64     `json:"chat_id"`
	Mode             string    `json:"mode"`
	Format           string    `json:"format"`
	Provider         string    `json:"provider"`
	MimeType         string    `json:"mime_type,omitempty"`
	Text             string    `json:"text"`
	PromptTokens     int       `json:"prompt_tokens,omitempty"`
	CompletionTokens int       `json:"completion_tokens,omitempty"`
	Cost             float64   `json:"cost,omitempty"`
}

// Store appends to and reads back a single JSONL file.
//
// Every method tolerates a nil receiver, which is what an unconfigured
// HISTORY_FILE produces: persistence then costs the callers no nil checks and
// simply does nothing.
type Store struct {
	mu   sync.Mutex
	path string
}

// Open prepares the file at path for appending, creating it and any missing
// parent directories. An empty path disables persistence and returns a nil
// store. The file is opened once here rather than kept open for the process
// lifetime so that a path that cannot be written is reported at startup instead
// of at the end of the first transcript the user waited for.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, nil
	}

	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}

	return &Store{path: path}, nil
}

// Path reports the file being written, or "" when persistence is off.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Append writes one entry. The whole line is marshalled before the file is
// touched, so a value that cannot be encoded never leaves a half-written record
// behind.
//
// What keeps concurrent writers from interleaving is O_APPEND, not the mutex:
// the kernel makes the seek-and-write one operation. The mutex is still here to
// serialise the whole open/write/sync sequence, because os.File.Write does not
// retry a short write and a multi-megabyte transcript is large enough for one to
// be possible. Neither guarantee is observable from a test, which is why this
// note exists instead.
func (s *Store) Append(e Entry) error {
	if s == nil {
		return nil
	}

	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(line); err != nil {
		return err
	}
	return f.Sync()
}

// Last returns the most recent entry for a chat. The bool is false when the chat
// has no entries yet, which is an ordinary state and not an error.
func (s *Store) Last(chatID int64) (Entry, bool, error) {
	var last Entry
	var found bool

	err := s.each(func(e Entry) {
		if e.ChatID == chatID {
			last, found = e, true
		}
	})
	return last, found, err
}

// Spend totals what was recorded inside the calendar day and month containing
// now. It keys on the same formatted local dates budget.Tracker rolls over on,
// so a seeded tracker and a fresh one agree on where a window starts.
func (s *Store) Spend(now time.Time) (daily, monthly float64, err error) {
	dayKey, monthKey := budget.DayKey(now), budget.MonthKey(now)

	err = s.each(func(e Entry) {
		if e.Cost <= 0 {
			return
		}
		at := e.At.Local()
		if budget.MonthKey(at) != monthKey {
			return
		}
		monthly += e.Cost
		if budget.DayKey(at) == dayKey {
			daily += e.Cost
		}
	})
	return daily, monthly, err
}

// each walks the file, handing every decodable entry to fn.
//
// Lines that do not parse are logged and skipped rather than aborting the walk:
// an append-only log interrupted by a crash or a full disk can end in a partial
// line, and one truncated record must not make the whole history unreadable.
// A missing file is an empty history, not a failure.
func (s *Store) each(fn func(Entry)) error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	// bufio.Reader rather than bufio.Scanner: a transcript easily exceeds
	// Scanner's 64 KB line cap, and Scanner reports that as an error that would
	// hide every entry after the long one.
	r := bufio.NewReader(f)
	for {
		line, readErr := r.ReadBytes('\n')
		if len(line) > 0 {
			var e Entry
			if err := json.Unmarshal(line, &e); err != nil {
				// A final line without a trailing newline is normal at EOF; only
				// complain about content that is genuinely malformed.
				if len(bytes.TrimSpace(line)) > 0 {
					slog.Warn("⚠️ skipped an unreadable line in the history file", "file", s.path, "error", err)
				}
			} else {
				fn(e)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
	}
}
