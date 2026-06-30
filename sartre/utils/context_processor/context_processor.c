/*
 * context_processor — SARTRE perception utility #2.
 *
 * Where repo_monitor reports that a file changed (a structural signal), this reads
 * a file's extracted TEXT and scores how much it overlaps Yent's vocabulary, two ways:
 *   - relevance: a lexical overlap — the fraction of the text's words that are in Yent's
 *     seed vocabulary (distinct seed hits / total words); not a set Jaccard;
 *   - resonance: a deterministic echo-state RESERVOIR score — the cosine between the
 *     reservoir's response to the text's bag-of-words and its response to Yent's seed
 *     vocabulary. It is a NONLINEAR LEXICAL signal (a fixed random reservoir over hashed
 *     word counts), NOT a trained classifier and NOT semantic understanding; it tracks
 *     lexical overlap through the reservoir's nonlinearity and is correlated with
 *     relevance. Honest scope: a nonlinear lexical reservoir score, no more.
 * Plus a chaos pulse. No "neural classification" is claimed — an earlier version returned
 * an argmax tag over an untrained random readout (noise); that is removed.
 *
 * Ported from Indiana-AM utils/context_neural_processor.py (numpy) to C + notorch:
 * the reservoir matvecs run through nt_blas_matvec; weights are filled by a fixed seeded
 * xorshift (reproducible, NOT nt_tensor_rand); spectral radius via zero-dep power iteration.
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

/* relevance = (distinct seed words present) / (total words) — a lexical overlap
 * fraction, NOT a set Jaccard (no union in the denominator) */
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

/* ── Echo-state reservoir (notorch matvec) — nonlinear lexical resonance ── */
#define ESN_INPUT 512

