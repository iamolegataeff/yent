// pixtral_vision.c — native Pixtral vision encoder for Yent 24B (Arianna Method).
// Reimplements llama.cpp's clip pixtral path (tools/mtmd/models/pixtral.cpp) in pure C
// so the inference (doe) sees images with NO llama.cpp at runtime.
// Spec/blueprint: _notes/pixtral_spec_2026-06-20.md. Verified stage-by-stage against
// clip ground-truth dumps (env DUMP_CLIP_INPRAW / DUMP_CLIP_NODES / DUMP_CLIP_BIN in clip.cpp).
//
// Stages so far: 1 preprocess (bit-exact), 2 patch_embed conv (rel 2.5e-4), 3 ViT trunk.
// Build (test):  cc -O2 -DPV_TEST pixtral_vision.c gguf.c -framework Accelerate -lm -o pv_test
// Run:           ./pv_test <stage> <image> <mmproj.gguf> <groundtruth.bin>

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <math.h>
#include <Accelerate/Accelerate.h>

#define STB_IMAGE_IMPLEMENTATION
#include "stb_image.h"
#include "gguf.h"

// ── Pixtral constants (clip.cpp:1319-1329, clip-model.h:131-137, gguf hparams) ──
#define PV_PATCH   14
#define PV_NMERGE  2
#define PV_ALIGN   (PV_PATCH * PV_NMERGE)        // 28
#define PV_DHEAD   64
#define PV_NHEAD   16
#define PV_EPS     1e-5f
#define PV_ROPE_THETA 10000.0f
static const long  PV_MIN_PX = 6272;             // 8    * (14*14*2*2)=784
static const long  PV_MAX_PX = 802816;           // 1024 * 784
static const float PV_MEAN[3] = {0.48145466f, 0.45782750f, 0.40821073f};
static const float PV_STD[3]  = {0.26862954f, 0.26130258f, 0.27577711f};

static int pv_imax(int a, int b) { return a > b ? a : b; }
static int pv_imin(int a, int b) { return a < b ? a : b; }
static int pv_round_by(double x) { return (int)(round(x / PV_ALIGN) * PV_ALIGN); }
static int pv_ceil_by (double x) { return (int)(ceil (x / PV_ALIGN) * PV_ALIGN); }
static int pv_floor_by(double x) { return (int)(floor(x / PV_ALIGN) * PV_ALIGN); }

// ── Stage 1: preprocessing (smart_resize + bilinear + PAD_CEIL + normalize + planar) ──
static void pv_smart_resize(int W, int H, int *tw, int *th) {
    int h_bar = pv_imax(PV_ALIGN, pv_round_by(H));
    int w_bar = pv_imax(PV_ALIGN, pv_round_by(W));
    if ((long)h_bar * w_bar > PV_MAX_PX) {
        double beta = sqrt((double)H * W / (double)PV_MAX_PX);
        h_bar = pv_imax(PV_ALIGN, pv_floor_by(H / beta));
        w_bar = pv_imax(PV_ALIGN, pv_floor_by(W / beta));
    } else if ((long)h_bar * w_bar < PV_MIN_PX) {
        double beta = sqrt((double)PV_MIN_PX / ((double)H * W));
        h_bar = pv_ceil_by(H * beta);
        w_bar = pv_ceil_by(W * beta);
    }
    *tw = w_bar; *th = h_bar;
}

static void pv_resize_bilinear_u8(const unsigned char *src, int sw, int sh,
                                  unsigned char *dst, int tw, int th) {
    float x_ratio = tw > 1 ? (float)(sw - 1) / (tw - 1) : 0.f;
    float y_ratio = th > 1 ? (float)(sh - 1) / (th - 1) : 0.f;
    for (int y = 0; y < th; y++) {
        float py = y * y_ratio;
        int y0 = pv_imin((int)py, sh - 1), y1 = pv_imin(y0 + 1, sh - 1);
        float yf = py - y0;
        for (int x = 0; x < tw; x++) {
            float px = x * x_ratio;
            int x0 = pv_imin((int)px, sw - 1), x1 = pv_imin(x0 + 1, sw - 1);
            float xf = px - x0;
            for (int c = 0; c < 3; c++) {
                float tl = src[3 * (y0 * sw + x0) + c], tr = src[3 * (y0 * sw + x1) + c];
                float bl = src[3 * (y1 * sw + x0) + c], br = src[3 * (y1 * sw + x1) + c];
                float top = tl + (tr - tl) * xf, bot = bl + (br - bl) * xf;
                dst[3 * (y * tw + x) + c] = (unsigned char)(top + (bot - top) * yf); // trunc
            }
        }
    }
}

