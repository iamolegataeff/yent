package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// This file is the will loop's real wiring: how the dock finds the physics script, spawns the
// self-reading utility binaries, and feeds their perception back to sartreSense. The decision
// logic lives in will.go (willTicker); these are the OS hands it drives.

// willScriptPath resolves Janus/the_will_design.aml: YENT_WILL_AML if set, else the canonical file
// resolved relative to the executable, falling back to the CWD-relative path (mirrors
// metajanusAMLPath so the default does not depend on the working directory). The script must be
// READ-ONLY over the field — it only reads FIELD_F symbols and its own persistent globals. The
// dock reads C.am_get_state() for telemetry outside Body.mu, so a YENT_WILL_AML override that
// MUTATES a field directive could race a concurrent dream's telemetry read; the canonical script
// only reads, so there is no race by default.
func willScriptPath() string {
	if p := strings.TrimSpace(os.Getenv("YENT_WILL_AML")); p != "" {
		return p
	}
	const rel = "Janus/the_will_design.aml"
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), rel)
		if _, statErr := os.Stat(cand); statErr == nil {
			return cand
		}
	}
	return rel
}

// willTickEvery is the will's host cadence (YENT_WILL_TICK_SEC), default 500ms. The AML physics
// is breath-counted: retention and refractory are per will breath, not per wall-clock second.
// Changing this value changes how often breaths happen in real time; receipts record cadence_ms.
func willTickEvery() time.Duration {
	if d := durationEnv("YENT_WILL_TICK_SEC"); d > 0 {
		return d
	}
	return 500 * time.Millisecond
}

// willRefractoryTicks is how many will breaths the hand waits after a reach before it can reach
// again (YENT_WILL_REFRACTORY_TICKS), default 6 — long enough that even a sustained high-strain
// crest spaces its reaches out instead of firing every breath.
func willRefractoryTicks() int {
	if n := positiveIntEnv("YENT_WILL_REFRACTORY_TICKS"); n > 0 {
		return n
	}
	return 6
}

// willReachMaxAttempts bounds a single causal reach before the will records a terminal
// dead-letter consequence and lets the field continue. The dead letter is still a typed
// learning receipt; it is not a silent drop.
func willReachMaxAttempts() int {
	if n := positiveIntEnv("YENT_WILL_REACH_MAX_ATTEMPTS"); n > 0 {
		return n
	}
	return willReachMaxAttemptsDefault
}

// willReachTimeout bounds a single reach (YENT_WILL_REACH_SEC), default 5s, so a hung utility
// never stalls the will longer than one bounded wait.
func willReachTimeout() time.Duration {
	if d := durationEnv("YENT_WILL_REACH_SEC"); d > 0 {
		return d
	}
	return 5 * time.Second
}

// osSpawner runs a self-reading utility binary from dir, one-shot, under a timeout, and returns
// its stdout — 0..N SARTRE JSONL event lines (one per change the utility saw). The utility's
// diagnostics go to its own stderr (passed through), never mixed into the event stream.
type osSpawner struct {
	dir      string        // directory holding the built utility binaries
	root     string        // the repo root the utilities read about Yent (may be "")
	stateDir string        // where each utility keeps its --state file across reaches
	timeout  time.Duration // a reach cannot hang the will forever
}

func (s osSpawner) Spawn(ctx context.Context, util string) (willSpawnResult, error) {
	bin := filepath.Join(s.dir, util)
	statePath := filepath.Join(s.stateDir, util+".state")
	pendingPath := statePath + ".pending"
	if err := preparePendingWillState(statePath, pendingPath); err != nil {
		return willSpawnResult{}, err
	}
	cctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, willUtilArgsWithState(util, s.root, pendingPath)...)
	cw := &capWriter{max: willMaxStdout} // the timeout bounds time; this bounds bytes
	cmd.Stdout = cw
	cmd.Stderr = os.Stderr // the utility's diagnostics pass through; only stdout is the event stream
	if err := cmd.Run(); err != nil {
		return willSpawnResult{}, fmt.Errorf("run %s: %w", bin, err)
	}
	return willSpawnResult{
		Line:             cw.buf.Bytes(),
		Overflow:         cw.overflow,
		PendingStatePath: pendingPath,
		StatePath:        statePath,
		Commit: func() error {
			digest, err := willFileSHA256(pendingPath)
			if err != nil {
				return err
			}
			return publishPendingWillState(pendingPath, statePath, digest)
		},
	}, nil
}

