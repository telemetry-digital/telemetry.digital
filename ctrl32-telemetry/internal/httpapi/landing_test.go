package httpapi

import (
	"strings"
	"testing"
)

func TestDefaultLandingValid(t *testing.T) {
	sp := defaultLanding()
	if err := validateLanding(&sp, func(string) bool { return false }); err != nil {
		t.Fatal(err)
	}
}

// The front page is public: no field may carry script, foreign resources or links that run code (spec 012).
func TestLandingRejectsUnsafeContent(t *testing.T) {
	btn := func(href string) LandingSpec {
		return LandingSpec{Version: 1, Blocks: []LandingBlock{{ID: "a", Type: "cta", Buttons: []LandingButton{{Label: "x", Href: href}}}}}
	}
	for _, href := range []string{"javascript:alert(1)", "JavaScript:alert(1)", " javascript:alert(1)", "data:text/html,<script>", "//evil.example/x", "http://plain.example",
		"https://a.example/\"onmouseover=\"x", "vbscript:x", "/\\evil", "https://", ""} {
		sp := btn(href)
		if validateLanding(&sp, nil) == nil {
			t.Errorf("href %q accepted", href)
		}
	}
	for _, href := range []string{"/login", "/p/abc?x=1#top", "https://example.com/a?b=c", "mailto:info@example.com", "tel:+421 2 123 456"} {
		sp := btn(href)
		if err := validateLanding(&sp, nil); err != nil {
			t.Errorf("href %q refused: %v", href, err)
		}
	}
	bad := []LandingSpec{
		{Version: 2},
		{Version: 1, Blocks: []LandingBlock{{ID: "a", Type: "html"}}},
		{Version: 1, Blocks: []LandingBlock{{ID: "a", Type: "text"}, {ID: "a", Type: "text"}}},
		{Version: 1, Blocks: []LandingBlock{{ID: "A B", Type: "text"}}},
		{Version: 1, Blocks: []LandingBlock{{ID: "a", Type: "hero", Background: "red;background:url(x)"}}},
		{Version: 1, Theme: LandingTheme{Accent: "expression(x)"}},
		{Version: 1, Blocks: []LandingBlock{{ID: "a", Type: "image", Image: "https://evil.example/x.png"}}},
		{Version: 1, Blocks: []LandingBlock{{ID: "a", Type: "image", Image: "01a0dfc5-3db0-7441-8ed4-0b512090e72c"}}}, // unknown asset
		{Version: 1, Blocks: []LandingBlock{{ID: "a", Type: "features", Items: []LandingItem{{Icon: "<svg onload=x>"}}}}},
		{Version: 1, Blocks: []LandingBlock{{ID: "a", Type: "text", Text: strings.Repeat("x", 4001)}}},
		{Version: 1, Blocks: []LandingBlock{{ID: "a", Type: "contact", Email: "a@b\"><script>"}}},
		{Version: 1, Blocks: []LandingBlock{{ID: "a", Type: "text", Columns: 9}}},
	}
	for i, sp := range bad {
		if validateLanding(&sp, func(string) bool { return false }) == nil {
			t.Errorf("spec %d accepted", i)
		}
	}
	many := LandingSpec{Version: 1}
	for i := 0; i <= landingMaxBlocks; i++ {
		many.Blocks = append(many.Blocks, LandingBlock{ID: "b" + strings.Repeat("x", i%20) + string(rune('a'+i%26)), Type: "spacer"})
	}
	if validateLanding(&many, nil) == nil {
		t.Error("too many blocks accepted")
	}
	// text with markup is allowed as text: the browser shows it literally
	ok := LandingSpec{Version: 1, Blocks: []LandingBlock{{ID: "a", Type: "text", Title: "<b>x</b>", Text: "<script>alert(1)</script>"}}}
	if err := validateLanding(&ok, nil); err != nil {
		t.Fatal(err)
	}
}
