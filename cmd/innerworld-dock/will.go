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
	Exec(script string) error        // execute a transactional AML block
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

const (
	willOutcomeNoNovelty           = "no_novelty"
	willOutcomePerceptionCommitted = "perception_committed"
	willOutcomeSensorError         = "sensor_error"
	willOutcomeStateError          = "state_error"
	willOutcomeOverflow            = "overflow"
)

type willTideSnapshot struct {
	Threshold     float32 `json:"threshold"`
	Gaze          float32 `json:"gaze"`
	PullOrigin    float32 `json:"pull_origin"`
	PullPressure  float32 `json:"pull_pressure"`
	PullCuriosity float32 `json:"pull_curiosity"`
	PullCare      float32 `json:"pull_care"`
	PullBoundary  float32 `json:"pull_boundary"`
	OriginTide    float32 `json:"origin_tide"`
	PressureTide  float32 `json:"pressure_tide"`
	CuriosityTide float32 `json:"curiosity_tide"`
	CareTide      float32 `json:"care_tide"`
	BoundaryTide  float32 `json:"boundary_tide"`
}

func readWillTide(field willField) willTideSnapshot {
	return willTideSnapshot{
		Threshold:     field.GetVarFloat("will_threshold"),
		Gaze:          field.GetVarFloat("will_gaze"),
		PullOrigin:    field.GetVarFloat("pull_origin"),
		PullPressure:  field.GetVarFloat("pull_pressure"),
		PullCuriosity: field.GetVarFloat("pull_curiosity"),
		PullCare:      field.GetVarFloat("pull_care"),
		PullBoundary:  field.GetVarFloat("pull_boundary"),
		OriginTide:    field.GetVarFloat("will_origin_tide"),
		PressureTide:  field.GetVarFloat("will_pressure_tide"),
		CuriosityTide: field.GetVarFloat("will_curiosity_tide"),
		CareTide:      field.GetVarFloat("will_care_tide"),
		BoundaryTide:  field.GetVarFloat("will_boundary_tide"),
	}
}

func (t willTideSnapshot) dominantUtil() (string, bool) {
	if t.OriginTide != 0 || t.PressureTide != 0 || t.CuriosityTide != 0 || t.CareTide != 0 || t.BoundaryTide != 0 {
		name, value := "origin", t.OriginTide
		for _, c := range []struct {
			name  string
			value float32
		}{
			{"pressure", t.PressureTide},
			{"curiosity", t.CuriosityTide},
			{"care", t.CareTide},
			{"boundary", t.BoundaryTide},
		} {
			if c.value > value {
				name, value = c.name, c.value
			}
		}
		if value <= 0 {
			return "", false
		}
		switch name {
		case "origin":
			return willUtilOrigin, true
		case "pressure":
			return willUtilPressure, true
		default:
			return "", false
		}
	}
	if t.PullOrigin > t.PullPressure {
		return willUtilOrigin, true
	}
	return willUtilPressure, true
}

