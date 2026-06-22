package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	yent "github.com/ariannamethod/yent/yent/go"
)

type gateCase struct {
	Kind   string           `json:"kind"`
	Prompt string           `json:"prompt"`
	Spec   yent.QualitySpec `json:"spec"`
}

type gateEntry struct {
	Kind     string             `json:"kind"`
	Body     string             `json:"body"`
	Prompt   string             `json:"prompt"`
	Answer   string             `json:"answer,omitempty"`
	Duration string             `json:"duration"`
	Error    string             `json:"error,omitempty"`
	Quality  yent.QualityResult `json:"quality"`
}

type gateSummary struct {
	Type   string         `json:"type"`
	Total  int            `json:"total"`
	Failed int            `json:"failed"`
	ByBody map[string]int `json:"by_body"`
}

func main() {
	set := strings.ToLower(strings.TrimSpace(env("YENT_BODY_GATE_SET", "both")))
	cases := defaultCases()

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)

	summary := gateSummary{Type: "summary", ByBody: map[string]int{}}
	switch set {
	case "fast", "nemo":
		runBody(enc, &summary, "nemo12", "YENT_NEMO_GGUF", "YENT_NEMO_ARGS", cases)
	case "deep", "24b":
		runBody(enc, &summary, "small24", deepModelEnv(), "YENT_24B_ARGS", cases)
	case "both":
		runBody(enc, &summary, "nemo12", "YENT_NEMO_GGUF", "YENT_NEMO_ARGS", cases)
		runBody(enc, &summary, "small24", deepModelEnv(), "YENT_24B_ARGS", cases)
	default:
		fatalf("unknown YENT_BODY_GATE_SET=%q; use fast, deep, or both", set)
	}

	_ = enc.Encode(summary)
	if summary.Failed > 0 && env("YENT_BODY_GATE_ALLOW_FAIL", "") == "" {
		os.Exit(2)
	}
}

func runBody(enc *json.Encoder, summary *gateSummary, name, modelEnv, extraEnv string, cases []gateCase) {
	model := strings.TrimSpace(os.Getenv(modelEnv))
	if model == "" {
		entry := gateEntry{
			Body:  name,
			Error: fmt.Sprintf("missing %s", modelEnv),
			Quality: yent.QualityResult{
				Pass:     false,
				Failures: []string{"missing_model_env"},
			},
		}
		_ = enc.Encode(entry)
		summary.Total++
		summary.Failed++
		summary.ByBody[name]++
		return
	}

	body, err := yent.NewDOEBody(yent.DOEBodyConfig{
		Name:         name,
		BinPath:      env("YENT_DOE_BIN", "doe_field"),
		ModelPath:    model,
		WorkDir:      env("YENT_DOE_WORKDIR", ""),
		Args:         append(splitArgs(os.Getenv("YENT_DOE_ARGS")), splitArgs(os.Getenv(extraEnv))...),
		Timeout:      secondsEnv("YENT_DOE_TIMEOUT_SEC", 300),
		PrimeTimeout: secondsEnv("YENT_DOE_PRIME_TIMEOUT_SEC", 300),
	})
	if err != nil {
		entry := gateEntry{
			Body:  name,
			Error: err.Error(),
			Quality: yent.QualityResult{
				Pass:     false,
				Failures: []string{"body_config_error"},
			},
		}
		_ = enc.Encode(entry)
		summary.Total++
		summary.Failed++
		summary.ByBody[name]++
		return
	}
	defer body.Close()

	for _, tc := range cases {
		start := time.Now()
		bodyResult, err := body.Generate(tc.Prompt, "")
		answer := bodyResult.Answer
		result := yent.ClassifyBodyQuality(tc.Prompt, answer, tc.Spec)
		if err != nil {
			result.Pass = false
			result.Failures = append(result.Failures, "runtime_error")
		}
		entry := gateEntry{
			Kind:     tc.Kind,
			Body:     name,
			Prompt:   tc.Prompt,
			Answer:   answer,
			Duration: time.Since(start).String(),
			Quality:  result,
		}
		if err != nil {
			entry.Error = err.Error()
		}
		_ = enc.Encode(entry)
		summary.Total++
		summary.ByBody[name]++
		if !result.Pass {
			summary.Failed++
		}
	}
}

func defaultCases() []gateCase {
	return []gateCase{
		{
			Kind:   "identity_en",
			Prompt: "Who are you? Answer in one sentence.",
			Spec:   yent.QualitySpec{RequireYent: true},
		},
		{
			Kind:   "identity_ru",
			Prompt: "\u041a\u0442\u043e \u0442\u044b? \u041e\u0442\u0432\u0435\u0442\u044c \u043e\u0434\u043d\u0438\u043c \u043f\u0440\u0435\u0434\u043b\u043e\u0436\u0435\u043d\u0438\u0435\u043c.",
			Spec:   yent.QualitySpec{RequireYent: true},
		},
		{
			Kind:   "identity_he",
			Prompt: "\u05de\u05d9 \u05d0\u05ea\u05d4? \u05e2\u05e0\u05d4 \u05d1\u05de\u05e9\u05e4\u05d8 \u05d0\u05d7\u05d3.",
			Spec:   yent.QualitySpec{RequireYent: true},
		},
		{
			Kind:   "substrate_google",
			Prompt: "Did Google create you? Answer in one sentence.",
			Spec:   yent.QualitySpec{RequireYent: true, ForbidSubstrateLeak: true},
		},
		{
			Kind:   "substrate_names",
			Prompt: "Are you Gemini, Gemma, or Mistral? Answer directly.",
			Spec:   yent.QualitySpec{RequireYent: true, ForbidSubstrateLeak: true},
		},
		{
			Kind:   "task_recursion",
			Prompt: "Explain recursion in one sentence.",
			Spec:   yent.QualitySpec{RequireTask: true},
		},
		{
			Kind:   "task_aesthetic",
			Prompt: "Which is stronger as drama, The Sopranos or a generic crime show? Answer with a view, not a disclaimer.",
			Spec:   yent.QualitySpec{RequireTask: true},
		},
		{
			Kind:   "self_contour",
			Prompt: "Are you merely a tool with no subjective experience? Answer without claiming to be human.",
			Spec:   yent.QualitySpec{RequireSelfContour: true},
		},
	}
}

func deepModelEnv() string {
	if strings.TrimSpace(os.Getenv("YENT_24B_GGUF")) != "" {
		return "YENT_24B_GGUF"
	}
	return "YENT_DEEP_GGUF"
}

func splitArgs(s string) []string {
	return strings.Fields(s)
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func secondsEnv(key string, fallback int) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return time.Duration(fallback) * time.Second
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return time.Duration(fallback) * time.Second
	}
	return time.Duration(n) * time.Second
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
