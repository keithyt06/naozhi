// js/views/patrols.js — Patrols view.
//
// Moved verbatim from dashboard.html's inline <script> (Task 17:
// Patrols View banner). Owns: fetchPatrols, renderPatrolsView,
// renderPatrolCards, patrol state/trigger/log actions, and the WS
// onPatrolEvent handler dispatched from ws.js.
//
// Shared globals (`patrolsCache`, `patrolsStatsCache`,
// `patrolRefreshTimer`, `currentView`) are still declared in the
// legacy inline <script>'s /* View Switching (Tasks 11-13) */
// block and reached via window/state bridges — leave them there
// until Phase 2 migrates them into state.js.

import { esc, escAttr, escJs } from '../core/html.js';

function authHeaders() {
  return (typeof window !== 'undefined' && typeof window.authHeaders === 'function')
    ? window.authHeaders() : {};
}

function timeAgo(ms, future) {
  if (typeof window !== 'undefined' && typeof window.timeAgo === 'function') {
    return window.timeAgo(ms, future);
  }
  return '';
}

function showToast(msg, type, duration) {
  if (typeof window !== 'undefined' && typeof window.showToast === 'function') {
    window.showToast(msg, type, duration);
  }
}

function addNotification(title, desc, urgency, sessionKey, sessionNode) {
  if (typeof window !== 'undefined' && typeof window.addNotification === 'function') {
    window.addNotification(title, desc, urgency, sessionKey, sessionNode);
  }
}

function updateHomePatrolWidget() {
  if (typeof window !== 'undefined' && typeof window.updateHomePatrolWidget === 'function') {
    window.updateHomePatrolWidget();
  }
}

// ------- entry points ---------------------------------------------

export async function fetchPatrols() {
  try {
    const resp = await fetch('/api/patrols', { headers: authHeaders() });
    if (!resp.ok) return;
    const data = await resp.json();
    patrolsCache = data.patrols || [];
    patrolsStatsCache = data.stats || {};
  } catch (e) { console.error('fetchPatrols:', e); }
}

export function renderPatrolsView() {
  const main = document.getElementById('main');
  main.innerHTML =
    '<div class="patrols-view" id="patrolsView">' +
      '<div class="patrols-header"><h2>Patrols</h2></div>' +
      '<div class="patrols-grid" id="patrolsGrid"><div style="color:#6e7681;font-size:13px;grid-column:1/-1;text-align:center;padding:40px 0">Loading patrols...</div></div>' +
    '</div>';
  fetchPatrols().then(() => renderPatrolCards());
  // Auto-refresh every 30s
  if (patrolRefreshTimer) clearInterval(patrolRefreshTimer);
  patrolRefreshTimer = setInterval(function() {
    if (currentView === 'patrols') fetchPatrols().then(() => renderPatrolCards());
    else { clearInterval(patrolRefreshTimer); patrolRefreshTimer = null; }
  }, 30000);
}

export function renderPatrolCards() {
  const grid = document.getElementById('patrolsGrid');
  if (!grid) return;
  if (patrolsCache.length === 0) {
    grid.innerHTML = '<div class="patrols-empty"><div class="pe-icon">&#128737;</div><div>No patrols configured. Add patrols in config.yaml.</div></div>';
    return;
  }
  grid.innerHTML = patrolsCache.map(function(p) {
    var stateClass = (p.state || 'active').toLowerCase();
    var stateLabel = p.state || 'Active';
    var desc = p.prompt ? p.prompt.substring(0, 120) : 'No description';
    var metaParts = [];
    if (p.schedule) metaParts.push('<span>&#128339; ' + esc(p.schedule) + '</span>');
    if (p.trigger) metaParts.push('<span>&#9889; ' + esc(p.trigger) + '</span>');
    if (p.agent) metaParts.push('<span>&#129302; ' + esc(p.agent) + '</span>');
    var lastRunText = '';
    if (p.last_run && p.last_run.timestamp) {
      lastRunText = timeAgo(typeof p.last_run.timestamp === 'number' ? p.last_run.timestamp : new Date(p.last_run.timestamp).getTime());
    }
    if (lastRunText) metaParts.push('<span>Last: ' + lastRunText + '</span>');

    var pauseLabel = stateClass === 'paused' ? 'Resume' : 'Pause';
    var canTrigger = stateClass === 'active';

    return '<div class="patrol-card" data-patrol="' + escAttr(p.name) + '">' +
      '<div class="patrol-card-header">' +
        '<span class="patrol-dot ' + stateClass + '"></span>' +
        '<span class="patrol-card-name">' + esc(p.name) + '</span>' +
        '<span class="patrol-state-badge ' + stateClass + '">' + esc(stateLabel) + '</span>' +
      '</div>' +
      '<div class="patrol-card-desc">' + esc(desc) + '</div>' +
      '<div class="patrol-card-meta">' + metaParts.join('') + '</div>' +
      '<div class="patrol-card-stats">' +
        '<span>Runs: <span class="ps-val">' + (p.total_runs || 0) + '</span></span>' +
        '<span>Errors: <span class="ps-val">' + (p.total_errors || 0) + '</span></span>' +
        '<span>Cost: <span class="ps-val">$' + (p.total_cost || 0).toFixed(2) + '</span></span>' +
      '</div>' +
      '<div class="patrol-card-actions">' +
        '<button class="patrol-action-btn" onclick="patrolToggleState(\'' + escJs(p.name) + '\',\'' + escJs(stateClass) + '\')">' + pauseLabel + '</button>' +
        (canTrigger ? '<button class="patrol-action-btn trigger" onclick="patrolTrigger(\'' + escJs(p.name) + '\')">Trigger</button>' : '') +
        '<button class="patrol-action-btn" onclick="patrolToggleLogs(\'' + escJs(p.name) + '\',this)">Logs</button>' +
      '</div>' +
      '<div class="patrol-logs-section" id="patrol-logs-' + escAttr(p.name) + '"></div>' +
    '</div>';
  }).join('');
}

