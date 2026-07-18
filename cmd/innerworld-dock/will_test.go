package main

import (
	"context"
	"errors"
	"os"
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
	util             string // the last util asked for
	line             []byte
	overflow         bool
	err              error
	commitErr        error
	afterCommit      func() error
	pendingStatePath string
	statePath        string
	committed        bool
	calls            int
}

func (s *fakeSpawner) Spawn(_ context.Context, util string) (willSpawnResult, error) {
	s.calls++
	s.util = util
	if s.err != nil {
		return willSpawnResult{}, s.err
	}
	pendingPath, statePath, err := s.ensurePendingState(util)
	if err != nil {
		return willSpawnResult{}, err
	}
	return willSpawnResult{
		Line:             s.line,
		Overflow:         s.overflow,
		PendingStatePath: pendingPath,
		StatePath:        statePath,
		Commit: func(expectedSHA256 string) error {
			if s.commitErr != nil {
				return s.commitErr
			}
			digest := expectedSHA256
			if digest == "" {
				var err error
				digest, err = willFileSHA256(pendingPath)
				if err != nil {
					return err
				}
			}
			if err := publishPendingWillState(pendingPath, statePath, digest); err != nil {
				return err
			}
			s.committed = true
			if s.afterCommit != nil {
				return s.afterCommit()
			}
			return nil
		},
	}, nil
}

func (s *fakeSpawner) ensurePendingState(util string) (string, string, error) {
	if s.pendingStatePath != "" || s.statePath != "" {
		if s.pendingStatePath == "" || s.statePath == "" {
			return "", "", errors.New("fake spawner needs both pending and canonical state paths")
		}
		return s.pendingStatePath, s.statePath, nil
	}
	dir, err := os.MkdirTemp("", "will-fake-state-*")
	if err != nil {
		return "", "", err
	}
	statePath := filepath.Join(dir, util+".state")
	pendingPath := statePath + ".pending"
	if err := os.WriteFile(pendingPath, []byte("pending\n"), 0o644); err != nil {
		return "", "", err
	}
	return pendingPath, statePath, nil
}

type blockingSpawner struct {
	base    fakeSpawner
	entered chan struct{}
	release chan struct{}
}

func (s *blockingSpawner) Spawn(_ context.Context, util string) (willSpawnResult, error) {
	select {
	case <-s.entered:
	default:
		close(s.entered)
	}
	<-s.release
	return s.base.Spawn(context.Background(), util)
}

type fakeSink struct {
	lines     [][]byte
	err       error
	afterEmit func()
}

func (s *fakeSink) Emit(line []byte) error {
	if s.err != nil {
		return s.err
	}
	s.lines = append(s.lines, append([]byte(nil), line...))
	if s.afterEmit != nil {
		s.afterEmit()
	}
	return nil
}

type afterEmitFileSink struct {
	path       string
	afterEmit  func()
	afterEvent func(willEvent)
}

func (s *afterEmitFileSink) Emit(line []byte) error {
	if err := (fileSink{path: s.path}).Emit(line); err != nil {
		return err
	}
	if len(completeSartreJSONLines(line)) > 0 && s.afterEmit != nil {
		s.afterEmit()
	}
	return nil
}

func (s *afterEmitFileSink) EmitEvent(ev willEvent) error {
	if err := (fileSink{path: s.path}).EmitEvent(ev); err != nil {
		return err
	}
	if s.afterEvent != nil {
		s.afterEvent(ev)
	}
	return nil
}

type emitOnlyFileSink struct{ path string }

func (s emitOnlyFileSink) Emit(line []byte) error {
	return (fileSink{path: s.path}).Emit(line)
}

func (s emitOnlyFileSink) EmitEvent(ev willEvent) error {
	return (fileSink{path: s.path}).EmitEvent(ev)
}

func newWill(vars map[string]float32, sp *fakeSpawner, sk *fakeSink) (*willTicker, *fakeWillField) {
	f := &fakeWillField{vars: vars}
	return &willTicker{field: f, script: "x.aml", spawner: sp, sink: sk}, f
}

