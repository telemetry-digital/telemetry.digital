package httpapi

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
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
	order := []string{"label: 'Overview', icon:", "href: '/dashboards'", "{ head: 'Monitoring' }", "href: '/counting', label: 'Object counting'", "href: '/incidents'",
		"label: 'Reports', icon:", "{ head: 'Devices and automation' }", "href: '/devices'", "label: 'Automation', icon:", "href: '/fota'",
		"{ head: 'Administration' }", "href: '/users'", "label: 'System'", "{ head: '' }", "label: 'My account'"}
	at := 0
	for _, o := range order {
		i := strings.Index(s[at:], o)
		if i < 0 {
			t.Fatalf("menu: %q missing or out of order", o)
		}
		at += i + len(o)
	}
}

// 0.75.0: the report pages are one group (Reports), alarm and automation rules are under Automation, My account is
// listed once (the sidebar shows it in its footer: foot) and the two kinds of notifications have distinct names.
func TestMenuClarity(t *testing.T) {
	b, err := static.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"{ href: '/reports/templates', label: 'Report templates'", "{ href: '/reports', label: 'Stored reports'",
		"{ href: '/reports/scheduled', label: 'Scheduled reports'", "{ href: '/reports/signatures', label: 'Report signatures'",
		"{ href: '/automation/alarm-rules', label: 'Alarm rules' }", "{ href: '/automation/rules', label: 'Automation rules'",
		"label: 'Notification channels'", "label: 'Browser notifications'", "label: 'My account', icon: ICON.user, show: !!me, foot: true",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("app.js menu: %q missing", want)
		}
	}
	// the old addresses are no menu items any more (the server redirects them)
	for _, old := range []string{"'/settings/templates'", "'/settings/reports'", "'/settings/scheduled-reports'", "'/audit/reports'", "'/assets/rules'", "'/assets/automation'"} {
		if strings.Contains(s, "href: "+old) {
			t.Errorf("app.js menu still links %s", old)
		}
	}
	if n := strings.Count(s, "label: 'My account'"); n != 1 {
		t.Errorf("My account appears %d times in the menu", n)
	}
	if strings.Contains(s, `title="My account">${ICON.user}<span>My account</span></a>`) {
		t.Error("the sidebar footer still has its own My account link next to the menu item")
	}
}

func TestMovedTo(t *testing.T) {
	for _, c := range []struct{ to, url, want string }{
		{"/reports/scheduled", "/settings/scheduled-reports", "/reports/scheduled"},
		{"/reports/signatures", "/audit/reports?x=1&y=2", "/reports/signatures?x=1&y=2"},
	} {
		w := httptest.NewRecorder()
		movedTo(c.to).ServeHTTP(w, httptest.NewRequest("GET", c.url, nil))
		if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != c.want {
			t.Errorf("%s: %d %q, want 301 %q", c.url, w.Code, w.Header().Get("Location"), c.want)
		}
	}
}
