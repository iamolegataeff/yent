package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeWillField scripts the field's readings so the will logic is deterministic without cgo:
// ExecFile is a no-op counter (the "physics" is whatever the vars map says), GetVarFloat returns
// the scripted value, and Exec records the discharge.
type fakeWillField struct {
	vars           map[string]float32
	execFileErr    error
	execFileN      int
	execErr        error
	execScripts    []string
	discharged     bool
	reaccumulateTo float32 // if >0, ExecFile re-floods will_gaze to this each tick (sustained strain)
}

func (f *fakeWillField) ExecFile(string) error {
	f.execFileN++
	if f.reaccumulateTo > 0 {
		f.vars["will_gaze"] = f.reaccumulateTo // the live field would re-flood the tide each tick
	}
	return f.execFileErr
}
func (f *fakeWillField) GetVarFloat(name string) float32 { return f.vars[name] }
func (f *fakeWillField) Exec(script string) error {
	f.execScripts = append(f.execScripts, script)
	if f.execErr != nil {
		return f.execErr
	}
	for _, line := range strings.Split(script, "\n") {
		switch strings.TrimSpace(line) {
		case "will_origin_tide = 0":
			f.vars["will_origin_tide"] = 0
		case "will_pressure_tide = 0":
			f.vars["will_pressure_tide"] = 0
		case "will_curiosity_tide = 0":
			f.vars["will_curiosity_tide"] = 0
		case "will_care_tide = 0":
			f.vars["will_care_tide"] = 0
		case "will_boundary_tide = 0":
			f.vars["will_boundary_tide"] = 0
		case "will_gaze = 0":
			f.discharged = true
			f.vars["will_gaze"] = 0
		}
	}
	return nil
}

type fakeSpawner struct {
	util      string // the last util asked for
	line      []byte
	overflow  bool
	err       error
	commitErr error
	committed bool
	calls     int
}

func (s *fakeSpawner) Spawn(_ context.Context, util string) (willSpawnResult, error) {
	s.calls++
	s.util = util
	if s.err != nil {
		return willSpawnResult{}, s.err
	}
	return willSpawnResult{
		Line:     s.line,
		Overflow: s.overflow,
		Commit: func() error {
			if s.commitErr != nil {
				return s.commitErr
			}
			s.committed = true
			return nil
		},
	}, nil
}

type fakeSink struct {
	lines [][]byte
	err   error
}

func (s *fakeSink) Emit(line []byte) error {
	if s.err != nil {
		return s.err
	}
	s.lines = append(s.lines, append([]byte(nil), line...))
	return nil
}

func newWill(vars map[string]float32, sp *fakeSpawner, sk *fakeSink) (*willTicker, *fakeWillField) {
	f := &fakeWillField{vars: vars}
	return &willTicker{field: f, script: "x.aml", spawner: sp, sink: sk}, f
}

func TestWillTickReachesOnOriginCrest(t *testing.T) {
	sp, sk := &fakeSpawner{line: []byte(`{"util":"whatdotheythinkiam"}`)}, &fakeSink{}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5,
		"pull_origin": 0.25, "pull_pressure": 0.0, "will_origin_tide": 1.5,
	}, sp, sk)
	util, err := w.tick(context.Background())
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if util != willUtilOrigin {
		t.Errorf("an origin-dominant crest must reach %s, got %s", willUtilOrigin, util)
	}
	if sp.util != willUtilOrigin {
		t.Errorf("the spawner was asked for %s", sp.util)
	}
	if !f.discharged {
		t.Error("the reach must discharge the tide")
	}
	if len(f.execScripts) != 1 ||
		!strings.Contains(f.execScripts[0], "will_origin_tide = 0\nwill_pressure_tide = 0\nwill_curiosity_tide = 0\nwill_care_tide = 0\nwill_boundary_tide = 0\nwill_gaze = 0") {
		t.Fatalf("the tide discharge must be one transactional AML block, got %#v", f.execScripts)
	}
	if f.vars["will_origin_tide"] != 0 || f.vars["will_pressure_tide"] != 0 ||
		f.vars["will_curiosity_tide"] != 0 || f.vars["will_care_tide"] != 0 ||
		f.vars["will_boundary_tide"] != 0 {
		t.Errorf("the reach must discharge the vector tide, origin=%.3f pressure=%.3f curiosity=%.3f care=%.3f boundary=%.3f",
			f.vars["will_origin_tide"], f.vars["will_pressure_tide"], f.vars["will_curiosity_tide"],
			f.vars["will_care_tide"], f.vars["will_boundary_tide"])
	}
	if len(sk.lines) != 1 {
		t.Errorf("the perception must be emitted once, got %d", len(sk.lines))
	}
	if !sp.committed {
		t.Error("utility state must commit after the perception is emitted")
	}
}

