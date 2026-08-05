package mode

import (
	_ "embed"
	"encoding/json"
	"log/slog"
	"os"
	"sort"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

//go:embed default_modes.json
var defaultModesJSON []byte

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

// LoadDefaultModes restores the embedded mode set, discarding anything loaded from
// disk. Modes live in package-level state, so tests that swap them need a way back.
func LoadDefaultModes() {
	LoadModesFromBytes(defaultModesJSON, "gömülü varsayılan modlar")
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
		slog.Error("⚠️ Parse hatası, varsayılan modlar korunuyor", "source", sourceName, "error", err)
		return false
	}

	newModes := make(map[string]ModeInfo, len(customModes))
	for id, m := range customModes {
		m.ID = id
		if !m.Format.valid() {
			if m.Format != "" {
				slog.Warn("⚠️ Bilinmeyen format, 'code' varsayılıyor", "source", sourceName, "mode", id, "format", m.Format)
			}
			m.Format = FormatCode
		}
		newModes[id] = m
	}

	modesMu.Lock()
	modes = newModes
	modesMu.Unlock()

	slog.Info("✅ Modlar yüklendi", "source", sourceName, "count", len(newModes))
	return true
}

func LoadCustomModes(filename string) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		if writeErr := os.WriteFile(filename, defaultModesJSON, 0644); writeErr != nil {
			slog.Error("⚠️ Varsayılan modes.json dosyası oluşturulamadı", "filename", filename, "error", writeErr)
		} else {
			slog.Info("📝 Varsayılan modes.json dosyası otomatik oluşturuldu", "filename", filename)
		}
		return
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		slog.Error("⚠️ Harici mod dosyası okunurken hata", "filename", filename, "error", err)
		return
	}

	// Unmarshal safely into temporary map first without clearing current modes
	LoadModesFromBytes(data, filename)
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
