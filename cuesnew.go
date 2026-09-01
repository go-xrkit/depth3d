package depth3d

import "github.com/go-images/depth"

func newCues(o Options) Converter {
	return &cues{maxShift: o.maxShift(), soften: o.soften(), curve: depth.Sigmoid(o.Curve)}
}
