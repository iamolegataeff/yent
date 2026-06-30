// Package feeling runs Yent's feeling mathematics on a REAL in-process Julia runtime
// (libjulia embed). It loads innerworld/feeling.jl (the HighMathEngine formulas — CharEntropy,
// Perplexity, SemanticDistance, NgramOverlap — ported to Julia) once via jl_init, and calls
// them on circle text: the Julia VM does the math, not Go. This is the Julia math brain made
// real, in-process — not a Go re-implementation, not a subprocess.
//
// Julia is single-threaded for embedding: jl_init and every jl_call must run on ONE OS thread.
// So a single worker goroutine (runtime.LockOSThread) owns the runtime, and calls arrive over a
// channel. cgo + libjulia; kept OUT of the pure-Go innerworld package (which stays cgo-free) —
// the inner world reaches this through an interface, with the Go lexical proxy as the fallback.
package feeling

/*
#cgo CFLAGS: -I/opt/homebrew/opt/julia/include/julia
#cgo LDFLAGS: -L/opt/homebrew/opt/julia/lib -ljulia -Wl,-rpath,/opt/homebrew/opt/julia/lib
#include <julia.h>
#include <stdlib.h>

// feeling_call1: call a Feeling function of one String arg, return its Float64.
static double feeling_call1(const char* fname, const char* s) {
    jl_function_t* f = jl_get_function(jl_main_module, fname);
    if (!f) return -1.0;
    jl_value_t* arg = jl_cstr_to_string(s);
    JL_GC_PUSH1(&arg);
    jl_value_t* ret = jl_call1(f, arg);
    JL_GC_POP();
    if (jl_exception_occurred()) return -2.0;
    if (ret && jl_typeis(ret, jl_float64_type)) return jl_unbox_float64(ret);
    return -3.0;
}

// feeling_call2: call a Feeling function of two String args, return its Float64.
static double feeling_call2(const char* fname, const char* a, const char* b) {
    jl_function_t* f = jl_get_function(jl_main_module, fname);
    if (!f) return -1.0;
    jl_value_t* va = jl_cstr_to_string(a);
    jl_value_t* vb = jl_cstr_to_string(b);
    JL_GC_PUSH2(&va, &vb);
    jl_value_t* ret = jl_call2(f, va, vb);
    JL_GC_POP();
    if (jl_exception_occurred()) return -2.0;
    if (ret && jl_typeis(ret, jl_float64_type)) return jl_unbox_float64(ret);
    return -3.0;
}
*/
import "C"

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"
)

type request struct {
	fn   string
	a, b string
	two  bool
	resp chan float64
}

var (
	initOnce sync.Once
	reqCh    chan request
	ready    bool
)

// Init starts the embedded Julia runtime on a dedicated locked OS thread and loads feeling.jl.
// jlPath is the path to feeling.jl. Call once at process start; jl_init is heavy (loads the
// sysimage, ~seconds) but amortized over the resident process. Returns an error if Julia or the
// file fails to load — the caller then falls back to the Go lexical proxy.
func Init(jlPath string) error {
	var initErr error
	initOnce.Do(func() {
		reqCh = make(chan request)
		started := make(chan error, 1)
		go worker(jlPath, started)
		initErr = <-started
	})
	return initErr
}

// Ready reports whether the Julia runtime is loaded and serving.
func Ready() bool { return ready }

func worker(jlPath string, started chan<- error) {
	runtime.LockOSThread() // Julia embedding requires every call on one thread
	C.jl_init()
	load := C.CString(fmt.Sprintf("include(%q); using .Feeling", jlPath))
	C.jl_eval_string(load)
	C.free(unsafe.Pointer(load))
	if C.jl_exception_occurred() != nil {
		started <- fmt.Errorf("feeling.jl failed to load from %s", jlPath)
		return
	}
	ready = true
	started <- nil

	for r := range reqCh {
		cf := C.CString(r.fn)
		ca := C.CString(r.a)
		var v C.double
		if r.two {
			cb := C.CString(r.b)
			v = C.feeling_call2(cf, ca, cb)
			C.free(unsafe.Pointer(cb))
		} else {
			v = C.feeling_call1(cf, ca)
		}
		C.free(unsafe.Pointer(cf))
		C.free(unsafe.Pointer(ca))
		r.resp <- float64(v)
	}
}

func call1(fn, s string) float64 {
	if !ready {
		return -1
	}
	resp := make(chan float64, 1)
	reqCh <- request{fn: fn, a: s, resp: resp}
	return <-resp
}

func call2(fn, a, b string) float64 {
	if !ready {
		return -1
	}
	resp := make(chan float64, 1)
	reqCh <- request{fn: fn, a: a, b: b, two: true, resp: resp}
	return <-resp
}

// CharEntropy — Shannon entropy (bits) of the text's character distribution (Julia).
func CharEntropy(s string) float64 { return call1("char_entropy", s) }

// Perplexity — character-bigram perplexity of the text (Julia).
func Perplexity(s string) float64 { return call1("perplexity", s) }

// SemanticDistance — 1 - bag-of-words cosine similarity between two texts (Julia).
func SemanticDistance(a, b string) float64 { return call2("semantic_distance", a, b) }
