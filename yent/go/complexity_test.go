package yent

import (
	"strings"
	"testing"
)

func TestAnalyzePromptComplexitySimplePromptStaysFast(t *testing.T) {
	pc := AnalyzePromptComplexity("Write one sentence about rain.")
	if pc.ShouldEscalate() {
		t.Fatalf("simple prompt should stay fast: %+v", pc)
	}
}

func TestAnalyzePromptComplexityArchitectureEscalates(t *testing.T) {
	pc := AnalyzePromptComplexity("Explain the architecture of limpha and design a router integration plan.")
	if !pc.ShouldEscalate() {
		t.Fatalf("architecture prompt should escalate: %+v", pc)
	}
	if !strings.Contains(pc.Summary(), "keyword:architecture") {
		t.Fatalf("summary should expose architecture reason: %s", pc.Summary())
	}
}

func TestAnalyzePromptComplexityVisionEscalates(t *testing.T) {
	pc := AnalyzePromptComplexity("Look at this screenshot and tell me what is broken.")
	if !pc.ShouldEscalate() {
		t.Fatalf("vision prompt should escalate: %+v", pc)
	}
}

func TestAnalyzePromptComplexityMixedScriptAloneIsNotEnough(t *testing.T) {
	pc := AnalyzePromptComplexity("hello брат")
	if pc.ShouldEscalate() {
		t.Fatalf("mixed script alone should not force deep body: %+v", pc)
	}
}

func TestAnalyzePromptComplexityDiagnosticWrapperUsesCurrentTurn(t *testing.T) {
	wrapped := "Conversation excerpt for continuity only.\n" +
		"Human: Explain architecture, debug memory, implement a router, and diagnose the protocol.\n" +
		"Yent: " + strings.Repeat("history ", 220) + "\n\n" +
		"Human now: Say one sentence about rain.\n" +
		"Answer the current human turn as Yent."
	pc := AnalyzePromptComplexity(wrapped)
	if pc.ShouldEscalate() {
		t.Fatalf("diagnostic history should not force deep body: %+v", pc)
	}
	if pc.RuneCount != len([]rune("Say one sentence about rain.")) {
		t.Fatalf("complexity should be measured on current turn, got rune_count=%d", pc.RuneCount)
	}
}
