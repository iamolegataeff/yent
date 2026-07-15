package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeFakeUtil drops an executable that echoes the given line to stdout and exits 0 — a stand-in
// for the real Rust utility, so the spawn+capture path is tested without building the binaries.
func writeFakeUtil(t *testing.T, dir, name, stdout string) {
	t.Helper()
	script := "#!/bin/sh\nprintf '%s\\n' '" + stdout + "'\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake util: %v", err)
	}
}

func TestOsSpawnerCapturesStdout(t *testing.T) {
	dir := t.TempDir()
	line := `{"util":"whatdotheythinkiam","kind":"framing","reduced":2,"recognized":5}`
	writeFakeUtil(t, dir, willUtilOrigin, line)
	sp := osSpawner{dir: dir, stateDir: t.TempDir(), timeout: 5 * time.Second}
	out, err := sp.Spawn(context.Background(), willUtilOrigin)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !strings.Contains(string(out), `"util":"whatdotheythinkiam"`) {
		t.Errorf("the spawner must capture the utility's stdout JSON, got %q", out)
	}
}

func TestOsSpawnerMissingBinaryErrors(t *testing.T) {
	sp := osSpawner{dir: t.TempDir(), stateDir: t.TempDir(), timeout: 5 * time.Second}
	if _, err := sp.Spawn(context.Background(), willUtilPressure); err == nil {
		t.Error("a missing utility binary must error (fail-soft), not pass")
	}
}

func TestCapWriterBounds(t *testing.T) {
	cw := &capWriter{max: 10}
	if n, err := cw.Write([]byte("hello world this is long")); err != nil || n != 24 {
		t.Fatalf("Write must report the full length so the child never blocks, n=%d err=%v", n, err)
	}
	if got := cw.buf.String(); got != "hello worl" {
		t.Errorf("capWriter keeps only the first max bytes, got %q", got)
	}
	if n, _ := cw.Write([]byte("more")); n != 4 || cw.buf.Len() != 10 {
		t.Errorf("past the cap, writes are dropped but still reported, len=%d n=%d", cw.buf.Len(), n)
	}
}

func TestOsSpawnerCapsStdout(t *testing.T) {
	dir := t.TempDir()
	// a fake utility that floods stdout with more than the cap (1.1 MB > willMaxStdout)
	if err := os.WriteFile(filepath.Join(dir, willUtilOrigin), []byte("#!/bin/sh\nyes AAAA | head -c 1100000\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sp := osSpawner{dir: dir, stateDir: t.TempDir(), timeout: 10 * time.Second}
	out, err := sp.Spawn(context.Background(), willUtilOrigin)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if len(out) > willMaxStdout {
		t.Errorf("stdout must be capped at %d bytes, got %d", willMaxStdout, len(out))
	}
	if len(out) == 0 {
		t.Error("a flooding utility should still yield the captured head, got 0")
	}
}

func TestWillUtilArgs(t *testing.T) {
	// repo_monitor: --once --state <dir>/repo_monitor.state --path <root>
	a := strings.Join(willUtilArgs(willUtilPressure, "/repo", "/st"), " ")
	if !strings.Contains(a, "--once") || !strings.Contains(a, "--state /st/repo_monitor.state") || !strings.Contains(a, "--path /repo") {
		t.Errorf("repo_monitor args wrong: %q", a)
	}
	// whatdotheythinkiam: --readme <root>/README.md --research <root>
	b := strings.Join(willUtilArgs(willUtilOrigin, "/repo", "/st"), " ")
	if !strings.Contains(b, "--readme /repo/README.md") || !strings.Contains(b, "--research /repo") {
		t.Errorf("whatdotheythinkiam args wrong: %q", b)
	}
	// no root: just --once --state, no path flags to confuse the utility's parser
	c := strings.Join(willUtilArgs(willUtilPressure, "", "/st"), " ")
	if strings.Contains(c, "--path") {
		t.Errorf("no root must drop --path: %q", c)
	}
}

func TestFileSinkAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	sink := fileSink{path: path}
	if err := sink.Emit([]byte(`{"util":"a"}`)); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if err := sink.Emit([]byte(`{"util":"b"}` + "\n")); err != nil { // trailing newline normalized
		t.Fatalf("Emit: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 || lines[0] != `{"util":"a"}` || lines[1] != `{"util":"b"}` {
		t.Errorf("the sink must append one clean line per Emit, got %q", data)
	}
}

func TestFileSinkNoop(t *testing.T) {
	if err := (fileSink{path: ""}).Emit([]byte("x")); err != nil {
		t.Errorf("an empty path must be a silent no-op, got %v", err)
	}
	path := filepath.Join(t.TempDir(), "e.jsonl")
	if err := (fileSink{path: path}).Emit(nil); err != nil {
		t.Errorf("an empty line must be a silent no-op, got %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("an empty-line Emit must not create the file")
	}
}
