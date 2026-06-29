package innerworld

import "testing"

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