func TestWillOwnerLivesUntilRunReturns(t *testing.T) {
	stateDir := t.TempDir()
	owner, err := acquireWillNamespaceOwner(stateDir)
	if err != nil {
		t.Fatalf("acquire owner: %v", err)
	}
	sp := &blockingSpawner{
		base: fakeSpawner{
			line: []byte(`{"util":"repo_monitor","kind":"modified","path":"README.md"}`),
		},
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	sk := &fakeSink{}
	f := &fakeWillField{vars: map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}}
	wt := &willTicker{
		field:             f,
		script:            "x.aml",
		spawner:           sp,
		sink:              sk,
		rootID:            "rootabc",
		reachStatePath:    willReachStatePath(stateDir),
		learningStatePath: willLearningStatePath(stateDir),
		refractory:        1,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runWillWithOwner(ctx, wt, owner, stateDir, time.Millisecond)
		close(done)
	}()

	select {
	case <-sp.entered:
	case <-time.After(2 * time.Second):
		cancel()
		close(sp.release)
		t.Fatal("will run did not enter the blocked reach")
	}
	cancel()
	if out, err := willNamespaceOwnerHelperCommand(stateDir).CombinedOutput(); err == nil {
		t.Fatalf("namespace became claimable while cancelled run was still inside reach, output=%s", out)
	}
	close(sp.release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("will run did not return after blocked reach was released")
	}
	if out, err := willNamespaceOwnerHelperCommand(stateDir).CombinedOutput(); err != nil {
		t.Fatalf("namespace should be claimable only after run returns, err=%v output=%s", err, out)
	}
}

func TestWillTickReachesOnOriginCrest(t *testing.T) {
	sp, sk := &fakeSpawner{line: []byte(`{"util":"whatdotheythinkiam","recognized":1}`)}, &fakeSink{}
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
	sp, sk := &fakeSpawner{line: []byte(`{"util":"repo_monitor","kind":"modified","path":"README.md"}`)}, &fakeSink{}
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
	sp, sk := &fakeSpawner{line: []byte(`{"util":"whatdotheythinkiam","recognized":1}`)}, &fakeSink{}
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
	sp, sk := &fakeSpawner{line: []byte(`{"util":"repo_monitor","kind":"modified","path":"README.md"}`)}, &fakeSink{}
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

func TestWillTickDeadLettersRepeatedSensorFailure(t *testing.T) {
	stateDir := t.TempDir()
	sp := &fakeSpawner{err: errors.New("missing utility")}
	sk := &typedFakeSink{}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &sk.fakeSink)
	w.sink = sk
	w.rootID = "rootabc"
	w.refractory = 2
	w.maxReachAttempts = 2
	w.reachStatePath = willReachStatePath(stateDir)
	w.learningStatePath = willLearningStatePath(stateDir)

	util, err := w.tick(context.Background())
	if err == nil || util != willUtilPressure {
		t.Fatalf("first failed reach should surface the sensor error, util=%q err=%v", util, err)
	}
	if f.discharged {
		t.Fatal("the first failed attempt must not spend the tide")
	}
	if len(sk.events) != 2 || sk.events[1].Outcome != willOutcomeSensorError || sk.events[1].Attempts != 1 {
		t.Fatalf("first failure must emit attempt=1 sensor_error, got %#v", sk.events)
	}
	reachID := sk.events[0].ID
	st, err := loadWillReachState(w.reachStatePath)
	if err != nil {
		t.Fatalf("load pending reach: %v", err)
	}
	if st.Pending == nil || st.Pending.ID != reachID || st.Pending.Attempts != 1 ||
		st.Pending.LastFailureOutcome != willOutcomeSensorError || st.Pending.ConsequenceCommitted {
		t.Fatalf("first failure must keep typed pending reach for retry, got %#v", st.Pending)
	}

	sk.events = nil
	util, err = w.tick(context.Background())
	if err != nil || util != willUtilPressure {
		t.Fatalf("second failed reach should close as dead-letter, util=%q err=%v", util, err)
	}
	if sp.calls != 2 {
		t.Fatalf("dead-letter should happen after the configured retry window, calls=%d", sp.calls)
	}
	if !f.discharged {
		t.Fatal("terminal dead-letter must spend the stored tide")
	}
	if sp.committed {
		t.Fatal("dead-letter must not commit failed utility state")
	}
	if len(sk.events) != 3 {
		t.Fatalf("second failure should emit intention, failure learning, terminal learning, got %#v", sk.events)
	}
	if sk.events[0].ID != reachID || sk.events[1].ID != reachID || sk.events[2].ID != reachID {
		t.Fatalf("dead-letter must preserve the reach id, got %#v", sk.events)
	}
	if sk.events[1].Outcome != willOutcomeSensorError || sk.events[1].Attempts != 2 {
		t.Fatalf("second failure must be attempt=2 sensor_error, got %#v", sk.events[1])
	}
	dead := sk.events[2]
	if dead.Phase != "learning" || dead.Outcome != willOutcomeDeadLetter || dead.Attempts != 2 ||
		dead.FailureOutcome != willOutcomeSensorError || dead.EffectCount != 0 || dead.CooldownBreaths != 3 {
		t.Fatalf("terminal receipt must be a typed dead-letter consequence, got %#v", dead)
	}
	st, err = loadWillReachState(w.reachStatePath)
	if err != nil {
		t.Fatalf("reload reach state: %v", err)
	}
	if st.Pending != nil || st.NextSeq != 2 {
		t.Fatalf("dead-letter must clear reach state and advance sequence, got %#v", st)
	}
	learning, err := loadWillLearningState(w.learningStatePath)
	if err != nil {
		t.Fatalf("load learning state: %v", err)
	}
	if learning.LastOutcome != willOutcomeDeadLetter || learning.LastEffectCount != 0 ||
		learning.LastReachID != reachID || learning.CooldownBreaths != 3 {
		t.Fatalf("dead-letter must become durable learning state, got %#v", learning)
	}
}

