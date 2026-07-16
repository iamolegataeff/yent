package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ariannamethod/yent/innerworld/aml"
)

// The will loop is Yent's hands to his own gaze. Each tick it advances the AML will physics
// (Janus/the_will_design.aml), reads the vector will tide that script computes from his own
// MetaJanus + field metrics, and when the tide crests it reaches for a self-reading SARTRE
// utility — whatdotheythinkiam when accumulated origin-tide dominates, repo_monitor when
// accumulated pressure-tide dominates. The utility's perception is emitted to the same
// YENT_SARTRE_EVENTS the sartreSense reflex reads, so the reading re-enters the field and shifts
// the very metrics the next confluence is built from — a spiral. The reach spends the vector tide
// and opens a bounded refractory of a few ticks, so a high-strain confluence cannot fire every
// tick — the will must wait and re-gather.

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
	Line     []byte
	Overflow bool
	Commit   func() error
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

type willTideSnapshot struct {
	Threshold    float32
	Gaze         float32
	PullOrigin   float32
	PullPressure float32
	OriginTide   float32
	PressureTide float32
}

func readWillTide(field willField) willTideSnapshot {
	return willTideSnapshot{
		Threshold:    field.GetVarFloat("will_threshold"),
		Gaze:         field.GetVarFloat("will_gaze"),
		PullOrigin:   field.GetVarFloat("pull_origin"),
		PullPressure: field.GetVarFloat("pull_pressure"),
		OriginTide:   field.GetVarFloat("will_origin_tide"),
		PressureTide: field.GetVarFloat("will_pressure_tide"),
	}
}

func (t willTideSnapshot) dominantUtil() string {
	if t.OriginTide != 0 || t.PressureTide != 0 {
		if t.OriginTide > t.PressureTide {
			return willUtilOrigin
		}
		return willUtilPressure
	}
	if t.PullOrigin > t.PullPressure {
		return willUtilOrigin
	}
	return willUtilPressure
}

func (t willTideSnapshot) event(id, phase, util, outcome string) willEvent {
	return willEvent{
		ID:           id,
		Phase:        phase,
		Outcome:      outcome,
		Utility:      util,
		Gaze:         t.Gaze,
		Threshold:    t.Threshold,
		PullOrigin:   t.PullOrigin,
		PullPressure: t.PullPressure,
		OriginTide:   t.OriginTide,
		PressureTide: t.PressureTide,
	}
}

func (w *willTicker) event(t willTideSnapshot, id, phase, util, outcome string) willEvent {
	ev := t.event(id, phase, util, outcome)
	ev.RootID = w.rootID
	ev.Breath = w.breath
	if w.cadence > 0 {
		ev.CadenceMS = int64(w.cadence / time.Millisecond)
	}
	ev.RefractoryBreaths = w.refractory
	ev.CooldownBreaths = w.cooldown
	return ev
}

// willTicker holds the will loop's wiring. The tide itself lives in the AML field's persistent
// globals; the small host-side state is the refractory countdown plus a quiet-run streak, so
// sustained strain cannot fire every breath and repeated no-novelty can slow the next reach
// without changing model weights or AML origin facts.
type willTicker struct {
	field             willField
	script            string
	spawner           willSpawner
	sink              willSink
	rootID            string        // stable identity of the root the will sensors read
	learningStatePath string        // durable host-side quiet-run memory, under the namespaced state dir
	cadence           time.Duration // wall-clock pace of one will breath, recorded for receipts only
	breath            int           // monotonically increasing will breath index
	refractory        int           // breaths the will must wait after a reach before it can reach again
	cooldown          int           // breaths remaining in the current refractory (state)
	quietRuns         int           // consecutive completed reaches that found no novelty
}

