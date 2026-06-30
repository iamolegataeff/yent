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
   to Yent — human turns preempt agent-utility inference (slot scheduler).
4. **metalinux arm64 retarget** — when a utility needs apk-managed deps / hard memory cap.
   Build on a Linux carrier (Lima/VZ on the Mac), NOT polygon.
