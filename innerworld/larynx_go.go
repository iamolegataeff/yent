package innerworld

import (
	"hash/fnv"
	"math"
	"strings"
)

// Larynx is the membrane between the two bodies. It reads the texture of the fast
// body's circles and returns a coupling factor in [0,1]: how strongly the deep
// body should attend to them. larynx.zig is the optimized membrane on the Metal
// runtime; textureLarynx is the portable Go mirror used elsewhere — both compute
// the same entropy * (1 - repetition) coupling.
type Larynx interface {
	Couple(circles []Circle) float32
}

// textureLarynx is the pure-Go membrane, mirroring larynx.zig: a flowing,
// non-looping stream of thought couples; a flat or looping one does not.
type textureLarynx struct{}

func (textureLarynx) Couple(circles []Circle) float32 {
	toks := tokenize(circles)
	if len(toks) == 0 {
		return 0
	}
	n := float32(len(toks))

	freq := make(map[uint32]float32, len(toks))
	for _, t := range toks {
		freq[t]++
	}

	var h float32
	for _, c := range freq {
		p := c / n
		h -= p * float32(math.Log2(float64(p)))
	}

	distinct := float32(len(freq))
	repetition := 1 - distinct/n
	maxH := float32(1)
	if distinct > 1 {
		maxH = float32(math.Log2(float64(distinct)))
	}
	entropy := float32(0)
	if maxH > 0 {
		entropy = h / maxH
	}
	return clamp01(entropy * (1 - repetition))
}

// tokenize splits the circle texts into word tokens hashed to u32 — a portable
// stand-in for the runtime's tekken tokenizer. The texture measure only needs a
// token stream, not the exact vocabulary.
func tokenize(circles []Circle) []uint32 {
	var toks []uint32
	for _, c := range circles {
		for _, w := range strings.Fields(c.Text) {
			hsh := fnv.New32a()
			_, _ = hsh.Write([]byte(strings.ToLower(w)))
			toks = append(toks, hsh.Sum32())
		}
	}
	return toks
}