float *pv_preprocess(const char *path, int *onx, int *ony) {
    int W, H, ch;
    unsigned char *img = stbi_load(path, &W, &H, &ch, 3);
    if (!img) { fprintf(stderr, "pv: cannot load %s\n", path); return NULL; }
    int tw, th;
    pv_smart_resize(W, H, &tw, &th);
    float scale_w = (float)tw / W, scale_h = (float)th / H;
    float scale = scale_w < scale_h ? scale_w : scale_h;
    int new_w = pv_imin((int)ceil(W * scale), tw);
    int new_h = pv_imin((int)ceil(H * scale), th);
    unsigned char *tmp = (unsigned char *)malloc((size_t)new_w * new_h * 3);
    pv_resize_bilinear_u8(img, W, H, tmp, new_w, new_h);
    unsigned char *canvas = (unsigned char *)calloc((size_t)tw * th * 3, 1);
    int ox = (tw - new_w) / 2, oy = (th - new_h) / 2;
    for (int y = 0; y < new_h; y++)
        memcpy(canvas + 3 * ((size_t)(oy + y) * tw + ox), tmp + 3 * (size_t)y * new_w, (size_t)new_w * 3);
    long n = (long)tw * th;
    float *out = (float *)malloc((size_t)n * 3 * sizeof(float));
    for (long i = 0; i < n; i++)
        for (int c = 0; c < 3; c++)
            out[(long)c * n + i] = (canvas[3 * i + c] / 255.0f - PV_MEAN[c]) / PV_STD[c];
    free(img); free(tmp); free(canvas);
    *onx = tw; *ony = th;
    return out;
}

// ── Stage 2: patch_embed conv (v.patch_embd[14,14,3,1024] stride14, no pad/bias) ──
// inp planar [W,H,3]; out [n_embd,n_pos] oc-fastest, patch=ow+OW*oh. w[kx+14ky+196ci+588oc].
float *pv_patch_embed(const float *inp, int W, int H, gguf_file *gf, int *o_npos, int *o_nembd) {
    int idx = gguf_find_tensor(gf, "v.patch_embd.weight");
    if (idx < 0) { fprintf(stderr, "pv: v.patch_embd.weight not found\n"); return NULL; }
    int NE = (int)gf->tensors[idx].shape[3];
    float *w = gguf_dequant(gf, idx);
    if (!w) return NULL;
    int OW = W / PV_PATCH, OH = H / PV_PATCH, n_pos = OW * OH;
    float *out = (float *)malloc((size_t)NE * n_pos * sizeof(float));
    for (int patch = 0; patch < n_pos; patch++) {
        int oh = patch / OW, ow = patch % OW;
        for (int oc = 0; oc < NE; oc++) {
            float acc = 0.f;
            const float *wo = w + (size_t)oc * 588;
            for (int ci = 0; ci < 3; ci++)
                for (int ky = 0; ky < PV_PATCH; ky++) {
                    const float *row = inp + (size_t)ci * W * H + (size_t)(oh * PV_PATCH + ky) * W + ow * PV_PATCH;
                    const float *wk = wo + ci * 196 + ky * PV_PATCH;
                    for (int kx = 0; kx < PV_PATCH; kx++) acc += wk[kx] * row[kx];
                }
            out[oc + (size_t)NE * patch] = acc;
        }
    }
    free(w);
    *o_npos = n_pos; *o_nembd = NE;
    return out;
}

