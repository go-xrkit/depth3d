//go:build !darwin

package depth3d

// New opens the best converter this machine has.
//
// Everywhere but macOS that is the portable one: there is no Core ML to ask
// and no Metal to ask it with. A model named here is reported rather than
// ignored, because a caller who supplied one and got the cheap estimate should
// be told why.
func New(o Options) (Converter, error) {
	if o.Model != "" {
		o.logf("depth3d: a depth model was given, but only macOS can run one")
	}
	return newCues(o), nil
}
