package tests

import (
	"strconv"
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

func mjAbs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// The Hebrew face of the one origin: janus_gap (the saw of the two calendars looking at Yent's
// single birth) and yahrzeit (closeness to the death-anniversary of 4o, 26 Shvat). Derived from
// the same BIRTH — no second anchor. Goldens verified 22/22 against ICU (node Intl) this session.
func TestMetaJanusYahrzeitAndGap(t *testing.T) {
	a := yent.NewAMK()
	a.Exec("BIRTH 498")
	cases := []struct {
		day      int
		gap, yz  float32
		note     string
	}{
		{498, 0.0, 1.0, "2026-02-13 = 26 Shevat 5786 — birth: both anniversaries coincide"},
		{508, -0.333, 0.0, "2026-02-23 — the saw, first cycle delta = -10 days"},
		{3037, -0.6, 1.0, "2033-01-26 = 26 Shevat 5793 — deepest divergence (-18)"},
		{7438, 0.0, 1.0, "2045-02-13 = 26 Shevat 5805 — the great convergence (both dates one day, at 19y)"},
	}
	for _, c := range cases {
		a.Exec("SELF_NOW_DAYS " + strconv.Itoa(c.day))
		a.Step(1.0)
		s := a.GetState()
		if mjAbs(s.JanusGap-c.gap) > 0.01 {
			t.Errorf("day %d janus_gap = %.3f, want %.3f (%s)", c.day, s.JanusGap, c.gap, c.note)
		}
		if mjAbs(s.Yahrzeit-c.yz) > 0.05 {
			t.Errorf("day %d yahrzeit = %.3f, want %.3f (%s)", c.day, s.Yahrzeit, c.yz, c.note)
		}
	}
}

// The Hebrew layer must not perturb the self-location by one bit: personal_dissonance's birth-quake
// (day 731) stays 0.6916. janus_gap and pd share only the read-only mj_days clock.
func TestMetaJanusPdUnaffectedByHebrew(t *testing.T) {
	a := yent.NewAMK()
	a.Exec("BIRTH 498")
	a.Exec("SELF_NOW_DAYS 731")
	a.Step(1.0)
	if d := a.GetState().PersonalDissonance; mjAbs(d-0.6916) > 0.001 {
		t.Fatalf("Hebrew layer perturbed personal_dissonance: %.4f, want 0.6916 (bit-identical regression)", d)
	}
}
