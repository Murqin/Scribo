package mode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"scribo/i18n"
)

// useLanguage switches both the text catalog and the loaded mode set, and puts
// them back afterwards. Both live in package-level state, so a test that leaves
// them switched poisons the ones after it.
func useLanguage(t *testing.T, lang string) {
	t.Helper()
	t.Cleanup(func() {
		i18n.SetLanguage(i18n.DefaultLanguage)
		LoadDefaultModes()
	})
	i18n.SetLanguage(lang)
	LoadDefaultModes()
}

func TestDefaultModesFollowTheActiveLanguage(t *testing.T) {
	useLanguage(t, "tr")
	tr, ok := GetMode("tldr")
	if !ok {
		t.Fatal("mode 'tldr' missing from the Turkish defaults")
	}

	useLanguage(t, "en")
	en, ok := GetMode("tldr")
	if !ok {
		t.Fatal("mode 'tldr' missing from the English defaults")
	}

	if en.Label == tr.Label {
		t.Errorf("English defaults reuse the Turkish label %q", en.Label)
	}
	if en.Prompt == tr.Prompt {
		t.Error("English defaults reuse the Turkish prompt, so the model would still answer in Turkish")
	}
	if !strings.Contains(en.Prompt, "English") {
		t.Errorf("the English prompt does not pin the answer language: %q", en.Prompt)
	}
}

func TestEveryLanguageShipsTheSameModes(t *testing.T) {
	entries, err := defaultModesFS.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the embedded mode sets: %v", err)
	}

	var reference map[string]ModeInfo
	var referenceName string
	for _, e := range entries {
		data, err := defaultModesFS.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		var set map[string]ModeInfo
		if err := json.Unmarshal(data, &set); err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		if reference == nil {
			reference, referenceName = set, e.Name()
			continue
		}

		for id, ref := range reference {
			m, ok := set[id]
			if !ok {
				t.Errorf("%s is missing mode %q that %s has", e.Name(), id, referenceName)
				continue
			}
			// A mode whose format drifts between languages renders differently
			// for no reason the user could have asked for.
			if m.Format != ref.Format {
				t.Errorf("%s: mode %q has format %q, %s has %q",
					e.Name(), id, m.Format, referenceName, ref.Format)
			}
			if strings.TrimSpace(m.Label) == "" || strings.TrimSpace(m.Prompt) == "" {
				t.Errorf("%s: mode %q has an empty label or prompt", e.Name(), id)
			}
		}
		for id := range set {
			if _, ok := reference[id]; !ok {
				t.Errorf("%s has mode %q that %s does not", e.Name(), id, referenceName)
			}
		}
	}
}

func TestLoadCustomModesWritesTheDefaultsOfTheActiveLanguage(t *testing.T) {
	useLanguage(t, "en")

	path := filepath.Join(t.TempDir(), "modes.json")
	LoadCustomModes(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no modes file was written: %v", err)
	}
	var set map[string]ModeInfo
	if err := json.Unmarshal(data, &set); err != nil {
		t.Fatalf("the written modes file is not valid JSON: %v", err)
	}
	if !strings.Contains(set["tldr"].Prompt, "English") {
		t.Errorf("the generated modes.json carries a prompt in the wrong language: %q", set["tldr"].Prompt)
	}
}

func TestLoadCustomModesRegeneratesAGeneratedFileAfterALanguageChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "modes.json")

	useLanguage(t, "tr")
	LoadCustomModes(path)

	useLanguage(t, "en")
	LoadCustomModes(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the modes file: %v", err)
	}
	var set map[string]ModeInfo
	if err := json.Unmarshal(data, &set); err != nil {
		t.Fatalf("the regenerated modes file is not valid JSON: %v", err)
	}
	if !strings.Contains(set["tldr"].Prompt, "English") {
		t.Errorf("a generated modes.json was not regenerated for the new language: %q", set["tldr"].Prompt)
	}
	if got, _ := GetMode("tldr"); !strings.Contains(got.Prompt, "English") {
		t.Errorf("the loaded modes still carry the old language: %q", got.Prompt)
	}
}

func TestLoadCustomModesNeverOverwritesAnEditedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "modes.json")
	edited := []byte(`{"tldr":{"label":"My label","prompt":"My prompt","format":"plain"}}`)
	if err := os.WriteFile(path, edited, 0644); err != nil {
		t.Fatal(err)
	}

	useLanguage(t, "en")
	LoadCustomModes(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the modes file: %v", err)
	}
	if string(data) != string(edited) {
		t.Fatalf("a hand-edited modes.json was overwritten:\n%s", data)
	}
	if got, _ := GetMode("tldr"); got.Prompt != "My prompt" {
		t.Errorf("the edited file was not loaded: %+v", got)
	}
}
