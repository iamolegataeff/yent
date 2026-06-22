package yent

import "testing"

func testRoPEState() *LlamaState {
	return &LlamaState{
		CosCache: []float32{0, 1},
		SinCache: []float32{1, 0},
	}
}

func TestRoPENEOXUsesHalfSplitPairs(t *testing.T) {
	vec := []float32{1, 2, 3, 4}

	applyRoPENEOX(vec, 0, testRoPEState(), 4)

	want := []float32{-3, 2, 1, 4}
	for i := range want {
		if vec[i] != want[i] {
			t.Fatalf("index %d: got %v want %v (vec=%v)", i, vec[i], want[i], vec)
		}
	}
}

func TestRoPENormalUsesAdjacentPairs(t *testing.T) {
	vec := []float32{1, 2, 3, 4}

	applyRoPENormal(vec, 0, testRoPEState(), 4)

	want := []float32{-2, 1, 3, 4}
	for i := range want {
		if vec[i] != want[i] {
			t.Fatalf("index %d: got %v want %v (vec=%v)", i, vec[i], want[i], vec)
		}
	}
}

func TestRoPENormalForArch(t *testing.T) {
	normal := []string{"llama", "mistral", "mistral3", "mistral4", "MISTRAL3"}
	for _, arch := range normal {
		if !ropeNormalForArch(arch) {
			t.Fatalf("expected normal RoPE for arch %q", arch)
		}
	}

	neox := []string{"qwen2", "gemma", "phi3", "nanollama", ""}
	for _, arch := range neox {
		if ropeNormalForArch(arch) {
			t.Fatalf("expected NEOX RoPE for arch %q", arch)
		}
	}
}
