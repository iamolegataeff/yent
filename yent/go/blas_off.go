//go:build !blas

package yent

// Stub: no BLAS acceleration. Pure Go fallback.
// Build with -tags blas to enable hardware acceleration.

var useBLAS = false

func blasMatMulF32(out []float32, w []float32, x []float32, rows, cols int) {}
func blasApplySVDStep1(bx []float32, B []float32, x []float32, rank, hiddenDim int) {}
func blasApplySVDStep2(logits []float32, A []float32, bx []float32, vocabSize, rank int, alpha float32) {}
func blasDot(a, b []float32) float32 { return 0 }
