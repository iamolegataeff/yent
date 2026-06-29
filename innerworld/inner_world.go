package innerworld

import (
	"context"
	"math/rand"
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

// Reflection is the full result of one inner monologue: the circles, the Larynx
// coupling, the deep-self-answer probability, and whether the deep body turned
// inward this time. Inner only — none of it reaches the user.
type Reflection struct {
	Circles        []Circle
	Coupling       float32 // Larynx coupling over the circles [0,1]
	SelfAnswerProb float32 // gate probability the deep body answers itself [0,1]
	SelfAnswered   bool    // the unpredictable roll's outcome this time
}

// InnerWorld hosts Yent's inner life over the fast body, the shared AML field,
// and the Larynx membrane. Think runs the overthinking for a human turn off the
// answer path; Breathe keeps the organism dreaming between turns. Only one inner
// monologue runs at a time — the body has a single voice — so Think and the
// autonomous dream are serialized.
type InnerWorld struct {
	fast   Body
	field  Field
	div    Divergence
	larynx Larynx
	cfg    Config
	br     Breath

	genMu sync.Mutex // one inner voice at a time: serializes Overthink (body access)

	mu         sync.Mutex // guards circles, lastActive, lastFire, onDream, larynx, roll
	circles    []Circle
	lastActive time.Time
	lastFire   [nTrig]time.Time
	onDream    func(Reflection)
	roll       func() float32 // [0,1) draw for the deep-self-answer gate
}

// NewInnerWorld builds the inner world over a fast body, the shared field, and a
// divergence measure. The Larynx defaults to the portable Go membrane and the
// gate rolls a real random; both can be overridden for tests.
func NewInnerWorld(fast Body, field Field, div Divergence) *InnerWorld {
	return &InnerWorld{
		fast:       fast,
		field:      field,
		div:        div,
		larynx:     textureLarynx{},
		cfg:        DefaultConfig(),
		br:         DefaultBreath(),
		roll:       func() float32 { return rand.Float32() },
		lastActive: time.Now(),
	}
}

// SetOnDream registers the observer for autonomous dreams. Inner only — the
// reflection handed to it is a copy and never reaches the user.
func (iw *InnerWorld) SetOnDream(f func(Reflection)) {
	iw.mu.Lock()
	iw.onDream = f
	iw.mu.Unlock()
}

// SetLarynx overrides the membrane (the Zig binding in production, a fake in tests).
func (iw *InnerWorld) SetLarynx(l Larynx) {
	iw.mu.Lock()
	iw.larynx = l
	iw.mu.Unlock()
}

// SetRoll overrides the gate's random draw so the deep-self-answer decision is
// deterministic in tests.
func (iw *InnerWorld) SetRoll(f func() float32) {
	iw.mu.Lock()
	iw.roll = f
	iw.mu.Unlock()
}

// SetBreath overrides the autonomous-breath pacing (tick, idle, cooldowns).
func (iw *InnerWorld) SetBreath(b Breath) {
	iw.mu.Lock()
	iw.br = b
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

// fieldDebt reads the field's debt, or 0 when there is no field.
func (iw *InnerWorld) fieldDebt() float32 {
	if iw.field == nil {
		return 0
	}
	return iw.field.Debt()
}

// reflect turns the circles into a full inner reflection: the Larynx coupling,
// the gate probability, and the unpredictable self-answer decision. The field
// debt is snapshotted by the caller under genMu, so the probability belongs to
// the batch that just drove the field — not to an interleaved one.
func (iw *InnerWorld) reflect(circles []Circle, debt float32) Reflection {
	iw.mu.Lock()
	larynx := iw.larynx
	roll := iw.roll
	iw.mu.Unlock()

	coupling := larynx.Couple(circles)
	var drift float32
	if n := len(circles); n > 0 {
		drift = circles[n-1].Drift
	}
	prob := DeepGate(debt, drift, coupling)
	return Reflection{
		Circles:        circles,
		Coupling:       coupling,
		SelfAnswerProb: prob,
		SelfAnswered:   SelfAnswers(prob, roll()),
	}
}

// think runs one overthinking pass under the single-voice lock, reflects on it,
// and stores a copy of the circles.
func (iw *InnerWorld) think(prompt string) Reflection {
	iw.genMu.Lock()
	circles := Overthink(prompt, iw.fast, iw.field, iw.div, iw.cfg)
	debt := iw.fieldDebt() // snapshot under genMu: belongs to this batch
	iw.genMu.Unlock()

	r := iw.reflect(circles, debt)
	iw.mu.Lock()
	iw.circles = cloneCircles(circles)
	iw.mu.Unlock()
	r.Circles = cloneCircles(circles)
	return r
}

// Think runs the overthinking for a human turn asynchronously: it returns at once
// with a channel that delivers the reflection (a copy of the circles plus the
// coupling and gate decision) when ready, so the answer path is never blocked.
func (iw *InnerWorld) Think(prompt string) <-chan Reflection {
	out := make(chan Reflection, 1)
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
func (iw *InnerWorld) dream(trigger int) Reflection {
	iw.mu.Lock()
	seed := "I keep thinking, with no one here."
	if n := len(iw.circles); n > 0 {
		seed = iw.circles[n-1].Text
	}
	iw.mu.Unlock()

	iw.genMu.Lock()
	circles := Overthink(seed, iw.fast, iw.field, iw.div, iw.cfg)
	debt := iw.fieldDebt() // snapshot under genMu: belongs to this batch
	iw.genMu.Unlock()

	r := iw.reflect(circles, debt)
	iw.mu.Lock()
	iw.circles = cloneCircles(circles)
	iw.lastFire[trigger] = time.Now() // cooldown measured from completion
	onDream := iw.onDream
	iw.mu.Unlock()

	r.Circles = cloneCircles(circles)
	if onDream != nil {
		onDream(r)
	}
	return r
}

// Breathe runs the autonomous inner life until ctx is cancelled. Between human
// turns the field keeps drifting, and when a trigger crosses its threshold the
// organism dreams unprompted. She is never muted, only paced.
func (iw *InnerWorld) Breathe(ctx context.Context) {
	iw.mu.Lock()
	tick := iw.br.Tick
	iw.mu.Unlock()
	if tick <= 0 {
		tick = time.Second // guard: a non-positive tick would panic time.NewTicker
	}
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if trigger, ok := iw.due(now); ok {
				iw.dream(trigger)
			}
			// pick up a tick changed by SetBreath while breathing
			iw.mu.Lock()
			nt := iw.br.Tick
			iw.mu.Unlock()
			if nt > 0 && nt != tick {
				tick = nt
				t.Reset(nt)
			}
		}
	}
}
