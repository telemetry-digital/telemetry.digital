package video

import (
	"bytes"
	"testing"

	"github.com/telemetry-digital/ctrl32-telemetry/internal/video/testsrc"
)

// A camera that rewrites its SPS (here only the VUI timing: 25 → 1 fps in an idle-scene mode) keeps its stream: the
// published Track stays, the new SPS travels in front of the key frame. A new picture size starts over.
func TestParamsChangeKeepsTheStream(t *testing.T) {
	e25 := &testsrc.Encoder{W: 64, H: 48, FPS: 25}
	e1 := &testsrc.Encoder{W: 64, H: 48, FPS: 1}
	if bytes.Equal(e25.SPS(), e1.SPS()) {
		t.Fatal("the two SPS must differ")
	}
	tr := &Track{Codec: H264, SPS: e25.SPS(), PPS: e25.PPS()}
	tr.size()
	vp := &videoParams{pub: tr, cur: tr}

	out, restart := vp.take(e25.Frame())
	if restart || vp.pub != tr || len(out) != 1 {
		t.Fatalf("same sets: restart %v, out %d", restart, len(out))
	}
	out, restart = vp.take(e1.Frame())
	if restart || vp.pub != tr {
		t.Fatal("a timing-only SPS change must not start the stream over")
	}
	if !bytes.Equal(vp.cur.SPS, e1.SPS()) {
		t.Fatal("the new SPS is not followed")
	}
	key := withParams(true, vp.cur, out)
	if !bytes.Equal(key[0], e1.SPS()) || !hasParams(H264, key) {
		t.Fatal("the key frame must carry the new SPS in front")
	}
	if a := AnnexB(vp.pub, &Sample{Key: true, AU: key}); bytes.Count(a, []byte{0, 0, 0, 1, 0x67}) != 1 {
		t.Fatal("the detector stream must carry the SPS once")
	}

	big := &testsrc.Encoder{W: 128, H: 96, FPS: 1}
	if _, restart = vp.take(big.Frame()); !restart || vp.pub == tr || vp.pub.Width != 128 {
		t.Fatal("a new picture size must start over")
	}
}
