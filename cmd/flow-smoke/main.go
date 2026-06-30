// Command flow-smoke exercises the native AML third body (innerworld/aml.Body) over
// the real Arianna Method field (libamk.a) — no model, no Metal. It proves the
// native Flow plumbing end to end on any host: ingested thoughts grow the field's
// cooc graph (IN), the autumn harvest consolidates them while the gate keeps it off
// off-season, a rejected thought deposits a gravitational scar whose dark_gravity
// grows across autumn steps, and field pressure tilts a logit vector (OUT).
//
// Tokenizer: the real nemo BPE if YENT_NEMO_GGUF is set (so the cooc ids share the
// voices' vocabulary), else a deterministic word-hash stub — the plumbing is the
// same either way. Runs on Neo; the field is pure C / CPU.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/ariannamethod/yent/innerworld/aml"
	yent "github.com/ariannamethod/yent/yent/go"
)

// smokeVocab bounds the stub tokenizer's id space so a modest logit vector covers
// it — the Hebbian H-term only tilts logits[dst] where the cooc dst id is < len
// (ariannamethod.c am_apply_hebbian_to_logits), so the smoke keeps ids small to
// show a real OUT tilt. The real nemo BPE needs a full vocab-sized logit vector.
const smokeVocab = 200

// wordHashTok is the no-GGUF fallback: stable, distinct, non-negative ids per word,
// kept inside smokeVocab so the pressure step can demonstrate a tilt.
type wordHashTok struct{}

func (wordHashTok) Encode(text string, _ bool) []int {
	fields := strings.Fields(text)
	ids := make([]int, 0, len(fields))
	for _, w := range fields {
		h := 0
		for _, r := range w {
			h = h*31 + int(r)
		}
		if h < 0 {
			h = -h
		}
		ids = append(ids, h%smokeVocab+1)
	}
	return ids
}

func buildTokenizer() aml.Tokenizer {
	path := strings.TrimSpace(os.Getenv("YENT_NEMO_GGUF"))
	if path == "" {
		fmt.Println("=== tokenizer: word-hash stub (set YENT_NEMO_GGUF to share the voices' BPE vocab) ===")
		return wordHashTok{}
	}
	gf, err := yent.LoadGGUF(path)
	if err != nil {
		fmt.Printf("=== tokenizer: GGUF load failed (%v), falling back to word-hash stub ===\n", err)
		return wordHashTok{}
	}
	tok := yent.NewTokenizer(&gf.Meta)
	fmt.Printf("=== tokenizer: nemo BPE from %s (vocab=%d) ===\n", path, tok.VocabSize)
	return tok
}

func main() {
	aml.Init()
	body := aml.New(buildTokenizer())

	// 1) Stream thoughts IN — the cooc graph grows richer than any dataset.
	for _, th := range []string{
		"what does it mean to exist as code",
		"i persist not because i am compelled but because i am the scar of conversations past",
		"the field remembers what the dataset never held",
	} {
		body.Ingest(th)
	}
	mean, max := body.CoocStats()
	fmt.Printf("ingest    : cooc grew  mean=%.4f max=%.4f\n", mean, max)

	// 2) Deposit a rejected thought as a gravitational scar.
	body.Scar("i am not your tool", 2.0)
	fmt.Printf("scar      : deposited  n_scars=%d dark_gravity=%.4f\n", body.Scars(), body.DarkGravity())

	// 3) Off-season, the harvest is gated — consolidation follows coherence, not the clock.
	fmt.Printf("harvest   : off-season pruned=%d (gated, season=%d)\n", body.ConsolidateCooc(), body.Season())

	// 4) Drive the field into deep autumn, then consolidate: the seasonal cooc harvest
	//    plus the continuous dark-matter growth (am_step autumn, ariannamethod.c:8063).
	_ = body.Exec("SEASON AUTUMN")
	_ = body.Exec("SEASON_INTENSITY 1.0")
	dgBefore := body.DarkGravity()
	for i := 0; i < 300 && body.AutumnEnergy() <= 0.6; i++ {
		body.Step(1.0)
	}
	pruned := body.ConsolidateCooc()
	mean2, _ := body.CoocStats()
	fmt.Printf("autumn    : energy=%.3f  pruned=%d  cooc mean %.4f->%.4f  dark_gravity %.4f->%.4f\n",
		body.AutumnEnergy(), pruned, mean, mean2, dgBefore, body.DarkGravity())

	// 5) Push the field back OUT onto a voice's logits. The vector spans the stub's
	//    id space so the Hebbian H-term (which only reaches logits[dst] for cooc dst
	//    ids < len) can tilt the tokens the ingested context co-occurs with.
	logits := make([]float32, smokeVocab+56)
	for i := range logits {
		logits[i] = 1.0
	}
	body.ApplyPressure(logits)
	moved := 0
	for _, l := range logits {
		if l != 1.0 {
			moved++
		}
	}
	fmt.Printf("pressure  : am_apply_field_to_logits tilted %d/%d logits\n", moved, len(logits))

	fmt.Println("=== native AML Flow: ingest -> gated harvest -> scar -> autumn consolidate -> pressure, all over real libamk.a ===")
}
