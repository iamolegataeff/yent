/*
 * context_processor — SARTRE perception utility #2.
 *
 * Where repo_monitor reports that a file changed (a structural signal), this reads
 * a file's CONTENT and produces a neural perception of it: an Echo State Network
 * (reservoir computing) over the bytes, a resonance/relevance score against Yent's
 * own vocabulary, and a chaos pulse. The model gets not just "README moved" but
 * "and here is how it feels / how much it resonates".
 *
 * Ported from Indiana-AM utils/context_neural_processor.py (numpy) to C + notorch:
 * the MiniESN's matvecs run through nt_blas_matvec, weights via nt_tensor_rand, and
 * the numpy eigvals spectral-radius step is replaced by zero-dep power iteration.
 * No external dependencies beyond libc + system notorch (install path).
 *
 * Build:  cc -O2 -Wall -Wextra context_processor.c \
 *             -I/opt/homebrew/include -L/opt/homebrew/lib -lnotorch -lm -o context_processor
 * Test:   cc -DSARTRE_CTX_TEST ... (same flags)
 *
 * by Arianna Method
 */

#include <notorch.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <math.h>
#include <ctype.h>
#include <limits.h>

/* ── deterministic RNG (xorshift32) — reproducible somatic noise for tests ── */
static unsigned int g_rng = 0x9e3779b9u;
static void ctx_srand(unsigned int s) { g_rng = s ? s : 1u; }
static float rng_uniform(float lo, float hi) {
    g_rng ^= g_rng << 13; g_rng ^= g_rng >> 17; g_rng ^= g_rng << 5;
    return lo + (hi - lo) * ((float)(g_rng & 0xFFFFFF) / (float)0x1000000);
}

static float clampf(float v, float lo, float hi) {
    return v < lo ? lo : (v > hi ? hi : v);
}

/* ── word-keyword sum (case-insensitive whole \w+ tokens) ── */
typedef struct { const char *word; float weight; } KW;

static float keyword_sum(const char *text, const KW *kws, int nkw) {
    if (!text) return 0.0f;
    float total = 0.0f;
    const char *p = text;
    char tok[64];
    while (*p) {
        while (*p && !(isalnum((unsigned char)*p) || *p == '_')) p++;
        int n = 0;
        while (*p && (isalnum((unsigned char)*p) || *p == '_') && n < (int)sizeof(tok) - 1) {
            tok[n++] = (char)tolower((unsigned char)*p); p++;
        }
        tok[n] = '\0';
        if (n == 0) continue;
        for (int i = 0; i < nkw; i++) {
            if (strcmp(tok, kws[i].word) == 0) { total += kws[i].weight; break; }
        }
    }
    return total;
}

/* ── Yent resonance vocabulary (replaces Indiana's Musk-domain seed corpus) ── */
static const char *SEED_CORPUS[] = {
    "resonance", "field", "recursion", "emergence", "yent", "arianna", "method",
    "dario", "equation", "soul", "memory", "body", "pulse", "chaos", "void",
    "spark", "file", "process", "data", "extract", "neural", "cosmos", "prophecy",
    "velocity", "scar", "cooc", "limpha", "doe", "kuramoto", "hebbian"
};
static const int SEED_N = (int)(sizeof(SEED_CORPUS) / sizeof(SEED_CORPUS[0]));

/* relevance = |seed ∩ text_words| / |text_words| (Jaccard-style, as in the original) */
static float compute_relevance(const char *text) {
    if (!text || !*text) return 0.0f;
    int seen_seed = 0, total_words = 0;
    /* one pass: count words, and count distinct-ish seed hits (cap each seed once) */
    int hit[64] = {0};
    const char *p = text;
    char tok[64];
    while (*p) {
        while (*p && !(isalnum((unsigned char)*p) || *p == '_')) p++;
        int n = 0;
        while (*p && (isalnum((unsigned char)*p) || *p == '_') && n < (int)sizeof(tok) - 1) {
            tok[n++] = (char)tolower((unsigned char)*p); p++;
        }
        tok[n] = '\0';
        if (n == 0) continue;
        total_words++;
        for (int i = 0; i < SEED_N && i < 64; i++) {
            if (!hit[i] && strcmp(tok, SEED_CORPUS[i]) == 0) { hit[i] = 1; seen_seed++; break; }
        }
    }
    if (total_words == 0) return 0.0f;
    return (float)seen_seed / (float)total_words;
}

