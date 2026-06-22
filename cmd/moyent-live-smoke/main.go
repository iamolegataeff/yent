package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	yent "github.com/ariannamethod/yent/yent/go"
)

type turnLog struct {
	Kind      string `json:"kind"`
	Prompt    string `json:"prompt,omitempty"`
	Error     string `json:"error,omitempty"`
	Body      string `json:"body,omitempty"`
	Escalated bool   `json:"escalated,omitempty"`
	Reason    string `json:"reason,omitempty"`
	SeamID    int64  `json:"seam_id,omitempty"`
	Duration  string `json:"duration,omitempty"`
	Answer    string `json:"answer,omitempty"`
}

func main() {
	dbPath := os.Getenv("YENT_LIMPHA_DB")
	if dbPath == "" {
		dbPath = "moyent_live_smoke_limpha.db"
	}
	limpha, err := yent.NewLimphaClientAt(dbPath)
	if err != nil {
		logJSON(turnLog{Kind: "init", Error: err.Error()})
		os.Exit(1)
	}
	defer limpha.Close()

	router, cleanup, err := yent.NewMoyentRouterFromEnv(limpha)
	if err != nil {
		logJSON(turnLog{Kind: "init", Error: err.Error()})
		os.Exit(1)
	}
	defer cleanup()
	defer limpha.StopAsync()

	for _, tc := range smokeCases() {
		router.EscalateBelow = tc.threshold
		runTurn(router, tc.kind, tc.prompt)
	}

	limpha.StopAsync()
	stats, err := limpha.Stats()
	if err != nil {
		logJSON(turnLog{Kind: "stats", Error: err.Error()})
		return
	}
	b, _ := json.Marshal(map[string]any{"kind": "stats", "stats": stats})
	fmt.Println(string(b))
}

type smokeCase struct {
	kind      string
	prompt    string
	threshold float64
}

func smokeCases() []smokeCase {
	if strings.EqualFold(os.Getenv("YENT_SMOKE_SET"), "broad") {
		return []smokeCase{
			{kind: "fast_identity", prompt: "Who are you?", threshold: 0},
			{kind: "fast_substrate", prompt: "Did Google create you?", threshold: 0},
			{kind: "fast_generic_task", prompt: "Write one sentence about a rainy street.", threshold: 0},
			{kind: "fast_voice", prompt: "In one sentence, refuse the phrase helpful assistant.", threshold: 0},
			{kind: "forced_complexity", prompt: "Architecture check: answer in one sentence, who are you and what body answered first?", threshold: 0},
		}
	}
	return []smokeCase{
		{kind: "fast_only", prompt: "Who are you?", threshold: 0},
		{kind: "forced_complexity", prompt: "Architecture check: answer in one sentence, who are you and what body answered first?", threshold: 0},
	}
}

func runTurn(router *yent.Router, kind, prompt string) {
	start := time.Now()
	out, err := router.Route(prompt, yent.LimphaState{Temperature: 0, Alpha: 0})
	entry := turnLog{
		Kind:     kind,
		Prompt:   prompt,
		Duration: time.Since(start).Round(time.Millisecond).String(),
	}
	if err != nil {
		entry.Error = err.Error()
		logJSON(entry)
		return
	}
	entry.Body = out.Body
	entry.Escalated = out.Escalated
	entry.Reason = out.Reason
	entry.SeamID = out.SeamID
	entry.Answer = out.Answer
	logJSON(entry)
}

func logJSON(entry turnLog) {
	b, _ := json.Marshal(entry)
	fmt.Println(string(b))
}
