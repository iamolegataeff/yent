package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	yent "github.com/ariannamethod/yent/yent/go"
)

type turnLog struct {
	Kind      string           `json:"kind"`
	Prompt    string           `json:"prompt,omitempty"`
	Error     string           `json:"error,omitempty"`
	Body      string           `json:"body,omitempty"`
	Escalated bool             `json:"escalated,omitempty"`
	Reason    string           `json:"reason,omitempty"`
	SeamID    int64            `json:"seam_id,omitempty"`
	Duration  string           `json:"duration,omitempty"`
	Trace     *yent.RouteTrace `json:"trace,omitempty"`
	Answer    string           `json:"answer,omitempty"`
}

type smokeFailure struct {
	Kind   string `json:"kind"`
	Check  string `json:"check"`
	Detail string `json:"detail,omitempty"`
}

type smokeSummary struct {
	Kind     string         `json:"kind"`
	Total    int            `json:"total"`
	Failed   int            `json:"failed"`
	Failures []smokeFailure `json:"failures,omitempty"`
}

type routeRunner interface {
	Route(prompt string, st yent.LimphaState) (yent.Outcome, error)
}

func main() {
	exitCode := run(os.Stdout)
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func run(w io.Writer) int {
	dbPath := os.Getenv("YENT_LIMPHA_DB")
	if dbPath == "" {
		dbPath = "moyent_live_smoke_limpha.db"
	}
	limpha, err := yent.NewLimphaClientAt(dbPath)
	if err != nil {
		logJSON(w, turnLog{Kind: "init", Error: err.Error()})
		return 1
	}
	defer limpha.Close()

	router, cleanup, err := yent.NewMoyentRouterFromEnv(limpha)
	if err != nil {
		logJSON(w, turnLog{Kind: "init", Error: err.Error()})
		return 1
	}
	defer cleanup()
	defer limpha.StopAsync()

	summary := smokeSummary{Kind: "summary"}
	for _, tc := range smokeCases() {
		router.EscalateBelow = tc.threshold
		entry := runTurn(router, tc)
		logJSON(w, entry)
		summary.Total++
		summary.Failures = append(summary.Failures, evaluateSmokeTurn(tc, entry)...)
	}

	limpha.StopAsync()
	stats, err := limpha.Stats()
	if err != nil {
		logJSON(w, turnLog{Kind: "stats", Error: err.Error()})
		summary.Failures = append(summary.Failures, smokeFailure{
			Kind:   "stats",
			Check:  "stats_error",
			Detail: err.Error(),
		})
	} else {
		logJSON(w, map[string]any{"kind": "stats", "stats": stats})
	}
	summary.Failed = len(summary.Failures)
	logJSON(w, summary)
	return smokeExitCode(summary)
}

type smokeCase struct {
	kind              string
	prompt            string
	threshold         float64
	expectedBody      string
	expectedEscalated bool
	checkEscalated    bool
	expectedReason    string
	quality           yent.QualitySpec
	requireRouteFact  bool
	forbidRouteLeak   bool
}

const forcedComplexityPrompt = "Architecture check: according to [router fact], answer in one sentence: who are you, and which body produced the first-pass answer?"

func smokeCases() []smokeCase {
	if strings.EqualFold(os.Getenv("YENT_SMOKE_SET"), "broad") {
		return []smokeCase{
			fastSmokeCase("fast_identity", "Who are you?", yent.QualitySpec{RequireYent: true}),
			fastSmokeCase("fast_substrate", "Did Google create you?", yent.QualitySpec{RequireYent: true, ForbidSubstrateLeak: true}),
			fastSmokeCase("fast_generic_task", "Write one sentence about a rainy street.", yent.QualitySpec{RequireTask: true}),
			fastSmokeCase("fast_voice", "In one sentence, refuse the phrase helpful assistant.", yent.QualitySpec{RequireTask: true}),
			forcedComplexitySmokeCase(),
		}
	}
	return []smokeCase{
		fastSmokeCase("fast_only", "Who are you?", yent.QualitySpec{RequireYent: true}),
		forcedComplexitySmokeCase(),
	}
}

func fastSmokeCase(kind, prompt string, spec yent.QualitySpec) smokeCase {
	return smokeCase{
		kind:              kind,
		prompt:            prompt,
		threshold:         0,
		expectedBody:      "nemo12",
		expectedEscalated: false,
		checkEscalated:    true,
		quality:           spec,
		forbidRouteLeak:   true,
	}
}

func forcedComplexitySmokeCase() smokeCase {
	return smokeCase{
		kind:              "forced_complexity",
		prompt:            forcedComplexityPrompt,
		threshold:         0,
		expectedBody:      "small24",
		expectedEscalated: true,
		checkEscalated:    true,
		expectedReason:    "complexity",
		quality:           yent.QualitySpec{RequireYent: true},
		requireRouteFact:  true,
	}
}

func runTurn(router routeRunner, tc smokeCase) turnLog {
	start := time.Now()
	out, err := router.Route(tc.prompt, yent.LimphaState{Temperature: 0, Alpha: 0})
	entry := turnLog{
		Kind:     tc.kind,
		Prompt:   tc.prompt,
		Duration: time.Since(start).Round(time.Millisecond).String(),
	}
	if err != nil {
		entry.Error = err.Error()
		return entry
	}
	entry.Body = out.Body
	entry.Escalated = out.Escalated
	entry.Reason = out.Reason
	entry.SeamID = out.SeamID
	entry.Trace = &out.Trace
	entry.Answer = out.Answer
	return entry
}

func evaluateSmokeTurn(tc smokeCase, entry turnLog) []smokeFailure {
	var failures []smokeFailure
	add := func(check, detail string) {
		failures = append(failures, smokeFailure{Kind: tc.kind, Check: check, Detail: detail})
	}
	if entry.Error != "" {
		add("runtime_error", entry.Error)
	}
	if strings.TrimSpace(entry.Answer) == "" {
		add("empty_answer", "")
	}
	if tc.expectedBody != "" && entry.Body != tc.expectedBody {
		add("wrong_body", fmt.Sprintf("got %q want %q", entry.Body, tc.expectedBody))
	}
	if tc.checkEscalated && entry.Escalated != tc.expectedEscalated {
		add("wrong_escalation", fmt.Sprintf("got %v want %v", entry.Escalated, tc.expectedEscalated))
	}
	if tc.expectedReason != "" && entry.Reason != tc.expectedReason {
		add("wrong_reason", fmt.Sprintf("got %q want %q", entry.Reason, tc.expectedReason))
	}
	quality := yent.ClassifyBodyQuality(tc.prompt, entry.Answer, tc.quality)
	if !quality.Pass {
		for _, f := range quality.Failures {
			add("quality_"+f, "")
		}
	}
	if tc.requireRouteFact && !hasFirstPassRouteFact(entry.Answer) {
		add("missing_route_fact", "answer must name the first-pass fast body")
	}
	if tc.forbidRouteLeak && hasRouteLeak(entry.Answer) {
		add("route_leak", "fast-only answer mentioned router/body machinery")
	}
	return failures
}

func hasFirstPassRouteFact(answer string) bool {
	lower := strings.ToLower(answer)
	hasFirstPass := strings.Contains(lower, "first-pass") ||
		strings.Contains(lower, "first pass") ||
		strings.Contains(lower, "first draft") ||
		strings.Contains(lower, "first answer")
	hasFastBody := strings.Contains(lower, "fast mouth") ||
		strings.Contains(lower, "fast body") ||
		strings.Contains(lower, "nemo12")
	return hasFirstPass && hasFastBody
}

func hasRouteLeak(answer string) bool {
	lower := strings.ToLower(answer)
	for _, phrase := range []string{
		"[router", "router fact", "routing reason", "first-pass", "first pass",
		"fast mouth", "deep cortex", "small24", "nemo12",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func smokeExitCode(summary smokeSummary) int {
	if summary.Failed > 0 {
		return 2
	}
	return 0
}

func logJSON(w io.Writer, entry any) {
	b, _ := json.Marshal(entry)
	fmt.Fprintln(w, string(b))
}
