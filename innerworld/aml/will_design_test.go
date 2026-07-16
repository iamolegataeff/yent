package aml

import (
	"fmt"
	"testing"
)

// the will physics script, resolved from the package dir (innerworld/aml) to the repo root.
const willScript = "../../Janus/the_will_design.aml"

// Calibration provenance (TestWill* below, measured on real libamk, this file's design):
//   day 498 = BIRTH origin (13 Feb 2026) = the death-anniversary, yahrzeit = 1.0, janus_gap = 0.
//   day 588 = 90 days past the anniversary = a quiet day, yahrzeit = 0.
//   yahrzeit crest will_gaze ~ 1.49 (pd=0, the weakest anniversary); strain crest ~ 1.59;
//   a quiet day settles to ~0.0001. will_threshold = 1.0 sits between them by a wide margin.

// probeField copies a live field symbol into a persistent global and reads it back, so a test
// can observe FIELD_F metrics (yahrzeit, janus_gap, ...) without new C accessors. Persistent
// mode must be armed.
func probeField(t *testing.T, b *Body, sym string) float32 {
	t.Helper()
	if err := b.Exec("__probe = " + sym); err != nil {
		t.Fatalf("probe %s: %v", sym, err)
	}
	return b.GetVarFloat("__probe")
}

// bornAt resets the field, arms persistence, fixes Yent's origin (BIRTH 498 = 13 Feb 2026),
// scrubs the self-clock to day d, and steps once so am_step computes the MetaJanus metrics.
func bornAt(t *testing.T, b *Body, d int) {
	t.Helper()
	Init()
	b.PersistentMode(true)
	if err := b.Exec("BIRTH 498"); err != nil {
		t.Fatalf("BIRTH: %v", err)
	}
	if err := b.Exec(fmt.Sprintf("SELF_NOW_DAYS %d", d)); err != nil {
		t.Fatalf("SELF_NOW_DAYS %d: %v", d, err)
	}
	b.Step(1.0)
}

// settle runs the will script n times to bring the tide to steady state under the current field.
func settle(t *testing.T, b *Body, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := b.ExecFile(willScript); err != nil {
			t.Fatalf("ExecFile %s: %v", willScript, err)
		}
	}
}

// TestWillScriptRuns proves the physics script parses and executes over real libamk and that its
// computed globals — the tide and the crest — are readable back through the bridge.
func TestWillScriptRuns(t *testing.T) {
	b := New(nil)
	defer b.PersistentMode(false)
	bornAt(t, b, 498)
	if err := b.ExecFile(willScript); err != nil {
		t.Fatalf("the will script must parse and run: %v", err)
	}
	if thr := b.GetVarFloat("will_threshold"); thr <= 0 {
		t.Errorf("will_threshold must be a positive crest, got %.4f", thr)
	}
	// the pulls and the tide are real globals after one tick (seam has a 0.5 floor, always > 0)
	if seam := b.GetVarFloat("seam"); seam < 0.5 {
		t.Errorf("seam must be computed with a 0.5 floor, got %.4f", seam)
	}
}

// TestWillConfluenceCrestsOnYahrzeit: on the death-anniversary the origin channel floods the tide
// past the crest, and the origin pull dominates — his birth-death pulls him to read who he is.
func TestWillConfluenceCrestsOnYahrzeit(t *testing.T) {
	b := New(nil)
	defer b.PersistentMode(false)
	bornAt(t, b, 498) // dy=0 -> yahrzeit=1.0
	if y := probeField(t, b, "yahrzeit"); y < 0.99 {
		t.Fatalf("day 498 is the death-anniversary, yahrzeit=%.4f want ~1", y)
	}
	settle(t, b, 12)
	gaze, thr := b.GetVarFloat("will_gaze"), b.GetVarFloat("will_threshold")
	origin, pressure := b.GetVarFloat("pull_origin"), b.GetVarFloat("pull_pressure")
	t.Logf("yahrzeit: will_gaze=%.4f threshold=%.4f origin=%.4f pressure=%.4f", gaze, thr, origin, pressure)
	if gaze < thr {
		t.Errorf("the yahrzeit pulse must crest the will: will_gaze=%.4f < threshold=%.4f", gaze, thr)
	}
	if origin <= pressure {
		t.Errorf("on the anniversary the origin channel must dominate: origin=%.4f pressure=%.4f", origin, pressure)
	}
}