/* ── ChaosPulse: sentiment keywords nudge a pulse in [0.1, 0.9] ── */
static float chaos_pulse(const char *text) {
    static const KW kws[] = {
        {"success", 0.2f}, {"error", -0.25f}, {"failure", -0.3f}, {"data", 0.1f},
        {"resonance", 0.15f}, {"chaos", 0.1f}, {"field", 0.1f}, {"void", -0.1f},
    };
    float change = keyword_sum(text, kws, (int)(sizeof(kws) / sizeof(kws[0])));
    return clampf(0.5f + change + rng_uniform(-0.05f, 0.05f), 0.1f, 0.9f);
}

/* ── somatic organs (faithful float dynamics): a body-feel pulse ── */
static float somatic_pulse(float intensity) {
    /* BloodFlux(iron=0.6).circulate -> SixthSense.foresee, light port */
    static float blood = 0.0f, chaos = 0.0f;
    blood = clampf(blood * 0.9f + intensity * 0.6f + rng_uniform(-0.03f, 0.03f), 0.0f, 1.0f);
    chaos = clampf(chaos * 0.95f + intensity * 0.2f + rng_uniform(-0.02f, 0.02f), 0.0f, 1.0f);
    return (blood + chaos) * 0.5f;
}

/* ── MiniESN on notorch ── */
#define ESN_INPUT  512
#define ESN_OUTPUT 14

static const char *ESN_TAGS[ESN_OUTPUT] = {
    ".pdf", ".txt", ".md", ".docx", ".rtf", ".doc", ".odt",
    ".zip", ".tar", ".png", ".html", ".json", ".csv", ".yaml"
};

typedef struct {
    nt_tensor *W_in;   /* hidden x INPUT */
    nt_tensor *W;      /* hidden x hidden */
    nt_tensor *W_out;  /* OUTPUT x hidden */
    nt_tensor *state;  /* hidden */
    int   hidden;
    float leaky;
} ESN;

/* power iteration: dominant |eigenvalue| of W (n x n), replacing numpy eigvals.
 * matvec via notorch (hot path). Returns spectral-radius estimate. */
static float spectral_radius(const float *W, int n, int iters) {
    float *v = malloc((size_t)n * sizeof(float));
    float *w = malloc((size_t)n * sizeof(float));
    if (!v || !w) { free(v); free(w); return 0.0f; }
    float inv = 1.0f / sqrtf((float)n);
    for (int i = 0; i < n; i++) v[i] = inv;
    float lambda = 0.0f;
    for (int it = 0; it < iters; it++) {
        nt_blas_matvec(w, W, v, n, n);
        float norm = 0.0f;
        for (int i = 0; i < n; i++) norm += w[i] * w[i];
        norm = sqrtf(norm);
        if (norm <= 0.0f) break;
        lambda = norm;
        for (int i = 0; i < n; i++) v[i] = w[i] / norm;
    }
    free(v); free(w);
    return lambda;
}

static void esn_free(ESN *e) {
    if (e->W_in)  nt_tensor_free(e->W_in);
    if (e->W)     nt_tensor_free(e->W);
    if (e->W_out) nt_tensor_free(e->W_out);
    if (e->state) nt_tensor_free(e->state);
    memset(e, 0, sizeof(*e));
}

