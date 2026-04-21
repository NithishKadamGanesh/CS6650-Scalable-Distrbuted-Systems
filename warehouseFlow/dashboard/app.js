/* WarehouseFlow Operations Control — self-contained simulator.
   No backend required. Drives all visuals including particles, charts,
   circuit breakers, failure injection, and experiment autopilot. */

(() => {
'use strict';

const CONFIG = {
  tickMs: 100,
  maxHistory: 120,
  particleDuration: 900,
  maxFeedEntries: 40,
  routerTasks: 4,
};

const state = {
  traffic: { active: false, ratePerMin: 3000, pattern: 'steady' },
  warehouses: {
    a: { id: 'a', region: 'US-EAST', status: 'ok', inventory: 2400, pickersUsed: 2, pickersTotal: 20, cbState: 'CLOSED', cbFailures: 0 },
    b: { id: 'b', region: 'US-CENTRAL', status: 'ok', inventory: 2400, pickersUsed: 1, pickersTotal: 20, cbState: 'CLOSED', cbFailures: 0 },
    c: { id: 'c', region: 'US-WEST', status: 'ok', inventory: 2400, pickersUsed: 0, pickersTotal: 20, cbState: 'CLOSED', cbFailures: 0 },
  },
  strategy: 'optimistic',
  resilience: { cb: true, bulkhead: true, retry: true, failfast: true },
  totals: { routed: 0, rejected: 0, oversells: 0 },
  counters: { dlq: 0, kafkaLag: 0, retries: 0 },
  series: {
    throughput: new Array(CONFIG.maxHistory).fill(0),
    p50: new Array(CONFIG.maxHistory).fill(0),
    p95: new Array(CONFIG.maxHistory).fill(0),
    p99: new Array(CONFIG.maxHistory).fill(0),
    whA: new Array(CONFIG.maxHistory).fill(0),
    whB: new Array(CONFIG.maxHistory).fill(0),
    whC: new Array(CONFIG.maxHistory).fill(0),
    dlq: new Array(CONFIG.maxHistory).fill(0),
    kafkaLag: new Array(CONFIG.maxHistory).fill(0),
    retries: new Array(CONFIG.maxHistory).fill(0),
  },
  latencyReservoir: [],
};

const $ = sel => document.querySelector(sel);
const dom = {
  rateSlider: $('#rate-slider'),
  rateValue: $('#rate-value'),
  trafficBtn: $('#traffic-toggle'),
  recoverAll: $('#recover-all'),
  strategyHint: $('#strategy-hint'),
  sysHealthDot: $('#sys-health-dot'),
  sysHealthText: $('#sys-health-text'),
  sysClock: $('#sys-clock'),
  sysMode: $('#sys-mode'),
  routerTasks: $('#router-task-count'),
  kpi: {
    throughput: $('#kpi-throughput'), p95: $('#kpi-p95'), p99: $('#kpi-p99'),
    success: $('#kpi-success'), dlq: $('#kpi-dlq'), oversells: $('#kpi-oversells'),
  },
  invA: $('#inv-a'), invB: $('#inv-b'), invC: $('#inv-c'),
  pickA: $('#pick-a'), pickB: $('#pick-b'), pickC: $('#pick-c'),
  particleLayer: $('#routing-particles'),
  feed: $('#order-feed'),
  feedCount: $('#feed-count'),
  ticker: $('#event-ticker'),
  charts: {
    throughput: $('#chart-throughput'),
    latency: $('#chart-latency'),
    warehouse: $('#chart-warehouse'),
    counters: $('#chart-counters'),
  },
};

const fmt = (n, dec = 0) => {
  if (n == null || isNaN(n)) return '—';
  if (n === 0) return dec === 0 ? '0' : '0.' + '0'.repeat(dec);
  return n.toLocaleString(undefined, { minimumFractionDigits: dec, maximumFractionDigits: dec });
};
const uuid = () => 'xxxx-xxxx'.replace(/x/g, () => Math.floor(Math.random() * 16).toString(16));
const now = () => performance.now();

function tickerPush(msg, tone = 'info') {
  const item = document.createElement('span');
  item.className = 'ticker-item ' + (tone === 'info' ? '' : tone);
  item.textContent = msg;
  dom.ticker.appendChild(item);
  while (dom.ticker.children.length > 30) dom.ticker.removeChild(dom.ticker.firstChild);
  dom.ticker.scrollLeft = dom.ticker.scrollWidth;
}

function pushSeries(key, value) { state.series[key].shift(); state.series[key].push(value); }

function recordLatency(ms) {
  state.latencyReservoir.push(ms);
  if (state.latencyReservoir.length > 500) state.latencyReservoir.shift();
}

function percentile(arr, p) {
  if (!arr.length) return 0;
  const sorted = [...arr].sort((a, b) => a - b);
  return sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * p))];
}

