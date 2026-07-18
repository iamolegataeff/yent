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
