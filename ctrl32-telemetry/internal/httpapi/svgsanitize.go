package httpapi

// SVG import sanitiser (spec 001, US6, research R5). Customer graphics enter the object library only through this
// whitelist: known SVG shape, text, gradient and structure elements with presentation attributes. Everything else
// is dropped together with its content — scripts, event handlers, foreignObject, embedded HTML, images, links,
// animations, external references (href only as "#fragment", url() only as "url(#id)"), style sheets, comments and
// DOCTYPE declarations. The result is re-serialised from tokens, never passed through. It is a second line of defence:
// the renderer shows imported graphics through <image>, where browsers do not run scripts or load resources anyway.

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	svgNS          = "http://www.w3.org/2000/svg"
	xlinkNS        = "http://www.w3.org/1999/xlink"
	maxSVGBytes    = 512 << 10
	maxSVGElements = 20000
	maxSVGDepth    = 64
)

var (
	svgElements = setOf("svg", "g", "defs", "path", "rect", "circle", "ellipse", "line", "polyline", "polygon", "text", "tspan",
		"title", "desc", "linearGradient", "radialGradient", "stop", "clipPath", "mask", "pattern", "symbol", "use", "marker")
	svgAttrs = setOf("id", "class", "d", "x", "y", "x1", "y1", "x2", "y2", "cx", "cy", "r", "rx", "ry", "fx", "fy", "fr", "width", "height",
		"points", "viewBox", "preserveAspectRatio", "transform", "offset", "gradientUnits", "gradientTransform", "spreadMethod",
		"clipPathUnits", "maskUnits", "maskContentUnits", "patternUnits", "patternContentUnits", "patternTransform",
		"markerWidth", "markerHeight", "refX", "refY", "orient", "markerUnits", "dx", "dy", "rotate", "textLength", "lengthAdjust", "version",
		// presentation attributes (also allowed as style declarations)
		"fill", "fill-opacity", "fill-rule", "stroke", "stroke-width", "stroke-opacity", "stroke-linecap", "stroke-linejoin",
		"stroke-dasharray", "stroke-dashoffset", "stroke-miterlimit", "opacity", "stop-color", "stop-opacity", "clip-path", "clip-rule",
		"mask", "marker-start", "marker-mid", "marker-end", "font-family", "font-size", "font-weight", "font-style", "text-anchor",
		"dominant-baseline", "alignment-baseline", "letter-spacing", "word-spacing", "visibility", "display", "color", "paint-order",
		"vector-effect", "shape-rendering", "text-rendering")
	svgStyleProps = svgAttrs // CSS properties allowed in style="…" are the presentation attributes
	urlRe         = regexp.MustCompile(`(?i)url\s*\(`)
	localURLRe    = regexp.MustCompile(`(?i)^url\(\s*['"]?#[A-Za-z0-9_.:-]+['"]?\s*\)`)
	badValueRe    = regexp.MustCompile(`(?i)javascript:|vbscript:|data:|expression\s*\(|@import|behavior\s*:|-moz-binding|\\`)
	numRe         = regexp.MustCompile(`^\s*([0-9]+(?:\.[0-9]+)?)`)
)

func setOf(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// safeValue accepts an attribute or CSS value: no script schemes, escapes or foreign url().
func safeValue(v string) bool {
	if badValueRe.MatchString(v) {
		return false
	}
	for _, loc := range urlRe.FindAllStringIndex(v, -1) {
		if !localURLRe.MatchString(v[loc[0]:]) {
			return false
		}
	}
	return true
}

func cleanStyle(v string) string {
	var keep []string
	for _, decl := range strings.Split(v, ";") {
		k, val, ok := strings.Cut(decl, ":")
		k, val = strings.TrimSpace(strings.ToLower(k)), strings.TrimSpace(val)
		if !ok || !svgStyleProps[k] || val == "" || !safeValue(val) {
			continue
		}
		keep = append(keep, k+":"+val)
	}
	return strings.Join(keep, ";")
}

func escAttr(v string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(v))
	return strings.ReplaceAll(b.String(), `"`, "&#34;")
}

// SanitizedSVG is the result of an import: the clean document, its size and what was removed.
type SanitizedSVG struct {
	SVG     string   `json:"svg"`
	Width   float64  `json:"width"`
	Height  float64  `json:"height"`
	Removed []string `json:"removed"`
}

