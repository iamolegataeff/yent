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

// willTickEvery is the will's cadence (YENT_WILL_TICK_SEC), default 500ms — the tide needs ~6
// ticks to crest from rest, so a slower cadence would never fire inside a short-lived dock run.
func willTickEvery() time.Duration {
	if d := durationEnv("YENT_WILL_TICK_SEC"); d > 0 {
		return d
	}
	return 500 * time.Millisecond
}

// willRefractoryTicks is how many ticks the will waits after a reach before it can reach again
// (YENT_WILL_REFRACTORY_TICKS), default 6 — long enough that even a sustained high-strain crest
// spaces its reaches out instead of firing every tick.
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
// <root>, whatdotheythinkiam reads --readme/--research under <root>. The dock always supplies a
// root (default: its working directory), because repo_monitor scans NOTHING without --path — so
// a pressure crest must never resolve to an empty scan. An empty root here (tests) drops the path
// flags, which is repo_monitor's silent-no-op case, exactly what the wiring's default prevents.
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

func tagSartreEffectLines(raw []byte, eventID string) []byte {
	var out bytes.Buffer
	for _, line := range completeSartreJSONLines(raw) {
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			continue
		}
		if _, ok := obj["id"]; !ok && eventID != "" {
			obj["id"] = eventID
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
	ID            string  `json:"id,omitempty"`
	Phase         string  `json:"phase,omitempty"`
	Outcome       string  `json:"outcome,omitempty"`
	Utility       string  `json:"util"`
	Kind          string  `json:"kind,omitempty"`
	Path          string  `json:"path,omitempty"`
	Timestamp     int64   `json:"ts,omitempty"`
	Gaze          float32 `json:"will_gaze,omitempty"`
	Threshold     float32 `json:"will_threshold,omitempty"`
	PullOrigin    float32 `json:"pull_origin,omitempty"`
	PullPressure  float32 `json:"pull_pressure,omitempty"`
	OriginTide    float32 `json:"will_origin_tide,omitempty"`
	PressureTide  float32 `json:"will_pressure_tide,omitempty"`
	EffectCount   int     `json:"effect_count,omitempty"`
	BytesCaptured int     `json:"bytes_captured,omitempty"`
	BytesLimit    int     `json:"bytes_limit,omitempty"`
}

func newWillEventID(util string, tide willTideSnapshot) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%.6f|%.6f|%.6f|%.6f|%.6f",
		util, time.Now().UnixNano(), tide.Gaze, tide.PullOrigin, tide.PullPressure,
		tide.OriginTide, tide.PressureTide)))
	return hex.EncodeToString(sum[:8])
}

func willStateDir(root string) string {
	if stateDir := strings.TrimSpace(os.Getenv("YENT_WILL_STATE_DIR")); stateDir != "" {
		return stateDir
	}
	key := "none"
	if root != "" {
		if abs, err := filepath.Abs(root); err == nil {
			root = abs
		}
		keyBytes := sha256.Sum256([]byte(filepath.Clean(root)))
		key = hex.EncodeToString(keyBytes[:8])
	}
	return filepath.Join(os.TempDir(), "yent-will-"+key)
}
