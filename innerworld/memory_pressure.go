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
	Prophecy int
	Velocity string
	Step     float32
}

// FieldPressureForMemory turns selected memory traces into one bounded AML pulse.
// RI/limpha traces should not become a larger prompt wall; this gives them a
// second route into the organism as field pressure. Empty traces produce no pulse.
func FieldPressureForMemory(traces []string) (MemoryFieldPressure, bool) {
	if len(traces) == 0 {
		return MemoryFieldPressure{}, false
	}
	score := 0
	for _, trace := range traces {
		score += tracePressureScore(trace)
	}
	if score <= 0 {
		return MemoryFieldPressure{}, false
	}
	if score > 5 {
		score = 5
	}
	p := MemoryFieldPressure{
		Prophecy: 1 + score, // 2..6; memory never claims the whole horizon alone.
		Velocity: "WALK",
		Step:     0.15 + 0.04*float32(score),
	}
	if p.Step > 0.35 {
		p.Step = 0.35
	}
	return p, true
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
	p, ok := FieldPressureForMemory(traces)
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
