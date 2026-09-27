package httpapi

import (
	"strings"
	"testing"
)

// Known XSS and resource-loading vectors must not survive the sanitiser (spec 001, T029).
func TestSanitizeSVGStripsActiveContent(t *testing.T) {
	in := `<?xml version="1.0"?>
<!DOCTYPE svg [<!ENTITY x "boom">]>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="200" height="100" onload="alert(1)">
  <script>alert(2)</script>
  <style>rect{fill:url(https://evil/x)}</style>
  <!-- comment -->
  <defs><linearGradient id="g"><stop offset="0" stop-color="#fff"/></linearGradient></defs>
  <rect x="1" y="2" width="50" height="40" fill="url(#g)" style="stroke:red;background:url(https://evil/y);fill-opacity:.5" onclick="alert(3)"/>
  <circle cx="10" cy="10" r="5" fill="url(https://evil/z)"/>
  <a href="javascript:alert(4)"><text x="0" y="10">link</text></a>
  <foreignObject><div xmlns="http://www.w3.org/1999/xhtml"><img src="x" onerror="alert(5)"/></div></foreignObject>
  <image href="https://evil/pixel.png"/>
  <use xlink:href="https://evil/sprite.svg#a"/>
  <use href="#g"/>
  <animate attributeName="href" to="javascript:alert(6)"/>
  <set attributeName="onmouseover" to="alert(7)"/>
  <path d="M0 0L10 10" transform="rotate(10)" data-x="1"/>
  <text x="5" y="50" font-size="12">&lt;b&gt;safe text</text>
</svg>`
	out, err := sanitizeSVG(in)
	if err != nil {
		t.Fatal(err)
	}
	low := strings.ToLower(out.SVG)
	for _, bad := range []string{"script", "alert", "onload", "onclick", "onerror", "evil", "javascript", "foreignobject", "<image", "<a ", "<animate", "<set", "<style", "data-x", "boom", "<!"} {
		if strings.Contains(low, bad) {
			t.Errorf("output still contains %q:\n%s", bad, out.SVG)
		}
	}
	for _, good := range []string{`<rect `, `fill="url(#g)"`, `style="stroke:red;fill-opacity:.5"`, `<use href="#g">`, `<path d="M0 0L10 10" transform="rotate(10)">`, `&lt;b&gt;safe text`, `xmlns="http://www.w3.org/2000/svg"`} {
		if !strings.Contains(out.SVG, good) {
			t.Errorf("output lost %q:\n%s", good, out.SVG)
		}
	}
	if out.Width != 200 || out.Height != 100 {
		t.Errorf("size %v × %v", out.Width, out.Height)
	}
	if len(out.Removed) < 8 {
		t.Errorf("removed list too short: %v", out.Removed)
	}
}

func TestSanitizeSVGRejects(t *testing.T) {
	for name, in := range map[string]string{
		"not xml":        `<svg><rect></svg>`,
		"html root":      `<html><body><svg/></body></html>`,
		"no svg":         `<?xml version="1.0"?><x/>`,
		"too large":      "<svg>" + strings.Repeat(" ", maxSVGBytes) + "</svg>",
		"too deep":       strings.Repeat("<g>", maxSVGDepth+1) + strings.Repeat("</g>", maxSVGDepth+1),
		"external ents":  `<!DOCTYPE svg [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><svg>&xxe;</svg>`,
		"billion laughs": `<!DOCTYPE svg [<!ENTITY a "aa"><!ENTITY b "&a;&a;">]><svg>&b;</svg>`,
	} {
		if _, err := sanitizeSVG(in); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSanitizeSVGSizeFromViewBox(t *testing.T) {
	out, err := sanitizeSVG(`<svg viewBox="0 0 640 480" width="100%"><rect width="10" height="10"/></svg>`)
	if err != nil {
		t.Fatal(err)
	}
	if out.Width != 640 || out.Height != 480 {
		t.Errorf("size %v × %v", out.Width, out.Height)
	}
}

// Composites keep their tag placeholders through validation; unknown placeholders are refused (spec 001, T030).
func TestCompositePlaceholders(t *testing.T) {
	body, err := validateHMIBody("composite", []byte(`{"width":300,"height":200,"tags":["level","run"],"elements":[
		{"type":"symbol","symbol":"tank_vertical","x":0,"y":0,"w":100,"h":180,"tag":"$level","dyn":[{"prop":"level","lin":[0,100,0,1]}]},
		{"type":"symbol","symbol":"pump_centrifugal","x":150,"y":100,"w":90,"h":84,"dyn":[{"prop":"state","tag":"$run","map":[{"op":"==","v":1,"to":"run"}]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	els := body["elements"].([]map[string]any)
	if els[0]["tag"] != "$level" || els[1]["dyn"].([]map[string]any)[0]["tag"] != "$run" {
		t.Errorf("placeholders not restored: %+v", els)
	}
	for name, in := range map[string]string{
		"unlisted placeholder": `{"width":100,"height":100,"tags":[],"elements":[{"type":"rect","tag":"$x"}]}`,
		"bad placeholder name": `{"width":100,"height":100,"tags":["X Y"],"elements":[{"type":"rect"}]}`,
		"no elements":          `{"width":100,"height":100,"tags":[],"elements":[]}`,
		"bad element":          `{"width":100,"height":100,"tags":[],"elements":[{"type":"script"}]}`,
		"no size":              `{"tags":[],"elements":[{"type":"rect"}]}`,
	} {
		if _, err := validateHMIBody("composite", []byte(in)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := validateHMIBody("macro", []byte(`{}`)); err == nil {
		t.Error("unknown kind accepted")
	}
}
