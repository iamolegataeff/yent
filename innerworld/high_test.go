package innerworld

import (
	"strings"
	"testing"
)

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
