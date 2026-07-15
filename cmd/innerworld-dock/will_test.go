package main

import (
	"context"
	"errors"
	"testing"
)

// fakeWillField scripts the field's readings so the will logic is deterministic without cgo:
// ExecFile is a no-op counter (the "physics" is whatever the vars map says), GetVarFloat returns
// the scripted value, and Exec records the discharge.
type fakeWillField struct {
	vars           map[string]float32
	execFileErr    error
	execFileN      int
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
	if script == "will_gaze = 0" {
		f.discharged = true
		f.vars["will_gaze"] = 0
	}
	return nil
}

type fakeSpawner struct {
	util string // the last util asked for
	line []byte
	err  error
}

func (s *fakeSpawner) Spawn(_ context.Context, util string) ([]byte, error) {
	s.util = util
	return s.line, s.err
}

type fakeSink struct{ lines [][]byte }

func (s *fakeSink) Emit(line []byte) error {
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
		"pull_origin": 0.25, "pull_pressure": 0.0,
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
	if len(sk.lines) != 1 {
		t.Errorf("the perception must be emitted once, got %d", len(sk.lines))
	}
}

func TestWillTickReachesOnPressureCrest(t *testing.T) {
	sp, sk := &fakeSpawner{line: []byte(`{"util":"repo_monitor"}`)}, &fakeSink{}
	w, _ := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.6,
		"pull_origin": 0.0, "pull_pressure": 0.27,
	}, sp, sk)
	util, err := w.tick(context.Background())
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if util != willUtilPressure {
		t.Errorf("a pressure-dominant crest must reach %s, got %s", willUtilPressure, util)
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
	if !f.discharged {
		t.Error("the tide is spent even if the spawn failed — the will reached")
	}
	if len(sk.lines) != 0 {
		t.Error("nothing is emitted when the spawn failed")
	}
}
