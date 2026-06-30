//go:build julia

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/ariannamethod/yent/innerworld"
	"github.com/ariannamethod/yent/innerworld/feeling"
)

// wireFeelingMath (—tags julia) runs the High brain's feeling math on a REAL in-process Julia
// runtime: it loads YENT_FEELING_JL (the HighMathEngine formulas, innerworld/feeling.jl) into
// libjulia and injects it as the inner world's FeelMath backend. If Julia or the file fails to
// load, the dock falls back to the Go lexical proxy rather than dying.
func wireFeelingMath(iw *innerworld.InnerWorld) {
	jl := strings.TrimSpace(os.Getenv("YENT_FEELING_JL"))
	if jl == "" {
		return
	}
	if err := feeling.Init(jl); err != nil {
		fmt.Fprintf(os.Stderr, "[dock] Julia feeling-math unavailable (%v); using the Go lexical proxy\n", err)
		return
	}
	iw.SetFeelMath(feeling.JuliaFeelMath{})
	fmt.Println("=== Julia feeling-math wired: HighMathEngine formulas run in-process on libjulia ===")
}