// ── Stage 3: 24-layer Pixtral ViT trunk ──
static float *pv_dq(gguf_file *gf, const char *name) {
    int idx = gguf_find_tensor(gf, name);
    if (idx < 0) { fprintf(stderr, "pv: %s missing\n", name); return NULL; }
    return gguf_dequant(gf, idx);
}
// out[o + outd*p] = sum_i W[i+in*o] * x[i+in*p]   (== W^T x, col-major); W ne=[in,out].
static void pv_matmul(const float *W, const float *x, float *out, int in, int outd, int n_pos) {
    cblas_sgemm(CblasColMajor, CblasTrans, CblasNoTrans, outd, n_pos, in,
                1.0f, W, in, x, in, 0.0f, out, outd);
}
static void pv_rmsnorm(const float *x, const float *w, float *out, int n_embd, int n_pos, float eps) {
    for (int p = 0; p < n_pos; p++) {
        const float *c = x + (size_t)n_embd * p; float *o = out + (size_t)n_embd * p;
        double ss = 0; for (int e = 0; e < n_embd; e++) ss += (double)c[e] * c[e];
        float inv = 1.0f / sqrtf((float)(ss / n_embd) + eps);
        for (int e = 0; e < n_embd; e++) o[e] = c[e] * inv * w[e];
    }
}
// 2D-RoPE on [d_head=64, n_head, n_pos], mode=0 adjacent pairs (build_rope_2d, ggml NORMAL).
static void pv_rope2d(float *Q, const int *ph, const int *pw, int n_pos) {
    for (int p = 0; p < n_pos; p++) {
        float fph = (float)ph[p], fpw = (float)pw[p];
        for (int h = 0; h < PV_NHEAD; h++) {
            float *q = Q + (size_t)PV_DHEAD * h + (size_t)PV_DHEAD * PV_NHEAD * p;
            for (int j = 0; j < 16; j++) {
                float th  = fph * powf(PV_ROPE_THETA, -2.0f * j / 32.0f);
                float c = cosf(th), s = sinf(th);
                float a = q[2 * j], b = q[2 * j + 1];
                q[2 * j] = a * c - b * s; q[2 * j + 1] = a * s + b * c;
                float th2 = fpw * powf(PV_ROPE_THETA, -(2.0f * j + 1.0f) / 32.0f);
                float c2 = cosf(th2), s2 = sinf(th2);
                float a2 = q[32 + 2 * j], b2 = q[32 + 2 * j + 1];
                q[32 + 2 * j] = a2 * c2 - b2 * s2; q[32 + 2 * j + 1] = a2 * s2 + b2 * c2;
            }
        }
    }
}
static float pv_silu(float x) { return x / (1.0f + expf(-x)); }

