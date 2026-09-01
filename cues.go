package depth3d

import (
	"image"
	"unsafe"

	"github.com/go-images/depth"
)

// cues is the portable path: depth guessed from the picture itself, and both
// views synthesised on the processor.
//
// No model, no GPU, no download. It runs on any machine and in a browser, and
// it is visibly not as good as a real depth network — which is why Describe
// says so rather than leaving a caller to wonder why the effect got weaker.
type cues struct {
	maxShift int
	soften   int
	curve    []byte
	closed   bool
}

func (c *cues) Describe() string {
	return "depth from cues in the picture, views on the processor"
}

// Close marks the converter spent. The accelerated path releases GPU buffers
// here and would crash if used afterwards; this one would quietly keep working,
// and two paths that disagree about that is how a bug survives a platform
// change.
func (c *cues) Close() { c.closed = true }

func (c *cues) Convert(left, right []uint32, stride int, src []uint32, srcStride, w, h int) error {
	if c.closed {
		return ErrNothingToConvert
	}
	if err := checkSizes(left, right, stride, src, srcStride, w, h); err != nil {
		return err
	}
	// The destinations are wrapped where they lie rather than copied into.
	// left and right are routinely the two halves of one side-by-side frame,
	// and an image.RGBA with the frame's stride addresses either half exactly.
	//
	// The decoder's frames are BGRA and image.RGBA reads them as RGBA, so red
	// and blue are transposed for the luminance term. That is left alone on
	// purpose: correcting it costs a copy of every frame, and what it changes
	// is the WEIGHTS of a greyscale that only has to say which parts of the
	// picture are bright. The pixels themselves move whole, so nothing that
	// reaches an eye is discoloured.
	in := &image.RGBA{Pix: asBytes(src), Stride: srcStride * 4, Rect: image.Rect(0, 0, w, h)}
	l := &image.RGBA{Pix: asBytes(left), Stride: stride * 4, Rect: image.Rect(0, 0, w, h)}
	r := &image.RGBA{Pix: asBytes(right), Stride: stride * 4, Rect: image.Rect(0, 0, w, h)}

	m := depth.Soften(depth.Cues(in), c.soften)
	if err := viewsInto(l, r, in, m, depth.Options{
		MaxShift: c.maxShift,
		Curve:    c.curve,
	}); err != nil {
		return ErrNothingToConvert
	}
	return nil
}

// checkSizes is the one place that decides whether a call can be served, so
// the two paths cannot disagree about what they accept.
func checkSizes(left, right []uint32, stride int, src []uint32, srcStride, w, h int) error {
	if w < 1 || h < 1 {
		return ErrNothingToConvert
	}
	if stride < w || srcStride < w {
		return ErrNothingToConvert
	}
	need := (h-1)*stride + w
	if len(left) < need || len(right) < need || len(src) < (h-1)*srcStride+w {
		return ErrNothingToConvert
	}
	return nil
}

// asBytes views a pixel buffer as bytes. The buffers come from a decoder or a
// GPU allocation and are at least four-byte aligned.
func asBytes(w []uint32) []byte {
	if len(w) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&w[0])), len(w)*4)
}

// asWords is the reverse of asBytes: a GPU buffer's bytes as pixels.
func asWords(b []byte) []uint32 {
	if len(b) < 4 {
		return nil
	}
	return unsafe.Slice((*uint32)(unsafe.Pointer(&b[0])), len(b)/4)
}

// viewsInto is the synthesis, behind a seam.
//
// checkSizes has already refused everything the call below can complain about,
// so its error is unreachable through this package as it stands — and that is
// exactly why it is not simply ignored. What guarantees it today is an
// invariant of ANOTHER module, asserted by its documentation; if that ever
// loosens, this must still refuse rather than draw something wrong. The seam
// lets a test prove the refusal without breaking the machine.
var viewsInto = depth.ViewsInto
