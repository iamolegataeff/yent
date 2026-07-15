package main

import (
	"context"
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
// metajanusAMLPath so the default does not depend on the working directory).
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

// willTickEvery is the will's cadence (YENT_WILL_TICK_SEC), default 2s.
func willTickEvery() time.Duration {
	if d := durationEnv("YENT_WILL_TICK_SEC"); d > 0 {
		return d
	}
	return 2 * time.Second
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

func (s osSpawner) Spawn(ctx context.Context, util string) ([]byte, error) {
	bin := filepath.Join(s.dir, util)
	cctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, willUtilArgs(util, s.root, s.stateDir)...)
	cmd.Stderr = os.Stderr // the utility's diagnostics pass through; only stdout is the event stream
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("run %s: %w", bin, err)
	}
	return out, nil
}

// willUtilArgs builds each utility's one-shot argv from its own CLI (they differ): both take
// --once and keep a --state file so a reach diffs against the last; repo_monitor scans --path
// <root>, whatdotheythinkiam reads --readme/--research under <root>. With no root the utility
// falls back to its own default paths.
func willUtilArgs(util, root, stateDir string) []string {
	args := []string{"--once", "--state", filepath.Join(stateDir, util+".state")}
	switch util {
	case willUtilPressure: // repo_monitor
		if root != "" {
			args = append(args, "--path", root)
		}
	case willUtilOrigin: // whatdotheythinkiam
		if root != "" {
			args = append(args, "--readme", filepath.Join(root, "README.md"), "--research", root)
		}
	}
	return args
}

// fileSink appends a utility's event line(s) to YENT_SARTRE_EVENTS — the same file the sartreSense
// reflex reads each ripple, so the reach's perception re-enters the field and shifts the next
// confluence (the spiral). An empty path or empty line is a silent no-op.
type fileSink struct{ path string }

func (s fileSink) Emit(line []byte) error {
	if s.path == "" || len(line) == 0 {
		return nil
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	body := append([]byte(strings.TrimRight(string(line), "\n")), '\n')
	_, err = f.Write(body)
	return err
}
