package innerworld

import (
	"strings"
	"testing"
)

func TestNgramDivergence(t *testing.T) {
	if d := NgramDivergence("the void is patient", "the void is patient"); d > 0.001 {
		t.Errorf("identical should be ~0, got %.3f", d)
	}
	if d := NgramDivergence("abcabc", "xyzxyz"); d < 0.99 {
		t.Errorf("disjoint should be ~1, got %.3f", d)
	}
	if d := NgramDivergence("", ""); d != 0 {
		t.Errorf("empty/empty should be 0, got %.3f", d)
	}
	if d := NgramDivergence("something", ""); d != 1 {
		t.Errorf("one empty should be 1, got %.3f", d)
	}
	for _, p := range [][2]string{{"a", "b"}, {"persist", "persistence"}, {"hello world", "world hello"}} {
		if d := NgramDivergence(p[0], p[1]); d < 0 || d > 1 {
			t.Errorf("out of range %q/%q -> %.3f", p[0], p[1], d)
		}
	}
}

func TestNgramBeatsJaccardOnMorphology(t *testing.T) {
	// "persist" and "persistence" share trigrams but are disjoint whole words, so the
	// trigram divergence must be strictly smaller than a word-set Jaccard would give.
	a, b := "I persist", "persistence remains"
	ng := NgramDivergence(a, b)
	jac := wordJaccard(a, b)
	if jac < 1.0 {
		t.Fatalf("test premise broken: these share a whole word (jaccard=%.3f)", jac)
	}
	if ng >= jac {
		t.Errorf("trigram (%.3f) should be closer than word Jaccard (%.3f) on morphological overlap", ng, jac)
	}
}

// wordJaccard is a local reference to the old primitive, so the test can show the
// trigram measure is strictly better on morphology.
func wordJaccard(a, b string) float32 {
	wa, wb := map[string]bool{}, map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(a)) {
		wa[w] = true
	}
	for _, w := range strings.Fields(strings.ToLower(b)) {
		wb[w] = true
	}
	if len(wa) == 0 && len(wb) == 0 {
		return 0
	}
	inter := 0
	for w := range wa {
		if wb[w] {
			inter++
		}
	}
	union := len(wa) + len(wb) - inter
	if union == 0 {
		return 0
	}
	return 1 - float32(inter)/float32(union)
}
