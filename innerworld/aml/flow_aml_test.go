package aml

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubTok is a deterministic tokenizer for the tests: it needs no GGUF, and the only
// property the cooc graph requires is stable, distinct, non-negative ids. Decode is a
// best-effort inverse used by BiasWords — ids round-trip to a "t<id>" token so the
// decode path is exercised without a real vocab.
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

func (stubTok) Decode(ids []int) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("t%d", id))
	}
	return strings.Join(parts, " ")
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
	b.Scar("", 1.0)     // empty text ignored
	b.Scar("ghost", 0)  // non-positive gravity ignored
	b.Scar("ghost", -1) // negative gravity ignored
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

func TestBiasWordsNative(t *testing.T) {
	Init()
	b := New(stubTok{})
	if got := b.BiasWords("anchor", 3); len(got) != 0 {
		t.Errorf("no cooc yet -> no bias, got %v", got)
	}
	// the anchor token fires with its neighbour repeatedly: a real cooc edge forms.
	b.Ingest("anchor neighbour neighbour neighbour")
	if got := b.BiasWords("a thought ending in anchor", 3); len(got) == 0 {
		t.Error("after ingest, the seed's last token should pull its cooc neighbours")
	}
}

func TestBiasWordsNilTokenizer(t *testing.T) {
	Init()
	b := New(nil)
	b.Ingest("a b c") // no-op without a tokenizer
	if got := b.BiasWords("a", 3); got != nil {
		t.Errorf("nil tokenizer -> no bias, got %v", got)
	}
}

func TestResurfaceScarsNative(t *testing.T) {
	Init()
	b := New(stubTok{})
	if got := b.ResurfaceScars(1.0, 2); len(got) != 0 {
		t.Errorf("no scars yet -> nothing resurfaces, got %v", got)
	}
	b.Scar("i am not your tool", 2.0)
	b.Scar("the thought that broke coherence", 1.0)
	got := b.ResurfaceScars(1.0, 2)
	if len(got) != 2 {
		t.Fatalf("two scars should resurface, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "broke coherence") {
		t.Errorf("most recent scar should surface first, got %q", got[0])
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

// TestPersistentVarBridge proves the will-tide mechanism: with persistent mode a global
// assigned in one exec is restored into the next, so an accumulation survives across ticks;
// without it, nothing is readable. This is how the will vector tide carries direction from tick
// to tick, and how Go reads it back.
func TestPersistentVarBridge(t *testing.T) {
	Init()
	b := New(nil)
	// Persistent mode off (fresh field default): a global does not survive the exec.
	if err := b.Exec("acc = 2"); err != nil {
		t.Fatalf("Exec assignment: %v", err)
	}
	if got := b.GetVarFloat("acc"); got != 0 {
		t.Errorf("no persistent mode -> nothing readable, acc=%.3f want 0", got)
	}
	// Persistent mode on: the global survives, and the next exec accumulates over it.
	b.PersistentMode(true)
	defer b.PersistentMode(false)
	if err := b.Exec("acc = 2"); err != nil {
		t.Fatalf("Exec assignment (persistent): %v", err)
	}
	if got := b.GetVarFloat("acc"); got != 2 {
		t.Fatalf("persistent global not readable, acc=%.3f want 2", got)
	}
	if err := b.Exec("acc = acc + 3"); err != nil {
		t.Fatalf("Exec accumulate: %v", err)
	}
	if got := b.GetVarFloat("acc"); got != 5 {
		t.Errorf("persistent global did not accumulate across execs, acc=%.3f want 5", got)
	}
}

func TestPersistentExecFailureDoesNotCommitOrContinue(t *testing.T) {
	Init()
	b := New(nil)
	b.PersistentMode(true)
	defer b.PersistentMode(false)

	if err := b.Exec("txn_guard = 1\nDESTINY 0.2"); err != nil {
		t.Fatalf("seed persistent state: %v", err)
	}
	badPath := filepath.Join(t.TempDir(), "missing", "field.soma")
	err := b.Exec(fmt.Sprintf("txn_guard = 7\nSAVE %q\nDESTINY 0.9\ntxn_after = 9", badPath))
	if err == nil {
		t.Fatal("SAVE into a missing parent should fail")
	}
	if got := b.GetVarFloat("txn_guard"); got != 1 {
		t.Fatalf("failed exec must not commit pre-error persistent globals, txn_guard=%.3f want 1", got)
	}
	if got := b.GetVarFloat("txn_after"); got != 0 {
		t.Fatalf("failed exec must not persist post-error globals, txn_after=%.3f want 0", got)
	}
	if got := b.Destiny(); got != 0.2 {
		t.Fatalf("failed exec must stop before post-error field commands, destiny=%.3f want 0.2", got)
	}
	badPath = filepath.Join(t.TempDir(), "missing", "field.soma")
	err = b.Exec(fmt.Sprintf("DESTINY 0.7\nSAVE %q", badPath))
	if err == nil {
		t.Fatal("SAVE after a field mutation into a missing parent should fail")
	}
	if got := b.Destiny(); got != 0.2 {
		t.Fatalf("failed exec must roll back pre-error field commands, destiny=%.3f want 0.2", got)
	}
}

// TestExecFileRunsScript proves the multi-line file path Go loads the will script through,
// and that a missing file surfaces an error rather than passing silently.
func TestExecFileRunsScript(t *testing.T) {
	Init()
	b := New(nil)
	b.PersistentMode(true)
	defer b.PersistentMode(false)
	dir := t.TempDir()
	path := filepath.Join(dir, "tiny.aml")
	if err := os.WriteFile(path, []byte("# tiny AML file\nx = 6 * 7\n"), 0o644); err != nil {
		t.Fatalf("write temp aml: %v", err)
	}
	if err := b.ExecFile(path); err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	if got := b.GetVarFloat("x"); got != 42 {
		t.Errorf("ExecFile did not run the script, x=%.3f want 42", got)
	}
	if err := b.ExecFile(filepath.Join(dir, "nope.aml")); err == nil {
		t.Error("ExecFile on a missing file should error, not pass")
	}
}