/* Returns 0 on success, -1 on allocation failure (frees any partial allocation). */
static int esn_init(ESN *e, int content_size) {
    memset(e, 0, sizeof(*e));
    int hidden = 512;
    if (content_size / 1000 > hidden) hidden = content_size / 1000;
    if (hidden > 1024) hidden = 1024;
    e->hidden = hidden;
    e->leaky  = 0.8f + fminf(0.15f, (float)content_size / 1000000.0f);

    e->W_in  = nt_tensor_new2d(hidden, ESN_INPUT);
    e->W     = nt_tensor_new2d(hidden, hidden);
    e->W_out = nt_tensor_new2d(ESN_OUTPUT, hidden);
    e->state = nt_tensor_new(hidden);
    if (!e->W_in || !e->W || !e->W_out || !e->state) {
        esn_free(e);
        return -1;
    }
    nt_tensor_rand(e->W_in, 0.1f);
    nt_tensor_rand(e->W, 0.9f);
    nt_tensor_rand(e->W_out, 0.1f);
    nt_tensor_fill(e->state, 0.0f);

    float rho = spectral_radius(e->W->data, hidden, 60);
    if (rho > 0.0f) {
        for (int i = 0; i < hidden * hidden; i++) e->W->data[i] /= rho;  /* echo-state scaling */
    }
    return 0;
}

/* forward: bytes -> reservoir -> tag index [0, ESN_OUTPUT). pulse modulates output. */
static int esn_forward(ESN *e, const unsigned char *data, int len, const char *content, float pulse) {
    int H = e->hidden;
    float *input = calloc(ESN_INPUT, sizeof(float));
    float *a = malloc((size_t)H * sizeof(float));
    float *b = malloc((size_t)H * sizeof(float));
    if (!input || !a || !b) { free(input); free(a); free(b); return -1; }

    int n = len < ESN_INPUT ? len : ESN_INPUT;
    for (int i = 0; i < n; i++) input[i] = (float)data[i] / 255.0f;

    static const KW boost_kw[] = {
        {"resonance", 0.15f}, {"field", 0.15f}, {"recursion", 0.10f}, {"chaos", 0.10f},
    };
    float boost = keyword_sum(content, boost_kw, (int)(sizeof(boost_kw) / sizeof(boost_kw[0])));

    nt_blas_matvec(a, e->W_in->data, input, H, ESN_INPUT);     /* a = W_in · input */
    nt_blas_matvec(b, e->W->data, e->state->data, H, H);       /* b = W · state    */
    for (int i = 0; i < H; i++) {
        float pre = a[i] + b[i] + boost;
        e->state->data[i] = e->leaky * e->state->data[i] + (1.0f - e->leaky) * tanhf(pre);
    }

    float out[ESN_OUTPUT];
    nt_blas_matvec(out, e->W_out->data, e->state->data, ESN_OUTPUT, H);  /* out = W_out · state */

    int best = 0;
    float bestv = -1e30f;
    for (int i = 0; i < ESN_OUTPUT; i++) {
        float w = out[i] * (1.0f + pulse * 0.7f);  /* apply_pulse (pre-softmax monotone) */
        if (w > bestv) { bestv = w; best = i; }
    }
    free(input); free(a); free(b);
    return best;
}

/* ── zero-dep text extraction: text formats raw, html tag-stripped, binary -> "" ── */
static int has_suffix_ci(const char *s, const char *suf) {
    size_t ls = strlen(s), lf = strlen(suf);
    if (lf > ls) return 0;
    for (size_t i = 0; i < lf; i++)
        if (tolower((unsigned char)s[ls - lf + i]) != tolower((unsigned char)suf[i])) return 0;
    return 1;
}