let tickIndex = 0;

function simulateTick() {
  tickIndex++;
  let ordersThisTick = 0;
  if (state.traffic.active) {
    const base = state.traffic.ratePerMin / 600;
    if (state.traffic.pattern === 'burst') {
      const phase = Math.sin(tickIndex / 20);
      ordersThisTick = Math.round(base * (1.4 + phase * 1.1));
    } else {
      ordersThisTick = Math.round(base * (0.85 + Math.random() * 0.3));
    }
  }

  let routed = 0, rejected = 0;
  const whCount = { a: 0, b: 0, c: 0 };

  for (let i = 0; i < ordersThisTick; i++) {
    const order = {
      id: uuid(),
      region: ['US-EAST', 'US-CENTRAL', 'US-WEST'][Math.floor(Math.random() * 3)],
      sku: ['SKU-ALPHA', 'SKU-BETA', 'SKU-GAMMA'][Math.floor(Math.random() * 3)],
    };
    const result = routeOrder(order);
    if (result.routed) {
      state.totals.routed++; routed++;
      whCount[result.warehouse]++;
      if (Math.random() < 0.35) animateParticle(result.warehouse, result.color);
      if (Math.random() < 0.15) addFeedEntry(order, result);
    } else {
      state.totals.rejected++; rejected++;
      state.counters.dlq++;
      if (Math.random() < 0.5) addFeedEntry(order, result);
    }
    if (result.retries) state.counters.retries += result.retries;
  }

  const capacity = CONFIG.routerTasks * 25;
  const overflow = Math.max(0, ordersThisTick - capacity);
  state.counters.kafkaLag = Math.max(0, state.counters.kafkaLag + overflow - capacity * 0.2);

  const allHealthy = Object.values(state.warehouses).every(w => w.status === 'ok');
  if (allHealthy && state.counters.dlq > 0) {
    state.counters.dlq = Math.max(0, state.counters.dlq - Math.floor(5 + Math.random() * 5));
  }

  Object.values(state.warehouses).forEach(updateWarehouseCB);

  Object.values(state.warehouses).forEach(w => {
    if (w.status === 'failed') { w.pickersUsed = 0; return; }
    const target = Math.min(w.pickersTotal, Math.floor(whCount[w.id] * 0.6));
    w.pickersUsed = Math.max(0, Math.min(w.pickersTotal, target + (Math.random() < 0.5 ? -1 : 1)));
  });

  pushSeries('throughput', routed * 10);
  pushSeries('p50', percentile(state.latencyReservoir, 0.50));
  pushSeries('p95', percentile(state.latencyReservoir, 0.95));
  pushSeries('p99', percentile(state.latencyReservoir, 0.99));
  pushSeries('whA', whCount.a * 10);
  pushSeries('whB', whCount.b * 10);
  pushSeries('whC', whCount.c * 10);
  pushSeries('dlq', state.counters.dlq);
  pushSeries('kafkaLag', state.counters.kafkaLag);
  pushSeries('retries', state.counters.retries);

  renderKPIs();
  renderWarehouses();
  renderCharts();
  renderSystemHealth();
}