func TestWillTickEmitErrorDoesNotDischarge(t *testing.T) {
	sp, sk := &fakeSpawner{line: []byte(`{"util":"repo_monitor","kind":"added","path":"README.md"}`)}, &fakeSink{err: errors.New("disk full")}
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
	sp := &fakeSpawner{line: []byte(`{"util":"repo_monitor","kind":"added","path":"README.md"}`)}
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
	dir := t.TempDir()
	pendingPath := filepath.Join(dir, "repo_monitor.state.pending")
	statePath := filepath.Join(t.TempDir(), "missing", "repo_monitor.state")
	if err := os.WriteFile(pendingPath, []byte("pending\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sp, sk := &fakeSpawner{
		line:             []byte(`{"util":"repo_monitor","kind":"added","path":"README.md"}`),
		pendingStatePath: pendingPath,
		statePath:        statePath,
	}, &fakeSink{}
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
	events     []willEvent
	failPhase  string
	phaseErr   error
	afterEvent func(willEvent)
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
	if s.afterEvent != nil {
		s.afterEvent(ev)
	}
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

func TestWillTickLearningStateErrorKeepsCommittedReachPending(t *testing.T) {
	sp := &fakeSpawner{}
	sk := &typedFakeSink{}
	reachPath := willReachStatePath(t.TempDir())
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &sk.fakeSink)
	w.sink = sk
	w.refractory = 2
	w.reachStatePath = reachPath
	w.learningStatePath = t.TempDir() // rename(file.pending, existing directory) must fail

	util, err := w.tick(context.Background())
	if err == nil {
		t.Fatal("a learning-state write error must surface")
	}
	if util != willUtilPressure {
		t.Fatalf("got util %q", util)
	}
	if !f.discharged {
		t.Fatal("the tide is spent before durable learning state, so learning can receipt spent cause")
	}
	if w.quietRuns != 0 || w.cooldown != 0 {
		t.Fatalf("in-memory learning must not advance on a failed durable write, quiet=%d cooldown=%d", w.quietRuns, w.cooldown)
	}
	if len(sk.events) != 2 || sk.events[0].Phase != "intention" || sk.events[1].Phase != "act" {
		t.Fatalf("failed learning-state commit must not emit success learning, got %#v", sk.events)
	}
	st, err := loadWillReachState(reachPath)
	if err != nil {
		t.Fatalf("load reach state: %v", err)
	}
	if st.Pending == nil || st.Pending.ID != sk.events[0].ID || !st.Pending.ConsequenceCommitted {
		t.Fatalf("failed learning-state write must keep committed reach pending for retry, got %#v", st)
	}
}

func TestWillTickDischargeErrorDoesNotEmitSuccessLearning(t *testing.T) {
	sp := &fakeSpawner{}
	sk := &typedFakeSink{}
	learningPath := willLearningStatePath(t.TempDir())
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &sk.fakeSink)
	w.sink = sk
	w.rootID = "rootabc"
	w.reachStatePath = willReachStatePath(t.TempDir())
	w.learningStatePath = learningPath
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
	learning, err := loadWillLearningState(learningPath)
	if err != nil {
		t.Fatalf("load learning state: %v", err)
	}
	if learning.LastOutcome != "" || learning.LastReachID != "" || learning.CooldownBreaths != 0 {
		t.Fatalf("failed discharge must not publish durable learning state, got %#v", learning)
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

func TestWillTickRetryDoesNotDoubleLearnNoNoveltyAfterFinishGap(t *testing.T) {
	stateDir := t.TempDir()
	reachPath := willReachStatePath(stateDir)
	learningPath := willLearningStatePath(stateDir)
	sp := &fakeSpawner{}
	sk := &typedFakeSink{}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &sk.fakeSink)
	w.sink = sk
	w.rootID = "rootabc"
	w.reachStatePath = reachPath
	w.learningStatePath = learningPath
	w.refractory = 3
	w.quietRuns = 2
	brokenReachPath := t.TempDir()
	sk.afterEvent = func(ev willEvent) {
		if ev.Phase == "learning" && ev.Outcome == willOutcomeNoNovelty {
			w.reachStatePath = brokenReachPath
		}
	}

	util, err := w.tick(context.Background())
	if err == nil {
		t.Fatal("finish reach failure after learning receipt must surface")
	}
	if util != willUtilPressure {
		t.Fatalf("got util %q", util)
	}
	if sp.calls != 1 || !sp.committed || !f.discharged {
		t.Fatalf("first no-novelty reach must spawn, commit, and discharge once; calls=%d committed=%v discharged=%v",
			sp.calls, sp.committed, f.discharged)
	}
	learning, err := loadWillLearningState(learningPath)
	if err != nil {
		t.Fatalf("load learning after first failure: %v", err)
	}
	if learning.QuietRuns != 3 || learning.CooldownBreaths != 6 {
		t.Fatalf("first learning state should apply no_novelty once, got %#v", learning)
	}
	st, err := loadWillReachState(reachPath)
	if err != nil {
		t.Fatalf("load pending reach: %v", err)
	}
	if st.Pending == nil || !st.Pending.ConsequenceCommitted || st.Pending.Outcome != willOutcomeNoNovelty {
		t.Fatalf("failed finish must keep the committed no_novelty reach pending, got %#v", st.Pending)
	}

	sp2 := &fakeSpawner{}
	sk2 := &typedFakeSink{}
	w2, _ := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 0,
	}, sp2, &sk2.fakeSink)
	w2.sink = sk2
	w2.rootID = "rootabc"
	w2.reachStatePath = reachPath
	w2.learningStatePath = learningPath
	w2.pendingReach = st.Pending
	w2.nextReachSeq = st.NextSeq
	w2.refractory = 3
	w2.quietRuns = learning.QuietRuns
	w2.cooldown = learning.CooldownBreaths

	util, err = w2.tick(context.Background())
	if err != nil {
		t.Fatalf("retry should finish the same no_novelty learning without respawning: %v", err)
	}
	if util != willUtilPressure {
		t.Fatalf("retry util changed, got %q", util)
	}
	if sp2.calls != 0 {
		t.Fatalf("retry after committed no_novelty consequence must not respawn the utility, calls=%d", sp2.calls)
	}
	learning, err = loadWillLearningState(learningPath)
	if err != nil {
		t.Fatalf("reload learning: %v", err)
	}
	if learning.QuietRuns != 3 || learning.CooldownBreaths != 6 {
		t.Fatalf("retry must not learn no_novelty twice, got %#v", learning)
	}
}

