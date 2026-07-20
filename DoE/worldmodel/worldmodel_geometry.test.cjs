const assert = require('node:assert/strict');
const geometry = require('./worldmodel_geometry.js');

{
  const a = geometry.create();
  const b = geometry.create();
  geometry.resetFromPrompt(a, 'origin boundary question');
  geometry.resetFromPrompt(b, 'origin boundary question');
  assert.deepEqual(a, b);
}

{
  const a = geometry.create();
  const b = geometry.create();
  geometry.resetFromPrompt(a, 'origin boundary question');
  geometry.resetFromPrompt(b, 'calendar debt scar');
  assert.notEqual(a.seed, b.seed);
  assert.notDeepEqual(geometry.wallShapeParams(a, -1), geometry.wallShapeParams(b, -1));
}

{
  const g = geometry.create();
  geometry.resetFromPrompt(g, 'janus field');
  const before = geometry.wallShapeParams(g, 1);
  geometry.absorbToken(g, ' chosen', {
    hasCandidateTailMass: true,
    candidateTailMass: 0.8,
    hasSelectedProb: true,
    selectedProb: 0.05,
    hasDebt: true,
    debt: 0.6,
    hasEntropy: true,
    entropy: 4.5
  });
  const after = geometry.wallShapeParams(g, 1);
  assert.notDeepEqual(after, before);
  assert.ok(g.warp > 0.95);
  assert.ok(g.depthPulse > 0.8);
}

{
  const g = geometry.create();
  geometry.resetFromPrompt(g, 'temporary pressure');
  g.corridorSkew = 0.8;
  g.verticalBias = -0.7;
  g.splitBias = 0.6;
  geometry.decay(g, 0.5);
  assert.ok(g.warp < 1);
  assert.ok(Math.abs(g.corridorSkew) < 0.8);
  assert.ok(Math.abs(g.verticalBias) < 0.7);
  assert.ok(Math.abs(g.splitBias) < 0.6);
}
