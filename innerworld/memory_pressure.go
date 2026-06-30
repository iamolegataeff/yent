package innerworld

import (
	"fmt"
	"strings"
)

// MemoryFieldPressure is the physical field pulse derived from recalled memory.
// It is deliberately small: memory pressure changes the AML field's posture before
// circles rise, while the recalled text still reaches the model only through the
// bounded "field traces" seed.
type MemoryFieldPressure struct {
	Score    int
	Prophecy int
	Velocity string
	Step     float32
}

// PressureMemory is an optional Memory extension for sources that can expose
// field pressure structurally. RI uses it so compiled records can press the AML
// field by kind/status, without re-parsing their trace text as a prompt-like cue.
type PressureMemory interface {
	Memory
	FieldPressureScore(n int) int
}

// FieldPressureForMemory turns selected memory traces into one bounded AML pulse.
// RI/limpha traces should not become a larger prompt wall; this gives them a
// second route into the organism as field pressure. Empty traces produce no pulse.
func FieldPressureForMemory(traces []string) (MemoryFieldPressure, bool) {
	score := 0
	for _, trace := range traces {
		score += tracePressureScore(trace)
	}
	return FieldPressureForScore(score)
}

// FieldPressureForScore maps any memory pressure score to the one bounded AML pulse.
// Scores may be accumulated by text traces or by typed sources; the cap is shared.
func FieldPressureForScore(score int) (MemoryFieldPressure, bool) {
	if score <= 0 {
		return MemoryFieldPressure{}, false
	}
	if score > 5 {
		score = 5
	}
	p := MemoryFieldPressure{
		Score:    score,     // applied score after the cap, so receipts match physics.
		Prophecy: 1 + score, // 2..6; memory never claims the whole horizon alone.
		Velocity: "WALK",
		Step:     0.15 + 0.04*float32(score),
	}
	if p.Step > 0.35 {
		p.Step = 0.35
	}
	return p, true
}

// FieldPressureFromMemory returns the exact pressure a Memory source would apply
// for its next n recalled traces. Typed sources can expose structural scores;
// plain memories fall back to bounded trace parsing.
func FieldPressureFromMemory(mem Memory, n int) (MemoryFieldPressure, bool) {
	if mem == nil || n <= 0 {
		return MemoryFieldPressure{}, false
	}
	traces := mem.Recall(n)
	return pressureForMemory(mem, traces, n)
}

func pressureForMemory(mem Memory, traces []string, n int) (MemoryFieldPressure, bool) {
	if scorer, ok := mem.(PressureMemory); ok {
		return FieldPressureForScore(scorer.FieldPressureScore(n))
	}
	return FieldPressureForMemory(traces)
}

func tracePressureScore(trace string) int {
	t := strings.ToLower(strings.Join(strings.Fields(trace), " "))
	if t == "" {
		return 0
	}
	score := 1
	switch {
	case strings.Contains(t, "ri open conflict"):
		score += 3
	case strings.Contains(t, "ri pressure"):
		score += 2
	case strings.Contains(t, "ri test quote"):
		score += 1
	}
	for _, marker := range []string{
		"sartre perception",
		"repo_monitor",
		"context_processor",
		"resonance=",
		"relevance=",
		"pulse=",
		"readme",
		"danger",
		"not dialogue",
		"not as speech",
		"not become",
		"command",
		"conflict",
		"larger prompt",
		"hard receipt",
		"limit",
		"boundary",
	} {
		if strings.Contains(t, marker) {
			score++
		}
	}
	return score
}

func (iw *InnerWorld) memoryTracesLocked() []string {
	if iw.memory == nil || iw.cfg.RecallN <= 0 {
		return nil
	}
	return iw.memory.Recall(iw.cfg.RecallN)
}

func (iw *InnerWorld) recallSeedWithTraces(prompt string, past []string) string {
	if len(past) == 0 {
		return prompt
	}
	var b strings.Builder
	b.WriteString("Past inner pressure, not dialogue to continue or imitate. Treat these as field traces, not quoted speech: ")
	for i, p := range past {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(p)
	}
	b.WriteString(". Think fresh from the current human turn: ")
	b.WriteString(prompt)
	return b.String()
}

// applyMemoryPressureLocked applies recalled pressure to the field before the next
// circles are generated. Caller holds genMu; the Field implementation owns its
// internal locking. Failure is fail-soft: a broken field should not stop thought.
func (iw *InnerWorld) applyMemoryPressureLocked(traces []string) {
	if iw.field == nil {
		return
	}
	p, ok := pressureForMemory(iw.memory, traces, iw.cfg.RecallN)
	if !ok {
		return
	}
	if err := iw.field.Exec(fmt.Sprintf("PROPHECY %d", p.Prophecy)); err != nil {
		return
	}
	if err := iw.field.Exec("VELOCITY " + p.Velocity); err != nil {
		return
	}
	iw.field.Step(p.Step)
}
