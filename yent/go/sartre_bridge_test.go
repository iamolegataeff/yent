package yent

import (
	"strings"
	"testing"
)

func TestParseSartreEventsJSONLIgnoresSlotNoise(t *testing.T) {
	jsonl := `[pipe] slot[0] pid=123 reading events:
{"util":"repo_monitor","kind":"modified","path":"/Users/ariannamethod/arianna/yent/README.md","ts":1}
not json
{"util":"context_processor","path":"/Users/ariannamethod/arianna/yent/research/recursive_resonance_preprint.md","resonance":0.52,"relevance":0.42,"pulse":0.73}
{"util":"whatdotheythinkiam","path":"README.md","kind":"modified","reduced":4,"recognized":2,"ts":1}
`
	events := ParseSartreEventsJSONL(jsonl)
	if len(events) != 3 {
		t.Fatalf("want three events, got %#v", events)
	}
	if events[0].Utility != "repo_monitor" || events[0].Kind != "modified" ||
		events[0].Path != ".../arianna/yent/README.md" {
		t.Fatalf("repo event not normalized: %#v", events[0])
	}
	if events[1].Utility != "context_processor" || events[1].Tag != "" ||
		events[1].Resonance != 0.52 || events[1].Relevance != 0.42 || events[1].Pulse != 0.73 ||
		events[1].Path != ".../yent/research/recursive_resonance_preprint.md" {
		t.Fatalf("context event not normalized: %#v", events[1])
	}
	if events[2].Utility != "whatdotheythinkiam" || events[2].Kind != "modified" ||
		events[2].Path != "README.md" || events[2].Reduced != 4 || events[2].Recognized != 2 {
		t.Fatalf("framing event not preserved: %#v", events[2])
	}
}

func TestStoreSartreEventsAndRecall(t *testing.T) {
	lc := newTestLimpha(t)
	events := []SartreEvent{
		{Utility: "repo_monitor", Kind: "modified", Path: "/repo/README.md"},
		{Utility: "context_processor", Path: "/repo/research/recursive_resonance_preprint.md", Resonance: 0.63, Relevance: 0.5, Pulse: 0.7},
		{Utility: "whatdotheythinkiam", Kind: "modified", Path: "README.md", Reduced: 4, Recognized: 2},
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
	if !strings.Contains(seams[0]["memory_delta"].(string), `"max_resonance":0.63`) {
		t.Fatalf("memory_delta must preserve resonance: %s", seams[0]["memory_delta"])
	}
	if !strings.Contains(seams[0]["memory_delta"].(string), `"framing_event_count":1`) ||
		!strings.Contains(seams[0]["memory_delta"].(string), `"max_reduced":4`) ||
		!strings.Contains(seams[0]["memory_delta"].(string), `"max_recognized":2`) {
		t.Fatalf("memory_delta must preserve framing counts: %s", seams[0]["memory_delta"])
	}

	got := NewSartreMemory(lc).Recall(2)
	if len(got) != 1 {
		t.Fatalf("want one sartre memory trace, got %#v", got)
	}
	if !strings.Contains(got[0], "SARTRE perception:") ||
		!strings.Contains(got[0], "repo_monitor modified") ||
		!strings.Contains(got[0], "context_processor") ||
		!strings.Contains(got[0], "resonance=0.63") ||
		!strings.Contains(got[0], "whatdotheythinkiam README.md modified reduced=4 recognized=2") ||
		strings.Contains(got[0], "tag=?") {
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
	events = append(events, SartreEvent{Utility: "context_processor", Path: "research/hot.md", Resonance: 0.94, Relevance: 0.91, Pulse: 0.88})
	events = append(events, SartreEvent{Utility: "whatdotheythinkiam", Path: "README.md", Kind: "modified", Reduced: 3, Recognized: 7})

	receipt := BuildSartreReceipt(events)
	if receipt.EventCount != 22 {
		t.Fatalf("receipt should count the whole capped packet, got %d", receipt.EventCount)
	}
	if receipt.MaxResonance != 0.94 || receipt.MaxRelevance != 0.91 || receipt.MaxPulse != 0.88 {
		t.Fatalf("late metric event should still count, resonance=%.2f relevance=%.2f pulse=%.2f", receipt.MaxResonance, receipt.MaxRelevance, receipt.MaxPulse)
	}
	if len(receipt.Trace) > 12 {
		t.Fatalf("trace should stay capped, got %d", len(receipt.Trace))
	}
	if receipt.FramingEventCount != 1 || receipt.MaxReduced != 3 || receipt.MaxRecognized != 7 {
		t.Fatalf("framing counts should survive receipt build: %+v", receipt)
	}
}

func TestBuildSartreReceiptKeepsWillPhasesOutOfChangeCounts(t *testing.T) {
	events := []SartreEvent{
		{ID: "r1", Phase: "intention", Outcome: "crest", Utility: "repo_monitor", Kind: "modified", Path: "README.md", RootID: "rootabc", Breath: 2, CadenceMS: 750, RefractoryBreaths: 3},
		{ID: "r1", Phase: "act", Outcome: "spawned", Utility: "repo_monitor"},
		{ID: "r1", Phase: "learning", Outcome: "no_novelty", Utility: "repo_monitor"},
		{ID: "r2", Phase: "learning", Outcome: "overflow", Utility: "repo_monitor", BytesCaptured: 1024, BytesLimit: 2048},
		{ID: "r2", Phase: "effect", Utility: "repo_monitor", Kind: "modified", Path: "README.md"},
		{ID: "r2", Phase: "learning", Outcome: "perception_committed", Utility: "repo_monitor", EffectCount: 1},
	}
	receipt := BuildSartreReceipt(events)
	if receipt.EventCount != 6 {
		t.Fatalf("all typed events should be counted as received, got %d", receipt.EventCount)
	}
	if receipt.Changed != 1 || !receipt.ReadmeChanged {
		t.Fatalf("only the actual effect/change event should move change counters: %+v", receipt)
	}
	if receipt.OutcomeCounts["no_novelty"] != 1 ||
		receipt.OutcomeCounts["overflow"] != 1 ||
		receipt.OutcomeCounts["perception_committed"] != 1 {
		t.Fatalf("learning outcomes should be typed counters: %+v", receipt.OutcomeCounts)
	}
	trace := strings.Join(receipt.Trace, " | ")
	if len(receipt.Trace) < 2 ||
		!strings.Contains(trace, "will repo_monitor intention crest") ||
		!strings.Contains(trace, "root=rootabc") ||
		!strings.Contains(trace, "breath=2 cadence_ms=750 refractory_breaths=3") ||
		!strings.Contains(trace, "will repo_monitor learning overflow bytes=1024/2048") ||
		!strings.Contains(trace, "will repo_monitor learning perception_committed effects=1") ||
		!strings.Contains(trace, "repo_monitor modified README.md") {
		t.Fatalf("receipt should preserve will phases and effect trace: %#v", receipt.Trace)
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
