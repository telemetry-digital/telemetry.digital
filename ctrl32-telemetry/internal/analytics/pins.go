package analytics

import "path"

// Pinned third-party parts of object counting on camera pictures (ADR-0022): ONNX Runtime (MIT), an unmodified LGPL
// build of ffmpeg and the YOLOX nano/tiny models (Apache-2.0). They are downloaded at install time from their official
// releases over HTTPS and kept only when the SHA-256 matches the pin; they are never compiled into our binary. Since
// 0.76.0 the offline update package and a mirror of the distribution point also carry them unmodified (analytics/,
// with their licences and SOURCES.txt) for servers without the Internet; the same pins apply (ADR-0024).
//
// This file is the one place of the pins. deploy/install.sh and deploy/install.ps1 carry the same values for an install
// without the server (TestInstallerPinsMatch keeps all three in sync); the server agent installs from these
// (System → Server and Object counting → Cameras, "Install the detector"; internal/analytics/setup).
const (
	ORTVersion    = "1.30.0"                       // github.com/microsoft/onnxruntime releases
	FFmpegRelease = "autobuild-2026-08-31-13-27"   // github.com/BtbN/FFmpeg-Builds (month-end builds are kept)
	FFmpegBuild   = "ffmpeg-n8.1.2-50-g1a748fe2cd" // FFmpeg 8.1.2, LGPL v3 ("lgpl-shared": no --enable-gpl)
	FFmpegBranch  = "8.1"
	YOLOXRelease  = "0.1.1rc0" // github.com/Megvii-BaseDetection/YOLOX releases

	YOLOXNanoSHA256    = "c789161ed43c8269fcd4e67c67eeeb4e80c622da2eb296a20bc6007bd18a0b7d"
	YOLOXTinySHA256    = "427cc366d34e27ff7a03e2899b5e3671425c262ea2291f88bb942bc1cc70b0f7"
	YOLOXLicenseSHA256 = "577c03d505ec80f667ebf96ebd0cc4f6825c817ca1088ee348c48aaabd51bd92"
)

// ORTSHA256 and FFmpegSHA256 pin the archive of each platform ("os/arch").
var (
	ORTSHA256 = map[string]string{
		"linux/amd64":   "a5ed5a3cac51fbb2e90da632ae43d19212faaa20e76484e62bcb7c23ddb3b3fd", // onnxruntime-linux-x64-1.30.0.tgz
		"linux/arm64":   "e16a27a8ed330bbc698df7330b0cf56e722f354e3bcc92118682c74ef3c3e3da", // onnxruntime-linux-aarch64-1.30.0.tgz
		"windows/amd64": "c6ba983baf5681af108599675d2a89c2d145512d02de28aed0bff177cd0ba949", // onnxruntime-win-x64-1.30.0.zip
	}
	FFmpegSHA256 = map[string]string{
		"linux/amd64":   "5634d61d98fd647acb3b2d8f08f20e7ab858c5f5341f1cea36069d8ef45c48c4", // …-linux64-lgpl-shared-8.1.tar.xz
		"linux/arm64":   "ba53bfc17b496a07c432c8a06644195cbf2a38e7820844c377e489ee84354077", // …-linuxarm64-lgpl-shared-8.1.tar.xz
		"windows/amd64": "e9712ffbdb03ef71bbab660c75b835bfe698ef6fad0247c76d8d394a39a3db63", // …-win64-lgpl-shared-8.1.zip
	}
)

// InstallPlatforms are the platforms the detector can be installed on from the user interface.
var InstallPlatforms = []string{"linux/amd64", "linux/arm64", "windows/amd64"}

// Archive formats of an Artifact.
const (
	FormatFile  = "file"   // a single file kept as it is
	FormatTarGz = "tar.gz" // gzip-compressed tar
	FormatTarXz = "tar.xz" // xz-compressed tar
	FormatZip   = "zip"
)

// Artifact is one pinned download.
type Artifact struct {
	Name   string `json:"name"`   // ffmpeg, onnxruntime, model_nano, model_tiny, model_licence
	Label  string `json:"label"`  // shown in the progress
	URL    string `json:"url"`    // official release, HTTPS
	SHA256 string `json:"sha256"` // lower-case hex
	Format string `json:"format"`
}

// MirrorName is the file name of a part on a mirror of the distribution point and in an offline update package
// (analytics/<name>, ADR-0024): the file name of its official download, the YOLOX licence with its release. The pin,
// not the source, decides whether a file is kept.
func MirrorName(a Artifact) string {
	if a.Name == "model_licence" {
		return "YOLOX-" + YOLOXRelease + "-LICENSE"
	}
	return path.Base(a.URL)
}

// PinnedArtifacts lists the downloads for a platform ("os/arch") in install order; nil when the platform is not one of
// InstallPlatforms.
func PinnedArtifacts(platform string) []Artifact {
	ort, ff := ORTSHA256[platform], FFmpegSHA256[platform]
	if ort == "" || ff == "" {
		return nil
	}
	gh := "https://github.com/"
	var ortURL, ffURL, ortFmt, ffFmt string
	switch platform {
	case "linux/amd64", "linux/arm64":
		oarch, farch := "x64", "linux64"
		if platform == "linux/arm64" {
			oarch, farch = "aarch64", "linuxarm64"
		}
		ortURL = gh + "microsoft/onnxruntime/releases/download/v" + ORTVersion + "/onnxruntime-linux-" + oarch + "-" + ORTVersion + ".tgz"
		ffURL = gh + "BtbN/FFmpeg-Builds/releases/download/" + FFmpegRelease + "/" + FFmpegBuild + "-" + farch + "-lgpl-shared-" + FFmpegBranch + ".tar.xz"
		ortFmt, ffFmt = FormatTarGz, FormatTarXz
	case "windows/amd64":
		ortURL = gh + "microsoft/onnxruntime/releases/download/v" + ORTVersion + "/onnxruntime-win-x64-" + ORTVersion + ".zip"
		ffURL = gh + "BtbN/FFmpeg-Builds/releases/download/" + FFmpegRelease + "/" + FFmpegBuild + "-win64-lgpl-shared-" + FFmpegBranch + ".zip"
		ortFmt, ffFmt = FormatZip, FormatZip
	}
	yb := gh + "Megvii-BaseDetection/YOLOX/releases/download/" + YOLOXRelease + "/"
	return []Artifact{
		{Name: "onnxruntime", Label: "ONNX Runtime " + ORTVersion, URL: ortURL, SHA256: ort, Format: ortFmt},
		{Name: "ffmpeg", Label: "ffmpeg " + FFmpegBuild[len("ffmpeg-"):] + " (LGPL)", URL: ffURL, SHA256: ff, Format: ffFmt},
		{Name: "model_nano", Label: "YOLOX nano", URL: yb + "yolox_nano.onnx", SHA256: YOLOXNanoSHA256, Format: FormatFile},
		{Name: "model_tiny", Label: "YOLOX tiny", URL: yb + "yolox_tiny.onnx", SHA256: YOLOXTinySHA256, Format: FormatFile},
		{Name: "model_licence", Label: "YOLOX licence", URL: "https://raw.githubusercontent.com/Megvii-BaseDetection/YOLOX/" + YOLOXRelease + "/LICENSE", SHA256: YOLOXLicenseSHA256, Format: FormatFile},
	}
}

// OfficialHosts are the only hosts the pinned downloads start from (GitHub releases redirect to its object storage,
// also HTTPS); the SHA-256 pin decides whether a file is kept.
var OfficialHosts = []string{"github.com", "raw.githubusercontent.com"}
