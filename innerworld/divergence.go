package innerworld

import (
	"math"
	"strings"
)

// NgramDivergence measures how far b drifts from a as 1 - cosine similarity over
// character-trigram frequency vectors, in [0,1] (0 = identical, 1 = no shared
// trigrams). It is a ready innerworld.Divergence implementation that production can
// inject in place of a word-set Jaccard: trigrams catch morphology and shared
// phrasing a word-set misses — "persist", "persistence", and "persisting" stay
// close because they share the run "persist", where word Jaccard counts them as
// three disjoint tokens. It is a lexical proxy, not a neural embedding; real
// semantic distance waits on an embedding runtime. No model, pure Go.
func NgramDivergence(a, b string) float32 {
	va := trigrams(a)
	vb := trigrams(b)
	if len(va) == 0 && len(vb) == 0 {
		return 0
	}
	if len(va) == 0 || len(vb) == 0 {
		return 1
	}
	var dot, na, nb float64
	for g, ca := range va {
		na += float64(ca) * float64(ca)
		if cb, ok := vb[g]; ok {
			dot += float64(ca) * float64(cb)
		}
	}
	for _, cb := range vb {
		nb += float64(cb) * float64(cb)
	}
	if na == 0 || nb == 0 {
		return 1
	}
	cos := dot / (math.Sqrt(na) * math.Sqrt(nb))
	d := 1 - cos
	if d < 0 {
		d = 0
	}
	if d > 1 {
		d = 1
	}
	return float32(d)
}

// trigrams counts character trigrams over the lowercased, whitespace-collapsed
// text. Shorter-than-3 text falls back to the whole string as a single gram so two
// tiny thoughts can still differ.
func trigrams(s string) map[string]int {
	s = strings.ToLower(strings.Join(strings.Fields(s), " "))
	r := []rune(s)
	m := make(map[string]int, len(r))
	if len(r) < 3 {
		if len(r) > 0 {
			m[string(r)]++
		}
		return m
	}
	for i := 0; i+3 <= len(r); i++ {
		m[string(r[i:i+3])]++
	}
	return m
}