func TestWillTickRetryDoesNotDuplicateLearningReceiptAfterFinishGap(t *testing.T) {
	stateDir := t.TempDir()
	reachPath := willReachStatePath(stateDir)
	learningPath := willLearningStatePath(stateDir)
	eventPath := filepath.Join(t.TempDir(), "sartre.jsonl")
	sp := &fakeSpawner{}
	sk := &afterEmitFileSink{path: eventPath}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &fakeSink{})
	w.sink = sk
	w.rootID = "rootabc"
	w.reachStatePath = reachPath
	w.learningStatePath = learningPath
	w.refractory = 3
	w.quietRuns = 2
	brokenReachPath := t.TempDir()
	sk.afterEvent = func(ev willEvent) {
		if ev.Phase == "learning" && f.discharged {
			w.reachStatePath = brokenReachPath
		}
	}

	util, err := w.tick(context.Background())
	if err == nil {
		t.Fatal("finish reach failure after file-backed learning receipt must surface")
	}
	if util != willUtilPressure {
		t.Fatalf("got util %q", util)
	}
	data, err := os.ReadFile(eventPath)
	if err != nil {
		t.Fatalf("read first learning delivery: %v", err)
	}
	if got := strings.Count(string(data), `"phase":"learning"`); got != 1 {
		t.Fatalf("first pass should deliver one learning receipt, got %d lines: %s", got, data)
	}
	st, err := loadWillReachState(reachPath)
	if err != nil {
		t.Fatalf("load pending reach: %v", err)
	}
	if st.Pending == nil || !st.Pending.LearningPrepared || st.Pending.LearningRecorded {
		t.Fatalf("failed finish must leave prepared unrecorded learning pending, got %#v", st.Pending)
	}

	sp2 := &fakeSpawner{}
	w2, _ := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 0,
	}, sp2, &fakeSink{})
	w2.sink = emitOnlyFileSink{path: eventPath}
	w2.rootID = "rootabc"
	w2.reachStatePath = reachPath
	w2.learningStatePath = learningPath
	w2.pendingReach = st.Pending
	w2.nextReachSeq = st.NextSeq
	w2.refractory = 3
	w2.quietRuns = 3
	w2.cooldown = 6

	util, err = w2.tick(context.Background())
	if err != nil {
		t.Fatalf("retry should finish the same learning receipt without duplicate append: %v", err)
	}
	if util != willUtilPressure {
		t.Fatalf("retry util changed, got %q", util)
	}
	if sp2.calls != 0 {
		t.Fatalf("retry after delivered learning receipt must not respawn the utility, calls=%d", sp2.calls)
	}
	data, err = os.ReadFile(eventPath)
	if err != nil {
		t.Fatalf("read retry learning delivery: %v", err)
	}
	if got := strings.Count(string(data), `"phase":"learning"`); got != 1 {
		t.Fatalf("retry must not append a duplicate learning receipt, got %d lines: %s", got, data)
	}
}

