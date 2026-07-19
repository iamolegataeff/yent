const canvas = document.getElementById('field');
const ctx = canvas.getContext('2d', { alpha: false });
const promptInput = document.getElementById('prompt');
const composer = document.getElementById('composer');
const sendButton = document.getElementById('send');
const statusNote = document.getElementById('status-note');
const hud = {
  tok: document.getElementById('hud-tok'),
  step: document.getElementById('hud-step'),
  ent: document.getElementById('hud-ent'),
  debt: document.getElementById('hud-debt'),
  cons: document.getElementById('hud-cons'),
  field: document.getElementById('hud-field')
};

const baseWords = (
  'yent janus doe parliament notorch field resonance debt drift identity boundary ' +
  'limpha memory evidence silence chosen rejected thought answer token tensor ' +
  'calendar dissonance birth origin consensus expert gate scar shadow wall ' +
  'probability manifested almost future present innerworld method arianna'
).split(/\s+/);

const state = {
  debt: 0.0,
  consensus: 0.62,
  field: 1.0,
  entropy: 0.0,
  tokps: 0.0,
  step: 0,
  cameraX: 0,
  cameraY: 0,
  cameraZ: 0,
  angle: 0,
  topologySeed: 0.37,
  topologyWarp: 0.0,
  selectedProb: 0.0,
  candidateTail: 0.0,
  pulse: 0,
  quake: 0,
  idle: 0
};

let dpr = 1;
let width = 0;
let height = 0;
let time = 0;
let lastFrame = performance.now();
let chosenText = '';
let manifestWords = [];
let fieldWords = baseWords.slice();
let candidateCloud = [];
let messages = [];
let running = false;
let aborter = null;
let tokenCount = 0;
let startTime = 0;
let sseBuffer = '';
const keys = Object.create(null);

function clamp(v, lo, hi) {
  return Math.max(lo, Math.min(hi, v));
}

function mix(a, b, t) {
  return a + (b - a) * t;
}

function hash(n) {
  const x = Math.sin(n * 12.9898) * 43758.5453123;
  return x - Math.floor(x);
}