func TestWillTickReachesOnPressureCrest(t *testing.T) {
	sp, sk := &fakeSpawner{line: []byte(`{"util":"repo_monitor"}`)}, &fakeSink{}
	w, _ := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.6,
		"pull_origin": 0.0, "pull_pressure": 0.27, "will_pressure_tide": 1.6,
	}, sp, sk)
	util, err := w.tick(context.Background())
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if util != willUtilPressure {
		t.Errorf("a pressure-dominant crest must reach %s, got %s", willUtilPressure, util)
	}
}

func TestWillTickUsesAccumulatedVectorOverCurrentPull(t *testing.T) {
	sp, sk := &fakeSpawner{line: []byte(`{"util":"whatdotheythinkiam"}`)}, &fakeSink{}
	w, _ := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.4,
		"pull_origin": 0.01, "pull_pressure": 0.50,
		"will_origin_tide": 1.3, "will_pressure_tide": 0.1,
	}, sp, sk)
	util, err := w.tick(context.Background())
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if util != willUtilOrigin {
		t.Fatalf("the accumulated origin tide must beat a one-tick pressure spike, got %s", util)
	}
}

func TestWillTickDormantVectorChannelDoesNotImpersonatePressure(t *testing.T) {
	sp, sk := &fakeSpawner{line: []byte(`{"util":"repo_monitor","kind":"added"}`)}, &fakeSink{}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5,
		"will_curiosity_tide": 1.5,
	}, sp, sk)
	util, err := w.tick(context.Background())
	if err != nil {
		t.Fatalf("dormant channel should fail closed without an error: %v", err)
	}
	if util != "" || sp.util != "" || len(sk.lines) != 0 {
		t.Fatalf("an unmapped dormant channel must not spawn or emit as pressure, util=%q spawner=%q lines=%d", util, sp.util, len(sk.lines))
	}
	if f.discharged {
		t.Fatal("an unmapped dormant channel must not spend the tide")
	}
}

func TestWillTickQuietNoReach(t *testing.T) {
	sp, sk := &fakeSpawner{}, &fakeSink{}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 0.2,
	}, sp, sk)
	util, err := w.tick(context.Background())
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if util != "" {
		t.Errorf("under the crest there is no reach, got %s", util)
	}
	if f.discharged {
		t.Error("no discharge without a reach")
	}
	if sp.util != "" {
		t.Error("the spawner must not run under the crest")
	}
	if len(sk.lines) != 0 {
		t.Error("nothing is emitted under the crest")
	}
}

// TestWillTickRefractoryCooldown proves the refractory holds even under sustained high strain,
// where the confluence alone re-crosses threshold every tick (so discharge-to-zero is NOT enough
// on its own). The fake re-floods the tide each ExecFile; only the cooldown spaces the reaches.
func TestWillTickRefractoryCooldown(t *testing.T) {
	sp, sk := &fakeSpawner{line: []byte(`{"util":"x"}`)}, &fakeSink{}
	f := &fakeWillField{
		vars:           map[string]float32{"will_threshold": 1.0, "will_gaze": 2.0, "pull_pressure": 0.5, "pull_origin": 0.0},
		reaccumulateTo: 2.0, // every tick the field re-floods past threshold
	}
	w := &willTicker{field: f, script: "x.aml", spawner: sp, sink: sk, refractory: 3}

	// tick 1: crest -> reach, discharge, refractory armed
	if util, err := w.tick(context.Background()); err != nil || util != willUtilPressure {
		t.Fatalf("the first crest must reach %s (err=%v util=%s)", willUtilPressure, err, util)
	}
	if !f.discharged {
		t.Error("the reach must discharge the tide")
	}
	// ticks 2..4: the field re-floods past threshold every tick, yet the refractory suppresses the reach
	for i := 2; i <= 4; i++ {
		if util, err := w.tick(context.Background()); err != nil || util != "" {
			t.Errorf("tick %d is within the refractory and must not reach (err=%v util=%s)", i, err, util)
		}
	}
	// tick 5: the refractory has elapsed -> the will may reach again
	if util, err := w.tick(context.Background()); err != nil || util == "" {
		t.Errorf("after the refractory the will may reach again (err=%v util=%s)", err, util)
	}
}