func TestWillTickRetriesRecordedEffectBaselineWithoutRespawning(t *testing.T) {
	stateDir := t.TempDir()
	reachPath := willReachStatePath(stateDir)
	learningPath := willLearningStatePath(stateDir)
	pendingPath := filepath.Join(stateDir, "repo_monitor.state.pending")
	statePath := filepath.Join(t.TempDir(), "missing", "repo_monitor.state")
	if err := os.WriteFile(pendingPath, []byte("baseline-v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sp := &fakeSpawner{
		line:             []byte(`{"util":"repo_monitor","kind":"modified","path":"README.md"}`),
		pendingStatePath: pendingPath,
		statePath:        statePath,
	}
	sk := &typedFakeSink{}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &sk.fakeSink)
	w.sink = sk
	w.rootID = "rootabc"
	w.reachStatePath = reachPath
	w.learningStatePath = learningPath

	util, err := w.tick(context.Background())
	if err == nil {
		t.Fatal("baseline publish failure must surface")
	}
	if util != willUtilPressure {
		t.Fatalf("got util %q", util)
	}
	if f.discharged {
		t.Fatal("the tide must not discharge before the recorded baseline is published")
	}
	if _, err := os.Stat(pendingPath); err != nil {
		t.Fatalf("failed baseline publish must preserve the exact pending artifact: %v", err)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("canonical baseline must not be visible yet, err=%v", err)
	}
	st, err := loadWillReachState(reachPath)
	if err != nil {
		t.Fatalf("load reach state: %v", err)
	}
	if st.Pending == nil || !st.Pending.EffectRecorded || st.Pending.BaselineCommitted ||
		st.Pending.Outcome != willOutcomePerceptionCommitted || st.Pending.EffectCount != 1 ||
		st.Pending.BaselinePendingPath != pendingPath || st.Pending.BaselineStatePath != statePath ||
		!validSHA256Hex(st.Pending.BaselineSHA256) {
		t.Fatalf("pending reach must journal the delivered effect and exact baseline artifact, got %#v", st.Pending)
	}

	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatal(err)
	}
	sp2 := &fakeSpawner{}
	sk2 := &typedFakeSink{}
	w2, f2 := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 0,
	}, sp2, &sk2.fakeSink)
	w2.sink = sk2
	w2.rootID = "rootabc"
	w2.reachStatePath = reachPath
	w2.learningStatePath = learningPath
	w2.pendingReach = st.Pending
	w2.nextReachSeq = st.NextSeq

	util, err = w2.tick(context.Background())
	if err != nil {
		t.Fatalf("retry should publish the recorded baseline and finish without respawning: %v", err)
	}
	if util != willUtilPressure {
		t.Fatalf("retry util changed, got %q", util)
	}
	if sp2.calls != 0 {
		t.Fatalf("retry from recorded effect stage must not respawn the utility, calls=%d", sp2.calls)
	}
	if !f2.discharged {
		t.Fatal("retry must discharge the original tide after publishing the baseline")
	}
	if data, err := os.ReadFile(statePath); err != nil || string(data) != "baseline-v2\n" {
		t.Fatalf("retry must publish the exact recorded baseline, data=%q err=%v", data, err)
	}
	if len(sk2.events) != 1 || sk2.events[0].Outcome != willOutcomePerceptionCommitted || sk2.events[0].EffectCount != 1 {
		t.Fatalf("retry must learn the original committed perception, got %#v", sk2.events)
	}
}

