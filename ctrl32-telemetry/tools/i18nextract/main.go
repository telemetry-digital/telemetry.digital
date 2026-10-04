// i18nextract collects the English UI strings from the static pages (text nodes, placeholders, titles and quoted
// string literals in page scripts, app.js and the shared telemetry.js and alarmrules.js) and writes them as the key
// list of the base catalog: internal/i18n/catalog/en.json (a JSON object whose keys are the English strings and
// values are the same strings).
//
//	go run ./tools/i18nextract
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	tagRe    = regexp.MustCompile(`(?s)<[^>]+>`)
	scriptRe = regexp.MustCompile(`(?s)<script>(.*?)</script>`)
	styleRe  = regexp.MustCompile(`(?s)<style>.*?</style>`)
	attrRe   = regexp.MustCompile(`(?:placeholder|title|aria-label|alt)="([^"]{2,120})"`)
	litRe    = regexp.MustCompile(`'((?:[^'\\]|\\.){2,120})'`)
	letterRe = regexp.MustCompile(`[A-Za-z]`)
)

// keep decides whether a candidate looks like human text rather than code, ids or markup.
func keep(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 2 || len(s) > 120 || !letterRe.MatchString(s) {
		return false
	}
	if strings.ContainsAny(s, "{}<>=/\\$#_[]|;") || strings.HasPrefix(s, ".") || strings.HasPrefix(s, "#") {
		return false
	}
	first := s[0]
	if !(first >= 'A' && first <= 'Z') && !(first >= 'a' && first <= 'z') && !strings.HasPrefix(s, "…") && !strings.HasPrefix(s, "—") && !strings.HasPrefix(s, "+") {
		return false
	}
	if !strings.Contains(s, " ") && (strings.ToLower(s) == s && len(s) < 4) {
		return false
	}
	// identifiers such as data.read, method names, css classes
	if !strings.Contains(s, " ") && strings.ContainsAny(s, ".:-") {
		return false
	}
	lower := strings.ToLower(s)
	for _, bad := range []string{"application/", "image/", "text/", "utf-8", "px", "onload", "ok\"", "csrf"} {
		if strings.Contains(lower, bad) {
			return false
		}
	}
	return true
}

func main() {
	dir := "internal/httpapi/static"
	out := "internal/i18n/catalog/en.json"
	keys := map[string]bool{}
	files, _ := filepath.Glob(filepath.Join(dir, "*.html"))
	files = append(files, filepath.Join(dir, "app.js"), filepath.Join(dir, "telemetry.js"), filepath.Join(dir, "alarmrules.js"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		s := string(b)
		if strings.HasSuffix(f, ".html") {
			s = styleRe.ReplaceAllString(s, "")
			for _, m := range scriptRe.FindAllStringSubmatch(s, -1) {
				for _, l := range litRe.FindAllStringSubmatch(m[1], -1) {
					if keep(l[1]) {
						keys[strings.TrimSpace(l[1])] = true
					}
				}
			}
			s = scriptRe.ReplaceAllString(s, "")
			for _, a := range attrRe.FindAllStringSubmatch(s, -1) {
				if keep(a[1]) {
					keys[strings.TrimSpace(a[1])] = true
				}
			}
			for _, t := range tagRe.Split(s, -1) {
				t = strings.Join(strings.Fields(t), " ")
				if keep(t) {
					keys[t] = true
				}
			}
		} else {
			for _, l := range litRe.FindAllStringSubmatch(s, -1) {
				if keep(l[1]) {
					keys[strings.TrimSpace(l[1])] = true
				}
			}
		}
	}
	list := make([]string, 0, len(keys))
	for k := range keys {
		list = append(list, k)
	}
	sort.Strings(list)
	m := map[string]string{}
	for _, k := range list {
		m[k] = k
	}
	_ = os.MkdirAll(filepath.Dir(out), 0o755)
	b, _ := json.MarshalIndent(m, "", " ")
	if err := os.WriteFile(out, b, 0o644); err != nil {
		panic(err)
	}
	fmt.Println(len(list), "strings ->", out)
}