func preparePendingWillState(statePath, pendingPath string) error {
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return err
	}
	if data, err := os.ReadFile(statePath); err == nil {
		return os.WriteFile(pendingPath, data, 0o644)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(pendingPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

const willLearningStateVersion = 1

type willLearningState struct {
	Version         int               `json:"version"`
	QuietRuns       int               `json:"quiet_runs"`
	LastReachID     string            `json:"last_reach_id,omitempty"`
	LastUtility     string            `json:"last_util,omitempty"`
	LastOutcome     string            `json:"last_outcome,omitempty"`
	LastEffectCount int               `json:"last_effect_count,omitempty"`
	LastCooldown    int               `json:"last_cooldown_breaths,omitempty"`
	CooldownBreaths int               `json:"cooldown_breaths,omitempty"`
	CurrentBreath   int               `json:"current_breath,omitempty"`
	LastBreath      int               `json:"last_breath,omitempty"`
	LastTide        *willTideSnapshot `json:"last_tide,omitempty"`
}

func willLearningStatePath(stateDir string) string {
	return filepath.Join(stateDir, "will-learning.state.json")
}

func loadWillLearningState(path string) (willLearningState, error) {
	st := willLearningState{Version: willLearningStateVersion}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return st, fmt.Errorf("empty learning state")
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, err
	}
	if st.Version != willLearningStateVersion {
		return st, fmt.Errorf("unsupported learning state version %d", st.Version)
	}
	if err := validateWillLearningState(st); err != nil {
		return st, err
	}
	return st, nil
}

func saveWillLearningState(path string, st willLearningState) error {
	if path == "" {
		return nil
	}
	if st.QuietRuns < 0 {
		st.QuietRuns = 0
	}
	if st.QuietRuns > willQuietRunMax {
		st.QuietRuns = willQuietRunMax
	}
	if st.CooldownBreaths < 0 {
		st.CooldownBreaths = 0
	}
	if st.CurrentBreath < 0 {
		st.CurrentBreath = 0
	}
	st.Version = willLearningStateVersion
	if err := validateWillLearningState(st); err != nil {
		return err
	}
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".pending"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if err := writeAll(f, data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return publishDurableFile(tmp, path)
}

func validateWillLearningState(st willLearningState) error {
	if st.QuietRuns < 0 || st.QuietRuns > willQuietRunMax {
		return fmt.Errorf("invalid quiet_runs %d", st.QuietRuns)
	}
	if st.CooldownBreaths < 0 {
		return fmt.Errorf("invalid current cooldown %d", st.CooldownBreaths)
	}
	if st.CurrentBreath < 0 {
		return fmt.Errorf("invalid current breath %d", st.CurrentBreath)
	}
	if st.LastOutcome == "" {
		if st.LastEffectCount != 0 {
			return fmt.Errorf("learning state has effect_count %d without outcome", st.LastEffectCount)
		}
		if st.LastCooldown < 0 {
			return fmt.Errorf("invalid last cooldown %d", st.LastCooldown)
		}
		return nil
	}
	if strings.TrimSpace(st.LastReachID) == "" {
		return fmt.Errorf("learning state has outcome %q without reach id", st.LastOutcome)
	}
	if st.LastUtility != willUtilOrigin && st.LastUtility != willUtilPressure {
		return fmt.Errorf("learning state has unknown utility %q", st.LastUtility)
	}
	if !validWillCommittedOutcome(st.LastOutcome, st.LastEffectCount) {
		return fmt.Errorf("learning state has invalid outcome %q with %d effects", st.LastOutcome, st.LastEffectCount)
	}
	if st.LastCooldown < 0 {
		return fmt.Errorf("invalid last cooldown %d", st.LastCooldown)
	}
	if st.LastBreath < 0 {
		return fmt.Errorf("invalid last breath %d", st.LastBreath)
	}
	if st.LastTide != nil && !finiteWillTide(*st.LastTide) {
		return fmt.Errorf("learning state has non-finite last tide")
	}
	return nil
}

const willReachStateVersion = 1

type willPendingReach struct {
	Seq                  int64            `json:"seq"`
	ID                   string           `json:"id"`
	Utility              string           `json:"util"`
	Tide                 willTideSnapshot `json:"tide"`
	Breath               int              `json:"breath"`
	Attempts             int              `json:"attempts,omitempty"`
	LastFailureOutcome   string           `json:"last_failure_outcome,omitempty"`
	EffectRecorded       bool             `json:"effect_recorded,omitempty"`
	BaselineCommitted    bool             `json:"baseline_committed,omitempty"`
	BaselinePendingPath  string           `json:"baseline_pending_path,omitempty"`
	BaselineStatePath    string           `json:"baseline_state_path,omitempty"`
	BaselineSHA256       string           `json:"baseline_sha256,omitempty"`
	ConsequenceCommitted bool             `json:"consequence_committed,omitempty"`
	Outcome              string           `json:"outcome,omitempty"`
	EffectCount          int              `json:"effect_count,omitempty"`
}

type willReachState struct {
	Version int               `json:"version"`
	NextSeq int64             `json:"next_seq"`
	Pending *willPendingReach `json:"pending,omitempty"`
}

func willReachStatePath(stateDir string) string {
	return filepath.Join(stateDir, "will-reach.state.json")
}

func loadWillReachState(path string) (willReachState, error) {
	st := willReachState{Version: willReachStateVersion, NextSeq: 1}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return st, fmt.Errorf("empty reach state")
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, err
	}
	if err := validateWillReachState(st); err != nil {
		return st, err
	}
	return st, nil
}

func saveWillReachState(path string, st willReachState) error {
	if path == "" {
		return nil
	}
	st.Version = willReachStateVersion
	if err := validateWillReachState(st); err != nil {
		return err
	}
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".pending"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if err := writeAll(f, data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return publishDurableFile(tmp, path)
}

func validateWillReachState(st willReachState) error {
	if st.Version != willReachStateVersion {
		return fmt.Errorf("unsupported reach state version %d", st.Version)
	}
	if st.NextSeq <= 0 {
		return fmt.Errorf("invalid next reach sequence %d", st.NextSeq)
	}
	if st.Pending == nil {
		return nil
	}
	p := st.Pending
	if p.Seq <= 0 || p.Seq != st.NextSeq {
		return fmt.Errorf("invalid pending reach sequence %d for next %d", p.Seq, st.NextSeq)
	}
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("pending reach has empty id")
	}
	if p.Utility != willUtilOrigin && p.Utility != willUtilPressure {
		return fmt.Errorf("pending reach has unknown utility %q", p.Utility)
	}
	if !finiteWillTide(p.Tide) {
		return fmt.Errorf("pending reach has non-finite tide")
	}
	if p.EffectCount < 0 {
		return fmt.Errorf("pending reach has negative effect count %d", p.EffectCount)
	}
	if p.Breath < 0 {
		return fmt.Errorf("pending reach has negative breath %d", p.Breath)
	}
	if p.Attempts < 0 {
		return fmt.Errorf("pending reach has negative attempts %d", p.Attempts)
	}
	if p.Attempts > 0 && !willFailureOutcome(p.LastFailureOutcome) && !p.ConsequenceCommitted && !p.EffectRecorded {
		return fmt.Errorf("pending reach has attempts without a typed failure outcome %q", p.LastFailureOutcome)
	}
	if p.LastFailureOutcome != "" && !willFailureOutcome(p.LastFailureOutcome) {
		return fmt.Errorf("pending reach has invalid failure outcome %q", p.LastFailureOutcome)
	}
	if p.BaselineCommitted && !p.EffectRecorded && !p.ConsequenceCommitted {
		return fmt.Errorf("pending reach has committed baseline without a recorded effect stage")
	}
	if p.ConsequenceCommitted {
		if !validWillCommittedOutcome(p.Outcome, p.EffectCount) {
			return fmt.Errorf("pending reach has invalid committed outcome %q with %d effects", p.Outcome, p.EffectCount)
		}
		if p.Outcome == willOutcomeDeadLetter {
			if p.Attempts <= 0 {
				return fmt.Errorf("pending reach has dead-letter without attempts")
			}
			if !willFailureOutcome(p.LastFailureOutcome) {
				return fmt.Errorf("pending reach has dead-letter without a typed failure outcome")
			}
		}
		return nil
	}
	if p.EffectRecorded {
		if !validWillCommittedOutcome(p.Outcome, p.EffectCount) || p.Outcome == willOutcomeDeadLetter {
			return fmt.Errorf("pending reach has invalid recorded outcome %q with %d effects", p.Outcome, p.EffectCount)
		}
		if strings.TrimSpace(p.BaselineStatePath) == "" {
			if p.BaselineCommitted {
				return nil
			}
			return fmt.Errorf("pending reach has no utility baseline state path")
		}
		if !p.BaselineCommitted {
			if strings.TrimSpace(p.BaselinePendingPath) == "" {
				return fmt.Errorf("pending reach has no utility baseline pending path")
			}
			if !validSHA256Hex(p.BaselineSHA256) {
				return fmt.Errorf("pending reach has invalid utility baseline sha256")
			}
		}
		return nil
	}
	if p.Outcome != "" || p.EffectCount != 0 {
		return fmt.Errorf("pending reach has uncommitted outcome %q with %d effects", p.Outcome, p.EffectCount)
	}
	return nil
}

func validSHA256Hex(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
			continue
		}
		return false
	}
	return true
}