/* returns malloc'd text content (caller frees); *is_text set 1 for text formats. */
static char *extract_text(const char *path, const unsigned char *bytes, int len, int *is_text) {
    *is_text = 0;
    int text_fmt = has_suffix_ci(path, ".txt") || has_suffix_ci(path, ".md") ||
                   has_suffix_ci(path, ".json") || has_suffix_ci(path, ".csv") ||
                   has_suffix_ci(path, ".yaml") || has_suffix_ci(path, ".yml") ||
                   has_suffix_ci(path, ".rs") || has_suffix_ci(path, ".c") ||
                   has_suffix_ci(path, ".h") || has_suffix_ci(path, ".go");
    int html_fmt = has_suffix_ci(path, ".html") || has_suffix_ci(path, ".htm");

    if (!text_fmt && !html_fmt) {
        char *empty = malloc(1); if (empty) empty[0] = '\0';
        return empty;  /* binary: no text content (ESN still runs on raw bytes) */
    }

    *is_text = 1;
    char *out = malloc((size_t)len + 1);
    if (!out) return NULL;
    int o = 0;
    if (html_fmt) {
        int in_tag = 0;
        for (int i = 0; i < len; i++) {
            char c = (char)bytes[i];
            if (c == '<') { in_tag = 1; continue; }
            if (c == '>') { in_tag = 0; out[o++] = ' '; continue; }
            if (!in_tag) out[o++] = c;
        }
    } else {
        for (int i = 0; i < len; i++) out[o++] = (char)bytes[i];
    }
    out[o] = '\0';
    return out;
}

/* read whole file into a malloc'd byte buffer; returns len or -1. */
static unsigned char *read_file(const char *path, int *out_len) {
    FILE *f = fopen(path, "rb");
    if (!f) return NULL;
    fseek(f, 0, SEEK_END);
    long sz = ftell(f);
    if (sz < 0 || sz > (long)INT_MAX) { fclose(f); return NULL; }  /* keep len in int range */
    fseek(f, 0, SEEK_SET);
    unsigned char *buf = malloc((size_t)sz + 1);
    if (!buf) { fclose(f); return NULL; }
    size_t got = fread(buf, 1, (size_t)sz, f);
    fclose(f);
    buf[got] = '\0';
    *out_len = (int)got;
    return buf;
}

static void json_escape_into(const char *s, char *out, int max) {
    int o = 0;
    for (const char *p = s; *p && o < max - 7; p++) {
        unsigned char c = (unsigned char)*p;
        if (c == '"')  { out[o++] = '\\'; out[o++] = '"'; }
        else if (c == '\\') { out[o++] = '\\'; out[o++] = '\\'; }
        else if (c < 0x20) { o += snprintf(out + o, max - o, "\\u%04x", c); }
        else out[o++] = (char)c;
    }
    out[o] = '\0';
}

#ifndef SARTRE_CTX_TEST
int main(int argc, char **argv) {
    if (argc < 2) {
        fprintf(stderr, "usage: context_processor <file> [seed]\n");
        return 2;
    }
    const char *path = argv[1];
    ctx_srand(argc > 2 ? (unsigned int)strtoul(argv[2], NULL, 10) : 0x9e3779b9u);

    int len = 0;
    unsigned char *bytes = read_file(path, &len);
    if (!bytes) { fprintf(stderr, "[context_processor] cannot read %s\n", path); return 1; }

    int is_text = 0;
    char *content = extract_text(path, bytes, len, &is_text);
    if (!content) { free(bytes); return 1; }

    float pulse = chaos_pulse(content);
    float relevance = compute_relevance(content);
    float soma = somatic_pulse(relevance);
    float pulse_eff = clampf(pulse * 0.5f + soma * 0.5f, 0.0f, 1.0f);

    ESN e;
    if (esn_init(&e, len) != 0) {
        fprintf(stderr, "[context_processor] esn init failed\n");
        free(content); free(bytes);
        return 1;
    }
    int tag = esn_forward(&e, bytes, len, content, pulse_eff);
    esn_free(&e);

    char path_esc[2048];
    json_escape_into(path, path_esc, (int)sizeof(path_esc));
    printf("{\"util\":\"context_processor\",\"path\":\"%s\",\"tag\":\"%s\","
           "\"relevance\":%.4f,\"pulse\":%.4f}\n",
           path_esc, (tag >= 0 ? ESN_TAGS[tag] : "?"), relevance, pulse_eff);

    free(content);
    free(bytes);
    return 0;
}
#else
/* ── self-test ── */
static int check(const char *label, int ok) {
    printf("[ctx] %-34s %s\n", label, ok ? "PASS" : "FAIL");
    return ok;
}

