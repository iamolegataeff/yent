package yent

import (
	"strings"
	"testing"
)

func TestParseSartreEventsJSONLIgnoresSlotNoise(t *testing.T) {
	jsonl := `[pipe] slot[0] pid=123 reading events:
{"util":"repo_monitor","kind":"modified","path":"/Users/ariannamethod/arianna/yent/README.md","ts":1}
not json
{"util":"context_processor","path":"/Users/ariannamethod/arianna/yent/research/recursive_resonance_preprint.md","tag":".md","relevance":0.42,"pulse":0.73}
`
	events := ParseSartreEventsJSONL(jsonl)
	if len(events) != 2 {
		t.Fatalf("want two events, got %#v", events)
	}
	if events[0].Utility != "repo_monitor" || events[0].Kind != "modified" ||
		events[0].Path != ".../arianna/yent/README.md" {
		t.Fatalf("repo event not normalized: %#v", events[0])
	}
	if events[1].Utility != "context_processor" || events[1].Tag != ".md" ||
		events[1].Relevance != 0.42 || events[1].Pulse != 0.73 ||
		events[1].Path != ".../yent/research/recursive_resonance_preprint.md" {
		t.Fatalf("context event not normalized: %#v", events[1])
	}
}

func TestStoreSartreEventsAndRecall(t *testing.T) {
	lc := newTestLimpha(t)
	events := []SartreEvent{
		{Utility: "repo_monitor", Kind: "modified", Path: "/repo/README.md"},
		{Utility: "context_processor", Path: "/repo/research/recursive_resonance_preprint.md", Tag: ".md", Relevance: 0.5, Pulse: 0.7},
	}
	id, err := lc.StoreSartreEvents(events, LimphaState{Destiny: 0.3, Velocity: 2})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected seam id")
	}
	stats, err := lc.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats["total_conversations"].(int64) != 1 || stats["total_seams"].(int64) != 1 {
		t.Fatalf("want 1 conversation / 1 seam, got %v / %v", stats["total_conversations"], stats["total_seams"])
	}
	seams, err := lc.RecentSeams(1)
	if err != nil || len(seams) != 1 {
		t.Fatalf("RecentSeams: err=%v len=%d", err, len(seams))
	}
	if seams[0]["body_a"] != "sartre" || seams[0]["body_b"] != "limpha" ||
		seams[0]["reason"] != SartreSeamReason || seams[0]["winner"] != "limpha" {
		t.Fatalf("wrong sartre seam: %+v", seams[0])
	}
	if !strings.Contains(seams[0]["memory_delta"].(string), `"kind":"sartre_perception"`) {
		t.Fatalf("memory_delta must be typed JSON: %s", seams[0]["memory_delta"])
	}

	got := NewSartreMemory(lc).Recall(2)
	if len(got) != 1 {
		t.Fatalf("want one sartre memory trace, got %#v", got)
	}
	if !strings.Contains(got[0], "SARTRE perception:") ||
		!strings.Contains(got[0], "repo_monitor modified") ||
		!strings.Contains(got[0], "context_processor") {
		t.Fatalf("bad sartre recall trace: %#v", got)
	}
	if strings.Contains(got[0], "/Users/") {
		t.Fatalf("absolute local path leaked into trace: %q", got[0])
	}
}

func TestBuildSartreReceiptCapsTraceButCountsMetrics(t *testing.T) {
	var events []SartreEvent
	for i := 0; i < 20; i++ {
		events = append(events, SartreEvent{Utility: "repo_monitor", Kind: "modified", Path: "research/file.md"})
	}
	events = append(events, SartreEvent{Utility: "context_processor", Path: "research/hot.md", Tag: ".md", Relevance: 0.91, Pulse: 0.88})

	receipt := BuildSartreReceipt(events)
	if receipt.EventCount != 21 {
		t.Fatalf("receipt should count the whole capped packet, got %d", receipt.EventCount)
	}
	if receipt.MaxRelevance != 0.91 || receipt.MaxPulse != 0.88 {
		t.Fatalf("late metric event should still count, relevance=%.2f pulse=%.2f", receipt.MaxRelevance, receipt.MaxPulse)
	}
	if len(receipt.Trace) > 12 {
		t.Fatalf("trace should stay capped, got %d", len(receipt.Trace))
	}
}

func TestSartreMemoryFiltersOtherSeams(t *testing.T) {
	lc := newTestLimpha(t)
	if _, err := lc.StoreSeam(Seam{
		BodyA: "nemo12", BodyB: "small24", Reason: "innerworld_self_answer",
		BClaim: "inner monologue should not be returned by SARTRE memory",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := lc.StoreSartreEvents([]SartreEvent{{Utility: "repo_monitor", Kind: "added", Path: "research/new.md"}}, LimphaState{}); err != nil {
		t.Fatal(err)
	}
	got := NewSartreMemory(lc).Recall(3)
	if len(got) != 1 || !strings.Contains(got[0], "research/new.md") {
		t.Fatalf("SARTRE memory should return only sartre seams, got %#v", got)
	}
}
