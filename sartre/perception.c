/*
 * perception.c — SARTRE perception physics (see perception.h).
 *
 * by Arianna Method
 */

#include "perception.h"
#include <stdio.h>
#include <string.h>

/* Case-insensitive substring search (README / readme / ReadMe). */
static int contains_ci(const char *hay, const char *needle) {
    size_t nl = strlen(needle);
    if (nl == 0) return 1;
    for (const char *h = hay; *h; h++) {
        size_t i = 0;
        while (i < nl && h[i] &&
               (h[i] | 0x20) == (needle[i] | 0x20)) {
            i++;
        }
        if (i == nl) return 1;
    }
    return 0;
}

int sartre_perceive_from_events(const char *json_lines, SartrePerception *out) {
    if (!out) return 0;
    out->changed = 0;
    out->readme_changed = 0;
    if (!json_lines) return 0;

    /* Walk line by line. Each event line carries "kind" and "path". */
    const char *p = json_lines;
    while (*p) {
        const char *eol = strchr(p, '\n');
        size_t len = eol ? (size_t)(eol - p) : strlen(p);
        if (len > 0) {
            char line[2048];
            size_t n = len < sizeof(line) - 1 ? len : sizeof(line) - 1;
            memcpy(line, p, n);
            line[n] = '\0';
            if (strstr(line, "\"kind\"")) {       /* it is an event */
                out->changed++;
                const char *path = strstr(line, "\"path\"");
                if (path && contains_ci(path, "README")) {
                    out->readme_changed = 1;
                }
            }
        }
        if (!eol) break;
        p = eol + 1;
    }
    return out->changed;
}

int sartre_perceive_to_aml(const SartrePerception *p, char *buf, int max) {
    if (!p || !buf || max <= 0) return -1;

    int n;
    if (p->changed <= 0) {
        /* field at rest: no motion, minimal horizon */
        n = snprintf(buf, max, "VELOCITY NOMOVE\nPROPHECY %d", SARTRE_PROPHECY_MIN);
    } else {
        /* motion in the tree -> RUN; horizon scales with how much moved, and a README
         * (Yent's self-description) shifting weighs more than a routine research scan.
         * long arithmetic so a flood of changes clamps, never overflows before clamp. */
        long prophecy = 2L + (long)p->changed + (p->readme_changed ? 7L : 0L);
        if (prophecy < SARTRE_PROPHECY_MIN) prophecy = SARTRE_PROPHECY_MIN;
        if (prophecy > SARTRE_PROPHECY_MAX) prophecy = SARTRE_PROPHECY_MAX;
        n = snprintf(buf, max, "VELOCITY RUN\nPROPHECY %d", (int)prophecy);
    }

    if (n < 0 || n >= max) return -1;   /* encode error or truncated: bytes-written contract */
    return n;
}

#ifdef SARTRE_PERCEPTION_TEST
/* Standalone self-test: cc -DSARTRE_PERCEPTION_TEST perception.c -o perception_test */
static int check(const char *label, const char *got, const char *want) {
    int ok = strcmp(got, want) == 0;
    printf("[perc] %-28s %s\n", label, ok ? "PASS" : "FAIL");
    if (!ok) printf("        got:  %s\n        want: %s\n", got, want);
    return ok;
}

int main(void) {
    int pass = 0, total = 0;
    char buf[256];
    SartrePerception p;

    /* 1. quiet scan -> NOMOVE */
    p = (SartrePerception){ .changed = 0, .readme_changed = 0 };
    sartre_perceive_to_aml(&p, buf, sizeof buf);
    total++; pass += check("quiet -> NOMOVE", buf, "VELOCITY NOMOVE\nPROPHECY 1");

    /* 2. one research file -> RUN, modest prophecy (2+1) */
    p = (SartrePerception){ .changed = 1, .readme_changed = 0 };
    sartre_perceive_to_aml(&p, buf, sizeof buf);
    total++; pass += check("1 change -> RUN P3", buf, "VELOCITY RUN\nPROPHECY 3");

    /* 3. README change weighs more (2+1+7=10) */
    p = (SartrePerception){ .changed = 1, .readme_changed = 1 };
    sartre_perceive_to_aml(&p, buf, sizeof buf);
    total++; pass += check("README -> RUN P10", buf, "VELOCITY RUN\nPROPHECY 10");

    /* 4. flood clamps to 64 */
    p = (SartrePerception){ .changed = 200, .readme_changed = 1 };
    sartre_perceive_to_aml(&p, buf, sizeof buf);
    total++; pass += check("flood -> clamp P64", buf, "VELOCITY RUN\nPROPHECY 64");

    /* 5. parse a real repo_monitor event stream */
    const char *events =
        "{\"util\":\"repo_monitor\",\"kind\":\"added\",\"path\":\"/r/x.rs\",\"ts\":1}\n"
        "{\"util\":\"repo_monitor\",\"kind\":\"modified\",\"path\":\"/r/README.md\",\"ts\":2}\n";
    sartre_perceive_from_events(events, &p);
    total++;
    int ok5 = (p.changed == 2 && p.readme_changed == 1);
    printf("[perc] %-28s %s (changed=%d readme=%d)\n", "parse events", ok5 ? "PASS" : "FAIL",
           p.changed, p.readme_changed);
    pass += ok5;

    /* 6. end-to-end: those events -> AML */
    sartre_perceive_to_aml(&p, buf, sizeof buf);
    total++; pass += check("events -> RUN P11", buf, "VELOCITY RUN\nPROPHECY 11");

    printf("[perc] %d/%d PASS\n", pass, total);
    return pass == total ? 0 : 1;
}
#endif
