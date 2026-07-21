package mode

import (
	_ "embed"
	"encoding/json"
	"log"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

//go:embed default_modes.json
var defaultModesJSON []byte

type ModeInfo struct {
	ID     string `json:"id,omitempty"`
	Label  string `json:"label"`
	Prompt string `json:"prompt"`
}

var Modes = make(map[string]ModeInfo)

func init() {
	LoadModesFromBytes(defaultModesJSON, "gömülü varsayılan modlar")
}

func LoadModesFromBytes(data []byte, sourceName string) {
	var customModes map[string]ModeInfo
	if err := json.Unmarshal(data, &customModes); err != nil {
		log.Printf("⚠️ %s parse edilirken hata: %v", sourceName, err)
		return
	}

	for id, m := range customModes {
		m.ID = id
		Modes[id] = m
	}
	log.Printf("✅ %s yüklendi (%d mod).", sourceName, len(customModes))
}

func LoadCustomModes(filename string) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		log.Printf("⚠️ %s okunurken hata: %v", filename, err)
		return
	}

	// Reset existing modes so modes.json becomes 100% single source of truth if provided!
	Modes = make(map[string]ModeInfo)
	LoadModesFromBytes(data, filename)
}

func GetModeKeyboard() tgbotapi.InlineKeyboardMarkup {
	order := []string{"tldr", "trans", "fix", "note", "blog", "brainstorm", "social", "translate", "master"}
	visited := make(map[string]bool)
	var modeList []ModeInfo

	for _, id := range order {
		if m, ok := Modes[id]; ok {
			modeList = append(modeList, m)
			visited[id] = true
		}
	}

	for id, m := range Modes {
		if !visited[id] {
			m.ID = id
			modeList = append(modeList, m)
		}
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
