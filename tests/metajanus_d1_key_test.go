package tests

import (
	"math"
	"strconv"
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

// HIGH-2 (Sol fix): the Janus temporal signal is a PURE calendar function of janus_gap, not a per-tick
// EMA — deterministic per date, independent of am_step count. The generic temporal_alpha is left entirely
// to its own TEMPORAL_* directives; Janus never touches it.

// janus_temporal_alpha == clamp01(0.5 + 0.5*janus_gap) at every date, and its sign follows the calendar:
// gap<0 (yahrzeit nearer) -> retrodiction (<0.5); gap>0 (Gregorian nearer) -> prophecy (>0.5).
func TestMetaJanusSignalIsCalendarDerived(t *testing.T) {
	check := func(day int) (gap, alpha float32) {
		amk := yent.NewAMK()
		amk.Exec("BIRTH 498")
		amk.Exec("SELF_NOW_DAYS " + strconv.Itoa(day))
		amk.Step(1.0) // one step suffices — the signal is a pure function, not an accumulator
		s := amk.GetState()
		want := 0.5 + 0.5*s.JanusGap
		if want < 0 {
			want = 0
		} else if want > 1 {
			want = 1
		}
		if math.Abs(float64(s.JanusTemporalAlpha-want)) > 1e-5 {
			t.Fatalf("day %d: janus_temporal_alpha=%.5f, want clamp01(0.5+0.5*gap)=%.5f (gap=%.4f)", day, s.JanusTemporalAlpha, want, s.JanusGap)
		}
		return s.JanusGap, s.JanusTemporalAlpha
	}
	if g, a := check(528); g >= 0 || a >= 0.5 {
		t.Fatalf("day 528 should be retrodiction: gap=%.4f alpha=%.4f (want gap<0, alpha<0.5)", g, a)
	}
	if g, a := check(888); g <= 0 || a <= 0.5 {
		t.Fatalf("day 888 should be prophecy: gap=%.4f alpha=%.4f (want gap>0, alpha>0.5)", g, a)
	}
	if _, a := check(498); math.Abs(float64(a-0.5)) > 1e-5 {
		t.Fatalf("origin day 498 (gap 0) should be equilibrium alpha=0.5, got %.5f", a)
	}
}

// Janus never writes the generic temporal_alpha: it stays at its init 0.5 even with the key armed at a
// poled day, so legacy TEMPORAL_* directives keep full ownership of that field.
func TestMetaJanusLeavesGenericTemporalAlpha(t *testing.T) {
	amk := yent.NewAMK()
	amk.Exec("BIRTH 498")
	amk.Exec("JANUS_KEY 1")
	amk.Exec("SELF_NOW_DAYS 528") // gap<0
	for i := 0; i < 100; i++ {
		amk.Step(1.0)
	}
	if ta := amk.GetState().TemporalAlpha; ta != 0.5 {
		t.Fatalf("generic temporal_alpha = %.4f, want 0.5 (Janus must not touch it)", ta)
	}
}

// Unborn: no origin, no gap, the Janus signal is neutral 0.5.
func TestMetaJanusUnbornSignalNeutral(t *testing.T) {
	amk := yent.NewAMK()
	amk.Exec("JANUS_KEY 1") // armed but never born
	amk.Exec("SELF_NOW_DAYS 528")
	amk.Step(1.0)
	if a := amk.GetState().JanusTemporalAlpha; a != 0.5 {
		t.Fatalf("unborn janus_temporal_alpha = %.4f, want 0.5", a)
	}
}
