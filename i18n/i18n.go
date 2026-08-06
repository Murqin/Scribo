// Package i18n serves every string the user of the bot can see, plus the parts
// of the model prompt that decide which language the model answers in.
//
// Catalogs are embedded rather than read from disk: Scribo ships as a single
// static binary, and a release must not depend on a data file the user could
// forget to copy next to it.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
)

//go:embed locales/*.json
var localeFS embed.FS

// DefaultLanguage is what an unset, unknown or unparsable language setting falls
// back to. Turkish, because that is what every existing deployment already runs;
// a release that silently switched them to English would be a regression.
const DefaultLanguage = "tr"

type catalog map[string]string

var (
	mu       sync.RWMutex
	active   catalog
	current  = DefaultLanguage
	fallback catalog
)

func init() {
	c, err := load(DefaultLanguage)
	if err != nil {
		// The default catalog is embedded, so this can only fail if the binary
		// itself was built from a broken tree.
		panic("i18n: default catalog unusable: " + err.Error())
	}
	fallback, active = c, c
}

func load(lang string) (catalog, error) {
	data, err := localeFS.ReadFile("locales/" + lang + ".json")
	if err != nil {
		return nil, err
	}
	var c catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("locales/%s.json: %w", lang, err)
	}
	return c, nil
}

// Normalize reduces a POSIX locale to the bare language code, so both LANG
// values (en_US.UTF-8) and hand-written settings (EN) reach the same catalog.
func Normalize(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if i := strings.IndexAny(s, "_-.@"); i >= 0 {
		s = s[:i]
	}
	return s
}

// SetLanguage switches the active catalog and reports the language actually in
// use, which is DefaultLanguage whenever the requested one has no catalog.
func SetLanguage(raw string) string {
	lang := Normalize(raw)
	c, err := load(lang)
	if err != nil {
		if lang != "" && lang != DefaultLanguage {
			slog.Warn("no catalog for the requested language, falling back",
				"requested", lang, "fallback", DefaultLanguage)
		}
		lang, c = DefaultLanguage, fallback
	}

	mu.Lock()
	active, current = c, lang
	mu.Unlock()
	return lang
}

// Language reports the language code currently in use.
func Language() string {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Languages lists the embedded catalogs, sorted. Tests use it to hold every
// catalog to the same key set.
func Languages() []string {
	entries, err := localeFS.ReadDir("locales")
	if err != nil {
		return nil
	}
	var langs []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".json") {
			langs = append(langs, strings.TrimSuffix(name, ".json"))
		}
	}
	sort.Strings(langs)
	return langs
}

// T resolves a key in the active catalog, falling back to Turkish for a key a
// translation has not caught up with yet: a missing string must degrade to the
// wrong language, never to a blank message.
//
// With arguments the entry is treated as a format string. Callers pass the same
// arguments regardless of language, so every catalog has to spell the same verbs
// in the same order — TestCatalogsAgreeOnFormatVerbs is what enforces that.
func T(key string, args ...any) string {
	mu.RLock()
	s, ok := active[key]
	mu.RUnlock()
	if !ok || s == "" {
		s, ok = fallback[key]
	}
	if !ok {
		slog.Error("missing translation key", "key", key)
		return key
	}
	if len(args) == 0 {
		return s
	}
	return fmt.Sprintf(s, args...)
}