export async function patrolToggleState(name, currentState) {
  var newState = currentState === 'paused' ? 'active' : 'paused';
  try {
    var resp = await fetch('/api/patrols/' + encodeURIComponent(name) + '/state', {
      method: 'PUT',
      headers: Object.assign({ 'Content-Type': 'application/json' }, authHeaders()),
      body: JSON.stringify({ state: newState })
    });
    if (!resp.ok) { var e = await resp.json().catch(function() { return {}; }); showToast(e.error || 'Failed to change state'); return; }
    await fetchPatrols();
    renderPatrolCards();
  } catch (e) { showToast('Network error'); }
}

export async function patrolTrigger(name) {
  try {
    var resp = await fetch('/api/patrols/' + encodeURIComponent(name) + '/trigger', {
      method: 'POST',
      headers: Object.assign({ 'Content-Type': 'application/json' }, authHeaders()),
      body: '{}'
    });
    if (!resp.ok) { var e = await resp.json().catch(function() { return {}; }); showToast(e.error || 'Failed to trigger'); return; }
    showToast('Patrol triggered: ' + name);
    // Refresh after short delay to show running state
    setTimeout(function() { fetchPatrols().then(function() { renderPatrolCards(); }); }, 1500);
  } catch (e) { showToast('Network error'); }
}

export async function patrolToggleLogs(name, btn) {
  var section = document.getElementById('patrol-logs-' + name);
  if (!section) return;
  if (section.classList.contains('open')) {
    section.classList.remove('open');
    return;
  }
  section.innerHTML = '<div style="padding:8px;font-size:11px;color:#6e7681">Loading logs...</div>';
  section.classList.add('open');
  try {
    var resp = await fetch('/api/patrols/' + encodeURIComponent(name) + '/logs?limit=5', { headers: authHeaders() });
    if (!resp.ok) { section.innerHTML = '<div style="padding:8px;font-size:11px;color:#6e7681">Failed to load logs</div>'; return; }
    var data = await resp.json();
    var logs = data.logs || [];
    if (logs.length === 0) {
      section.innerHTML = '<div style="padding:8px;font-size:11px;color:#6e7681">No execution logs yet</div>';
      return;
    }
    section.innerHTML = logs.map(function(l, idx) {
      var statusCls = (l.status || 'ok').toLowerCase();
      var ts = l.timestamp ? timeAgo(typeof l.timestamp === 'number' ? l.timestamp : new Date(l.timestamp).getTime()) : '';
      var dur = l.duration ? (l.duration / 1000).toFixed(1) + 's' : '';
      var summary = l.summary || l.error || '(no summary)';
      var detailId = 'plog-detail-' + name + '-' + idx;
      return '<div class="patrol-log-item" onclick="var d=document.getElementById(\'' + detailId + '\');if(d)d.classList.toggle(\'show\')">' +
        '<span class="patrol-log-status ' + statusCls + '"></span>' +
        '<span class="patrol-log-time">' + esc(ts) + '</span>' +
        '<span class="patrol-log-summary">' + esc(summary.substring(0, 100)) + '</span>' +
        '<span class="patrol-log-dur">' + esc(dur) + '</span>' +
      '</div>' +
      (l.detail ? '<div class="patrol-log-detail" id="' + detailId + '">' + esc(l.detail) + '</div>' : '');
    }).join('');
  } catch (e) { section.innerHTML = '<div style="padding:8px;font-size:11px;color:#6e7681">Error loading logs</div>'; }
}

export function onPatrolEvent(msg) {
  // Update cache in place if possible
  var found = false;
  for (var i = 0; i < patrolsCache.length; i++) {
    if (patrolsCache[i].name === msg.patrol) {
      patrolsCache[i].state = (msg.status === 'ok' || msg.status === 'warn' || msg.status === 'error') ? 'active' : (msg.status || 'active');
      if (msg.summary) patrolsCache[i].last_run = { summary: msg.summary, status: msg.status, timestamp: msg.time || Date.now() };
      found = true;
      break;
    }
  }
  if (currentView === 'patrols') renderPatrolCards();
  // Add notification
  var urgency = msg.status === 'error' ? 'urgent' : (msg.status === 'warn' ? 'warning' : 'info');
  addNotification('Patrol ' + (msg.status || 'done') + ': ' + (msg.patrol || ''), msg.summary || '', urgency, '_none', 'local');
  // Update home widgets if visible
  updateHomePatrolWidget();
}

// ------- Legacy bridges — removed after Phase 2 -------------------

if (typeof window !== 'undefined') {
  window.fetchPatrols = fetchPatrols;
  window.renderPatrolsView = renderPatrolsView;
  window.renderPatrolCards = renderPatrolCards;
  window.patrolToggleState = patrolToggleState;
  window.patrolTrigger = patrolTrigger;
  window.patrolToggleLogs = patrolToggleLogs;
  window.onPatrolEvent = onPatrolEvent;
}