func validWillCommittedOutcome(outcome string, effectCount int) bool {
	switch outcome {
	case willOutcomeNoNovelty:
		return effectCount == 0
	case willOutcomePerceptionCommitted:
		return effectCount > 0
	case willOutcomeDeadLetter:
		return effectCount == 0
	default:
		return false
	}
}

func willFailureOutcome(outcome string) bool {
	switch strings.TrimSpace(outcome) {
	case willOutcomeSensorError, willOutcomeStateError, willOutcomeOverflow:
		return true
	default:
		return false
	}
}

func finiteWillTide(t willTideSnapshot) bool {
	for _, v := range []float32{
		t.Threshold, t.Gaze,
		t.PullOrigin, t.PullPressure, t.PullCuriosity, t.PullCare, t.PullBoundary,
		t.OriginTide, t.PressureTide, t.CuriosityTide, t.CareTide, t.BoundaryTide,
	} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
	}
	return true
}

// willMaxStdout caps how much of a utility's stdout the will buffers. A first scan over a large
// root can emit one JSON record per eligible file, so an unbounded buffer could reach hundreds of
// MB; 1 MiB is far more than any real reach's event stream and keeps memory bounded.
const willMaxStdout = 1 << 20

// capWriter buffers up to max bytes and marks overflow while reporting the full write, so the
// child never blocks on a full pipe. The caller must not commit utility state when overflow is set.
type capWriter struct {
	buf      bytes.Buffer
	max      int
	overflow bool
}

