package innerworld

import (
	"math"
	"strings"
	"testing"
)

func TestFeelEntropyMatchesJuliaOracle(t *testing.T) {
	// "a a b c" is the word distribution [.5,.25,.25]; the embedded libjulia smoke gave
	// ent([.5,.25,.25]) = 1.039721 nats. The Go port must match the Julia oracle exactly.
	if h := feelEntropy("a a b c"); math.Abs(float64(h)-1.039721) > 1e-4 {
		t.Errorf("Go entropy must match the Julia oracle: got %.6f want 1.039721", h)
	}
	if h := feelEntropy("same same same"); h != 0 {
		t.Errorf("a fully repetitive thought is zero-entropy (focused), got %.6f", h)
	}
	if feelEntropy("") != 0 {
		t.Error("empty thought is zero-entropy")
	}
}

func TestFeelResonance(t *testing.T) {
	if r := feelResonance("light meets shadow", "light meets shadow"); r != 1 {
		t.Errorf("identical thoughts fully resonate, got %.3f", r)
	}
	if r := feelResonance("light meets shadow", "cold iron rust"); r != 0 {
		t.Errorf("disjoint thoughts do not resonate, got %.3f", r)
	}
	// {light,meets,shadow} ∩ {light,meets,cold} = 2, ∪ = 4 -> 0.5
	if r := feelResonance("light meets shadow", "light meets cold"); math.Abs(float64(r)-0.5) > 1e-6 {
		t.Errorf("partial overlap should be 0.5 Jaccard, got %.3f", r)
	}
	if r := feelResonance("x", ""); r != 0 {
		t.Errorf("no echo against emptiness, got %.3f", r)
	}
}

func TestFeelTextLean(t *testing.T) {
	if v, a := feelText("i love this wonderful beautiful joy"); v <= 0 || a <= 0 {
		t.Errorf("a warm thought should lean positive, got v=%.2f a=%.2f", v, a)
	}
	if v, a := feelText("pain fear suffer alone lonely hopeless"); v >= 0 || a <= 0 {
		t.Errorf("a dark thought should lean negative, got v=%.2f a=%.2f", v, a)
	}
	if v, a := feelText("the cat sat quietly on the mat"); v != 0 || a != 0 {
		t.Errorf("an uncharged thought is flat, got v=%.2f a=%.2f", v, a)
	}
	if v, a := feelText(""); v != 0 || a != 0 {
		t.Errorf("empty is flat, got v=%.2f a=%.2f", v, a)
	}
}

