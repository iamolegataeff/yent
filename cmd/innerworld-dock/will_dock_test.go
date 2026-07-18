package main

import (
	"context"
	"os"
	osexec "os/exec"
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

func writeFakeStateUtil(t *testing.T, dir, name, stdout string) {
	t.Helper()
	script := "#!/bin/sh\n" +
		"while [ \"$#\" -gt 0 ]; do\n" +
		"  if [ \"$1\" = \"--state\" ]; then shift; printf 'state\\n' > \"$1\"; fi\n" +
		"  shift || break\n" +
		"done\n" +
		"printf '%s\\n' '" + stdout + "'\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake state util: %v", err)
	}
}

func TestOsSpawnerCapturesStdout(t *testing.T) {
	dir := t.TempDir()
	line := `{"util":"whatdotheythinkiam","kind":"framing","reduced":2,"recognized":5}`
	writeFakeUtil(t, dir, willUtilOrigin, line)
	sp := osSpawner{dir: dir, stateDir: t.TempDir(), timeout: 5 * time.Second}
	result, err := sp.Spawn(context.Background(), willUtilOrigin)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !strings.Contains(string(result.Line), `"util":"whatdotheythinkiam"`) {
		t.Errorf("the spawner must capture the utility's stdout JSON, got %q", result.Line)
	}
	if result.Commit == nil {
		t.Fatal("spawner must return a pending state commit")
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
	if !cw.overflow {
		t.Error("capWriter must mark overflow when bytes are dropped")
	}
	if n, _ := cw.Write([]byte("more")); n != 4 || cw.buf.Len() != 10 {
		t.Errorf("past the cap, writes are dropped but still reported, len=%d n=%d", cw.buf.Len(), n)
	}
	if !cw.overflow {
		t.Error("capWriter must keep overflow marked after later writes")
	}
}

func TestOsSpawnerCapsStdout(t *testing.T) {
	dir := t.TempDir()
	// a fake utility that floods stdout with more than the cap (1.1 MB > willMaxStdout)
	if err := os.WriteFile(filepath.Join(dir, willUtilOrigin), []byte("#!/bin/sh\nyes AAAA | head -c 1100000\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sp := osSpawner{dir: dir, stateDir: t.TempDir(), timeout: 10 * time.Second}
	result, err := sp.Spawn(context.Background(), willUtilOrigin)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if len(result.Line) > willMaxStdout {
		t.Errorf("stdout must be capped at %d bytes, got %d", willMaxStdout, len(result.Line))
	}
	if len(result.Line) == 0 {
		t.Error("a flooding utility should still yield the captured head, got 0")
	}
	if !result.Overflow {
		t.Error("a flooding utility must report overflow so state is not committed silently")
	}
}

func TestOsSpawnerCommitsPendingStateOnlyWhenAsked(t *testing.T) {
	dir := t.TempDir()
	stateDir := t.TempDir()
	writeFakeStateUtil(t, dir, willUtilPressure, `{"util":"repo_monitor","kind":"added","path":"a.md"}`)
	sp := osSpawner{dir: dir, root: t.TempDir(), stateDir: stateDir, timeout: 5 * time.Second}
	result, err := sp.Spawn(context.Background(), willUtilPressure)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	statePath := filepath.Join(stateDir, willUtilPressure+".state")
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("canonical state must not be published before commit, err=%v", err)
	}
	if err := result.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("canonical state should be published after commit: %v", err)
	}
}

func TestPublishPendingWillStateVerifiesRecordedHash(t *testing.T) {
	dir := t.TempDir()
	pendingPath := filepath.Join(dir, "repo_monitor.state.pending")
	statePath := filepath.Join(dir, "repo_monitor.state")
	if err := os.WriteFile(pendingPath, []byte("new-baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest, err := willFileSHA256(pendingPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := publishPendingWillState(pendingPath, statePath, digest); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := publishPendingWillState(pendingPath, statePath, digest); err != nil {
		t.Fatalf("missing pending with matching canonical hash should be recoverable: %v", err)
	}
	if err := os.WriteFile(statePath, []byte("old-baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := publishPendingWillState(pendingPath, statePath, digest); err == nil {
		t.Fatal("missing pending with stale canonical state must not be treated as committed")
	}
}

func TestWillUtilArgs(t *testing.T) {
	// repo_monitor: --once --state <dir>/repo_monitor.state --path <root>
	a := strings.Join(willUtilArgs(willUtilPressure, "/repo", "/st"), " ")
	if !strings.Contains(a, "--once") || !strings.Contains(a, "--state /st/repo_monitor.state") || !strings.Contains(a, "--path /repo") {
		t.Errorf("repo_monitor args wrong: %q", a)
	}
	// whatdotheythinkiam: --readme <root>/README.md --research <root>/research
	b := strings.Join(willUtilArgs(willUtilOrigin, "/repo", "/st"), " ")
	if !strings.Contains(b, "--readme /repo/README.md") || !strings.Contains(b, "--research /repo/research") {
		t.Errorf("whatdotheythinkiam args wrong: %q", b)
	}
	// no root: just --once --state, no path flags to confuse the utility's parser
	c := strings.Join(willUtilArgs(willUtilPressure, "", "/st"), " ")
	if strings.Contains(c, "--path") {
		t.Errorf("no root must drop --path: %q", c)
	}
}

func TestFindWillRepoRootFromSubdir(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"README.md",
		"Janus/the_will_design.aml",
		"sartre/utils/repo_monitor/.keep",
		"sartre/utils/whatdotheythinkiam/.keep",
		"cmd/innerworld-dock/.keep",
	} {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, ok := findWillRepoRoot(filepath.Join(root, "cmd", "innerworld-dock"))
	if !ok {
		t.Fatal("expected repo root to be found from a nested start path")
	}
	if got != canonicalWillPath(root) {
		t.Fatalf("wrong root\ngot:  %s\nwant: %s", got, canonicalWillPath(root))
	}
}

func TestWillStateDirNamespacesByRootAndOrganism(t *testing.T) {
	base := t.TempDir()
	t.Setenv("YENT_WILL_STATE_DIR", base)
	t.Setenv("YENT_WILL_ORGANISM_ID", "Yent Prime")
	root1 := filepath.Join(t.TempDir(), "repo-a")
	root2 := filepath.Join(t.TempDir(), "repo-b")
	if err := os.MkdirAll(root1, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root2, 0o755); err != nil {
		t.Fatal(err)
	}
	a := willStateDir(root1)
	b := willStateDir(root2)
	if filepath.Dir(a) != base || filepath.Dir(b) != base {
		t.Fatalf("YENT_WILL_STATE_DIR must be treated as a base dir, got %q / %q", a, b)
	}
	if a == b {
		t.Fatalf("different canonical roots must not share state dir: %q", a)
	}
	if !strings.Contains(filepath.Base(a), "org-yent_prime-root-") {
		t.Fatalf("state dir must include a sanitized organism id and root hash, got %q", a)
	}
	if !strings.Contains(filepath.Base(a), "-cfg-") {
		t.Fatalf("state dir must include sensor config namespace, got %q", a)
	}
}

func TestWillNamespaceOwnerLockRejectsSecondProcess(t *testing.T) {
	stateDir := t.TempDir()
	owner, err := acquireWillNamespaceOwner(stateDir)
	if err != nil {
		t.Fatalf("acquire owner: %v", err)
	}
	defer owner.Close()

	cmd := willNamespaceOwnerHelperCommand(stateDir)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("second process acquired an occupied will namespace, output=%s", out)
	}
	if !strings.Contains(string(out), "already has an active owner") {
		t.Fatalf("second process must fail with owner diagnostic, err=%v output=%s", err, out)
	}

	if err := owner.Close(); err != nil {
		t.Fatalf("release owner: %v", err)
	}
	out, err = willNamespaceOwnerHelperCommand(stateDir).CombinedOutput()
	if err != nil {
		t.Fatalf("released namespace should be claimable, err=%v output=%s", err, out)
	}
}

func willNamespaceOwnerHelperCommand(stateDir string) *osexec.Cmd {
	cmd := osexec.Command(os.Args[0], "-test.run=^TestWillNamespaceOwnerLockHelper$")
	cmd.Env = append(os.Environ(), "YENT_WILL_LOCK_HELPER=1", "YENT_WILL_LOCK_HELPER_DIR="+stateDir)
	return cmd
}

func TestWillNamespaceOwnerLockHelper(t *testing.T) {
	if os.Getenv("YENT_WILL_LOCK_HELPER") != "1" {
		return
	}
	owner, err := acquireWillNamespaceOwner(os.Getenv("YENT_WILL_LOCK_HELPER_DIR"))
	if err != nil {
		t.Fatalf("helper lock: %v", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("helper unlock: %v", err)
	}
}

func TestWillLearningStateRoundTrip(t *testing.T) {
	path := willLearningStatePath(t.TempDir())
	if st, err := loadWillLearningState(path); err != nil || st.QuietRuns != 0 {
		t.Fatalf("missing learning state should load as zero state, st=%#v err=%v", st, err)
	}
	if err := saveWillLearningState(path, willLearningState{
		QuietRuns:       3,
		LastReachID:     "reach3",
		LastUtility:     willUtilPressure,
		LastOutcome:     willOutcomeNoNovelty,
		LastEffectCount: 0,
		LastCooldown:    5,
		CooldownBreaths: 4,
		CurrentBreath:   13,
		LastBreath:      12,
		LastTide:        &willTideSnapshot{Threshold: 1, Gaze: 1.5, PressureTide: 1.5},
	}); err != nil {
		t.Fatalf("save learning state: %v", err)
	}
	st, err := loadWillLearningState(path)
	if err != nil {
		t.Fatalf("load learning state: %v", err)
	}
	if st.Version != willLearningStateVersion || st.QuietRuns != 3 ||
		st.LastReachID != "reach3" || st.LastOutcome != willOutcomeNoNovelty ||
		st.LastCooldown != 5 || st.CooldownBreaths != 4 || st.CurrentBreath != 13 || st.LastBreath != 12 || st.LastTide == nil ||
		st.LastTide.PressureTide != 1.5 {
		t.Fatalf("wrong learning state after round-trip: %#v", st)
	}
}

func TestWillLearningStateRejectsCorruptQuietRuns(t *testing.T) {
	path := willLearningStatePath(t.TempDir())
	if err := os.WriteFile(path, []byte(`{"version":1,"quiet_runs":-1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadWillLearningState(path); err == nil {
		t.Fatal("negative quiet_runs must fail loud instead of resetting the will silently")
	}
}

func TestWillLearningStateRejectsCorruptCooldown(t *testing.T) {
	path := willLearningStatePath(t.TempDir())
	if err := os.WriteFile(path, []byte(`{"version":1,"quiet_runs":0,"cooldown_breaths":-1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadWillLearningState(path); err == nil {
		t.Fatal("negative current cooldown must fail loud instead of resetting refractory silently")
	}
}

func TestWillLearningStateRejectsCorruptCurrentBreath(t *testing.T) {
	path := willLearningStatePath(t.TempDir())
	if err := os.WriteFile(path, []byte(`{"version":1,"quiet_runs":0,"current_breath":-1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadWillLearningState(path); err == nil {
		t.Fatal("negative current breath must fail loud instead of resetting the will clock silently")
	}
}

func TestWillLearningStateRejectsInvalidConsequence(t *testing.T) {
	path := willLearningStatePath(t.TempDir())
	if err := os.WriteFile(path, []byte(`{"version":1,"quiet_runs":0,"last_reach_id":"x","last_util":"repo_monitor","last_outcome":"perception_committed","last_effect_count":0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadWillLearningState(path); err == nil {
		t.Fatal("learning state must reject impossible typed consequences")
	}
}

func TestWillReachStateRoundTrip(t *testing.T) {
	path := willReachStatePath(t.TempDir())
	if st, err := loadWillReachState(path); err != nil || st.NextSeq != 1 || st.Pending != nil {
		t.Fatalf("missing reach state should load as first sequence, st=%#v err=%v", st, err)
	}
	pending := &willPendingReach{
		Seq:                  3,
		ID:                   "reach3",
		Utility:              willUtilPressure,
		Tide:                 willTideSnapshot{Threshold: 1, Gaze: 1.5, PressureTide: 1.5},
		Breath:               12,
		Attempts:             2,
		LastFailureOutcome:   willOutcomeSensorError,
		ConsequenceCommitted: true,
		Outcome:              willOutcomePerceptionCommitted,
		EffectCount:          2,
	}
	if err := saveWillReachState(path, willReachState{NextSeq: 3, Pending: pending}); err != nil {
		t.Fatalf("save pending reach: %v", err)
	}
	st, err := loadWillReachState(path)
	if err != nil {
		t.Fatalf("load pending reach: %v", err)
	}
	if st.NextSeq != 3 || st.Pending == nil || st.Pending.ID != "reach3" || st.Pending.Utility != willUtilPressure ||
		st.Pending.Attempts != 2 || st.Pending.LastFailureOutcome != willOutcomeSensorError ||
		!st.Pending.ConsequenceCommitted || st.Pending.Outcome != willOutcomePerceptionCommitted || st.Pending.EffectCount != 2 {
		t.Fatalf("wrong pending reach after round-trip: %#v", st)
	}
	if err := saveWillReachState(path, willReachState{NextSeq: 4}); err != nil {
		t.Fatalf("clear pending reach: %v", err)
	}
	st, err = loadWillReachState(path)
	if err != nil {
		t.Fatalf("reload cleared reach: %v", err)
	}
	if st.NextSeq != 4 || st.Pending != nil {
		t.Fatalf("cleared reach state should advance sequence only, got %#v", st)
	}
}

func TestWillReachStateRejectsCorruptPending(t *testing.T) {
	path := willReachStatePath(t.TempDir())
	if err := os.WriteFile(path, []byte(`{"version":1,"next_seq":2,"pending":{"seq":1,"id":"x","util":"repo_monitor"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadWillReachState(path); err == nil {
		t.Fatal("pending reach with mismatched sequence must fail loud")
	}
}

func TestWillReachStateRejectsAttemptWithoutFailureOutcome(t *testing.T) {
	path := willReachStatePath(t.TempDir())
	body := `{"version":1,"next_seq":2,"pending":{"seq":2,"id":"x","util":"repo_monitor","tide":{"threshold":1},"attempts":1}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadWillReachState(path); err == nil {
		t.Fatal("pending reach attempts must carry a typed failure outcome")
	}
}

func TestWillReachStateRejectsCorruptCommittedConsequence(t *testing.T) {
	cases := []string{
		`{"version":1,"next_seq":2,"pending":{"seq":2,"id":"x","util":"repo_monitor","tide":{"threshold":1},"consequence_committed":true,"outcome":"perception_committed","effect_count":0}}`,
		`{"version":1,"next_seq":2,"pending":{"seq":2,"id":"x","util":"repo_monitor","tide":{"threshold":1},"consequence_committed":true,"outcome":"no_novelty","effect_count":1}}`,
		`{"version":1,"next_seq":2,"pending":{"seq":2,"id":"x","util":"repo_monitor","tide":{"threshold":1},"consequence_committed":true,"outcome":"dead_letter","effect_count":0}}`,
		`{"version":1,"next_seq":2,"pending":{"seq":2,"id":"x","util":"repo_monitor","tide":{"threshold":1},"consequence_committed":true,"outcome":"dead_letter","effect_count":0,"attempts":2,"last_failure_outcome":"made_up"}}`,
		`{"version":1,"next_seq":2,"pending":{"seq":2,"id":"x","util":"repo_monitor","tide":{"threshold":1},"outcome":"perception_committed","effect_count":1}}`,
	}
	for i, body := range cases {
		path := willReachStatePath(t.TempDir())
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := loadWillReachState(path); err == nil {
			t.Fatalf("case %d should reject corrupt committed consequence", i)
		}
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

func TestFileSinkSkipsAlreadyDeliveredRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	sink := fileSink{path: path}
	line := []byte(`{"id":"reach1","phase":"effect","util":"repo_monitor","kind":"modified","path":"README.md"}`)
	if err := sink.Emit(line); err != nil {
		t.Fatalf("first Emit: %v", err)
	}
	if err := sink.Emit(line); err != nil {
		t.Fatalf("duplicate Emit: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := len(completeSartreJSONLines(data)); got != 1 {
		t.Fatalf("duplicate effect should be skipped, got %d lines: %s", got, data)
	}
}

func TestFileSinkDropsNoiseAndIncompleteRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	sink := fileSink{path: path}
	raw := []byte("not json with \"kind\"\n" +
		`{"util":"repo_monitor","kind":"added","path":"a.md"}` + "\n" +
		`{"util":"repo_monitor","kind":"modified"`)
	if err := sink.Emit(raw); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := strings.TrimSpace(string(data))
	want := `{"util":"repo_monitor","kind":"added","path":"a.md"}`
	if got != want {
		t.Fatalf("sink must persist only complete valid JSONL records\ngot:  %q\nwant: %q", got, want)
	}
}

func TestTagSartreEffectLines(t *testing.T) {
	raw := []byte(`{"util":"repo_monitor","kind":"added","path":"a.md"}` + "\nnoise\n")
	got := string(tagSartreEffectLines(raw, "reach1", "rootabc"))
	if !strings.Contains(got, `"id":"reach1"`) ||
		!strings.Contains(got, `"root_id":"rootabc"`) ||
		!strings.Contains(got, `"phase":"effect"`) ||
		!strings.Contains(got, `"util":"repo_monitor"`) ||
		strings.Contains(got, "noise") {
		t.Fatalf("effect tagging failed: %q", got)
	}
}

func TestTagSartreEffectLinesRejectsIncompleteEvents(t *testing.T) {
	raw := []byte(`{"util":"repo_monitor"}` + "\n" +
		`{"kind":"modified","path":"a.md"}` + "\n" +
		`{"util":"unknown","kind":"modified","path":"a.md"}`)
	got := string(tagSartreEffectLines(raw, "reach1", "rootabc"))
	if got != "" {
		t.Fatalf("incomplete utility records must not become SARTRE effects: %q", got)
	}
}

func TestFileSinkEmitEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := (fileSink{path: path}).EmitEvent(willEvent{
		ID:      "abc",
		Phase:   "learning",
		Outcome: "no_novelty",
		Utility: willUtilOrigin,
	}); err != nil {
		t.Fatalf("EmitEvent: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"phase":"learning"`) ||
		!strings.Contains(string(data), `"outcome":"no_novelty"`) ||
		!strings.Contains(string(data), `"util":"whatdotheythinkiam"`) {
		t.Fatalf("typed will event not written: %s", data)
	}
}

func TestFileSinkNoop(t *testing.T) {
	if err := (fileSink{path: ""}).Emit([]byte(`{"util":"repo_monitor","kind":"added"}`)); err == nil {
		t.Error("a non-empty event without a sink path must fail so sensor state is not committed")
	}
	path := filepath.Join(t.TempDir(), "e.jsonl")
	if err := (fileSink{path: path}).Emit(nil); err != nil {
		t.Errorf("an empty line must be a silent no-op, got %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("an empty-line Emit must not create the file")
	}
}
