package tests

import (
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

// HIGH-1 (Sol audit): JANUS_KEY must be observable so a consumer (D-2) can de-arm. am_janus_key_armed
// reports the key state true->false across JANUS_KEY 1 -> 0. HIGH-2: the Janus signal is calendar-derived,
// so across arm/disarm at the SAME date it is unchanged (deterministic, not a frozen EMA), and the generic
// temporal_alpha is never written by Janus.
func TestMetaJanusKeyArmedFlagTracksKey(t *testing.T) {
	amk := yent.NewAMK()
	amk.Exec("BIRTH 498")
	amk.Exec("SELF_NOW_DAYS 528") // janus_gap<0 -> retrodiction pole
	amk.Exec("JANUS_KEY 1")
	for i := 0; i < 100; i++ {
		amk.Step(1.0)
	}
	if !amk.JanusKeyArmed() {
		t.Fatal("after JANUS_KEY 1, JanusKeyArmed() = false, want true")
	}
	armedSignal := amk.GetState().JanusTemporalAlpha
	if armedSignal >= 0.5 {
		t.Fatalf("armed retrodiction should keep janus_temporal_alpha below 0.5, got %.4f", armedSignal)
	}
	// Disarm: the key flag goes false so the consumer can de-arm.
	amk.Exec("JANUS_KEY 0")
	for i := 0; i < 100; i++ {
		amk.Step(1.0)
	}
	if amk.JanusKeyArmed() {
		t.Fatal("after JANUS_KEY 0, JanusKeyArmed() = true, want false (the consumer can de-arm)")
	}
	// The signal is calendar-derived, so at the same date it is unchanged (deterministic, not frozen).
	if s := amk.GetState().JanusTemporalAlpha; s != armedSignal {
		t.Fatalf("janus_temporal_alpha changed across arm/disarm at one date: %.6f -> %.6f (must be date-deterministic)", armedSignal, s)
	}
	// Janus never wrote the generic temporal_alpha.
	if ta := amk.GetState().TemporalAlpha; ta != 0.5 {
		t.Fatalf("generic temporal_alpha = %.4f, want 0.5 (untouched by Janus)", ta)
	}
}
