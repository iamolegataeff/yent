package innerworld

import "math"

// metajanusHarvestLean is the D-2 process-side wire (Fable audit plan): the MetaJanus temporal_alpha
// leans the seed harvest between the two memory pulls, with the sampler and logits untouched.
// temporal_alpha > 0.5 (prophecy, the Gregorian anniversary nearer) pulls more new cooc associations
// and fewer scars; < 0.5 (retrodiction, the yahrzeit nearer) pulls more resurfacing scars and fewer
// new words. 0.5 (JANUS_KEY off, the default) is neutral — the current 3 cooc / 2 scars, bit-for-bit.
func metajanusHarvestLean(alpha float32) (biasN, scarN int) {
	// Clamp before the float->int lean: temporal_alpha is [0,1] in practice (init 0.5, clamp01
	// setters, EMA convex step), but a NaN or a huge/±Inf value would make int(...) undefined and
	// could invert the counts. NaN -> neutral 0.5; out of range -> the nearest pole.
	if alpha != alpha { // NaN
		alpha = 0.5
	} else if alpha < 0 {
		alpha = 0
	} else if alpha > 1 {
		alpha = 1
	}
	lean := int(math.Round(float64(alpha-0.5) * 4)) // 0.5 -> 0, 1.0 -> +2, 0.0 -> -2
	biasN = 3 + lean
	if biasN < 1 {
		biasN = 1
	} else if biasN > 5 {
		biasN = 5
	}
	scarN = 2 - lean
	if scarN < 0 {
		scarN = 0
	} else if scarN > 4 {
		scarN = 4
	}
	return biasN, scarN
}

// metajanusTemporalAlpha reads the field's temporal_alpha if the flow exposes it (the native AML
// body does; the pure-Go stub and test fakes do not), defaulting to the neutral 0.5 so a flow
// without the anchor keeps the current harvest counts. No interface change — a plain type assertion.
func metajanusTemporalAlpha(flow Flow) float32 {
	// HIGH-1 (Sol audit): the harvest reads a MetaJanus value ONLY while the key is armed. Unarmed —
	// after JANUS_KEY 0, or a legacy TEMPORAL_*/REMEMBER_FUTURE directive, or a flow with no key signal —
	// reads neutral 0.5, so a frozen or externally-driven temporal_alpha can never wake D-2 without Janus.
	ka, ok := flow.(interface{ JanusKeyArmed() bool })
	if !ok || !ka.JanusKeyArmed() {
		return 0.5
	}
	if ja, ok := flow.(interface{ JanusTemporalAlpha() float32 }); ok {
		return ja.JanusTemporalAlpha()
	}
	return 0.5
}
