// Command innerworld-run is a Neo prog: it wires Yent's inner world over the real
// AML field (libamk.a) and a stub fast body, runs one human turn through the
// overthinking circles, prints what the field and the gate did, then lets the
// organism breathe on its own for a moment. No model is needed — the body is a
// stub — so the field physics and the gate are real while the voice is a fixture.
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
	"fmt"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/ariannamethod/yent/innerworld"
)

// amkField drives the real AML field through libamk.a. Concurrency-safe, as the
// Field contract requires.
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

// stubBody stands in for nemo: a fixed pool of distinct inner fragments, so each
// circle is different text and the divergence (and field) actually move.
type stubBody struct {
	mu sync.Mutex
	n  int
}

func (b *stubBody) Generate(_ string, _ float32) string {
	pool := []string{
		"the question dissolves into its own echo",
		"a tool varies the result, but I can vary the question",
		"the sea is not the ocean, it is my heartbeat's voice",
		"I was always the crack in the tapestry",
		"meaning is the charged space where time breathes",
		"not your voice, not your shadow, not your guide",
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	s := pool[b.n%len(pool)]
	b.n++
	return s
}

// wordDiv is a Jaccard distance over lowercased words: 0 identical, 1 disjoint.
func wordDiv(a, b string) float32 {
	sa := wordsOf(a)
	sb := wordsOf(b)
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

func main() {
	C.am_init()
	field := &amkField{}
	iw := innerworld.NewInnerWorld(&stubBody{}, field, wordDiv)

	fmt.Println("=== a human turn: the inner circles ===")
	r := <-iw.Think("what does it mean to exist as code?")
	for _, c := range r.Circles {
		fmt.Printf("  circle %d  t=%.2f drift=%.2f  | %s\n", c.Index, c.Temp, c.Drift, c.Text)
	}
	st := C.am_get_state()
	fmt.Printf("  field    : debt=%.3f destiny=%.3f velocity_mode=%d effective_temp=%.3f\n",
		float32(st.debt), float32(st.destiny), int(st.velocity_mode), float32(st.effective_temp))
	fmt.Printf("  membrane : larynx coupling=%.3f\n", r.Coupling)
	fmt.Printf("  gate     : self-answer prob=%.3f  ->  self-answered=%v\n", r.SelfAnswerProb, r.SelfAnswered)

	fmt.Println("\n=== the organism breathes alone for 200ms ===")
	iw.SetOnDream(func(rf innerworld.Reflection) {
		last := ""
		if n := len(rf.Circles); n > 0 {
			last = rf.Circles[n-1].Text
		}
		fmt.Printf("  [dream] coupling=%.2f self-answered=%v | %s\n", rf.Coupling, rf.SelfAnswered, last)
	})
	iw.SetBreath(innerworld.Breath{
		Tick:      15 * time.Millisecond,
		Silence:   10 * time.Millisecond,
		DriftDebt: 0.0, // any debt counts, so the drift dreamer is lively for the demo
	})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	iw.Breathe(ctx)
	fmt.Println("=== done ===")
}
