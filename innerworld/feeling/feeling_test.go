//go:build julia

package feeling

import (
	"math"
	"testing"
)

// TestJuliaComputesFeeling proves the math runs on the real in-process Julia runtime: the
// numbers must match `julia -e include(feeling.jl)` (char distribution of "hello" etc.), and
// they cannot be produced without the Julia VM actually executing the HighMathEngine formulas.
func TestJuliaComputesFeeling(t *testing.T) {
	if err := Init("../feeling.jl"); err != nil {
		t.Fatalf("embedded Julia failed to init: %v", err)
	}
	if !Ready() {
		t.Fatal("Julia runtime not ready after Init")
	}
	if h := CharEntropy("hello"); math.Abs(h-1.9219280948873623) > 1e-6 {
		t.Errorf("CharEntropy(\"hello\") via Julia: got %.10f want 1.9219280948873623", h)
	}
	if p := Perplexity("abracadabra"); math.Abs(p-1.6572270086699934) > 1e-6 {
		t.Errorf("Perplexity(\"abracadabra\") via Julia: got %.10f want 1.6572270086699934", p)
	}
	if d := SemanticDistance("a b c", "a b d"); math.Abs(d-0.33333333333333326) > 1e-9 {
		t.Errorf("SemanticDistance via Julia: got %.10f want 0.3333333333333333", d)
	}
}
