//go:build julia

package feeling

// JuliaFeelMath is the production feeling-math backend: it implements innerworld.FeelMath
// (Entropy/Resonance) on the REAL in-process Julia runtime. Entropy is the character Shannon
// entropy (CharEntropy, run by Julia); Resonance is 1 - the semantic cosine distance
// (SemanticDistance, run by Julia). Satisfied structurally — no import of innerworld. The dock
// calls Init(feeling.jl) then injects this via innerworld.SetFeelMath; if Julia failed to load,
// the dock skips it and the inner world keeps the Go lexical proxy.
type JuliaFeelMath struct{}

func (JuliaFeelMath) Entropy(text string) float32 {
	return float32(CharEntropy(text))
}

func (JuliaFeelMath) Resonance(a, b string) float32 {
	r := 1.0 - SemanticDistance(a, b)
	if r < 0 {
		r = 0
	}
	return float32(r)
}