func TestWillTickRejectsBaselineMutationAfterPreparedSHA(t *testing.T) {
	stateDir := t.TempDir()
	reachPath := willReachStatePath(stateDir)
	learningPath := willLearningStatePath(stateDir)
	pendingPath := filepath.Join(stateDir, "repo_monitor.state.pending")
	statePath := filepath.Join(stateDir, "repo_monitor.state")
	if err := os.WriteFile(pendingPath, []byte("baseline-original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sp := &fakeSpawner{
		line:             []byte(`{"util":"repo_monitor","kind":"modified","path":"README.md"}`),
		pendingStatePath: pendingPath,
		statePath:        statePath,
	}
	sk := &fakeSink{}
	sk.afterEmit = func() {
		if err := os.WriteFile(pendingPath, []byte("baseline-mutated\n"), 0o644); err != nil {
			t.Fatalf("mutate pending state: %v", err)
		}
	}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, sk)
	w.rootID = "rootabc"
	w.reachStatePath = reachPath
	w.learningStatePath = learningPath

	util, err := w.tick(context.Background())
	if err == nil {
		t.Fatal("baseline mutation after recorded SHA must fail closed")
	}
	if util != willUtilPressure {
		t.Fatalf("got util %q", util)
	}
	if sp.committed {
		t.Fatal("mutated baseline must not be accepted as committed")
	}
	if f.discharged {
		t.Fatal("the tide must not discharge after a baseline integrity failure")
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("mutated baseline must not be published, err=%v", err)
	}
	st, err := loadWillReachState(reachPath)
	if err != nil {
		t.Fatalf("load reach state: %v", err)
	}
	if st.Pending == nil || !st.Pending.EffectRecorded || st.Pending.BaselineCommitted ||
		st.Pending.BaselineSHA256 == "" {
		t.Fatalf("reach must keep the delivered effect and original baseline SHA for retry, got %#v", st.Pending)
	}
}

