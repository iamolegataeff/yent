(function (root) {
  'use strict';

  function clamp(value, min, max) {
    return Math.max(min, Math.min(max, value));
  }

  function mix(a, b, t) {
    return a + (b - a) * t;
  }

  function hash(n) {
    const x = Math.sin(n * 12.9898) * 43758.5453123;
    return x - Math.floor(x);
  }

  function textSeed(text) {
    text = typeof text === 'string' ? text : '';
    let h = 2166136261;
    for (let i = 0; i < text.length; i++) {
      h ^= text.charCodeAt(i);
      h = Math.imul(h, 16777619);
    }
    return ((h >>> 0) % 1000003) / 1000003;
  }

  function create(options) {
    options = options || {};
    const seed = Number.isFinite(options.seed) ? clamp(options.seed, 0, 1) : 0.37;
    return {
      seed,
      promptSeed: seed,
      warp: 0,
      corridorSkew: 0,
      verticalBias: 0,
      splitBias: 0,
      depthPulse: 0
    };
  }

  function resetFromPrompt(state, text) {
    if (!state) state = create();
    const seed = textSeed(text);
    const wordCount = (typeof text === 'string' && text.trim()) ? text.trim().split(/\s+/).length : 0;
    const density = clamp(wordCount / 56, 0, 1);
    state.seed = seed;
    state.promptSeed = seed;
    state.warp = 1;
    state.corridorSkew = (seed - 0.5) * (0.74 + density * 0.34);
    state.verticalBias = (hash(seed * 17.17 + wordCount) - 0.5) * (0.92 + density * 0.3);
    state.splitBias = (hash(seed * 31.31 + wordCount * 3.7) - 0.5) * (0.72 + density * 0.24);
    state.depthPulse = 0.8 + density * 0.2;
    return state;
  }

  function absorbToken(state, token, telemetry) {
    if (!state) state = create();
    const tokenSeed = textSeed(token);
    const tail = telemetry && telemetry.hasCandidateTailMass ? telemetry.candidateTailMass : 0;
    const surprise = telemetry && telemetry.hasSelectedProb ? 1 - telemetry.selectedProb : 0;
    const debt = telemetry && telemetry.hasDebt ? telemetry.debt : 0;
    const entropy = telemetry && telemetry.hasEntropy ? clamp(telemetry.entropy / 6, 0, 1) : 0;
    const pressure = clamp(tail * 0.42 + surprise * 0.34 + debt * 0.18 + entropy * 0.12, 0, 1);

    state.seed = (state.seed * (0.982 - pressure * 0.012) + tokenSeed * (0.018 + pressure * 0.012)) % 1;
    state.warp = clamp(state.warp + 0.016 + pressure * 0.042, 0, 1);
    state.corridorSkew = mix(state.corridorSkew, (tokenSeed - 0.5) * 1.4, 0.018 + pressure * 0.038);
    state.verticalBias = mix(state.verticalBias, (hash(tokenSeed * 23.23) - 0.5) * 1.3, 0.014 + pressure * 0.032);
    state.splitBias = mix(state.splitBias, (hash(tokenSeed * 41.41) - 0.5) * 1.2, 0.012 + tail * 0.048);
    state.depthPulse = clamp(state.depthPulse + pressure * 0.13, 0, 1);
    return state;
  }

  function decay(state, dt) {
    if (!state) return create();
    const frames = Math.max(0, dt * 60);
    state.warp *= Math.pow(0.955, frames);
    state.depthPulse *= Math.pow(0.935, frames);
    state.corridorSkew = mix(state.corridorSkew, 0, 0.006 * frames);
    state.verticalBias = mix(state.verticalBias, 0, 0.005 * frames);
    state.splitBias = mix(state.splitBias, 0, 0.005 * frames);
    return state;
  }

  function wallShapeParams(state, side) {
    state = state || create();
    side = side < 0 ? -1 : 1;
    const topo = state.seed + side * 7.13;
    const spread = 1 +
      (hash(topo) - 0.5) * 0.34 +
      state.warp * 0.08 +
      side * state.corridorSkew * 0.16 +
      state.depthPulse * 0.08;
    const lift =
      (hash(topo + 9.1) - 0.5) * 140 +
      state.verticalBias * 170 +
      side * state.splitBias * 95;
    const farLean =
      (hash(topo + 4.7) - 0.5) * 190 +
      state.corridorSkew * 270 +
      side * state.splitBias * 110;
    const nearDepth = 170 + state.depthPulse * 38;
    const farDepth = 3450 - state.depthPulse * 260 + side * state.corridorSkew * 140;

    return {
      nearDepth,
      farDepth,
      nearOuterX: 1120 + 190 * spread,
      nearOuterY: 470 + lift * 0.25,
      nearTopX: 1060 + 150 * spread,
      nearTopY: -160 + lift * 0.34,
      farTopX: 370 + farLean * 0.35,
      farTopY: -70 + lift * 0.18,
      farBottomX: 390 + farLean * 0.35,
      farBottomY: 270 + lift * 0.12,
      topo
    };
  }

  const api = {
    create,
    resetFromPrompt,
    absorbToken,
    decay,
    wallShapeParams,
    hash,
    textSeed
  };

  root.YentWorldmodelGeometry = api;
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
})(typeof globalThis !== 'undefined' ? globalThis : this);