function routeOrder(order) {
  const regionPref = {
    'US-EAST': ['a', 'b', 'c'], 'US-CENTRAL': ['b', 'a', 'c'], 'US-WEST': ['c', 'b', 'a'],
  };
  const tryOrder = regionPref[order.region] || ['a', 'b', 'c'];

  let retries = 0;
  for (const wid of tryOrder) {
    const w = state.warehouses[wid];
    if (state.resilience.cb && w.cbState === 'OPEN') continue;
    if (w.status === 'failed') { recordCBFailure(w); continue; }

    let latency = 4 + Math.random() * 8;
    if (w.status === 'slow') {
      if (state.resilience.failfast) { recordCBFailure(w); continue; }
      else latency = 800 + Math.random() * 1200;
    }

    if (w.inventory < 1) continue;
    if (w.pickersUsed >= w.pickersTotal && state.resilience.bulkhead) continue;

    if (state.strategy === 'optimistic' && w.inventory < 100 && Math.random() < 0.25) {
      retries++;
      state.counters.retries++;
      if (retries >= 3) continue;
      latency += 2 + Math.random() * 4;
    }
    if (state.strategy === 'pessimistic' && w.inventory < 150) latency += 8 + Math.random() * 10;

    w.inventory = Math.max(0, w.inventory - 1);
    recordLatency(latency);
    recordCBSuccess(w);

    const colors = { a: 'to-a', b: 'to-b', c: 'to-c' };
    return { routed: true, warehouse: wid, latency, retries, color: colors[wid] };
  }

  return { routed: false, reason: 'all warehouses unavailable', retries };
}

const CB_THRESHOLD = 5, CB_COOLDOWN_MS = 4000;

function recordCBFailure(w) {
  if (!state.resilience.cb) return;
  w.cbFailures++;
  if (w.cbState === 'CLOSED' && w.cbFailures >= CB_THRESHOLD) {
    w.cbState = 'OPEN';
    w.cbOpenedAt = now();
    tickerPush(`CIRCUIT BREAKER OPEN — WAREHOUSE ${w.id.toUpperCase()}`, 'warn');
  } else if (w.cbState === 'HALF-OPEN') {
    w.cbState = 'OPEN';
    w.cbOpenedAt = now();
    tickerPush(`CB PROBE FAILED — ${w.id.toUpperCase()} REMAINS OPEN`, 'warn');
  }
}
function recordCBSuccess(w) {
  w.cbFailures = 0;
  if (w.cbState === 'HALF-OPEN') {
    w.cbSuccessProbes = (w.cbSuccessProbes || 0) + 1;
    if (w.cbSuccessProbes >= 2) {
      w.cbState = 'CLOSED';
      w.cbSuccessProbes = 0;
      tickerPush(`CB CLOSED — ${w.id.toUpperCase()} RECOVERED`, 'ok');
    }
  }
}
function updateWarehouseCB(w) {
  if (!state.resilience.cb) { w.cbState = 'CLOSED'; return; }
  if (w.cbState === 'OPEN' && now() - (w.cbOpenedAt || 0) > CB_COOLDOWN_MS) {
    w.cbState = 'HALF-OPEN';
    w.cbSuccessProbes = 0;
    tickerPush(`CB → HALF-OPEN — ${w.id.toUpperCase()} PROBING`, 'warn');
  }
}