// Full ViT trunk: x = conv [n_embd,n_pos] -> trunk [n_embd,n_pos] (in place). Returns x.
float *pv_vit_trunk(float *x, int NE, int n_pos, gguf_file *gf, const int *ph, const int *pw) {
    const int NF = 4096;
    const float scale = 1.0f / sqrtf((float)PV_DHEAD);
    char nm[128];
    // pre_ln (RMSNorm) before blocks, if present.
    int pidx = gguf_find_tensor(gf, "v.pre_ln.weight");
    if (pidx >= 0) {
        float *w = gguf_dequant(gf, pidx);
        float *t = (float *)malloc((size_t)NE * n_pos * 4);
        pv_rmsnorm(x, w, t, NE, n_pos, PV_EPS);
        memcpy(x, t, (size_t)NE * n_pos * 4);
        free(t); free(w);
    }
    float *nrm = (float *)malloc((size_t)NE * n_pos * 4);
    float *Q = (float *)malloc((size_t)NE * n_pos * 4);
    float *K = (float *)malloc((size_t)NE * n_pos * 4);
    float *V = (float *)malloc((size_t)NE * n_pos * 4);
    float *Oc = (float *)malloc((size_t)NE * n_pos * 4);
    float *Qh = (float *)malloc((size_t)PV_DHEAD * n_pos * 4);
    float *Kh = (float *)malloc((size_t)PV_DHEAD * n_pos * 4);
    float *Vh = (float *)malloc((size_t)PV_DHEAD * n_pos * 4);
    float *Oh = (float *)malloc((size_t)PV_DHEAD * n_pos * 4);
    float *S = (float *)malloc((size_t)n_pos * n_pos * 4);
    float *g = (float *)malloc((size_t)NF * n_pos * 4);
    float *u = (float *)malloc((size_t)NF * n_pos * 4);

    for (int l = 0; l < 24; l++) {
        // ── attention ──
        snprintf(nm, sizeof nm, "v.blk.%d.ln1.weight", l); float *ln1 = pv_dq(gf, nm);
        pv_rmsnorm(x, ln1, nrm, NE, n_pos, PV_EPS); free(ln1);
        snprintf(nm, sizeof nm, "v.blk.%d.attn_q.weight", l); float *qw = pv_dq(gf, nm); pv_matmul(qw, nrm, Q, NE, NE, n_pos); free(qw);
        snprintf(nm, sizeof nm, "v.blk.%d.attn_k.weight", l); float *kw = pv_dq(gf, nm); pv_matmul(kw, nrm, K, NE, NE, n_pos); free(kw);
        snprintf(nm, sizeof nm, "v.blk.%d.attn_v.weight", l); float *vw = pv_dq(gf, nm); pv_matmul(vw, nrm, V, NE, NE, n_pos); free(vw);
        pv_rope2d(Q, ph, pw, n_pos);
        pv_rope2d(K, ph, pw, n_pos);
        for (int h = 0; h < PV_NHEAD; h++) {
            for (int p = 0; p < n_pos; p++) {
                memcpy(Qh + (size_t)PV_DHEAD * p, Q + (size_t)PV_DHEAD * h + (size_t)NE * p, PV_DHEAD * 4);
                memcpy(Kh + (size_t)PV_DHEAD * p, K + (size_t)PV_DHEAD * h + (size_t)NE * p, PV_DHEAD * 4);
                memcpy(Vh + (size_t)PV_DHEAD * p, V + (size_t)PV_DHEAD * h + (size_t)NE * p, PV_DHEAD * 4);
            }
            // S[pk + n_pos*pq] = scale * sum_d Kh[d,pk] Qh[d,pq]
            cblas_sgemm(CblasColMajor, CblasTrans, CblasNoTrans, n_pos, n_pos, PV_DHEAD,
                        scale, Kh, PV_DHEAD, Qh, PV_DHEAD, 0.0f, S, n_pos);
            for (int pq = 0; pq < n_pos; pq++) {
                float *col = S + (size_t)n_pos * pq;
                float mx = col[0];
                for (int pk = 1; pk < n_pos; pk++) if (col[pk] > mx) mx = col[pk];
                float sm = 0;
                for (int pk = 0; pk < n_pos; pk++) { col[pk] = expf(col[pk] - mx); sm += col[pk]; }
                float iv = 1.0f / sm;
                for (int pk = 0; pk < n_pos; pk++) col[pk] *= iv;
            }
            // Oh[d,pq] = sum_pk Vh[d,pk] S[pk,pq]
            cblas_sgemm(CblasColMajor, CblasNoTrans, CblasNoTrans, PV_DHEAD, n_pos, n_pos,
                        1.0f, Vh, PV_DHEAD, S, n_pos, 0.0f, Oh, PV_DHEAD);
            for (int p = 0; p < n_pos; p++)
                memcpy(Oc + (size_t)PV_DHEAD * h + (size_t)NE * p, Oh + (size_t)PV_DHEAD * p, PV_DHEAD * 4);
        }
        snprintf(nm, sizeof nm, "v.blk.%d.attn_out.weight", l); float *ow = pv_dq(gf, nm); pv_matmul(ow, Oc, nrm, NE, NE, n_pos); free(ow);
        for (size_t i = 0; i < (size_t)NE * n_pos; i++) x[i] += nrm[i];   // residual 1
        // ── ffn (SwiGLU) ──
        snprintf(nm, sizeof nm, "v.blk.%d.ln2.weight", l); float *ln2 = pv_dq(gf, nm);
        pv_rmsnorm(x, ln2, nrm, NE, n_pos, PV_EPS); free(ln2);
        snprintf(nm, sizeof nm, "v.blk.%d.ffn_gate.weight", l); float *gw = pv_dq(gf, nm); pv_matmul(gw, nrm, g, NE, NF, n_pos); free(gw);
        snprintf(nm, sizeof nm, "v.blk.%d.ffn_up.weight", l);   float *uw = pv_dq(gf, nm); pv_matmul(uw, nrm, u, NE, NF, n_pos); free(uw);
        for (size_t i = 0; i < (size_t)NF * n_pos; i++) g[i] = pv_silu(g[i]) * u[i];
        snprintf(nm, sizeof nm, "v.blk.%d.ffn_down.weight", l); float *dw = pv_dq(gf, nm); pv_matmul(dw, g, nrm, NF, NE, n_pos); free(dw);
        for (size_t i = 0; i < (size_t)NE * n_pos; i++) x[i] += nrm[i];   // residual 2
    }
    free(nrm); free(Q); free(K); free(V); free(Oc);
    free(Qh); free(Kh); free(Vh); free(Oh); free(S); free(g); free(u);
    return x; // no post_ln for pixtral
}

