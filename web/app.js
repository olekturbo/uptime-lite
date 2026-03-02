'use strict';

// ══════════════════════════════════════════════════════════════════
//  API client
// ══════════════════════════════════════════════════════════════════

const BASE = '/api/v1';

async function apiFetch(path, opts = {}) {
  const res = await fetch(BASE + path, opts);
  if (res.status === 204) return null;
  const body = await res.json().catch(() => null);
  if (!res.ok) throw new Error(body?.error || res.statusText);
  return body;
}

const api = {
  listTargets:  ()          => apiFetch('/targets'),
  createTarget: (data)      => apiFetch('/targets', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  }),
  deleteTarget: (id)        => apiFetch(`/targets/${id}`, { method: 'DELETE' }),
  getStats:     (id)        => apiFetch(`/targets/${id}/stats`),
  getHistory:   (id, n=50)  => apiFetch(`/targets/${id}/history?limit=${n}`),

  // /status returns 404 when Redis has no entry yet — treat as null, not an error.
  getStatus: async (id) => {
    const res = await fetch(`${BASE}/targets/${id}/status`);
    if (res.status === 404) return null;
    if (!res.ok) throw new Error(res.statusText);
    return res.json();
  },
};

// ══════════════════════════════════════════════════════════════════
//  Application state
// ══════════════════════════════════════════════════════════════════

const state = {
  targets:  [],              // models.Target[]
  statuses: {},              // { [id]: models.TargetStatus | null }
  stats:    {},              // { [id]: { uptime_pct } | null }
  view:     'dashboard',     // 'dashboard' | 'detail'
  detailId: null,
  timer:    null,
  countdown: 30,
};

// ══════════════════════════════════════════════════════════════════
//  Utilities
// ══════════════════════════════════════════════════════════════════

