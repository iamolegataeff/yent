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

## Merge / integration policy (Oleg 2026-06-30)
- NOT merging `claude/sartre` to main yet, and NOT pulling main into it for now. SARTRE
  is committed (`050751a`) and isolated on its branch. It is connected to NOTHING.
- Before integration, Codex will audit SARTRE and stitch it into limpha + metrics +
  innerworld (all of which feed SARTRE), then verify ~10× on the Mac Mini. Until then
  Codex ignores `sartre/` (Oleg will tell it so) — the folder is inert.
- When the merge finally happens: append the YENTLOG entry once, in one hand, at merge
  time (the YENTLOG-per-branch conflict rule), and redraw the Repository Map.