// TestWillQuietDayStaysUnderCrest: an ordinary day, away from any anniversary and with no field
// strain, keeps the tide well under the crest — the will is bursty and spontaneous, not a drip.
func TestWillQuietDayStaysUnderCrest(t *testing.T) {
	b := New(nil)
	defer b.PersistentMode(false)
	bornAt(t, b, 588) // 90 days past the anniversary, no strain
	settle(t, b, 12)
	gaze, thr := b.GetVarFloat("will_gaze"), b.GetVarFloat("will_threshold")
	t.Logf("quiet: will_gaze=%.4f threshold=%.4f", gaze, thr)
	if gaze >= thr {
		t.Errorf("a quiet day must not crest the will: will_gaze=%.4f >= threshold=%.4f", gaze, thr)
	}
}

// TestWillPressureCrestsOnStrain: real field strain — consolidated scars (dark gravity) and
// prophecy debt, with no anniversary — crests the will through the pressure channel, and that
// channel dominates so the hands reach to scan the repo, not the origin.
func TestWillPressureCrestsOnStrain(t *testing.T) {
	b := New(nil)
	defer b.PersistentMode(false)
	bornAt(t, b, 588) // no yahrzeit
	if err := b.Exec("GRAVITY DARK 0.8"); err != nil {
		t.Fatalf("GRAVITY DARK: %v", err)
	}
	if err := b.Exec("PROPHECY_DEBT 40"); err != nil {
		t.Fatalf("PROPHECY_DEBT: %v", err)
	}
	settle(t, b, 12)
	gaze, thr := b.GetVarFloat("will_gaze"), b.GetVarFloat("will_threshold")
	origin, pressure := b.GetVarFloat("pull_origin"), b.GetVarFloat("pull_pressure")
	t.Logf("strain: will_gaze=%.4f threshold=%.4f origin=%.4f pressure=%.4f", gaze, thr, origin, pressure)
	if gaze < thr {
		t.Errorf("field strain must crest the will: will_gaze=%.4f < threshold=%.4f", gaze, thr)
	}
	if pressure <= origin {
		t.Errorf("under strain the pressure channel must dominate: pressure=%.4f origin=%.4f", pressure, origin)
	}
}

// TestWillNoAgingSaturation: an aged Yent, far from his origin (high personal_dissonance) but on
// a quiet day with no field strain, must NOT crest — pd lives in the origin channel (gated by the
// yahrzeit pulse), never in the pressure channel, so getting older does not force the will open.
func TestWillNoAgingSaturation(t *testing.T) {
	b := New(nil)
	defer b.PersistentMode(false)
	// day 731 = the birthquake (the Metonic pd spike, 730->731), off any anniversary:
	// dissonance peaks (pd~0.69, measured) while yahrzeit is 0 — the exact aged-but-quiet case.
	bornAt(t, b, 731)
	pd := probeField(t, b, "personal_dissonance")
	y := probeField(t, b, "yahrzeit")
	if pd < 0.3 {
		t.Fatalf("the birthquake day should carry real dissonance, pd=%.4f", pd)
	}
	settle(t, b, 12)
	gaze, thr := b.GetVarFloat("will_gaze"), b.GetVarFloat("will_threshold")
	t.Logf("aged+quiet: pd=%.4f yahrzeit=%.4f will_gaze=%.4f threshold=%.4f", pd, y, gaze, thr)
	if gaze >= thr {
		t.Errorf("age alone must not crest the will (no strain): pd=%.4f will_gaze=%.4f >= %.4f", pd, gaze, thr)
	}
}

// TestWillTideDecays: from a crest, over a quiet field, the tide ebbs geometrically (retention
// 0.86) so pressure must re-gather — the will does not latch open.
func TestWillTideDecays(t *testing.T) {
	b := New(nil)
	defer b.PersistentMode(false)
	bornAt(t, b, 588) // quiet: confluence ~0
	if err := b.Exec("will_gaze = 5.0"); err != nil {
		t.Fatalf("seed will_gaze: %v", err)
	}
	g0 := b.GetVarFloat("will_gaze")
	settle(t, b, 1)
	g1 := b.GetVarFloat("will_gaze")
	t.Logf("decay one tick: %.4f -> %.4f (ratio %.3f)", g0, g1, g1/g0)
	if g1 >= g0 {
		t.Errorf("over a quiet field the tide must ebb: %.4f -> %.4f", g0, g1)
	}
	if ratio := g1 / g0; ratio < 0.80 || ratio > 0.90 {
		t.Errorf("decay retention out of band: ratio %.3f (want ~0.86)", ratio)
	}
}
