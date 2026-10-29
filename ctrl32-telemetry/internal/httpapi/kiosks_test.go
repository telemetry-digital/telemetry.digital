package httpapi

import (
	"strings"
	"testing"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
)

func TestValidServerURL(t *testing.T) {
	for in, want := range map[string]string{
		"http://10.48.8.30:8080":          "http://10.48.8.30:8080",
		"https://cams.example.com/":       "https://cams.example.com",
		" https://a.example.com/sub/ ":    "https://a.example.com/sub",
		"ftp://x":                         "",
		"http://user:pw@host":             "",
		"http://host/?q=1":                "",
		"http://host/'; Remove-Item C:\\": "",
		"http://ho$t":                     "",
		"":                                "",
	} {
		got, ok := validServerURL(in)
		if (want != "") != ok || got != want {
			t.Errorf("%q: got %q %v, want %q", in, got, ok, want)
		}
	}
}

func TestMeshNodeAndLink(t *testing.T) {
	id := "GKoRdAeAaVGSpMuLK$tDXYIdAedxLQA$3bd2NJKM4F4meclHarQgVk4Ax24jHMNC"
	for _, in := range []string{id, "node//" + id, "node/example/" + id, " " + id + " "} {
		if n, ok := normalizeMeshNode(in); !ok || n != id {
			t.Errorf("%q: %q %v", in, n, ok)
		}
	}
	for _, bad := range []string{"short", "<script>alert(1)</script>xxxxxxxxxxxxxxxxxxx", id + "&viewmode=13", "node/../" + id + "\""} {
		if _, ok := normalizeMeshNode(bad); ok {
			t.Errorf("accepted %q", bad)
		}
	}
	if l := meshLink("https://mesh.example.com", "a+b/c=$@"); l != "https://mesh.example.com/?gotonode=a%2Bb%2Fc%3D$@&viewmode=11&hide=16" {
		t.Errorf("link %s", l)
	}
	for in, ok := range map[string]bool{"https://mesh.example.com": true, "https://mesh.example.com/": true, "http://mesh.example.com": false, "https://u:p@mesh": false, "https://m/?a=1": false, "": true} {
		if _, got := validMeshURL(in); got != ok {
			t.Errorf("mesh url %q: %v", in, got)
		}
	}
}

func TestKioskScriptsAndSetup(t *testing.T) {
	s := &Server{cfg: &config.Config{}}
	s.cfg.HTTP.BaseURL = "https://portal.example.com"
	k := &kioskRow{ID: "0192a000-0000-7000-8000-000000000001", Name: "Stena BB – horný monitor's PC", Monitors: 2, ServerURL: "http://10.48.8.30:8080"}
	v1, files := s.kioskScripts(k)
	for _, n := range []string{"kiosk-agent.ps1", "kiosk-launcher.ps1", "capture.ps1", "launcher.json"} {
		if files[n] == "" {
			t.Fatalf("no %s", n)
		}
	}
	if !strings.Contains(files["launcher.json"], `"url": "http://10.48.8.30:8080/display"`) || !strings.Contains(files["launcher.json"], `"origin": "http://10.48.8.30:8080"`) || !strings.Contains(files["launcher.json"], `"monitors": 2`) || !strings.Contains(files["launcher.json"], `"cdp_port": 9222`) {
		t.Errorf("launcher.json: %s", files["launcher.json"])
	}
	// screenshots come from the browsers (0.86.5): no screen grab left, the DevTools port only on 127.0.0.1
	if strings.Contains(files["capture.ps1"], "CopyFromScreen") || strings.Contains(files["kiosk-agent.ps1"], "CopyFromScreen") || !strings.Contains(files["kiosk-agent.ps1"], "Page.captureScreenshot") {
		t.Error("the screenshot still grabs the screen")
	}
	if !strings.Contains(files["kiosk-launcher.ps1"], "--remote-debugging-port=") || strings.Contains(files["kiosk-launcher.ps1"], "--remote-debugging-address") || !strings.Contains(files["kiosk-agent.ps1"], "http://127.0.0.1:$port/json/list") {
		t.Error("the DevTools port must stay on 127.0.0.1")
	}
	// the scripts are read by Windows PowerShell 5.1 as ANSI: ASCII only
	for n, text := range files {
		for i, r := range text {
			if r > 126 && n != "launcher.json" {
				t.Fatalf("%s has a non-ASCII character at %d", n, i)
			}
		}
	}
	if v2, _ := s.kioskScripts(k); v2 != v1 || len(v1) != 12 {
		t.Errorf("version not stable: %s %s", v1, v2)
	}
	k.Monitors = 3
	if v3, _ := s.kioskScripts(k); v3 == v1 {
		t.Error("the version follows the settings")
	}
	name, text := s.kioskSetupScript(k, "c32k_secret'token")
	if name != "setup-kiosk-stena-bb-horn-monitor-s-pc.ps1" {
		t.Errorf("file name %q", name)
	}
	if strings.Contains(text, "__CTRL32_") {
		t.Error("a placeholder is left")
	}
	if !strings.Contains(text, "$Token = 'c32k_secret''token'") || !strings.Contains(text, "$Server = 'http://10.48.8.30:8080'") || !strings.Contains(text, "$KioskId = '"+k.ID+"'") {
		t.Error("values not filled in")
	}
	if !strings.Contains(text, "\r\n") || strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\n") {
		t.Error("Windows line ends")
	}
	for i, r := range text {
		if r > 126 && r != '\r' {
			t.Fatalf("setup script has a non-ASCII character at %d", i)
		}
	}
	k.ServerURL = ""
	if _, text := s.kioskSetupScript(k, "c32k_x"); !strings.Contains(text, "$Server = 'https://portal.example.com'") {
		t.Error("base_url as the default")
	}
}

func TestShotPerMonitor(t *testing.T) {
	pendingShot("k-test", 7, "u", 2)
	defer dropShot("k-test")
	x := shots.m["k-test"]
	if x.complete() {
		t.Fatal("complete before any picture")
	}
	x.imgs[0] = shotImage{data: []byte{0xFF, 0xD8}}
	if x.complete() {
		t.Fatal("complete with 1 of 2 monitors")
	}
	shotDone("k-test", 6, "other operation")
	if x.done {
		t.Fatal("the result of another operation ended it")
	}
	shotDone("k-test", 7, "screenshots of 1 of 2 monitors sent; monitor 2: debugging port 9223 not reachable")
	if !x.complete() || x.msg == "" {
		t.Fatal("the agent's result ends it")
	}
	pendingShot("k-test", 8, "u", 1)
	shots.m["k-test"].imgs[0] = shotImage{data: []byte{0xFF, 0xD8}}
	if !shots.m["k-test"].complete() {
		t.Fatal("all monitors sent")
	}
}

func TestDisplayHealthSummary(t *testing.T) {
	m := 1
	h := displayHealth{PageVersion: "0.86.0", Tiles: []displayTile{{Camera: "a", State: "playing"}, {Camera: "b", State: "stalled", Since: 40}, {Camera: "c", State: "playing"}}, Monitor: &m}
	s := h.summary()
	if s["tiles"] != 3 || s["playing"] != 2 || len(s["problems"].([]map[string]any)) != 1 || s["problems"].([]map[string]any)[0]["camera"] != "b" {
		t.Errorf("summary %v", s)
	}
}