func TestWillTickRecoveryKeepsEffectOutcomeAfterBaselinePublicationGap(t *testing.T) {
	stateDir := t.TempDir()
	reachPath := willReachStatePath(stateDir)
	learningPath := willLearningStatePath(stateDir)
	pendingPath := filepath.Join(stateDir, "repo_monitor.state.pending")
	statePath := filepath.Join(stateDir, "repo_monitor.state")
	if err := os.WriteFile(pendingPath, []byte("baseline-v3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sp := &fakeSpawner{
		line:             []byte(`{"util":"repo_monitor","kind":"modified","path":"README.md"}`),
		pendingStatePath: pendingPath,
		statePath:        statePath,
	}
	sk := &typedFakeSink{}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &sk.fakeSink)
	w.sink = sk
	w.rootID = "rootabc"
	w.reachStatePath = reachPath
	w.learningStatePath = learningPath
	brokenReachPath := t.TempDir()
	sp.afterCommit = func() error {
		w.reachStatePath = brokenReachPath
		return nil
	}

	util, err := w.tick(context.Background())
	if err == nil {
		t.Fatal("reach-state publication failure after baseline publish must surface")
	}
	if util != willUtilPressure {
		t.Fatalf("got util %q", util)
	}
	if !sp.committed {
		t.Fatal("the utility baseline should have been published before the forced reach-state failure")
	}
	if f.discharged {
		t.Fatal("the tide must not discharge before the consequence is durably marked")
	}
	if data, err := os.ReadFile(statePath); err != nil || string(data) != "baseline-v3\n" {
		t.Fatalf("baseline should be visible after the gap, data=%q err=%v", data, err)
	}
	st, err := loadWillReachState(reachPath)
	if err != nil {
		t.Fatalf("load reach state: %v", err)
	}
	if st.Pending == nil || !st.Pending.EffectRecorded || st.Pending.BaselineCommitted ||
		st.Pending.Outcome != willOutcomePerceptionCommitted || st.Pending.EffectCount != 1 {
		t.Fatalf("original reach state must retain the recoverable effect stage, got %#v", st.Pending)
	}

	sp2 := &fakeSpawner{}
	sk2 := &typedFakeSink{}
	w2, f2 := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 0,
	}, sp2, &sk2.fakeSink)
	w2.sink = sk2
	w2.rootID = "rootabc"
	w2.reachStatePath = reachPath
	w2.learningStatePath = learningPath
	w2.pendingReach = st.Pending
	w2.nextReachSeq = st.NextSeq

	util, err = w2.tick(context.Background())
	if err != nil {
		t.Fatalf("retry should recognize the already-published baseline and finish: %v", err)
	}
	if util != willUtilPressure {
		t.Fatalf("retry util changed, got %q", util)
	}
	if sp2.calls != 0 {
		t.Fatalf("retry after baseline gap must not respawn and re-derive no_novelty, calls=%d", sp2.calls)
	}
	if !f2.discharged {
		t.Fatal("retry must discharge the original tide after consequence recovery")
	}
	if len(sk2.events) != 1 || sk2.events[0].Outcome != willOutcomePerceptionCommitted || sk2.events[0].EffectCount != 1 {
		t.Fatalf("retry must preserve the delivered effect outcome, got %#v", sk2.events)
	}
	learning, err := loadWillLearningState(learningPath)
	if err != nil {
		t.Fatalf("load learning: %v", err)
	}
	if learning.LastOutcome != willOutcomePerceptionCommitted || learning.LastEffectCount != 1 {
		t.Fatalf("learning must not relabel the committed effect as no_novelty, got %#v", learning)
	}
}

