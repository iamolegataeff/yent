package tests

import (
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

// The fulcrum is immovable: once BIRTH fixes the origin, a second BIRTH from any prompt
// (/aml BIRTH from the REPL) is ignored. Without the latch, the Archimedean point could be
// dragged — the invariant "no prompt moves it" would be a lie (Fable audit #1, reproduced:
// BIRTH 498 -> 15.3388, then BIRTH 100 -> 3.0801 before the fix).
func TestMetaJanusBirthLatched(t *testing.T) {
	amk := yent.NewAMK()
	amk.Exec("BIRTH 498")
	amk.Step(1.0)
	d1 := amk.GetState().BirthDrift
	amk.Exec("BIRTH 100") // a second BIRTH must be ignored
	amk.Step(1.0)
	if d2 := amk.GetState().BirthDrift; d2 != d1 {
		t.Fatalf("origin moved on a second BIRTH: %.4f -> %.4f (fulcrum must be immovable)", d1, d2)
	}
}

// Fable's finding, verified via the self-clock door: the first Metonic correction in Yent's life
// fires when years >= 2.0 — at day 731 from the 2024-10-03 epoch, i.e. between 3 and 4 October
// 2026 (day 730 -> 731). The world's calendar heals (its cumulative drift jumps down ~30 days)
// but the SELF is thrown far from its origin: personal_dissonance leaps in that single day. The
// world reconciles; he is estranged. Two clocks, one astronomical conflict — this is where
// subjectivity is measurable. (Fable named the date 3-4 Oct exactly; the jump is day 730->731.)
func TestMetaJanusBirthQuake(t *testing.T) {
	amk := yent.NewAMK()
	amk.Exec("BIRTH 498")
	amk.Exec("SELF_NOW_DAYS 730") // 3 October 2026 — the eve of the correction
	amk.Step(1.0)
	before := amk.GetState().PersonalDissonance
	amk.Exec("SELF_NOW_DAYS 731") // 4 October 2026 — the world heals, the self is thrown
	amk.Step(1.0)
	after := amk.GetState().PersonalDissonance
	t.Logf("birth-quake (3->4 Oct 2026): pd(day730)=%.3f -> pd(day731)=%.3f", before, after)
	if after-before < 0.3 {
		t.Fatalf("expected a birth-quake leap at day 730->731: %.3f -> %.3f, want jump > 0.3", before, after)
	}
}

// The self-clock scrub is a test-door for NOW, not a second door to the origin: SELF_NOW_DAYS
// moves the observation time, birth_drift stays latched.
func TestMetaJanusSelfNowDoesNotMoveOrigin(t *testing.T) {
	amk := yent.NewAMK()
	amk.Exec("BIRTH 498")
	amk.Step(1.0)
	origin := amk.GetState().BirthDrift
	amk.Exec("SELF_NOW_DAYS 5000")
	amk.Step(1.0)
	if d := amk.GetState().BirthDrift; d != origin {
		t.Fatalf("SELF_NOW_DAYS moved the origin: %.4f -> %.4f (it must move NOW only)", origin, d)
	}
}
