package innerworld

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// ScarMemory is the sea of rejected thoughts — gravitational metanotes. A thought
// that dissonates with the field's prophecy (high prophecy-debt) is not discarded
// but kept as a scar with gravity. Gravity decays slowly across sleeps (leo
// klaus-scar, ~0.985), the seasonal harvest reinforces recurring scars and forgets
// the faded, and a scar can RESURRECT when a future metric resonates above its
// threshold (leo sea-of-memory: "weak memories sleep, resonance can resurrect
// them"). This is the AML SCAR / dark-matter lineage — gravitational memory from
// rejected injections — that meta-learning on what the organism refused continues
// straight from our DPO epistemic-self-contour work. Pure Go; the C `SCAR` operator
// is a later wiring. Concurrency-safe.
type ScarMemory struct {
	mu    sync.Mutex
	scars map[string]float32 // scar text -> gravity
	decay float32            // gravity multiplier per consolidation (leo ~0.985)
}

// NewScarMemory builds an empty sea with the given per-consolidation gravity decay
// (0<decay<=1; 0 or out-of-range falls back to 0.985, the leo klaus-scar rate).
func NewScarMemory(decay float32) *ScarMemory {
	if decay <= 0 || decay > 1 {
		decay = 0.985
	}
	return &ScarMemory{scars: map[string]float32{}, decay: decay}
}

// Scar sinks a rejected thought into the sea, adding gravity (a recurring rejection
// accumulates and so survives longer — the wound that keeps reopening holds). Empty
// text or non-positive gravity is ignored.
func (s *ScarMemory) Scar(text string, gravity float32) {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" || gravity <= 0 {
		return
	}
	if r := []rune(text); len(r) > 240 {
		text = string(r[:240])
	}
	s.mu.Lock()
	s.scars[text] += gravity
	s.mu.Unlock()
}

// Consolidate is the seasonal pass over the sea: every scar's gravity decays, and
// scars that fall under the floor are forgotten. Recurring scars (heavier gravity)
// survive many sleeps; one-off dissonance fades. Returns the number forgotten.
// (Reinforce is implicit — recurring rejection accumulates gravity at Scar time, the
// way trauma-spore in leo holds longer.)
func (s *ScarMemory) Consolidate(pruneFloor float32) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	forgotten := 0
	for text, g := range s.scars {
		g *= s.decay
		if g < pruneFloor {
			delete(s.scars, text)
			forgotten++
		} else {
			s.scars[text] = g
		}
	}
	return forgotten
}

// Resurrect surfaces the scars whose gravity has risen above the resonance level —
// the rejected thoughts a present metric pulls back up from the sea. Strongest
// first, up to n. resonance<=0 or n<=0 returns nothing.
func (s *ScarMemory) Resurrect(resonance float32, n int) []string {
	if resonance <= 0 || n <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	type sc struct {
		text string
		g    float32
	}
	var hits []sc
	for text, g := range s.scars {
		if g >= resonance {
			hits = append(hits, sc{text, g})
		}
	}
	sort.Slice(hits, func(a, b int) bool { return hits[a].g > hits[b].g })
	if n > len(hits) {
		n = len(hits)
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = hits[i].text
	}
	return out
}

// Stats reports the live scar count and total gravity.
func (s *ScarMemory) Stats() (n int, total float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, g := range s.scars {
		n++
		total += g
	}
	return n, total
}

// ScarConsolidator is the Б3 consolidation stage: the seasonal decay/prune of the
// sea of rejected thoughts, run during sleep.
type ScarConsolidator struct {
	Sea        *ScarMemory
	PruneFloor float32
}

func (c *ScarConsolidator) Consolidate(_ context.Context) error {
	if c.Sea == nil {
		return nil
	}
	c.Sea.Consolidate(c.PruneFloor)
	return nil
}

func (c *ScarConsolidator) Name() string { return "scar" }

// SetScar wires the sea of rejected thoughts and the prophecy-debt threshold above
// which a thought is scarred (rejected by the field). Set before Think/Breathe start.
func (iw *InnerWorld) SetScar(sea *ScarMemory, debtThreshold float32) {
	iw.genMu.Lock()
	iw.scar = sea
	iw.scarThreshold = debtThreshold
	iw.genMu.Unlock()
}

// scarLocked sinks the just-finished thought into the sea when it dissonated with the
// field's prophecy — i.e. the prophecy-debt of this batch rose above the threshold.
// The gravity is the debt itself: the more the thought broke prophecy-destiny
// coherence, the deeper the scar. Caller holds genMu.
func (iw *InnerWorld) scarLocked(circles []Circle, debt float32) {
	if len(circles) == 0 || debt <= iw.scarThreshold {
		return
	}
	text := circles[len(circles)-1].Text
	if iw.flow != nil {
		iw.flow.Scar(text, debt) // native gravitational memory: the SCAR operator
		return
	}
	if iw.scar == nil {
		return
	}
	iw.scar.Scar(text, debt)
}

// scarSurface lets a scar resurrect into the next seed when the present field debt
// resonates with a past rejection (leo sea-of-memory: a metric pulls a sleeping
// memory back up). The rejected thought returns not as a quote to continue but as a
// surfaced scar — meta-learning on what the organism refused. No scar / no resonance
// = unchanged. NO-SEED-FROM-PROMPT holds (still transformed by innerSeed).
func (iw *InnerWorld) scarSurface(prompt string) string {
	// Resonance pulls scars up by the field's prophecy-debt OR the organism's present
	// emotional intensity — a strong feeling now resurfaces past intense feelings too.
	resonance := iw.fieldDebt()
	if iw.feelIntensity > resonance {
		resonance = iw.feelIntensity
	}
	var risen []string
	switch {
	case iw.flow != nil:
		_, scarN := metajanusHarvestLean(metajanusTemporalAlpha(iw.flow)) // D-2: temporal_alpha leans the harvest
		risen = iw.flow.ResurfaceScars(resonance, scarN) // native scar sea
	case iw.scar != nil:
		risen = iw.scar.Resurrect(resonance, 2)
	}
	if len(risen) == 0 {
		return prompt
	}
	return "(a scar resurfaces, not to repeat but to remember what was refused: " +
		strings.Join(risen, " | ") + ") " + prompt
}