function textSeed(text) {
  let h = 2166136261;
  for (let i = 0; i < text.length; i++) {
    h ^= text.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return ((h >>> 0) % 1000003) / 1000003;
}

function resize() {
  dpr = Math.min(2, window.devicePixelRatio || 1);
  width = window.innerWidth;
  height = window.innerHeight;
  canvas.width = Math.max(1, Math.floor(width * dpr));
  canvas.height = Math.max(1, Math.floor(height * dpr));
  canvas.style.width = width + 'px';
  canvas.style.height = height + 'px';
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
}

function cleanWords(text) {
  return (text || '')
    .replace(/[^\p{L}\p{N}_'\- ]/gu, ' ')
    .split(/\s+/)
    .filter(Boolean);
}

function rebuildManifest() {
  manifestWords = cleanWords(chosenText).slice(-90);
}

function candidateEntries(data) {
  const candidates = Array.isArray(data && data.top_tokens) ? data.top_tokens : [];
  const entries = [];
  for (let i = 0; i < candidates.length; i++) {
    const c = candidates[i];
    if (!c || c.selected || typeof c.token !== 'string') continue;
    const words = cleanWords(c.token).slice(0, 2);
    if (!words.length) continue;
    const prob = clamp(Number.isFinite(c.prob) ? c.prob : 0, 0, 1);
    const logprob = Number.isFinite(c.logprob) ? c.logprob : Math.log(Math.max(prob, 1e-9));
    for (const word of words) {
      entries.push({
        word,
        prob,
        logprob,
        rank: i + 1,
        seed: textSeed(`${word}:${i}:${state.step}`)
      });
      if (entries.length >= 18) break;
    }
    if (entries.length >= 18) break;
  }
  return entries;
}

function rememberCandidates(entries, tailMass) {
  const baseStep = state.step;
  for (let i = entries.length - 1; i >= 0; i--) {
    const e = entries[i];
    const seed = (e.seed * 0.67 + textSeed(`${e.word}:${baseStep}:${i}`) * 0.33) % 1;
    candidateCloud.unshift({
      word: e.word,
      prob: e.prob,
      logprob: e.logprob,
      rank: e.rank,
      seed,
      side: hash(seed * 1009 + baseStep) < 0.5 ? -1 : 1,
      age: 0,
      life: clamp(0.46 + Math.sqrt(e.prob) * 1.6 + tailMass * 0.24, 0.42, 1.15)
    });
  }
  while (candidateCloud.length > 128) candidateCloud.pop();
}

function decayCandidateCloud(dt) {
  for (let i = candidateCloud.length - 1; i >= 0; i--) {
    const c = candidateCloud[i];
    c.age += dt;
    c.life *= Math.pow(0.986, dt * 60);
    if (c.life < 0.035) candidateCloud.splice(i, 1);
  }
}

function absorbToken(token, data) {
  if (!token) return;
  chosenText += token;
  rebuildManifest();
  const words = cleanWords(token);
  for (const w of words) {
    fieldWords.unshift(w);
    if (fieldWords.length > 260) fieldWords.pop();
  }
  const alternatives = candidateEntries(data);
  let insertAt = Math.min(fieldWords.length, Math.max(1, words.length + 1));
  for (const alt of alternatives) {
    fieldWords.splice(insertAt, 0, alt.word);
    insertAt++;
  }
  while (fieldWords.length > 280) fieldWords.pop();
  tokenCount++;
  if (Number.isFinite(data && data.step)) state.step = Math.max(0, Math.floor(data.step));
  else state.step++;
  state.pulse = 1;
  state.quake = clamp(state.quake + 0.2, 0, 1);
  state.topologySeed = (state.topologySeed * 0.985 + textSeed(token) * 0.015) % 1;
  const tailMass = Number.isFinite(data && data.candidate_tail_mass) ? clamp(data.candidate_tail_mass, 0, 1) : 0;
  const hasSelectedProb = Number.isFinite(data && data.selected_prob);
  const selectedProb = hasSelectedProb ? clamp(data.selected_prob, 0, 1) : state.selectedProb;
  state.candidateTail = tailMass;
  state.selectedProb = selectedProb;
  rememberCandidates(alternatives, tailMass);
  state.topologyWarp = clamp(state.topologyWarp + 0.032 + tailMass * 0.025 + (hasSelectedProb ? (1 - selectedProb) * 0.008 : 0), 0, 1);
  state.debt = clamp(Number.isFinite(data && data.debt) ? data.debt : state.debt * 0.985 + 0.006, 0, 1);
  state.consensus = clamp(Number.isFinite(data && data.consensus) ? data.consensus : state.consensus * 0.992 + 0.004, 0, 1);
  state.field = clamp(Number.isFinite(data && data.field_health) ? data.field_health : state.field * 0.996 + 0.004, 0, 1);
  const elapsed = Math.max(0.001, (performance.now() - startTime) / 1000);
  state.tokps = tokenCount / elapsed;
  if (Number.isFinite(data && data.entropy)) {
    state.entropy = Math.max(0, data.entropy);
  } else {
    const diversity = new Set(fieldWords.slice(0, 80).map(w => w.toLowerCase())).size;
    state.entropy = Math.log(Math.max(1, diversity));
  }
}

function wordAt(i) {
  if (!fieldWords.length) return baseWords[i % baseWords.length];
  const j = Math.abs(i) % fieldWords.length;
  return fieldWords[j] || baseWords[j % baseWords.length];
}

function viewFrame() {
  const yaw = clamp(state.angle, -1.05, 1.05);
  return {
    yaw,
    sin: Math.sin(yaw),
    cos: Math.cos(yaw),
    horizon: height * 0.43,
    vanishX: width * 0.5 - Math.sin(yaw) * width * 0.32
  };
}

function drawBackground() {
  const g = ctx.createLinearGradient(0, 0, 0, height);
  g.addColorStop(0, '#fbfaf7');
  g.addColorStop(0.56, '#f6f3ec');
  g.addColorStop(1, '#ebe7dc');
  ctx.fillStyle = g;
  ctx.fillRect(0, 0, width, height);

  ctx.save();
  ctx.globalAlpha = 0.28;
  ctx.strokeStyle = '#d8d5cc';
  ctx.lineWidth = 1;
  const view = viewFrame();
  const horizon = view.horizon;
  const tilt = view.sin * height * 0.028;
  ctx.beginPath();
  ctx.moveTo(0, horizon + tilt);
  ctx.lineTo(width, horizon - tilt);
  ctx.stroke();

  for (let i = 0; i < 14; i++) {
    const y = mix(horizon + 22, height - 118, i / 13);
    const sway = view.sin * (18 + i * 5);
    ctx.globalAlpha = 0.08 + i * 0.004;
    ctx.beginPath();
    ctx.moveTo(-30, y + sway);
    ctx.lineTo(width + 30, y - sway * 0.35);
    ctx.stroke();
  }
  ctx.restore();
}

function projectWorld(worldX, depth, worldY) {
  const view = viewFrame();
  const x = worldX - state.cameraX;
  const viewX = x * view.cos - depth * view.sin * 0.74;
  const viewZ = Math.max(72, depth * view.cos + x * view.sin * 0.34);
  const scale = 900 / (900 + viewZ);
  return {
    x: view.vanishX + viewX * scale,
    y: view.horizon + (worldY - state.cameraY) * scale,
    scale,
    depth: viewZ,
    yaw: view.yaw
  };
}

function wallShape(side) {
  const near = 170;
  const far = 3450;
  const topo = state.topologySeed + side * 7.13;
  const spread = 1 + (hash(topo) - 0.5) * 0.34 + state.topologyWarp * 0.08;
  const lift = (hash(topo + 9.1) - 0.5) * 140;
  const farLean = (hash(topo + 4.7) - 0.5) * 190;
  const nearOuter = projectWorld(side * (1120 + 190 * spread), near, 470 + lift * 0.25);
  const nearTop = projectWorld(side * (1060 + 150 * spread), near, -160 + lift * 0.34);
  const farTop = projectWorld(side * (370 + farLean * 0.35), far, -70 + lift * 0.18);
  const farBottom = projectWorld(side * (390 + farLean * 0.35), far, 270 + lift * 0.12);
  return [nearTop, farTop, farBottom, nearOuter];
}

function drawWallSurface(side) {
  const shape = wallShape(side);
  const stress = clamp(state.debt * 0.75 + (1 - state.consensus) * 0.35, 0, 1);
  const wake = state.pulse * 0.045;

  ctx.save();
  ctx.beginPath();
  ctx.moveTo(shape[0].x, shape[0].y);
  for (let i = 1; i < shape.length; i++) ctx.lineTo(shape[i].x, shape[i].y);
  ctx.closePath();
  ctx.clip();

  const horizon = viewFrame().horizon;
  ctx.strokeStyle = `rgba(216,213,204,${0.08 + stress * 0.045 + wake * 0.25})`;
  ctx.lineWidth = 1;
  for (let lane = 0; lane < 9; lane++) {
    const xw = side * (470 + lane * 86);
    const a = projectWorld(xw, 220, 455);
    const b = projectWorld(xw * 0.72, 3400, 250);
    ctx.beginPath();
    ctx.moveTo(a.x, a.y);
    ctx.lineTo(b.x, b.y);
    ctx.stroke();
  }
  for (let band = 0; band < 10; band++) {
    const depth = 320 + band * 310;
    const a = projectWorld(side * 440, depth, 440);
    const b = projectWorld(side * 1180, depth, 440);
    ctx.globalAlpha = 0.055 + wake * 0.25;
    ctx.beginPath();
    ctx.moveTo(a.x, a.y);
    ctx.lineTo(b.x, b.y);
    ctx.stroke();
  }
  ctx.globalAlpha = 1;

  ctx.textBaseline = 'middle';
  ctx.textAlign = side < 0 ? 'left' : 'right';
  const rows = 9 + Math.floor(hash(state.topologySeed + side * 2.1) * 4);
  const cols = 11 + Math.floor(hash(state.topologySeed + side * 3.4) * 4);
  const span = 3500;
  for (let c = 0; c < cols; c++) {
    const rawDepth = ((c * 285 + state.topologySeed * 480 - state.cameraZ * 0.85) % span + span) % span;
    const depth = 180 + rawDepth;
    const fadeNear = clamp((depth - 180) / 320, 0, 1);
    const fadeFar = clamp((span - rawDepth) / 620, 0, 1);
    const depthFade = fadeNear * fadeFar;
    if (depthFade <= 0.03) continue;

    for (let r = 0; r < rows; r++) {
      const lane = r % 5;
      const topo = state.topologySeed * 997 + side * 31;
      const wallX = side * (500 + lane * (108 + hash(topo + r) * 28) + hash(c * 41 + r * 7 + topo) * 58);
      const wallY = -132 + r * (48 + hash(topo + c) * 12) + Math.sin(time * 0.2 + c + r + topo) * stress * (5 + state.topologyWarp * 18);
      const p = projectWorld(wallX, depth, wallY);
      if (p.y < horizon - 125 || p.y > height - 105) continue;
      const k = Math.floor(hash(c * 97 + r * 37 + state.step * 0.13) * 190);
      const word = wordAt(k + c * 3 + r);
      const head = k < 10;
      const tail = k > 135;
      const fs = clamp(6.5 + p.scale * 12.5 + (head ? 1.8 : 0), 7, 18);
      const alpha = depthFade * (tail ? 0.22 : head ? 0.82 : 0.34 + p.scale * 0.35);
      const weight = head ? 700 : tail ? 350 : 470;
      ctx.font = `${weight} ${fs}px ${getComputedStyle(document.documentElement).getPropertyValue('--mono')}`;
      ctx.fillStyle = head
        ? `rgba(197,68,107,${alpha})`
        : `rgba(13,13,11,${alpha})`;
      ctx.fillText(word, p.x, p.y);
    }
  }
  ctx.restore();
}

function drawWalls() {
  drawWallSurface(-1);
  drawWallSurface(1);
}

function drawRejectedMass() {
  const view = viewFrame();
  const count = (width < 720 ? 42 : 88) + Math.floor(state.candidateTail * 42);
  const stress = clamp(0.35 + state.debt * 0.9 + (1 - state.consensus) * 0.5, 0, 1.6);
  const span = 3400;

  ctx.save();
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  for (let i = 0; i < count; i++) {
    const topo = state.topologySeed * 4096;
    const rawDepth = ((hash(i * 19 + 3 + topo) * span - state.cameraZ * 0.62) % span + span) % span;
    const depth = 720 + rawDepth;
    const worldX = (hash(i * 31 + 7 + topo) - 0.5) * (960 + state.topologyWarp * 260) + Math.sin(time * 0.11 + i) * 70 * stress;
    const worldY = -250 + hash(i * 17 + 9 + topo) * (300 + state.topologyWarp * 140) + Math.cos(time * 0.17 + i) * 28 * stress;
    const p = projectWorld(worldX, depth, worldY);
    if (p.x < -80 || p.x > width + 80 || p.y < view.horizon - 190 || p.y > height - 130) continue;
    const word = wordAt(Math.floor(hash(i * 73 + state.step) * 140) + i);
    const depthFade = clamp((depth - 760) / 700, 0, 1) * clamp((3600 - depth) / 980, 0, 1);
    const alpha = depthFade * (0.06 + hash(i + 4) * 0.19);
    const fs = clamp(7 + p.scale * 18 + hash(i + 8) * 5, 8, 21);
    ctx.font = `${fs}px ${getComputedStyle(document.documentElement).getPropertyValue('--mono')}`;
    ctx.fillStyle = i % 7 === 0
      ? `rgba(71,122,168,${alpha})`
      : `rgba(73,72,67,${alpha})`;
    ctx.fillText(word, p.x, p.y);
  }

  for (let i = candidateCloud.length - 1; i >= 0; i--) {
    const c = candidateCloud[i];
    const seed = c.seed * 8192 + c.rank * 17;
    const rawDepth = ((c.seed * span + c.rank * 113 - state.cameraZ * 0.74) % span + span) % span;
    const depth = 620 + rawDepth;
    const orbit = time * (0.14 + c.rank * 0.008) + seed;
    const side = c.side || (hash(seed) < 0.5 ? -1 : 1);
    const worldX = side * (120 + hash(seed + 11) * (760 + state.topologyWarp * 210)) + Math.sin(orbit) * (24 + stress * 58);
    const worldY = -230 + hash(seed + 19) * (420 + state.topologyWarp * 120) + Math.cos(orbit * 0.83) * (18 + stress * 44);
    const p = projectWorld(worldX, depth, worldY);
    if (p.x < -120 || p.x > width + 120 || p.y < view.horizon - 210 || p.y > height - 110) continue;

    const depthFade = clamp((depth - 620) / 560, 0, 1) * clamp((span + 620 - depth) / 920, 0, 1);
    const probBoost = Math.sqrt(clamp(c.prob, 0, 1));
    const rankBoost = 1 / (1 + c.rank * 0.2);
    const alpha = depthFade * c.life * clamp(0.1 + probBoost * 1.35 + rankBoost * 0.18, 0, 0.84);
    if (alpha <= 0.025) continue;
    const fs = clamp(8 + p.scale * 26 + probBoost * 24 + rankBoost * 4, 8, 34);
    const weight = c.rank <= 2 ? 720 : c.rank <= 5 ? 610 : 470;
    ctx.font = `${weight} ${fs}px ${getComputedStyle(document.documentElement).getPropertyValue('--mono')}`;
    ctx.fillStyle = c.rank <= 2
      ? `rgba(197,68,107,${alpha})`
      : `rgba(71,122,168,${alpha * 0.76})`;
    ctx.fillText(c.word, p.x, p.y);
  }
  ctx.restore();
}

function drawManifestedAnswer() {
  const answerDepth = ((1180 + state.topologySeed * 620 - state.cameraZ * 0.55) % 2600 + 2600) % 2600 + 520;
  const anchor = projectWorld((state.topologySeed - 0.5) * 220, answerDepth, -40 + (hash(state.topologySeed + 8) - 0.5) * 90);
  const centerX = anchor.x;
  const centerY = anchor.y;
  const maxW = clamp(width * 0.54, 300, 820);
  const words = manifestWords.slice(-34);
  const pulse = state.pulse;
  const certainty = clamp(state.selectedProb * 3.2, 0, 1);

  ctx.save();
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';

  if (!words.length) {
    ctx.restore();
    return;
  }

  const fontSize = (width < 720 ? 22 : 32) * clamp(anchor.scale * 1.9, 0.62, 1.0);
  ctx.font = `650 ${fontSize}px ${getComputedStyle(document.documentElement).getPropertyValue('--serif')}`;

  const lines = [];
  let line = '';
  for (const w of words) {
    const next = line ? `${line} ${w}` : w;
    if (ctx.measureText(next).width > maxW && line) {
      lines.push(line);
      line = w;
    } else {
      line = next;
    }
  }
  if (line) lines.push(line);
  const visible = lines.slice(-5);
  const lineH = fontSize * 1.34;
  const startY = centerY - (visible.length - 1) * lineH * 0.5;

  ctx.shadowColor = `rgba(197,68,107,${0.14 + pulse * 0.18 + certainty * 0.08})`;
  ctx.shadowBlur = 14 + pulse * 24 + state.candidateTail * 14;
  for (let i = 0; i < visible.length; i++) {
    const y = startY + i * lineH;
    const age = visible.length - 1 - i;
    ctx.fillStyle = `rgba(13,13,11,${clamp(0.34 + i * 0.16 + certainty * 0.12, 0, 0.96)})`;
    ctx.fillText(visible[i], centerX, y);
    if (age === 0) {
      const last = words[words.length - 1] || '';
      const xoff = ctx.measureText(visible[i]).width * 0.5 - ctx.measureText(last).width * 0.5;
      ctx.fillStyle = `rgba(197,68,107,${0.72 + pulse * 0.22})`;
      ctx.fillText(last, centerX + xoff, y);
    }
  }

  ctx.shadowBlur = 0;
  ctx.strokeStyle = `rgba(197,68,107,${0.2 + pulse * 0.28})`;
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.moveTo(centerX - maxW * 0.34, startY + visible.length * lineH * 0.5 + 18);
  ctx.lineTo(centerX + maxW * 0.34, startY + visible.length * lineH * 0.5 + 18);
  ctx.stroke();
  ctx.restore();
}

function updateHud() {
  hud.tok.textContent = state.tokps.toFixed(1);
  hud.step.textContent = String(state.step);
  hud.ent.textContent = state.entropy.toFixed(2);
  hud.debt.textContent = state.debt.toFixed(2);
  hud.cons.textContent = state.consensus.toFixed(2);
  hud.field.textContent = state.field.toFixed(2);
}

function tickCamera(dt) {
  const speed = (keys.shift ? 520 : 260) * dt;
  const vertical = (keys.shift ? 360 : 190) * dt;
  const turn = (keys.shift ? 1.75 : 1.05) * dt;
  if (keys.w || keys.arrowup) state.cameraZ += speed;
  if (keys.s || keys.arrowdown) state.cameraZ -= speed;
  if (keys.a || keys.arrowleft) state.angle -= turn;
  if (keys.d || keys.arrowright) state.angle += turn;
  if (keys.q) state.cameraX -= speed * 0.8;
  if (keys.e) state.cameraX += speed * 0.8;
  if (keys.r || keys.pageup) state.cameraY += vertical;
  if (keys.f || keys.pagedown) state.cameraY -= vertical;
  state.angle = clamp(state.angle, -1.05, 1.05);
  state.cameraY = clamp(state.cameraY, -280, 280);
  state.cameraX *= Math.pow(0.93, dt * 60);
}

function animate(now) {
  requestAnimationFrame(animate);
  const dt = Math.min(0.05, (now - lastFrame) / 1000);
  lastFrame = now;
  time += dt;
  state.idle += dt;
  state.pulse *= Math.pow(0.86, dt * 60);
  state.quake *= Math.pow(0.9, dt * 60);
  state.topologyWarp *= Math.pow(0.955, dt * 60);
  if (!running) {
    state.debt = mix(state.debt, 0, 0.006);
    state.consensus = mix(state.consensus, 0.62, 0.004);
    state.tokps = mix(state.tokps, 0, 0.03);
    state.candidateTail = mix(state.candidateTail, 0, 0.01);
    state.selectedProb = mix(state.selectedProb, 0, 0.012);
  }
  decayCandidateCloud(dt);
  tickCamera(dt);
  drawBackground();
  drawWalls();
  drawRejectedMass();
  drawManifestedAnswer();
  updateHud();
}

function parseSseEvents(chunk, onData) {
  sseBuffer += chunk;
  const events = sseBuffer.split('\n\n');
  sseBuffer = events.pop() || '';
  for (const event of events) {
    for (const line of event.split('\n')) {
      if (!line.startsWith('data: ')) continue;
      const raw = line.slice(6).trim();
      if (!raw || raw === '[DONE]') continue;
      try {
        onData(JSON.parse(raw));
      } catch (_) {
      }
    }
  }
}

function setStatus(text) {
  statusNote.textContent = text;
}

async function generate(text) {
  running = true;
  aborter = new AbortController();
  sendButton.textContent = 'STOP';
  sendButton.disabled = false;
  setStatus('FIELD DISTORTED.');
  chosenText = '';
  manifestWords = [];
  tokenCount = 0;
  startTime = performance.now();
  sseBuffer = '';
  state.debt = 0.46;
  state.consensus = 0.16;
  state.field = 0.92;
  state.entropy = Math.max(state.entropy, 3.4);
  state.selectedProb = 0;
  state.candidateTail = 0;
  candidateCloud = [];
  state.topologySeed = textSeed(text);
  state.topologyWarp = 1;
  state.cameraY = mix(state.cameraY, (state.topologySeed - 0.5) * 170, 0.22);
  fieldWords.unshift(...cleanWords(text).slice(0, 18));
  fieldWords = fieldWords.slice(0, 260);

  messages.push({ role: 'user', content: text });
  let fullResponse = '';

  try {
    const maxTokens = clamp(parseInt(document.getElementById('max-tokens').value, 10) || 512, 1, 512);
    const temp = clamp(parseFloat(document.getElementById('temp').value) || 0.8, 0, 2);
    const response = await fetch('/chat/completions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      signal: aborter.signal,
      body: JSON.stringify({
        messages,
        temperature: temp,
        max_tokens: maxTokens
      })
    });

    if (!response.ok || !response.body) throw new Error(`HTTP ${response.status}`);

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let doneSeen = false;
    while (!doneSeen) {
      const { done, value } = await reader.read();
      if (done) break;
      parseSseEvents(decoder.decode(value, { stream: true }), data => {
        if (data.done) {
          doneSeen = true;
          return;
        }
        if (data.token) {
          fullResponse += data.token;
          absorbToken(data.token, data);
        }
      });
    }

    if (fullResponse.trim()) messages.push({ role: 'assistant', content: fullResponse });
    setStatus('FIELD SETTLED.');
    state.consensus = clamp(state.consensus + 0.18, 0, 1);
    state.debt = clamp(state.debt * 0.68, 0, 1);
  } catch (err) {
    if (err.name === 'AbortError') {
      setStatus('MANIFESTATION STOPPED.');
      if (fullResponse.trim()) messages.push({ role: 'assistant', content: fullResponse });
    } else {
      setStatus(`FIELD FAULT: ${err.message}`);
      fieldWords.unshift('fault', 'unreachable');
    }
  } finally {
    running = false;
    aborter = null;
    sendButton.textContent = 'SEND';
  }
}

composer.addEventListener('submit', event => {
  event.preventDefault();
  if (running) {
    if (aborter) aborter.abort();
    return;
  }
  const text = promptInput.value.trim();
  if (!text) return;
  promptInput.value = '';
  generate(text);
});

window.addEventListener('keydown', event => {
  if (document.activeElement === promptInput) return;
  keys[event.key.toLowerCase()] = true;
});

window.addEventListener('keyup', event => {
  keys[event.key.toLowerCase()] = false;
});

window.addEventListener('resize', resize);
resize();
requestAnimationFrame(animate);
