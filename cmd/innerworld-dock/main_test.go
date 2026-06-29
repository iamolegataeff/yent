package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

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
