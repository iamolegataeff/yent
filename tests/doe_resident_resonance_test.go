package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDOEResidentParliamentRequiresLayerFrequencyParity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("doe.c helper harness is POSIX-only")
	}
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skipf("cc not found: %v", err)
	}

	root := repoRootForTest(t)
	dir := t.TempDir()
	harness := filepath.Join(dir, "doe_resonance_harness.c")
	src := `#include <stdio.h>
#include <string.h>
#define main doe_embedded_main
#include "` + filepath.Join(root, "DoE", "doe.c") + `"
#undef main

float *pv_encode_image(const char *img_path, const char *mmproj_path, int *o_ntok, int *o_dim) {
    (void)img_path; (void)mmproj_path; (void)o_ntok; (void)o_dim;
    return NULL;
}

static void set_alive(GGUFIndex *ps, int layer, int expert, float freq) {
    ps->field_layers[layer].experts[expert].alive = 1;
    ps->field_layers[layer].experts[expert].frequency = freq;
}

int main(void) {
    GGUFIndex ps;
    memset(&ps, 0, sizeof(ps));
    ps.n_field_layers = 2;
    set_alive(&ps, 0, 0, 1.0f);
    set_alive(&ps, 1, 0, 1.0f);
	if (!field_resonance_frequencies_compatible(&ps, NULL, NULL)) {
        fprintf(stderr, "matching frequencies rejected\n");
        return 1;
    }

    ps.field_layers[1].experts[0].frequency = 1.25f;
    int layer = -1, expert = -1;
	if (field_resonance_frequencies_compatible(&ps, &layer, &expert)) {
        fprintf(stderr, "mismatched frequencies accepted\n");
        return 2;
    }
    if (layer != 1 || expert != 0) {
        fprintf(stderr, "wrong mismatch location layer=%d expert=%d\n", layer, expert);
        return 3;
    }

    ps.field_layers[1].experts[0].alive = 0;
	if (!field_resonance_frequencies_compatible(&ps, NULL, NULL)) {
        fprintf(stderr, "dead expert frequency should be ignored by frequency parity helper\n");
        return 4;
    }
    return 0;
}
`
	if err := os.WriteFile(harness, []byte(src), 0o600); err != nil {
		t.Fatalf("write harness: %v", err)
	}
	exe := filepath.Join(dir, "doe_resonance_harness")
	cmd := exec.Command("cc", "-O0", "-Wall", "-Wextra", harness, "-lm", "-lpthread", "-o", exe)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile harness: %v\n%s", err, string(out))
	}
	if out, err := exec.Command(exe).CombinedOutput(); err != nil {
		t.Fatalf("run harness: %v\n%s", err, string(out))
	}
}
