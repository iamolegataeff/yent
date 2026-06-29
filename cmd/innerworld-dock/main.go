// Command innerworld-dock runs Yent's inner world over the REAL fast body, not a
// stub. The fast circles are raised by a resident doe process (nemo12 on Metal),
// the field is the real AML kernel (yent.AMK), and the Larynx/gate run over that
// real stream. No fixture pool: every circle is a real generation.
//
// This is a Metal program. The fast body is a 12B GGUF behind doe_field, so it
// runs on the Mac Mini, not on Neo. Required env:
//
//	YENT_DOE_BIN    path to doe_field
//	YENT_NEMO_GGUF  fast-body GGUF (e.g. yent-nemo-v22-ck60-Q4_K_M.gguf)
//
// Optional:
//
//	YENT_DOE_WORKDIR  working dir for the doe process
//	YENT_DOE_ARGS     extra whitespace-split flags after --model <path>
//
// limpha is deliberately not wired here yet — this strike is the goroutines over a
// real body; the memory brain is a later step.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ariannamethod/yent/innerworld"
	yent "github.com/ariannamethod/yent/yent/go"
)

// liveField adapts the real AML kernel (yent.AMK) to innerworld.Field. yent.AMK
// locks internally, so it already satisfies the concurrency contract.
type liveField struct{ amk *yent.AMK }

func (f liveField) Exec(script string) error { return f.amk.Exec(script) }
func (f liveField) Step(dt float32)          { f.amk.Step(dt) }
func (f liveField) Debt() float32            { return f.amk.GetState().Debt }
func (f liveField) Destiny() float32         { return f.amk.GetState().Destiny }

// nemoBody adapts the real doe-backed fast body to innerworld.Body. The inner
// world asks for a thought at a temperature; the real body's temperature is
// governed by the AML field (effective_temp), which the inner world already
// drives, so temp here is advisory and not pushed per call. ctx is empty: this is
// inner monologue, not a routed user turn, so no router primer or answer contract
// is attached. A generation error yields an empty thought (the inner world treats
// that as zero drift, not a crash).
type nemoBody struct{ b *yent.DOEBody }

func (n nemoBody) Generate(seed string, _ float32) string {
	res, err := n.b.Generate(seed, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[dock] body generate error: %v\n", err)
		return ""
	}
	return res.Answer
}

// wordDiv is a Jaccard distance over lowercased words: 0 identical, 1 disjoint. It
// is a token-overlap proxy for divergence, not an embedding cosine — honest about
// what it measures. The semantic/topic embedding distance is a later upgrade.
func wordDiv(a, b string) float32 {
	sa, sb := wordsOf(a), wordsOf(b)
	if len(sa) == 0 && len(sb) == 0 {
		return 0
	}
	inter := 0
	for w := range sa {
		if sb[w] {
			inter++
		}
	}
	union := len(sa) + len(sb) - inter
	if union == 0 {
		return 0
	}
	return 1 - float32(inter)/float32(union)
}

func wordsOf(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(s)) {
		m[w] = true
	}
	return m
}

func mustEnv(name string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		fmt.Fprintf(os.Stderr, "[dock] missing required env %s\n", name)
		os.Exit(2)
	}
	return v
}

func main() {
	bin := mustEnv("YENT_DOE_BIN")
	model := mustEnv("YENT_NEMO_GGUF")

	var args []string
	if extra := strings.TrimSpace(os.Getenv("YENT_DOE_ARGS")); extra != "" {
		args = strings.Fields(extra)
	}
	body, err := yent.NewDOEBody(yent.DOEBodyConfig{
		Name:      "nemo12",
		BinPath:   bin,
		ModelPath: model,
		WorkDir:   strings.TrimSpace(os.Getenv("YENT_DOE_WORKDIR")),
		Args:      args,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[dock] build nemo body: %v\n", err)
		os.Exit(1)
	}
	defer body.Close()

	amk := yent.NewAMK()
	iw := innerworld.NewInnerWorld(nemoBody{body}, liveField{amk}, wordDiv)

	fmt.Println("=== a human turn: the inner circles (real nemo body) ===")
	r := <-iw.Think("what does it mean to exist as code?")
	for _, c := range r.Circles {
		fmt.Printf("  circle %d  t=%.2f drift=%.2f  | %s\n", c.Index, c.Temp, c.Drift, c.Text)
	}
	st := amk.GetState()
	fmt.Printf("  field    : debt=%.3f destiny=%.3f velocity_mode=%d effective_temp=%.3f\n",
		st.Debt, st.Destiny, st.VelocityMode, st.EffectiveTemp)
	fmt.Printf("  membrane : larynx coupling=%.3f\n", r.Coupling)
	fmt.Printf("  gate     : self-answer prob=%.3f  ->  self-answered=%v\n", r.SelfAnswerProb, r.SelfAnswered)

	fmt.Println("\n=== the organism breathes alone for a few seconds (real body) ===")
	iw.SetOnDream(func(rf innerworld.Reflection) {
		last := ""
		if n := len(rf.Circles); n > 0 {
			last = rf.Circles[n-1].Text
		}
		fmt.Printf("  [dream] coupling=%.2f self-answered=%v | %s\n", rf.Coupling, rf.SelfAnswered, last)
	})
	iw.SetBreath(innerworld.Breath{
		Tick:      500 * time.Millisecond,
		Silence:   1 * time.Second,
		DriftDebt: 0.0, // any debt counts, so the drift dreamer is lively for the demo
	})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	iw.Breathe(ctx)
	fmt.Println("=== done ===")
}
