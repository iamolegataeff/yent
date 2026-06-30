package innerworld

import "context"

// Consolidation is one stage of the sleep grind — the hook Level B plugs into. Б1
// (cooc), Б2 (weights+spore), Б3 (scar/velocity), Б4 (emotion→sea of memory) each
// implement it; this skeleton only sequences them under the single inner voice.
// Inner only — nothing here reaches the user.
type Consolidation interface {
	// Consolidate runs one grind stage. It is called under genMu (the single voice),
	// so fast/deep generation never overlaps a stage. Respect ctx for cancellation.
	Consolidate(ctx context.Context) error
	// Name labels the stage in observers, logs, and tests.
	Name() string
}

// SleepTrigger reports whether the field has reached critical mass — the point where
// the organism must sleep and consolidate. Modelled on arianna.c: high coherence
// drives the field into autumn (the harvest), so production wires
// coherence→autumn + a debt threshold here. nil = the organism never sleeps.
type SleepTrigger func(field Field) bool

// SetSleepTrigger wires the critical-mass test. Set before Breathe starts.
func (iw *InnerWorld) SetSleepTrigger(t SleepTrigger) {
	iw.mu.Lock()
	iw.sleepTrigger = t
	iw.mu.Unlock()
}

// AddConsolidator appends a consolidation stage; stages run in the order added, once
// per sleep. Set before Breathe starts.
func (iw *InnerWorld) AddConsolidator(c Consolidation) {
	iw.mu.Lock()
	iw.consolidators = append(iw.consolidators, c)
	iw.mu.Unlock()
}

// SetOnSleep registers an inner-only observer, notified with each stage's Name as the
// grind runs. Inner only.
func (iw *InnerWorld) SetOnSleep(f func(stage string)) {
	iw.mu.Lock()
	iw.onSleep = f
	iw.mu.Unlock()
}

// Asleep reports whether the organism is mid-consolidation.
func (iw *InnerWorld) Asleep() bool {
	iw.mu.Lock()
	defer iw.mu.Unlock()
	return iw.asleep
}

// criticalMass reports whether the field reached the sleep threshold. No trigger, no
// field, or a false trigger keeps the organism awake.
func (iw *InnerWorld) criticalMass() bool {
	iw.mu.Lock()
	t := iw.sleepTrigger
	iw.mu.Unlock()
	if t == nil || iw.field == nil {
		return false
	}
	return t(iw.field)
}

// readyToSleep converts the critical-mass level signal into one sleep episode. A
// field can stay in autumn for many ticks; the harvest should run once for that
// episode, then re-arm only after the trigger falls false and rises again.
func (iw *InnerWorld) readyToSleep(critical bool) bool {
	iw.mu.Lock()
	defer iw.mu.Unlock()
	if !critical {
		iw.sleepLatched = false
		return false
	}
	if iw.asleep || iw.sleepLatched {
		return false
	}
	iw.sleepLatched = true
	return true
}

// sleep runs the consolidation grind: every consolidator in order. Each stage takes
// genMu, so no generation overlaps a stage, but genMu is RELEASED between stages —
// a human turn waiting on Think interleaves at a stage boundary instead of waiting
// out the whole grind. That is the asynchronous sleep: the organism consolidates
// without ever monopolising its single voice. ctx cancels the grind at the next
// boundary. A failing stage does not abort the rest. Inner only.
func (iw *InnerWorld) sleep(ctx context.Context) {
	iw.mu.Lock()
	iw.asleep = true
	stages := make([]Consolidation, len(iw.consolidators))
	copy(stages, iw.consolidators)
	onSleep := iw.onSleep
	iw.mu.Unlock()

	// asleep is cleared no matter what — even if a stage panics — so a faulty
	// consolidator can never leave the organism stuck "asleep".
	defer func() {
		iw.mu.Lock()
		iw.asleep = false
		iw.mu.Unlock()
	}()

	for _, c := range stages {
		if ctx.Err() != nil {
			break
		}
		iw.runStage(ctx, c, onSleep)
	}
}

// runStage runs one consolidation stage under genMu. A panicking stage is contained
// and genMu is always released (deferred), so a faulty consolidator can neither
// wedge the single inner voice nor abort the rest of the grind — the same
// fail-soft stance driveField takes on a broken field.
func (iw *InnerWorld) runStage(ctx context.Context, c Consolidation, onSleep func(string)) {
	iw.genMu.Lock()
	defer iw.genMu.Unlock()
	defer func() { _ = recover() }()
	if onSleep != nil {
		onSleep(c.Name())
	}
	_ = c.Consolidate(ctx)
}