function renderKPIs() {
  const tp = state.series.throughput[state.series.throughput.length - 1];
  const p95 = state.series.p95[state.series.p95.length - 1];
  const p99 = state.series.p99[state.series.p99.length - 1];
  const total = state.totals.routed + state.totals.rejected;
  const successRate = total > 0 ? (state.totals.routed / total) * 100 : 100;

  dom.kpi.throughput.textContent = fmt(tp);
  dom.kpi.p95.textContent = p95 > 0 ? fmt(p95, 1) : '—';
  dom.kpi.p99.textContent = p99 > 0 ? fmt(p99, 1) : '—';
  dom.kpi.success.textContent = fmt(successRate, 1);
  dom.kpi.dlq.textContent = fmt(state.counters.dlq);
  dom.kpi.oversells.textContent = fmt(state.totals.oversells);

  dom.kpi.p99.className = 'kpi-value ' + (p99 > 200 ? 'crit' : p99 > 100 ? 'warn' : p99 > 0 ? 'ok' : '');
  dom.kpi.p95.className = 'kpi-value ' + (p95 > 100 ? 'crit' : p95 > 50 ? 'warn' : p95 > 0 ? 'ok' : '');
  dom.kpi.success.className = 'kpi-value ' + (successRate < 95 ? 'crit' : successRate < 99 ? 'warn' : 'ok');
  dom.kpi.dlq.className = 'kpi-value ' + (state.counters.dlq > 50 ? 'crit' : state.counters.dlq > 0 ? 'warn' : '');
  dom.kpi.oversells.className = 'kpi-value ' + (state.totals.oversells > 0 ? 'crit' : 'ok');
  dom.kpi.throughput.className = 'kpi-value info';
}

function renderWarehouses() {
  ['a', 'b', 'c'].forEach(id => {
    const w = state.warehouses[id];
    const g = document.getElementById('warehouse-' + id);
    g.classList.remove('slow', 'failed');
    if (w.status === 'slow') g.classList.add('slow');
    if (w.status === 'failed') g.classList.add('failed');

    dom['inv' + id.toUpperCase()].textContent = fmt(w.inventory);
    dom['pick' + id.toUpperCase()].textContent = `${w.pickersUsed}/${w.pickersTotal}`;

    const cb = document.getElementById('cb-' + id);
    cb.classList.remove('open', 'half');
    const cbText = cb.querySelector('text');
    if (w.cbState === 'OPEN') { cb.classList.add('open'); cbText.textContent = 'CB: OPEN'; }
    else if (w.cbState === 'HALF-OPEN') { cb.classList.add('half'); cbText.textContent = 'CB: HALF'; }
    else cbText.textContent = 'CB: CLOSED';
  });
}

function renderSystemHealth() {
  const failed = Object.values(state.warehouses).filter(w => w.status === 'failed').length;
  const slow = Object.values(state.warehouses).filter(w => w.status === 'slow').length;
  let dotClass = 'pulse-dot', text = 'SYSTEM NOMINAL';
  if (failed > 0) { dotClass = 'pulse-dot crit'; text = `${failed} NODE${failed > 1 ? 'S' : ''} FAILED`; }
  else if (slow > 0) { dotClass = 'pulse-dot warn'; text = `${slow} NODE${slow > 1 ? 'S' : ''} DEGRADED`; }
  dom.sysHealthDot.className = dotClass;
  dom.sysHealthText.textContent = text;
  dom.sysClock.textContent = new Date().toISOString().substr(11, 8) + ' UTC';
}

const paths = {
  a: { x0: 296, y0: 210, cx: 500, cy: 150, x1: 620, y1: 100 },
  b: { x0: 296, y0: 210, cx: null, cy: null, x1: 620, y1: 210 },
  c: { x0: 296, y0: 210, cx: 500, cy: 260, x1: 620, y1: 320 },
};

function animateParticle(wid, colorClass) {
  const p = paths[wid];
  const circle = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
  circle.setAttribute('r', 3);
  circle.setAttribute('class', 'particle ' + colorClass);
  dom.particleLayer.appendChild(circle);

  const start = now();
  function step() {
    const t = Math.min(1, (now() - start) / CONFIG.particleDuration);
    let x, y;
    if (p.cx === null) { x = p.x0 + (p.x1 - p.x0) * t; y = p.y0 + (p.y1 - p.y0) * t; }
    else {
      const mt = 1 - t;
      x = mt * mt * p.x0 + 2 * mt * t * p.cx + t * t * p.x1;
      y = mt * mt * p.y0 + 2 * mt * t * p.cy + t * t * p.y1;
    }
    circle.setAttribute('cx', x);
    circle.setAttribute('cy', y);
    if (t < 1) requestAnimationFrame(step);
    else circle.remove();
  }
  requestAnimationFrame(step);
}