func (w *capWriter) Write(p []byte) (int, error) {
	if rem := w.max - w.buf.Len(); rem > 0 {
		if len(p) > rem {
			w.buf.Write(p[:rem])
			w.overflow = true
		} else {
			w.buf.Write(p)
		}
	} else if len(p) > 0 {
		w.overflow = true
	}
	return len(p), nil
}

// willUtilArgs builds each utility's one-shot argv from its own CLI (they differ): both take
// --once and keep a --state file so a reach diffs against the last; repo_monitor scans --path
// <root>, whatdotheythinkiam reads --readme <root>/README.md and --research <root>/research.
// Production wiring passes a canonical root resolved by resolveWillRoot; an empty root here
// (tests) drops the path flags, which is repo_monitor's silent-no-op case.
func willUtilArgs(util, root, stateDir string) []string {
	return willUtilArgsWithState(util, root, filepath.Join(stateDir, util+".state"))
}

func willUtilArgsWithState(util, root, statePath string) []string {
	args := []string{"--once", "--state", statePath}
	switch util {
	case willUtilPressure: // repo_monitor
		if root != "" {
			args = append(args, "--path", root)
		}
	case willUtilOrigin: // whatdotheythinkiam
		if root != "" {
			args = append(args, "--readme", filepath.Join(root, "README.md"), "--research", filepath.Join(root, "research"))
		}
	}
	return args
}

