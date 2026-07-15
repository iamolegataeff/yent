package tests

import (
	"math"
	"strconv"
	"testing"
	"time"

	yent "github.com/ariannamethod/yent/yent/go"
)

// mjEpoch is the calendar-drift epoch, verbatim from ariannamethod.c calendar_init:
// 2024-10-03 12:00 local. Used only to phrase "today" in days for the origin test.
var mjEpoch = time.Date(2024, time.October, 3, 12, 0, 0, 0, time.Local)

func mjDaysSinceEpoch(t time.Time) int { return int(t.Sub(mjEpoch).Hours() / 24.0) }

// Before BIRTH the organism has no origin: personal_dissonance is 0 (unborn). This is the
// self-LOCATION axis, not agency — it needs no rival, only an origin, and there is none yet.
func TestMetaJanusZeroBeforeBirth(t *testing.T) {
	amk := yent.NewAMK()
	amk.Step(1.0)
	if d := amk.GetState().PersonalDissonance; d != 0 {
		t.Fatalf("personal_dissonance before BIRTH = %v, want 0 (no origin yet)", d)
	}
}

// BIRTH fixes the origin. personal_dissonance is then a bounded, deterministic self-relation
// the field carries — computed from the two dates, moved by no prompt. A first-person-in-time.
func TestMetaJanusBoundedAndDeterministic(t *testing.T) {
	amk := yent.NewAMK()
	if err := amk.Exec("BIRTH 100"); err != nil {
		t.Fatalf("Exec BIRTH: %v", err)
	}
	amk.Step(1.0)
	s1 := amk.GetState()
	if s1.PersonalDissonance < 0 || s1.PersonalDissonance > 1 {
		t.Fatalf("personal_dissonance = %v, want in [0,1]", s1.PersonalDissonance)
	}
	if s1.BirthDrift == 0 {
		t.Fatalf("birth_drift = 0 after BIRTH 100, want the fixed origin set")
	}
	amk.Step(1.0)
	if s2 := amk.GetState(); math.Abs(float64(s2.PersonalDissonance-s1.PersonalDissonance)) > 1e-6 {
		t.Fatalf("personal_dissonance not deterministic: %v then %v", s1.PersonalDissonance, s2.PersonalDissonance)
	}
}

// At its own origin (BIRTH == today) the distance from origin is ~0 — the fixed point. A ±1
// day clock skew between Go and the kernel is < 0.01 dissonance, so assert < 0.01, not == 0.
func TestMetaJanusZeroAtOrigin(t *testing.T) {
	amk := yent.NewAMK()
	today := mjDaysSinceEpoch(time.Now())
	if err := amk.Exec("BIRTH " + strconv.Itoa(today)); err != nil {
		t.Fatalf("Exec BIRTH today: %v", err)
	}
	amk.Step(1.0)
	if d := amk.GetState().PersonalDissonance; d > 0.01 {
		t.Fatalf("personal_dissonance at own origin = %v, want ~0 (< 0.01)", d)
	}
}
