//go:build blas

package yent

// BLAS acceleration for Delta Voice and MatMulF32.
// Evolved in molequla, ported to AML core, propagated here.
//
// Build: go build -tags blas
// macOS: Apple Accelerate (AMX/Neural Engine, zero deps)
// Linux: OpenBLAS (apt install libopenblas-dev)

/*
#cgo darwin CFLAGS: -DACCELERATE
#cgo darwin LDFLAGS: -framework Accelerate
#cgo linux LDFLAGS: -lopenblas

#ifdef ACCELERATE
#define ACCELERATE_NEW_LAPACK
#include <Accelerate/Accelerate.h>
#else
#include <cblas.h>
#endif

// Thin C wrappers to avoid CGO enum issues

// out[rows] = W[rows,cols] @ x[cols]
static void blas_sgemv_nn(float* out, const float* w, const float* x,
                          int rows, int cols) {
    cblas_sgemv(CblasRowMajor, CblasNoTrans, rows, cols,
                1.0f, w, cols, x, 1, 0.0f, out, 1);
}

// out[rows] += alpha * W[rows,cols] @ x[cols]
static void blas_sgemv_add(float* out, const float* w, const float* x,
                           int rows, int cols, float alpha) {
    cblas_sgemv(CblasRowMajor, CblasNoTrans, rows, cols,
                alpha, w, cols, x, 1, 1.0f, out, 1);
}

// dot product: sum(a[i] * b[i])
static float blas_sdot(const float* a, const float* b, int n) {
    return cblas_sdot(n, a, 1, b, 1);
}
*/
import "C"
import "unsafe"

var useBLAS = true

// blasMatMulF32 replaces MatMulF32 with cblas_sgemv
func blasMatMulF32(out []float32, w []float32, x []float32, rows, cols int) {
	C.blas_sgemv_nn(
		(*C.float)(unsafe.Pointer(&out[0])),
		(*C.float)(unsafe.Pointer(&w[0])),
		(*C.float)(unsafe.Pointer(&x[0])),
		C.int(rows), C.int(cols))
}

// blasApplySVDStep1 computes Bx = B @ x (rank × hiddenDim @ hiddenDim → rank)
func blasApplySVDStep1(bx []float32, B []float32, x []float32, rank, hiddenDim int) {
	C.blas_sgemv_nn(
		(*C.float)(unsafe.Pointer(&bx[0])),
		(*C.float)(unsafe.Pointer(&B[0])),
		(*C.float)(unsafe.Pointer(&x[0])),
		C.int(rank), C.int(hiddenDim))
}

// blasApplySVDStep2 computes logits += alpha * A @ Bx (vocabSize × rank @ rank → vocabSize)
func blasApplySVDStep2(logits []float32, A []float32, bx []float32, vocabSize, rank int, alpha float32) {
	C.blas_sgemv_add(
		(*C.float)(unsafe.Pointer(&logits[0])),
		(*C.float)(unsafe.Pointer(&A[0])),
		(*C.float)(unsafe.Pointer(&bx[0])),
		C.int(vocabSize), C.int(rank), C.float(alpha))
}

// blasDot computes dot product of two float32 slices
func blasDot(a, b []float32) float32 {
	n := len(a)
	if n == 0 {
		return 0
	}
	return float32(C.blas_sdot(
		(*C.float)(unsafe.Pointer(&a[0])),
		(*C.float)(unsafe.Pointer(&b[0])),
		C.int(n)))
}