// fileSink appends complete JSONL utility events to YENT_SARTRE_EVENTS — the same file the
// sartreSense reflex reads each ripple, so the reach's perception re-enters the field and shifts
// the next confluence (the spiral). An empty line is a silent no-op; a non-empty event without a
// path is a delivery error, because the will must not commit sensor state into the void.
type fileSink struct{ path string }

func (s fileSink) Emit(line []byte) error {
	if len(line) == 0 {
		return nil
	}
	lines := completeSartreJSONLines(line)
	if len(lines) == 0 {
		return nil
	}
	if s.path == "" {
		return fmt.Errorf("SARTRE event sink path is empty")
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
	}()
	for _, body := range lines {
		if err := writeAll(f, append(body, '\n')); err != nil {
			return err
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	closed = true
	return syncParentDir(s.path)
}

func writeAll(w io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := w.Write(p)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		p = p[n:]
	}
	return nil
}

func syncFilePath(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func syncParentDir(path string) error {
	f, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		// Some filesystems reject directory fsync. The file itself has already
		// been fsynced; keep those platforms usable instead of turning a
		// durability hardening pass into an unrelated portability failure.
		if errors.Is(err, syscall.EINVAL) {
			return nil
		}
		return err
	}
	return nil
}

func willFileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func publishPendingWillState(pendingPath, statePath, wantSHA256 string) error {
	if strings.TrimSpace(pendingPath) == "" || strings.TrimSpace(statePath) == "" {
		return fmt.Errorf("missing pending or canonical utility state path")
	}
	verifyState := func() error {
		if _, err := os.Stat(statePath); err != nil {
			return err
		}
		if wantSHA256 == "" {
			return nil
		}
		got, err := willFileSHA256(statePath)
		if err != nil {
			return err
		}
		if got != wantSHA256 {
			return fmt.Errorf("published utility state hash mismatch: got %s want %s", got, wantSHA256)
		}
		return nil
	}
	if wantSHA256 != "" {
		got, err := willFileSHA256(pendingPath)
		if err != nil {
			if os.IsNotExist(err) {
				return verifyState()
			}
			return err
		}
		if got != wantSHA256 {
			return fmt.Errorf("pending utility state hash mismatch: got %s want %s", got, wantSHA256)
		}
	} else if _, err := os.Stat(pendingPath); err != nil {
		if os.IsNotExist(err) {
			return verifyState()
		}
		return err
	}
	if err := syncFilePath(pendingPath); err != nil {
		if os.IsNotExist(err) {
			return verifyState()
		}
		return err
	}
	if err := os.Rename(pendingPath, statePath); err != nil {
		return err
	}
	if err := syncParentDir(statePath); err != nil {
		if verifyErr := verifyState(); verifyErr == nil {
			return nil
		}
		return err
	}
	return nil
}

func publishDurableFile(tmp, path string) error {
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return syncParentDir(path)
}

func (s fileSink) EmitEvent(ev willEvent) error {
	if ev.Timestamp == 0 {
		ev.Timestamp = time.Now().Unix()
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return s.Emit(b)
}

func completeSartreJSONLines(raw []byte) [][]byte {
	var out [][]byte
	for _, part := range bytes.Split(raw, []byte{'\n'}) {
		line := bytes.TrimSpace(part)
		if len(line) == 0 || line[0] != '{' || !json.Valid(line) {
			continue
		}
		out = append(out, append([]byte(nil), line...))
	}
	return out
}

func hasNonEmptySartreOutput(raw []byte) bool {
	return len(bytes.TrimSpace(raw)) > 0
}

func tagSartreEffectLines(raw []byte, eventID, rootID string) []byte {
	var out bytes.Buffer
	for _, line := range completeSartreJSONLines(raw) {
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			continue
		}
		if _, ok := obj["id"]; !ok && eventID != "" {
			obj["id"] = eventID
		}
		if _, ok := obj["root_id"]; !ok && rootID != "" {
			obj["root_id"] = rootID
		}
		obj["phase"] = "effect"
		if !validWillEffectObject(obj) {
			continue
		}
		b, err := json.Marshal(obj)
		if err != nil {
			continue
		}
		out.Write(b)
		out.WriteByte('\n')
	}
	return out.Bytes()
}

func validWillEffectObject(obj map[string]any) bool {
	util, _ := obj["util"].(string)
	switch strings.TrimSpace(util) {
	case willUtilPressure:
		kind, _ := obj["kind"].(string)
		path, _ := obj["path"].(string)
		return sartreRepoChangeKind(kind) && strings.TrimSpace(path) != ""
	case willUtilOrigin:
		if willPositiveJSONNumber(obj["reduced"]) || willPositiveJSONNumber(obj["recognized"]) {
			return true
		}
		kind, _ := obj["kind"].(string)
		path, _ := obj["path"].(string)
		return sartreRepoChangeKind(kind) && strings.TrimSpace(path) != ""
	default:
		return false
	}
}

func willPositiveJSONNumber(v any) bool {
	switch n := v.(type) {
	case float64:
		return n > 0
	case float32:
		return n > 0
	case int:
		return n > 0
	case int64:
		return n > 0
	case json.Number:
		f, err := n.Float64()
		return err == nil && f > 0
	default:
		return false
	}
}

type willEvent struct {
	ID                string  `json:"id,omitempty"`
	Phase             string  `json:"phase,omitempty"`
	Outcome           string  `json:"outcome,omitempty"`
	Utility           string  `json:"util"`
	Kind              string  `json:"kind,omitempty"`
	Path              string  `json:"path,omitempty"`
	Timestamp         int64   `json:"ts,omitempty"`
	Gaze              float32 `json:"will_gaze,omitempty"`
	Threshold         float32 `json:"will_threshold,omitempty"`
	PullOrigin        float32 `json:"pull_origin,omitempty"`
	PullPressure      float32 `json:"pull_pressure,omitempty"`
	PullCuriosity     float32 `json:"pull_curiosity,omitempty"`
	PullCare          float32 `json:"pull_care,omitempty"`
	PullBoundary      float32 `json:"pull_boundary,omitempty"`
	OriginTide        float32 `json:"will_origin_tide,omitempty"`
	PressureTide      float32 `json:"will_pressure_tide,omitempty"`
	CuriosityTide     float32 `json:"will_curiosity_tide,omitempty"`
	CareTide          float32 `json:"will_care_tide,omitempty"`
	BoundaryTide      float32 `json:"will_boundary_tide,omitempty"`
	RootID            string  `json:"root_id,omitempty"`
	Breath            int     `json:"breath,omitempty"`
	CadenceMS         int64   `json:"cadence_ms,omitempty"`
	RefractoryBreaths int     `json:"refractory_breaths,omitempty"`
	CooldownBreaths   int     `json:"cooldown_breaths,omitempty"`
	EffectCount       int     `json:"effect_count,omitempty"`
	BytesCaptured     int     `json:"bytes_captured,omitempty"`
	BytesLimit        int     `json:"bytes_limit,omitempty"`
	Attempts          int     `json:"attempts,omitempty"`
	FailureOutcome    string  `json:"failure_outcome,omitempty"`
}

func newWillEventID(rootID string, seq int64, util string, tide willTideSnapshot) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s|%.6f|%.6f|%.6f|%.6f|%.6f|%.6f|%.6f|%.6f|%.6f|%.6f|%.6f",
		rootID, seq, util, tide.Gaze, tide.PullOrigin, tide.PullPressure,
		tide.PullCuriosity, tide.PullCare, tide.PullBoundary, tide.OriginTide,
		tide.PressureTide, tide.CuriosityTide, tide.CareTide, tide.BoundaryTide)))
	return hex.EncodeToString(sum[:8])
}

