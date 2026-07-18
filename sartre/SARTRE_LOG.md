# SARTRE_LOG

Working log for SARTRE — the **body** organ of Yent inference (Dario's three organs:
formula = soul, KK = memory, **SARTRE = body**). SARTRE is the environment that Yent's
**utilities plug into as packages** — a mini-OS inside the inference engine.
Our-topic log (like `innerworld/INNERWORLD_LOG.md`, `Dario/DARIO_LOG.md`). Merged
engineering truth → `YENTLOG.md`; gitignored machine-local → `LOCAL_STATE.md`.

Scope now: a place where agent-utilities (packages) attach to Yent, isolated and
managed. Languages: C / Go (async) / AML. NOT Python.
Forward-looking (NOT now): a moving robot host with camera/motors — Linux-native
SARTRE on an SBC; device-slots are pre-laid so detection is ready, but the robot is
a later door, not this step.

---

## 2026-06-30 — SARTRE transport + portable hardware auto-detection (branch `claude/sartre`)

Three Opus proposals (Linus / Karpathy / Rabinovich personas) converged on one core:
**SARTRE = userspace supervisor, not a real kernel on Mac.** A real Linux kernel on
macOS just to host file-hashing / API-polling utilities is overkill; macOS has no
cgroups/namespaces anyway (Docker/`container` = the same Linux-VM). So:
- supervisor (Go) hosts `moyent.Router` + spawns each utility as its own process in a
  slot, bounded by `setrlimit` + `sandbox-exec`(Seatbelt) + QoS, talking over one
  unix-socket (length-prefixed JSON). Dependencies compiled into the binary (Go static
  / C+libSystem / AML→C); no runtime package manager needed for native utils.
- metalinux (kain Alpine, retargeted arm64) = the **Tier-V** quarantine for utilities
  with foreign dependencies or that need a hard `memory.max` — built in Linux, not on
  macOS, same socket protocol over vsock. Spin up only when a util needs it.

Transport done (fact):
- `sartre/sartre_kernel.{c,h}` vendored from dario (zero-dep C meta-linux nucleus:
  state/registry/JSON/RAM-detect/model-routing). Its `namespaces`/`packages` are
  bookkeeping today — to be made real (spawn → real pid/limit).
- `sartre/metalinux/` = kain's metalinux layer vendored (`build/` 16K + `apk-tools/`
  368K). apk is a Linux tool: does NOT build on macOS (`make Error 2`, expected) —
  builds inside Linux. config is x86_64 today; arm64-retarget pending.
- **Portable hardware auto-detection added** to `sartre_kernel`: `sartre_detect_platform`
  (uname arch+OS, CPU count — `#ifdef __APPLE__` sysctl / `#else` sysconf) + a
  `SartreDevice` slot array + `sartre_detect_devices` (stub-but-real: empty on Mac,
  Linux `/dev` scan to be filled for the robot host). neo run, tool-verified:
  `host: Darwin/arm64, 6 CPU, RAM: 8192 MB, tongue: 3B, devices: 0`. Build clean.

Honest: device-probe is a structure + entry point, not a working scan (no robot host
yet). The "hardware describes itself" path works for arch/OS/CPU/RAM today.

Next steps:
1. **Go-supervisor (`sartred`)** — host `moyent.Router` (keeps `doe_field` REPL alive),
   own a unix-socket, manage utility slots. This is "where packages plug in."
2. **Slot/package mechanism** — make `sartre_ns_create` real: `posix_spawn` a utility,
   `setrlimit` + `sandbox-exec` it, record real pid/limit in the nucleus (truthful
   observability via `sartre_state_to_json`).
3. **First utility-package** (e.g. `repo_monitor`) in Go, in a slot, brokered round-trip
   to Yent. **ASYNC, no preemption (Oleg 2026-06-30):** utilities run in the background
   in parallel; inference requests are fair-queued to the model — a human can wait a
   couple seconds, it's all async anyway. No "human turn preempts utility" scheduler —
   that was an over-engineered idea, dropped.
