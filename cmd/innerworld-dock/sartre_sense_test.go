package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ariannamethod/yent/innerworld"
)

// TestSartreSensePerceivesMotion proves the cgo perception binding end to end on Neo
// (no model): events -> sartre_perceive_from_events -> sartre_perceive_to_aml. Two
// changes including a README -> prophecy 2+2+7=11, matching the C self-test case.
func TestSartreSensePerceivesMotion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	events := `{"util":"repo_monitor","kind":"added","path":"/r/x.rs","ts":1}
{"util":"repo_monitor","kind":"modified","path":"/r/README.md","ts":2}`
	if err := os.WriteFile(path, []byte(events), 0o644); err != nil {
		t.Fatal(err)
	}
	aml, ok := sartreSense{eventsPath: path}.Pressure()
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
	if _, ok := (sartreSense{eventsPath: empty}).Pressure(); ok {
		t.Error("an empty events file is a quiet world: no reflex")
	}
	if _, ok := (sartreSense{eventsPath: ""}).Pressure(); ok {
		t.Error("no path is no environment: no reflex")
	}
	if _, ok := (sartreSense{eventsPath: filepath.Join(dir, "nope.jsonl")}).Pressure(); ok {
		t.Error("a missing file is no environment: no reflex")
	}
}

func TestSartreMetricSinkPublishesFieldWeather(t *testing.T) {
	initSartreHub()
	defer shutdownSartreHub()

	if err := (sartreMetricSink{}).PublishMetrics(innerworld.MetricSnapshot{
		Debt:      2.25,
		Coherence: 0.50,
		Entropy:   0.40,
		Valence:   -0.30,
		Arousal:   0.70,
		Trauma:    0.30,
		Warmth:    0.0,
		Flow:      0.0,
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
		got["trauma"].(float64) != 0.3 {
		t.Fatalf("SARTRE hub did not receive field weather: %+v", got)
	}
}
