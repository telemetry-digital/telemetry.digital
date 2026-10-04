package analytics

import (
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The installers carry the same pins as pins.go: an install from the command line and one from the user interface
// download exactly the same files.
func TestInstallerPinsMatch(t *testing.T) {
	sh := readInstaller(t, "../../deploy/install.sh")
	ps := readInstaller(t, "../../deploy/install.ps1")
	shVar := func(name string) string {
		m := regexp.MustCompile(`(?m)^` + name + `=(\S+)`).FindStringSubmatch(sh)
		if m == nil {
			t.Fatalf("install.sh: %s not found", name)
		}
		return m[1]
	}
	psVar := func(name string) string {
		m := regexp.MustCompile(`(?m)^\$` + name + ` = "([^"]+)"`).FindStringSubmatch(ps)
		if m == nil {
			t.Fatalf("install.ps1: $%s not found", name)
		}
		return m[1]
	}
	for _, c := range []struct{ name, got, want string }{
		{"install.sh ORT_VERSION", shVar("ORT_VERSION"), ORTVersion},
		{"install.sh ORT_SHA256_amd64", shVar("ORT_SHA256_amd64"), ORTSHA256["linux/amd64"]},
		{"install.sh ORT_SHA256_arm64", shVar("ORT_SHA256_arm64"), ORTSHA256["linux/arm64"]},
		{"install.sh FFMPEG_RELEASE", shVar("FFMPEG_RELEASE"), FFmpegRelease},
		{"install.sh FFMPEG_BUILD", shVar("FFMPEG_BUILD"), FFmpegBuild},
		{"install.sh FFMPEG_BRANCH", shVar("FFMPEG_BRANCH"), FFmpegBranch},
		{"install.sh FFMPEG_SHA256_amd64", shVar("FFMPEG_SHA256_amd64"), FFmpegSHA256["linux/amd64"]},
		{"install.sh FFMPEG_SHA256_arm64", shVar("FFMPEG_SHA256_arm64"), FFmpegSHA256["linux/arm64"]},
		{"install.sh YOLOX_RELEASE", shVar("YOLOX_RELEASE"), YOLOXRelease},
		{"install.sh YOLOX_NANO_SHA256", shVar("YOLOX_NANO_SHA256"), YOLOXNanoSHA256},
		{"install.sh YOLOX_TINY_SHA256", shVar("YOLOX_TINY_SHA256"), YOLOXTinySHA256},
		{"install.sh YOLOX_LICENSE_SHA256", shVar("YOLOX_LICENSE_SHA256"), YOLOXLicenseSHA256},
		{"install.ps1 OrtVersion", psVar("OrtVersion"), ORTVersion},
		{"install.ps1 OrtSha256", psVar("OrtSha256"), ORTSHA256["windows/amd64"]},
		{"install.ps1 FfmpegRelease", psVar("FfmpegRelease"), FFmpegRelease},
		{"install.ps1 FfmpegBuild", psVar("FfmpegBuild"), FFmpegBuild},
		{"install.ps1 FfmpegBranch", psVar("FfmpegBranch"), FFmpegBranch},
		{"install.ps1 FfmpegSha256", psVar("FfmpegSha256"), FFmpegSHA256["windows/amd64"]},
		{"install.ps1 YoloxRelease", psVar("YoloxRelease"), YOLOXRelease},
		{"install.ps1 YoloxNanoSha256", psVar("YoloxNanoSha256"), YOLOXNanoSHA256},
		{"install.ps1 YoloxTinySha256", psVar("YoloxTinySha256"), YOLOXTinySHA256},
		{"install.ps1 YoloxLicenseSha256", psVar("YoloxLicenseSha256"), YOLOXLicenseSHA256},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, pins.go has %q", c.name, c.got, c.want)
		}
	}
	// the download addresses are built the same way as in pins.go
	if !strings.Contains(sh, `onnxruntime-linux-$oarch-$ORT_VERSION.tgz`) || !strings.Contains(sh, `$FFMPEG_BUILD-$farch-lgpl-shared-$FFMPEG_BRANCH`) ||
		!strings.Contains(ps, `onnxruntime-win-x64-$OrtVersion.zip`) || !strings.Contains(ps, `$FfmpegBuild-win64-lgpl-shared-$FfmpegBranch.zip`) {
		t.Error("the installers no longer build the archive names that pins.go builds")
	}
}

func readInstaller(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// Every platform has five pinned downloads, over HTTPS from the official hosts, with a SHA-256 pin.
func TestPinnedArtifacts(t *testing.T) {
	hex := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for _, p := range InstallPlatforms {
		as := PinnedArtifacts(p)
		if len(as) != 5 {
			t.Fatalf("%s: %d artifacts", p, len(as))
		}
		for _, a := range as {
			u, err := url.Parse(a.URL)
			if err != nil || u.Scheme != "https" || !slices.Contains(OfficialHosts, u.Host) {
				t.Errorf("%s %s: %s is not an official HTTPS address", p, a.Name, a.URL)
			}
			if !hex.MatchString(a.SHA256) {
				t.Errorf("%s %s: pin %q", p, a.Name, a.SHA256)
			}
		}
		want := FormatTarXz
		if strings.HasPrefix(p, "windows/") {
			want = FormatZip
		}
		if as[1].Name != "ffmpeg" || as[1].Format != want {
			t.Errorf("%s: ffmpeg %s %s", p, as[1].Name, as[1].Format)
		}
	}
	if PinnedArtifacts("darwin/arm64") != nil || PinnedArtifacts("windows/arm64") != nil {
		t.Error("an unsupported platform has pins")
	}
}