func TestWillTickRecoveryDoesNotRespawnAfterEffectDeliveryStateGap(t *testing.T) {
	stateDir := t.TempDir()
	reachPath := willReachStatePath(stateDir)
	learningPath := willLearningStatePath(stateDir)
	eventPath := filepath.Join(t.TempDir(), "sartre.jsonl")
	sp := &fakeSpawner{
		line: []byte(`{"util":"repo_monitor","kind":"modified","path":"README.md"}`),
	}
	sk := &afterEmitFileSink{path: eventPath}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &fakeSink{})
	w.sink = sk
	w.rootID = "rootabc"
	w.reachStatePath = reachPath
	w.learningStatePath = learningPath
	brokenReachPath := t.TempDir()
	sk.afterEmit = func() {
		w.reachStatePath = brokenReachPath
	}

	util, err := w.tick(context.Background())
	if err == nil {
		t.Fatal("reach-state publication failure after effect delivery must surface")
	}
	if util != willUtilPressure {
		t.Fatalf("got util %q", util)
	}
	if sp.calls != 1 {
		t.Fatalf("first pass should spawn once, calls=%d", sp.calls)
	}
	data, err := os.ReadFile(eventPath)
	if err != nil {
		t.Fatalf("read first effect delivery: %v", err)
	}
	if got := strings.Count(string(data), `"phase":"effect"`); got != 1 {
		t.Fatalf("first pass should deliver the effect once before failing state save, got %d effect lines: %s", got, data)
	}
	if f.discharged {
		t.Fatal("the tide must not discharge before the delivered effect is recoverably journaled")
	}
	st, err := loadWillReachState(reachPath)
	if err != nil {
		t.Fatalf("load reach state: %v", err)
	}
	if st.Pending == nil || !st.Pending.EffectPrepared || st.Pending.EffectRecorded ||
		st.Pending.Outcome != willOutcomePerceptionCommitted || st.Pending.EffectCount != 1 ||
		len(completeSartreJSONLines([]byte(st.Pending.EffectLine))) != 1 {
		t.Fatalf("pending reach must remember the prepared effect payload without respawn, got %#v", st.Pending)
	}

	sp2 := &fakeSpawner{}
	w2, f2 := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 0,
	}, sp2, &fakeSink{})
	w2.sink = emitOnlyFileSink{path: eventPath}
	w2.rootID = "rootabc"
	w2.reachStatePath = reachPath
	w2.learningStatePath = learningPath
	w2.pendingReach = st.Pending
	w2.nextReachSeq = st.NextSeq

	util, err = w2.tick(context.Background())
	if err != nil {
		t.Fatalf("retry should finish the same delivered effect without respawning: %v", err)
	}
	if util != willUtilPressure {
		t.Fatalf("retry util changed, got %q", util)
	}
	if sp2.calls != 0 {
		t.Fatalf("retry after delivered effect must not respawn the utility, calls=%d", sp2.calls)
	}
	if !f2.discharged {
		t.Fatal("retry must discharge the original tide")
	}
	data, err = os.ReadFile(eventPath)
	if err != nil {
		t.Fatalf("read retry effect delivery: %v", err)
	}
	if got := strings.Count(string(data), `"phase":"effect"`); got != 1 {
		t.Fatalf("retry must not append a duplicate effect, got %d effect lines: %s", got, data)
	}
	learning, err := loadWillLearningState(learningPath)
	if err != nil {
		t.Fatalf("load learning: %v", err)
	}
	if learning.LastOutcome != willOutcomePerceptionCommitted || learning.LastEffectCount != 1 {
		t.Fatalf("learning must preserve the delivered effect, got %#v", learning)
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
	if f.execFileN != 0 {
		t.Fatalf("committed pending retry must finish before a new physics breath, execFileN=%d", f.execFileN)
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

func TestWillTickMalformedUtilityOutputDoesNotCommitOrDischarge(t *testing.T) {
	sp := &fakeSpawner{line: []byte(`{"util":"repo_monitor","kind":"modified"`)}
	sk := &typedFakeSink{}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &sk.fakeSink)
	w.sink = sk

	util, err := w.tick(context.Background())
	if err == nil {
		t.Fatal("malformed utility stdout must fail closed instead of becoming no_novelty")
	}
	if util != willUtilPressure {
		t.Fatalf("got util %q", util)
	}
	if f.discharged {
		t.Error("malformed utility stdout must not spend the tide")
	}
	if sp.committed {
		t.Error("malformed utility stdout must not commit utility state")
	}
	if len(sk.lines) != 0 {
		t.Fatalf("malformed output must not emit effect records, got %q", sk.lines)
	}
	if len(sk.events) != 3 {
		t.Fatalf("want intention/act/learning sensor_error events, got %#v", sk.events)
	}
	learn := sk.events[2]
	if learn.Phase != "learning" || learn.Outcome != willOutcomeSensorError || learn.BytesCaptured != len(sp.line) {
		t.Fatalf("malformed output must become a typed sensor_error receipt, got %#v", learn)
	}
}

func TestWillTickIncompleteSartreEventDoesNotCommitOrDischarge(t *testing.T) {
	sp := &fakeSpawner{line: []byte(`{"util":"repo_monitor"}`)}
	sk := &typedFakeSink{}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.0, "pull_pressure": 0.3,
		"will_pressure_tide": 1.5,
	}, sp, &sk.fakeSink)
	w.sink = sk

	util, err := w.tick(context.Background())
	if err == nil {
		t.Fatal("incomplete SARTRE utility output must fail closed")
	}
	if util != willUtilPressure {
		t.Fatalf("got util %q", util)
	}
	if f.discharged {
		t.Error("incomplete SARTRE output must not spend the tide")
	}
	if sp.committed {
		t.Error("incomplete SARTRE output must not commit utility state")
	}
	if len(sk.lines) != 0 {
		t.Fatalf("incomplete output must not emit effect records, got %q", sk.lines)
	}
	if len(sk.events) != 3 {
		t.Fatalf("want intention/act/learning sensor_error events, got %#v", sk.events)
	}
	learn := sk.events[2]
	if learn.Phase != "learning" || learn.Outcome != willOutcomeSensorError || learn.BytesCaptured != len(sp.line) {
		t.Fatalf("incomplete output must become a typed sensor_error receipt, got %#v", learn)
	}
}
