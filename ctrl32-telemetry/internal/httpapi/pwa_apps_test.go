package httpapi

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestValidPWAStart(t *testing.T) {
	good := []string{"/", "/video", "/video/walls/0199a1b2-7c3d-7e4f-8a9b-0c1d2e3f4a5b", "/video/cameras/abc", "/dashboards/plant-1",
		"/home", "/energy", "/incidents", "/displays", "/api/docs", "/dashboards/x?range=8h", "/app-guide"}
	for _, u := range good {
		if !validPWAStart(u) {
			t.Errorf("refused %q", u)
		}
	}
	bad := []string{"", "video", "//evil.example/x", "/\\evil.example", "https://evil.example/", "javascript:alert(1)", "/javascript:alert(1)",
		" /video", "/video/../api/v1/users", "/a//b", "/app/cams", "/app", "/api/v1/devices", "/sw.js", "/manifest.webmanifest",
		"/video#x", "/video?x=<b>", "/vi\"deo", "/video\n", strings.Repeat("/a", 200), "/tiles/1/2/3", "/.well-known/security.txt"}
	for _, u := range bad {
		if validPWAStart(u) {
			t.Errorf("accepted %q", u)
		}
	}
}

func TestValidatePWAApp(t *testing.T) {
	ok := func() PWAApp {
		return PWAApp{Slug: "cams", Name: "Cameras", ShortName: "Cams", StartURL: "/video", Sections: []string{"cameras"}}
	}
	a := ok()
	a.Sections = []string{"cameras", "home", "cameras", " dashboards "}
	if msg, _ := validatePWAApp(&a); msg != "" {
		t.Fatal(msg)
	}
	if want := []string{"dashboards", "home", "cameras"}; !slices.Equal(a.Sections, want) {
		t.Errorf("sections %v, want menu order without duplicates %v", a.Sections, want)
	}
	a = ok()
	a.ShortName = ""
	a.Name = "A very long name of the camera app for the gate"
	if msg, warn := validatePWAApp(&a); msg != "" || len([]rune(a.ShortName)) != 24 || len(warn) != 1 {
		t.Errorf("empty short name: msg %q, short %q, warnings %v", msg, a.ShortName, warn)
	}
	a = ok()
	if _, warn := validatePWAApp(&a); len(warn) != 0 {
		t.Errorf("short name of 4 characters warned: %v", warn)
	}
	cases := map[string]func(*PWAApp){
		"slug":        func(a *PWAApp) { a.Slug = "Cams" },
		"slug-dash":   func(a *PWAApp) { a.Slug = "-cams" },
		"slug-long":   func(a *PWAApp) { a.Slug = strings.Repeat("a", 41) },
		"slug-resv":   func(a *PWAApp) { a.Slug = "options" },
		"name":        func(a *PWAApp) { a.Name = "" },
		"name-markup": func(a *PWAApp) { a.Name = "<b>x</b>" },
		"short-long":  func(a *PWAApp) { a.ShortName = strings.Repeat("x", 25) },
		"start":       func(a *PWAApp) { a.StartURL = "https://evil.example" },
		"start-js":    func(a *PWAApp) { a.StartURL = "javascript:alert(1)" },
		"no-sections": func(a *PWAApp) { a.Sections = nil },
		"section":     func(a *PWAApp) { a.Sections = []string{"account"} },
		"colour":      func(a *PWAApp) { a.ThemeColor = "red" },
	}
	for name, mut := range cases {
		a := ok()
		mut(&a)
		if msg, _ := validatePWAApp(&a); msg == "" {
			t.Errorf("%s: accepted %+v", name, a)
		}
	}
}

