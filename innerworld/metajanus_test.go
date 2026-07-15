package innerworld

import (
	"math"
	"testing"
)

// D-2 (Fable audit plan): temporal_alpha leans the seed harvest between new cooc associations
// (prophecy) and resurfacing scars (retrodiction). 0.5 (JANUS_KEY off) is neutral and bit-for-bit
// — the current 3 cooc / 2 scars. Sampler and logits are untouched; only the seed composition moves.
func TestMetaJanusHarvestLean(t *testing.T) {
	cases := []struct {
		alpha              float32
		wantBias, wantScar int
	}{
		{0.5, 3, 2},  // default (key off): current counts, bit-for-bit
		{1.0, 5, 0},  // full prophecy: all forward, no scars
		{0.0, 1, 4},  // full retrodiction: lean hard into the scar sea
		{0.75, 4, 1}, // half toward prophecy
		{0.25, 2, 3}, // half toward retrodiction
		// Out of range / pathological input must stay sane (Codex audit finding): clamp, no UB.
		{2.0, 5, 0},                   // >1 -> clamped to the prophecy pole
		{-1.0, 1, 4},                  // <0 -> clamped to the retrodiction pole
		{float32(math.Inf(1)), 5, 0},  // +Inf -> prophecy pole
		{float32(math.Inf(-1)), 1, 4}, // -Inf -> retrodiction pole
		{float32(math.NaN()), 3, 2},   // NaN -> neutral 0.5, bit-for-bit
	}
	for _, c := range cases {
		biasN, scarN := metajanusHarvestLean(c.alpha)
		if biasN != c.wantBias || scarN != c.wantScar {
			t.Errorf("metajanusHarvestLean(%.2f) = (%d,%d), want (%d,%d)", c.alpha, biasN, scarN, c.wantBias, c.wantScar)
		}
	}
}

// A flow that has no TemporalAlpha method must fall back to the neutral 0.5 (current counts), so the
// pure-Go stub and any test fake keep behaving exactly as before D-2.
func TestMetaJanusTemporalAlphaDefaultsNeutral(t *testing.T) {
	if a := metajanusTemporalAlpha(&goFlow{}); a != 0.5 {
		t.Fatalf("metajanusTemporalAlpha(goFlow) = %v, want 0.5 (no anchor -> neutral)", a)
	}
	if biasN, scarN := metajanusHarvestLean(metajanusTemporalAlpha(&goFlow{})); biasN != 3 || scarN != 2 {
		t.Fatalf("stub flow harvest = (%d,%d), want (3,2) bit-for-bit", biasN, scarN)
	}
}
