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
	vars        map[string]float32
	execFileErr error
	execFileN   int
	discharged  bool
}

func (f *fakeWillField) ExecFile(string) error { f.execFileN++; return f.execFileErr }
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

func TestWillTickDischargeRefractory(t *testing.T) {
	sp, sk := &fakeSpawner{line: []byte(`{"util":"x"}`)}, &fakeSink{}
	w, f := newWill(map[string]float32{
		"will_threshold": 1.0, "will_gaze": 1.5, "pull_origin": 0.3, "pull_pressure": 0.0,
	}, sp, sk)
	if _, err := w.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !f.discharged || f.vars["will_gaze"] != 0 {
		t.Fatal("the first crest must discharge the tide to 0")
	}
	// Second tick: the fake ExecFile does not re-accumulate, so gaze stays 0 — no reach until the
	// tide re-gathers. (In the live field the confluence re-floods it over ~12 ticks.)
	sp.util = ""
	util, err := w.tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if util != "" {
		t.Errorf("a discharged tide must not reach again until it re-gathers, got %s", util)
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
