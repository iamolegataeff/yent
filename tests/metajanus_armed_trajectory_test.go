package tests

import (
	"strconv"
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

// HIGH-2 (Sol fix): the Janus temporal signal is calendar-deterministic and partition-invariant — the
// same origin + same date give the same value regardless of how many am_step calls (traffic / replay /
// restart) got there. This replaces the old D-3 EMA "ladder", which was a per-tick artifact (Sol: on one
// date the old EMA gave 0.475 at 1 step vs 0.003 at 100 steps). The model-external constant is restored.
func TestMetaJanusSignalPartitionInvariant(t *testing.T) {
	at := func(day, steps int) float32 {
		amk := yent.NewAMK()
		amk.Exec("BIRTH 498")
		amk.Exec("JANUS_KEY 1")
		amk.Exec("SELF_NOW_DAYS " + strconv.Itoa(day))
		for i := 0; i < steps; i++ {
			amk.Step(1.0)
		}
		return amk.GetState().JanusTemporalAlpha
	}
	// Partition-invariance across the whole window: 1 step and 100 steps at the same date must agree.
	for _, day := range []int{498, 528, 858, 888, 1218} {
		one, hundred := at(day, 1), at(day, 100)
		if one != hundred {
			t.Fatalf("day %d not partition-invariant: 1 step=%.6f, 100 steps=%.6f (Janus must not follow ticks)", day, one, hundred)
		}
	}
	// The signal still swings by the calendar: retrodiction (<0.5) in the long yahrzeit-near stretch,
	// prophecy (>0.5) once the Gregorian face leads — now instant per date, not gradual over ticks.
	if a := at(528, 1); a >= 0.5 {
		t.Fatalf("day 528 alpha=%.4f, want <0.5 (retrodiction stretch)", a)
	}
	if a := at(888, 1); a <= 0.5 {
		t.Fatalf("day 888 alpha=%.4f, want >0.5 (prophecy stretch)", a)
	}
	// A backward scrub to the origin returns the exact equilibrium — reversible, replayable.
	if a := at(498, 1); a != 0.5 {
		t.Fatalf("origin day 498 alpha=%.4f, want 0.5 (reversible to equilibrium)", a)
	}
}
