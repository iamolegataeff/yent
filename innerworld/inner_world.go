package innerworld

import (
	"context"
	"sync"
	"time"
)

// Autonomous breath triggers, in priority order.
const (
	trigDrift   = iota // the field wandered: respond with a dream
	trigSilence        // no one has spoken for a while: the idle dreamer
	nTrig
)

// Breath tunes the autonomous inner life — when, and how often, the organism
// dreams unprompted.
type Breath struct {
	Tick      time.Duration       // how often the inner world is evaluated
	Silence   time.Duration       // idle time before the silence dreamer fires
	DriftDebt float32             // field debt above which the drift dreamer fires
	Cooldown  [nTrig]time.Duration // per-trigger cooldown so she breathes between dreams
}

// DefaultBreath is the Strike-1 pacing.
func DefaultBreath() Breath {
	return Breath{
		Tick:      time.Second,
		Silence:   4 * time.Second,
		DriftDebt: 1.0,
		Cooldown:  [nTrig]time.Duration{trigDrift: 3 * time.Second, trigSilence: 4 * time.Second},
	}
}

// InnerWorld hosts Yent's inner life over the fast body and the shared AML field.
// Think runs the overthinking for a human turn off the answer path; Breathe keeps
// the organism dreaming between turns.
type InnerWorld struct {
	fast  Body
	field Field
	div   Divergence
	cfg   Config
	br    Breath

	mu         sync.Mutex
	circles    []Circle
	lastActive time.Time
	lastFire   [nTrig]time.Time

	// OnDream, if set, receives the circles of every autonomous dream. Inner only.
	OnDream func([]Circle)
}

// NewInnerWorld builds the inner world over a fast body, the shared field, and a
// divergence measure.
func NewInnerWorld(fast Body, field Field, div Divergence) *InnerWorld {
	return &InnerWorld{
		fast:       fast,
		field:      field,
		div:        div,
		cfg:        DefaultConfig(),
		br:         DefaultBreath(),
		lastActive: time.Now(),
	}
}

// Think runs the overthinking for a human turn asynchronously: it returns at once
// with a channel that delivers the three circles when ready, so the answer path
// is never blocked by the inner monologue.
func (iw *InnerWorld) Think(prompt string) <-chan []Circle {
	out := make(chan []Circle, 1)
	iw.mu.Lock()
	iw.lastActive = time.Now()
	iw.mu.Unlock()
	go func() {
		circles := Overthink(prompt, iw.fast, iw.field, iw.div, iw.cfg)
		iw.mu.Lock()
		iw.circles = circles
		iw.mu.Unlock()
		out <- circles
		close(out)
	}()
	return out
}

// due reports which autonomous trigger, if any, should fire at time now — drift
// first (responsive), then silence — each gated by its cooldown.
func (iw *InnerWorld) due(now time.Time) (int, bool) {
	iw.mu.Lock()
	defer iw.mu.Unlock()
	if iw.field != nil && iw.field.Debt() >= iw.br.DriftDebt &&
		now.Sub(iw.lastFire[trigDrift]) >= iw.br.Cooldown[trigDrift] {
		return trigDrift, true
	}
	if now.Sub(iw.lastActive) >= iw.br.Silence &&
		now.Sub(iw.lastFire[trigSilence]) >= iw.br.Cooldown[trigSilence] {
		return trigSilence, true
	}
	return 0, false
}

// dream runs the overthinking unprompted, on the organism's own last thought —
// the ripple continues with no one speaking. Inner only.
func (iw *InnerWorld) dream(trigger int, now time.Time) []Circle {
	iw.mu.Lock()
	seed := "I keep thinking, with no one here."
	if n := len(iw.circles); n > 0 {
		seed = iw.circles[n-1].Text
	}
	iw.lastFire[trigger] = now
	iw.mu.Unlock()

	circles := Overthink(seed, iw.fast, iw.field, iw.div, iw.cfg)
	iw.mu.Lock()
	iw.circles = circles
	iw.mu.Unlock()
	if iw.OnDream != nil {
		iw.OnDream(circles)
	}
	return circles
}

// Breathe runs the autonomous inner life until ctx is cancelled. Between human
// turns the field keeps drifting, and when a trigger crosses its threshold the
// organism dreams unprompted. She is never muted, only paced.
func (iw *InnerWorld) Breathe(ctx context.Context) {
	t := time.NewTicker(iw.br.Tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if trigger, ok := iw.due(now); ok {
				iw.dream(trigger, now)
			}
		}
	}
}