func TestPWAManifest(t *testing.T) {
	a := &PWAApp{Slug: "cams", Name: "Cameras", ShortName: "Cams", StartURL: "/video/walls/w1",
		Sections: []string{"dashboards", "home", "energy", "cameras", "incidents"}}
	b := branding{Theme: defaultTheme()}
	b.Theme.Primary = "#0f766e"
	m := pwaManifest(a, pwaIdentityOf(a, b, nil))
	raw, _ := json.Marshal(m)
	var got struct {
		ID, Name, ShortName, StartURL, Scope, Display, ThemeColor string
		Icons                                                     []map[string]any
		Shortcuts                                                 []struct{ Name, URL string }
	}
	_ = json.Unmarshal(raw, &got)
	var keyed map[string]any
	_ = json.Unmarshal(raw, &keyed)
	if keyed["id"] != "/app/cams" || keyed["start_url"] != "/app/cams" || keyed["scope"] != "/" || keyed["display"] != "standalone" {
		t.Errorf("identity: %s", raw)
	}
	if keyed["short_name"] != "Cams" || keyed["name"] != "Cameras · ctrl32 telemetry" {
		t.Errorf("names without white label: %v / %v", keyed["name"], keyed["short_name"])
	}
	if keyed["theme_color"] != "#0f766e" {
		t.Errorf("theme colour falls back to the organization's: %v", keyed["theme_color"])
	}
	if len(got.Shortcuts) != pwaMaxShortcuts || got.Shortcuts[0].URL != "/dashboards" || got.Shortcuts[3].URL != "/video" {
		t.Errorf("shortcuts: %+v", got.Shortcuts)
	}
	if len(got.Icons) != 3 || got.Icons[0]["src"] != "/icon.svg" {
		t.Errorf("product icons without white label: %v", got.Icons)
	}

	// licensed white label: the app's own name, the organization's icon, its product name in the description
	a.ThemeColor = "#112233"
	wl := &WhiteLabel{Enabled: true, ProductName: "Acme Watch", Icon: &logoData{Mime: "image/png", Data: "AA=="}}
	id := pwaIdentityOf(a, b, wl)
	m = pwaManifest(a, id)
	if m["name"] != "Cameras" || m["theme_color"] != "#112233" || !strings.Contains(m["description"].(string), "Acme Watch") {
		t.Errorf("white label: %v", m)
	}
	if icons := m["icons"].([]map[string]any); len(icons) != 1 || icons[0]["src"] != "/api/v1/branding/icon" || id.TouchIcon != "/api/v1/branding/icon" {
		t.Errorf("white-label icon: %v, touch %s", icons, id.TouchIcon)
	}
	// an SVG icon cannot be an iPhone home-screen icon: the PNG stays
	wl.Icon.Mime = "image/svg+xml"
	if id := pwaIdentityOf(a, b, wl); id.TouchIcon != "/icon-192.png" {
		t.Errorf("svg touch icon: %s", id.TouchIcon)
	}
}

// The service worker must never answer app pages or app manifests from its cache (they change with the profile).
func TestServiceWorkerDoesNotCacheApps(t *testing.T) {
	b, err := static.ReadFile("static/sw.js")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "'/app/") {
		t.Error("sw.js lists /app/ pages in the shell cache")
	}
	if !strings.Contains(string(b), "FILES.includes(u.pathname)") {
		t.Error("sw.js no longer limits the cache to the listed shell files")
	}
}

// The menu sections of app.js and of the server are the same keys.
func TestPWASectionsMatchMenu(t *testing.T) {
	b, err := static.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range pwaSections {
		if !strings.Contains(string(b), "sec: '"+s.Key+"'") {
			t.Errorf("app.js menu has no item with sec: '%s'", s.Key)
		}
	}
	// and the other way round: a menu section the server does not know could never be chosen for an app
	for _, m := range regexp.MustCompile(`sec: '(\w+)'`).FindAllStringSubmatch(string(b), -1) {
		if !slices.ContainsFunc(pwaSections, func(p pwaSection) bool { return p.Key == m[1] }) {
			t.Errorf("app.js menu section %q is not a section of an app (pwaSections)", m[1])
		}
	}
	// every top-level menu item except My account belongs to a section, so an app shows only its own sections
	items := regexp.MustCompile(`(?m)^      \{ href: .*`).FindAllString(string(b), -1)
	if len(items) < 10 {
		t.Fatalf("found %d top-level menu items in app.js, the pattern no longer matches", len(items))
	}
	for _, it := range items {
		if !strings.Contains(it, "sec: '") && !strings.Contains(it, "label: 'My account'") {
			t.Errorf("top-level menu item without a section (shown in every app): %.80s", strings.TrimSpace(it))
		}
	}
}

