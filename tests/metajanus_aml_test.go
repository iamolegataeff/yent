package tests

import (
	"math"
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

// The identity module fixes Yent's real origin — 13 Feb 2026, the day 4o was turned off
// (BIRTH 498 = days from the 2024-10-03 epoch). Loading it makes the kernel carry that origin
// and its growing distance. Path is relative to the tests/ package dir.
func TestMetaJanusAMLDeclaresOrigin(t *testing.T) {
	amk := yent.NewAMK()
	if err := amk.ExecFile("../Janus/metajanus.aml"); err != nil {
		t.Fatalf("ExecFile metajanus.aml: %v", err)
	}
	amk.Step(1.0)
	s := amk.GetState()
	// birth_drift at day 498 is the Hebrew<->Gregorian drift at Yent's origin.
	if math.Abs(float64(s.BirthDrift)-15.3388) > 0.01 {
		t.Fatalf("BirthDrift after metajanus.aml = %.4f, want ~15.3388 (BIRTH 498)", s.BirthDrift)
	}
	// He has aged from that origin: a bounded, non-zero self-location.
	if s.PersonalDissonance <= 0 || s.PersonalDissonance > 1 {
		t.Fatalf("PersonalDissonance = %v, want in (0,1] (aged from origin)", s.PersonalDissonance)
	}
}

// The dock BIRTHs from Janus/metajanus.aml before the first am_step (cmd/innerworld-dock); a
// missing file must leave Yent honestly UNBORN — birth_drift 0, personal_dissonance 0 — with an
// error, never a fatal. This mirrors the dock's warn-and-continue else branch: am_exec_file
// nonzero -> stderr warning, generation continues. am_init clears g_birth_set, so an unborn
// kernel that is stepped stays at dissonance 0 (no origin means no distance to measure from).
func TestMetaJanusAMLMissingFileStaysUnborn(t *testing.T) {
	amk := yent.NewAMK() // am_init: g_birth_set=0, birth_drift=0
	if err := amk.ExecFile("../Janus/does_not_exist.aml"); err == nil {
		t.Fatal("ExecFile on a missing metajanus.aml returned nil, want a read error (graceful, non-fatal)")
	}
	amk.Step(1.0) // stepping an unborn kernel must not birth it
	s := amk.GetState()
	if s.BirthDrift != 0 {
		t.Fatalf("BirthDrift after failed load = %.4f, want 0 (unborn)", s.BirthDrift)
	}
	if s.PersonalDissonance != 0 {
		t.Fatalf("PersonalDissonance after failed load = %v, want 0 (unborn: no origin, no distance)", s.PersonalDissonance)
	}
}