func willStateDir(root string) string {
	base := strings.TrimSpace(os.Getenv("YENT_WILL_STATE_DIR"))
	if base == "" {
		base = filepath.Join(os.TempDir(), "yent-will-state")
	}
	return filepath.Join(base, willStateNamespace(root))
}

func willStateNamespace(root string) string {
	organism := safeWillNamespacePart(willOrganismID())
	rootID := willRootID(root)
	cfgID := willSensorConfigID(root)
	return fmt.Sprintf("org-%s-root-%s-cfg-%s", organism, rootID, cfgID)
}

func willOrganismID() string {
	for _, name := range []string{"YENT_WILL_ORGANISM_ID", "YENT_ORGANISM_ID"} {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v
		}
	}
	return "yent"
}

func willRootID(root string) string {
	sum := sha256.Sum256([]byte(canonicalWillPath(root)))
	return hex.EncodeToString(sum[:8])
}

func willSensorConfigID(root string) string {
	croot := canonicalWillPath(root)
	cfg := strings.Join([]string{
		"will-state:v2",
		"repo_monitor:root:" + croot + ":ext=.md,.txt,.rs,.c,.h,.go,.json",
		"whatdotheythinkiam:readme:" + filepath.Join(croot, "README.md") + ":research:" + filepath.Join(croot, "research") + ":ext=.md,.txt",
	}, "\n")
	sum := sha256.Sum256([]byte(cfg))
	return hex.EncodeToString(sum[:8])
}

