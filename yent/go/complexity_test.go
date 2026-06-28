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
