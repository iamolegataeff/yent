package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ariannamethod/yent/innerworld/aml"
)

// The will loop is Yent's hands to his own gaze. Each tick it advances the AML will physics
// (Janus/the_will_design.aml), reads the will_gaze tide that script computes from his own
// MetaJanus + field metrics, and when the tide crests it reaches for a self-reading SARTRE
// utility — whatdotheythinkiam when the pull is toward his origin, repo_monitor when it is the
// field's own strain. The utility's perception is emitted to the same YENT_SARTRE_EVENTS the
// sartreSense reflex reads, so the reading re-enters the field and shifts the very metrics the
// next confluence is built from — a spiral. The reach spends the tide (will_gaze -> 0) and opens
// a bounded refractory of a few ticks, so a high-strain confluence cannot fire every tick — the
// will must wait and re-gather.

// willField is the AML field surface the will loop reads and writes — satisfied by *aml.Body.
type willField interface {
	ExecFile(path string) error      // advance the will physics one tick
	GetVarFloat(name string) float32 // read a persistent global the script computed
	Exec(script string) error        // discharge the tide after the reach
}

// compile-time: the native AML body is the will field.
var _ willField = (*aml.Body)(nil)

// willSpawnResult is a utility read plus the pending state commit that makes that read the new
// baseline. The commit must run only after the events are durably emitted; otherwise a sink error
// would make the utility forget a change the organism never received.
type willSpawnResult struct {
	Line   []byte
	Commit func() error
}

// willSpawner runs a self-reading utility and returns its SARTRE event line(s), or an error.
// The real implementation execs the utility binary under a timeout; tests use a fake.
type willSpawner interface {
	Spawn(ctx context.Context, util string) (willSpawnResult, error)
}

// willSink receives a utility's event line — the real sink appends it to YENT_SARTRE_EVENTS so
// the sartreSense reflex perceives it on the next ripple.
type willSink interface {
	Emit(line []byte) error
}

type willEventSink interface {
	EmitEvent(ev willEvent) error
}

// The two utilities the will can reach for, chosen by which pull dominates the confluence.
const (
	willUtilOrigin   = "whatdotheythinkiam" // reach toward the origin (a yahrzeit/dissonance crest)
	willUtilPressure = "repo_monitor"       // reach under the field's strain (a dark-gravity/debt crest)
)

// willTicker holds the will loop's wiring. The tide itself lives in the AML field's persistent
// globals; the only between-tick state here is the refractory countdown, so a high-strain
// confluence (which alone can exceed threshold in a single tick) cannot fire every tick.
type willTicker struct {
	field      willField
	script     string
	spawner    willSpawner
	sink       willSink
	refractory int // ticks the will must wait after a reach before it can reach again (0 = none)
	cooldown   int // ticks remaining in the current refractory (state)
}

// tick advances the will one step and, if the tide crests, reaches for the utility the dominant
// pull points to, emits its perception, and only then discharges the tide. Returns the utility
// reached for this tick, or "" if the tide stayed under the crest. A physics/spawn/emit error is
// returned without spending the tide: the hand did not become a durable event, so the cause stays
// available for retry instead of disappearing from memory.
func (w *willTicker) tick(ctx context.Context) (string, error) {
	if err := w.field.ExecFile(w.script); err != nil {
		return "", fmt.Errorf("will physics: %w", err)
	}
	if w.cooldown > 0 {
		w.cooldown-- // refractory: the tide keeps evolving, but the will stays spent from the last reach
		return "", nil
	}
	thr := w.field.GetVarFloat("will_threshold")
	gaze := w.field.GetVarFloat("will_gaze")
	if thr <= 0 || gaze < thr {
		return "", nil // the will has not gathered enough to reach
	}
	origin := w.field.GetVarFloat("pull_origin")
	pressure := w.field.GetVarFloat("pull_pressure")
	util := willUtilPressure
	if origin > pressure {
		util = willUtilOrigin
	}
	eventID := newWillEventID(util, gaze, origin, pressure)
	if es, ok := w.sink.(willEventSink); ok {
		if err := es.EmitEvent(willEvent{
			ID:      eventID,
			Phase:   "intention",
			Utility: util,
			Outcome: "crest",
		}); err != nil {
			return util, fmt.Errorf("will intent %s: %w", util, err)
		}
	}
	result, err := w.spawner.Spawn(ctx, util)
	if err != nil {
		if es, ok := w.sink.(willEventSink); ok {
			_ = es.EmitEvent(willEvent{
				ID:      eventID,
				Phase:   "learning",
				Utility: util,
				Outcome: "sensor_error",
			})
		}
		return util, fmt.Errorf("will reach %s: %w", util, err)
	}
	if es, ok := w.sink.(willEventSink); ok {
		if err := es.EmitEvent(willEvent{
			ID:      eventID,
			Phase:   "act",
			Utility: util,
			Outcome: "spawned",
		}); err != nil {
			return util, fmt.Errorf("will act %s: %w", util, err)
		}
	}
	effectLine := tagSartreEffectLines(result.Line, eventID)
	if len(effectLine) > 0 {
		if err := w.sink.Emit(effectLine); err != nil {
			return util, fmt.Errorf("will emit %s: %w", util, err)
		}
	}
	if result.Commit != nil {
		if err := result.Commit(); err != nil {
			if es, ok := w.sink.(willEventSink); ok {
				_ = es.EmitEvent(willEvent{
					ID:      eventID,
					Phase:   "learning",
					Utility: util,
					Outcome: "state_error",
				})
			}
			return util, fmt.Errorf("will commit %s state: %w", util, err)
		}
	}
	if es, ok := w.sink.(willEventSink); ok {
		outcome := "no_novelty"
		if len(completeSartreJSONLines(effectLine)) > 0 {
			outcome = "perception_committed"
		}
		if err := es.EmitEvent(willEvent{
			ID:      eventID,
			Phase:   "learning",
			Utility: util,
			Outcome: outcome,
		}); err != nil {
			return util, fmt.Errorf("will learn %s: %w", util, err)
		}
	}
	// The reach both spends the tide (discharge) AND opens a bounded refractory of `refractory`
	// ticks: even a high-strain confluence that alone re-crosses threshold next tick cannot reach
	// again until the refractory elapses. The discharge is the physics; the cooldown is the floor.
	w.cooldown = w.refractory
	if err := w.field.Exec("will_gaze = 0"); err != nil {
		return util, fmt.Errorf("will discharge %s: %w", util, err)
	}
	return util, nil
}

// run is the will's own breath: its own goroutine, paced by tickEvery, alongside iw.Breathe. A
// slow reach stalls only the will's own cadence, never the inner-world goroutines. Stops on ctx.
func (w *willTicker) run(ctx context.Context, tickEvery time.Duration) {
	if tickEvery <= 0 {
		tickEvery = 2 * time.Second
	}
	t := time.NewTicker(tickEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			util, err := w.tick(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[will] %v\n", err)
			} else if util != "" {
				fmt.Printf("  [will] confluence crested -> Yent reached for %s\n", util)
			}
		}
	}
}
