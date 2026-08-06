package mode

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"sync"

	"scribo/i18n"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

//go:embed default_modes.*.json
var defaultModesFS embed.FS

// defaultModes returns the built-in mode set for the active language. Both the
// button labels and the prompts differ per language: a translated interface in
// front of a Turkish prompt would still produce Turkish answers.
func defaultModes() []byte {
	data, err := defaultModesFS.ReadFile(fmt.Sprintf("default_modes.%s.json", i18n.Language()))
	if err == nil {
		return data
	}
	// A language with a text catalog but no mode file still gets a working bot.
	data, err = defaultModesFS.ReadFile("default_modes." + i18n.DefaultLanguage + ".json")
	if err != nil {
		panic("mode: default mode set unusable: " + err.Error())
	}
	return data
}

// Format selects how a mode's output is rendered in Telegram. Wrapping everything in
// <code> is right for transcripts (one tap copies them) but destroys the markdown that
// prose-oriented modes ask the model for, so each mode declares its own.
type Format string

const (
	// FormatCode wraps the output in <code> for tap-to-copy. Default for modes that
	// do not declare a format, so existing modes.json files keep their behaviour.
	FormatCode Format = "code"
	// FormatMarkdown renders the model's markdown as Telegram HTML.
	FormatMarkdown Format = "markdown"
	// FormatPlain sends the text verbatim, with no parse mode.
	FormatPlain Format = "plain"
)

func (f Format) valid() bool {
	switch f {
	case FormatCode, FormatMarkdown, FormatPlain:
		return true
	}
	return false
}

type ModeInfo struct {
	ID     string `json:"id,omitempty"`
	Label  string `json:"label"`
	Prompt string `json:"prompt"`
	Format Format `json:"format,omitempty"`
}

var (
	modesMu sync.RWMutex
	modes   = make(map[string]ModeInfo)
)

func init() {
	LoadDefaultModes()
}

// LoadDefaultModes restores the embedded mode set for the active language,
// discarding anything loaded from disk. Modes live in package-level state, so
// tests that swap them need a way back — and so does a language switch, which
// happens after this package has already initialised itself in Turkish.
func LoadDefaultModes() {
	LoadModesFromBytes(defaultModes(), "embedded default modes ("+i18n.Language()+")")
}

func GetMode(id string) (ModeInfo, bool) {
	modesMu.RLock()
	defer modesMu.RUnlock()
	m, ok := modes[id]
	return m, ok
}

func LoadModesFromBytes(data []byte, sourceName string) bool {
	var customModes map[string]ModeInfo
	if err := json.Unmarshal(data, &customModes); err != nil {
		slog.Error("⚠️ parse error, keeping the current modes", "source", sourceName, "error", err)
		return false
	}

	newModes := make(map[string]ModeInfo, len(customModes))
	for id, m := range customModes {
		m.ID = id
		if !m.Format.valid() {
			if m.Format != "" {
				slog.Warn("⚠️ unknown format, defaulting to 'code'", "source", sourceName, "mode", id, "format", m.Format)
			}
			m.Format = FormatCode
		}
		newModes[id] = m
	}

	modesMu.Lock()
	modes = newModes
	modesMu.Unlock()

	slog.Info("✅ modes loaded", "source", sourceName, "count", len(newModes))
	return true
}

func LoadCustomModes(filename string) {
	data, err := os.ReadFile(filename)
	if os.IsNotExist(err) {
		writeDefaultModes(filename)
		return
	}
	if err != nil {
		slog.Error("⚠️ could not read the external modes file", "filename", filename, "error", err)
		return
	}

	// A file that still matches a default set verbatim was written by an earlier
	// run, not by the user. Leaving it in place after a language change would
	// translate the interface but not the prompts, so the model would keep
	// answering in the old language — the one thing this setting exists to fix.
	// A file the user has actually edited is never touched.
	if isGeneratedDefault(data) && !bytes.Equal(data, defaultModes()) {
		slog.Info("♻️ regenerating the modes file for the active language",
			"filename", filename, "language", i18n.Language())
		writeDefaultModes(filename)
		return
	}

	// Unmarshal safely into temporary map first without clearing current modes
	LoadModesFromBytes(data, filename)
}

func writeDefaultModes(filename string) {
	if err := os.WriteFile(filename, defaultModes(), 0644); err != nil {
		slog.Error("⚠️ could not create the default modes file", "filename", filename, "error", err)
		return
	}
	slog.Info("📝 default modes file created", "filename", filename, "language", i18n.Language())
}

// isGeneratedDefault reports whether the bytes are one of the embedded default
// sets, in any language.
func isGeneratedDefault(data []byte) bool {
	entries, err := defaultModesFS.ReadDir(".")
	if err != nil {
		return false
	}
	for _, e := range entries {
		embedded, err := defaultModesFS.ReadFile(e.Name())
		if err == nil && bytes.Equal(data, embedded) {
			return true
		}
	}
	return false
}

func GetModeKeyboard() tgbotapi.InlineKeyboardMarkup {
	modesMu.RLock()
	modesCopy := make(map[string]ModeInfo, len(modes))
	for k, v := range modes {
		modesCopy[k] = v
	}
	modesMu.RUnlock()

	order := []string{"tldr", "trans", "fix", "note", "blog", "brainstorm", "social", "translate", "master"}
	visited := make(map[string]bool)
	var modeList []ModeInfo

	for _, id := range order {
		if m, ok := modesCopy[id]; ok {
			modeList = append(modeList, m)
			visited[id] = true
		}
	}

	var customIDs []string
	for id := range modesCopy {
		if !visited[id] {
			customIDs = append(customIDs, id)
		}
	}
	sort.Strings(customIDs)

	for _, id := range customIDs {
		m := modesCopy[id]
		m.ID = id
		modeList = append(modeList, m)
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	var currentRow []tgbotapi.InlineKeyboardButton

	for _, m := range modeList {
		btn := tgbotapi.NewInlineKeyboardButtonData(m.Label, m.ID)
		currentRow = append(currentRow, btn)
		if len(currentRow) == 2 {
			rows = append(rows, currentRow)
			currentRow = nil
		}
	}
	if len(currentRow) > 0 {
		rows = append(rows, currentRow)
	}

	return tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
}
