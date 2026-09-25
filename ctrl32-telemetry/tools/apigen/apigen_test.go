package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/httpapi"
)

// The committed clients must equal what the generator produces from the current API description.
func TestClientsAreCurrent(t *testing.T) {
	files := Generate(httpapi.OpenAPIDocument(""))
	if len(files) < 100 {
		t.Fatalf("only %d files generated", len(files))
	}
	var stale []string
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join("..", "..", "clients", filepath.FromSlash(name)))
		if err != nil || !bytes.Equal(got, want) {
			stale = append(stale, name)
		}
	}
	if len(stale) > 0 {
		if len(stale) > 10 {
			stale = append(stale[:10], "…")
		}
		t.Fatalf("clients are stale — run: go run ./tools/apigen\n  %s", strings.Join(stale, "\n  "))
	}
}

func TestTypeScriptShape(t *testing.T) {
	ts := string(Generate(httpapi.OpenAPIDocument(""))["typescript/ctrl32-api.ts"])
	for _, want := range []string{"export type Device = {", "export class Ctrl32Client", "getDevices(): Promise<Device[]>", "postDevicesIdCommands(id: string, body: {", "getObservations(query: { datastream_id: string;", "async *pages<T>"} {
		if !strings.Contains(ts, want) {
			t.Errorf("TypeScript client lacks %q", want)
		}
	}
	if strings.Contains(ts, "unknown[]") && strings.Count(ts, "unknown[]") > 5 {
		t.Errorf("too many untyped arrays")
	}
}
