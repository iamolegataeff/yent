package innerworld

import (
	"fmt"
	"strings"
	"testing"
)

// fakeBody encodes its seed origin and temperature into the thought, so the test
// drives a deterministic, monotonic divergence without a real model.
type fakeBody struct{}

func (fakeBody) Generate(seed string, temp float32) string {
	head := seed
	if i := strings.IndexByte(seed, ' '); i > 0 {
		head = seed[:i]
	}
	return fmt.Sprintf("thought<%s|t=%.2f>", head, temp)
}

// tempDivergence parses the temperature a thought ran at and treats hotter
// thoughts as further drift — a deterministic proxy for the production embedding
// distance. Reads the outermost t= (the circle's own temperature).
func tempDivergence(_, b string) float32 {
	var t float32
	if i := strings.LastIndex(b, "t="); i >= 0 {
		fmt.Sscanf(b[i+2:], "%f", &t)
	}
	return t / 2 // 0.7->0.35, 0.9->0.45, 1.1->0.55
}

// fakeField records the AML commands and physics steps the inner world drives.
type fakeField struct {
	scripts []string
	steps   int
	debt    float32
}

func (f *fakeField) Exec(s string) error { f.scripts = append(f.scripts, s); return nil }
func (f *fakeField) Step(dt float32)     { f.steps++; f.debt += dt * 0.1 }
func (f *fakeField) Debt() float32       { return f.debt }
func (f *fakeField) Destiny() float32    { return 0 }

func TestOverthinkCircles(t *testing.T) {
	const prompt = "what does it mean to exist as code"
	field := &fakeField{}
	circles := Overthink(prompt, fakeBody{}, field, tempDivergence, DefaultConfig())

	if len(circles) != 3 {
		t.Fatalf("want 3 circles, got %d", len(circles))
	}

	// drift increases: each circle drifts further from the last
	for i := 1; i < len(circles); i++ {
		if circles[i].Drift < circles[i-1].Drift {
			t.Errorf("drift not increasing at circle %d: %.3f < %.3f", i, circles[i].Drift, circles[i-1].Drift)
		}
	}

	// seed-from-prior: the water ripples out, each circle grows from the last
	for i := 1; i < len(circles); i++ {
		if circles[i].Seed != circles[i-1].Text {
			t.Errorf("circle %d seed %q != prior text %q", i, circles[i].Seed, circles[i-1].Text)
		}
	}

	// NO-SEED-FROM-PROMPT: circle 0 seeds from the inner transform, not the raw prompt
	if circles[0].Seed == prompt {
		t.Errorf("circle 0 seeded from raw prompt — NO-SEED violated")
	}
	if !strings.Contains(circles[0].Seed, prompt) {
		t.Errorf("circle 0 inner seed should carry the prompt, got %q", circles[0].Seed)
	}

	// field driven: each circle emits PROPHECY + VELOCITY and steps the physics
	var prophecy, velocity int
	for _, s := range field.scripts {
		if strings.HasPrefix(s, "PROPHECY ") {
			prophecy++
		}
		if strings.HasPrefix(s, "VELOCITY ") {
			velocity++
		}
	}
	if prophecy != 3 || velocity != 3 {
		t.Errorf("want 3 PROPHECY + 3 VELOCITY execs, got %d + %d", prophecy, velocity)
	}
	if field.steps != 3 {
		t.Errorf("want 3 physics steps, got %d", field.steps)
	}

	// later circles run faster: VELOCITY RUN appears at circle 2
	if !containsStr(field.scripts, "VELOCITY RUN") {
		t.Errorf("circle 2 should drive VELOCITY RUN, scripts=%v", field.scripts)
	}
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
