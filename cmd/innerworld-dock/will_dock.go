package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		Line:     cw.buf.Bytes(),
		Overflow: cw.overflow,
		Commit: func() error {
			return os.Rename(pendingPath, statePath)
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
// the next confluence (the spiral). An empty path or empty line is a silent no-op.
type fileSink struct{ path string }

func (s fileSink) Emit(line []byte) error {
	if s.path == "" || len(line) == 0 {
		return nil
	}
	lines := completeSartreJSONLines(line)
	if len(lines) == 0 {
		return nil
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, body := range lines {
		if _, err := f.Write(append(body, '\n')); err != nil {
			return err
		}
	}
	return nil
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
		b, err := json.Marshal(obj)
		if err != nil {
			continue
		}
		out.Write(b)
		out.WriteByte('\n')
	}
	return out.Bytes()
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
	OriginTide        float32 `json:"will_origin_tide,omitempty"`
	PressureTide      float32 `json:"will_pressure_tide,omitempty"`
	RootID            string  `json:"root_id,omitempty"`
	Breath            int     `json:"breath,omitempty"`
	CadenceMS         int64   `json:"cadence_ms,omitempty"`
	RefractoryBreaths int     `json:"refractory_breaths,omitempty"`
	CooldownBreaths   int     `json:"cooldown_breaths,omitempty"`
	EffectCount       int     `json:"effect_count,omitempty"`
	BytesCaptured     int     `json:"bytes_captured,omitempty"`
	BytesLimit        int     `json:"bytes_limit,omitempty"`
}

func newWillEventID(util string, tide willTideSnapshot) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%.6f|%.6f|%.6f|%.6f|%.6f",
		util, time.Now().UnixNano(), tide.Gaze, tide.PullOrigin, tide.PullPressure,
		tide.OriginTide, tide.PressureTide)))
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