// sanitizeSVG returns a clean SVG document or an error when the input is not usable SVG.
func sanitizeSVG(in string) (*SanitizedSVG, error) {
	if len(in) > maxSVGBytes {
		return nil, fmt.Errorf("SVG larger than %d KiB", maxSVGBytes>>10)
	}
	dec := xml.NewDecoder(strings.NewReader(in))
	dec.Strict = true
	dec.Entity = map[string]string{} // no custom entities
	var out bytes.Buffer
	removed := map[string]int{}
	depth, skip, count := 0, 0, 0
	rootSeen, rootClosed := false, false
	var w, h float64
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("SVG is not well-formed XML: %v", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth > maxSVGDepth {
				return nil, errors.New("SVG nesting too deep")
			}
			if skip > 0 {
				skip++
				continue
			}
			name := t.Name.Local
			if (t.Name.Space != "" && t.Name.Space != svgNS) || !svgElements[name] || (rootClosed) {
				removed["<"+name+">"]++
				skip = 1
				continue
			}
			if !rootSeen {
				if name != "svg" {
					return nil, errors.New("the document root must be <svg>")
				}
				rootSeen = true
			}
			count++
			if count > maxSVGElements {
				return nil, fmt.Errorf("SVG has more than %d elements", maxSVGElements)
			}
			out.WriteString("<" + name)
			if depth == 1 {
				out.WriteString(` xmlns="` + svgNS + `"`)
			}
			attrs := map[string]string{}
			for _, a := range t.Attr {
				an := a.Name.Local
				switch {
				case a.Name.Space == "xmlns" || (a.Name.Space == "" && an == "xmlns"):
					continue // namespaces are re-declared on the root only
				case an == "href" && (a.Name.Space == "" || a.Name.Space == xlinkNS):
					if strings.HasPrefix(a.Value, "#") && !strings.ContainsAny(a.Value, " ()") {
						attrs["href"] = a.Value
					} else {
						removed["href="+truncate(a.Value, 24)]++
					}
					continue
				case a.Name.Space != "":
					removed[a.Name.Local+" (foreign attribute)"]++
					continue
				case strings.HasPrefix(strings.ToLower(an), "on"):
					removed[an]++
					continue
				case an == "style":
					if st := cleanStyle(a.Value); st != "" {
						attrs["style"] = st
					}
					if cleanStyle(a.Value) != strings.Join(strings.Fields(a.Value), "") {
						removed["style declarations"]++
					}
					continue
				case !svgAttrs[an]:
					removed[an]++
					continue
				case !safeValue(a.Value):
					removed[an+" value"]++
					continue
				}
				attrs[an] = a.Value
			}
			if depth == 1 {
				w, h = svgSize(attrs)
			}
			keys := make([]string, 0, len(attrs))
			for k := range attrs {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				out.WriteString(" " + k + `="` + escAttr(attrs[k]) + `"`)
			}
			out.WriteString(">")
		case xml.EndElement:
			depth--
			if skip > 0 {
				skip--
				continue
			}
			out.WriteString("</" + t.Name.Local + ">")
			if depth == 0 {
				rootClosed = true
			}
		case xml.CharData:
			if skip > 0 || depth == 0 {
				continue
			}
			_ = xml.EscapeText(&out, t)
		case xml.Comment:
			// dropped silently
		case xml.ProcInst:
			if t.Target != "xml" {
				removed["processing instruction"]++
			}
		case xml.Directive:
			removed["DOCTYPE / directive"]++
		}
	}
	if !rootSeen {
		return nil, errors.New("no <svg> element found")
	}
	list := make([]string, 0, len(removed))
	for k, n := range removed {
		if n > 1 {
			k += " ×" + strconv.Itoa(n)
		}
		list = append(list, k)
	}
	sort.Strings(list)
	return &SanitizedSVG{SVG: out.String(), Width: w, Height: h, Removed: list}, nil
}

// svgSize takes the size from width/height, else from the viewBox, else 100 × 100.
func svgSize(attrs map[string]string) (float64, float64) {
	num := func(s string) float64 {
		if m := numRe.FindStringSubmatch(s); m != nil {
			f, _ := strconv.ParseFloat(m[1], 64)
			return f
		}
		return 0
	}
	w, h := num(attrs["width"]), num(attrs["height"])
	if w <= 0 || h <= 0 || strings.Contains(attrs["width"], "%") {
		if f := strings.Fields(strings.ReplaceAll(attrs["viewBox"], ",", " ")); len(f) == 4 {
			w, h = num(f[2]), num(f[3])
		}
	}
	if w <= 0 || h <= 0 || w > 100000 || h > 100000 {
		w, h = 100, 100
	}
	return w, h
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
