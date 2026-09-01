// Package depth3d turns a flat picture into two eyes, using the best path the
// machine has.
//
// It is the same job whether the picture is a film frame or a captured desktop,
// and both callers in this organisation want it: go-xrkit/player converts a
// flat film as it plays, and go-xrkit/desk converts whatever the glasses are
// showing. The second of those is why this is a package rather than a file in
// one of them.
//
//	c, err := depth3d.New(depth3d.Options{Model: "Depth.mlpackage", MaxShift: 24})
//	defer c.Close()
//	err = c.Convert(left, right, stride, src, srcStride, w, h)
//
// # Two paths, and it always says which
//
// With a Core ML depth model on a Mac, the depth comes from a real network on
// the Neural Engine and the two views from compute kernels on the GPU: a frame
// costs about four tenths of a millisecond of processor time, which is what
// lets a compositor keep the rest.
//
// With no model, or on any other platform, the depth is guessed from cues in
// the picture itself (go-images/depth). It needs nothing at all, runs
// everywhere including a browser, and is visibly not as good.
//
// Describe reports which one is running. A converter that quietly fell back
// would look identical from the outside except for being worse.
//
// # What it cannot do
//
// It cannot invent what the camera never saw. Where a near object moves aside,
// what is behind it is guessed from the same row, so an edge is a little
// smeared. That is the honest cost of the effect.
package depth3d

import (
	"errors"
	"fmt"
)

// ErrNothingToConvert is returned for a frame that cannot be turned into a
// pair — an empty picture, or a destination that does not match it.
var ErrNothingToConvert = errors.New("depth3d: nothing to convert in this frame")

// Options configure a converter.
type Options struct {
	// Model names a Core ML depth model — an .mlpackage or an already compiled
	// .mlmodelc. Empty falls back to the estimate from cues in the picture,
	// which needs nothing and is visibly not as good.
	Model string

	// MaxShift is how far apart the two eyes put the NEAREST thing, in pixels
	// of the source: the total between them, half of it each way, so that the
	// original stays in the middle. Zero means 24.
	//
	// Small on purpose. A large disparity makes an impressive still and an
	// unwatchable film, because the eyes must converge differently on every
	// cut.
	MaxShift int

	// Soften is how much the depth map is blurred before it moves any pixels.
	// Zero means 2, which is measured: it takes the worst movement of an edge
	// between one frame and the next from 4.94 pixels to 1.10, for 4.8% less
	// relief.
	Soften int

	// Curve reshapes depth before it becomes disparity, as an S-curve of this
	// strength. Zero means none.
	//
	// It flattens both ends of the range and gives the middle their relief, so
	// the subject stands out. It is NOT a comfort control: a near object gets
	// slightly MORE disparity. And at a comfortable MaxShift it is swamped by
	// quantisation — twelve pixels each way is thirteen distinct shifts for the
	// whole depth range — so raise MaxShift before expecting to see it.
	Curve float64

	// Log, when set, receives one line saying which path opened and why. It is
	// how a caller finds out that the accelerated path was unavailable rather
	// than discovering it as a slowdown.
	Log func(string)
}

func (o Options) maxShift() int {
	if o.MaxShift <= 0 {
		return 24
	}
	return o.MaxShift
}

func (o Options) soften() int {
	if o.Soften <= 0 {
		return 2
	}
	return o.Soften
}

func (o Options) logf(format string, args ...any) {
	if o.Log != nil {
		o.Log(fmt.Sprintf(format, args...))
	}
}

// Converter turns one flat frame into two eyes. It is not safe for concurrent
// use: it owns buffers sized to the last frame it saw.
type Converter interface {
	// Convert writes w by h pixels into left and right.
	//
	// left and right may point into the SAME buffer — the two halves of a
	// side-by-side frame — provided they do not overlap; stride is then that
	// frame's stride. They may equally be two separate pictures.
	Convert(left, right []uint32, stride int, src []uint32, srcStride, w, h int) error

	// Describe says which path is doing the work, for a log or a menu.
	Describe() string

	// Close releases whatever the path holds.
	Close()
}
