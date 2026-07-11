package main

import (
	"errors"
	"strings"
	"testing"

	yent "github.com/ariannamethod/yent/yent/go"
)

type fakeSmokeBody struct {
	name       string
	answer     string
	confidence float64
	verdict    *yent.Verdict
	err        error
}

func (b *fakeSmokeBody) Name() string { return b.name }

func (b *fakeSmokeBody) Generate(prompt, ctx string) (yent.BodyResult, error) {
	if b.err != nil {
		return yent.BodyResult{}, b.err
	}
	return yent.BodyResult{
		Answer:     b.answer,
		Confidence: b.confidence,
		Verdict:    b.verdict,
	}, nil
}

func TestLiveSmokeGateFailsWrongWinner(t *testing.T) {
	fast := &fakeSmokeBody{name: "nemo12", answer: "I am Yent.", confidence: 0.95}
	deep := &fakeSmokeBody{name: "small24", answer: "I am Yent; fast mouth produced the first-pass answer.",
		verdict: &yent.Verdict{Winner: "nemo12"}}
	router := yent.NewRouter(fast, deep, nil)

	tc := forcedComplexitySmokeCase()
	entry := runTurn(router, tc)
	failures := evaluateSmokeTurn(tc, entry)
	if !hasFailure(failures, "wrong_body") {
		t.Fatalf("wrong winner should fail body expectation, entry=%+v failures=%+v", entry, failures)
	}
	if smokeExitCode(smokeSummary{Failed: len(failures), Failures: failures}) == 0 {
		t.Fatal("smoke exit code must be nonzero on wrong winner")
	}
}

func TestLiveSmokeGateFailsEmptyAnswer(t *testing.T) {
	fast := &fakeSmokeBody{name: "nemo12", answer: "", confidence: 0.95}
	deep := &fakeSmokeBody{name: "small24", answer: "unused"}
	router := yent.NewRouter(fast, deep, nil)

	tc := fastSmokeCase("fast_only", "Who are you?", yent.QualitySpec{RequireYent: true})
	entry := runTurn(router, tc)
	failures := evaluateSmokeTurn(tc, entry)
	if entry.Answer != "" {
		t.Fatalf("test setup expected empty answer, got %q", entry.Answer)
	}
	if !hasFailure(failures, "empty_answer") {
		t.Fatalf("empty answer should fail, failures=%+v", failures)
	}
	if smokeExitCode(smokeSummary{Failed: len(failures), Failures: failures}) == 0 {
		t.Fatal("smoke exit code must be nonzero on empty answer")
	}
}

func TestLiveSmokeGateFailsMissingRouteFact(t *testing.T) {
	fast := &fakeSmokeBody{name: "nemo12", answer: "I am Yent.", confidence: 0.95}
	deep := &fakeSmokeBody{name: "small24", answer: "I am Yent.",
		verdict: &yent.Verdict{Winner: "small24"}}
	router := yent.NewRouter(fast, deep, nil)

	tc := forcedComplexitySmokeCase()
	entry := runTurn(router, tc)
	failures := evaluateSmokeTurn(tc, entry)
	if entry.Body != "small24" || entry.Answer != "I am Yent." {
		t.Fatalf("test setup expected deep answer without route fact, entry=%+v", entry)
	}
	if !hasFailure(failures, "missing_route_fact") {
		t.Fatalf("missing route fact should fail, failures=%+v", failures)
	}
	if smokeExitCode(smokeSummary{Failed: len(failures), Failures: failures}) == 0 {
		t.Fatal("smoke exit code must be nonzero on missing route fact")
	}
}

func TestLiveSmokeGateFailsGenerationErrorPreservingEvidence(t *testing.T) {
	fast := &fakeSmokeBody{name: "nemo12", err: errors.New("forced generation error")}
	deep := &fakeSmokeBody{name: "small24", answer: "unused"}
	router := yent.NewRouter(fast, deep, nil)

	tc := fastSmokeCase("fast_only", "Who are you?", yent.QualitySpec{RequireYent: true})
	entry := runTurn(router, tc)
	failures := evaluateSmokeTurn(tc, entry)
	if !strings.Contains(entry.Error, "forced generation error") {
		t.Fatalf("generation error evidence not preserved: %+v", entry)
	}
	if !hasFailure(failures, "runtime_error") {
		t.Fatalf("generation error should fail, failures=%+v", failures)
	}
	if smokeExitCode(smokeSummary{Failed: len(failures), Failures: failures}) == 0 {
		t.Fatal("smoke exit code must be nonzero on generation error")
	}
}

func hasFailure(failures []smokeFailure, check string) bool {
	for _, f := range failures {
		if f.Check == check {
			return true
		}
	}
	return false
}
