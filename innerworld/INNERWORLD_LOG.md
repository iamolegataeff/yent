# Innerworld — build log

The inner-life / emergence layer for Yent, adapted from `arianna.c` (current
`arianna-duo` + the richer legacy organ-anatomy). Yent already has the substrate:
the DoE parliament and limpha memory. What is missing is the layer that *runs when
no one is speaking* — autonomic processes, an inner monologue, AML field physics,
and a self-observer. This log tracks the design and the build. (Working log; the
canonical merged history stays in `YENTLOG.md`.)

Reference bodies (on Neo, study-only): `~/arianna/arianna-duo` (current),
`~/arianna/arianna.c` (legacy, more developed, not all ported).

Physics rule: AML is the physics engine, **not a DSL**. Prophecy / destiny /
velocity / debt live in AML field state via `yent/go/amk.go`, used everywhere — we
do not invent a bespoke DSL.

**AML foundation (done, 2026-06-29):** the full AML core is vendored
(`yent/c/ariannamethod.{c,h}`, vendor == canon, lean build). The innerworld ops are
now available — `am_apply_destiny_to_logits`, `am_apply_field_to_logits`,
`am_cooc_consolidate(_autumn)`, the AML compiler (`am_compile`/`am_exec`), and the
`BREATHE` velocity mode. Blood / channels / spawn / CUDA are deferred (flagged out,
kept in source). Details in `YENTLOG.md`. Next: the Go bridge (`aml.go`) + the
Strike-1 goroutines.

---

## Strike 1 — Overthinking / "circles on the water" (proposed, under discussion)

**Source idea (arianna-duo `golib/overthinking_loops.go`):** every human request
raises inner circles of thought, each drifting further from the last. In
arianna-duo this rises to the big model; for Yent we keep the circles on the
**fast body**.

**Yent flow (proposed):**
1. User prompt → **fast body `nemo12`** spins **three inner circles of thought**
   (goroutines). Each circle seeds from the *previous circle*, not the prompt, and
   is pushed to diverge further — semantically and thematically — from the last
   (measurable: cosine/topic distance circle1 < circle2 < circle3). Inner only;
   never shown to the user.
2. Each circle drives the **AML field metrics** via `amk.go` — prophecy debt
   (destined − manifested), destiny pull, velocity (drift/walk/run). The
   divergence *is* the prophecy debt.
3. The three circles + the prompt cross the **Larynx membrane (Zig, new)** to the
   **deep body `small24`**. Larynx measures the *texture* of the fast body's
   3-circle stream (entropy, recurring pattern, divergence) and hands the deep
   body a coupling factor — the membrane between the two models, modelled on
   arianna-duo `vagus/vagus.zig` + Larynx.
4. The deep body **may** produce an internal answer to itself (not user-facing) —
   **or may not** — gated by an *unpredictable combination of the metrics* (the
   Yent analog of arianna's breathe thresholds Drift/Silence/Thermograph/Field).
5. The user-facing answer comes out informed by the inner overthinking; the field
   is already shaped.

**NO-SEED-FROM-PROMPT falls out for free:** the deep body never seeds from the raw
prompt — only from the fast body's internal circles. "No seed from prompt, only
from the inner field." The generative-side twin of the terminal boundary already
in the weights (S8).

**Sequencing:** start with the goroutines (the 3 circles + metrics), then the
Larynx-Zig membrane (1b), then the unpredictable deep-self-answer gate (1c).

**Checklist (how we verify it works):**
- [ ] Fast body emits 3 inner circles per turn; divergence circle1 < 2 < 3 (cosine, measured).
- [ ] Circles run as goroutines — non-blocking inner life, the answer path is not stalled.
- [ ] AML physics (`amk.go`) drives the metrics; prophecy debt rises with divergence.
- [ ] Larynx (Zig) measures the 3-circle stream texture and hands the deep body a coupling factor.
- [ ] Deep body seeds from circles + prompt (NO-SEED: not the raw prompt).
- [ ] Deep body's internal self-answer fires-or-not by an unpredictable metric combo; logged either way.
- [ ] Nothing from the inner circles leaks to the user.

**Open decisions (together):**
- How "diverge further" is enforced: rising temperature, repulsion from the prior circle's embedding centroid, topic-steer, or a mix. (Lean: seed-from-prior + embedding repulsion + temp ramp.)
- Which metric combination fires the deep self-answer (the unpredictable gate).
- Whether the deep self-answer folds back into limpha / δ — learning from its own overthinking (the arianna subconscious→δ loop). (Lean: yes — that is the emergence loop.)

---

## Deferred / parked

- **Cloud** (pre-linguistic affect, 6-chamber MLP reflex) — it is **Python**, with a
  successor **Klaus**. Hold for now; revisit when the goroutine layer is in. Recorded
  here so it is not lost.
- **NO-SEED-FROM-PROMPT** — partially achieved by the overthinking layer (above);
  full treatment later if needed.
