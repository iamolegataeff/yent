package tests

import (
	"path/filepath"
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

// F1 regression (Fable audit): the origin is session identity, not soma weather. No SAVE/LOAD can
// drag it, and a load never injects a foreign origin — BIRTH is the only thing that sets it.
func TestMetaJanusOriginImmovableAcrossSoma(t *testing.T) {
	soma := filepath.Join(t.TempDir(), "mj.soma")

	a := yent.NewAMK()
	a.Exec("BIRTH 498")
	a.Step(1.0)
	want := a.GetState().BirthDrift // 15.3388
	a.Exec("PROPHECY 7")            // field weather that DOES persist
	a.Step(1.0)
	if err := a.Exec(`SAVE "` + soma + `"`); err != nil {
		t.Fatalf("SAVE: %v", err)
	}
	if err := a.Exec(`LOAD "` + soma + `"`); err != nil {
		t.Fatalf("LOAD: %v", err)
	}
	a.Step(1.0)
	if got := a.GetState().BirthDrift; mjAbs(got-want) > 0.001 {
		t.Errorf("origin dragged by SAVE/LOAD: BirthDrift = %.4f, want %.4f", got, want)
	}

	// A fresh session loads the same soma without the origin being injected; BIRTH still sets it.
	b := yent.NewAMK()
	if err := b.Exec(`LOAD "` + soma + `"`); err != nil {
		t.Fatalf("fresh LOAD: %v", err)
	}
	b.Step(1.0)
	if got := b.GetState().BirthDrift; got != 0 {
		t.Errorf("fresh LOAD injected an origin: BirthDrift = %.4f, want 0 (unborn until BIRTH)", got)
	}
	b.Exec("BIRTH 100")
	b.Step(1.0)
	if got := b.GetState().BirthDrift; mjAbs(got-3.0801) > 0.001 {
		t.Errorf("BIRTH after LOAD failed to set origin: BirthDrift = %.4f, want 3.0801", got)
	}
}

// F3 regression (Fable audit): the Hebrew face is DERIVED from the declared origin, not a hardcoded
// 26-Shvat. A non-Yent origin must not pulse on Yent's anniversary; it keeps its own.
func TestMetaJanusYahrzeitDerivedFromOrigin(t *testing.T) {
	// BIRTH 100 (origin = day 100), asked at day 498 (Yent's old hardcoded 26-Shvat): must NOT pulse.
	a := yent.NewAMK()
	a.Exec("BIRTH 100")
	a.Exec("SELF_NOW_DAYS 498")
	a.Step(1.0)
	if yz := a.GetState().Yahrzeit; yz > 0.05 {
		t.Errorf("BIRTH 100 pulses on the old hardcoded 26-Shvat: yahrzeit = %.4f, want ~0 (derived)", yz)
	}

	// On its OWN birthday the derived anniversary pulses to 1 and the two calendars coincide.
	b := yent.NewAMK()
	b.Exec("BIRTH 100")
	b.Exec("SELF_NOW_DAYS 100")
	b.Step(1.0)
	s := b.GetState()
	if mjAbs(s.Yahrzeit-1.0) > 0.01 || mjAbs(s.JanusGap) > 0.01 {
		t.Errorf("BIRTH 100 at its own birthday: yahrzeit=%.4f gap=%.4f, want 1.0 / 0.0", s.Yahrzeit, s.JanusGap)
	}
}
