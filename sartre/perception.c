/*
 * perception.c — SARTRE perception physics (see perception.h).
 *
 * by Arianna Method
 */

#include "perception.h"
#include <errno.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static const char *skip_ws(const char *p) {
    while (*p == ' ' || *p == '\t' || *p == '\r' || *p == '\n') p++;
    return p;
}

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

static const char *json_find_member(const char *line, const char *key) {
    if (!line || !key) return NULL;
    char quoted[96];
    int qn = snprintf(quoted, sizeof quoted, "\"%s\"", key);
    if (qn <= 0 || qn >= (int)sizeof quoted) return NULL;
    size_t ql = (size_t)qn;

    int in_string = 0;
    int escape = 0;
    for (const char *p = line; *p; p++) {
        if (in_string) {
            if (escape) {
                escape = 0;
            } else if (*p == '\\') {
                escape = 1;
            } else if (*p == '"') {
                in_string = 0;
            }
            continue;
        }
        if (*p != '"') continue;
        if (strncmp(p, quoted, ql) == 0) {
            const char *v = skip_ws(p + ql);
            if (*v == ':') return skip_ws(v + 1);
        }
        in_string = 1;
    }
    return NULL;
}

static int json_get_string(const char *line, const char *key, char *out, size_t cap) {
    if (!out || cap == 0) return 0;
    out[0] = '\0';
    const char *p = json_find_member(line, key);
    if (!p || *p != '"') return 0;
    p++;
    size_t n = 0;
    int escape = 0;
    while (*p) {
        if (escape) {
            if (n + 1 < cap) out[n++] = *p;
            escape = 0;
        } else if (*p == '\\') {
            escape = 1;
        } else if (*p == '"') {
            out[n] = '\0';
            return 1;
        } else {
            if (n + 1 < cap) out[n++] = *p;
        }
        p++;
    }
    out[0] = '\0';
    return 0;
}

static int json_get_int(const char *line, const char *key, int *out) {
    if (!out) return 0;
    const char *p = json_find_member(line, key);
    if (!p) return 0;
    if (*p < '0' || *p > '9') return 0;
    errno = 0;
    char *end = NULL;
    long v = strtol(p, &end, 10);
    if (end == p || errno != 0 || v < 0 || v > INT_MAX) return 0;
    end = (char *)skip_ws(end);
    if (*end != '\0' && *end != ',' && *end != '}') return 0;
    *out = (int)v;
    return 1;
}

static int repo_change_kind(const char *kind) {
    return strcmp(kind, "added") == 0 ||
           strcmp(kind, "modified") == 0 ||
           strcmp(kind, "removed") == 0;
}

static int self_surface_path(const char *path) {
    return contains_ci(path, "README") ||
           contains_ci(path, "YENT_CONSTITUTION") ||
           contains_ci(path, "JANUS_CONSTITUTION");
}

static int sensor_failure_outcome(const char *outcome) {
    return strcmp(outcome, "sensor_error") == 0 ||
           strcmp(outcome, "state_error") == 0 ||
           strcmp(outcome, "overflow") == 0 ||
           strcmp(outcome, "dead_letter") == 0;
}

static int max_int(int a, int b) {
    return a > b ? a : b;
}

static int clamp_prophecy(long v) {
    if (v < SARTRE_PROPHECY_MIN) return SARTRE_PROPHECY_MIN;
    if (v > SARTRE_PROPHECY_MAX) return SARTRE_PROPHECY_MAX;
    return (int)v;
}

