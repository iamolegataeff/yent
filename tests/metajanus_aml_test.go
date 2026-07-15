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
