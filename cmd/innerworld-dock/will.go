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

// willSpawner runs a self-reading utility and returns its SARTRE event line (JSON), or an error.
// The real implementation execs the utility binary under a timeout; tests use a fake.
type willSpawner interface {
	Spawn(ctx context.Context, util string) ([]byte, error)
}

// willSink receives a utility's event line — the real sink appends it to YENT_SARTRE_EVENTS so
// the sartreSense reflex perceives it on the next ripple.
type willSink interface {
	Emit(line []byte) error
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
// pull points to, emits its perception, and discharges the tide. Returns the utility reached for
// this tick, or "" if the tide stayed under the crest. A physics/exec error is returned without a
// reach; a spawn/emit error is returned after the (already-applied) discharge — the will reached
// even if the hand came back empty.
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
	util := willUtilPressure
	if w.field.GetVarFloat("pull_origin") > w.field.GetVarFloat("pull_pressure") {
		util = willUtilOrigin
	}
	// The reach both spends the tide (discharge) AND opens a bounded refractory of `refractory`
	// ticks: even a high-strain confluence that alone re-crosses threshold next tick cannot reach
	// again until the refractory elapses. The discharge is the physics; the cooldown is the floor.
	w.cooldown = w.refractory
	_ = w.field.Exec("will_gaze = 0")
	line, err := w.spawner.Spawn(ctx, util)
	if err != nil {
		return util, fmt.Errorf("will reach %s: %w", util, err)
	}
	if len(line) > 0 {
		if err := w.sink.Emit(line); err != nil {
			return util, fmt.Errorf("will emit %s: %w", util, err)
		}
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
