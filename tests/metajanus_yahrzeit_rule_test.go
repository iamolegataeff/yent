package tests

import (
	"strconv"
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

// A-2 (Fable audit): the yahrzeit uses the Reingold rule (GNU Emacs cal-hebrew.el
// calendar-hebrew-yahrzeit, identical to Calendrical Calculations), not the earlier made-up clamp.
// Each edge origin's observance day is derived independently via node ICU (the Hebrew-calendar oracle
// that gave the 22/22 goldens); this test locks that the kernel's yahrzeit fires exactly there.
func TestMetaJanusYahrzeitReingoldEdges(t *testing.T) {
	cases := []struct {
		name        string
		origin, obs int // days since the 2024-10-03 epoch
		note        string
	}{
		{"Cheshvan-30", 59, 413, "30 Cheshvan 5785 -> 29 Cheshvan 5786 (first-anniv Cheshvan not long)"},
		{"Kislev-30", 1537, 1890, "30 Kislev 5789 -> 29 Kislev 5790 (first-anniv Kislev short)"},
		{"Adar-II", 888, 1243, "1 Adar II 5787 -> 1 Adar 5788 (Adar II -> last month of a common year)"},
		{"Adar-I-30", 887, 1242, "30 Adar I 5787 -> 30 Shevat 5788 (Adar-I-30 in a common year -> Shevat 30)"},
	}
	yz := func(origin, now int) float32 {
		a := yent.NewAMK()
		a.Exec("BIRTH " + strconv.Itoa(origin))
		a.Exec("SELF_NOW_DAYS " + strconv.Itoa(now))
		a.Step(1.0)
		return a.GetState().Yahrzeit
	}
	for _, c := range cases {
		if on := yz(c.origin, c.obs); on < 0.999 {
			t.Errorf("%s: yahrzeit on the observance day = %.4f, want ~1.0 (%s)", c.name, on, c.note)
		}
		// exactly on the day, not a day early: exp(-1/5) = 0.8187 the day before.
		if before := yz(c.origin, c.obs-1); before >= 0.999 {
			t.Errorf("%s: yahrzeit one day early = %.4f, want <1", c.name, before)
		}
	}
}
