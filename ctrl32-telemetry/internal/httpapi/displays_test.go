package httpapi

import (
	"strings"
	"testing"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/iam"
)

func TestPairingCode(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		c, err := randomCode()
		if err != nil || len(c) != 8 {
			t.Fatalf("code %q %v", c, err)
		}
		if strings.ContainsAny(c, "01OI") {
			t.Fatalf("ambiguous character in %q", c)
		}
		seen[c] = true
	}
	if len(seen) < 199 {
		t.Fatal("codes repeat")
	}
	if got := normalizeCode(" abcd-efgh\n"); got != "ABCDEFGH" {
		t.Fatalf("normalize: %q", got)
	}
	if got := normalizeCode("ab'; drop--"); got != "ABDRP" {
		t.Fatalf("normalize keeps only the alphabet: %q", got)
	}
}

func TestCameraGroups(t *testing.T) {
	all := &iam.Principal{}
	if !all.SeesCameraGroup("anything") || !all.SeesCameraGroup("") {
		t.Fatal("an unlimited user sees every camera")
	}
	gate := &iam.Principal{CameraLimited: true, CameraGroups: []string{"gate"}}
	if !gate.SeesCameraGroup("gate") || gate.SeesCameraGroup("yard") || gate.SeesCameraGroup("") {
		t.Fatal("a limited user sees only their groups")
	}
	none := &iam.Principal{CameraLimited: true}
	if none.SeesCameraGroup("") {
		t.Fatal("limited to no group: no camera")
	}
	var nobody *iam.Principal
	if !nobody.SeesCameraGroup("x") {
		t.Fatal("system calls without a principal are not limited")
	}
}

func TestSpecCameras(t *testing.T) {
	validated := map[string]any{"widgets": []map[string]any{{"type": "camera", "params": map[string]any{"camera": "a"}}, {"type": "chart"}}}
	stored := map[string]any{"widgets": []any{map[string]any{"type": "camera", "params": map[string]any{"camera": "b"}}}}
	if got := specCameras(validated); len(got) != 1 || got[0] != "a" {
		t.Fatalf("validated spec: %v", got)
	}
	if got := specCameras(stored); len(got) != 1 || got[0] != "b" {
		t.Fatalf("stored spec: %v", got)
	}
}
