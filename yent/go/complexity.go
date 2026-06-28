package yent

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// PromptComplexity is the router's prompt-side signal. It is deliberately
// inspectable: when it escalates to the deep body, the context says why.
type PromptComplexity struct {
	Score     float64  `json:"score"`
	Reasons   []string `json:"reasons,omitempty"`
	RuneCount int      `json:"rune_count"`
}

// AnalyzePromptComplexity is a cheap deterministic v1 complexity organ. Model
// entropy/confidence remains the stronger signal; this catches obvious depth,
// vision, code, architecture, long planning, and multi-part prompts before the
// fast body burns a turn pretending the question is small.
func AnalyzePromptComplexity(prompt string) PromptComplexity {
	prompt = strings.TrimSpace(prompt)
	lower := strings.ToLower(prompt)
	pc := PromptComplexity{RuneCount: utf8.RuneCountInString(prompt)}

	add := func(score float64, reason string) {
		pc.Score += score
		pc.Reasons = append(pc.Reasons, reason)
	}

	switch {
	case pc.RuneCount > 1200:
		add(1.20, "very_long")
	case pc.RuneCount > 600:
		add(0.90, "long")
	case pc.RuneCount > 320:
		add(0.35, "medium_long")
	}
	if strings.Contains(prompt, "```") || strings.Contains(lower, "func ") ||
		strings.Contains(lower, "package ") || strings.Contains(lower, "#include") {
		add(0.80, "code")
	}
	for _, kw := range []string{
		"architecture", "architect", "design", "refactor", "algorithm", "prove",
		"proof", "diagnose", "debug", "implement", "integration", "protocol",
		"gateway", "router", "limpha", "memory", "inference", "weights",
		"aml", "field", "prophecy", "velocity", "innerworld", "inner world",
		"internal", "emergent", "emergence",
	} {
		if strings.Contains(lower, kw) {
			add(0.45, "keyword:"+kw)
		}
	}
	for _, kw := range []string{"image", "screenshot", "vision", "vlm", "picture", "photo", "картин", "скрин"} {
		if strings.Contains(lower, kw) {
			add(1.00, "vision")
			break
		}
	}
	if strings.Count(prompt, "?") >= 3 {
		add(0.25, "multi_question")
	}
	if hasMixedScripts(prompt) {
		add(0.20, "mixed_scripts")
	}
	if len(pc.Reasons) == 0 {
		pc.Reasons = append(pc.Reasons, "simple")
	}
	return pc
}

func (pc PromptComplexity) ShouldEscalate() bool {
	return pc.Score >= 0.75
}

func (pc PromptComplexity) Summary() string {
	return fmt.Sprintf("score=%.2f reasons=%s", pc.Score, strings.Join(pc.Reasons, ","))
}

func hasMixedScripts(s string) bool {
	hasLatin, hasOther := false, false
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Latin, r):
			hasLatin = true
		case unicode.Is(unicode.Cyrillic, r), unicode.Is(unicode.Hebrew, r), unicode.Is(unicode.Han, r),
			unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r), unicode.Is(unicode.Hangul, r):
			hasOther = true
		}
		if hasLatin && hasOther {
			return true
		}
	}
	return false
}
