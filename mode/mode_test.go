package mode

import (
	"os"
	"path/filepath"
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

func TestMode_GetModeKeyboard(t *testing.T) {
	kb := GetModeKeyboard()
	if len(kb.InlineKeyboard) == 0 {
		t.Error("expected non-empty inline keyboard")
	}
}

func TestMode_LoadCustomModes_InvalidJSONResilience(t *testing.T) {
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "modes.json")

	// Write corrupt JSON
	if err := os.WriteFile(invalidFile, []byte("{ invalid json "), 0644); err != nil {
		t.Fatal(err)
	}

	// Should not clear existing embedded modes
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