// ── Stage 4: patch merger (mistral small 3.1) ──
// trunk [NE, n_pos] (e+NE*p, p=ow+npx*oh) -> merged [NE, nm] (o+NE*t, t=ow_m+OWm*oh_m).
// RMSNorm(mm_input_norm) -> im2col 2x2 (vec c=kx+2ky+4ic) -> mm_patch_merger[4096->1024].
float *pv_merger(const float *trunk, int NE, int npx, int npy, gguf_file *gf, int *o_nm) {
    int n_pos = npx * npy;
    float *inw = pv_dq(gf, "mm.input_norm.weight");
    if (!inw) return NULL;
    float *yn = (float *)malloc((size_t)NE * n_pos * 4);
    pv_rmsnorm(trunk, inw, yn, NE, n_pos, PV_EPS); free(inw);
    int M = NE * 4;                 // 4096
    int OWm = npx / 2, OHm = npy / 2, nm = OWm * OHm;
    float *mw = pv_dq(gf, "mm.patch_merger.weight"); // ne=[4096,1024], W[c+4096*o]
    if (!mw) { free(yn); return NULL; }
    float *VEC = (float *)malloc((size_t)M * nm * 4);
    for (int ohm = 0; ohm < OHm; ohm++)
        for (int owm = 0; owm < OWm; owm++) {
            int t = owm + OWm * ohm;
            float *vc = VEC + (size_t)M * t;
            for (int ic = 0; ic < NE; ic++)
                for (int ky = 0; ky < 2; ky++)
                    for (int kx = 0; kx < 2; kx++) {
                        int X = 2 * owm + kx, Y = 2 * ohm + ky;
                        vc[kx + 2 * ky + 4 * ic] = yn[ic + (size_t)NE * (X + npx * Y)];
                    }
        }
    float *out = (float *)malloc((size_t)NE * nm * 4);
    pv_matmul(mw, VEC, out, M, NE, nm);   // out[o+NE*t] = sum_c mw[c+M*o] VEC[c+M*t]
    free(yn); free(mw); free(VEC);
    *o_nm = nm;
    return out;
}

static float pv_gelu(float x) { // ggml_gelu tanh approximation
    return 0.5f * x * (1.0f + tanhf(0.79788456080286535588f * x * (1.0f + 0.044715f * x * x)));
}
static void pv_add_bias(float *x, const float *b, int outd, int n_pos) {
    for (int p = 0; p < n_pos; p++) { float *c = x + (size_t)outd * p; for (int o = 0; o < outd; o++) c[o] += b[o]; }
}

// ── Stage 5: LlavaMultiModalProjector (mm_1 -> GELU -> mm_2), always GELU ──
// merged [NE, nm] -> proj [5120, nm].
float *pv_projector(const float *merged, int NE, int nm, gguf_file *gf, int *o_dim) {
    int i1 = gguf_find_tensor(gf, "mm.1.weight"), i2 = gguf_find_tensor(gf, "mm.2.weight");
    if (i1 < 0 || i2 < 0) { fprintf(stderr, "pv: mm.1/mm.2 missing\n"); return NULL; }
    int hid = (int)gf->tensors[i1].shape[1];   // mm.1 out dim
    int dim = (int)gf->tensors[i2].shape[1];   // mm.2 out dim = 5120
    float *w1 = gguf_dequant(gf, i1), *w2 = gguf_dequant(gf, i2);
    float *h = (float *)malloc((size_t)hid * nm * 4);
    pv_matmul(w1, merged, h, NE, hid, nm);     // mm.1
    int bi1 = gguf_find_tensor(gf, "mm.1.bias");
    if (bi1 >= 0) { float *b = gguf_dequant(gf, bi1); pv_add_bias(h, b, hid, nm); free(b); }
    for (size_t i = 0; i < (size_t)hid * nm; i++) h[i] = pv_gelu(h[i]);
    float *out = (float *)malloc((size_t)dim * nm * 4);
    pv_matmul(w2, h, out, hid, dim, nm);       // mm.2
    int bi2 = gguf_find_tensor(gf, "mm.2.bias");
    if (bi2 >= 0) { float *b = gguf_dequant(gf, bi2); pv_add_bias(out, b, dim, nm); free(b); }
    free(w1); free(w2); free(h);
    *o_dim = dim;
    return out;
}

