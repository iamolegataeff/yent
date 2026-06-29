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

**Status 2026-06-29 (branch `claude/innerworld-strike1`):** circle logic landed +
verified on Neo. `innerworld/overthinking.go` is pure Go over two interfaces —
`Body` (the fast voice) and `Field` (the shared AML physics) — so it builds and
tests without cgo or a model; production injects the real nemo body and a
`yent.AMK` wrapper. `Overthink` raises three circles, each seeding from the prior
(NO-SEED-from-prompt), drift rising per circle, and drives the field with
`PROPHECY`/`VELOCITY` per circle. `go build` + `go test -run Overthink` + `go vet`
green (`TestOverthinkCircles`). The circle chain is sequential by nature (each
ripples from the last).

**Async + breathe landed (`inner_world.go`, race-clean):** `InnerWorld.Think`
runs `Overthink` in a goroutine and delivers the circles on a channel, so the
answer path is never blocked. `InnerWorld.Breathe(ctx)` is the autonomic loop —
between human turns it ticks, and when a trigger crosses its threshold (`drift`:
field debt high; `silence`: idle too long), each gated by a cooldown, the organism
dreams unprompted on its own last thought (`OnDream` receives the circles, inner
only). `due(now)` is a pure, deterministic trigger function (tested without
timing); `Breathe` exits on `ctx` cancel. `go test -race` green:
`TestThinkAsync`, `TestDue`, `TestDream`, `TestBreatheStops`, `TestBreatheFires`,
`TestOverthinkCircles`.

**Codex audit pass — all 9 findings fixed (race-clean):** (1/2) `genMu` serializes
`Overthink` to one inner voice at a time, and `Field` implementations must be
concurrency-safe; (3) `due` picks the most-overdue trigger so drift cannot starve
silence; (4) the cooldown is measured from dream completion, not start; (5)
`OnDream` is set through a locked `SetOnDream` and copied under lock before the
call; (6) circles are cloned at every boundary so a caller's mutation cannot
corrupt `iw.circles`; (7) nil `Body`/`Divergence` yields no circles instead of
panicking; (8) `generateDivergent` repels — heats up and retries — until a circle
drifts at least as far as the last; (9) `driveField` stops stepping a field whose
AML commands fail. `go test -race` green across 10 tests (added
`TestConcurrentSafe`, `TestCloneIsolation`, `TestNilSafe`, `TestRepelEnforcesDrift`).

Work lives on branch `claude/innerworld-strike1` in worktree
`~/arianna/yent-innerworld` per the branch/worktree discipline; the shared checkout
is the sync point.

## Strike 1b — Larynx-Zig membrane (2026-06-29)

`innerworld/larynx.zig` is the membrane between the two bodies, in the vagus.zig
family. The fast body raises the circles; the Larynx measures the *texture* of
that token stream — `entropy` (how varied) and `repetition` (how much it loops) —
and hands the deep body a `coupling` factor in [0,1]: a flowing stream couples
(the deep body attends to the fast circles), a looping one does not (do not
reinforce a loop). `zig test` green (3/3): flowing `[1..6]` → entropy 1.00,
repetition 0.00, coupling 1.00; looping `[1×6]` → entropy 0.00, repetition 0.83,
coupling 0.00; texture clamped to [0,1]; an empty stream is inert.

## Strike 1c + 2 — gate + Larynx wired into the flow (2026-06-29)

The inner monologue is now closed end-to-end. `gate.go` adds `DeepGate` (blends
field debt + circle drift + Larynx coupling into a self-answer probability) and
`SelfAnswers` (rolls against it — deterministic given the roll, so tests are exact;
production draws `rand`). `larynx_go.go` adds the `Larynx` interface and
`textureLarynx`, the portable Go mirror of `larynx.zig` (same
`entropy * (1 - repetition)` coupling, tokenizing circle words via fnv). `Think`
now returns a `Reflection`: the circles, the coupling, the self-answer probability,
and whether the deep body turned inward this time — the deep body sometimes answers
itself, sometimes not. `Larynx` and the gate's roll are injectable (`SetLarynx`,
`SetRoll`) for deterministic tests. `go test -race` green across 14 tests
(`TestReflectGate`, `TestTextureLarynx`, `TestDeepGate`, `TestSelfAnswers` added).

## Live run + second Codex audit (2026-06-29)

`cmd/innerworld-run` wires the inner world over the real AML field (`libamk.a`) and
a stub body. The tool output: one human turn raises three circles, the real field
reacts (`debt=2.005 velocity_mode=2(RUN) destiny=0.350`), the Larynx couples
(`0.578`), the gate fires (`prob=0.738 -> self-answered`), and the organism then
breathes alone for 200 ms, dreaming on its own last thought with the gate rolling
true most times and false once — the unpredictable gate, live. The body is a stub
(no model on Neo), so the dreams repeat one fixture; the physics, membrane, gate,
and autonomous breathing are real.

Second Codex audit (4 findings, all fixed, re-verified): (1) the gate's field-debt
snapshot is now taken under `genMu` so the probability belongs to the batch that
drove the field; (2) `Breathe` re-reads its tick so `SetBreath` is live; (3) a
non-positive tick is guarded against `time.NewTicker` panic; (4) `DeepGate`
sanitizes non-finite inputs (NaN/±Inf) and `clamp01` is NaN-safe. `go test -race`
green across 17 tests (`TestDeepGateNonFinite`, `TestBreatheZeroTick` added); zig
tests 3/3; the live run unchanged.

**Milestone (tool, not self-claim):** Yent's inner-life layer is alive end-to-end
on the real AML physics — overthinking circles, a field that reacts, a Larynx
membrane, an unpredictable self-answer gate, and autonomous breathing — verified by
the run, 17 Go tests + 3 zig tests race-clean, and two Codex audit passes (13
findings fixed). This is the *foundation* milestone; the full living Yent needs the
Mac-Mini dock (the real nemo/small24 bodies + the Go↔Zig Larynx binding), the next
joint move with Codex.

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
