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

type ModeInfo struct {
	ID     string `json:"id,omitempty"`
	Label  string `json:"label"`
	Prompt string `json:"prompt"`
}

var (
	modesMu sync.RWMutex
	modes   = make(map[string]ModeInfo)
)

func init() {
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