func (t willTideSnapshot) event(id, phase, util, outcome string) willEvent {
	return willEvent{
		ID:            id,
		Phase:         phase,
		Outcome:       outcome,
		Utility:       util,
		Gaze:          t.Gaze,
		Threshold:     t.Threshold,
		PullOrigin:    t.PullOrigin,
		PullPressure:  t.PullPressure,
		PullCuriosity: t.PullCuriosity,
		PullCare:      t.PullCare,
		PullBoundary:  t.PullBoundary,
		OriginTide:    t.OriginTide,
		PressureTide:  t.PressureTide,
		CuriosityTide: t.CuriosityTide,
		CareTide:      t.CareTide,
		BoundaryTide:  t.BoundaryTide,
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
	reachStatePath    string        // durable pending reach id/sequence, so retries keep one causal identity
	cadence           time.Duration // wall-clock pace of one will breath, recorded for receipts only
	breath            int           // monotonically increasing will breath index
	refractory        int           // breaths the will must wait after a reach before it can reach again
	cooldown          int           // breaths remaining in the current refractory (state)
	quietRuns         int           // consecutive completed reaches that found no novelty
	nextReachSeq      int64         // next durable reach sequence in the namespaced state dir
	pendingReach      *willPendingReach
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
		nextCooldown := w.cooldown - 1 // refractory: the tide keeps evolving, but the will stays spent from the last reach
		if err := w.saveCooldownState(nextCooldown); err != nil {
			return "", fmt.Errorf("will cooldown state: %w", err)
		}
		w.cooldown = nextCooldown
		return "", nil
	}
	tide := readWillTide(w.field)
	if w.pendingReach == nil && (tide.Threshold <= 0 || tide.Gaze < tide.Threshold) {
		return "", nil // the will has not gathered enough to reach
	}
	util := ""
	if w.pendingReach != nil {
		util = w.pendingReach.Utility
	} else {
		var ok bool
		util, ok = tide.dominantUtil()
		if !ok {
			return "", nil // a future/dormant vector channel crested before an audited hand exists
		}
	}
	reach, err := w.beginReach(util, tide)
	if err != nil {
		return util, fmt.Errorf("will reach state %s: %w", util, err)
	}
	tide = reach.Tide
	util = reach.Utility
	eventID := reach.ID
	outcome := reach.Outcome
	effectCount := reach.EffectCount
	if !reach.ConsequenceCommitted {
		if es, ok := w.sink.(willEventSink); ok {
			if err := es.EmitEvent(w.event(tide, eventID, "intention", util, "crest")); err != nil {
				return util, fmt.Errorf("will intent %s: %w", util, err)
			}
		}
		result, err := w.spawner.Spawn(ctx, util)
		if err != nil {
			if es, ok := w.sink.(willEventSink); ok {
				_ = es.EmitEvent(w.event(tide, eventID, "learning", util, willOutcomeSensorError))
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
				ev := w.event(tide, eventID, "learning", util, willOutcomeOverflow)
				ev.BytesCaptured = len(result.Line)
				ev.BytesLimit = willMaxStdout
				if err := es.EmitEvent(ev); err != nil {
					return util, fmt.Errorf("will overflow %s: %w", util, err)
				}
			}
			return util, fmt.Errorf("will reach %s overflowed stdout cap (%d bytes)", util, willMaxStdout)
		}
		effectLine := tagSartreEffectLines(result.Line, eventID, w.rootID)
		effectCount = len(completeSartreJSONLines(effectLine))
		if len(effectLine) > 0 {
			if err := w.sink.Emit(effectLine); err != nil {
				return util, fmt.Errorf("will emit %s: %w", util, err)
			}
		}
		if result.Commit != nil {
			if err := result.Commit(); err != nil {
				if es, ok := w.sink.(willEventSink); ok {
					_ = es.EmitEvent(w.event(tide, eventID, "learning", util, willOutcomeStateError))
				}
				return util, fmt.Errorf("will commit %s state: %w", util, err)
			}
		}
		outcome = willOutcomeNoNovelty
		if effectCount > 0 {
			outcome = willOutcomePerceptionCommitted
		}
		reach, err = w.markReachConsequenceCommitted(reach, outcome, effectCount)
		if err != nil {
			return util, fmt.Errorf("will commit %s consequence: %w", util, err)
		}
	}
	nextQuiet, nextCooldown := w.plannedLearningState(outcome)
	if err := w.saveLearningState(reach, outcome, effectCount, nextQuiet, nextCooldown); err != nil {
		return util, fmt.Errorf("will learn %s state: %w", util, err)
	}
	if err := dischargeWillTide(w.field); err != nil {
		return util, fmt.Errorf("will discharge %s: %w", util, err)
	}
	// Success learning is a receipt of committed host state plus spent tide, not a promise.
	if es, ok := w.sink.(willEventSink); ok {
		ev := w.event(tide, eventID, "learning", util, outcome)
		ev.EffectCount = effectCount
		ev.CooldownBreaths = nextCooldown
		if err := es.EmitEvent(ev); err != nil {
			return util, fmt.Errorf("will learn %s: %w", util, err)
		}
	}
	if err := w.finishReach(reach.Seq); err != nil {
		return util, fmt.Errorf("will finish %s reach: %w", util, err)
	}
	w.quietRuns = nextQuiet
	w.cooldown = nextCooldown
	return util, nil
}