function addFeedEntry(order, result) {
  const entry = document.createElement('div');
  entry.className = 'feed-entry';
  if (!result.routed) entry.classList.add('rejected');
  else if (result.latency > 100) entry.classList.add('slow');

  const ts = new Date().toISOString().substr(14, 8);
  const meta = result.routed
    ? `→ W/${result.warehouse.toUpperCase()} · ${fmt(result.latency, 1)}ms`
    : `✕ ${result.reason.toUpperCase().slice(0, 14)}`;

  entry.innerHTML = `<span class="t">${ts}</span><span class="oid">${order.id} · ${order.sku}</span><span class="meta">${meta}</span>`;
  dom.feed.insertBefore(entry, dom.feed.firstChild);
  while (dom.feed.children.length > CONFIG.maxFeedEntries) dom.feed.removeChild(dom.feed.lastChild);
  dom.feedCount.textContent = fmt(state.totals.routed) + ' routed';
}


function drawLineChart(canvas, series, opts = {}) {
  const ctx = canvas.getContext('2d');
  const dpr = window.devicePixelRatio || 1;
  const w = canvas.clientWidth, h = canvas.clientHeight;
  if (canvas.width !== w * dpr) { canvas.width = w * dpr; canvas.height = h * dpr; }
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, w, h);

  const allData = series.map(s => s.data).flat();
  const rawMax = Math.max(...allData, opts.floor || 0);
  const max = rawMax * 1.15 || 10;

  ctx.strokeStyle = '#1a2230';
  ctx.lineWidth = 0.5;
  for (let i = 0; i <= 3; i++) {
    const y = (h - 10) * (i / 3) + 5;
    ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(w, y); ctx.stroke();
  }

  series.forEach(s => {
    const n = s.data.length;
    if (opts.filled) {
      ctx.fillStyle = s.fill || s.color + '22';
      ctx.beginPath();
      ctx.moveTo(0, h - 5);
      s.data.forEach((v, i) => {
        const x = (i / (n - 1)) * w;
        const y = h - 5 - ((v / max) * (h - 10));
        ctx.lineTo(x, y);
      });
      ctx.lineTo(w, h - 5);
      ctx.closePath();
      ctx.fill();
    }
    ctx.strokeStyle = s.color;
    ctx.lineWidth = s.width || 1.5;
    ctx.beginPath();
    s.data.forEach((v, i) => {
      const x = (i / (n - 1)) * w;
      const y = h - 5 - ((v / max) * (h - 10));
      if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
    });
    ctx.stroke();
  });

  ctx.fillStyle = '#64748b';
  ctx.font = '10px JetBrains Mono, monospace';
  ctx.textAlign = 'right';
  ctx.fillText(fmt(max, 0), w - 4, 12);
}

function drawStackedChart(canvas, series) {
  const ctx = canvas.getContext('2d');
  const dpr = window.devicePixelRatio || 1;
  const w = canvas.clientWidth, h = canvas.clientHeight;
  if (canvas.width !== w * dpr) { canvas.width = w * dpr; canvas.height = h * dpr; }
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, w, h);

  const n = series[0].data.length;
  const stacked = new Array(n).fill(0).map((_, i) => series.reduce((sum, s) => sum + s.data[i], 0));
  const max = Math.max(...stacked, 10) * 1.15;

  ctx.strokeStyle = '#1a2230';
  ctx.lineWidth = 0.5;
  for (let i = 0; i <= 3; i++) {
    const y = (h - 10) * (i / 3) + 5;
    ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(w, y); ctx.stroke();
  }

  const running = new Array(n).fill(0);
  series.forEach(s => {
    ctx.fillStyle = s.color + 'cc';
    ctx.beginPath();
    for (let i = 0; i < n; i++) {
      const x = (i / (n - 1)) * w;
      const yTop = h - 5 - ((running[i] + s.data[i]) / max) * (h - 10);
      if (i === 0) ctx.moveTo(x, yTop); else ctx.lineTo(x, yTop);
    }
    for (let i = n - 1; i >= 0; i--) {
      const x = (i / (n - 1)) * w;
      const yBase = h - 5 - (running[i] / max) * (h - 10);
      ctx.lineTo(x, yBase);
    }
    ctx.closePath();
    ctx.fill();
    for (let i = 0; i < n; i++) running[i] += s.data[i];
  });

  ctx.font = '9px JetBrains Mono, monospace';
  ctx.textAlign = 'left';
  let lx = 6;
  series.forEach(s => {
    ctx.fillStyle = s.color; ctx.fillRect(lx, 6, 8, 8);
    ctx.fillStyle = '#94a3b8'; ctx.fillText(s.label, lx + 12, 13);
    lx += 80;
  });
}

