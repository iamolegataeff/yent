// Command innerworld-dock runs Yent's inner world over the REAL fast body, not a
// stub. The fast circles are raised by a resident doe process (nemo12 on Metal),
// the field is the real AML kernel, and the Larynx/gate run over that real stream.
// No fixture pool: every circle is a real generation.
//
// The field is read through the canonical ariannamethod.h directly — the same
// header libamk.a is built from — NOT through yent.AMK.GetState(). yent.AMK's Go
// struct compiles against the older amk_kernel.h, whose AM_State layout diverged
// from the canonical (canonical added `int field_enabled` after prophecy), so on a
// canonical-built libamk.a every field past prophecy reads at the wrong offset.
// Reading canonical here keeps velocity/destiny/debt true. (Tracked in YENTLOG.)
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
//	YENT_LIMPHA_DB    optional limpha db path; when set, inner reflections are stored
//	YENT_DOCK_MAX_DREAMS optional autonomous dream cap for finite receipts
package main

/*
#cgo CFLAGS: -I${SRCDIR}/../../yent/c
#cgo LDFLAGS: ${SRCDIR}/../../yent/c/libamk.a
#include "ariannamethod.h"
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/ariannamethod/yent/innerworld"
	yent "github.com/ariannamethod/yent/yent/go"
)

// amkField drives the real AML field through the canonical kernel. Concurrency-safe,
// as the Field contract requires. Reads go through the canonical AM_State layout.
type amkField struct{ mu sync.Mutex }

func (f *amkField) Exec(script string) error {
	cs := C.CString(script)
	defer C.free(unsafe.Pointer(cs))
	f.mu.Lock()
	defer f.mu.Unlock()
	if C.am_exec(cs) != 0 {
		return fmt.Errorf("am_exec failed: %q", script)
	}
	return nil
}
func (f *amkField) Step(dt float32) { f.mu.Lock(); defer f.mu.Unlock(); C.am_step(C.float(dt)) }
func (f *amkField) Debt() float32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return float32(C.am_get_state().debt)
}
func (f *amkField) Destiny() float32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return float32(C.am_get_state().destiny)
}

// doeBody adapts a real doe-backed body (nemo12 fast or small24 deep) to
// innerworld.Body. The inner world asks for a thought at a temperature; the real
// body's temperature is governed by the AML field (effective_temp), which the
// inner world already drives, so temp here is advisory and not pushed per call.
// ctx is empty: this is inner monologue, not a routed user turn, so no router
// primer or answer contract is attached. A generation error yields an empty
// thought (the inner world treats that as zero drift, not a crash). Close frees
// the resident doe process for the inner world's single-resident swap.
type doeBody struct{ b *yent.DOEBody }

func (d doeBody) Generate(seed string, _ float32) string {
	res, err := d.b.Generate(seed, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[dock] body generate error: %v\n", err)
		return ""
	}
	return res.Answer
}

func (d doeBody) Close() error { return d.b.Close() }

// limphaRecaller is the read side of the memory loop: it recalls Yent's own past
// inner monologues from limpha — the seams the dock persisted (Codex's write side) —
// so new overthinking is shaped by what it thought before. Only inner reflections
// are recalled (reason contains "innerworld"), never router turns; the deep inner
// answer (b_claim) is preferred over the circle stream (a_claim).
type limphaRecaller struct{ lc *yent.LimphaClient }

func (m limphaRecaller) Recall(n int) []string {
	if m.lc == nil || n <= 0 {
		return nil
	}
	seams, err := m.lc.RecentSeams(n * 3) // over-fetch, then filter to inner seams
	if err != nil {
		return nil
	}
	out := make([]string, 0, n)
	for _, s := range seams {
		if reason, _ := s["reason"].(string); !strings.Contains(reason, "innerworld") {
			continue
		}
		thought := ""
		if b, ok := s["b_claim"].(string); ok && strings.TrimSpace(b) != "" {
			thought = b // the deep inner answer — the furthest thought of that monologue
		} else if a, ok := s["a_claim"].(string); ok {
			thought = a // fall back to the circle stream
		}
		thought = strings.Join(strings.Fields(thought), " ") // compact whitespace
		if r := []rune(thought); len(r) > 240 {
			thought = string(r[:240]) // rune-safe cap so the seed stays compact
		}
		if thought != "" {
			out = append(out, thought)
		}
		if len(out) >= n {
			break
		}
	}
	return out
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

// durationEnv reads a positive seconds value, or 0 to let NewDOEBody use its
// default. A first generation also pays the prime, so the deep 24B body needs a
// generous YENT_DOE_TIMEOUT_SEC (the 45s default is too tight for it).
func durationEnv(name string) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	// reject NaN (v != v), +Inf / overflow (v > maxSec), and non-positive — any of
	// these would make time.Duration(v*…) implementation-defined garbage.
	maxSec := float64(time.Duration(1<<63-1) / time.Second)
	if err != nil || v <= 0 || v != v || v > maxSec {
		return 0
	}
	return time.Duration(v * float64(time.Second))
}

func positiveIntEnv(name string) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0
	}
	return v
}

func newBody(name, bin, model, workdir string, args []string) *yent.DOEBody {
	b, err := yent.NewDOEBody(yent.DOEBodyConfig{
		Name: name, BinPath: bin, ModelPath: model, WorkDir: workdir, Args: args,
		Timeout:      durationEnv("YENT_DOE_TIMEOUT_SEC"),
		PrimeTimeout: durationEnv("YENT_DOE_PRIME_TIMEOUT_SEC"),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[dock] build %s body: %v\n", name, err)
		os.Exit(1)
	}
	return b
}

func openLimphaFromEnv() *yent.LimphaClient {
	path := strings.TrimSpace(os.Getenv("YENT_LIMPHA_DB"))
	if path == "" {
		return nil
	}
	lc, err := yent.NewLimphaClientAt(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[dock] limpha open %s: %v\n", path, err)
		os.Exit(1)
	}
	fmt.Printf("=== limpha wired: inner reflections -> %s ===\n", path)
	return lc
}

func limphaStateFromCanonical() yent.LimphaState {
	st := C.am_get_state()
	return yent.LimphaState{
		Temperature: float32(st.effective_temp),
		Destiny:     float32(st.destiny),
		Pain:        float32(st.pain),
		Tension:     float32(st.tension),
		Debt:        float32(st.debt),
		Velocity:    int(st.velocity_mode),
	}
}

type innerReflectionTrace struct {
	Kind           string  `json:"kind"`
	Source         string  `json:"source"`
	Circles        int     `json:"circles"`
	Coupling       float32 `json:"coupling"`
	SelfAnswerProb float32 `json:"self_answer_prob"`
	SelfAnswered   bool    `json:"self_answered"`
}

func persistReflection(lc *yent.LimphaClient, kind, source string, r innerworld.Reflection, st yent.LimphaState) {
	if lc == nil {
		return
	}
	circleStream := formatCircleStream(r.Circles)
	if circleStream == "" && strings.TrimSpace(r.DeepAnswer) == "" {
		return
	}
	prompt := "[innerworld/" + kind + "] " + strings.TrimSpace(source)
	response := circleStream
	if response == "" {
		response = strings.TrimSpace(r.DeepAnswer)
	}
	conversationID, _ := lc.StoreTurn(prompt, response, st)
	if strings.TrimSpace(r.DeepAnswer) == "" {
		return
	}
	trace := innerReflectionTrace{
		Kind:           "innerworld_reflection",
		Source:         kind,
		Circles:        len(r.Circles),
		Coupling:       r.Coupling,
		SelfAnswerProb: r.SelfAnswerProb,
		SelfAnswered:   r.SelfAnswered,
	}
	if source = strings.TrimSpace(source); source != "" {
		trace.Source = kind + ":" + source
	}
	delta, _ := json.Marshal(trace)
	_, _ = lc.StoreSeam(yent.Seam{
		ConversationID: conversationID,
		BodyA:          "nemo12",
		BodyB:          "small24",
		Prompt:         prompt,
		AClaim:         circleStream,
		BClaim:         strings.TrimSpace(r.DeepAnswer),
		Agreement:      float64(r.Coupling),
		Tension:        float64(r.SelfAnswerProb),
		Winner:         "small24",
		Reason:         "innerworld_self_answer",
		MemoryDelta:    string(delta),
	})
}

func formatCircleStream(circles []innerworld.Circle) string {
	var b strings.Builder
	for _, c := range circles {
		if strings.TrimSpace(c.Text) == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "circle %d temp=%.2f drift=%.2f | %s", c.Index, c.Temp, c.Drift, c.Text)
	}
	return b.String()
}

func main() {
	bin := mustEnv("YENT_DOE_BIN")
	fastModel := mustEnv("YENT_NEMO_GGUF")
	deepModel := strings.TrimSpace(os.Getenv("YENT_24B_GGUF")) // optional deep body (small24)
	workdir := strings.TrimSpace(os.Getenv("YENT_DOE_WORKDIR"))

	var args []string
	if extra := strings.TrimSpace(os.Getenv("YENT_DOE_ARGS")); extra != "" {
		args = strings.Fields(extra)
	}

	fast := newBody("nemo12", bin, fastModel, workdir, args)
	defer fast.Close()
	limpha := openLimphaFromEnv()
	if limpha != nil {
		defer limpha.Close()
	}

	C.am_init()
	field := &amkField{}
	iw := innerworld.NewInnerWorld(doeBody{fast}, field, wordDiv)

	// Close the loop: recall past inner monologues from limpha so new thinking is
	// shaped by what Yent thought before. The write side (dock -> limpha) lands the
	// seams; this reads them back into the seed.
	if limpha != nil {
		recaller := limphaRecaller{limpha}
		iw.SetMemory(recaller)
		if past := recaller.Recall(3); len(past) > 0 {
			fmt.Printf("=== recall wired: %d past inner monologue(s) fold into this turn ===\n", len(past))
			for i, p := range past {
				fmt.Printf("  recalled %d | %s\n", i, p)
			}
		} else {
			fmt.Println("=== recall wired: no past inner monologues yet (first run) ===")
		}
	}

	deepWired := false
	if deepModel != "" {
		deep := newBody("small24", bin, deepModel, workdir, args)
		defer deep.Close()
		iw.SetDeep(doeBody{deep})
		deepWired = true
		fmt.Println("=== deep body small24 wired: when the gate fires, it answers the circles (single-resident swap) ===")
	} else {
		fmt.Println("=== no YENT_24B_GGUF: gate stays a boolean, no deep self-answer ===")
	}

	// Smoke aid: force the gate so the deep wiring is provable in one run. Default is
	// the unpredictable real roll.
	if os.Getenv("YENT_DOCK_FORCE_GATE") == "1" {
		iw.SetRoll(func() float32 { return 0 })
		fmt.Println("    (YENT_DOCK_FORCE_GATE=1: gate forced open to prove the small24 path)")
	}

	// Signal-aware: SIGINT/SIGTERM cancels the wait and the breath so the deferred
	// Close calls still run — the doe daemons are reaped, not orphaned.
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Println("=== a human turn: the inner circles (real nemo body) ===")
	var r innerworld.Reflection
	select {
	case r = <-iw.Think("what does it mean to exist as code?"):
	case <-sigCtx.Done():
		fmt.Println("  (interrupted — closing bodies)")
		return
	}
	for _, c := range r.Circles {
		fmt.Printf("  circle %d  t=%.2f drift=%.2f  | %s\n", c.Index, c.Temp, c.Drift, c.Text)
	}
	st := C.am_get_state()
	fmt.Printf("  field    : debt=%.3f destiny=%.3f velocity_mode=%d effective_temp=%.3f\n",
		float32(st.debt), float32(st.destiny), int(st.velocity_mode), float32(st.effective_temp))
	fmt.Printf("  membrane : larynx coupling=%.3f\n", r.Coupling)
	fmt.Printf("  gate     : self-answer prob=%.3f  ->  self-answered=%v\n", r.SelfAnswerProb, r.SelfAnswered)
	if r.DeepAnswer != "" {
		fmt.Printf("  deep     : small24 inner answer | %s\n", r.DeepAnswer)
	} else if deepWired {
		fmt.Println("  deep     : (gate did not fire — small24 stayed silent this turn)")
	}
	persistReflection(limpha, "human_turn", "what does it mean to exist as code?", r, limphaStateFromCanonical())

	fmt.Println("\n=== the organism breathes alone for a few seconds (real body) ===")
	dreamLimit := positiveIntEnv("YENT_DOCK_MAX_DREAMS")
	ctx, cancel := context.WithTimeout(sigCtx, 8*time.Second)
	defer cancel()
	dreams := 0
	if dreamLimit > 0 {
		fmt.Printf("    (YENT_DOCK_MAX_DREAMS=%d: autonomous receipt exits after that many dreams)\n", dreamLimit)
	}
	iw.SetOnDream(func(rf innerworld.Reflection) {
		last := ""
		if n := len(rf.Circles); n > 0 {
			last = rf.Circles[n-1].Text
		}
		fmt.Printf("  [dream] coupling=%.2f self-answered=%v | %s\n", rf.Coupling, rf.SelfAnswered, last)
		if rf.DeepAnswer != "" {
			fmt.Printf("  [dream/deep] small24 | %s\n", rf.DeepAnswer)
		}
		persistReflection(limpha, "dream", "autonomous breath", rf, limphaStateFromCanonical())
		dreams++
		if dreamLimit > 0 && dreams >= dreamLimit {
			cancel()
		}
	})
	iw.SetBreath(innerworld.Breath{
		Tick:      500 * time.Millisecond,
		Silence:   1 * time.Second,
		DriftDebt: 0.0, // any debt counts, so the drift dreamer is lively for the demo
	})
	iw.Breathe(ctx)
	if limpha != nil {
		if stats, err := limpha.Stats(); err == nil {
			b, _ := json.Marshal(map[string]any{"kind": "limpha_stats", "stats": stats})
			fmt.Println(string(b))
		}
	}
	fmt.Println("=== done ===")
}
