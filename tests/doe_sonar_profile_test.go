package tests

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDOESonarProfileShapeUsesSizeTUnderUBSan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("doe.c sonar harness is POSIX-only")
	}
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skipf("cc not found: %v", err)
	}
	root := repoRootForTest(t)
	doePath := strings.ReplaceAll(filepath.Join(root, "DoE", "doe.c"), `\`, `\\`)
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "sonar_profile_harness.c")
	exe := filepath.Join(dir, "sonar_profile_harness")
	src := fmt.Sprintf(`#include <limits.h>
#include <stdint.h>
#include <stdlib.h>

float *pv_encode_image(const char *img_path, const char *mmproj_path, int *o_ntok, int *o_dim) {
    (void)img_path; (void)mmproj_path; (void)o_ntok; (void)o_dim;
    return NULL;
}

#define main doe_embedded_main
#include "%s"
#undef main

int main(void) {
    if (sizeof(size_t) < 8) return 0;
    size_t n = 0;
    if (!profile_weight_shape_checked((size_t)INT_MAX, 1, &n) || n != (size_t)INT_MAX) return 10;
    if (!profile_weight_shape_checked((size_t)INT_MAX + 1u, 1, &n) || n != (size_t)INT_MAX + 1u) return 11;
    if (!profile_weight_shape_checked((size_t)65536, (size_t)32768, &n) || n != (size_t)2147483648ULL) return 12;
    if (!profile_weight_shape_checked((size_t)262144, (size_t)16384, &n) || n != (size_t)4294967296ULL) return 13;
    if (profile_weight_shape_checked((size_t)SIZE_MAX, 2, &n)) return 14;

    float small[6] = {1.0f, 0.0f, -1.0f, 2.0f, 3.0f, 4.0f};
    LayerProfile lp;
    profile_weights(small, 2, 3, &lp);
    if (!(lp.health > 0.0f) || !(lp.mean_abs > 0.0f)) return 15;
    return 0;
}
`, doePath)
	if err := os.WriteFile(srcPath, []byte(src), 0o600); err != nil {
		t.Fatalf("write harness: %v", err)
	}
	cmd := exec.Command("cc", "-O0", "-fsanitize=undefined", "-fno-sanitize-recover=undefined",
		"-Wall", "-Wextra", srcPath, "-lm", "-lpthread", "-o", exe)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compile sonar profile harness: %v\n%s", err, string(out))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	run := exec.CommandContext(ctx, exe)
	out, err = run.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("sonar profile harness timed out:\n%s", string(out))
	}
	if err != nil {
		t.Fatalf("sonar profile harness failed: %v\n%s", err, string(out))
	}
}
