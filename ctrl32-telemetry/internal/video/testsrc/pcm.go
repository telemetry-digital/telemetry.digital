// Package testsrc produces a real H.264 video stream without an encoder library, for simulated cameras in tests and
// demos (tools/camsim): every frame is an IDR picture of I_PCM macroblocks (raw samples), which every H.264 decoder
// accepts. The picture shows moving colour bars and a bar that encodes the frame number.
package testsrc

import (
	"bytes"
	"image"
)

type bitWriter struct {
	buf  []byte
	cur  byte
	nbit uint
}

func (w *bitWriter) bit(b uint) {
	w.cur = w.cur<<1 | byte(b&1)
	w.nbit++
	if w.nbit == 8 {
		w.buf = append(w.buf, w.cur)
		w.cur, w.nbit = 0, 0
	}
}

func (w *bitWriter) bits(v uint64, n int) {
	for i := n - 1; i >= 0; i-- {
		w.bit(uint(v>>uint(i)) & 1)
	}
}

// ue writes an unsigned Exp-Golomb code.
func (w *bitWriter) ue(v uint) {
	v++
	n := 0
	for x := v; x > 1; x >>= 1 {
		n++
	}
	w.bits(0, n)
	w.bits(uint64(v), n+1)
}

func (w *bitWriter) se(v int) {
	if v <= 0 {
		w.ue(uint(-2 * v))
	} else {
		w.ue(uint(2*v - 1))
	}
}

func (w *bitWriter) align() {
	for w.nbit != 0 {
		w.bit(0)
	}
}

// trailing writes rbsp_trailing_bits.
func (w *bitWriter) trailing() []byte {
	w.bit(1)
	w.align()
	return w.buf
}

// nal wraps an RBSP into a NAL unit with emulation prevention.
func nal(header byte, rbsp []byte) []byte {
	out := []byte{header}
	zeros := 0
	for _, b := range rbsp {
		if zeros >= 2 && b <= 3 {
			out = append(out, 3)
			zeros = 0
		}
		out = append(out, b)
		if b == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return out
}

// Encoder makes I_PCM frames of a fixed size (multiples of 16).
type Encoder struct {
	W, H  int
	frame int
	idr   int
}

// SPS returns the sequence parameter set (Baseline profile, level 3.0).
func (e *Encoder) SPS() []byte {
	w := &bitWriter{}
	w.bits(66, 8) // profile_idc: baseline
	w.bits(0xC0, 8)
	w.bits(30, 8) // level_idc
	w.ue(0)       // seq_parameter_set_id
	w.ue(0)       // log2_max_frame_num_minus4
	w.ue(2)       // pic_order_cnt_type
	w.ue(1)       // max_num_ref_frames
	w.bit(0)      // gaps_in_frame_num_value_allowed_flag
	w.ue(uint(e.W/16 - 1))
	w.ue(uint(e.H/16 - 1))
	w.bit(1) // frame_mbs_only_flag
	w.bit(1) // direct_8x8_inference_flag
	w.bit(0) // frame_cropping_flag
	w.bit(0) // vui_parameters_present_flag
	return nal(0x67, w.trailing())
}

// PPS returns the picture parameter set.
func (e *Encoder) PPS() []byte {
	w := &bitWriter{}
	w.ue(0)  // pic_parameter_set_id
	w.ue(0)  // seq_parameter_set_id
	w.bit(0) // entropy_coding_mode_flag: CAVLC
	w.bit(0) // bottom_field_pic_order_in_frame_present_flag
	w.ue(0)  // num_slice_groups_minus1
	w.ue(0)  // num_ref_idx_l0_default_active_minus1
	w.ue(0)  // num_ref_idx_l1_default_active_minus1
	w.bit(0) // weighted_pred_flag
	w.bits(0, 2)
	w.se(0)  // pic_init_qp_minus26
	w.se(0)  // pic_init_qs_minus26
	w.se(0)  // chroma_qp_index_offset
	w.bit(1) // deblocking_filter_control_present_flag
	w.bit(0) // constrained_intra_pred_flag
	w.bit(0) // redundant_pic_cnt_present_flag
	return nal(0x68, w.trailing())
}

// Frame returns the next access unit of the built-in test picture: SPS, PPS and one IDR slice.
func (e *Encoder) Frame() [][]byte {
	n := e.frame
	e.frame++
	img := image.NewYCbCr(image.Rect(0, 0, e.W, e.H), image.YCbCrSubsampleRatio420)
	mbw, mbh := e.W/16, e.H/16
	for my := 0; my < mbh; my++ {
		for mx := 0; mx < mbw; mx++ {
			y, u, v := e.colour(mx, my, n)
			for py := 0; py < 16; py++ {
				for px := 0; px < 16; px++ {
					img.Y[img.YOffset(mx*16+px, my*16+py)] = y
					ci := img.COffset(mx*16+px, my*16+py)
					img.Cb[ci], img.Cr[ci] = u, v
				}
			}
		}
	}
	return e.Encode(img)
}

// Encode turns a 4:2:0 picture of the encoder's size into an access unit: SPS, PPS and one IDR slice of I_PCM
// macroblocks.
func (e *Encoder) Encode(img *image.YCbCr) [][]byte {
	n := e.idr
	e.idr++
	w := &bitWriter{}
	w.ue(0)           // first_mb_in_slice
	w.ue(7)           // slice_type: I (all slices)
	w.ue(0)           // pic_parameter_set_id
	w.bits(0, 4)      // frame_num
	w.ue(uint(n % 2)) // idr_pic_id
	w.bit(0)          // no_output_of_prior_pics_flag
	w.bit(0)          // long_term_reference_flag
	w.se(0)           // slice_qp_delta
	w.ue(1)           // disable_deblocking_filter_idc
	mbw, mbh := e.W/16, e.H/16
	for my := 0; my < mbh; my++ {
		for mx := 0; mx < mbw; mx++ {
			w.ue(25) // mb_type I_PCM
			w.align()
			for py := 0; py < 16; py++ {
				o := img.YOffset(mx*16, my*16+py)
				w.buf = append(w.buf, img.Y[o:o+16]...)
			}
			for _, plane := range [][]byte{img.Cb, img.Cr} {
				for py := 0; py < 8; py++ {
					o := img.COffset(mx*16, my*16+py*2)
					w.buf = append(w.buf, plane[o:o+8]...)
				}
			}
		}
	}
	slice := nal(0x65, w.trailing())
	return [][]byte{e.SPS(), e.PPS(), slice}
}

// colour of a macroblock: eight colour bars moving right, a white frame counter bar in the last row.
func (e *Encoder) colour(mx, my, n int) (y, u, v byte) {
	mbw, mbh := e.W/16, e.H/16
	if my == mbh-1 {
		if mx < (n % (mbw + 1)) {
			return 235, 128, 128
		}
		return 16, 128, 128
	}
	bars := [8][3]byte{{235, 128, 128}, {210, 16, 146}, {170, 166, 16}, {145, 54, 34}, {106, 202, 222}, {81, 90, 240}, {41, 240, 110}, {16, 128, 128}}
	b := bars[((mx+n)*8/mbw)%8]
	return b[0], b[1], b[2]
}

// Bytes is the size of one encoded frame (for bitrate estimates).
func (e *Encoder) Bytes() int {
	var b bytes.Buffer
	for _, p := range e.Frame() {
		b.Write(p)
	}
	e.frame--
	e.idr--
	return b.Len()
}
