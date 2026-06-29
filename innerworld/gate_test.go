package innerworld

import (
	"math"
	"testing"
)

func TestDeepGateNonFinite(t *testing.T) {
	inf := float32(math.Inf(1))
	ninf := float32(math.Inf(-1))
	nan := float32(math.NaN())
	for _, p := range []float32{
		DeepGate(inf, 0.5, 0.5),
		DeepGate(ninf, 0.5, 0.5),
		DeepGate(nan, nan, nan),
		DeepGate(5, inf, nan),
	} {
		if p != p || p < 0 || p > 1 { // p != p catches NaN
			t.Errorf("non-finite input produced %v (must be finite in [0,1])", p)
		}
	}
}

func TestDeepGate(t *testing.T) {
	low := DeepGate(0, 0, 0)
	high := DeepGate(5, 0.9, 0.9)
	if high <= low {
		t.Errorf("agitation should raise the self-answer probability: high=%.3f low=%.3f", high, low)
	}
	for _, p := range []float32{low, high, DeepGate(-1, 2, -3), DeepGate(100, 1, 1)} {
		if p < 0 || p > 1 {
			t.Errorf("probability out of [0,1]: %.3f", p)
		}
	}
}

func TestSelfAnswers(t *testing.T) {
	if !SelfAnswers(0.7, 0.5) {
		t.Errorf("roll 0.5 < p 0.7 should self-answer")
	}
	if SelfAnswers(0.3, 0.5) {
		t.Errorf("roll 0.5 >= p 0.3 should not self-answer")
	}
	if SelfAnswers(0, 0) {
		t.Errorf("p=0 should never self-answer")
	}
	if !SelfAnswers(1, 0.999) {
		t.Errorf("p=1 should always self-answer")
	}
}
