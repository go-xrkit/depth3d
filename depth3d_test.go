package depth3d

import (
	"errors"
	"image"
	"strings"
	"testing"

	"github.com/go-images/depth"
)

func TestTheDefaultsAreTheMeasuredOnes(t *testing.T) {
	var o Options
	if got := o.maxShift(); got != 24 {
		t.Errorf("MaxShift defaulted to %d, want 24", got)
	}
	if got := o.soften(); got != 2 {
		t.Errorf("Soften defaulted to %d, want 2", got)
	}
	if got := (Options{MaxShift: -3}).maxShift(); got != 24 {
		t.Errorf("a negative MaxShift gave %d", got)
	}
	if got := (Options{Soften: -1}).soften(); got != 2 {
		t.Errorf("a negative Soften gave %d", got)
	}
	if got := (Options{MaxShift: 8, Soften: 5}); got.maxShift() != 8 || got.soften() != 5 {
		t.Errorf("explicit values were overridden: %d, %d", got.maxShift(), got.soften())
	}
}

func TestTheLogIsOptionalAndUsedWhenGiven(t *testing.T) {
	var o Options
	o.logf("this must not panic with no log %d", 1) // the common case

	var got []string
	o.Log = func(s string) { got = append(got, s) }
	o.logf("path %s chosen", "portable")
	if len(got) != 1 || !strings.Contains(got[0], "portable") {
		t.Fatalf("the log received %v", got)
	}
}

func TestASizeThatCannotBeServedIsRefusedRatherThanRead(t *testing.T) {
	// Every one of these would be an out-of-bounds read in the middle of a
	// frame loop, which is a crash in someone's compositor rather than an
	// error in their log.
	const w, h, stride = 8, 4, 8
	full := make([]uint32, stride*h)
	for _, tc := range []struct {
		name                    string
		left, right, src        []uint32
		stride, srcStride, w, h int
	}{
		{"no width", full, full, full, stride, stride, 0, h},
		{"no height", full, full, full, stride, stride, w, 0},
		{"a stride narrower than the picture", full, full, full, 4, stride, w, h},
		{"a source stride narrower than the picture", full, full, full, stride, 4, w, h},
		{"a left eye too small", full[:8], full, full, stride, stride, w, h},
		{"a right eye too small", full, full[:8], full, stride, stride, w, h},
		{"a source too small", full, full, full[:8], stride, stride, w, h},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkSizes(tc.left, tc.right, tc.stride, tc.src, tc.srcStride, tc.w, tc.h)
			if !errors.Is(err, ErrNothingToConvert) {
				t.Fatalf("it was accepted (%v)", err)
			}
		})
	}
	// The negative control: the same call, valid, is served.
	if err := checkSizes(full, full, stride, full, stride, w, h); err != nil {
		t.Fatalf("a valid call was refused: %v", err)
	}
}

func TestTheHalvesOfOneFrameAreBothWritten(t *testing.T) {
	// The two eyes are routinely the two halves of ONE side-by-side frame,
	// addressed by the same stride. If either half were written at the wrong
	// offset the picture would still look plausible in one eye.
	const w, h = 48, 16
	frame := make([]uint32, 2*w*h)
	src := make([]uint32, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Detail low in the frame, flat sky above, and every column
			// distinct so a shift is visible.
			v := uint32(x * 5 % 256)
			if y < h/3 {
				v = 40
			}
			src[y*w+x] = 0xFF000000 | v<<16 | v<<8 | v
		}
	}
	c := newCues(Options{MaxShift: 24})
	defer c.Close()
	if c.Describe() == "" {
		t.Error("the converter does not say what it is")
	}
	if err := c.Convert(frame[0:], frame[w:], 2*w, src, w, w, h); err != nil {
		t.Fatal(err)
	}
	blank := 0
	same := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			l, r := frame[y*2*w+x], frame[y*2*w+w+x]
			if l == 0 || r == 0 {
				blank++
			}
			if l == r {
				same++
			}
		}
	}
	if blank != 0 {
		t.Fatalf("%d pixels were never written", blank)
	}
	if same == w*h {
		t.Fatal("the two halves are identical; nothing was converted")
	}
}

func TestTheCuePathRefusesAFrameItCannotUse(t *testing.T) {
	c := newCues(Options{})
	defer c.Close()
	if err := c.Convert(nil, nil, 0, nil, 0, 0, 0); !errors.Is(err, ErrNothingToConvert) {
		t.Fatalf("a call with nothing in it gave %v", err)
	}
}

func TestTheWordAndByteViewsAreEmptySafely(t *testing.T) {
	if asBytes(nil) != nil {
		t.Error("asBytes invented a slice")
	}
	if asWords(nil) != nil || asWords([]byte{1, 2}) != nil {
		t.Error("asWords invented a slice")
	}
	if got := asWords(asBytes([]uint32{0x11223344})); len(got) != 1 || got[0] != 0x11223344 {
		t.Errorf("a round trip through bytes gave %v", got)
	}
}

func TestAClosedConverterRefusesRatherThanQuietlyWorking(t *testing.T) {
	// The accelerated path releases GPU buffers on Close and would crash if
	// used afterwards. This one holds nothing and would keep working, and two
	// paths that disagree about that is how a bug survives a platform change.
	const w, h = 16, 8
	frame := make([]uint32, 2*w*h)
	src := make([]uint32, w*h)
	c := newCues(Options{})
	if err := c.Convert(frame[0:], frame[w:], 2*w, src, w, w, h); err != nil {
		t.Fatalf("before Close: %v", err)
	}
	c.Close()
	if err := c.Convert(frame[0:], frame[w:], 2*w, src, w, w, h); !errors.Is(err, ErrNothingToConvert) {
		t.Fatalf("after Close: %v, want a refusal", err)
	}
}

func TestASynthesisFailureBecomesARefusal(t *testing.T) {
	// The synthesis cannot fail through this package as it stands: checkSizes
	// has already refused everything it complains about. What guarantees that
	// is an invariant of ANOTHER module, and a dependency that loosens its
	// rules does so in silence -- so the refusal is proved here through a seam
	// rather than left to be believed.
	const w, h = 16, 8
	frame := make([]uint32, 2*w*h)
	src := make([]uint32, w*h)

	saved := viewsInto
	defer func() { viewsInto = saved }()
	viewsInto = func(_, _, _ *image.RGBA, _ depth.Map, _ depth.Options) error {
		return errors.New("the synthesis changed its mind")
	}
	c := newCues(Options{})
	if err := c.Convert(frame[0:], frame[w:], 2*w, src, w, w, h); !errors.Is(err, ErrNothingToConvert) {
		t.Fatalf("a failed synthesis gave %v", err)
	}
}
