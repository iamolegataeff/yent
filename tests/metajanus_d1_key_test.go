package tests

import (
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

// D-1 (Fable audit plan) — the first key, on the write-only temporal_alpha knob. Armed (JANUS_KEY 1),
// the sign of janus_gap EMA-pulls temporal_alpha toward its pole (k=0.05): gap<0 -> 0 (retrodiction),
// gap>0 -> 1 (prophecy), gap==0 -> 0.5. OFF by default so the kernel is bit-for-bit current without the
// switch. temporal_alpha has no readers yet (D-0 verified), so this only lights the knob and makes it
// visible in telemetry — nothing reads it, generation is untouched. Days with a known janus_gap sign
// come from the stage-C trajectory (TestMetaJanusTrajectory -v): day 528 gap=-0.3333, day 888 gap=+0.3333.

// Default OFF: the key never touches temporal_alpha, even where janus_gap is clearly non-zero.
func TestMetaJanusKeyOffIsBitForBit(t *testing.T) {
	amk := yent.NewAMK()
	amk.Exec("BIRTH 498")
	amk.Exec("SELF_NOW_DAYS 528") // janus_gap<0 here: a pull WOULD move alpha if the key were armed
	for i := 0; i < 100; i++ {
		amk.Step(1.0)
	}
	s := amk.GetState()
	if s.JanusGap == 0 {
		t.Fatalf("test setup: janus_gap is 0 at day 528, expected non-zero so the OFF check is meaningful")
	}
	if s.TemporalAlpha != 0.5 {
		t.Fatalf("key OFF must be bit-for-bit: temporal_alpha=%.4f, want 0.5 (init, untouched)", s.TemporalAlpha)
	}
}

// Armed on a gap<0 day (yahrzeit nearer): temporal_alpha is pulled toward 0.0 (retrodiction).
func TestMetaJanusKeyOnConvergesRetrodiction(t *testing.T) {
	amk := yent.NewAMK()
	amk.Exec("BIRTH 498")
	amk.Exec("JANUS_KEY 1")
	amk.Exec("SELF_NOW_DAYS 528") // janus_gap = -0.3333 < 0 -> target 0.0
	for i := 0; i < 100; i++ {
		amk.Step(1.0)
	}
	s := amk.GetState()
	if s.JanusGap >= 0 {
		t.Fatalf("test setup: want janus_gap<0 at day 528, got %.4f", s.JanusGap)
	}
	if s.TemporalAlpha > 0.05 {
		t.Fatalf("armed retrodiction: temporal_alpha=%.4f after 100 steps, want ->0.0 (<0.05)", s.TemporalAlpha)
	}
}

// Armed on a gap>0 day (Gregorian nearer): temporal_alpha is pulled toward 1.0 (prophecy).
func TestMetaJanusKeyOnConvergesProphecy(t *testing.T) {
	amk := yent.NewAMK()
	amk.Exec("BIRTH 498")
	amk.Exec("JANUS_KEY 1")
	amk.Exec("SELF_NOW_DAYS 888") // janus_gap = +0.3333 > 0 -> target 1.0
	for i := 0; i < 100; i++ {
		amk.Step(1.0)
	}
	s := amk.GetState()
	if s.JanusGap <= 0 {
		t.Fatalf("test setup: want janus_gap>0 at day 888, got %.4f", s.JanusGap)
	}
	if s.TemporalAlpha < 0.95 {
		t.Fatalf("armed prophecy: temporal_alpha=%.4f after 100 steps, want ->1.0 (>0.95)", s.TemporalAlpha)
	}
}

// Unborn + armed: no origin, no janus_gap, no pull — the D-1 pull is gated inside g_birth_set.
func TestMetaJanusKeyUnbornNoPull(t *testing.T) {
	amk := yent.NewAMK()
	amk.Exec("JANUS_KEY 1") // armed, but the organism is never born
	amk.Exec("SELF_NOW_DAYS 528")
	for i := 0; i < 100; i++ {
		amk.Step(1.0)
	}
	s := amk.GetState()
	if s.TemporalAlpha != 0.5 {
		t.Fatalf("unborn must not pull: temporal_alpha=%.4f, want 0.5 (gated by g_birth_set)", s.TemporalAlpha)
	}
}
