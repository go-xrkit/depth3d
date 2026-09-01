# depth3d

[![Go Reference](https://pkg.go.dev/badge/github.com/go-xrkit/depth3d.svg)](https://pkg.go.dev/github.com/go-xrkit/depth3d)
[![License: BSD-3-Clause](https://img.shields.io/badge/License-BSD%203--Clause-blue.svg)](https://opensource.org/licenses/BSD-3-Clause)
[![Pure Go](https://img.shields.io/badge/pure%20Go-CGO%3D0-00ADD8?logo=go&logoColor=white)](https://github.com/go-xrkit/depth3d)

**One flat picture in, two eyes out — using the best path the machine has.**

```go
c, err := depth3d.New(depth3d.Options{Model: "Depth.mlpackage", MaxShift: 24})
defer c.Close()

err = c.Convert(left, right, stride, src, srcStride, w, h)
```

It is the same job whether the picture is a film frame or a captured desktop,
and both callers in this organisation want it:
[player](https://github.com/go-xrkit/player) converts a flat film as it plays,
and [desk](https://github.com/go-xrkit/desk) converts whatever the glasses are
showing. The second of those is why this is a package rather than a file in one
of them.

## Two paths, and it always says which

With a Core ML depth model on a Mac, depth comes from a real network on the
**Neural Engine** ([go-macos/coreml](https://github.com/go-macos/coreml)) and
the two views from compute kernels on the **GPU**
([go-macos/metal](https://github.com/go-macos/metal)). Measured on an M4 Max, a
frame costs about **0.4 ms of processor time** — which is what lets a
compositor keep the rest of the machine.

With no model, or on any other platform, depth is guessed from cues in the
picture itself ([go-images/depth](https://github.com/go-images/depth)). It
needs nothing at all, runs everywhere including a browser, and is visibly not
as good.

`Describe` reports which one is running, and a fall back is always logged. A
converter that quietly took the cheap path would look identical from the
outside except for being worse.

## The two eyes may be one frame

`left` and `right` may point into the same buffer — the two halves of a
side-by-side frame, addressed by that frame's stride — or they may be two
separate pictures. The player wants the first, the desk the second, and neither
pays a copy for the other's preference.

## What it cannot do

It cannot invent what the camera never saw. Where a near object moves aside,
what is behind it is guessed from the same row, so an edge is a little smeared.
That is the honest cost of the effect.

`Curve` is worth reading before turning on: an S-curve flattens both ends of
the depth range and gives the middle their relief, so the subject stands out —
it is **not** a comfort control, and a near object gets slightly *more*
disparity. At a comfortable `MaxShift` it is swamped anyway: twelve pixels each
way is thirteen distinct shifts for the whole range.

## Testing

The portable layer — sizing, defaults, the fall back, and the synthesis on the
processor — is held at **100%**, gated in CI. The accelerated path needs a GPU
and a depth model, which a build machine has neither of, so its tests skip:

```
XRKIT_TEST_MODEL=/path/to/Depth.mlpackage go test ./...
```

The kernels are a string compiled at run time, so nothing in an ordinary build
looks at them; a test compiles every one of them, with a negative control. The
curve test binds a table and checks it reaches the kernel, again with a control
— without it, "the curve changed the picture" could be a rebound allocation.

Both implementations are checked against **each other**: on a real photograph
with a real network's depth map, the GPU synthesis and the portable one agree
on 0 bytes out of 86 999 040. Floating point would agree almost always, which
is the worst kind of agreement, so the arithmetic is integer throughout and the
curve travels as a table both sides index.

## Install

```
go get github.com/go-xrkit/depth3d
```

`CGO_ENABLED=0`.
