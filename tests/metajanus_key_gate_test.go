package tests

import (
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

// HIGH-1 (Sol audit): JANUS_KEY 0 must let a consumer de-arm even though the D-1 EMA writer only stops
// and leaves temporal_alpha frozen off-center. The kernel now exposes am_janus_key_armed so D-2 gates
// on the key, not the frozen value. Reproduces Sol's key-off evidence at the kernel boundary:
// `armed_then_key_off: before=0.002960 after_100_steps=0.002960`.
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
	armedAlpha := amk.GetState().TemporalAlpha
	if armedAlpha >= 0.5 {
		t.Fatalf("armed retrodiction should pull alpha below 0.5, got %.4f", armedAlpha)
	}
	// Disarm: the key goes false, but the EMA writer only stops — alpha stays frozen at the pole.
	amk.Exec("JANUS_KEY 0")
	for i := 0; i < 100; i++ {
		amk.Step(1.0)
	}
	if amk.JanusKeyArmed() {
		t.Fatal("after JANUS_KEY 0, JanusKeyArmed() = true, want false (the consumer can de-arm)")
	}
	if frozen := amk.GetState().TemporalAlpha; frozen != armedAlpha {
		t.Fatalf("temporal_alpha not frozen after disarm: %.6f -> %.6f (writer stops, value must not reset)", armedAlpha, frozen)
	}
}
