package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ariannamethod/yent/innerworld"
	yent "github.com/ariannamethod/yent/yent/go"
)

// TestSartreSensePerceivesMotion proves the cgo perception binding end to end on Neo
// (no model): events -> sartre_perceive_from_events -> sartre_perceive_to_aml. Two
// changes including a README -> prophecy 2+2+7=11, matching the C self-test case.
func TestSartreSensePerceivesMotion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	events := `{"util":"repo_monitor","kind":"added","path":"/r/x.rs","ts":1}
{"util":"repo_monitor","kind":"modified","path":"/r/README.md","ts":2}
`
	if err := os.WriteFile(path, []byte(events), 0o644); err != nil {
		t.Fatal(err)
	}
	aml, ok := (&sartreSense{eventsPath: path}).Pressure()
	if !ok {
		t.Fatal("two changes including a README should produce a reflex")
	}
	if !strings.Contains(aml, "VELOCITY RUN") || !strings.Contains(aml, "PROPHECY 11") {
		t.Errorf("perception AML mismatch, got %q (want VELOCITY RUN + PROPHECY 11)", aml)
	}
}

// TestSartreSenseQuietNoReflex: a still or absent environment feels nothing.
func TestSartreSenseQuietNoReflex(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(empty, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := (&sartreSense{eventsPath: empty}).Pressure(); ok {
		t.Error("an empty events file is a quiet world: no reflex")
	}
	if _, ok := (&sartreSense{eventsPath: ""}).Pressure(); ok {
		t.Error("no path is no environment: no reflex")
	}
	if _, ok := (&sartreSense{eventsPath: filepath.Join(dir, "nope.jsonl")}).Pressure(); ok {
		t.Error("a missing file is no environment: no reflex")
	}
}

// TestSartreSenseCursorConsumes proves the HIGH fix: the cursor makes each appended event be
// perceived exactly once, so the continuously-appended will events file is not replayed in full
// every ripple (which would latch the field on a growing history).
func TestSartreSenseCursorConsumes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(`{"util":"repo_monitor","kind":"added","path":"/r/a.rs","ts":1}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &sartreSense{eventsPath: path}
	if got := s.readNew(); len(got) == 0 {
		t.Fatal("the first read must see the event")
	}
	if got := s.readNew(); len(got) != 0 {
		t.Errorf("no new events -> nothing replayed, got %q", got)
	}
	// append a new event: only the new bytes are read, never the already-perceived history
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"util":"repo_monitor","kind":"modified","path":"/r/b.rs","ts":2}` + "\n")
	f.Close()
	got := string(s.readNew())
	if !strings.Contains(got, "b.rs") || strings.Contains(got, "a.rs") {
		t.Errorf("only the newly appended event must be read (no replay of a.rs), got %q", got)
	}
	// a truncation/rotation resets the cursor so the fresh content is seen
	if err := os.WriteFile(path, []byte(`{"util":"repo_monitor","kind":"added","path":"/r/c.rs","ts":3}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := string(s.readNew()); !strings.Contains(got, "c.rs") {
		t.Errorf("a truncated/rotated file must re-read from the start, got %q", got)
	}
}

func TestSartreSenseKeepsPartialRecordUntilNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(`{"util":"repo_monitor","kind":"added"`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &sartreSense{eventsPath: path}
	if got := s.readNew(); got != nil {
		t.Fatalf("a partial record must not be consumed as an event, got %q", got)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`,"path":"late.md"}` + "\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	got := string(s.readNew())
	if !strings.Contains(got, `"path":"late.md"`) {
		t.Fatalf("the completed record must be perceived after its newline, got %q", got)
	}
}

func TestSartreEOFOffsetStartsLiveReaderAfterStartupHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(`{"util":"repo_monitor","kind":"added","path":"old.md"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &sartreSense{eventsPath: path, offset: sartreEOFOffset(path)}
	if got := s.readNew(); len(got) != 0 {
		t.Fatalf("startup history should not be replayed by the live reader, got %q", got)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"util":"repo_monitor","kind":"added","path":"new.md"}` + "\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	got := string(s.readNew())
	if !strings.Contains(got, "new.md") || strings.Contains(got, "old.md") {
		t.Fatalf("live reader should see only post-startup events, got %q", got)
	}
}

func TestSartreSenseIgnoresForgedKindNoise(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(`plain text with "kind" should not move the field`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if aml, ok := (&sartreSense{eventsPath: path}).Pressure(); ok {
		t.Fatalf("non-JSON kind noise must not become field pressure, got %q", aml)
	}
}

func TestSartreSenseStoresIdentityEventWithoutForcingRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	event := `{"util":"whatdotheythinkiam","kind":"modified","path":"README.md","reduced":3,"recognized":7}`
	if err := os.WriteFile(path, []byte(event+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lc, err := yent.NewLimphaClientAt(filepath.Join(dir, "limpha.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer lc.Close()
	if aml, ok := (&sartreSense{eventsPath: path, limpha: lc}).Pressure(); ok {
		t.Fatalf("identity framing is memory pressure, not a coarse field RUN, got %q", aml)
	}
	traces := yent.NewSartreMemory(lc).Recall(1)
	if len(traces) != 1 || !strings.Contains(traces[0], "whatdotheythinkiam README.md modified reduced=3 recognized=7") {
		t.Fatalf("identity event should be stored in limpha, got %#v", traces)
	}
}

func TestSartreMetricSinkPublishesFieldWeather(t *testing.T) {
	initSartreHub()
	defer shutdownSartreHub()

	if err := (sartreMetricSink{}).PublishMetrics(innerworld.MetricSnapshot{
		Debt:                2.25,
		Coherence:           0.50,
		Entropy:             0.40,
		Valence:             -0.30,
		Arousal:             0.70,
		Trauma:              0.30,
		Warmth:              0.0,
		Flow:                0.0,
		MemoryFieldScore:    4,
		MemoryFieldProphecy: 5,
		MemoryFieldStep:     0.31,
	}); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(sartreStateJSON()), &got); err != nil {
		t.Fatalf("bad SARTRE JSON: %v", err)
	}
	if got["prophecy_debt"].(float64) != 2.25 ||
		got["coherence"].(float64) != 0.5 ||
		got["valence"].(float64) != -0.3 ||
		got["arousal"].(float64) != 0.7 ||
		got["trauma"].(float64) != 0.3 ||
		got["memory_field_score"].(float64) != 4 ||
		got["memory_field_prophecy"].(float64) != 5 ||
		got["memory_field_step"].(float64) != 0.31 {
		t.Fatalf("SARTRE hub did not receive field weather: %+v", got)
	}
}