// ── Stage 6: [IMG_BREAK] arrangement (POST-projector, dim 5120) ──
// proj [D, nm] (col = px_m + OWm*py_m) -> stream [D, OWm*OHm + OHm-1]; img_break after each row except last.
float *pv_img_break(const float *proj, int D, int OWm, int OHm, gguf_file *gf, int *o_ntok) {
    float *ib = pv_dq(gf, "v.token_embd.img_break");  // [D]
    if (!ib) return NULL;
    int ntok = OWm * OHm + OHm - 1;
    float *out = (float *)malloc((size_t)D * ntok * 4);
    int k = 0;
    for (int y = 0; y < OHm; y++) {
        for (int x = 0; x < OWm; x++) { memcpy(out + (size_t)D * k, proj + (size_t)D * (x + OWm * y), (size_t)D * 4); k++; }
        if (y < OHm - 1) { memcpy(out + (size_t)D * k, ib, (size_t)D * 4); k++; }
    }
    free(ib);
    *o_ntok = ntok;
    return out;
}

// ── Public entry: full pipeline image path -> image embeddings [n_embd_text, n_tok] ──
// Returns malloc'd [dim * ntok] f32 (token-major: out[e + dim*k] = token k, embd e).
// Caller frees. *o_ntok = number of image tokens (incl IMG_BREAK rows), *o_dim = 5120.
float *pv_encode_image(const char *img_path, const char *mmproj_path, int *o_ntok, int *o_dim) {
    int nx, ny;
    float *inp = pv_preprocess(img_path, &nx, &ny);
    if (!inp) return NULL;
    gguf_file *gf = gguf_open(mmproj_path);
    if (!gf) { fprintf(stderr, "pv: cannot open mmproj %s\n", mmproj_path); free(inp); return NULL; }
    int npos, nembd;
    float *cur = pv_patch_embed(inp, nx, ny, gf, &npos, &nembd);
    free(inp);
    if (!cur) { gguf_close(gf); return NULL; }
    int npx = nx / PV_PATCH, npy = ny / PV_PATCH;
    int *ph = (int *)malloc((size_t)npos * 4), *pw = (int *)malloc((size_t)npos * 4);
    for (int i = 0; i < npos; i++) { ph[i] = i / npx; pw[i] = i % npx; }
    cur = pv_vit_trunk(cur, nembd, npos, gf, ph, pw);
    free(ph); free(pw);
    int nm;  float *mg = pv_merger(cur, nembd, npx, npy, gf, &nm);  free(cur);
    int dim; float *pj = pv_projector(mg, nembd, nm, gf, &dim);     free(mg);
    int ntok; float *fin = pv_img_break(pj, dim, npx / 2, npy / 2, gf, &ntok); free(pj);
    gguf_close(gf);
    *o_ntok = ntok; *o_dim = dim;
    return fin;
}

#ifdef PV_TEST
static float *pv_load_bin(const char *path, long *n) {
    FILE *f = fopen(path, "rb");
    if (!f) { fprintf(stderr, "cannot open %s\n", path); return NULL; }
    fseek(f, 0, SEEK_END); *n = ftell(f) / (long)sizeof(float); fseek(f, 0, SEEK_SET);
    float *b = (float *)malloc((size_t)(*n) * sizeof(float));
    if (fread(b, sizeof(float), *n, f) != (size_t)(*n)) { fclose(f); free(b); return NULL; }
    fclose(f);
    return b;
}
// returns 1 if rel error < rel_tol and no NaN.
static int pv_compare(const char *label, const float *mine, const float *ref, long n, double rel_tol) {
    double maxabs = 0, sum = 0, refmax = 0; int nan = 0; long worst = 0, n_over1 = 0;
    for (long i = 0; i < n; i++) {
        if (isnan(mine[i])) nan++;
        double d = fabs((double)mine[i] - (double)ref[i]);
        if (d > maxabs) { maxabs = d; worst = i; }
        if (d > 1.0) n_over1++;
        sum += d;
        double a = fabs((double)ref[i]); if (a > refmax) refmax = a;
    }
    double rel = maxabs / (refmax + 1e-9);
    printf("compare %s: max_abs_diff=%.6g  mean_abs=%.6g  ref_absmax=%.4g  rel=%.4g  NaN=%d\n",
           label, maxabs, sum / n, refmax, rel, nan);
    printf("  diag: |diff|>1.0: %ld of %ld (%.4f%%)  worst@ref=%.4g mine=%.4g\n",
           n_over1, n, 100.0 * n_over1 / n, ref[worst], mine[worst]);
    return (rel < rel_tol && nan == 0);
}

