package innerworld

import (
	"context"
	"sync"
	"time"
)

// Autonomous breath triggers.
const (
	trigDrift   = iota // the field wandered: respond with a dream
	trigSilence        // no one has spoken for a while: the idle dreamer
	nTrig
)

// Breath tunes the autonomous inner life — when, and how often, the organism
// dreams unprompted.
type Breath struct {
	Tick      time.Duration        // how often the inner world is evaluated
	Silence   time.Duration        // idle time before the silence dreamer fires
	DriftDebt float32              // field debt above which the drift dreamer fires
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
// the organism dreaming between turns. Only one inner monologue runs at a time —
// the body has a single voice — so Think and the autonomous dream are serialized.
type InnerWorld struct {
	fast  Body
	field Field
	div   Divergence
	cfg   Config
	br    Breath

	genMu sync.Mutex // one inner voice at a time: serializes Overthink (body access)

	mu         sync.Mutex // guards circles, lastActive, lastFire, onDream
	circles    []Circle
	lastActive time.Time
	lastFire   [nTrig]time.Time
	onDream    func([]Circle)
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

// SetOnDream registers the observer for autonomous dreams. Inner only — the
// circles handed to it are a copy and never reach the user.
func (iw *InnerWorld) SetOnDream(f func([]Circle)) {
	iw.mu.Lock()
	iw.onDream = f
	iw.mu.Unlock()
}

func cloneCircles(c []Circle) []Circle {
	if c == nil {
		return nil
	}
	out := make([]Circle, len(c))
	copy(out, c)
	return out
}

// think runs one overthinking pass under the single-voice lock and stores a copy.
func (iw *InnerWorld) think(prompt string) []Circle {
	iw.genMu.Lock()
	circles := Overthink(prompt, iw.fast, iw.field, iw.div, iw.cfg)
	iw.genMu.Unlock()

	iw.mu.Lock()
	iw.circles = cloneCircles(circles)
	iw.mu.Unlock()
	return cloneCircles(circles)
}

// Think runs the overthinking for a human turn asynchronously: it returns at once
// with a channel that delivers a copy of the three circles when ready, so the
// answer path is never blocked by the inner monologue.
func (iw *InnerWorld) Think(prompt string) <-chan []Circle {
	out := make(chan []Circle, 1)
	iw.mu.Lock()
	iw.lastActive = time.Now()
	iw.mu.Unlock()
	go func() {
		out <- iw.think(prompt)
		close(out)
	}()
	return out
}

// due reports which autonomous trigger, if any, should fire at time now. The
// most-overdue eligible trigger wins, so a steadily high-debt field can never
// starve the silence dreamer.
func (iw *InnerWorld) due(now time.Time) (int, bool) {
	iw.mu.Lock()
	defer iw.mu.Unlock()

	best, bestOver := -1, time.Duration(0)
	consider := func(trig int, eligible bool) {
		if !eligible {
			return
		}
		over := now.Sub(iw.lastFire[trig]) - iw.br.Cooldown[trig]
		if over < 0 {
			return // still on cooldown
		}
		if best < 0 || over > bestOver {
			best, bestOver = trig, over
		}
	}
	consider(trigDrift, iw.field != nil && iw.field.Debt() >= iw.br.DriftDebt)
	consider(trigSilence, now.Sub(iw.lastActive) >= iw.br.Silence)

	if best < 0 {
		return 0, false
	}
	return best, true
}

// dream runs the overthinking unprompted, on the organism's own last thought —
// the ripple continues with no one speaking. The cooldown starts when the dream
// completes, not when it begins. Inner only; OnDream receives a copy.
func (iw *InnerWorld) dream(trigger int) []Circle {
	iw.mu.Lock()
	seed := "I keep thinking, with no one here."
	if n := len(iw.circles); n > 0 {
		seed = iw.circles[n-1].Text
	}
	iw.mu.Unlock()

	iw.genMu.Lock()
	circles := Overthink(seed, iw.fast, iw.field, iw.div, iw.cfg)
	iw.genMu.Unlock()

	iw.mu.Lock()
	iw.circles = cloneCircles(circles)
	iw.lastFire[trigger] = time.Now() // cooldown measured from completion
	onDream := iw.onDream
	out := cloneCircles(circles)
	iw.mu.Unlock()

	if onDream != nil {
		onDream(out)
	}
	return out
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
				iw.dream(trigger)
			}
		}
	}
}
