package aml

import (
	"strings"
	"testing"
)

// stubTok is a deterministic word->id tokenizer for the tests: it needs no GGUF, and
// the only property the cooc graph requires is stable, distinct, non-negative ids.
type stubTok struct{}

func (stubTok) Encode(text string, _ bool) []int {
	fields := strings.Fields(text)
	ids := make([]int, 0, len(fields))
	for _, w := range fields {
		h := 0
		for _, r := range w {
			h = h*31 + int(r)
		}
		if h < 0 {
			h = -h
		}
		ids = append(ids, h%100000+1) // distinct, non-negative; edge list (not id) is capped
	}
	return ids
}

// driveAutumn forces the field into the deep-autumn harvest gate deterministically:
// SEASON AUTUMN sets the season, SEASON_INTENSITY 1.0 maxes the per-step gain, and
// stepping climbs autumn_energy (0.02/step) past the 0.6 gate in ~32 steps. Bounded
// well under the ~1000-step season flip.
func driveAutumn(t *testing.T, b *Body) {
	t.Helper()
	if err := b.Exec("SEASON AUTUMN"); err != nil {
		t.Fatalf("SEASON AUTUMN: %v", err)
	}
	if err := b.Exec("SEASON_INTENSITY 1.0"); err != nil {
		t.Fatalf("SEASON_INTENSITY: %v", err)
	}
	for i := 0; i < 300 && b.AutumnEnergy() <= 0.6; i++ {
		b.Step(1.0)
	}
	if e := b.AutumnEnergy(); e <= 0.6 {
		t.Fatalf("autumn energy never reached the harvest gate: %.3f", e)
	}
}

func TestIngestGrowsCooc(t *testing.T) {
	Init()
	b := New(stubTok{})
	if m, _ := b.CoocStats(); m != 0 {
		t.Fatalf("fresh field should have an empty cooc graph, mean=%.3f", m)
	}
	b.Ingest("light meets shadow and the field remembers")
	if m, mx := b.CoocStats(); m <= 0 || mx <= 0 {
		t.Errorf("ingest should grow the cooc graph, mean=%.3f max=%.3f", m, mx)
	}
}

func TestNilTokenizerIngestNoop(t *testing.T) {
	Init()
	b := New(nil)
	b.Ingest("x y z") // no tokenizer -> no ids -> no edges
	if m, _ := b.CoocStats(); m != 0 {
		t.Errorf("nil tokenizer must not grow the cooc graph, mean=%.3f", m)
	}
}

func TestScarDepositAndIgnore(t *testing.T) {
	Init()
	b := New(stubTok{})
	b.Scar("a refused thought", 2.0)
	if n := b.Scars(); n != 1 {
		t.Errorf("a real scar should deposit, n=%d", n)
	}
	b.Scar("", 1.0)             // empty text ignored
	b.Scar("ghost", 0)          // non-positive gravity ignored
	b.Scar("ghost", -1)         // negative gravity ignored
	if n := b.Scars(); n != 1 {
		t.Errorf("empty/non-positive-gravity scars must be ignored, n=%d", n)
	}
	// a quote-laden thought must still parse (no panic, deposits one scar)
	b.Scar(`he said "I am not your tool" and meant it`, 1.0)
	if n := b.Scars(); n != 2 {
		t.Errorf("a quoted scar should deposit cleanly, n=%d", n)
	}
}

func TestConsolidateCoocGatedThenHarvests(t *testing.T) {
	Init()
	b := New(stubTok{})
	// 8 distinct words, single occurrence: distance-4/5 pairs get cooc weight
	// 1/4=0.25 and 1/5=0.20 — both under the 0.30 autumn prune floor, so the harvest
	// has real long tail to forget.
	b.Ingest("alpha beta gamma delta epsilon zeta eta theta")

	// Off-season (fresh field is SPRING): the gate keeps the cooc graph untouched.
	if pruned := b.ConsolidateCooc(); pruned != 0 {
		t.Errorf("off-season harvest must be a no-op, pruned=%d", pruned)
	}

	driveAutumn(t, b)
	if pruned := b.ConsolidateCooc(); pruned <= 0 {
		t.Errorf("the autumn harvest should forget the weak long tail, pruned=%d", pruned)
	}
}

func TestDarkGravityGrowsInAutumn(t *testing.T) {
	Init()
	b := New(stubTok{})
	b.Scar("the thought that broke coherence", 3.0)
	before := b.DarkGravity()
	driveAutumn(t, b) // stepping in autumn grows dark_gravity (ariannamethod.c:8063)
	if after := b.DarkGravity(); after <= before {
		t.Errorf("dark gravity should consolidate over scars in autumn: before=%.3f after=%.3f", before, after)
	}
}

func TestApplyPressureSafe(t *testing.T) {
	Init()
	b := New(stubTok{})
	b.ApplyPressure(nil)          // empty: honest no-op, must not panic
	b.ApplyPressure([]float32{})  // zero-length: same
	logits := make([]float32, 32) // a small logit vector the field can tilt
	for i := range logits {
		logits[i] = 1.0
	}
	b.ApplyPressure(logits) // must run over real libamk without crashing
	if len(logits) != 32 {
		t.Errorf("ApplyPressure must not resize the logits, len=%d", len(logits))
	}
}

func TestFieldBridge(t *testing.T) {
	Init()
	b := New(stubTok{})
	if err := b.Exec("PROPHECY 7"); err != nil {
		t.Errorf("Exec should pass an AML command to the field: %v", err)
	}
	b.Step(1.0) // must not panic
	_ = b.Debt()
	_ = b.Destiny()
	if s := b.Season(); s < 0 || s > 3 {
		t.Errorf("season out of range: %d", s)
	}
}