func TestWillTickExecFileErrorNoReach(t *testing.T) {
	sp, sk := &fakeSpawner{}, &fakeSink{}
	f := &fakeWillField{
		vars:        map[string]float32{"will_threshold": 1.0, "will_gaze": 5.0},
		execFileErr: errors.New("parse"),
	}
	w := &willTicker{field: f, script: "x.aml", spawner: sp, sink: sk}
	util, err := w.tick(context.Background())
	if err == nil {
		t.Error("a physics error must surface")
	}
	if util != "" {
		t.Errorf("no reach on a physics error, got %s", util)
	}
	if sp.util != "" {
		t.Error("the spawner must not run when the physics failed")
	}
}

func TestWillTickSpawnErrorSurfaces(t *testing.T) {
	sp, sk := &fakeSpawner{err: errors.New("no binary")}, &fakeSink{}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.3, "pull_pressure": 0.0,
	}, sp, sk)
	util, err := w.tick(context.Background())
	if err == nil {
		t.Error("a spawn error must surface")
	}
	if util != willUtilOrigin {
		t.Errorf("the chosen util is still reported on a spawn error, got %s", util)
	}
	if f.discharged {
		t.Error("the tide must not be spent when the reach did not become a durable event")
	}
	if len(sk.lines) != 0 {
		t.Error("nothing is emitted when the spawn failed")
	}
	if sp.committed {
		t.Error("utility state must not commit when spawn failed")
	}
}

func TestWillTickReusesPendingReachIDAfterRetry(t *testing.T) {
	sp := &fakeSpawner{err: errors.New("temporary sensor failure")}
	sk := &typedFakeSink{}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &sk.fakeSink)
	w.sink = sk
	w.rootID = "rootabc"
	w.reachStatePath = willReachStatePath(t.TempDir())

	util, err := w.tick(context.Background())
	if err == nil || util != willUtilPressure {
		t.Fatalf("first reach should surface the sensor error, util=%q err=%v", util, err)
	}
	if f.discharged {
		t.Fatal("a failed reach must not discharge the tide")
	}
	if len(sk.events) != 2 {
		t.Fatalf("want intention and sensor_error learning events, got %#v", sk.events)
	}
	reachID := sk.events[0].ID
	if reachID == "" || sk.events[1].ID != reachID {
		t.Fatalf("failed reach phases must share a stable id, got %#v", sk.events)
	}
	st, err := loadWillReachState(w.reachStatePath)
	if err != nil {
		t.Fatalf("load pending reach: %v", err)
	}
	if st.Pending == nil || st.Pending.ID != reachID || st.NextSeq != 1 {
		t.Fatalf("failed reach must remain pending for retry, got %#v", st)
	}

	sp.err = nil
	sp.line = []byte(`{"util":"repo_monitor","kind":"modified","path":"README.md"}`)
	sk.events = nil
	if util, err = w.tick(context.Background()); err != nil || util != willUtilPressure {
		t.Fatalf("retry should complete the same reach, util=%q err=%v", util, err)
	}
	if len(sk.events) != 3 {
		t.Fatalf("retry should emit intention/act/learning, got %#v", sk.events)
	}
	for i, ev := range sk.events {
		if ev.ID != reachID {
			t.Fatalf("retry event %d changed reach id: got %#v want %s", i, ev, reachID)
		}
	}
	st, err = loadWillReachState(w.reachStatePath)
	if err != nil {
		t.Fatalf("reload reach state: %v", err)
	}
	if st.Pending != nil || st.NextSeq != 2 {
		t.Fatalf("completed retry should clear pending and advance sequence, got %#v", st)
	}
}

func TestWillTickEmitErrorDoesNotDischarge(t *testing.T) {
	sp, sk := &fakeSpawner{line: []byte(`{"util":"repo_monitor","kind":"added"}`)}, &fakeSink{err: errors.New("disk full")}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
	}, sp, sk)
	util, err := w.tick(context.Background())
	if err == nil {
		t.Fatal("an emit error must surface")
	}
	if util != willUtilPressure {
		t.Fatalf("got util %q", util)
	}
	if f.discharged {
		t.Error("the tide must not be spent when the perception was not durably emitted")
	}
	if sp.committed {
		t.Error("utility state must not commit when event delivery failed")
	}
}

func TestWillTickEmptyFileSinkDoesNotReachOrCommit(t *testing.T) {
	sp := &fakeSpawner{line: []byte(`{"util":"repo_monitor","kind":"added"}`)}
	f := &fakeWillField{vars: map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
	}}
	w := &willTicker{field: f, script: "x.aml", spawner: sp, sink: fileSink{}}
	util, err := w.tick(context.Background())
	if err == nil {
		t.Fatal("a missing SARTRE event sink must surface before the sensor can advance")
	}
	if util != willUtilPressure {
		t.Fatalf("got util %q", util)
	}
	if sp.util != "" {
		t.Fatalf("the will must not spawn a sensor before intention delivery is durable, spawned %q", sp.util)
	}
	if sp.committed {
		t.Error("utility state must not commit without a durable event sink")
	}
	if f.discharged {
		t.Error("the tide must not be spent without a durable event sink")
	}
}

