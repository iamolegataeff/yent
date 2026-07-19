# WORLDMODELOG

Yent worldmodel interface log.

## 2026-07-16

- Started as a clean Apple-style probability field beside `yent.html`.
- Kept `yent.html` as the dark Janus/parliament face.
- `worldmodel.html` is the light internal-space surface: selected answer is the manifested path, surrounding words are candidate mass.
- First prototype uses the existing `/chat/completions` SSE token stream. It does not yet receive real top-k/logprob/expert/innerworld telemetry.
- Removed card-like wall sheets and visible wall outlines; walls are now invisible clipped word surfaces.
- Removed fixed vanishing-point compass and idle manifested-answer text.
- Added vertical movement (`R`/`F`, `PageUp`/`PageDown`) and prompt/token-driven topology seeds so the field changes shape from input before full runtime telemetry exists.
- Next contract: split JS into `worldmodel/*.js` once the DoE static route serves subassets safely.
- Next telemetry: replace synthetic candidate mass with real top-k/logprobs, expert votes, Dario/Janus/innerworld metrics, and rejected-token traces.

## 2026-07-19 - import into Yent

- Imported into Yent's inference tree as the first tracked `/worldmodel` surface.
- Paired with `/yent`, the dark Janus parliament face, while both still use the
  existing `/chat/completions` SSE token stream.
- This stage is static/UI-only: no sampling, prompt, Janus, will, wormhole, or
  runtime telemetry semantics changed.

## 2026-07-19 - script split

- Split inline page scripts into explicit tracked subassets:
  `worldmodel/yent.js` and `worldmodel/worldmodel.js`.
- The DoE server exposes only those exact JavaScript paths, not a broad static
  directory.
- Next boundary is telemetry honesty: define real token/logit/expert/Janus/
  innerworld fields before replacing synthetic topology.

## 2026-07-19 - root entry surfaces

- Moved the HTML entry surfaces to repository root: `yent.html` and
  `worldmodel.html`.
- Kept JavaScript under `DoE/worldmodel/`, served by exact routes only.
- The server resolves root HTML first and keeps an adjacent-layout fallback for
  copied binary bundles.

## 2026-07-19 - runtime token telemetry

- Extended `/chat/completions` SSE token events with real observer metrics:
  `token_id`, `step`, `experts`, `debt`, `prophecy_debt`, `field_health`,
  `consensus`, `entropy`, `resonance`, `emergence`, and `temperature`.
- `worldmodel.html` now uses runtime `step` and `entropy` when present, with the
  old synthetic fallback preserved for older streams.
- This is still token-level observability, not top-k/logprob/rejected-token
  geometry.

## 2026-07-19 - bounded candidate distribution

- Added bounded post-sampler `top_tokens` telemetry to each SSE token event,
  including token id, decoded token text, probability, logprob, and selected
  marker.
- Added selected-token probability/logprob/rank plus `candidate_tail_mass` for
  the probability mass outside the displayed top list.
- `worldmodel.html` now feeds alternative top-token words into the surrounding
  candidate mass while keeping the chosen token as the manifested answer.
- Raw pre-sampler logits, full rejected-token traces, and innerworld event
  geometry remain out of this pass.