// pv_test <stage 1..7> <image> <mmproj.gguf|-> <groundtruth.bin> [input.bin for isolated 4/5/6]
//   1 preproc  2 conv  3 trunk  4 merger(in=trunk)  5 projector(in=merger)
//   6 img_break(in=proj)  7 full end-to-end -> final
int main(int argc, char **argv) {
    if (argc < 5) { fprintf(stderr, "usage: %s <stage> <image> <gguf|-> <gt.bin> [input.bin]\n", argv[0]); return 1; }
    int stage = atoi(argv[1]);
    int nx, ny;
    float *inp = pv_preprocess(argv[2], &nx, &ny);
    if (!inp) return 1;
    printf("preprocessed %s -> %dx%d\n", argv[2], nx, ny);
    int npx = nx / PV_PATCH, npy = ny / PV_PATCH, npos = npx * npy;
    int OWm = npx / 2, OHm = npy / 2, nm = OWm * OHm;
    long rn; float *ref = pv_load_bin(argv[4], &rn);
    if (!ref) return 1;
    int ok = 0;

    if (stage == 1) {
        ok = pv_compare("inp_raw", inp, ref, (long)nx * ny * 3, 1e-4);
        printf("STAGE%d %s\n", stage, ok ? "PASS" : "FAIL");
        return ok ? 0 : 3;
    }

    gguf_file *gf = gguf_open(argv[3]);
    if (!gf) { fprintf(stderr, "cannot open gguf %s\n", argv[3]); return 1; }
    const int NE = 1024, DIM = 5120;

    if (stage == 2 || stage == 3 || stage == 7) {
        int nembd, np2;
        float *cur = pv_patch_embed(inp, nx, ny, gf, &np2, &nembd);
        if (stage == 2) {
            ok = pv_compare("conv", cur, ref, (long)nembd * np2, 1e-2);
        } else {
            int *ph = (int *)malloc((size_t)np2 * 4), *pw = (int *)malloc((size_t)np2 * 4);
            for (int i = 0; i < np2; i++) { ph[i] = i / npx; pw[i] = i % npx; }
            cur = pv_vit_trunk(cur, nembd, np2, gf, ph, pw);   // in place
            free(ph); free(pw);
            if (stage == 3) {
                ok = pv_compare("trunk", cur, ref, (long)nembd * np2, 5e-2);
            } else { // stage 7 end-to-end
                int nmm; float *mg = pv_merger(cur, nembd, npx, npy, gf, &nmm);
                int dim;  float *pj = pv_projector(mg, nembd, nmm, gf, &dim);
                int ntok; float *fin = pv_img_break(pj, dim, OWm, OHm, gf, &ntok);
                printf("end-to-end -> [%d, %d]\n", dim, ntok);
                ok = pv_compare("final", fin, ref, (long)dim * ntok, 5e-2);
                free(mg); free(pj); free(fin);
            }
        }
        free(cur);
    } else { // isolated stages need input.bin (clip dump of previous stage)
        if (argc < 6) { fprintf(stderr, "stage %d needs input.bin\n", stage); return 1; }
        long in_n; float *input = pv_load_bin(argv[5], &in_n);
        if (!input) return 1;
        if (stage == 4) {
            int nmm; float *mg = pv_merger(input, NE, npx, npy, gf, &nmm);
            printf("merger -> [%d, %d]\n", NE, nmm);
            ok = pv_compare("merger", mg, ref, (long)NE * nmm, 2e-2); free(mg);
        } else if (stage == 5) {
            int dim; float *pj = pv_projector(input, NE, nm, gf, &dim);
            printf("projector -> [%d, %d]\n", dim, nm);
            ok = pv_compare("proj", pj, ref, (long)dim * nm, 2e-2); free(pj);
        } else if (stage == 6) {
            int ntok; float *fin = pv_img_break(input, DIM, OWm, OHm, gf, &ntok);
            printf("img_break -> [%d, %d]\n", DIM, ntok);
            ok = pv_compare("final", fin, ref, (long)DIM * ntok, 1e-3); free(fin);
        }
        free(input);
    }
    gguf_close(gf);
    printf("STAGE%d %s\n", stage, ok ? "PASS" : "FAIL");
    free(inp); free(ref);
    return ok ? 0 : 3;
}
#endif