function renderCharts() {
  drawLineChart(dom.charts.throughput, [{ data: state.series.throughput, color: '#38bdf8', width: 1.8 }], { filled: true });
  drawLineChart(dom.charts.latency, [
    { data: state.series.p50, color: '#4ade80', width: 1.2 },
    { data: state.series.p95, color: '#fbbf24', width: 1.5 },
    { data: state.series.p99, color: '#ef4444', width: 1.5 },
  ]);
  drawStackedChart(dom.charts.warehouse, [
    { data: state.series.whA, color: '#38bdf8', label: 'WHSE A' },
    { data: state.series.whB, color: '#a78bfa', label: 'WHSE B' },
    { data: state.series.whC, color: '#4ade80', label: 'WHSE C' },
  ]);
  drawLineChart(dom.charts.counters, [
    { data: state.series.dlq, color: '#fbbf24', width: 1.5 },
    { data: state.series.kafkaLag, color: '#ef4444', width: 1.2 },
    { data: state.series.retries, color: '#a78bfa', width: 1.2 },
  ]);
}

// ─── Controls ───────────────────────────────────────────────────────────────
function initControls() {
  dom.rateSlider.addEventListener('input', e => {
    state.traffic.ratePerMin = parseInt(e.target.value, 10);
    dom.rateValue.textContent = fmt(state.traffic.ratePerMin);
  });

  dom.trafficBtn.addEventListener('click', () => {
    state.traffic.active = !state.traffic.active;
    if (state.traffic.active) {
      dom.trafficBtn.textContent = 'STOP LOAD';
      dom.trafficBtn.classList.add('active');
      tickerPush(`LOAD STARTED · ${fmt(state.traffic.ratePerMin)} orders/min`, 'ok');
    } else {
      dom.trafficBtn.textContent = 'START LOAD';
      dom.trafficBtn.classList.remove('active');
      tickerPush('LOAD STOPPED');
    }
  });

  document.querySelectorAll('[data-pattern]').forEach(btn => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('[data-pattern]').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      state.traffic.pattern = btn.dataset.pattern;
      tickerPush(`PATTERN → ${btn.dataset.pattern.toUpperCase()}`);
    });
  });

  document.querySelectorAll('[data-inject]').forEach(btn => {
    btn.addEventListener('click', () => injectFailure(btn.dataset.inject));
  });

  dom.recoverAll.addEventListener('click', recoverAll);

  document.querySelectorAll('[data-strategy]').forEach(btn => {
    btn.addEventListener('click', () => {
      document.querySelectorAll('[data-strategy]').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      state.strategy = btn.dataset.strategy;
      dom.strategyHint.textContent = state.strategy === 'optimistic'
        ? 'CAS retry · higher throughput · retries under contention'
        : 'SETNX lock · serialized access · zero contention';
      tickerPush(`STRATEGY → ${state.strategy.toUpperCase()}`);
    });
  });

  $('#toggle-cb').addEventListener('change', e => {
    state.resilience.cb = e.target.checked;
    tickerPush(`CIRCUIT BREAKER ${e.target.checked ? 'ENABLED' : 'DISABLED'}`, e.target.checked ? 'ok' : 'warn');
  });
  $('#toggle-bulkhead').addEventListener('change', e => {
    state.resilience.bulkhead = e.target.checked;
    tickerPush(`BULKHEAD ${e.target.checked ? 'ENABLED' : 'DISABLED'}`, e.target.checked ? 'ok' : 'warn');
  });
  $('#toggle-retry').addEventListener('change', e => {
    state.resilience.retry = e.target.checked;
    tickerPush(`RETRY ${e.target.checked ? 'ENABLED' : 'DISABLED'}`, e.target.checked ? 'ok' : 'warn');
  });
  $('#toggle-failfast').addEventListener('change', e => {
    state.resilience.failfast = e.target.checked;
    tickerPush(`FAIL-FAST ${e.target.checked ? 'ENABLED' : 'DISABLED'}`, e.target.checked ? 'ok' : 'warn');
  });

  document.querySelectorAll('[data-experiment]').forEach(btn => {
    btn.addEventListener('click', () => runExperiment(btn.dataset.experiment));
  });
}