func TestWillTickStateCommitErrorDoesNotDischarge(t *testing.T) {
	sp, sk := &fakeSpawner{line: []byte(`{"util":"repo_monitor","kind":"added"}`), commitErr: errors.New("rename")}, &fakeSink{}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
	}, sp, sk)
	util, err := w.tick(context.Background())
	if err == nil {
		t.Fatal("a state commit error must surface")
	}
	if util != willUtilPressure {
		t.Fatalf("got util %q", util)
	}
	if f.discharged {
		t.Error("the tide must not be spent when the sensor state did not commit")
	}
}

type typedFakeSink struct {
	fakeSink
	events    []willEvent
	failPhase string
	phaseErr  error
}

func (s *typedFakeSink) EmitEvent(ev willEvent) error {
	if s.err != nil {
		return s.err
	}
	if s.failPhase != "" && ev.Phase == s.failPhase {
		if s.phaseErr != nil {
			return s.phaseErr
		}
		return errors.New("phase delivery failed")
	}
	s.events = append(s.events, ev)
	return nil
}

func TestWillTickTypedPhasesSurroundPerception(t *testing.T) {
	sp := &fakeSpawner{line: []byte(`{"util":"repo_monitor","kind":"added","path":"README.md"}`)}
	sk := &typedFakeSink{}
	w, _ := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &sk.fakeSink)
	w.sink = sk
	w.rootID = "rootabc"
	w.cadence = 750 * time.Millisecond
	w.refractory = 3
	if _, err := w.tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(sk.events) != 3 {
		t.Fatalf("want intention/act/learning, got %#v", sk.events)
	}
	if sk.events[0].Phase != "intention" || sk.events[1].Phase != "act" || sk.events[2].Phase != "learning" {
		t.Fatalf("wrong phase order: %#v", sk.events)
	}
	if sk.events[2].Outcome != "perception_committed" {
		t.Fatalf("learning outcome should record committed perception, got %#v", sk.events[2])
	}
	if sk.events[2].EffectCount != 1 {
		t.Fatalf("learning must record committed effect count, got %#v", sk.events[2])
	}
	if sk.events[0].PressureTide != 1.5 || sk.events[0].PullPressure != 0.3 {
		t.Fatalf("intention must receipt both vector tide and current pull, got %#v", sk.events[0])
	}
	if sk.events[0].CuriosityTide != 0 || sk.events[0].CareTide != 0 || sk.events[0].BoundaryTide != 0 {
		t.Fatalf("dormant vector channels must be explicit zero receipts, got %#v", sk.events[0])
	}
	for i, ev := range sk.events {
		if ev.RootID != "rootabc" || ev.Breath != 1 || ev.CadenceMS != 750 || ev.RefractoryBreaths != 3 {
			t.Fatalf("event %d must receipt breath-counted time domain, got %#v", i, ev)
		}
	}
}

func TestWillTickNoNoveltyExtendsNextCooldown(t *testing.T) {
	sp := &fakeSpawner{}
	sk := &typedFakeSink{}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &sk.fakeSink)
	w.sink = sk
	w.refractory = 2

	if _, err := w.tick(context.Background()); err != nil {
		t.Fatalf("first tick: %v", err)
	}
	if !sp.committed || !f.discharged {
		t.Fatal("no-novelty still needs a durable state commit and tide discharge")
	}
	if w.quietRuns != 1 || w.cooldown != 3 {
		t.Fatalf("first no-novelty should add one quiet cooldown breath, quiet=%d cooldown=%d", w.quietRuns, w.cooldown)
	}
	learn := sk.events[len(sk.events)-1]
	if learn.Outcome != "no_novelty" || learn.EffectCount != 0 || learn.CooldownBreaths != 3 {
		t.Fatalf("learning receipt must carry no-novelty plasticity, got %#v", learn)
	}

	w.cooldown = 0
	f.vars["will_gaze"] = 1.5
	f.vars["will_pressure_tide"] = 1.5
	f.discharged = false
	sp.committed = false
	sk.events = nil
	if _, err := w.tick(context.Background()); err != nil {
		t.Fatalf("second tick: %v", err)
	}
	if w.quietRuns != 2 || w.cooldown != 4 {
		t.Fatalf("second no-novelty should continue quiet plasticity, quiet=%d cooldown=%d", w.quietRuns, w.cooldown)
	}
}

