// Package i18n holds the UI translation catalogs (ADR-0009, URS J-10): English is the key language, other
// languages are catalogs keyed by the English string. Built-in catalogs ship in the binary; an organization can
// enable languages and override or add translations (content framework, ADR-0010).
package i18n

import (
	"embed"
	"encoding/json"
	"io/fs"
	"sort"
	"sync"
)

//go:embed catalog/*.json
var catalogs embed.FS

// Language is a supported UI language.
type Language struct {
	Code    string `json:"code"`
	Name    string `json:"name"`    // native name
	Native  string `json:"native"`  // same as Name (kept for clients that expect it)
	Builtin int    `json:"builtin"` // number of built-in translations
}

// Supported languages in the order shown to users (English is the base and always enabled).
var Supported = []Language{
	{Code: "en", Name: "English"}, {Code: "sk", Name: "Slovenčina"}, {Code: "cs", Name: "Čeština"}, {Code: "uk", Name: "Українська"},
	{Code: "de", Name: "Deutsch"}, {Code: "pl", Name: "Polski"}, {Code: "hu", Name: "Magyar"}, {Code: "es", Name: "Español"},
	{Code: "fr", Name: "Français"}, {Code: "it", Name: "Italiano"}, {Code: "ro", Name: "Română"}, {Code: "fi", Name: "Suomi"},
	{Code: "sv", Name: "Svenska"}, {Code: "no", Name: "Norsk"}, {Code: "da", Name: "Dansk"}, {Code: "et", Name: "Eesti"}, {Code: "nl", Name: "Nederlands"},
	{Code: "bg", Name: "Български"}, {Code: "el", Name: "Ελληνικά"},
}

var (
	once    sync.Once
	builtin map[string]map[string]string
)

func load() {
	once.Do(func() {
		builtin = map[string]map[string]string{}
		entries, _ := fs.ReadDir(catalogs, "catalog")
		for _, e := range entries {
			b, err := fs.ReadFile(catalogs, "catalog/"+e.Name())
			if err != nil {
				continue
			}
			m := map[string]string{}
			if json.Unmarshal(b, &m) == nil {
				builtin[e.Name()[:len(e.Name())-len(".json")]] = m
			}
		}
	})
}

// IsSupported reports whether the code is a known language.
func IsSupported(code string) bool {
	for _, l := range Supported {
		if l.Code == code {
			return true
		}
	}
	return false
}

// Languages returns the supported languages with their built-in translation counts.
func Languages() []Language {
	load()
	out := make([]Language, len(Supported))
	for i, l := range Supported {
		l.Native = l.Name
		l.Builtin = len(builtin[l.Code])
		out[i] = l
	}
	return out
}

// Keys returns the sorted English keys (the base catalog).
func Keys() []string {
	load()
	keys := make([]string, 0, len(builtin["en"]))
	for k := range builtin["en"] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Builtin returns the built-in catalog of a language (may be empty).
func Builtin(code string) map[string]string {
	load()
	return builtin[code]
}
