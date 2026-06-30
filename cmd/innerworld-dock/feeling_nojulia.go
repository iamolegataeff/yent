//go:build !julia

package main

import "github.com/ariannamethod/yent/innerworld"

// wireFeelingMath (default, no Julia) leaves the High brain on the Go lexical proxy. Build the
// dock with -tags julia (and libjulia installed) to run the HighMathEngine formulas on a real
// in-process Julia runtime instead.
func wireFeelingMath(*innerworld.InnerWorld) {}