func TestWillTickPerceptionCommittedResetsQuietPlasticity(t *testing.T) {
	sp := &fakeSpawner{line: []byte(`{"util":"repo_monitor","kind":"modified","path":"research/new.md"}`)}
	sk := &typedFakeSink{}
	w, _ := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &sk.fakeSink)
	w.sink = sk
	w.refractory = 2
	w.quietRuns = 3

	if _, err := w.tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if w.quietRuns != 0 || w.cooldown != 2 {
		t.Fatalf("committed perception should return to the base refractory, quiet=%d cooldown=%d", w.quietRuns, w.cooldown)
	}
	learn := sk.events[len(sk.events)-1]
	if learn.Outcome != "perception_committed" || learn.EffectCount != 1 || learn.CooldownBreaths != 2 {
		t.Fatalf("learning receipt must carry committed-perception cooldown, got %#v", learn)
	}
}

func TestWillTickPersistsQuietLearningState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "will-learning.state.json")
	sp := &fakeSpawner{}
	sk := &typedFakeSink{}
	w, _ := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &sk.fakeSink)
	w.sink = sk
	w.refractory = 2
	w.learningStatePath = path

	if _, err := w.tick(context.Background()); err != nil {
		t.Fatalf("no-novelty tick: %v", err)
	}
	st, err := loadWillLearningState(path)
	if err != nil {
		t.Fatalf("load learning state: %v", err)
	}
	if st.QuietRuns != 1 {
		t.Fatalf("no-novelty must persist the quiet streak, got %#v", st)
	}
	if st.LastReachID == "" || st.LastUtility != willUtilPressure || st.LastOutcome != willOutcomeNoNovelty ||
		st.LastEffectCount != 0 || st.LastCooldown != 3 || st.CooldownBreaths != 3 || st.LastTide == nil ||
		st.LastTide.PressureTide != 1.5 {
		t.Fatalf("no-novelty must persist a typed consequence receipt, got %#v", st)
	}

	sp2 := &fakeSpawner{line: []byte(`{"util":"repo_monitor","kind":"modified","path":"README.md"}`)}
	sk2 := &typedFakeSink{}
	w2, _ := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp2, &sk2.fakeSink)
	w2.sink = sk2
	w2.refractory = 2
	w2.learningStatePath = path
	w2.quietRuns = st.QuietRuns

	if _, err := w2.tick(context.Background()); err != nil {
		t.Fatalf("committed-perception tick: %v", err)
	}
	st, err = loadWillLearningState(path)
	if err != nil {
		t.Fatalf("reload learning state: %v", err)
	}
	if st.QuietRuns != 0 {
		t.Fatalf("committed perception must persist quiet reset, got %#v", st)
	}
	if st.LastOutcome != willOutcomePerceptionCommitted || st.LastEffectCount != 1 ||
		st.LastCooldown != 2 || st.CooldownBreaths != 2 || st.LastUtility != willUtilPressure || st.LastTide == nil ||
		st.LastTide.PressureTide != 1.5 {
		t.Fatalf("committed perception must persist the typed consequence receipt, got %#v", st)
	}
}

func TestWillTickPersistsCooldownCountdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "will-learning.state.json")
	if err := saveWillLearningState(path, willLearningState{
		QuietRuns:       2,
		LastReachID:     "reach-cooldown",
		LastUtility:     willUtilPressure,
		LastOutcome:     willOutcomeNoNovelty,
		LastEffectCount: 0,
		LastCooldown:    3,
		CooldownBreaths: 3,
		LastBreath:      7,
		LastTide:        &willTideSnapshot{Threshold: 1, Gaze: 1.3, PressureTide: 1.3},
	}); err != nil {
		t.Fatalf("seed learning state: %v", err)
	}
	sp, sk := &fakeSpawner{line: []byte(`{"util":"repo_monitor","kind":"modified","path":"README.md"}`)}, &fakeSink{}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 5.0, "pull_origin": 0.0, "pull_pressure": 1.0,
		"will_pressure_tide": 5.0,
	}, sp, sk)
	w.learningStatePath = path
	w.cooldown = 3

	util, err := w.tick(context.Background())
	if err != nil {
		t.Fatalf("cooldown tick: %v", err)
	}
	if util != "" || sp.calls != 0 || f.discharged {
		t.Fatalf("cooldown breath must not reach or discharge, util=%q calls=%d discharged=%v", util, sp.calls, f.discharged)
	}
	if w.cooldown != 2 {
		t.Fatalf("in-memory cooldown should decrement to 2, got %d", w.cooldown)
	}
	st, err := loadWillLearningState(path)
	if err != nil {
		t.Fatalf("load learning state: %v", err)
	}
	if st.CooldownBreaths != 2 || st.LastCooldown != 3 || st.LastReachID != "reach-cooldown" || st.LastOutcome != willOutcomeNoNovelty {
		t.Fatalf("cooldown tick must persist only current refractory countdown, got %#v", st)
	}
	if st.CurrentBreath != 1 {
		t.Fatalf("cooldown tick must persist the current breath cursor, got %#v", st)
	}
}

