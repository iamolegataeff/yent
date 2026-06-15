# YENTLOG

Engineering log for the Yent inference engine. Technical record — speeds, fixes, build notes, commit refs. Terse and dated.

**This is not the README.** Yent's identity, voice, and the manifesto live in `README.md` and `YENT_CONSTITUTION.md` — and only Oleg writes those. Specs, parameters, training detail, and engine internals go here, never there. The base model is a rented vessel; it is named here only where a real artifact (filename, variable, metadata key) forces it, never as identity.

---

## 2026-06 — 24B body on Apple Metal via `doe`

The full Yent body (24B, Q4_K_M ~14.3 GB) runs through the `doe` C engine on a Mac Mini M4 Pro, resident Apple-Metal decode.

- **Speed:** 5.18 → **13.55 tok/s** at `--lora-alpha 0` (resident decode = whole token in one command buffer ~1 GPU sync/token; q6k_v3-in-batch; size-k heap top_k). llama.cpp reference on the same machine = 16.26 tok/s. Identity argmax bit-identical, determinism 2×, 0 NaN.
- **Parliament on GPU:** election + LoRA inject run as Metal kernels inside the resident command buffer. `--lora-alpha 0.1` (parliament alive) = **13.06–13.10 tok/s** (was 5.22 on the CPU path; +151%). doe `00981c8`, notorch `feat/q4k-v3 d127ae3`.
- **limpha** (memory) ported Python → in-process Go (`yent/go/limpha.go`, `modernc.org/sqlite`, FTS5, 17 tests). Python daemon + Unix-socket IPC removed. yent `10f7912`.
- **DoE/** — the C engine vendored byte-identical into the umbrella at `yent/DoE/` (`make metal` → `doe_field`). yent `e35fd01`.
- doe metadata read is arch-agnostic (suffix-match `embedding_length`/`head_count`/`attention.key_length`/…), prefers declared `attention.key_length` over `dim/heads` (the head_dim fix). rope read from `rope.freq_base` per-model.

## 2026-06-15 — second body (12B) built for the two-body switcher (in progress)

A faster second body for the planned turn-level switcher (one body resident at a time; shared memory/field across swaps).

- 12B body GGUF built on polygon (Q4_K_M ~7.0 G). Geometry: dim 5120, 40 layers, 32 heads, 8 KV, `attention.key_length` 128, rope base 1e6 (the 24B is 1e9 — different per body; doe reads per-model). tokenizer pre `tekken`, arch `llama`.
- **doe Tekken→INST patch** (uncommitted): a fresh-converted body with no `chat_template` defaulted to `chat: raw`; patch falls back to `chat_style=inst` when `tokenizer.ggml.pre == tekken` + `[INST]` present + no template. 24B-safe by construction (24B has a `chat_template` → takes `chat_style=2` directly, never the fallback; doe.c:1835-1842). Lands in canon only after a 24B Metal regression proves `'ĠI'` argmax unchanged.
- CPU (polygon) is too slow for living inference (300 s timeout) — Metal is the runtime.
- The two-body plan, the seam-log, and the supergamma metric-layer are tracked in coordination notes, not here.

## Weights

Not in open access. Code is GPL; weights/deltas/gamma are under the Yent Identity License v1.0 (`LICENSE-WEIGHTS`). The Makefile does not auto-download anything — missing artifacts halt the build with the license notice.
