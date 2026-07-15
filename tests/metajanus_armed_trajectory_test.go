package tests

import (
	"math"
	"strconv"
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

// D-3 (Fable audit plan): observation UNDER the armed key. With JANUS_KEY armed, walk the self-clock
// continuously across ~2 years and watch temporal_alpha ladder over the janus_gap sawtooth — the EMA
// descends toward 0 (retrodiction) through the long negative-gap stretch, then climbs toward 1
// (prophecy) once the gap turns positive at the Hebrew anniversary. This is inert: it only reads the
// field through the SELF_NOW_DAYS test-door, and nothing in generation reads temporal_alpha yet (D-0).
// Run `go test ./tests -run ArmedTrajectory -v` to see the ladder track the gap.
func TestMetaJanusArmedTrajectory(t *testing.T) {
	amk := yent.NewAMK()
	if err := amk.ExecFile("../Janus/metajanus.aml"); err != nil {
		t.Fatalf("ExecFile metajanus.aml: %v", err)
	}
	amk.Exec("JANUS_KEY 1") // arm the first key
	const origin = 498
	const span = 732

	alpha := make([]float32, span)
	gap := make([]float32, span)
	var minAlpha float32 = 1
	var minAlphaDay int
	for i := 0; i < span; i++ {
		day := origin + i
		amk.Exec("SELF_NOW_DAYS " + strconv.Itoa(day))
		amk.Step(1.0)
		s := amk.GetState()
		// The armed key must not disturb the origin — only NOW is scrubbed.
		if math.Abs(float64(s.BirthDrift)-15.3388) > 0.01 {
			t.Fatalf("origin moved under the armed key at day %d: birth_drift=%.4f", day, s.BirthDrift)
		}
		alpha[i], gap[i] = s.TemporalAlpha, s.JanusGap
		if alpha[i] < minAlpha {
			minAlpha, minAlphaDay = alpha[i], day
		}
	}

	for i := 0; i < span; i += 30 {
		t.Logf("day %4d  janus_gap=%+.4f  temporal_alpha=%.4f", origin+i, gap[i], alpha[i])
	}
	final := alpha[span-1]
	t.Logf("LADDER: min temporal_alpha=%.4f at day %d (deep retrodiction) -> final=%.4f (climbed to prophecy)",
		minAlpha, minAlphaDay, final)

	// A real ladder swing: the long negative-gap stretch pulls temporal_alpha down near 0, then the
	// positive-gap stretch after the anniversary lifts it back up — not a flat line.
	if minAlpha > 0.1 {
		t.Fatalf("armed retrodiction never converged: min temporal_alpha=%.4f, want <0.1", minAlpha)
	}
	if final < 0.6 {
		t.Fatalf("armed prophecy never lifted temporal_alpha: final=%.4f, want >0.6", final)
	}
	if final-minAlpha < 0.5 {
		t.Fatalf("no ladder swing: final-min=%.4f, want >0.5", final-minAlpha)
	}
}