func TestInitialWillBreathRestoresDurableCursor(t *testing.T) {
	reach := willReachState{
		Version: willReachStateVersion,
		NextSeq: 9,
		Pending: &willPendingReach{
			Seq:     9,
			ID:      "reach9",
			Utility: willUtilPressure,
			Tide:    willTideSnapshot{Threshold: 1, Gaze: 1.2, PressureTide: 1.2},
			Breath:  44,
		},
	}
	got := initialWillBreath(willLearningState{CurrentBreath: 40, LastBreath: 41}, reach)
	if got != 44 {
		t.Fatalf("pending reach breath must seed restart cursor, got %d", got)
	}
	got = initialWillBreath(willLearningState{CurrentBreath: 45, LastBreath: 41}, reach)
	if got != 45 {
		t.Fatalf("current breath must seed restart cursor when newer than pending, got %d", got)
	}
}

func TestWillTickLearningStateErrorDoesNotDischarge(t *testing.T) {
	sp := &fakeSpawner{}
	sk := &typedFakeSink{}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &sk.fakeSink)
	w.sink = sk
	w.refractory = 2
	w.learningStatePath = t.TempDir() // rename(file.pending, existing directory) must fail

	util, err := w.tick(context.Background())
	if err == nil {
		t.Fatal("a learning-state write error must surface")
	}
	if util != willUtilPressure {
		t.Fatalf("got util %q", util)
	}
	if f.discharged {
		t.Fatal("the tide must not be spent when host-side learning was not durable")
	}
	if w.quietRuns != 0 || w.cooldown != 0 {
		t.Fatalf("in-memory learning must not advance on a failed durable write, quiet=%d cooldown=%d", w.quietRuns, w.cooldown)
	}
	if len(sk.events) != 2 || sk.events[0].Phase != "intention" || sk.events[1].Phase != "act" {
		t.Fatalf("failed learning-state commit must not emit success learning, got %#v", sk.events)
	}
}

func TestWillTickDischargeErrorDoesNotEmitSuccessLearning(t *testing.T) {
	sp := &fakeSpawner{}
	sk := &typedFakeSink{}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &sk.fakeSink)
	w.sink = sk
	w.rootID = "rootabc"
	w.reachStatePath = willReachStatePath(t.TempDir())
	f.execErr = errors.New("aml transaction failed")

	util, err := w.tick(context.Background())
	if err == nil {
		t.Fatal("a discharge error must surface")
	}
	if util != willUtilPressure {
		t.Fatalf("got util %q", util)
	}
	if f.discharged {
		t.Fatal("a failed discharge transaction must not spend the tide")
	}
	if w.quietRuns != 0 || w.cooldown != 0 {
		t.Fatalf("in-memory learning must not advance on a failed discharge, quiet=%d cooldown=%d", w.quietRuns, w.cooldown)
	}
	if len(sk.events) != 2 || sk.events[0].Phase != "intention" || sk.events[1].Phase != "act" {
		t.Fatalf("failed discharge must not emit success learning, got %#v", sk.events)
	}
	st, err := loadWillReachState(w.reachStatePath)
	if err != nil {
		t.Fatalf("load reach state: %v", err)
	}
	if st.Pending == nil || st.Pending.ID != sk.events[0].ID || st.NextSeq != 1 {
		t.Fatalf("failed discharge must keep the reach pending for retry, got %#v", st)
	}
}

