package httpapi

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// Every tab of a page's in-page sub-navigation (.subnav) is also a sub-item of the menu in app.js, with the same label:
// the desktop hides the .subnav because the sidebar lists the sub-pages, so a tab missing from the menu could not be
// reached there (0.73.4; Evidence, video Settings, License and the personal AI assistants were such tabs).
func TestSubnavTabsInMenu(t *testing.T) {
	b, err := static.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	menu := map[string]bool{}
	for _, m := range regexp.MustCompile(`\{ href: '([^']+)', label: '([^']+)'`).FindAllStringSubmatch(string(b), -1) {
		menu[m[1]+" "+m[2]] = true
	}
	navRe := regexp.MustCompile(`(?s)<nav class="subnav"[^>]*>(.*?)</nav>`)
	tabRe := regexp.MustCompile(`<a href="(/[^"]*)" data-section="[^"]+"[^>]*>([^<]+)</a>`)
	files, err := fs.Glob(static, "static/*.html")
	if err != nil {
		t.Fatal(err)
	}
	tabs := 0
	for _, f := range files {
		h, err := static.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range navRe.FindAllStringSubmatch(string(h), -1) {
			for _, a := range tabRe.FindAllStringSubmatch(n[1], -1) {
				tabs++
				if !menu[a[1]+" "+strings.TrimSpace(a[2])] {
					t.Errorf("%s: tab %q (%s) is not a menu item with that label in app.js", f, strings.TrimSpace(a[2]), a[1])
				}
			}
		}
	}
	if tabs < 40 {
		t.Fatalf("found %d page tabs, the patterns no longer match", tabs)
	}
}

// The menu has its section headings in this order, and every top-level item stays under the heading it belongs to.
func TestMenuSections(t *testing.T) {
	b, err := static.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	order := []string{"href: '/dashboards'", "{ head: 'Monitoring' }", "href: '/counting', label: 'Object counting'", "href: '/incidents'",
		"{ head: 'Devices and automation' }", "href: '/devices'", "href: '/fota'", "{ head: 'Administration' }", "href: '/users'",
		"label: 'System'", "{ head: '' }", "label: 'My account'"}
	at := 0
	for _, o := range order {
		i := strings.Index(s[at:], o)
		if i < 0 {
			t.Fatalf("menu: %q missing or out of order", o)
		}
		at += i + len(o)
	}
}
