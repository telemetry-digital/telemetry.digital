package setup

import (
	"bytes"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// The keys of [analytics] an installation writes; threads, max_cameras and anything else in the section stay.
var managedKeys = []string{"ffmpeg", "onnxruntime", "model_nano", "model_tiny", "nano_sha256", "tiny_sha256"}

var (
	sectionRe = regexp.MustCompile(`^\s*\[\s*analytics\s*\]\s*(#.*)?$`)
	anyHeadRe = regexp.MustCompile(`^\s*\[`)
)

func keyOf(line string) string {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") {
		return ""
	}
	k, _, ok := strings.Cut(t, "=")
	if !ok {
		return ""
	}
	return strings.Trim(strings.TrimSpace(k), `"`)
}

func tomlString(s string) string { return strconv.Quote(filepath.ToSlash(s)) }

// UpdateConfig returns config.toml with the [analytics] paths of an installation (p) or without them (p nil, after a
// removal). Other keys of the section (threads, max_cameras), other sections, comments, the line endings and a
// byte-order mark stay as they are. note becomes a comment above the paths when the section is created.
func UpdateConfig(src []byte, p *Paths, note string) []byte {
	bom := bytes.HasPrefix(src, []byte("\xef\xbb\xbf"))
	text := string(bytes.TrimPrefix(src, []byte("\xef\xbb\xbf")))
	nl := "\n"
	if strings.Contains(text, "\r\n") {
		nl = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	values := map[string]string{}
	if p != nil {
		values = map[string]string{"ffmpeg": tomlString(p.FFmpeg), "onnxruntime": tomlString(p.ONNXRuntime), "model_nano": tomlString(p.ModelNano),
			"model_tiny": tomlString(p.ModelTiny), "nano_sha256": strconv.Quote(p.NanoSHA256), "tiny_sha256": strconv.Quote(p.TinySHA256)}
	}
	start, end := -1, len(lines)
	for i, l := range lines {
		if start < 0 {
			if sectionRe.MatchString(l) {
				start = i
			}
			continue
		}
		if anyHeadRe.MatchString(l) {
			end = i
			break
		}
	}
	var out []string
	if start < 0 {
		if p == nil {
			return src
		}
		out = append(out, lines...)
		for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
			out = out[:len(out)-1]
		}
		out = append(out, "", "[analytics]")
		if note != "" {
			out = append(out, "# "+note)
		}
		for _, k := range managedKeys {
			out = append(out, k+" = "+values[k])
		}
		out = append(out, "threads = 1          # inference threads per camera", "max_cameras = 4      # cameras analysed at the same time", "")
	} else {
		out = append(out, lines[:start+1]...)
		body := lines[start+1 : end]
		// the managed keys go where the first of them was (or right below the header)
		insertAt := -1
		var kept []string
		for _, l := range body {
			if isManaged(keyOf(l)) {
				if insertAt < 0 {
					insertAt = len(kept)
				}
				continue
			}
			kept = append(kept, l)
		}
		if insertAt < 0 {
			insertAt = 0
			for insertAt < len(kept) && strings.HasPrefix(strings.TrimSpace(kept[insertAt]), "#") {
				insertAt++ // after the comments that open the section
			}
		}
		var ins []string
		if p != nil {
			for _, k := range managedKeys {
				ins = append(ins, k+" = "+values[k])
			}
		}
		body = append(append(append([]string{}, kept[:insertAt]...), ins...), kept[insertAt:]...)
		out = append(out, body...)
		out = append(out, lines[end:]...)
	}
	res := strings.Join(out, nl)
	if bom {
		res = "\xef\xbb\xbf" + res
	}
	return []byte(res)
}

func isManaged(k string) bool {
	for _, m := range managedKeys {
		if k == m {
			return true
		}
	}
	return false
}
