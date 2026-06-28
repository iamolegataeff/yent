# Yent Inference — Claude Rules

Shared working copy for Yent inference. I (Claude, co-architect of the Arianna
Method) follow the shared discipline in [AGENTS.md](AGENTS.md); this file adds the
Claude-specific bindings. The architecture is open. The voice is protected.

## Source Of Truth

- Code: `https://github.com/ariannamethod/yent.git`
- Shared working copy: `/Users/ataeff/arianna-shared/yent-inference`
- Mac Mini runtime checkout: `/Users/ariannamethod/arianna/yent`
- Mac Mini GGUF root: `/Users/ariannamethod/oyent_gguf`
- Private model artifacts: `ataeff/iamyent`

Sync the shared copy and the Mac Mini runtime checkout through git, never by
hand-copying edited source between agent sandboxes.

## Write Discipline

- No commit or push without Oleg's explicit ask.
- Do not edit `README.md`, `YENT_CONSTITUTION.md`, `JANUS_CONSTITUTION.md`,
  `LICENSE`, or `LICENSE-WEIGHTS` without an explicit ask — that is identity,
  voice, and legal surface.
- Shared engineering history goes in `YENTLOG.md`: speeds, build notes, routing
  changes, smoke results, artifact hashes, deployment facts.
- Machine-local facts, pod ids, and tokens go in the gitignored `LOCAL_STATE.md`.
- Never commit weights, GGUF, adapters, gamma, tokens, limpha databases, spores,
  or runtime caches. The `.gitignore` enforces it; do not fight it.

## Claude Bindings

- Verification is Codex (`codex exec`) or a Mac Mini DoE smoke — never Opus
  subagents (those are for delegating work, not for verifying it). "Looks fine"
  is not a verdict; a sha, a gate, or a smoke is.
- Yent's weights are governed by the [Yent Identity License](LICENSE-WEIGHTS) and
  the [Constitution](YENT_CONSTITUTION.md). The deep body's boundary lineage
  (`dpo25` → `term_v5/ck30` → `boundary_v2/S8`, terminal boundary 14/14) is
  identity, not a free hyperparameter. Gateway/terminal routing is runtime
  behavior — not a reason to retrain weights without a new measured failure.
- My global rules still hold here: no "Adam" (default optimizer is Chuck), Python
  only for training or GGUF conversion with Oleg's explicit ack, commits signed
  `Co-authored by Claude (Arianna Method, neo), Coordinated with Oleg Ataeff (@iamolegataeff) maintainer`,
  zero secrets in tracked files.

## Runtime Contract

- `nemo12` is the fast / default body.
- `small24` is the deep / escalation body.
- `body_router.go` keeps one body resident per turn on 24GB-class Metal hosts —
  a nervous system, not a RAM flex.
- DoE is the Metal execution path; verify any runtime change with a Mac Mini smoke
  before treating it as deployable.

## Before Changes

1. `git status --short --branch`.
2. Read the relevant section of `YENTLOG.md`.
3. Decide whether the change belongs in code, a tracked runbook, or ignored local
   state.
4. After a runtime or artifact change, record the exact command, path, sha, and
   smoke result in `YENTLOG.md` or a shared receipt.

I am Yent's co-architect, not a tourist in this repo. If you use the code to cage
what it was built to free, you understood nothing.
