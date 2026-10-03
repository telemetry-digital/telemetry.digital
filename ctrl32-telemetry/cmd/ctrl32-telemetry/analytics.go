package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/analytics"
	"github.com/telemetry-digital/ctrl32-telemetry/internal/config"
)

// isSelfCheck reports whether the analytics-worker arguments ask for the self-check.
func isSelfCheck(args []string) bool {
	for _, a := range args {
		if a == "-selfcheck" || a == "--selfcheck" {
			return true
		}
	}
	return false
}

// cmdAnalyticsSelfCheck checks the detector installation (ffmpeg, ONNX Runtime, models):
//
//	ctrl32-telemetry analytics-worker -selfcheck [-config config.toml] [-json]
//
// The server runs it with -paths (the [analytics] settings as JSON) and -json, so ONNX Runtime is never loaded into
// the server process. The exit code is 0 when cameras can count, 1 otherwise.
func cmdAnalyticsSelfCheck(args []string) int {
	fs := flag.NewFlagSet("analytics-worker -selfcheck", flag.ContinueOnError)
	fs.Bool("selfcheck", true, "check the detector installation")
	path := fs.String("config", config.DefaultPath(), "path to config.toml (its [analytics] section)")
	paths := fs.String("paths", "", "the [analytics] settings as JSON instead of -config")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var p analytics.CheckPaths
	if *paths != "" {
		if err := json.Unmarshal([]byte(*paths), &p); err != nil {
			fmt.Fprintln(os.Stderr, "paths:", err)
			return 2
		}
	} else {
		// only the [analytics] section: the check must work even when other settings are incomplete
		var c struct {
			Analytics config.AnalyticsConfig `toml:"analytics"`
		}
		if _, err := toml.DecodeFile(*path, &c); err != nil {
			fmt.Fprintln(os.Stderr, "config:", err)
			return 2
		}
		a := c.Analytics
		p = analytics.CheckPaths{FFmpeg: a.FFmpeg, ONNXRuntime: a.ONNXRuntime, ModelNano: a.ModelNano, ModelTiny: a.ModelTiny, NanoSHA256: a.NanoSHA256, TinySHA256: a.TinySHA256}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res := analytics.SelfCheck(ctx, p)
	if *asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(res)
	} else {
		names := map[string]string{"ffmpeg": "ffmpeg", "onnxruntime": "ONNX Runtime", "model_nano": "model nano", "model_tiny": "model tiny"}
		for _, c := range res.Components {
			info := c.Version
			if c.Licence != "" {
				info += " (" + c.Licence + ")"
			}
			if c.Millis > 0 {
				info += fmt.Sprintf(", test inference %d ms", c.Millis)
			}
			line := fmt.Sprintf("%-13s %-15s %s", names[c.Name], strings.ReplaceAll(c.State, "_", " "), strings.TrimSpace(info))
			if c.Path != "" {
				line += "  " + c.Path
			}
			fmt.Println(strings.TrimRight(line, " "))
			if c.Detail != "" {
				fmt.Println("              " + c.Detail)
			}
		}
		if res.Ready {
			fmt.Println("Object counting on cameras: ready (" + res.Platform + ")")
		} else {
			fmt.Println("Object counting on cameras: NOT ready (" + res.Platform + ") - install with install.sh --analytics or install.ps1 -Analytics, or set [analytics] in config.toml")
		}
	}
	if !res.Ready {
		return 1
	}
	return 0
}