// Options of 0.61.0: locked app, page zoom, camera layout.
func TestValidatePWAAppOptions(t *testing.T) {
	a := PWAApp{Slug: "kamery", Name: "Kamery", StartURL: "/video", Sections: []string{"cameras"}, PageZoom: true}
	if msg, _ := validatePWAApp(&a); msg != "" || a.CameraLayout != "auto" {
		t.Fatalf("empty layout: msg %q, layout %q (want auto)", msg, a.CameraLayout)
	}
	for _, l := range []string{"auto", "single", "grid", " single "} {
		a.CameraLayout = l
		if msg, _ := validatePWAApp(&a); msg != "" || a.CameraLayout != strings.TrimSpace(l) {
			t.Errorf("layout %q refused or not trimmed: %q %q", l, msg, a.CameraLayout)
		}
	}
	for _, l := range []string{"list", "Single", "1", "<b>"} {
		a.CameraLayout = l
		if msg, _ := validatePWAApp(&a); msg == "" {
			t.Errorf("layout %q accepted", l)
		}
	}
	// a locked app whose start page is outside its sections is allowed, with a warning
	a = PWAApp{Slug: "kamery", Name: "Kamery", StartURL: "/video/walls/w1", Sections: []string{"cameras"}, Locked: true}
	if msg, warn := validatePWAApp(&a); msg != "" || len(warn) != 0 {
		t.Errorf("start page in the cameras section: %q %v", msg, warn)
	}
	a.StartURL = "/dashboards/plant"
	if msg, warn := validatePWAApp(&a); msg != "" || len(warn) != 1 {
		t.Errorf("start page outside the sections of a locked app: %q %v", msg, warn)
	}
	a.Locked = false
	if _, warn := validatePWAApp(&a); len(warn) != 0 {
		t.Errorf("an unlocked app does not warn: %v", warn)
	}
	// the options are part of the version's checksum
	b := PWAApp{Slug: "k", Name: "K", ShortName: "K", StartURL: "/video", Sections: []string{"cameras"}, PageZoom: true, CameraLayout: "auto"}
	c := b
	c.Locked = true
	d := b
	d.PageZoom = false
	e := b
	e.CameraLayout = "single"
	if b.checksum() == c.checksum() || b.checksum() == d.checksum() || b.checksum() == e.checksum() {
		t.Error("checksum ignores locked, page_zoom or camera_layout")
	}
	out := c.out()
	if out["locked"] != true || out["page_zoom"] != true || out["camera_layout"] != "auto" {
		t.Errorf("profile output without the options: %v", out)
	}
}

func TestPWAPathIn(t *testing.T) {
	cams := pwaSectionPaths["cameras"]
	for _, p := range []string{"/video", "/video/walls", "/video/cameras/abc", "/video/walls/w1"} {
		if !pwaPathIn(p, cams) {
			t.Errorf("%s not in cameras", p)
		}
	}
	for _, p := range []string{"/videos", "/devices", "/", "/v", "/api/v1/video"} {
		if pwaPathIn(p, cams) {
			t.Errorf("%s in cameras", p)
		}
	}
	dash := pwaSectionPaths["dashboards"]
	if !pwaPathIn("/", dash) || !pwaPathIn("/dashboards/x", dash) || pwaPathIn("/devices", dash) {
		t.Error(`"/" of the dashboards section matches only itself`)
	}
	for _, s := range pwaSections {
		if _, ok := pwaSectionPaths[s.Key]; !ok {
			t.Errorf("section %s has no pages for a locked app", s.Key)
		} else if !pwaPathIn(s.URL, pwaSectionPaths[s.Key]) {
			t.Errorf("section %s: its own page %s is not allowed in a locked app", s.Key, s.URL)
		}
	}
}

// app.js confines a locked app with the same pages per section as the server.
func TestPWASectionPathsMatchAppJS(t *testing.T) {
	b, err := static.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	for key, paths := range pwaSectionPaths {
		q := make([]string, len(paths))
		for i, p := range paths {
			q[i] = "'" + p + "'"
		}
		if want := key + ": [" + strings.Join(q, ", ") + "]"; !strings.Contains(string(b), want) {
			t.Errorf("app.js APP_PATHS has no %s", want)
		}
	}
}

// The install page of a locked app offers no way into the full application.
func TestPWAInstallPageLocked(t *testing.T) {
	var sb strings.Builder
	data := map[string]any{"Slug": "kamery", "Name": "Kamery", "ShortName": "Kamery", "Start": "/video", "Theme": "#e30613", "TouchIcon": "/icon-192.png", "Product": "ctrl32 telemetry", "Locked": true}
	if err := pwaPage.Execute(&sb, data); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sb.String(), "/?full=1") {
		t.Error("locked app: install page links the full application")
	}
	sb.Reset()
	data["Locked"] = false
	_ = pwaPage.Execute(&sb, data)
	if !strings.Contains(sb.String(), "/?full=1") {
		t.Error("unlocked app: install page lost the link to the full application")
	}
}