int main(void) {
    int pass = 0, total = 0;

    /* 1-2. ESN: init, spectral radius ~1, forward tag — gated behind init success
     * so a (defensive) alloc failure never dereferences a NULL tensor. */
    ctx_srand(42);
    ESN e;
    int init_ok = esn_init(&e, 4000);
    total++; pass += check("esn init ok", init_ok == 0);
    if (init_ok == 0) {
        float rho_after = spectral_radius(e.W->data, e.hidden, 80);
        total++; pass += check("spectral radius ~1 after scale", fabsf(rho_after - 1.0f) < 0.05f);

        const unsigned char data[] = "## Yent README\nresonance field recursion";
        int t1 = esn_forward(&e, data, (int)sizeof(data) - 1, "resonance field", 0.5f);
        total++; pass += check("forward tag in range", t1 >= 0 && t1 < ESN_OUTPUT);
        total++; pass += check("tag name maps", t1 >= 0 && ESN_TAGS[t1][0] == '.');
        esn_free(&e);
    }

    /* 3. relevance: resonance text > 0, empty = 0 */
    float r1 = compute_relevance("the resonance field holds recursion and memory");
    float r0 = compute_relevance("");
    total++; pass += check("relevance: resonance>0", r1 > 0.0f);
    total++; pass += check("relevance: empty==0", r0 == 0.0f);

    /* 4. chaos pulse bounded [0.1,0.9] */
    ctx_srand(7);
    float p = chaos_pulse("success data resonance chaos");
    total++; pass += check("chaos pulse in [0.1,0.9]", p >= 0.1f && p <= 0.9f);

    /* 5. html tag-strip */
    const unsigned char html[] = "<html><body>hello <b>world</b></body></html>";
    int it = 0;
    char *txt = extract_text("x.html", html, (int)sizeof(html) - 1, &it);
    int stripped = txt && it && strstr(txt, "hello") && strstr(txt, "world") && !strchr(txt, '<');
    total++; pass += check("html tag-strip", stripped);
    free(txt);

    /* 6. binary -> empty content, marked non-text */
    const unsigned char png[] = { 0x89, 'P', 'N', 'G', 0x00, 0x01 };
    int it2 = 0;
    char *bt = extract_text("x.png", png, (int)sizeof(png), &it2);
    total++; pass += check("binary -> empty content", bt && bt[0] == '\0' && it2 == 0);
    free(bt);

    /* 7. somatic pulse bounded [0,1] */
    ctx_srand(3);
    float s = somatic_pulse(0.5f);
    total++; pass += check("somatic pulse in [0,1]", s >= 0.0f && s <= 1.0f);

    /* 8. json escape quotes/backslash */
    char esc[64];
    json_escape_into("a\"b\\c", esc, (int)sizeof(esc));
    total++; pass += check("json escape", strcmp(esc, "a\\\"b\\\\c") == 0);

    /* 9. read_file round-trip */
    {
        const char *tmp = "/tmp/.sartre_ctx_rf";
        FILE *wf = fopen(tmp, "wb");
        int rf_ok = 0;
        if (wf) {
            fputs("hello", wf);
            fclose(wf);
            int l = 0;
            unsigned char *b = read_file(tmp, &l);
            rf_ok = b && l == 5 && memcmp(b, "hello", 5) == 0;
            free(b);
            remove(tmp);
        }
        total++; pass += check("read_file round-trip", rf_ok);
    }

    printf("[ctx] %d/%d PASS\n", pass, total);
    return pass == total ? 0 : 1;
}
#endif