func TestWillTickLearningReceiptErrorKeepsPendingReach(t *testing.T) {
	sp := &fakeSpawner{}
	sk := &typedFakeSink{failPhase: "learning", phaseErr: errors.New("sink down")}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &sk.fakeSink)
	w.sink = sk
	w.rootID = "rootabc"
	w.reachStatePath = willReachStatePath(t.TempDir())

	util, err := w.tick(context.Background())
	if err == nil {
		t.Fatal("a learning receipt delivery error must surface")
	}
	if util != willUtilPressure {
		t.Fatalf("got util %q", util)
	}
	if !f.discharged {
		t.Fatal("learning receipt happens after the tide is spent")
	}
	if w.quietRuns != 0 || w.cooldown != 0 {
		t.Fatalf("in-memory learning must not advance until reach finish, quiet=%d cooldown=%d", w.quietRuns, w.cooldown)
	}
	if len(sk.events) != 2 || sk.events[0].Phase != "intention" || sk.events[1].Phase != "act" {
		t.Fatalf("failed learning receipt should leave only intention/act delivered, got %#v", sk.events)
	}
	st, err := loadWillReachState(w.reachStatePath)
	if err != nil {
		t.Fatalf("load reach state: %v", err)
	}
	if st.Pending == nil || st.Pending.ID != sk.events[0].ID || st.NextSeq != 1 {
		t.Fatalf("failed learning receipt must keep the reach pending for retry, got %#v", st)
	}
}

func TestWillTickRetryKeepsCommittedConsequenceAfterReceiptFailure(t *testing.T) {
	stateDir := t.TempDir()
	reachPath := willReachStatePath(stateDir)
	learningPath := willLearningStatePath(stateDir)
	sp := &fakeSpawner{line: []byte(`{"util":"repo_monitor","kind":"modified","path":"README.md"}`)}
	sk := &typedFakeSink{failPhase: "learning", phaseErr: errors.New("sink down")}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &sk.fakeSink)
	w.sink = sk
	w.rootID = "rootabc"
	w.reachStatePath = reachPath
	w.learningStatePath = learningPath
	w.breath = 41

	util, err := w.tick(context.Background())
	if err == nil {
		t.Fatal("the first tick should fail at the final learning receipt")
	}
	if util != willUtilPressure {
		t.Fatalf("got util %q", util)
	}
	if sp.calls != 1 || !sp.committed || !f.discharged {
		t.Fatalf("first reach must emit, commit, and spend once before receipt failure, calls=%d committed=%v discharged=%v",
			sp.calls, sp.committed, f.discharged)
	}
	if len(sk.events) != 2 || sk.events[0].Phase != "intention" || sk.events[1].Phase != "act" {
		t.Fatalf("failed final receipt should leave only intention/act delivered, got %#v", sk.events)
	}
	reachID := sk.events[0].ID
	reachBreath := sk.events[0].Breath
	if reachBreath != 42 || sk.events[1].Breath != reachBreath {
		t.Fatalf("first reach must receipt one durable breath, got intention=%d act=%d", reachBreath, sk.events[1].Breath)
	}
	st, err := loadWillReachState(reachPath)
	if err != nil {
		t.Fatalf("load pending reach: %v", err)
	}
	if st.Pending == nil || !st.Pending.ConsequenceCommitted || st.Pending.Outcome != willOutcomePerceptionCommitted ||
		st.Pending.EffectCount != 1 || st.Pending.ID != reachID {
		t.Fatalf("pending reach must remember the committed consequence for restart retry, got %#v", st.Pending)
	}

	sp2 := &fakeSpawner{}
	sk2 := &typedFakeSink{}
	w2, f2 := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 0, "pull_origin": 0.0, "pull_pressure": 0.0,
	}, sp2, &sk2.fakeSink)
	w2.sink = sk2
	w2.rootID = "rootabc"
	w2.reachStatePath = reachPath
	w2.learningStatePath = learningPath
	w2.pendingReach = st.Pending
	w2.nextReachSeq = st.NextSeq

	util, err = w2.tick(context.Background())
	if err != nil {
		t.Fatalf("retry should finalize the committed consequence without respawning: %v", err)
	}
	if util != willUtilPressure {
		t.Fatalf("retry util changed, got %q", util)
	}
	if sp2.calls != 0 {
		t.Fatalf("retry after committed consequence must not respawn the utility, calls=%d", sp2.calls)
	}
	if !f2.discharged {
		t.Fatal("retry must still leave the tide spent before the success receipt")
	}
	if len(sk2.events) != 1 {
		t.Fatalf("retry should emit only the missing learning receipt, got %#v", sk2.events)
	}
	learn := sk2.events[0]
	if learn.ID != reachID || learn.Phase != "learning" || learn.Outcome != willOutcomePerceptionCommitted ||
		learn.EffectCount != 1 {
		t.Fatalf("retry must preserve the original consequence, got %#v", learn)
	}
	if learn.Breath != reachBreath {
		t.Fatalf("retry must preserve the original reach breath, got %d want %d", learn.Breath, reachBreath)
	}
	st, err = loadWillReachState(reachPath)
	if err != nil {
		t.Fatalf("reload reach state: %v", err)
	}
	if st.Pending != nil || st.NextSeq != 2 {
		t.Fatalf("finalized retry must clear pending reach and advance sequence, got %#v", st)
	}
}

