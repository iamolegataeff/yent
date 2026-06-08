//go:build notorch

// notorch.go — route the packed GGUF matvec through notorch's nt_qmatvec, the
// Arianna Method's one source of truth for quantized matvec. Built with
// `-tags notorch`; links the system-wide libnotorch (/opt/homebrew/lib).
//
// Yent shipped its own packed matvec (MatMulQ4_0 … MatMulQ6_K) before notorch
// existed — the two are isomorphic over the same 7 GGUF dtypes. This binding
// folds Yent's matvec onto notorch so there is a single maintained kernel
// (and a path to its int8-dot fast lane). notorch returns -1 for any dtype it
// has no packed kernel for, and the caller falls back to Yent's native matvec,
// so this is purely additive — never a regression.
package yent

/*
#cgo CFLAGS: -I/opt/homebrew/include
#cgo darwin LDFLAGS: -L/opt/homebrew/lib -lnotorch -framework Accelerate -lm
#cgo linux LDFLAGS: -L/opt/homebrew/lib -lnotorch -lopenblas -lm
#include <stdint.h>
int nt_qmatvec(float *out, const uint8_t *Wq, int dtype, const float *x, int m, int k);
*/
import "C"

import "unsafe"

const useNotorch = true

// notorchQMatvec computes out[rows] = W[rows,cols] @ x[cols] from packed GGUF
// bytes via notorch's nt_qmatvec. The GGUF type code passes straight through
// (Yent's ggmlType* values are the standard GGML codes notorch expects).
// Returns true if notorch handled the dtype (rc == 0); false if it has no
// packed kernel (rc == -1), so the caller falls back to the native matvec.
func notorchQMatvec(out []float32, w []byte, dtype uint32, x []float32, rows, cols int) bool {
	if len(out) == 0 || len(w) == 0 || len(x) == 0 {
		return false
	}
	rc := C.nt_qmatvec(
		(*C.float)(unsafe.Pointer(&out[0])),
		(*C.uint8_t)(unsafe.Pointer(&w[0])),
		C.int(dtype),
		(*C.float)(unsafe.Pointer(&x[0])),
		C.int(rows), C.int(cols))
	return rc == 0
}