function esc(str) {
  return String(str ?? '')
    .replace(/&/g, '&amp;').replace(/</g, '&lt;')
    .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function timeAgo(iso) {
  if (!iso) return '—';
  const s = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (s < 5)    return 'just now';
  if (s < 60)   return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

function latencyFmt(ms) {
  if (ms == null) return '—';
  if (ms === 0)   return '—';
  return ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`;
}

function trim(s, n = 42) {
  return s.length > n ? s.slice(0, n - 1) + '…' : s;
}

// ══════════════════════════════════════════════════════════════════
//  Toast
// ══════════════════════════════════════════════════════════════════

let toastTimer = null;
function toast(msg, type = 'success') {
  const el = document.getElementById('toast');
  el.textContent = msg;
  el.className = `toast toast-${type} show`;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => el.classList.remove('show'), 3200);
}

// ══════════════════════════════════════════════════════════════════
//  Routing (hash-based)
// ══════════════════════════════════════════════════════════════════

function navigate(hash) {
  history.pushState(null, '', hash || '#/');
  handleRoute();
}

function handleRoute() {
  const hash = location.hash;
  const m = hash.match(/^#\/targets\/(\d+)$/);
  if (m) {
    state.view = 'detail';
    state.detailId = parseInt(m[1], 10);
    renderDetail(state.detailId);
  } else {
    state.view = 'dashboard';
    state.detailId = null;
    renderDashboard();
  }
}

// ══════════════════════════════════════════════════════════════════
//  Dashboard
// ══════════════════════════════════════════════════════════════════

function renderDashboard() {
  document.getElementById('view-detail').classList.add('hidden');
  document.getElementById('view-dashboard').classList.remove('hidden');
  document.getElementById('back-btn').classList.add('hidden');
  document.getElementById('add-btn').classList.remove('hidden');
  document.getElementById('stats-bar').classList.remove('hidden');

  updateStatsBar();

  const grid = document.getElementById('view-dashboard');

  if (state.targets.length === 0) {
    grid.innerHTML = `
      <div class="grid">
        <div class="empty">
          <span class="empty-icon">⏱</span>
          <h3>No targets yet</h3>
          <p class="text-muted">Click <strong>+ Add Target</strong> to start monitoring.</p>
        </div>
      </div>`;
    return;
  }

  grid.innerHTML = `<div class="grid">${state.targets.map(cardHTML).join('')}</div>`;
}

function cardHTML(t) {
  const status  = state.statuses[t.id];
  const stats   = state.stats[t.id];
  const pending = status === undefined;   // not fetched yet
  const noData  = status === null;        // fetched, but 404 (no probe yet)

  let dotCls, badgeCls, badgeTxt, cardCls;
  if (pending || noData) {
    dotCls = 'dot-unknown'; badgeCls = 'badge-unknown';
    badgeTxt = noData ? 'No data' : '…';
    cardCls = 'card-unknown';
  } else if (status.up) {
    dotCls = 'dot-up'; badgeCls = 'badge-up'; badgeTxt = 'Up'; cardCls = 'card-up';
  } else {
    dotCls = 'dot-down'; badgeCls = 'badge-down'; badgeTxt = 'Down'; cardCls = 'card-down';
  }

  const latency   = status ? latencyFmt(status.latency_ms)  : '—';
  const checked   = status ? timeAgo(status.checked_at)     : '—';
  const uptime    = stats  ? `${stats.uptime_pct.toFixed(1)}%` : '—';

  return `
    <div class="card ${cardCls}">
      <div class="card-top">
        <div class="dot ${dotCls}"></div>
        <div class="card-info">
          <span class="card-name">${esc(t.name)}</span>
          <span class="card-url">${esc(trim(t.url))}</span>
        </div>
        <span class="badge ${badgeCls}">${badgeTxt}</span>
      </div>
      <div class="card-metrics">
        <div class="metric">
          <span class="metric-val">${latency}</span>
          <span class="metric-lbl">Latency</span>
        </div>
        <div class="metric">
          <span class="metric-val">${uptime}</span>
          <span class="metric-lbl">Uptime 24h</span>
        </div>
        <div class="metric">
          <span class="metric-val text-xs">${checked}</span>
          <span class="metric-lbl">Last check</span>
        </div>
      </div>
      <div class="card-footer">
        <button class="btn btn-ghost"   data-action="detail" data-id="${t.id}">View history</button>
        <button class="btn btn-danger"  data-action="delete" data-id="${t.id}" data-name="${esc(t.name)}">Delete</button>
      </div>
    </div>`;
}

function updateStatsBar() {
  const total = state.targets.length;
  const up    = state.targets.filter(t => state.statuses[t.id]?.up === true).length;
  const down  = state.targets.filter(t => state.statuses[t.id]?.up === false).length;
  document.getElementById('stat-total').textContent = total;
  document.getElementById('stat-up').textContent    = up;
  document.getElementById('stat-down').textContent  = down;
}

// ══════════════════════════════════════════════════════════════════
//  Detail view
// ══════════════════════════════════════════════════════════════════

let chart = null;

async function renderDetail(id) {
  document.getElementById('view-dashboard').classList.add('hidden');
  document.getElementById('view-detail').classList.remove('hidden');
  document.getElementById('back-btn').classList.remove('hidden');
  document.getElementById('add-btn').classList.add('hidden');
  document.getElementById('stats-bar').classList.add('hidden');

  const target = state.targets.find(t => t.id === id);
  if (!target) { navigate('#/'); return; }

  const el = document.getElementById('view-detail');
  el.innerHTML = `
    <div>
      <h2 class="detail-title">${esc(target.name)}</h2>
      <a class="detail-url" href="${esc(target.url)}" target="_blank" rel="noopener noreferrer">${esc(target.url)}</a>
    </div>
    <div id="detail-status" class="status-row">
      <span class="spinner"></span> Loading…
    </div>
    <div class="panel">
      <p class="panel-title">Latency — last 50 checks</p>
      <canvas id="latency-chart" height="90"></canvas>
      <p id="chart-empty" class="text-muted text-xs hidden" style="text-align:center;padding:.5rem 0">No data yet.</p>
    </div>
    <div class="panel">
      <p class="panel-title">Recent checks</p>
      <div id="history-body"><span class="spinner"></span> Loading…</div>
    </div>`;

  // Destroy old chart instance to prevent "Canvas already in use" warning.
  if (chart) { chart.destroy(); chart = null; }

  const [status, stats, historyRaw] = await Promise.all([
    api.getStatus(id),
    api.getStats(id).catch(() => null),
    api.getHistory(id, 50).catch(() => []),
  ]);

  // ── Status row ──────────────────────────────────────────────
  const statusEl = document.getElementById('detail-status');
  if (!status) {
    statusEl.innerHTML = `
      <div class="dot dot-unknown dot-lg"></div>
      <span class="text-muted">No probe data yet — check back soon.</span>`;
  } else {
    const upCls     = status.up ? 'dot-up'    : 'dot-down';
    const labelCls  = status.up ? 'text-green' : 'text-red';
    const label     = status.up ? 'Up'         : 'Down';
    const uptimeTxt = stats ? `${stats.uptime_pct.toFixed(2)}% uptime (24h)` : '';
    statusEl.innerHTML = `
      <div class="dot ${upCls} dot-lg"></div>
      <span class="${labelCls} status-row-text">${label}</span>
      <span class="sep">•</span>
      <span>${latencyFmt(status.latency_ms)}</span>
      ${uptimeTxt ? `<span class="sep">•</span><span class="text-muted">${uptimeTxt}</span>` : ''}
      <span class="sep">•</span>
      <span class="text-muted text-xs">Checked ${timeAgo(status.checked_at)}</span>`;
  }

  // ── Latency chart ────────────────────────────────────────────
  const history = historyRaw ?? [];
  if (history.length === 0) {
    document.getElementById('latency-chart').classList.add('hidden');
    document.getElementById('chart-empty').classList.remove('hidden');
  } else {
    const pts = [...history].reverse(); // oldest → newest
    chart = new Chart(document.getElementById('latency-chart'), {
      type: 'line',
      data: {
        labels: pts.map(r => timeAgo(r.checked_at)),
        datasets: [{
          label: 'Latency (ms)',
          data:  pts.map(r => r.up ? r.latency_ms : null),
          borderColor: '#6366f1',
          backgroundColor: 'rgba(99,102,241,0.08)',
          borderWidth: 2,
          pointRadius: pts.length > 20 ? 2 : 4,
          pointBackgroundColor: pts.map(r => r.up ? '#22c55e' : '#ef4444'),
          tension: 0.3,
          fill: true,
          spanGaps: false,
        }],
      },
      options: {
        responsive: true,
        plugins: {
          legend: { display: false },
          tooltip: {
            callbacks: { label: ctx => `${ctx.parsed.y ?? 0}ms` },
          },
        },
        scales: {
          x: {
            ticks: { color: '#64748b', maxTicksLimit: 8, maxRotation: 0 },
            grid:  { color: '#334155' },
          },
          y: {
            ticks: { color: '#64748b', callback: v => `${v}ms` },
            grid:  { color: '#334155' },
            beginAtZero: true,
          },
        },
      },
    });
  }

  // ── History table ─────────────────────────────────────────────
  const tbody = document.getElementById('history-body');
  if (history.length === 0) {
    tbody.innerHTML = '<p class="text-muted text-xs">No history available yet.</p>';
    return;
  }
  tbody.innerHTML = `
    <table class="history-table">
      <thead>
        <tr>
          <th>Time</th>
          <th>Status</th>
          <th>Latency</th>
          <th>Error</th>
        </tr>
      </thead>
      <tbody>
        ${history.map(r => `
          <tr>
            <td class="text-muted">${timeAgo(r.checked_at)}</td>
            <td>
              <span class="dot dot-inline ${r.up ? 'dot-up' : 'dot-down'}"></span>
              <span class="${r.up ? 'text-green' : 'text-red'}">
                ${r.up ? r.status_code : (r.status_code || '—')}
              </span>
            </td>
            <td>${latencyFmt(r.latency_ms)}</td>
            <td class="text-muted text-xs">${esc(r.error) || '—'}</td>
          </tr>`).join('')}
      </tbody>
    </table>`;
}

// ══════════════════════════════════════════════════════════════════
//  Modal
// ══════════════════════════════════════════════════════════════════

function openModal() {
  document.getElementById('overlay').classList.remove('hidden');
  document.getElementById('f-name').focus();
}

function closeModal() {
  document.getElementById('overlay').classList.add('hidden');
  document.getElementById('add-form').reset();
  document.getElementById('form-error').textContent = '';
}

async function handleAddSubmit(e) {
  e.preventDefault();
  const form   = e.target;
  const errEl  = document.getElementById('form-error');
  const btn    = document.getElementById('submit-btn');
  errEl.textContent = '';

  const data = {
    name:             form.name.value.trim(),
    url:              form.url.value.trim(),
    interval_seconds: parseInt(form.interval.value, 10) || 60,
    timeout_seconds:  parseInt(form.timeout.value,  10) || 10,
  };

  btn.disabled    = true;
  btn.textContent = 'Adding…';

  try {
    const created = await api.createTarget(data);
    state.targets.push(created);
    closeModal();
    renderDashboard();
    // Kick off a status fetch immediately for the new target.
    fetchTargetData(created.id).then(renderDashboard);
    toast(`"${created.name}" added!`);
  } catch (err) {
    errEl.textContent = err.message;
  } finally {
    btn.disabled    = false;
    btn.textContent = 'Add Target';
  }
}

// ══════════════════════════════════════════════════════════════════
//  Delete
// ══════════════════════════════════════════════════════════════════

async function handleDelete(id, name) {
  if (!confirm(`Delete "${name}"?\n\nAll probe history will be removed.`)) return;
  try {
    await api.deleteTarget(id);
    state.targets = state.targets.filter(t => t.id !== id);
    delete state.statuses[id];
    delete state.stats[id];
    toast(`"${name}" deleted.`);
    renderDashboard();
  } catch (err) {
    toast(`Delete failed: ${err.message}`, 'error');
  }
}

// ══════════════════════════════════════════════════════════════════
//  Data refresh
// ══════════════════════════════════════════════════════════════════

async function fetchTargetData(id) {
  const [st, stats] = await Promise.allSettled([
    api.getStatus(id),
    api.getStats(id),
  ]);
  state.statuses[id] = st.status    === 'fulfilled' ? st.value    : null;
  state.stats[id]    = stats.status === 'fulfilled' ? stats.value : null;
}

async function refreshAll() {
  try {
    const targets = await api.listTargets();
    state.targets = targets ?? [];
  } catch (err) {
    console.error('listTargets:', err);
    state.targets = [];
  }
  await Promise.all(state.targets.map(t => fetchTargetData(t.id)));
  if (state.view === 'dashboard') renderDashboard();
}

// ══════════════════════════════════════════════════════════════════
//  Countdown badge
// ══════════════════════════════════════════════════════════════════

function startCountdown() {
  const badge = document.getElementById('refresh-badge');
  state.countdown = 30;

  const tick = setInterval(() => {
    state.countdown--;
    badge.textContent = `Refresh in ${state.countdown}s`;
    if (state.countdown <= 0) {
      clearInterval(tick);
      badge.textContent = 'Refreshing…';
      refreshAll().then(startCountdown);
    }
  }, 1000);
}

// ══════════════════════════════════════════════════════════════════
//  Event delegation
// ══════════════════════════════════════════════════════════════════

document.addEventListener('click', e => {
  const btn = e.target.closest('[data-action]');
  if (!btn) return;
  const { action, id, name } = btn.dataset;
  if (action === 'detail') navigate(`#/targets/${id}`);
  if (action === 'delete') handleDelete(parseInt(id, 10), name);
});

// ══════════════════════════════════════════════════════════════════
//  Boot
// ══════════════════════════════════════════════════════════════════

document.addEventListener('DOMContentLoaded', async () => {
  // Header buttons
  document.getElementById('add-btn').addEventListener('click', openModal);
  document.getElementById('back-btn').addEventListener('click', () => navigate('#/'));
  document.getElementById('modal-close').addEventListener('click', closeModal);
  document.getElementById('overlay').addEventListener('click', e => {
    if (e.target === e.currentTarget) closeModal();
  });
  document.getElementById('add-form').addEventListener('submit', handleAddSubmit);

  // Keyboard: Esc closes modal
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') closeModal();
  });

  // Hash routing
  window.addEventListener('popstate', handleRoute);

  // Initial load
  document.getElementById('refresh-badge').textContent = 'Loading…';
  await refreshAll();
  handleRoute();
  startCountdown();
});
