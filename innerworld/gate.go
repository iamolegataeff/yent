package innerworld

import "math"

// gate.go — the unpredictable decision of whether the deep body answers itself
// after the circles. The circles drive the field; the gate reads the field's
// agitation and the circles' texture and turns it into a probability. The deep
// body sometimes turns inward and answers itself, and sometimes does not — like
// arianna's breathe thresholds, the outcome is a roll against a blended metric,
// not a fixed rule. The self-answer is inner only; it never reaches the user.

// DeepGate returns the probability in [0,1] that the deep body answers itself.
// An agitated field (high debt), a thought that wandered far (high drift), and a
// coherent stream the deep body can grip (high coupling) all raise the chance the
// deep body turns inward.
func DeepGate(debt, drift, coupling float32) float32 {
	debt = finite(debt)
	drift = finite(drift)
	coupling = finite(coupling)
	sat := debt / (debt + 1) // debt is unbounded (0..inf); saturate to [0,1)
	if debt < 0 {
		sat = 0
	}
	p := 0.40*sat + 0.35*clamp01(drift) + 0.25*clamp01(coupling)
	return clamp01(p)
}

// finite maps a non-finite input to a sane value so the probability never becomes
// NaN: NaN -> 0, +Inf -> a large finite (saturates), -Inf -> 0.
func finite(x float32) float32 {
	d := float64(x)
	switch {
	case math.IsNaN(d):
		return 0
	case math.IsInf(d, 1):
		return 1e30
	case math.IsInf(d, -1):
		return 0
	default:
		return x
	}
}

// SelfAnswers rolls a [0,1) draw against the gate probability. Deterministic given
// the roll, so tests inject it; production draws from a rand source. At p=0 the
// deep body never answers itself; at p=1 it always does.
func SelfAnswers(p, roll float32) bool {
	return roll < clamp01(p)
}

func clamp01(x float32) float32 {
	if x != x { // NaN
		return 0
	}
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
