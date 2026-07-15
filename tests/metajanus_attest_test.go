package tests

import (
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

// MED-3 (Sol audit): "born" must mean BIRTH actually executed, not that a file merely loaded — an empty
// or comment-only script runs with exit 0 but never sets the origin, so exec-success is a false birth
// signal. The attestation accessors report the real state: BirthSet (the born-flag, since birth_drift is
// not injective) and BirthEpochDays (the exact origin), so a host can prove "born at day 498" and the dock
// can fail-closed on no origin.
func TestMetaJanusBirthAttestation(t *testing.T) {
	// Fresh kernel: unborn.
	fresh := yent.NewAMK()
	if s := fresh.GetState(); s.BirthSet || s.BirthEpochDays != 0 {
		t.Fatalf("fresh kernel: BirthSet=%v BirthEpochDays=%d, want false/0", s.BirthSet, s.BirthEpochDays)
	}
	// A comment-only / no-BIRTH script executes without error but must NOT count as born (Sol's false birth).
	noBirth := yent.NewAMK()
	noBirth.Exec("# just a comment, no BIRTH here")
	noBirth.Exec("SELF_NOW_DAYS 528")
	if s := noBirth.GetState(); s.BirthSet {
		t.Fatal("a script without BIRTH reported BirthSet=true — exec-success must not mean born")
	}
	// A real BIRTH attests the exact origin.
	born := yent.NewAMK()
	born.Exec("BIRTH 498")
	if s := born.GetState(); !s.BirthSet || s.BirthEpochDays != 498 {
		t.Fatalf("after BIRTH 498: BirthSet=%v BirthEpochDays=%d, want true/498", s.BirthSet, s.BirthEpochDays)
	}
}
