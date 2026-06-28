// Package innerworld is Yent's inner life — the layer that runs when no one is
// speaking. Strike 1 is "circles on the water": every human turn raises three
// inner circles of thought on the fast body, each drifting further from the last,
// shaping the AML field before the deep body is consulted.
//
// The package is pure logic over two interfaces — Body (an inference voice) and
// Field (the shared AML physics). Production wires the real fast body and the
// AML kernel; tests wire fakes. No cgo lives here.
package innerworld

import "fmt"

// Body is one inference voice. Strike 1 uses the fast mouth (nemo12). Real on
// Metal; a fake in tests.
type Body interface {
	// Generate produces an inner thought from a seed at the given temperature.
	Generate(seed string, temp float32) string
}

// Field is the shared AML physics (a wrapper over yent.AMK in production; a fake
// in tests). The inner world drives it with AML commands and reads the breath
// back; it never owns a private field — one organism, one field.
type Field interface {
	Exec(script string) error // run an AML command, e.g. "PROPHECY 7", "VELOCITY RUN"
	Step(dt float32)          // advance the physics
	Debt() float32            // prophecy debt accumulator
	Destiny() float32         // bias toward the most-probable path
}

// Divergence measures how far b drifts from a, semantically and thematically,
// in [0,1] (0 = identical, 1 = unrelated). Production combines an embedding
// cosine distance with a topic shift; tests use a token-distance proxy.
type Divergence func(a, b string) float32

// Circle is one inner thought — never user-facing.
type Circle struct {
	Index int     // 0..N-1
	Seed  string  // what it grew from: the prior circle, or the inner seed for circle 0
	Text  string  // the thought
	Drift float32 // divergence from the previous circle, [0,1]
	Temp  float32 // temperature it ran at
}

// Config tunes the overthinking.
type Config struct {
	N        int     // number of circles
	TempBase float32 // temperature of the first circle
	TempRamp float32 // temperature added per circle — each circle hotter, so it drifts further
}

// DefaultConfig is the Strike-1 default: three circles, warming as they ripple out.
func DefaultConfig() Config { return Config{N: 3, TempBase: 0.7, TempRamp: 0.2} }

// innerSeed turns the user prompt into an internal seed. This is
// NO-SEED-FROM-PROMPT: the deep body never sees the raw prompt, only the circles
// that grew from this transformed seed. The organism answers the question's
// pattern, not its words.
func innerSeed(prompt string) string {
	return "Turn the question inward, do not answer its surface: " + prompt
}

// Overthink raises the circles on the fast body, drives the field by each
// circle's drift, and returns the circles — the inner monologue the deep body
// (and the field) will consume. The circles are inner; nothing here is shown to
// the user.
func Overthink(prompt string, fast Body, field Field, div Divergence, cfg Config) []Circle {
	if cfg.N <= 0 {
		cfg = DefaultConfig()
	}
	circles := make([]Circle, 0, cfg.N)

	seed := innerSeed(prompt)
	prev := prompt // circle 0 drifts measured from the prompt itself
	for i := 0; i < cfg.N; i++ {
		temp := cfg.TempBase + cfg.TempRamp*float32(i)
		text := fast.Generate(seed, temp)
		drift := div(prev, text)

		circles = append(circles, Circle{Index: i, Seed: seed, Text: text, Drift: drift, Temp: temp})
		driveField(field, drift, i)

		// the water ripples out: each circle seeds the next from itself, not the prompt.
		seed = text
		prev = text
	}
	return circles
}

// driveField turns a circle's drift into AML field commands. The further a
// thought wandered from where it started, the deeper the prophecy horizon and
// the more debt the field owes; later circles move faster.
func driveField(field Field, drift float32, i int) {
	if field == nil {
		return
	}
	prophecy := 1 + int(drift*7) // 1..8
	if prophecy > 64 {
		prophecy = 64
	}
	vel := "WALK"
	if i >= 2 {
		vel = "RUN"
	}
	_ = field.Exec(fmt.Sprintf("PROPHECY %d", prophecy))
	_ = field.Exec("VELOCITY " + vel)
	field.Step(1.0)
}