func (w *willTicker) beginReach(util string, tide willTideSnapshot) (willPendingReach, error) {
	if w.pendingReach != nil {
		return *w.pendingReach, nil
	}
	seq := w.nextReachSeq
	if seq <= 0 {
		seq = 1
	}
	reach := willPendingReach{
		Seq:     seq,
		ID:      newWillEventID(w.rootID, seq, util, tide),
		Utility: util,
		Tide:    tide,
		Breath:  w.breath,
	}
	if err := w.saveReachState(seq, &reach); err != nil {
		return willPendingReach{}, err
	}
	w.nextReachSeq = seq
	w.pendingReach = &reach
	return reach, nil
}

func (w *willTicker) finishReach(seq int64) error {
	next := seq + 1
	if next <= 0 {
		return fmt.Errorf("invalid next reach sequence after %d", seq)
	}
	if err := w.saveReachState(next, nil); err != nil {
		return err
	}
	w.nextReachSeq = next
	w.pendingReach = nil
	return nil
}

func (w *willTicker) markReachConsequenceCommitted(reach willPendingReach, outcome string, effectCount int) (willPendingReach, error) {
	if !validWillCommittedOutcome(outcome, effectCount) {
		return willPendingReach{}, fmt.Errorf("invalid committed outcome %q with %d effects", outcome, effectCount)
	}
	reach.ConsequenceCommitted = true
	reach.Outcome = outcome
	reach.EffectCount = effectCount
	if err := w.saveReachState(reach.Seq, &reach); err != nil {
		return willPendingReach{}, err
	}
	w.nextReachSeq = reach.Seq
	w.pendingReach = &reach
	return reach, nil
}

func (w *willTicker) saveReachState(nextSeq int64, pending *willPendingReach) error {
	if w.reachStatePath == "" {
		return nil
	}
	return saveWillReachState(w.reachStatePath, willReachState{
		NextSeq: nextSeq,
		Pending: pending,
	})
}

const willQuietRefractoryMaxExtra = 4
const willQuietRunMax = 1 << 20

func (w *willTicker) plannedLearningState(outcome string) (quietRuns, cooldown int) {
	base := w.refractory
	if base < 0 {
		base = 0
	}
	switch outcome {
	case willOutcomeNoNovelty:
		quietRuns = w.quietRuns + 1
		if quietRuns < 0 || quietRuns > willQuietRunMax {
			quietRuns = willQuietRunMax
		}
		extra := quietRuns
		if extra > willQuietRefractoryMaxExtra {
			extra = willQuietRefractoryMaxExtra
		}
		return quietRuns, base + extra
	case willOutcomePerceptionCommitted:
		return 0, base
	default:
		return w.quietRuns, base
	}
}

func (w *willTicker) saveLearningState(reach willPendingReach, outcome string, effectCount, quietRuns, cooldown int) error {
	if w.learningStatePath == "" {
		return nil
	}
	tide := reach.Tide
	return saveWillLearningState(w.learningStatePath, willLearningState{
		QuietRuns:       quietRuns,
		LastReachID:     reach.ID,
		LastUtility:     reach.Utility,
		LastOutcome:     outcome,
		LastEffectCount: effectCount,
		LastCooldown:    cooldown,
		CooldownBreaths: cooldown,
		LastBreath:      reach.Breath,
		LastTide:        &tide,
	})
}

func (w *willTicker) saveCooldownState(cooldown int) error {
	if w.learningStatePath == "" {
		return nil
	}
	st, err := loadWillLearningState(w.learningStatePath)
	if err != nil {
		return err
	}
	st.CooldownBreaths = cooldown
	return saveWillLearningState(w.learningStatePath, st)
}

func dischargeWillTide(field willField) error {
	return field.Exec("will_origin_tide = 0\nwill_pressure_tide = 0\nwill_curiosity_tide = 0\nwill_care_tide = 0\nwill_boundary_tide = 0\nwill_gaze = 0")
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