function injectFailure(which) {
  if (which.startsWith('warehouse-')) {
    const id = which.slice(-1);
    state.warehouses[id].status = 'failed';
    tickerPush(`⚡ FAILURE — WAREHOUSE ${id.toUpperCase()}`, 'crit');
    dom.sysMode.textContent = 'DEGRADED';
  } else if (which === 'slow-b') {
    state.warehouses.b.status = 'slow';
    tickerPush(`WAREHOUSE B → DEGRADED (slow)`, 'warn');
    dom.sysMode.textContent = 'DEGRADED';
  } else if (which === 'partition') {
    state.warehouses.b.status = 'failed';
    state.warehouses.c.status = 'slow';
    tickerPush(`NETWORK PARTITION — B ISOLATED, C SLOW`, 'crit');
    dom.sysMode.textContent = 'PARTITIONED';
  }
}

function recoverAll() {
  Object.values(state.warehouses).forEach(w => {
    w.status = 'ok'; w.cbFailures = 0; w.cbState = 'CLOSED';
  });
  tickerPush(`✓ ALL WAREHOUSES RESTORED`, 'ok');
  dom.sysMode.textContent = 'LIVE';
}

// ─── Experiment autopilot ───────────────────────────────────────────────────
const delay = ms => new Promise(r => setTimeout(r, ms));
let expRunning = false;

function runExperiment(id) {
  if (expRunning) { tickerPush('EXPERIMENT ALREADY RUNNING', 'warn'); return; }
  const scripts = {
    '1': scriptExp1, '2': scriptExp2, '3': scriptExp3,
    '4': scriptExp4, '5': scriptExp5, '6': scriptExp6, '7': scriptExp7,
  };
  const script = scripts[id];
  if (!script) return;
  expRunning = true;
  tickerPush(`▶ EXPERIMENT ${id} START`, 'ok');
  script().finally(() => {
    expRunning = false;
    tickerPush(`✓ EXPERIMENT ${id} COMPLETE`, 'ok');
  });
}

async function scriptExp1() {
  state.traffic.ratePerMin = 1000; dom.rateSlider.value = 1000; dom.rateValue.textContent = '1,000';
  if (!state.traffic.active) dom.trafficBtn.click();
  await delay(4000);
  state.traffic.ratePerMin = 3000; dom.rateSlider.value = 3000; dom.rateValue.textContent = '3,000';
  tickerPush(`EXP 1 — 3,000 ORDERS/MIN`);
  await delay(4000);
  state.traffic.ratePerMin = 5000; dom.rateSlider.value = 5000; dom.rateValue.textContent = '5,000';
  tickerPush(`EXP 1 — 5,000 ORDERS/MIN (4 TASKS CAP)`);
  await delay(5000);
  state.traffic.ratePerMin = 7000; dom.rateSlider.value = 7000; dom.rateValue.textContent = '7,000';
  tickerPush(`EXP 1 — 7,000 ORDERS/MIN · WATCH KAFKA LAG`, 'warn');
  await delay(5000);
}