int sartre_perceive_from_events(const char *json_lines, SartrePerception *out) {
    if (!out) return 0;
    out->changed = 0;
    out->readme_changed = 0;
    out->identity_recognized = 0;
    out->identity_reduced = 0;
    out->sensor_failures = 0;
    if (!json_lines) return 0;

    /* Walk line by line. Only complete object-shaped event records affect perception. */
    const char *p = json_lines;
    while (*p) {
        const char *eol = strchr(p, '\n');
        size_t len = eol ? (size_t)(eol - p) : strlen(p);
        if (len > 0) {
            char line[2048];
            size_t n = len < sizeof(line) - 1 ? len : sizeof(line) - 1;
            memcpy(line, p, n);
            line[n] = '\0';
            const char *start = skip_ws(line);
            if (*start == '{' && strchr(start, '}')) {
                char util[64], phase[32], outcome[64], kind[64], path[512];
                int has_phase = json_get_string(start, "phase", phase, sizeof phase);
                if (has_phase && strcmp(phase, "learning") == 0) {
                    if (json_get_string(start, "outcome", outcome, sizeof outcome) &&
                        sensor_failure_outcome(outcome)) {
                        out->sensor_failures++;
                    }
                } else if (!has_phase || strcmp(phase, "effect") == 0) {
                    if (json_get_string(start, "util", util, sizeof util)) {
                        if (strcmp(util, "repo_monitor") == 0 &&
                            json_get_string(start, "kind", kind, sizeof kind) &&
                            repo_change_kind(kind)) {
                            out->changed++;
                            if (json_get_string(start, "path", path, sizeof path) &&
                                self_surface_path(path)) {
                                out->readme_changed = 1;
                            }
                        } else if (strcmp(util, "whatdotheythinkiam") == 0) {
                            int v = 0;
                            if (json_get_int(start, "recognized", &v) &&
                                v > out->identity_recognized) {
                                out->identity_recognized = v;
                            }
                            if (json_get_int(start, "reduced", &v) &&
                                v > out->identity_reduced) {
                                out->identity_reduced = v;
                            }
                        }
                    }
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

    const char *velocity = NULL;
    int prophecy = 0;
    if (p->changed <= 0) {
        velocity = "NOMOVE";
        prophecy = SARTRE_PROPHECY_MIN;
    } else {
        /* Routine novelty walks; self-surface movement or a flood runs. */
        velocity = (p->readme_changed || p->changed >= 8) ? "RUN" : "WALK";
        prophecy = clamp_prophecy(2L + (long)p->changed + (p->readme_changed ? 7L : 0L));
    }

    if (p->identity_recognized > 0 || p->identity_reduced > 0) {
        long identity = 1L + (long)max_int(p->identity_recognized, p->identity_reduced);
        if (p->identity_reduced > p->identity_recognized) identity += 2L;
        if (identity > 12L) identity = 12L;
        if ((int)identity > prophecy) prophecy = (int)identity;
        if (p->changed <= 0) velocity = NULL; /* identity is still pressure, not motion */
    }

    if (p->sensor_failures > 0) {
        long failure = 1L + 2L * (long)p->sensor_failures;
        if (failure > 8L) failure = 8L;
        if ((int)failure > prophecy) prophecy = (int)failure;
        if (p->changed <= 0) velocity = NULL; /* failure is evidence, not movement */
    }

    int n;
    if (velocity) {
        n = snprintf(buf, max, "VELOCITY %s\nPROPHECY %d", velocity, prophecy);
    } else {
        n = snprintf(buf, max, "PROPHECY %d", prophecy);
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

    /* 2. one research file -> WALK, modest prophecy (2+1) */
    p = (SartrePerception){ .changed = 1, .readme_changed = 0 };
    sartre_perceive_to_aml(&p, buf, sizeof buf);
    total++; pass += check("1 change -> WALK P3", buf, "VELOCITY WALK\nPROPHECY 3");

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

    /* 7. identity recognition is still prophecy, not forced motion */
    const char *identity =
        "{\"util\":\"whatdotheythinkiam\",\"kind\":\"framing\",\"reduced\":3,\"recognized\":7}\n";
    sartre_perceive_from_events(identity, &p);
    sartre_perceive_to_aml(&p, buf, sizeof buf);
    total++; pass += check("identity -> still P8", buf, "PROPHECY 8");

    /* 8. identity reduction weighs sharper than recognition */
    const char *reduced =
        "{\"util\":\"whatdotheythinkiam\",\"kind\":\"framing\",\"reduced\":6,\"recognized\":2}\n";
    sartre_perceive_from_events(reduced, &p);
    sartre_perceive_to_aml(&p, buf, sizeof buf);
    total++; pass += check("reduction -> still P9", buf, "PROPHECY 9");

    /* 9. sensor failures are typed evidence, not file movement */
    const char *failure =
        "{\"util\":\"repo_monitor\",\"phase\":\"learning\",\"outcome\":\"sensor_error\",\"ts\":1}\n";
    sartre_perceive_from_events(failure, &p);
    sartre_perceive_to_aml(&p, buf, sizeof buf);
    total++; pass += check("sensor failure -> still P3", buf, "PROPHECY 3");

    /* 10. plain text and quoted \"kind\" values are not events */
    const char *noise =
        "plain text with \"kind\" inside\n"
        "{\"util\":\"repo_monitor\",\"note\":\"kind is not a key\"}\n";
    sartre_perceive_from_events(noise, &p);
    total++;
    int ok10 = (p.changed == 0 && p.identity_recognized == 0 &&
                p.identity_reduced == 0 && p.sensor_failures == 0);
    printf("[perc] %-28s %s\n", "ignore forged kind noise", ok10 ? "PASS" : "FAIL");
    pass += ok10;

    printf("[perc] %d/%d PASS\n", pass, total);
    return pass == total ? 0 : 1;
}
#endif