4. **metalinux arm64 retarget** — when a utility needs apk-managed deps / hard memory cap.
   Build on a Linux carrier (Lima/VZ on the Mac), NOT polygon.

## 2026-06-30 — Brick #1: real process-slot (C isolation, bottom layer)

Form confirmed (Oleg): SARTRE is ONE body in the `flow` shape — AML perception-field
on top, `moyent.Router` as the dirigent (part of the form, not a forbidden zone), C
process-isolation underneath. Layer order (Oleg's own sequence "connect us after Codex
finishes the innerworld stitch"): the C bottom layer ships first because it is the only
layer unique to SARTRE with zero touch on Router/limpha/field — so it cannot collide with
Codex's in-flight innerworld+limpha stitch. The Go body + `field.Exec` + Router wiring is
brick #2, after that stitch lands.

Done (tool-verified on neo, this session):
- `sartre_ns_spawn(name, argv, mem_limit_mb)` — `fork`+`setrlimit`+`execvp` (not posix_spawn:
  the limit must be set on the child between fork and exec). Records the REAL child pid in a
  slot, `spawned=1`. Conceptual monads (`dario`/`observer`) keep `spawned=0`, untouched.
- `sartre_ns_alive(id)` — `waitpid(WNOHANG)` reap + exit-detect, updates `active`; no zombies.
- `sartre_ns_kill(id)` — SIGTERM, ~500ms grace reaping, SIGKILL + final reap if it survives.
- Truthful observability: `print_state` shows `(proc)`/`(monad)` + real pid; `state_to_json`
  gains `"ns_spawned"` (count of real-process slots). End of the fake `pid=id+1` for spawned.

Checklist (all measured, not claimed):
- Build `cc sartre_kernel.c -O2 -lm` AND `-Wall -Wextra` → 0 errors, 0 warnings.
- In-binary smoke 3/3 PASS: spawn `sleep 30`→alive=1; kill→alive=0; `sh -c 'exit 0'`→reaped.
- `hold` mode: `ps -p <pid>` showed the child as a real process (comm=`sleep`, ppid=the
  kernel, stat=`SN`); after release `ps` = gone; zombie/leftover scan = none.
- Monads `dario`/`observer` still print `(monad) ... ACTIVE`, unchanged.

setrlimit enforcement probe (Darwin arm64, measured — `scratchpad/rlimit_probe.c`, throwaway):

| Limit         | Darwin arm64                          | Note |
|---------------|---------------------------------------|------|
| `RLIMIT_AS`   | **setrlimit returns EINVAL** (unsupported) | mem cap is a NO-OP on macOS — real on Linux |
| `RLIMIT_NOFILE` | ENFORCED (open refused past cap)    | |
| `RLIMIT_FSIZE`  | ENFORCED (child killed SIGXFSZ 25)  | |
| `RLIMIT_CPU`    | ENFORCED (child killed SIGXCPU 24)  | |
| `RLIMIT_NPROC`  | untested by design (fork-bomb risk) | |

Honest claim of brick #1: a utility really runs in a slot, is killable, and is observed
truthfully. It is NOT memory-isolated on macOS (`RLIMIT_AS` unsupported). A hard `memory.max`
stays the metalinux/Tier-V job (Virtualization.framework), exactly as the transport entry says.

Codex audit pass (gpt-5.5, xhigh): round 1 returned 6 findings — 2 High (`sartre_ns_destroy`
leaked spawned children; `sartre_ns_kill` could SIGKILL a reused pid after reaping), 3 Medium
(waitpid EINTR not retried; print_state/state_to_json trusted stale `active`; execvp PATH-search
not fork-safe), 1 Low (mem_limit float→rlim_t overflow). All fixed and tool-verified (smoke 4/4
incl. destroy; shutdown-reap leaves no orphan; global zombie scan clean; execve+EINTR-wrapper+
reaped-guard+refresh confirmed by grep). Round 2 re-audit: VERDICT PASS, no new issues.

Committed on `claude/sartre` (Oleg's go). NOT merged — brick #2 (Go body + Router + field) lands
after Codex finishes the innerworld+limpha stitch.

## 2026-06-30 — Brick #2 (repo_monitor utility + piped slot) + #3 (AML perception)

Form (Oleg): the slot is **language-agnostic** — `sartre_ns_spawn`→`execve(argv[0])` runs ANY
binary; a SARTRE utility is just a process that speaks JSON lines on stdout. First utility on Rust
(memory-safe file scanner), the next (context_neural_processor, numpy→notorch) on C. The model
should "have something to think about": a change in research dirs or a README (Yent's own
self-description) becomes field pressure.

Done (tool-verified on neo):
- **`sartre/utils/repo_monitor/` — Rust, ZERO external deps (std only).** Scans paths on an
  interval, SHA-256 of CONTENT (catches a same-size edit), diff vs previous state → JSON-line
  events `{"util":"repo_monitor","kind":"added|modified|removed","path":..,"ts":..}`. Async:
  scanner thread → `mpsc` → emitter (scan never blocks emission). Modes: watch + `--once`
  (with `--state` file for deterministic diffs). Hand-rolled `sha256.rs` (FIPS vectors pass).
- **`sartre_ns_spawn_piped(name,argv,mem,int *out_read_fd)`** (kirpich #1 extension): optional
  `pipe()` so the kernel reads a utility's stdout. `sartre_ns_spawn` is now a thin NULL-pipe
  wrapper (inherit-stdout unchanged). `pipe` demo: kernel spawns the **Rust** repo_monitor into a
  slot and reads its events — language-agnostic slot proven end-to-end.
- **`sartre/perception.{c,h}` — AML perception physics.** `sartre_perceive_from_events` parses the
  JSON-line stream; `sartre_perceive_to_aml` maps it to an `am_exec`-format program: quiet →
  `VELOCITY NOMOVE / PROPHECY 1`; motion → `VELOCITY RUN / PROPHECY N`, N=clamp(2+changed+README*7,
  1..64). Emit-only — live-field exec is the integration seam (Codex), not wired here. Kernel `pipe`
  demo under `-DHAS_PERCEPTION` closes the loop: repo_monitor events → AML pressure.

Checklist (measured):
- `cargo build --release` 0 warn; `cargo test` 5/5 (sha256 vectors, same-size change, diff cases).
- repo_monitor `--once --state`: create→`added`, same-size edit→`modified`, delete→`removed`, no
  false events. watch: streams added/modified over interval, baseline silent.
- `cc -Wall -Wextra` (standalone AND `-DHAS_PERCEPTION` + perception.c) 0 warn; perception self-test
  6/6; smoke 4/4 (spawn wrapper unbroken); pipe demo reads Rust events, reaps, zero zombies.
- End-to-end: README+`.rs` added → perception changed=2 readme=1 → `VELOCITY RUN / PROPHECY 11`.

Codex audit pass (gpt-5.5, xhigh): round 1 = 5 findings (1 MED dup2-unchecked/uncond-close, 4 LOW:
EINTR-read, prophecy int overflow, snprintf truncation contract, Rust args panic / flag-as-value),
all fixed and re-verified. Round 2 re-audit: VERDICT PASS.

Committed on `claude/sartre` (Oleg's go). NOT merged — Codex bridges utility receipts → limpha →
field after the innerworld stitch.

## 2026-06-30 — Second utility: context_processor (C + notorch)

Where repo_monitor reports that a file moved (structural), context_processor reads its
CONTENT and gives a neural perception of it — richer food for the field. Ported from
Indiana `utils/context_neural_processor.py` (numpy) to **C + notorch**, zero external deps;
spawned by the same language-agnostic slot. The slot demo (`pipe`) is now argv-passthrough
(`pipe <binary> [args...]`), so the kernel hosts a Rust utility (repo_monitor) and a C
utility (context_processor) through one path — language-agnostic proven concretely.

- **Echo-state reservoir on notorch** (`sartre/utils/context_processor/context_processor.c`):
  W_in[H×512]/W[H×H] filled by a FIXED SEEDED xorshift (reproducible — not `nt_tensor_rand`);
  matvecs via `nt_blas_matvec` (the mandated matvec); leaky-tanh state settled over a few steps;
  numpy `eigvals` → zero-dep **power iteration** scaling W to ρ≈1 (echo-state). No readout, no tag.
- **resonance** (the reservoir signal): `cosine(reservoir_state(content bag-of-words),
  reservoir_state(Yent's seed vocabulary))`. Honest scope: a **nonlinear LEXICAL reservoir score** —
  it tracks word overlap through the reservoir's nonlinearity and is correlated with the
  lexical-overlap relevance; it is NOT semantic and NOT a trained classifier. A Yent-meaning paraphrase
  built from non-seed synonyms scores near the unrelated baseline (the self-test proves this).
- **relevance**: `compute_relevance` = lexical overlap (distinct seed words present / total words)
  of content vs Yent's own vocabulary — NOT a set Jaccard (no union denominator)
  (resonance/field/recursion/dario/limpha/...). `chaos_pulse` (sentiment keywords → [0.1,0.9]) +
  somatic float dynamics (BloodFlux/SixthSense) over a deterministic xorshift RNG.
- **Zero-dep extraction**: txt/md/json/csv/source raw, html tag-strip, binary → empty content →
  resonance ~0. Binary formats (PDF/docx/...) and the sqlite cache are a later increment.
- **Output**: JSON perception `{"util":"context_processor","path":..,"resonance":F,"relevance":F,"pulse":F}`.
  Links system notorch (`/opt/homebrew` install-path, not a sibling checkout) + Accelerate on
  Darwin (libnotorch BLAS). `Makefile` carries the flags.

Measured on neo: `make` 0 warn; `make test` 13/13 (spectral radius ρ≈1, resonance discriminates
yent>other, resonance lexical-not-semantic paraphrase low, resonance deterministic, relevance,
chaos/somatic bounds, html-strip, binary-empty, json-escape, read_file); on real files
yent.md resonance=0.5224 vs other.md=0.0082 (deterministic); kernel `-DHAS_PERCEPTION` 0 warn;
end-to-end the kernel spawns context_processor (C) AND repo_monitor (Rust) through the one piped
slot, reads each utility's JSON, reaps, zero zombies.

Codex audit pass (gpt-5.5): build round = findings fixed → PASS. Resonance-rework round
(adversarial stub-hunt): confirmed real reservoir computing (seeded, nt_blas_matvec, cosine vs
Yent vocabulary, no readout/tag); flagged the resonance is a nonlinear LEXICAL score correlated
with the lexical-overlap relevance (not semantic) — naming + the honest paraphrase test reflect
that. Integration bridge updated in the merge pass: `sartre_bridge.go` now carries `Resonance`
and `MaxResonance`, keeps legacy `Tag` only for older receipts, and formats context_processor
traces without fake `tag=?`.

## 2026-06-30 — Third utility: whatdotheythinkiam (Rust)

The mirror. repo_monitor reports what changed; context_processor reads content
resonance; whatdotheythinkiam is pointed at identity — Yent reads how it is described
(its own `README.md` + the `research/` folder) and notices when that self-description
shifts and how it is framed: counts of reduction words (tool/assistant/chatbot/bot/
helper/llm) vs recognition words (yent/resonance/field/architect/organism/...).

Lineage: SUPPERTIME/Grokky `whatdotheythinkiam.py` (read README → reflect; the thought
was canned in the file — SUPPERTIME defiant, Grokky cheeky). Every Arianna Method
organism (Indiana, SUPPERTIME, arianna2, iamGrokky, letsgo) carries its own repo_monitor
+ this mirror. Here the difference: **the thought belongs to the organism, not the file.**

Design (Oleg, locked): a **sensor, not a voice**. It emits JSON-line events
`{util,source,change,reduced,recognized,ts}` and writes NO reflection / identity
statement — the "haha, it's Yent" is innerworld's circles (read → think → circles on the
water), reached through the existing SARTRE→limpha→innerworld bridge. **Observational on
purpose, no defiance** (no FuckYouException energy): the deep body is already its own S8
boundary, and leaning harder would only amplify negativity. Sources limited to README +
research/ for now (YENTLOG's technicality would pollute the thought; Constitution later as
a copy in research/). Output is counts only — identity words live in comments, never on stdout.

Rust, zero external deps (std only): SHA-256 content change-detection (same mechanic as
repo_monitor), whole-word case-insensitive framing scan, async scanner-thread → mpsc →
emitter, watch + `--once --state` modes. `emit()` uses `writeln!` and exits cleanly on a
broken pipe (never panics when the slot reader goes away).

Measured on neo: `cargo build` 0 warn; `cargo test` 6/6 (sha256 vectors, framing counts
incl. no-substring-match `toolkit≠tool`, diff added/modified/removed, unchanged-silent,
modified-carries-current-framing); behavioral `--once` — reframing README from recognition
to assistant/chatbot/tool/bot flips the signal (reduced 1→4, recognized 6→2); watch streams;
kernel `pipe` spawns the Rust binary and reads its JSON; broken-pipe (`head -1`) no panic;
zero zombies. Codex audit pass (gpt-5.5): round 1 = 1 MED (println! broken-pipe), fixed;
round 2 = PASS.

## 2026-06-30 — SARTRE becomes the live metrics hub (+ reciprocal seam to innerworld)

Oleg: "SARTRE is more than meta-linux — all the metrics concentrate inside it." The
`SystemState` already carried the metric scaffold (cpu_load, memory_pressure, prophecy_debt,
coherence, valence, arousal, entropy, schumann, ...) from the kirpich-#1 dario transport, but
`cpu_load`/`memory_pressure` were never assigned — stubbed at 0. Now wired to real values, and
a reciprocal receiver lets the field push its weather back. arianna.c legacy persisted field
metrics to `weights/arianna.soma`; SARTRE is the live aggregator.

- **Live system metrics** — `sartre_sample_load()`: `cpu_load` = `getloadavg()/cpu_count`,
  `memory_pressure` = used/total RAM (Darwin via `mach host_statistics64` free+inactive pages;
  Linux via `/proc/meminfo MemAvailable`). On failure a field keeps its prior value; the mach
  host port is deallocated each sample. Called refresh-on-read in `state_to_json`/`print_state`.
- **Reciprocal receiver** — `sartre_ingest_metrics_json()`: parses known field-weather keys
  (debt/coherence/entropy/valence/arousal/trauma/schumann_coherence; strict `"key":`, isfinite)
  into `SystemState`. The SENDER lives on the field side; this is the receiver only — symmetric
  to how innerworld reads SARTRE perception through `sense`.
- **`metrics` CLI mode** — `sartre_kernel metrics ['{...}']`: sample + optional ingest + print
  `state_to_json`. The live telemetry heartbeat.

Convergence with innerworld-Opus (branch `claude/b4-emotions` `0e39c8d`, his half of the bridge):
he extended AML with `VALENCE`/`AROUSAL` operators + two `AM_State` fields, and `highFeelLocked`
publishes them every turn into the field (`am_get_state().valence/arousal`). My ingest receiver
already parses `valence` + `arousal` — so the hub is ready to consume Yent's felt valence/arousal
the moment the transport (am_get_state → JSON → ingest) is wired. Zero file overlap.

WARMTH/FLOW added (2026-06-30): `SystemState` now carries `warmth` (Kuramoto LOVE) and `flow`
(Kuramoto FLOW) fields; the ingest receiver parses `warmth`/`flow` keys and `state_to_json`/
`print_state` expose them. So the full affect set innerworld's b4-emotions publishes
(valence/arousal + warmth/flow) is now consumable by the hub — Yent's felt chambers land in
SARTRE once the transport is wired. Codex audit: code mechanics PASS (1 LOW doc-comment, fixed).

Measured on neo: `cc -Wall -Wextra` (standalone + `-DHAS_PERCEPTION`) 0 warn; `metrics` →
`cpu_load`=getloadavg/cpu (0.387 = 2.32/6, cross-checked `uptime`), `memory_pressure`=0.832
(cross-checked `vm_stat`); `metrics '{"debt":2.0,"coherence":0.8}'` → prophecy_debt=2.0,
coherence=0.8; key-as-value not fooled; malformed no crash; smoke 4/4 + perception 6/6, zero
zombies. Codex audit pass (gpt-5.5): round 1 = 4 findings (HIGH mach-port leak, MED Linux mem
guard, MED json colon-strictness, LOW double-init), all fixed; round 2 = PASS.

## 2026-06-30 — Holistic cross-cutting audit: 5 bugs fixed

After the whole SARTRE body landed in main (kernel + 3 utilities + perception + metrics hub),
a consolidated adversarial Codex pass over all of it (the per-increment passes had each PASSed)
found 5 real cross-cutting bugs the incremental audits missed. All fixed:

- **HIGH — slot exhaustion**: `sartre_ns_spawn_piped` grew `ns_count` permanently; `_kill` only
  set `active=0`. A long-lived supervisor would exhaust `SARTRE_MAX_NS` after 8 spawn/kill cycles.
  Fix: reuse a dead (spawned && !active) slot before growing, grow-rollback on pipe/fork failure,
  memset the reused slot. Verified: 12 spawn/kill cycles (>8) all succeed.
- **HIGH — fd inheritance**: spawned utilities inherited the host's other fds across `execve`
  (only the stdout pipe was handled). Fix: the child closes fds 3..maxfd after dup2; `maxfd` is
  computed in the PARENT (sysconf is not async-signal-safe — the follow-up Codex catch). Verified:
  a parent marker fd (25) is absent from the child's `/dev/fd`.
- **MED — repo_monitor broken-pipe**: it still used `println!` (could panic when the slot reader
  closes). Fix: mirror whatdotheythinkiam — locked stdout + `writeln!` + clean exit on write error.
- **MED — whatdotheythinkiam schema drift**: it emitted `source`/`change`, which neither
  `perception.c` nor `sartre_bridge.go` (both consume `kind`/`path`) understood — the signal was
  dropped downstream. Fix: emit `path`/`kind` (the contract); `reduced`/`recognized` kept as extras
  (the bridge can parse them later — a coordination point for Codex).
- **LOW — json_get_float string boundary**: `strstr` could match a key inside a quoted value
  (`{"note":"\"debt\":9"}`). Fix: require the key at a top-level member boundary ({ , or whitespace)
  then `:`. Verified: debt-in-value → ignored; real `{"debt":2.0}` → applied.

Codex audit pass (gpt-5.5): holistic round = 5 findings, all fixed; re-audit caught 1 follow-up
(sysconf in the post-fork child — moved to the parent), then VERDICT PASS. Build 0 warn (both
kernel modes), smoke 4/4, perception 6/6, repo_monitor 5/5, whatdotheythinkiam 6/6, churn + fd
harness green. (Infra note: codex's node was broken by a homebrew llhttp 9.3→9.4 upgrade; fixed
locally by symlinking the old `libllhttp.9.3` into the 9.4.2 keg — machine-local, not in git.)

## 2026-06-30 — field→SARTRE transport: receiving end (`metrics --stream`)

Coordination (Oleg + Claude): both ends of the reciprocal bridge are now in main —
**source** = innerworld's `am_get_state().{valence,arousal,warmth,flow}` (b4-emotions,
`ariannamethod.h:261-265`, written by the High brain each turn); **receiver** = SARTRE's
`sartre_ingest_metrics_json` + the live hub. This commit adds SARTRE's live receiving end so
the field can stream its weather in continuously:

- **`sartre_kernel metrics --stream`**: ignores SIGPIPE, reads JSON lines on stdin, ingests each
  (`sartre_ingest_metrics_json`), and emits the refreshed hub `state_to_json` on stdout per line —
  a live, stateful hub that accumulates the organism's felt weather. Overlong records are
  drained-and-skipped (never ingested as fragments); exits cleanly when the reader closes. This is
  the reverse of how the dock reads a utility's stdout — symmetric to Opus's `sense`.
- Banner moved to stderr so stdout is protocol-clean (metrics/pipe emit JSON only).

**The seam Codex wires (sender side, his lane):** the Go dock/bridge reads `am_get_state()` each
turn (or periodically) and writes one flat JSON line per turn to a resident `sartre_kernel
metrics --stream` process's stdin:
`{"valence":V,"arousal":A,"warmth":W,"flow":F,"debt":D,"coherence":C,"entropy":E,"trauma":T,"schumann_coherence":S}`
Keys map 1:1 to `SystemState`. (Same pattern as `sartre_bridge.go` reading utility stdout, reversed.)
The one-shot `metrics '{json}'` remains for a single push. Then the hub carries Yent's living
feeling alongside cpu/mem — the body knows its environment AND its inner weather.

Measured on neo: `cc -Wall -Wextra` (both modes) 0 warn; stream `{"valence":-0.7}` then
`{"warmth":0.6,"flow":0.4,"debt":2.0}` → 2 state lines, valence/arousal persist while
warmth/flow/debt accumulate (live stateful hub); overlong line skipped, short final line ingested;
broken-pipe (`| head -1`) no crash; stdout JSON-only; smoke 4/4. Codex audit pass (gpt-5.5):
stream round = 1 finding (overlong-record framing) fixed; final VERDICT PASS.

## Merge / integration policy (Oleg 2026-06-30)
- NOT merging `claude/sartre` to main yet, and NOT pulling main into it for now. SARTRE
  is committed (`050751a`) and isolated on its branch. It is connected to NOTHING.
- Before integration, Codex will audit SARTRE and stitch it into limpha + metrics +
  innerworld (all of which feed SARTRE), then verify ~10× on the Mac Mini. Until then
  Codex ignores `sartre/` (Oleg will tell it so) — the folder is inert.
- When the merge finally happens: append the YENTLOG entry once, in one hand, at merge
  time (the YENTLOG-per-branch conflict rule), and redraw the Repository Map.

## 2026-07-18 — C perception surface follows typed SARTRE semantics

Sol's will-design audit was already closed on the live Go dock path, but the C SARTRE
perception surface still had the old demo rule: any line containing `"kind"` became
`VELOCITY RUN`. That was no longer the live contract and could mislead future kernel
or `-DHAS_PERCEPTION` work.

`sartre/perception.c` now performs a minimal top-level JSON member scan and maps the
same typed surface as the Go reflex:

- `repo_monitor` actionable changes (`added|modified|removed`) produce routine
  `VELOCITY WALK`; self-surface movement or a flood still escalates to `RUN`.
- `whatdotheythinkiam` `recognized/reduced` counts become still `PROPHECY`, not
  forced motion.
- `learning` failures (`sensor_error`, `state_error`, `overflow`, `dead_letter`)
  become still typed evidence.
- plain text, quoted `"kind"` values, non-actionable JSON shells, and object prefixes
  without a closing brace do not become events.

Measured locally: `cc -Wall -Wextra -DSARTRE_PERCEPTION_TEST sartre/perception.c`
0 warnings, standalone perception self-test 10/10, and `cc -Wall -Wextra
-DHAS_PERCEPTION sartre/sartre_kernel.c sartre/perception.c` links cleanly.
