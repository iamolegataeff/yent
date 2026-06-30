package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

func TestRunIngestsEventsFile(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "limpha.db")
	events := filepath.Join(dir, "sartre.jsonl")
	writeTestEvents(t, events)

	var stdout, stderr bytes.Buffer
	if err := run([]string{"-db", db, "-events", events}, &stdout, &stderr, strings.NewReader("")); err != nil {
		t.Fatalf("run failed: %v stderr=%s", err, stderr.String())
	}
	var res ingestResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("bad json result %q: %v", stdout.String(), err)
	}
	if res.Kind != "sartre_limphed" || res.Events != 2 || res.SeamID == 0 {
		t.Fatalf("unexpected result: %+v", res)
	}

	lc, err := yent.NewLimphaClientAt(db)
	if err != nil {
		t.Fatal(err)
	}
	defer lc.Close()
	traces := yent.NewSartreMemory(lc).Recall(2)
	if len(traces) != 1 || !strings.Contains(traces[0], "context_processor") {
		t.Fatalf("stored SARTRE trace missing: %#v", traces)
	}
}

func TestRunIngestsStdin(t *testing.T) {
	db := filepath.Join(t.TempDir(), "limpha.db")
	input := `[pipe] slot ready
{"util":"repo_monitor","kind":"added","path":"/repo/research/new.md","ts":1}
`
	var stdout, stderr bytes.Buffer
	if err := run([]string{"-db", db}, &stdout, &stderr, strings.NewReader(input)); err != nil {
		t.Fatalf("run failed: %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"events":1`) {
		t.Fatalf("expected event count in stdout, got %q", stdout.String())
	}
}

func writeTestEvents(t *testing.T, path string) {
	t.Helper()
	data := `[pipe] slot ready
{"util":"repo_monitor","kind":"modified","path":"/repo/README.md","ts":1}
{"util":"context_processor","path":"/repo/research/dario_paper_v2.md","tag":".md","relevance":0.41,"pulse":0.66}
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