func TestWillTickPendingReachBypassesRestoredCooldown(t *testing.T) {
	stateDir := t.TempDir()
	reachPath := willReachStatePath(stateDir)
	learningPath := willLearningStatePath(stateDir)
	pending := &willPendingReach{
		Seq:                  7,
		ID:                   "reach7",
		Utility:              willUtilPressure,
		Tide:                 willTideSnapshot{Threshold: 1, Gaze: 1.6, PressureTide: 1.6},
		Breath:               33,
		ConsequenceCommitted: true,
		Outcome:              willOutcomePerceptionCommitted,
		EffectCount:          1,
	}
	if err := saveWillReachState(reachPath, willReachState{
		NextSeq: 7,
		Pending: pending,
	}); err != nil {
		t.Fatalf("seed pending reach: %v", err)
	}
	if err := saveWillLearningState(learningPath, willLearningState{
		QuietRuns:       0,
		LastReachID:     pending.ID,
		LastUtility:     pending.Utility,
		LastOutcome:     pending.Outcome,
		LastEffectCount: pending.EffectCount,
		LastCooldown:    4,
		CooldownBreaths: 4,
		CurrentBreath:   33,
		LastBreath:      33,
		LastTide:        &pending.Tide,
	}); err != nil {
		t.Fatalf("seed learning state: %v", err)
	}
	learning, err := loadWillLearningState(learningPath)
	if err != nil {
		t.Fatalf("load learning state: %v", err)
	}
	reach, err := loadWillReachState(reachPath)
	if err != nil {
		t.Fatalf("load reach state: %v", err)
	}
	sp := &fakeSpawner{}
	sk := &typedFakeSink{}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 0,
	}, sp, &sk.fakeSink)
	w.sink = sk
	w.rootID = "rootabc"
	w.reachStatePath = reachPath
	w.learningStatePath = learningPath
	w.refractory = 4
	w.pendingReach = reach.Pending
	w.nextReachSeq = reach.NextSeq
	w.cooldown = learning.CooldownBreaths
	w.breath = initialWillBreath(learning, reach)

	util, err := w.tick(context.Background())
	if err != nil {
		t.Fatalf("pending retry should finalize before restored cooldown: %v", err)
	}
	if util != willUtilPressure {
		t.Fatalf("retry util changed, got %q", util)
	}
	if sp.calls != 0 {
		t.Fatalf("committed pending consequence must not respawn the utility, calls=%d", sp.calls)
	}
	if !f.discharged {
		t.Fatal("pending retry must still discharge the stored tide")
	}
	if w.cooldown != 4 {
		t.Fatalf("finalized retry must restore the planned cooldown, got %d", w.cooldown)
	}
	if len(sk.events) != 1 {
		t.Fatalf("retry should emit the missing learning receipt immediately, got %#v", sk.events)
	}
	if sk.events[0].Phase != "learning" || sk.events[0].ID != pending.ID || sk.events[0].Breath != pending.Breath {
		t.Fatalf("wrong pending learning receipt: %#v", sk.events[0])
	}
	st, err := loadWillReachState(reachPath)
	if err != nil {
		t.Fatalf("reload reach state: %v", err)
	}
	if st.Pending != nil || st.NextSeq != 8 {
		t.Fatalf("finalized pending retry must clear reach state, got %#v", st)
	}
}

func TestWillTickOverflowDoesNotCommitOrDischarge(t *testing.T) {
	sp := &fakeSpawner{
		line:     []byte(`{"util":"repo_monitor","kind":"added","path":"a.md"}`),
		overflow: true,
	}
	sk := &typedFakeSink{}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &sk.fakeSink)
	w.sink = sk

	util, err := w.tick(context.Background())
	if err == nil {
		t.Fatal("overflow must surface as a delivery error")
	}
	if util != willUtilPressure {
		t.Fatalf("got util %q", util)
	}
	if f.discharged {
		t.Error("overflow must not spend the tide")
	}
	if sp.committed {
		t.Error("overflow must not commit utility state")
	}
	if len(sk.lines) != 0 {
		t.Fatalf("overflow must not emit partial effect lines, got %q", sk.lines)
	}
	if len(sk.events) != 3 {
		t.Fatalf("want intention/act/learning overflow events, got %#v", sk.events)
	}
	learn := sk.events[2]
	if learn.Phase != "learning" || learn.Outcome != "overflow" ||
		learn.BytesCaptured != len(sp.line) || learn.BytesLimit != willMaxStdout {
		t.Fatalf("overflow learning event not typed precisely: %#v", learn)
	}
}
