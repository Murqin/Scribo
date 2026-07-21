package mode

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestMode_DefaultModesLoaded(t *testing.T) {
	m, ok := GetMode("tldr")
	if !ok {
		t.Fatal("expected default mode 'tldr' to be loaded")
	}

	if m.Label == "" || m.Prompt == "" {
		t.Errorf("invalid tldr mode info: %+v", m)
	}
}

func TestMode_GetMode_NotFound(t *testing.T) {
	_, ok := GetMode("non_existent_mode_xyz")
	if ok {
		t.Error("expected ok=false for non-existent mode ID")
	}
}

func TestMode_GetModeKeyboard_Structure(t *testing.T) {
	kb := GetModeKeyboard()
	if len(kb.InlineKeyboard) == 0 {
		t.Fatal("expected non-empty inline keyboard")
	}

	// Verify maximum 2 buttons per row
	for rowIdx, row := range kb.InlineKeyboard {
		if len(row) > 2 {
			t.Errorf("row %d has %d buttons; max allowed is 2", rowIdx, len(row))
		}
		for _, btn := range row {
			if btn.Text == "" || btn.CallbackData == nil || *btn.CallbackData == "" {
				t.Errorf("invalid button in row %d: %+v", rowIdx, btn)
			}
		}
	}
}

func TestMode_LoadCustomModes_NonExistentFile(t *testing.T) {
	// Should return silently without clearing modes or panicking
	LoadCustomModes("non_existent_file_12345.json")

	_, ok := GetMode("tldr")
	if !ok {
		t.Error("default mode 'tldr' should be preserved when file does not exist")
	}
}

func TestMode_LoadCustomModes_InvalidJSONResilience(t *testing.T) {
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "modes.json")

	if err := os.WriteFile(invalidFile, []byte("{ invalid json "), 0644); err != nil {
		t.Fatal(err)
	}

	LoadCustomModes(invalidFile)

	_, ok := GetMode("tldr")
	if !ok {
		t.Error("embedded mode 'tldr' should be preserved when custom modes.json is invalid JSON")
	}
}

func TestMode_LoadCustomModes_ValidJSON(t *testing.T) {
	t.Cleanup(func() {
		LoadModesFromBytes(defaultModesJSON, "restore defaults")
	})

	tmpDir := t.TempDir()
	validFile := filepath.Join(tmpDir, "modes.json")

	validJSON := `{
		"custom1": {
			"label": "Custom Mode 1",
			"prompt": "Custom prompt 1"
		}
	}`

	if err := os.WriteFile(validFile, []byte(validJSON), 0644); err != nil {
		t.Fatal(err)
	}

	LoadCustomModes(validFile)

	m, ok := GetMode("custom1")
	if !ok {
		t.Fatal("expected custom mode 'custom1' to be loaded")
	}
	if m.Label != "Custom Mode 1" {
		t.Errorf("expected label 'Custom Mode 1', got %s", m.Label)
	}
}

func TestMode_ConcurrencySafety(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			GetMode("tldr")
		}()
		go func() {
			defer wg.Done()
			GetModeKeyboard()
		}()
	}
	wg.Wait()
}
