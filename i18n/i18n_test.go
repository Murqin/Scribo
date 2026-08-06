package i18n

import (
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// restoreDefault puts the package-level catalog back after a test moved it.
func restoreDefault(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { SetLanguage(DefaultLanguage) })
}

func TestLanguagesIncludesBothShippedCatalogs(t *testing.T) {
	got := Languages()
	if !reflect.DeepEqual(got, []string{"en", "tr"}) {
		t.Fatalf("Languages() = %v, want [en tr]", got)
	}
}

func TestCatalogsShareTheSameKeys(t *testing.T) {
	base, err := load(DefaultLanguage)
	if err != nil {
		t.Fatalf("default catalog: %v", err)
	}

	for _, lang := range Languages() {
		if lang == DefaultLanguage {
			continue
		}
		c, err := load(lang)
		if err != nil {
			t.Fatalf("%s catalog: %v", lang, err)
		}
		for key := range base {
			if _, ok := c[key]; !ok {
				t.Errorf("%s.json is missing key %q", lang, key)
			}
		}
		for key := range c {
			if _, ok := base[key]; !ok {
				t.Errorf("%s.json has key %q that %s.json does not", lang, key, DefaultLanguage)
			}
		}
	}
}

// reVerb matches a printf verb, including a width or precision. Callers pass the
// same arguments whatever the language is, so a translation that drops or
// reorders a verb produces %!s(MISSING) in front of the user.
var reVerb = regexp.MustCompile(`%(%|[#\-+ 0']*[0-9]*(?:\.[0-9]+)?[a-zA-Z])`)

func verbs(s string) []string {
	var out []string
	for _, m := range reVerb.FindAllString(s, -1) {
		if m == "%%" {
			continue
		}
		out = append(out, m)
	}
	return out
}

func TestCatalogsAgreeOnFormatVerbs(t *testing.T) {
	base, err := load(DefaultLanguage)
	if err != nil {
		t.Fatalf("default catalog: %v", err)
	}

	for _, lang := range Languages() {
		if lang == DefaultLanguage {
			continue
		}
		c, err := load(lang)
		if err != nil {
			t.Fatalf("%s catalog: %v", lang, err)
		}

		keys := make([]string, 0, len(base))
		for key := range base {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			want, got := verbs(base[key]), verbs(c[key])
			if !reflect.DeepEqual(want, got) {
				t.Errorf("%s: verbs %v in %s.json, %v in %s.json",
					key, want, DefaultLanguage, got, lang)
			}
		}
	}
}

func TestNormalizeReducesPOSIXLocaleToLanguage(t *testing.T) {
	tests := map[string]string{
		"en":            "en",
		"EN":            "en",
		" en ":          "en",
		"en_US.UTF-8":   "en",
		"en-GB":         "en",
		"tr_TR.UTF-8":   "tr",
		"tr_TR@islamic": "tr",
		"C.UTF-8":       "c",
		"":              "",
	}
	for raw, want := range tests {
		if got := Normalize(raw); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestSetLanguageSwitchesCatalog(t *testing.T) {
	restoreDefault(t)

	if got := SetLanguage("en_US.UTF-8"); got != "en" {
		t.Fatalf("SetLanguage(en_US.UTF-8) = %q, want en", got)
	}
	if Language() != "en" {
		t.Fatalf("Language() = %q after switching to en", Language())
	}
	if got := T("bot.btn_cancel"); got != "❌ Cancel" {
		t.Errorf("English catalog not in use: %q", got)
	}
}

func TestSetLanguageFallsBackToDefaultForUnknownLanguage(t *testing.T) {
	restoreDefault(t)

	for _, raw := range []string{"", "C", "de", "klingon"} {
		if got := SetLanguage(raw); got != DefaultLanguage {
			t.Errorf("SetLanguage(%q) = %q, want %q", raw, got, DefaultLanguage)
		}
		if got := T("bot.btn_cancel"); got != "❌ İptal Et" {
			t.Errorf("SetLanguage(%q) did not leave the default catalog active: %q", raw, got)
		}
	}
}

func TestTranslationFallsBackToDefaultForAKeyTheCatalogLacks(t *testing.T) {
	restoreDefault(t)

	// A half-translated catalog is the realistic case: a new key lands in tr.json
	// and en.json only catches up later. It must show the Turkish string, never
	// an empty message.
	mu.Lock()
	active, current = catalog{"bot.btn_cancel": "only key"}, "xx"
	mu.Unlock()

	if got := T("bot.btn_cancel"); got != "only key" {
		t.Errorf("present key not taken from the active catalog: %q", got)
	}
	if got := T("bot.btn_retry"); got != "🔄 Tekrar Dene" {
		t.Errorf("absent key did not fall back to %s: %q", DefaultLanguage, got)
	}
}

func TestTranslationOfAnUnknownKeyReturnsTheKey(t *testing.T) {
	if got := T("no.such.key"); got != "no.such.key" {
		t.Errorf("T on an unknown key = %q, want the key itself", got)
	}
}

func TestTranslationFormatsArguments(t *testing.T) {
	restoreDefault(t)
	SetLanguage("en")

	got := T("bot.long_media_warning", 120)
	if !strings.Contains(got, "120 s") {
		t.Errorf("argument not interpolated: %q", got)
	}
	if strings.Contains(got, "%") {
		t.Errorf("format verb left unconsumed: %q", got)
	}
}
