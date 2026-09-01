package depth3d

import (
	"errors"
	"os"
	"testing"

	"github.com/go-images/depth"
	"github.com/go-macos/metal"
)

// The kernels are a string, compiled at run time by the system compiler, so
// nothing in an ordinary build looks at them at all. Without this test a typo
// reaches a person as a fall back to the worse path, mid-frame, with a
// twenty-line compiler error in the log -- which is exactly what happened once,
// over a variable named `half`, that being a TYPE in Metal.
func TestTheKernelsCompileAndEveryOneOfThemIsThere(t *testing.T) {
	dev, err := metal.Default()
	if errors.Is(err, metal.ErrNoDevice) {
		t.Skip("no GPU on this machine")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer dev.Close()

	lib, err := dev.Compile(kernels)
	if err != nil {
		t.Fatalf("the kernels do not compile:\n%v", err)
	}
	defer lib.Close()

	for _, name := range []string{"eyes", "fill", "blurH", "blurV", "upsample"} {
		pipe, err := lib.Pipeline(name)
		if err != nil {
			t.Errorf("kernel %s: %v", name, err)
			continue
		}
		pipe.Close()
	}
	// The negative control: a name that is not there must fail, or the loop
	// above would pass against a library that answers to anything.
	if _, err := lib.Pipeline("thereIsNoSuchKernel"); err == nil {
		t.Error("a kernel that does not exist became a pipeline")
	}
}

// textured builds a picture in which every COLUMN is distinct, with something
// for a depth network to find in the lower middle.
//
// Distinct on purpose: a repeating pattern makes a shift land back on itself,
// so a picture that moved a long way scores as having moved less than one that
// barely moved.
func textured(w, h, stride int) []uint32 {
	src := make([]uint32, stride*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := uint32(x * 2 % 256)
			if y > h/2 && y < 5*h/6 && x > w/4 && x < 3*w/4 {
				c = uint32(x*2%128) + 128
			}
			src[y*stride+x] = 0xFF000000 | c<<16 | c<<8 | c
		}
	}
	return src
}

func moved(left []uint32, stride int, src []uint32, srcStride, w, h int) int {
	n := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if left[y*stride+x] != src[y*srcStride+x] {
				n++
			}
		}
	}
	return n
}

// It needs a depth model, which a build machine has no reason to carry:
//
//	XRKIT_TEST_MODEL=/path/to/Depth.mlpackage go test ./...
func modelOrSkip(t *testing.T) string {
	t.Helper()
	m := os.Getenv("XRKIT_TEST_MODEL")
	if m == "" {
		t.Skip("set XRKIT_TEST_MODEL to a Core ML depth model to run this")
	}
	return m
}

func TestTheAcceleratedPathMovesMoreWithMoreDisparity(t *testing.T) {
	// Two snapshots taken at different disparities once came out byte for byte
	// identical, and the cause was innocent -- the frame was nearly black, and
	// a depth map with no range moves nothing, correctly. Nothing in that run
	// could tell it from a parameter going nowhere. This can.
	model := modelOrSkip(t)
	const w, h = 256, 160
	stride := w + 7
	src := textured(w, h, stride)

	count := func(maxShift int) int {
		c, err := New(Options{Model: model, MaxShift: maxShift})
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		if _, ok := c.(*gpu); !ok {
			t.Fatalf("the accelerated path did not open: %s", c.Describe())
		}
		frame := make([]uint32, 2*w*h)
		if err := c.Convert(frame[0:], frame[w:], 2*w, src, stride, w, h); err != nil {
			t.Fatal(err)
		}
		for y := 0; y < h; y++ {
			for x := 0; x < 2*w; x++ {
				if frame[y*2*w+x]>>24 == 0 {
					t.Fatalf("a transparent pixel survived at %d,%d", x, y)
				}
			}
		}
		return moved(frame, 2*w, src, stride, w, h)
	}
	small, large := count(8), count(48)
	t.Logf("moved %d pixels at a disparity of 8, %d at 48", small, large)
	if small == 0 {
		t.Fatal("nothing moved at all")
	}
	if large <= small {
		t.Fatalf("a disparity of 48 moved %d pixels, no more than 8 did (%d)", large, small)
	}
}

func TestTheCurveReachesTheKernel(t *testing.T) {
	// The curve travels to the GPU as a table the kernel indexes. If it were
	// not bound, or bound at the wrong index, the kernel would read the
	// identity it is given when none was asked for -- and every other test
	// here would still pass.
	model := modelOrSkip(t)
	const w, h = 256, 160
	stride := w + 7
	src := textured(w, h, stride)

	run := func(curve float64) []uint32 {
		// A large disparity on purpose: at a comfortable one the curve is
		// swamped by quantisation, which go-images/depth measures and pins.
		c, err := New(Options{Model: model, MaxShift: 96, Curve: curve})
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		frame := make([]uint32, 2*w*h)
		if err := c.Convert(frame[0:], frame[w:], 2*w, src, stride, w, h); err != nil {
			t.Fatal(err)
		}
		return frame
	}
	plain, curved := run(0), run(6)
	diff := 0
	for i := range plain {
		if plain[i] != curved[i] {
			diff++
		}
	}
	if diff == 0 {
		t.Fatal("the curve changed nothing; it is not reaching the kernel")
	}
	t.Logf("the curve moved %d of %d pixels", diff, len(plain))

	// The negative control: a strength that builds NO curve must change
	// nothing at all, or the difference above could be anything -- a rebound
	// allocation, a stale buffer, noise from the network.
	if depth.Sigmoid(0) != nil {
		t.Fatal("a strength of zero built a curve")
	}
	again := run(0)
	for i := range plain {
		if plain[i] != again[i] {
			t.Fatalf("two identical runs differ at %d", i)
		}
	}
}

func TestNewFallsBackAndSaysSo(t *testing.T) {
	var said []string
	c, err := New(Options{Model: "/there/is/no/such/model.mlpackage",
		Log: func(s string) { said = append(said, s) }})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, ok := c.(*cues); !ok {
		t.Fatalf("a missing model still opened %s", c.Describe())
	}
	if len(said) == 0 {
		t.Fatal("it fell back in silence")
	}

	// And with no model at all it says that too, rather than leaving a caller
	// to wonder why the effect is weaker than it was on another machine.
	said = nil
	c2, err := New(Options{Log: func(s string) { said = append(said, s) }})
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	if len(said) == 0 {
		t.Fatal("no model, and nothing said")
	}
}
