// Package aml is the native AML third body behind innerworld.Flow — Yent's `flow`,
// over the real Arianna Method field (libamk.a) via cgo. It is the production
// counterpart to innerworld.goFlow: where goFlow keeps a pure-Go cooc graph and a
// pure-Go scar sea, this body folds thoughts into the AML field's OWN token
// co-occurrence graph (am_ingest_tokens), runs the seasonal autumn harvest
// (am_cooc_consolidate_autumn), deposits rejected thoughts as gravitational scars
// (the SCAR operator), reads the field's harvest ripeness (autumn_energy), and
// pushes the field back onto a voice's logits (am_apply_field_to_logits).
//
// One process holds one global AML field: every am_* call operates on the kernel's
// single AM_State, so this body IS that field plus the consolidation organs — the
// two voices (nemo fast + small24 deep) stream into one shared physics. Reads and
// writes go through a mutex, as the innerworld.Field contract requires.
//
// The field is pure C / CPU — no Metal, no model. libamk.a builds and runs on any
// host (lean build: -DAM_BLOOD_DISABLED -DAM_ASYNC_DISABLED), so the native body is
// testable off the Mac mini. Only the doe model voices need Metal.
package aml

/*
#cgo CFLAGS: -I${SRCDIR}/../../yent/c
#cgo LDFLAGS: ${SRCDIR}/../../yent/c/libamk.a
#include "ariannamethod.h"
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"github.com/ariannamethod/yent/innerworld"
)

// Body is the native AML third body. It satisfies innerworld.Flow over the global
// AML kernel. Concurrency-safe: all field access is serialized under mu.
type Body struct {
	mu  sync.Mutex
	tok Tokenizer
}

// compile-time guarantee: the native body is a drop-in innerworld.Flow.
var _ innerworld.Flow = (*Body)(nil)

// Tokenizer turns text into the token ids the AML cooc graph is built over. The
// production tokenizer is *yent.Tokenizer — the model's own BPE, so the inner cooc
// graph shares the voices' vocabulary — but any Encode(text, addBos) []int
// satisfies it. A nil tokenizer makes Ingest a no-op (no ids, no cooc edges).
type Tokenizer interface {
	Encode(text string, addBos bool) []int
}

// Init resets and initialises the global AML field (chambers, scars, prophecy debt,
// season, cooc graph). am_init is a hard reset, so call it ONCE at process start
// before any field use — or, in tests, at the top of each case to isolate global
// state. It is not safe to call concurrently with field use. New does not call it,
// so a host that already drives am_init (the dock) is never reset out from under.
func Init() { C.am_init() }

// New builds the native body over a tokenizer (may be nil — then Ingest no-ops).
// Init must have been called once for the process.
func New(tok Tokenizer) *Body { return &Body{tok: tok} }

// ── innerworld.Field — the AML bridge ───────────────────────────────────────────

// Exec runs one AML command (e.g. "PROPHECY 7", "VELOCITY RUN", "SEASON AUTUMN").
func (b *Body) Exec(script string) error {
	cs := C.CString(script)
	defer C.free(unsafe.Pointer(cs))
	b.mu.Lock()
	defer b.mu.Unlock()
	if C.am_exec(cs) != 0 {
		return fmt.Errorf("am_exec failed: %q", script)
	}
	return nil
}

// Step advances the field physics by dt — seasons, debt decay, dark-matter growth.
func (b *Body) Step(dt float32) { b.mu.Lock(); defer b.mu.Unlock(); C.am_step(C.float(dt)) }

// Debt reads the prophecy-debt accumulator.
func (b *Body) Debt() float32 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return float32(C.am_get_state().debt)
}

// Destiny reads the field's bias toward the most-probable path.
func (b *Body) Destiny() float32 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return float32(C.am_get_state().destiny)
}

// ── innerworld.Flow — the consolidation organs ──────────────────────────────────

// Ingest folds a thought into the field's token co-occurrence graph: the text is
// tokenized with the body's tokenizer and streamed through am_ingest_tokens, which
// adds distance-weighted cooc weight (1/|i-j|, windowed ±5, ariannamethod.c:6988)
// to every pair. This is the stream entering the body — circles and deep answers
// grow the field's own memory richer than the dataset (haze-emergence). No
// tokenizer, or fewer than two tokens, means no pair to fold (no-op).
func (b *Body) Ingest(text string) {
	if b.tok == nil {
		return
	}
	ids := b.tok.Encode(text, false)
	if len(ids) < 2 {
		return
	}
	cids := make([]C.int, len(ids))
	for i, id := range ids {
		cids[i] = C.int(id)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	C.am_ingest_tokens(&cids[0], C.int(len(cids)))
}

// ConsolidateCooc runs the seasonal autumn harvest on the field's cooc graph:
// am_cooc_consolidate_autumn reinforces edges above the median, decays the rest,
// and prunes the long tail — but ONLY in deep autumn (season==AUTUMN &&
// autumn_energy>0.6, ariannamethod.c:7082). The physics gates it, so consolidation
// follows the field's coherence into autumn, not the clock. Returns the number of
// edges pruned, or 0 when the harvest did not fire (off-season / low energy).
func (b *Body) ConsolidateCooc() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := int(C.am_cooc_consolidate_autumn())
	if n < 0 {
		return 0 // not triggered: wrong season or energy <= 0.6 — cooc untouched
	}
	return n
}

// Scar deposits a rejected thought into the field's gravitational memory via the
// AML SCAR operator (ariannamethod.c:3834): a thought that broke prophecy-destiny
// coherence is kept as dark matter — the lineage that biases the field away from
// what it refused, a direct continuation of the S8 DPO epistemic-self-contour work.
// gravity<=0 or empty text is ignored (matching goFlow). The field's dark_gravity
// then grows over the deposited scars in autumn (am_step, :8063). The text is kept
// to one line and stripped of the quote/backslash that would break the one-line
// SCAR parse, and capped at the field's 63-char scar slot.
func (b *Body) Scar(text string, gravity float32) {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" || gravity <= 0 {
		return
	}
	text = strings.NewReplacer(`"`, "", `\`, "").Replace(text)
	if r := []rune(text); len(r) > 63 { // AM_SCAR_MAX_LEN-1: the field stores 63 chars
		text = string(r[:63])
	}
	_ = b.Exec(`SCAR "` + text + `"`)
}

// ConsolidateScar is intentionally inert in the native AML body — and that is the
// honest mapping, not a stub hiding work. This AML build has no discrete scar-prune
// pass: deposited scars accumulate as gravitational memory (scar_texts[]), and the
// field's dark_gravity consolidates them CONTINUOUSLY inside am_step's autumn
// physics (dark_gravity += autumn_energy*0.002*dt, ariannamethod.c:8063), riding the
// field step the orchestrator already drives. The discrete sleep work in this body
// is the cooc autumn harvest (ConsolidateCooc). goFlow models per-scar decay (leo
// klaus-scar) because it has no field to step; the native body defers to the field's
// own dark-matter physics. Returns 0 — no discrete scar is forgotten here.
func (b *Body) ConsolidateScar() int { return 0 }

// ApplyPressure pushes the field back onto a voice's logits in place:
// am_apply_field_to_logits tilts the distribution toward what the field's
// cooc/destiny/dark-matter favour (ariannamethod.c:7132) — the OUT influence, the
// body shaping the next token a voice emits. Empty logits = no-op.
func (b *Body) ApplyPressure(logits []float32) {
	if len(logits) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	C.am_apply_field_to_logits((*C.float)(unsafe.Pointer(&logits[0])), C.int(len(logits)))
}

// AutumnEnergy reports the field's harvest ripeness in [0,1] — autumn_energy, which
// rises as the field's coherence drives it into autumn. Kairos reads it for critical
// mass: high autumn = time to sleep and consolidate. This is the real season the
// native body has, where goFlow can only synthesize one from field debt.
func (b *Body) AutumnEnergy() float32 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return float32(C.am_get_state().autumn_energy)
}

// ── telemetry (read-only, for Kairos observers and smoke) ────────────────────────

// CoocStats reports the mean and max live cooc edge weight (am_cooc_stats).
func (b *Body) CoocStats() (mean, max float32) {
	var m, mx C.float
	b.mu.Lock()
	defer b.mu.Unlock()
	C.am_cooc_stats(&m, &mx)
	return float32(m), float32(mx)
}

// DarkGravity reports the gravitational-memory strength (0..1) — how strongly the
// scars deposited so far bias the field.
func (b *Body) DarkGravity() float32 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return float32(C.am_get_state().dark_gravity)
}

// Scars reports how many scars are held in gravitational memory.
func (b *Body) Scars() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return int(C.am_get_state().n_scars)
}

// Season reports the current season (SPRING=0, SUMMER=1, AUTUMN=2, WINTER=3).
func (b *Body) Season() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return int(C.am_get_state().season)
}