func safeWillNamespacePart(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "yent"
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
		if b.Len() >= 48 {
			break
		}
	}
	if b.Len() == 0 {
		return "yent"
	}
	return b.String()
}

func resolveWillRoot(raw string) (string, error) {
	if root := strings.TrimSpace(raw); root != "" {
		return canonicalWillPath(root), nil
	}
	if exe, err := os.Executable(); err == nil {
		if root, ok := findWillRepoRoot(filepath.Dir(exe)); ok {
			return root, nil
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		if root, ok := findWillRepoRoot(cwd); ok {
			return root, nil
		}
	}
	return "", fmt.Errorf("YENT_WILL_ROOT is unset and no Yent repo root was found from executable or cwd")
}

func findWillRepoRoot(start string) (string, bool) {
	root := canonicalWillPath(start)
	for i := 0; i < 8; i++ {
		if isWillRepoRoot(root) {
			return root, true
		}
		parent := filepath.Dir(root)
		if parent == root {
			break
		}
		root = parent
	}
	return "", false
}

func isWillRepoRoot(root string) bool {
	for _, rel := range []string{"README.md", "Janus/the_will_design.aml", "sartre/utils/repo_monitor", "sartre/utils/whatdotheythinkiam"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			return false
		}
	}
	return true
}

func canonicalWillPath(path string) string {
	if path == "" {
		return "none"
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	path = filepath.Clean(path)
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	return filepath.Clean(path)
}
