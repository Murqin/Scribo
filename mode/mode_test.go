package mode_test

import (
	"testing"

	"scribo/mode"
)

func TestModesParity(t *testing.T) {
	expectedModes := []string{"tldr", "trans", "fix"}

	if len(mode.Modes) != len(expectedModes) {
		t.Fatalf("Mod sayısı uyuşmuyor! Beklenen: %d, Alınan: %d", len(expectedModes), len(mode.Modes))
	}

	for _, m := range expectedModes {
		info, ok := mode.Modes[m]
		if !ok {
			t.Errorf("Mod eksik: %s", m)
		}
		if info.Label == "" {
			t.Errorf("Mod etiketi boş: %s", m)
		}
		if info.Prompt == "" {
			t.Errorf("Mod promptu boş: %s", m)
		}
	}
}

func TestLoadCustomModes(t *testing.T) {
	mode.LoadCustomModes("../modes.example.json")
	if mode.Modes["tldr"].Label != "📝 Özet" {
		t.Errorf("Özet label uyuşmuyor: %s", mode.Modes["tldr"].Label)
	}
}
