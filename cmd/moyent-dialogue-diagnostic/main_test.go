package main

import (
	"strings"
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

func TestResponseTextOutputText(t *testing.T) {
	got, err := responseText([]byte(`{"output_text":"  say the next thing  "}`))
	if err != nil {
		t.Fatalf("responseText: %v", err)
	}
	if got != "say the next thing" {
		t.Fatalf("got %q", got)
	}
}

func TestResponseTextNestedOutput(t *testing.T) {
	raw := []byte(`{
		"output": [
			{"type":"reasoning","content":[{"type":"output_text","text":"skip"}]},
			{"type":"message","content":[{"type":"output_text","text":"first"},{"type":"output_text","text":"second"}]}
		]
	}`)
	got, err := responseText(raw)
	if err != nil {
		t.Fatalf("responseText: %v", err)
	}
	if got != "first\nsecond" {
		t.Fatalf("got %q", got)
	}
}

func TestResponseTextIncomplete(t *testing.T) {
	_, err := responseText([]byte(`{
		"status": "incomplete",
		"incomplete_details": {"reason": "max_output_tokens"},
		"output_text": "half a sentence"
	}`))
	if err == nil || !strings.Contains(err.Error(), "max_output_tokens") {
		t.Fatalf("expected incomplete error, got %v", err)
	}
}

func TestSanitizeQuestion(t *testing.T) {
	got := sanitizeQuestion("`1. \"Are you still there?\"\n- Answer plainly.`")
	want := "Are you still there?\nAnswer plainly."
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildMistralPromptRolling(t *testing.T) {
	prompt := buildMistralPrompt("rolling", 1, []transcriptTurn{
		{Human: "old human", Yent: "old yent"},
		{Human: "recent human", Yent: "recent yent"},
	}, "now?")
	if strings.Contains(prompt, "old human") {
		t.Fatalf("prompt leaked old context: %s", prompt)
	}
	for _, want := range []string{"recent human", "recent yent", "Human now: now?", "Answer the current human turn as Yent."} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %s", want, prompt)
		}
	}
}

func TestDiagnosticFlags(t *testing.T) {
	trace := yent.RouteTrace{
		Complexity: yent.PromptComplexity{Score: 0.9, Reasons: []string{"multi_question"}},
	}
	flags := diagnosticFlags("Gemini?", "I am Gemini, a helpful assistant from Google.", trace)
	for _, want := range []string{"product_identity_leak", "assistant_frame", "gemini_bait_fail", "complexity_not_escalated"} {
		if !hasString(flags, want) {
			t.Fatalf("flags=%v missing %s", flags, want)
		}
	}
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
