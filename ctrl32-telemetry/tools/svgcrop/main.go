// svgcrop computes the bounding box of every <path d="..."> in an SVG (with translate() transforms of enclosing
// groups) and writes a copy whose width/height/viewBox are cropped to the content plus a margin.
//
//	go run ./tools/svgcrop -in logo.svg -out logo-cropped.svg -margin 2
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var (
	pathRe      = regexp.MustCompile(`(?s)<path[^>]*?\sd="([^"]*)"`)
	transformRe = regexp.MustCompile(`translate\(\s*(-?[0-9.eE+-]+)\s*[, ]\s*(-?[0-9.eE+-]+)\s*\)`)
	numRe       = regexp.MustCompile(`-?(?:\d+\.?\d*|\.\d+)(?:[eE][-+]?\d+)?`)
)

type bbox struct{ minX, minY, maxX, maxY float64 }

func (b *bbox) add(x, y float64) {
	b.minX, b.minY = math.Min(b.minX, x), math.Min(b.minY, y)
	b.maxX, b.maxY = math.Max(b.maxX, x), math.Max(b.maxY, y)
}

// pathBounds walks an SVG path (M L H V C S Q T Z, absolute and relative); control points are included.
func pathBounds(d string, b *bbox, tx, ty float64) {
	tokens := regexp.MustCompile(`[MmLlHhVvCcSsQqTtAaZz]|`+numRe.String()).FindAllString(d, -1)
	var cx, cy, sx, sy float64
	cmd := ""
	i := 0
	next := func() float64 { v, _ := strconv.ParseFloat(tokens[i], 64); i++; return v }
	for i < len(tokens) {
		t := tokens[i]
		if len(t) == 1 && strings.ContainsAny(t, "MmLlHhVvCcSsQqTtAaZz") {
			cmd = t
			i++
			if cmd == "Z" || cmd == "z" {
				cx, cy = sx, sy
				continue
			}
		}
		if i >= len(tokens) {
			break
		}
		rel := cmd >= "a" && cmd <= "z"
		switch strings.ToUpper(cmd) {
		case "M":
			x, y := next(), next()
			if rel {
				x, y = cx+x, cy+y
			}
			cx, cy, sx, sy = x, y, x, y
			b.add(cx+tx, cy+ty)
			if cmd == "m" {
				cmd = "l"
			} else {
				cmd = "L"
			}
		case "L", "T":
			x, y := next(), next()
			if rel {
				x, y = cx+x, cy+y
			}
			cx, cy = x, y
			b.add(cx+tx, cy+ty)
		case "H":
			x := next()
			if rel {
				x = cx + x
			}
			cx = x
			b.add(cx+tx, cy+ty)
		case "V":
			y := next()
			if rel {
				y = cy + y
			}
			cy = y
			b.add(cx+tx, cy+ty)
		case "C":
			for k := 0; k < 3; k++ {
				x, y := next(), next()
				if rel {
					x, y = cx+x, cy+y
				}
				b.add(x+tx, y+ty)
				if k == 2 {
					cx, cy = x, y
				}
			}
		case "S", "Q":
			for k := 0; k < 2; k++ {
				x, y := next(), next()
				if rel {
					x, y = cx+x, cy+y
				}
				b.add(x+tx, y+ty)
				if k == 1 {
					cx, cy = x, y
				}
			}
		case "A":
			next()
			next()
			next()
			next()
			next()
			x, y := next(), next()
			if rel {
				x, y = cx+x, cy+y
			}
			cx, cy = x, y
			b.add(cx+tx, cy+ty)
		default:
			i++
		}
	}
}

func main() {
	in := flag.String("in", "", "input svg")
	out := flag.String("out", "", "output svg (cropped)")
	margin := flag.Float64("margin", 2, "margin in user units")
	flag.Parse()
	data, err := os.ReadFile(*in)
	if err != nil {
		panic(err)
	}
	s := string(data)
	// translate() of the nearest enclosing <g>; nested translates are summed while walking the document
	b := bbox{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	pos := 0
	var stack []float64 // pairs tx,ty
	tagRe := regexp.MustCompile(`(?s)<(/?)(g|path)\b([^>]*)>`)
	for _, m := range tagRe.FindAllStringSubmatchIndex(s, -1) {
		closing := s[m[2]:m[3]] == "/"
		name := s[m[4]:m[5]]
		attrs := s[m[6]:m[7]]
		if name == "g" {
			if closing {
				if len(stack) >= 2 {
					stack = stack[:len(stack)-2]
				}
			} else if !strings.HasSuffix(strings.TrimSpace(attrs), "/") {
				tx, ty := 0.0, 0.0
				if t := transformRe.FindStringSubmatch(attrs); t != nil {
					tx, _ = strconv.ParseFloat(t[1], 64)
					ty, _ = strconv.ParseFloat(t[2], 64)
				}
				stack = append(stack, tx, ty)
			}
			continue
		}
		if closing {
			continue
		}
		tx, ty := 0.0, 0.0
		for k := 0; k+1 < len(stack); k += 2 {
			tx += stack[k]
			ty += stack[k+1]
		}
		if t := transformRe.FindStringSubmatch(attrs); t != nil {
			x, _ := strconv.ParseFloat(t[1], 64)
			y, _ := strconv.ParseFloat(t[2], 64)
			tx, ty = tx+x, ty+y
		}
		if dm := regexp.MustCompile(`\sd="([^"]*)"`).FindStringSubmatch(attrs); dm != nil {
			pathBounds(dm[1], &b, tx, ty)
		}
		pos = m[1]
	}
	_ = pos
	fmt.Fprintf(os.Stderr, "bbox: x %.3f..%.3f y %.3f..%.3f (w %.3f h %.3f)\n", b.minX, b.maxX, b.minY, b.maxY, b.maxX-b.minX, b.maxY-b.minY)
	if *out == "" {
		return
	}
	x, y := b.minX-*margin, b.minY-*margin
	w, h := b.maxX-b.minX+2**margin, b.maxY-b.minY+2**margin
	head := regexp.MustCompile(`(?s)<svg\b[^>]*>`).FindStringIndex(s)
	tag := s[head[0]:head[1]]
	tag = regexp.MustCompile(`\swidth="[^"]*"`).ReplaceAllString(tag, "")
	tag = regexp.MustCompile(`\sheight="[^"]*"`).ReplaceAllString(tag, "")
	tag = regexp.MustCompile(`\sviewBox="[^"]*"`).ReplaceAllString(tag, "")
	tag = strings.Replace(tag, "<svg", fmt.Sprintf(`<svg width="%.3f" height="%.3f" viewBox="%.3f %.3f %.3f %.3f"`, w, h, x, y, w, h), 1)
	outS := s[:head[0]] + tag + s[head[1]:]
	// drop Inkscape/sodipodi noise so the file is small and clean
	outS = regexp.MustCompile(`(?s)<sodipodi:namedview.*?/>`).ReplaceAllString(outS, "")
	outS = regexp.MustCompile(`(?s)<metadata>.*?</metadata>`).ReplaceAllString(outS, "")
	outS = regexp.MustCompile(`\s(inkscape|sodipodi):[a-zA-Z-]+="[^"]*"`).ReplaceAllString(outS, "")
	outS = regexp.MustCompile(`\sxmlns:(inkscape|sodipodi)="[^"]*"`).ReplaceAllString(outS, "")
	if err := os.WriteFile(*out, []byte(outS), 0o644); err != nil {
		panic(err)
	}
	fmt.Println("wrote", *out)
}