// tick advances the will one step and, if the tide crests, reaches for the utility the dominant
// pull points to, emits its perception, and only then discharges the tide. Returns the utility
// reached for this tick, or "" if the tide stayed under the crest. A physics/spawn/emit error is
// returned without spending the tide: the hand did not become a durable event, so the cause stays
// available for retry instead of disappearing from memory.
func (w *willTicker) tick(ctx context.Context) (string, error) {
	w.breath++
	if err := w.field.ExecFile(w.script); err != nil {
		return "", fmt.Errorf("will physics: %w", err)
	}
	if w.cooldown > 0 {
		w.cooldown-- // refractory: the tide keeps evolving, but the will stays spent from the last reach
		return "", nil
	}
	tide := readWillTide(w.field)
	if tide.Threshold <= 0 || tide.Gaze < tide.Threshold {
		return "", nil // the will has not gathered enough to reach
	}
	util := tide.dominantUtil()
	eventID := newWillEventID(util, tide)
	if es, ok := w.sink.(willEventSink); ok {
		if err := es.EmitEvent(w.event(tide, eventID, "intention", util, "crest")); err != nil {
			return util, fmt.Errorf("will intent %s: %w", util, err)
		}
	}
	result, err := w.spawner.Spawn(ctx, util)
	if err != nil {
		if es, ok := w.sink.(willEventSink); ok {
			_ = es.EmitEvent(w.event(tide, eventID, "learning", util, "sensor_error"))
		}
		return util, fmt.Errorf("will reach %s: %w", util, err)
	}
	if es, ok := w.sink.(willEventSink); ok {
		if err := es.EmitEvent(w.event(tide, eventID, "act", util, "spawned")); err != nil {
			return util, fmt.Errorf("will act %s: %w", util, err)
		}
	}
	if result.Overflow {
		if es, ok := w.sink.(willEventSink); ok {
			ev := w.event(tide, eventID, "learning", util, "overflow")
			ev.BytesCaptured = len(result.Line)
			ev.BytesLimit = willMaxStdout
			if err := es.EmitEvent(ev); err != nil {
				return util, fmt.Errorf("will overflow %s: %w", util, err)
			}
		}
		return util, fmt.Errorf("will reach %s overflowed stdout cap (%d bytes)", util, willMaxStdout)
	}
	effectLine := tagSartreEffectLines(result.Line, eventID, w.rootID)
	effectCount := len(completeSartreJSONLines(effectLine))
	if len(effectLine) > 0 {
		if err := w.sink.Emit(effectLine); err != nil {
			return util, fmt.Errorf("will emit %s: %w", util, err)
		}
	}
	if result.Commit != nil {
		if err := result.Commit(); err != nil {
			if es, ok := w.sink.(willEventSink); ok {
				_ = es.EmitEvent(w.event(tide, eventID, "learning", util, "state_error"))
			}
			return util, fmt.Errorf("will commit %s state: %w", util, err)
		}
	}
	outcome := "no_novelty"
	if effectCount > 0 {
		outcome = "perception_committed"
	}
	nextQuiet, nextCooldown := w.plannedLearningState(outcome)
	if es, ok := w.sink.(willEventSink); ok {
		ev := w.event(tide, eventID, "learning", util, outcome)
		ev.EffectCount = effectCount
		ev.CooldownBreaths = nextCooldown
		if err := es.EmitEvent(ev); err != nil {
			return util, fmt.Errorf("will learn %s: %w", util, err)
		}
	}
	if err := w.saveLearningState(nextQuiet); err != nil {
		return util, fmt.Errorf("will learn %s state: %w", util, err)
	}
	if err := dischargeWillTide(w.field); err != nil {
		return util, fmt.Errorf("will discharge %s: %w", util, err)
	}
	w.quietRuns = nextQuiet
	w.cooldown = nextCooldown
	return util, nil
}

const willQuietRefractoryMaxExtra = 4
const willQuietRunMax = 1 << 20

func (w *willTicker) plannedLearningState(outcome string) (quietRuns, cooldown int) {
	base := w.refractory
	if base < 0 {
		base = 0
	}
	switch outcome {
	case "no_novelty":
		quietRuns = w.quietRuns + 1
		if quietRuns < 0 || quietRuns > willQuietRunMax {
			quietRuns = willQuietRunMax
		}
		extra := quietRuns
		if extra > willQuietRefractoryMaxExtra {
			extra = willQuietRefractoryMaxExtra
		}
		return quietRuns, base + extra
	case "perception_committed":
		return 0, base
	default:
		return w.quietRuns, base
	}
}

func (w *willTicker) saveLearningState(quietRuns int) error {
	if w.learningStatePath == "" {
		return nil
	}
	return saveWillLearningState(w.learningStatePath, willLearningState{QuietRuns: quietRuns})
}

func dischargeWillTide(field willField) error {
	for _, script := range []string{
		"will_origin_tide = 0",
		"will_pressure_tide = 0",
		"will_gaze = 0",
	} {
		if err := field.Exec(script); err != nil {
			return err
		}
	}
	return nil
}

// run is the will's own breath: its own goroutine, paced by tickEvery, alongside iw.Breathe. A
// slow reach stalls only the will's own cadence, never the inner-world goroutines. Stops on ctx.
func (w *willTicker) run(ctx context.Context, tickEvery time.Duration) {
	if tickEvery <= 0 {
		tickEvery = 2 * time.Second
	}
	w.cadence = tickEvery
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