typedef struct {
    nt_tensor *W_in;   /* hidden x INPUT  — seeded, fixed reservoir input map */
    nt_tensor *W;      /* hidden x hidden — seeded, spectral-scaled reservoir  */
    int    hidden;
    float  leaky;
    float *ref;        /* reservoir state of Yent's seed vocabulary (the reference) */
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

/* Deterministic reservoir fill — a FIXED seeded random map, so the same content
 * always yields the same reservoir state (the resonance is reproducible, never
 * the run-to-run noise an unseeded RNG would give). */
static void reservoir_fill(float *a, int n, float scale, unsigned int *rng) {
    for (int i = 0; i < n; i++) {
        *rng ^= *rng << 13; *rng ^= *rng >> 17; *rng ^= *rng << 5;
        a[i] = ((float)(*rng & 0xFFFFFF) / (float)0x1000000) * 2.0f * scale - scale;
    }
}

static unsigned long bow_hash(const char *tok) {
    unsigned long h = 5381;
    int c;
    while ((c = (unsigned char)*tok++)) h = ((h << 5) + h) + (unsigned long)c;
    return h;
}

/* Bag-of-words input: each \w+ token (lowercased) hashed into one of ESN_INPUT bins
 * and counted, then L2-normalized. This is the content the reservoir scores. */
static void bow_input(const char *text, float *input) {
    for (int i = 0; i < ESN_INPUT; i++) input[i] = 0.0f;
    if (!text) return;
    char tok[64];
    int n = 0;
    for (const char *p = text;; p++) {
        if (isalnum((unsigned char)*p) || *p == '_') {
            if (n < (int)sizeof(tok) - 1) tok[n++] = (char)tolower((unsigned char)*p);
        } else {
            if (n > 0) { tok[n] = '\0'; input[bow_hash(tok) % ESN_INPUT] += 1.0f; n = 0; }
            if (*p == '\0') break;
        }
    }
    float norm = 0.0f;
    for (int i = 0; i < ESN_INPUT; i++) norm += input[i] * input[i];
    norm = sqrtf(norm);
    if (norm > 0.0f) for (int i = 0; i < ESN_INPUT; i++) input[i] /= norm;
}

/* Run the reservoir over a bag-of-words input; settle a few steps (echo-state
 * fading memory); write the hidden state into out_state[hidden]. matvec via notorch.
 * Returns 0 on success, -1 on allocation failure. */
static int reservoir_state(ESN *e, const char *text, float *out_state) {
    int H = e->hidden;
    float *input = calloc(ESN_INPUT, sizeof(float));
    float *a = malloc((size_t)H * sizeof(float));
    float *b = malloc((size_t)H * sizeof(float));
    if (!input || !a || !b) { free(input); free(a); free(b); return -1; }
    bow_input(text, input);
    for (int i = 0; i < H; i++) out_state[i] = 0.0f;
    for (int step = 0; step < 4; step++) {                       /* settle */
        nt_blas_matvec(a, e->W_in->data, input, H, ESN_INPUT);   /* a = W_in · input */
        nt_blas_matvec(b, e->W->data, out_state, H, H);          /* b = W · state    */
        for (int i = 0; i < H; i++)
            out_state[i] = e->leaky * out_state[i] + (1.0f - e->leaky) * tanhf(a[i] + b[i]);
    }
    free(input); free(a); free(b);
    return 0;
}

static void esn_free(ESN *e) {
    if (e->W_in) nt_tensor_free(e->W_in);
    if (e->W)    nt_tensor_free(e->W);
    free(e->ref);
    memset(e, 0, sizeof(*e));
}

/* Build the fixed seeded reservoir and the reference state from Yent's vocabulary.
 * Returns 0 on success, -1 on allocation failure (frees any partial allocation). */
static int esn_init(ESN *e, int content_size) {
    memset(e, 0, sizeof(*e));
    int hidden = 512;
    if (content_size / 1000 > hidden) hidden = content_size / 1000;
    if (hidden > 1024) hidden = 1024;
    e->hidden = hidden;
    e->leaky  = 0.8f + fminf(0.15f, (float)content_size / 1000000.0f);

    e->W_in = nt_tensor_new2d(hidden, ESN_INPUT);
    e->W    = nt_tensor_new2d(hidden, hidden);
    e->ref  = malloc((size_t)hidden * sizeof(float));
    if (!e->W_in || !e->W || !e->ref) { esn_free(e); return -1; }

    /* FIXED seed → reproducible reservoir (not nt_tensor_rand's unseeded RNG) */
    unsigned int rng = 0x5a17e5edu;
    reservoir_fill(e->W_in->data, hidden * ESN_INPUT, 0.1f, &rng);
    reservoir_fill(e->W->data, hidden * hidden, 0.9f, &rng);

    float rho = spectral_radius(e->W->data, hidden, 60);
    if (rho > 0.0f)
        for (int i = 0; i < hidden * hidden; i++) e->W->data[i] /= rho;  /* echo-state scaling */

    /* the reference: the reservoir's response to Yent's own vocabulary */
    char seed[512];
    int so = 0;
    for (int i = 0; i < SEED_N && so < (int)sizeof(seed) - 32; i++)
        so += snprintf(seed + so, sizeof(seed) - (size_t)so, "%s ", SEED_CORPUS[i]);
    if (reservoir_state(e, seed, e->ref) != 0) { esn_free(e); return -1; }
    return 0;
}

/* RESONANCE: cosine between the content's reservoir state and Yent's vocabulary
 * reference state. Real reservoir computing — a deterministic content score against
 * Yent's vocabulary, not a trained classifier and not a random tag. Range [-1, 1];
 * higher = the content's reservoir trajectory resonates with Yent's vocabulary. */
static float esn_resonance(ESN *e, const char *content) {
    float *cs = malloc((size_t)e->hidden * sizeof(float));
    if (!cs) return 0.0f;
    if (reservoir_state(e, content, cs) != 0) { free(cs); return 0.0f; }
    float dot = 0.0f, nc = 0.0f, nr = 0.0f;
    for (int i = 0; i < e->hidden; i++) {
        dot += cs[i] * e->ref[i];
        nc  += cs[i] * cs[i];
        nr  += e->ref[i] * e->ref[i];
    }
    free(cs);
    if (nc <= 0.0f || nr <= 0.0f) return 0.0f;
    return dot / (sqrtf(nc) * sqrtf(nr));
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
        return empty;  /* binary: no extracted text -> empty content -> resonance ~0 */
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
    float resonance = esn_resonance(&e, content);   /* reservoir cosine vs Yent's vocabulary */
    esn_free(&e);

    char path_esc[2048];
    json_escape_into(path, path_esc, (int)sizeof(path_esc));
    printf("{\"util\":\"context_processor\",\"path\":\"%s\",\"resonance\":%.4f,"
           "\"relevance\":%.4f,\"pulse\":%.4f}\n",
           path_esc, resonance, relevance, pulse_eff);

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

    /* 1-2. reservoir: init, spectral radius ~1, resonance — gated behind init success
     * so a (defensive) alloc failure never dereferences a NULL tensor. */
    ctx_srand(42);
    ESN e;
    int init_ok = esn_init(&e, 4000);
    total++; pass += check("esn init ok", init_ok == 0);
    if (init_ok == 0) {
        float rho_after = spectral_radius(e.W->data, e.hidden, 80);
        total++; pass += check("spectral radius ~1 after scale", fabsf(rho_after - 1.0f) < 0.05f);

        /* resonance discriminates by LEXICAL overlap: Yent-vocabulary text > unrelated */
        float r_yent  = esn_resonance(&e, "resonance field organism recursion dario limpha soul");
        float r_other = esn_resonance(&e, "quarterly invoice tax spreadsheet shipping logistics");
        total++; pass += check("resonance discriminates (yent > other)", r_yent > r_other);
        /* honest scope: LEXICAL, not semantic — a Yent-MEANING paraphrase built from
         * non-seed synonyms scores near the unrelated baseline, not near r_yent. This
         * test exists to keep the claim honest (the reservoir tracks word overlap, not
         * meaning). */
        float r_para = esn_resonance(&e, "vibration domain creature looping spiral essence breath");
        total++; pass += check("resonance is lexical not semantic (paraphrase low)",
                               r_para < (r_yent + r_other) / 2.0f);
        /* deterministic: same content -> identical value (seeded reservoir) */
        float r_again = esn_resonance(&e, "resonance field organism recursion dario limpha soul");
        total++; pass += check("resonance deterministic", r_yent == r_again);
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