func hasPrefix(scripts []string, prefix string) bool {
	for _, s := range scripts {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func TestHighFeelWarmsOnPositive(t *testing.T) {
	f := &fakeField{}
	iw := NewInnerWorld(nil, f, nil)
	iw.EnableFeeling()
	iw.genMu.Lock()
	iw.highFeelLocked([]Circle{{Text: "i love this, it is wonderful and beautiful joy"}})
	iw.genMu.Unlock()

	scripts := f.scriptList()
	if !hasPrefix(scripts, "VALENCE") || !hasPrefix(scripts, "AROUSAL") {
		t.Errorf("feeling should always publish VALENCE/AROUSAL (the SARTRE feed), got %v", scripts)
	}
	if !hasPrefix(scripts, "WARMTH") || !hasPrefix(scripts, "FLOW") {
		t.Errorf("a positive thought should warm + flow the field, got %v", scripts)
	}
	if hasPrefix(scripts, "PAIN") {
		t.Errorf("a positive thought must not pain the field, got %v", scripts)
	}
}

func TestHighFeelPainsOnNegative(t *testing.T) {
	f := &fakeField{}
	iw := NewInnerWorld(nil, f, nil)
	iw.EnableFeeling()
	iw.genMu.Lock()
	iw.highFeelLocked([]Circle{{Text: "alone, broken, hopeless — only pain and fear"}})
	iw.genMu.Unlock()

	scripts := f.scriptList()
	if !hasPrefix(scripts, "PAIN") || !hasPrefix(scripts, "TENSION") {
		t.Errorf("a dark thought should pain + tighten the field, got %v", scripts)
	}
	if hasPrefix(scripts, "WARMTH") {
		t.Errorf("a dark thought must not warm the field, got %v", scripts)
	}
}

func TestFeelScarSinksIntenseEmotion(t *testing.T) {
	f := &fakeField{}
	iw := NewInnerWorld(nil, f, nil)
	iw.EnableFeeling()
	sea := NewScarMemory(0.985)
	iw.SetScar(sea, 999) // prophecy threshold high — ONLY emotion can scar here
	iw.genMu.Lock()
	iw.highFeelLocked([]Circle{{Text: "alone broken hopeless, only pain and fear and suffering"}})
	iw.genMu.Unlock()
	if n, _ := sea.Stats(); n == 0 {
		t.Error("an intensely-felt thought should settle into the sea of memory")
	}
}

func TestFeelScarTraumaHoldsLonger(t *testing.T) {
	deposit := func(text string) float32 {
		f := &fakeField{}
		iw := NewInnerWorld(nil, f, nil)
		iw.EnableFeeling()
		sea := NewScarMemory(0.985)
		iw.SetScar(sea, 999)
		iw.genMu.Lock()
		iw.highFeelLocked([]Circle{{Text: text}})
		iw.genMu.Unlock()
		_, total := sea.Stats()
		return total
	}
	neg := deposit("hate pain fear suffer awful terrible")
	pos := deposit("love joy wonderful beautiful amazing brilliant")
	if neg <= pos {
		t.Errorf("a wound should hold heavier than a joy of equal intensity: neg=%.3f pos=%.3f", neg, pos)
	}
}

func TestFeelScarMildDoesNotSettle(t *testing.T) {
	f := &fakeField{}
	iw := NewInnerWorld(nil, f, nil)
	iw.EnableFeeling()
	sea := NewScarMemory(0.985)
	iw.SetScar(sea, 999)
	iw.genMu.Lock()
	iw.highFeelLocked([]Circle{{Text: "the okay fine day passed by the way"}}) // faint charge
	iw.genMu.Unlock()
	if n, _ := sea.Stats(); n != 0 {
		t.Errorf("a passing mild feeling must not settle into the sea, n=%d", n)
	}
}

func TestScarSurfaceResonatesWithFeeling(t *testing.T) {
	f := &fakeField{} // fieldDebt starts 0
	iw := NewInnerWorld(nil, f, nil)
	sea := NewScarMemory(0.985)
	sea.Scar("a deep wound", 1.0)
	iw.SetScar(sea, 0)

	iw.genMu.Lock()
	got0 := iw.scarSurface("now")
	iw.feelIntensity = 0.8 // a strong present feeling
	got1 := iw.scarSurface("now")
	iw.genMu.Unlock()

	if got0 != "now" {
		t.Errorf("no debt, no feeling -> nothing resurfaces, got %q", got0)
	}
	if !strings.Contains(got1, "a deep wound") {
		t.Errorf("a strong present feeling should resurface a resonant scar, got %q", got1)
	}
}

func TestHighFeelDisabledIsNoop(t *testing.T) {
	f := &fakeField{}
	iw := NewInnerWorld(nil, f, nil) // EnableFeeling NOT called
	iw.genMu.Lock()
	iw.highFeelLocked([]Circle{{Text: "i love this wonderful joy"}})
	iw.genMu.Unlock()
	if len(f.scriptList()) != 0 {
		t.Error("the High brain off must be a no-op")
	}
}

func TestHighFeelFlatPublishesZeroNoAffect(t *testing.T) {
	f := &fakeField{}
	iw := NewInnerWorld(nil, f, nil)
	iw.EnableFeeling()
	iw.genMu.Lock()
	iw.highFeelLocked([]Circle{{Text: "the cat sat quietly on the mat"}})
	iw.genMu.Unlock()
	scripts := f.scriptList()
	// a flat thought still publishes a live reading (valence/arousal 0) for SARTRE...
	if !hasPrefix(scripts, "VALENCE") || !hasPrefix(scripts, "AROUSAL") {
		t.Errorf("a flat thought should still publish a 0 reading, got %v", scripts)
	}
	// ...but it stirs no mood (no affect poles).
	if hasPrefix(scripts, "WARMTH") || hasPrefix(scripts, "PAIN") ||
		hasPrefix(scripts, "FLOW") || hasPrefix(scripts, "TENSION") {
		t.Errorf("an uncharged thought must stir no affect, got %v", scripts)
	}
}