async function scriptExp2() {
  if (!state.traffic.active) { state.traffic.ratePerMin = 3000; dom.rateValue.textContent = '3,000'; dom.trafficBtn.click(); }
  await delay(3000);
  injectFailure('warehouse-b');
  await delay(8000);
  recoverAll();
}

async function scriptExp3() {
  tickerPush(`EXP 3 — BURST 1000 ORDERS FOR SKU-HOTITEM (100 UNITS)`);
  const orig = state.warehouses.a.inventory;
  state.warehouses.a.inventory = 100;
  state.traffic.ratePerMin = 10000; dom.rateSlider.value = 10000; dom.rateValue.textContent = '10,000';
  if (!state.traffic.active) dom.trafficBtn.click();
  await delay(5000);
  state.warehouses.a.inventory = orig;
}

async function scriptExp4() {
  if (!state.traffic.active) dom.trafficBtn.click();
  await delay(2000);
  injectFailure('partition');
  await delay(6000);
  recoverAll();
}

async function scriptExp5() {
  tickerPush(`EXP 5 — 3 PARTITIONS · ADDING CONSUMERS (NO GAIN)`);
  CONFIG.routerTasks = 4; dom.routerTasks.textContent = '4 TASKS';
  state.traffic.ratePerMin = 5000; dom.rateSlider.value = 5000; dom.rateValue.textContent = '5,000';
  if (!state.traffic.active) dom.trafficBtn.click();
  await delay(4000);
  tickerPush(`EXP 5 — 8 CONSUMERS · ONLY 3 ACTIVE`, 'warn');
  CONFIG.routerTasks = 3; dom.routerTasks.textContent = '8 TASKS · 3 ACTIVE';
  await delay(5000);
  CONFIG.routerTasks = 4; dom.routerTasks.textContent = '4 TASKS';
}

async function scriptExp6() {
  tickerPush(`EXP 6 — BASELINE SKU-ALPHA TRAFFIC`);
  state.traffic.ratePerMin = 3000; dom.rateSlider.value = 3000; dom.rateValue.textContent = '3,000';
  if (!state.traffic.active) dom.trafficBtn.click();
  await delay(3000);
  tickerPush(`EXP 6 — NOISY HOTITEM BURST`, 'warn');
  state.traffic.pattern = 'burst';
  state.traffic.ratePerMin = 8000; dom.rateSlider.value = 8000; dom.rateValue.textContent = '8,000';
  await delay(5000);
  state.traffic.pattern = 'steady';
  state.traffic.ratePerMin = 3000; dom.rateSlider.value = 3000; dom.rateValue.textContent = '3,000';
}

async function scriptExp7() {
  tickerPush(`EXP 7 — COLD START · 2 → 4 TASKS`);
  CONFIG.routerTasks = 2; dom.routerTasks.textContent = '2 TASKS';
  if (!state.traffic.active) { state.traffic.ratePerMin = 3000; dom.rateValue.textContent = '3,000'; dom.trafficBtn.click(); }
  await delay(3000);
  tickerPush(`EXP 7 — NEW TASKS WARMING`, 'warn');
  CONFIG.routerTasks = 4; dom.routerTasks.textContent = '4 TASKS · 2 WARMING';
  await delay(4000);
  dom.routerTasks.textContent = '4 TASKS';
  tickerPush(`EXP 7 — ALL TASKS WARM`, 'ok');
}

function init() {
  initControls();
  tickerPush('WAREHOUSEFLOW OPERATIONS CONTROL INITIALIZED');
  tickerPush('3 WAREHOUSES · 60 PICKERS · 4 ROUTING TASKS');
  setInterval(simulateTick, CONFIG.tickMs);
  setInterval(() => {
    Object.values(state.warehouses).forEach(w => {
      if (w.status === 'ok' && w.inventory < 500) w.inventory = Math.min(2400, w.inventory + 200);
    });
  }, 5000);
}

if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
else init();

})();
