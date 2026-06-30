package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ariannamethod/yent/innerworld"
	yent "github.com/ariannamethod/yent/yent/go"
)

func TestDurationEnv(t *testing.T) {
	const k = "YENT_TEST_DURATION"
	cases := []struct {
		raw  string
		want time.Duration
	}{
		{"", 0}, // unset/blank -> default
		{"300", 300 * time.Second},
		{"0", 0},     // non-positive -> default
		{"-5", 0},    // negative -> default
		{"NaN", 0},   // NaN must be rejected, not flow into time.Duration
		{"Inf", 0},   // +Inf rejected
		{"+Inf", 0},  // +Inf rejected
		{"-Inf", 0},  // -Inf is <= 0, rejected
		{"1e400", 0}, // parses to +Inf -> rejected
		{"abc", 0},   // unparseable -> default
		{"1.5", time.Duration(1.5 * float64(time.Second))},
	}
	for _, c := range cases {
		t.Setenv(k, c.raw)
		if got := durationEnv(k); got != c.want {
			t.Errorf("durationEnv(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}

func TestPersistReflectionStoresConversationAndSeam(t *testing.T) {
	lc, err := yent.NewLimphaClientAt(filepath.Join(t.TempDir(), "limpha.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer lc.Close()

	r := innerworld.Reflection{
		Circles: []innerworld.Circle{
			{Index: 0, Temp: 0.7, Drift: 0.2, Text: "first inner circle"},
			{Index: 1, Temp: 0.9, Drift: 0.6, Text: "second inner circle"},
		},
		Coupling:       0.75,
		SelfAnswerProb: 0.82,
		SelfAnswered:   true,
		DeepAnswer:     "the deep body answers inward",
		MemoryPressure: innerworld.MemoryFieldPressure{Score: 4, Prophecy: 5, Velocity: "WALK", Step: 0.31},
	}
	persistReflection(lc, "human_turn", "what is happening?", r, yent.LimphaState{Temperature: 0.9, Destiny: 0.3, Debt: 2, Velocity: 2})

	stats, err := lc.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats["total_conversations"].(int64) != 1 || stats["total_seams"].(int64) != 1 {
		t.Fatalf("want 1 conversation / 1 seam, got %v / %v", stats["total_conversations"], stats["total_seams"])
	}
	recent, _ := lc.Recent(1, true)
	if len(recent) != 1 || !strings.Contains(recent[0]["prompt"].(string), "[innerworld/human_turn]") ||
		!strings.Contains(recent[0]["response"].(string), "second inner circle") {
		t.Fatalf("conversation not stored as inner reflection: %+v", recent)
	}
	seams, _ := lc.RecentSeams(1)
	if len(seams) != 1 {
		t.Fatalf("want seam, got %d", len(seams))
	}
	if seams[0]["body_a"] != "nemo12" || seams[0]["body_b"] != "small24" ||
		seams[0]["reason"] != "innerworld_self_answer" ||
		!strings.Contains(seams[0]["memory_delta"].(string), "innerworld_reflection") {
		t.Fatalf("inner seam wrong: %+v", seams[0])
	}
	if seams[0]["conversation_id"] == nil {
		t.Fatalf("inner seam should link to its stored reflection conversation: %+v", seams[0])
	}
	delta := seams[0]["memory_delta"].(string)
	if !strings.Contains(delta, `"memory_pressure"`) || !strings.Contains(delta, `"score":4`) ||
		!strings.Contains(delta, `"prophecy":5`) {
		t.Fatalf("inner seam should preserve memory pressure receipt: %s", delta)
	}
}

func TestPersistReflectionSkipsEmpty(t *testing.T) {
	lc, err := yent.NewLimphaClientAt(filepath.Join(t.TempDir(), "limpha.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer lc.Close()

	persistReflection(lc, "dream", "empty", innerworld.Reflection{}, yent.LimphaState{})
	stats, err := lc.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats["total_conversations"].(int64) != 0 || stats["total_seams"].(int64) != 0 {
		t.Fatalf("empty reflection should not write memory, got %v / %v", stats["total_conversations"], stats["total_seams"])
	}
}

func TestLimphaRecallerFiltersAndCompactsInnerSeams(t *testing.T) {
	lc, err := yent.NewLimphaClientAt(filepath.Join(t.TempDir(), "limpha.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer lc.Close()

	if _, err := lc.StoreSeam(yent.Seam{
		BodyA:  "nemo12",
		BodyB:  "small24",
		Prompt: "old inner",
		AClaim: "circle 0\n   fallback   stream ",
		Reason: "innerworld_self_answer",
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := lc.StoreSeam(yent.Seam{
		BodyA:  "nemo12",
		BodyB:  "small24",
		Prompt: "router seam",
		BClaim: "router output must not become an inner recall",
		Reason: "route_escalation",
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := lc.StoreSeam(yent.Seam{
		BodyA:  "nemo12",
		BodyB:  "small24",
		Prompt: "new inner",
		AClaim: "circle stream should lose to the deep answer",
		BClaim: "deep answer " + strings.Repeat("with memory pressure ", 30),
		Reason: "innerworld_self_answer",
	}); err != nil {
		t.Fatal(err)
	}

	got := (limphaRecaller{lc: lc}).Recall(3)
	if len(got) != 2 {
		t.Fatalf("want only two inner recalls, got %d: %#v", len(got), got)
	}
	if !strings.HasPrefix(got[0], "deep answer with memory pressure") {
		t.Fatalf("newest inner b_claim should be recalled first, got %q", got[0])
	}
	if strings.Contains(got[0], "circle stream should lose") {
		t.Fatalf("b_claim should be preferred over a_claim, got %q", got[0])
	}
	if !utf8.ValidString(got[0]) || len([]rune(got[0])) > 240 {
		t.Fatalf("recall should be rune-valid and capped to 240 runes, len=%d valid=%v", len([]rune(got[0])), utf8.ValidString(got[0]))
	}
	if got[1] != "circle 0 fallback stream" {
		t.Fatalf("empty b_claim should fall back to compacted a_claim, got %q", got[1])
	}
	for _, recall := range got {
		if strings.Contains(recall, "router output") {
			t.Fatalf("non-inner seam leaked into recall: %#v", got)
		}
	}
}

func TestLimphaRecallerLimitAndNilSafe(t *testing.T) {
	if got := (limphaRecaller{}).Recall(3); got != nil {
		t.Fatalf("nil limpha client should recall nothing, got %#v", got)
	}

	lc, err := yent.NewLimphaClientAt(filepath.Join(t.TempDir(), "limpha.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer lc.Close()

	for _, claim := range []string{"first", "second"} {
		if _, err := lc.StoreSeam(yent.Seam{
			BodyA:  "nemo12",
			BodyB:  "small24",
			BClaim: claim,
			Reason: "innerworld_self_answer",
		}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	if got := (limphaRecaller{lc: lc}).Recall(0); got != nil {
		t.Fatalf("n<=0 should recall nothing, got %#v", got)
	}
	got := (limphaRecaller{lc: lc}).Recall(1)
	if len(got) != 1 || got[0] != "second" {
		t.Fatalf("limit should return the newest inner thought only, got %#v", got)
	}
}

func TestOpenRIFromEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.lines")
	if err := os.WriteFile(path, []byte(`packet	mode=runtime	input=ri/out/index.lines	count=3
pressure	text=RI must become structure.
quote	test=true	text=Don't become me.
quote	test=false	text=This should not reach runtime.
`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YENT_RI_LINES", path)
	t.Setenv("YENT_RI_MAX", "8")

	mem := openRIFromEnv()
	if mem == nil {
		t.Fatal("expected RI memory")
	}
	text := strings.Join(mem.Recall(4), "\n")
	if !strings.Contains(text, "RI pressure: RI must become structure.") ||
		!strings.Contains(text, "RI test quote: Don't become me.") {
		t.Fatalf("RI memory missing runtime records:\n%s", text)
	}
	if strings.Contains(text, "This should not reach runtime") {
		t.Fatalf("RI memory leaked non-test quote:\n%s", text)
	}
}

func TestIngestSartreFromEnvStoresPerception(t *testing.T) {
	lc, err := yent.NewLimphaClientAt(filepath.Join(t.TempDir(), "limpha.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer lc.Close()

	path := filepath.Join(t.TempDir(), "sartre.jsonl")
	if err := os.WriteFile(path, []byte(`[pipe] slot ready
{"util":"repo_monitor","kind":"modified","path":"/repo/README.md","ts":1}
{"util":"context_processor","path":"/repo/research/dario_paper_v2.md","tag":".md","relevance":0.41,"pulse":0.66}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("YENT_SARTRE_EVENTS", path)

	if got := ingestSartreFromEnv(lc, yent.LimphaState{Destiny: 0.2}); got != 2 {
		t.Fatalf("want 2 ingested events, got %d", got)
	}
	traces := yent.NewSartreMemory(lc).Recall(2)
	if len(traces) != 1 || !strings.Contains(traces[0], "SARTRE perception") ||
		!strings.Contains(traces[0], "context_processor") {
		t.Fatalf("SARTRE traces missing after ingest: %#v", traces)
	}
	stats, err := lc.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats["total_conversations"].(int64) != 1 || stats["total_seams"].(int64) != 1 {
		t.Fatalf("ingest should store one conversation and one seam, got %v / %v", stats["total_conversations"], stats["total_seams"])
	}
}
