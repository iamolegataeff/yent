package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDOECalendarEpochIsUTCFixed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("doe.c calendar harness is POSIX-only")
	}
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skipf("cc not found: %v", err)
	}

	root := repoRootForTest(t)
	doePath := strings.ReplaceAll(filepath.Join(root, "DoE", "doe.c"), `\`, `\\`)
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "doe_calendar_harness.c")
	exe := filepath.Join(dir, "doe_calendar_harness")
	src := fmt.Sprintf(`#include <stdio.h>
#include <stdlib.h>
#include <time.h>

float *pv_encode_image(const char *img_path, const char *mmproj_path, int *o_ntok, int *o_dim) {
    (void)img_path; (void)mmproj_path; (void)o_ntok; (void)o_dim;
    return NULL;
}

#define main doe_embedded_main
#include "%s"
#undef main

static int check_tz(const char *tz, long want) {
    if (setenv("TZ", tz, 1) != 0) return 10;
    tzset();
    calendar_init();
    if ((long)g_epoch_t != want) {
        fprintf(stderr, "epoch under %%s = %%ld, want %%ld\n", tz, (long)g_epoch_t, want);
        return 1;
    }
    return 0;
}

int main(void) {
    const long utc_noon = 1727956800L;
    int rc = check_tz("UTC", utc_noon);
    if (rc) return rc;
    rc = check_tz("Asia/Jerusalem", utc_noon);
    if (rc) return 20 + rc;
    rc = check_tz("America/New_York", utc_noon);
    if (rc) return 40 + rc;
    return 0;
}
`, doePath)
	if err := os.WriteFile(srcPath, []byte(src), 0o600); err != nil {
		t.Fatalf("write harness: %v", err)
	}
	cmd := exec.Command("cc", "-O0", "-Wall", "-Wextra", srcPath, "-lm", "-lpthread", "-o", exe)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compile doe calendar harness: %v\n%s", err, string(out))
	}
	out, err = exec.Command(exe).CombinedOutput()
	if err != nil {
		t.Fatalf("run doe calendar harness: %v\n%s", err, string(out))
	}
}
