// js/views/home.js — Home dashboard renderer.
//
// Home is NOT a separate top-nav view. It is the empty-state render of
// the chat view: the router falls through to renderHome() whenever
// currentView === 'chat' && !selectedKey. The legacy name is
// renderHomeView(); we expose renderHome(container) as the module
// entry point and keep a window.renderHomeView bridge so the router
// fallback chain (js/core/router.js) and all remaining legacy callers
// in the inline <script> continue to work during the Phase 1 split.
//
// All helper functions that were defined alongside renderHomeView in
// the inline script (loadHomeStats, loadHomeActivity, loadHomeWiki,
// loadHomePatrolsAndApprovals, renderActivityFeed,
// renderHomePatrolWidgetContent, renderHomeApprovalWidgetContent,
// updateHomePatrolWidget, updateHomeApprovalWidget) are moved here
// verbatim. updateHomePatrolWidget / updateHomeApprovalWidget are
// invoked from the legacy patrols/approvals code, so they are bridged
// onto window; the content renderers and activity feed are bridged
// too for defensive symmetry.
//
// This module reads shared mutable state still owned by the legacy
// inline script (patrolsCache, patrolsStatsCache, approvalsCache,
// approvalsStatsCache, wvPages, allSessionsCache) via window.* —
// those bindings will migrate to the core state module in later tasks.

import { esc, escAttr, escJs } from '../core/html.js';

// ------- small inline helpers (from legacy) ------------------------

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

// ------- entry point ----------------------------------------------

export function renderHome(container) {
  const main = container || document.getElementById('main');
  const hour = new Date().getHours();
  const greeting = hour < 12 ? 'Good morning' : (hour < 18 ? 'Good afternoon' : 'Good evening');
  const dateStr = new Date().toLocaleDateString(undefined, { weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' });

  main.innerHTML =
    '<div class="home-view">' +
      '<div class="home-greeting">' + esc(greeting) + '</div>' +
      '<div class="home-date">' + esc(dateStr) + '</div>' +
      '<div class="home-stats">' +
        '<div class="stat-card"><div class="sc-label">Active Sessions</div><div class="sc-value" id="stat-sessions">--</div><div class="sc-sub">running now</div></div>' +
        '<div class="stat-card"><div class="sc-label">Active Patrols</div><div class="sc-value" id="stat-patrols">--</div><div class="sc-sub">monitoring</div></div>' +
        '<div class="stat-card"><div class="sc-label">Pending Approvals</div><div class="sc-value" id="stat-approvals" style="color:#c9d1d9">--</div><div class="sc-sub">awaiting review</div></div>' +
        '<div class="stat-card"><div class="sc-label">Wiki Pages</div><div class="sc-value" id="stat-wiki">--</div><div class="sc-sub">compiled</div></div>' +
        '<div class="stat-card"><div class="sc-label">Today\'s Cost</div><div class="sc-value" id="stat-cost">--</div><div class="sc-sub">total spend</div></div>' +
      '</div>' +
      '<div class="home-actions">' +
        '<button class="home-action" onclick="switchView(\'chat\',document.querySelector(\'[data-view=chat]\'));openNewSessionModal()"><span class="ha-icon">\uD83D\uDCAC</span> New Chat</button>' +
        '<button class="home-action" onclick="switchView(\'patrols\',document.querySelector(\'[data-view=patrols]\'))"><span class="ha-icon">\uD83D\uDEE1</span> Patrols</button>' +
        '<button class="home-action" onclick="switchView(\'approvals\',document.querySelector(\'[data-view=approvals]\'))"><span class="ha-icon">\uD83D\uDCE5</span> Approvals</button>' +
        '<button class="home-action" onclick="switchView(\'graph\',document.querySelector(\'[data-view=graph]\'))"><span class="ha-icon">\uD83D\uDD17</span> Graph</button>' +
      '</div>' +
      '<div class="home-section-title">Patrol Status <span class="home-widget-link" onclick="switchView(\'patrols\',document.querySelector(\'[data-view=patrols]\'))">View all</span></div>' +
      '<div class="home-widget"><div id="homePatrolWidget"><div style="padding:12px 14px;font-size:12px;color:#6e7681">Loading...</div></div></div>' +
      '<div class="home-section-title">Pending Approvals <span class="home-widget-link" onclick="switchView(\'approvals\',document.querySelector(\'[data-view=approvals]\'))">View all</span></div>' +
      '<div class="home-widget"><div id="homeApprovalWidget"><div style="padding:12px 14px;font-size:12px;color:#6e7681">Loading...</div></div></div>' +
      '<div class="home-section-title">Recent Activity</div>' +
      '<div class="home-feed" id="homeFeed"><div class="feed-item"><div class="feed-icon">...</div><div class="feed-body"><div class="feed-title">Loading...</div></div></div></div>' +
      '<div class="home-section-title">Recently Compiled</div>' +
      '<div class="home-wiki-list" id="homeWikiList"></div>' +
    '</div>';

  loadHomeStats();
  loadHomeActivity();
  loadHomeWiki();
  loadHomePatrolsAndApprovals();
}

async function loadHomeStats() {
  try {
    const [sessResp, patrolResp, approvalResp, wikiResp] = await Promise.all([
      fetch('/api/sessions', { headers: authHeaders() }).then(r => r.ok ? r.json() : null),
      fetch('/api/patrols', { headers: authHeaders() }).then(r => r.ok ? r.json() : null).catch(() => null),
      fetch('/api/approvals?status=pending', { headers: authHeaders() }).then(r => r.ok ? r.json() : null).catch(() => null),
      fetch('/api/wiki', { headers: authHeaders() }).then(r => r.ok ? r.json() : []).catch(() => [])
    ]);
    const sessions = sessResp && sessResp.sessions ? sessResp.sessions : [];
    const active = sessions.filter(s => s.state === 'running').length;
    const totalCost = sessions.reduce((sum, s) => sum + (s.total_cost || 0), 0);

    // Patrol stats
    if (patrolResp) {
      window.patrolsCache = patrolResp.patrols || [];
      window.patrolsStatsCache = patrolResp.stats || {};
    }
    const pStats = window.patrolsStatsCache || {};
    const activePatrols = (pStats.active || 0) + (pStats.running || 0);

    // Approval stats
    if (approvalResp) {
      window.approvalsCache = approvalResp.approvals || [];
      window.approvalsStatsCache = approvalResp.stats || {};
    }
    const aStats = window.approvalsStatsCache || {};
    const pendingApprovals = aStats.pending || 0;

    // Wiki pages count
    const wikiPages = Array.isArray(wikiResp) ? wikiResp : [];
    const wikiCount = wikiPages.length;

    const el = (id, v, style) => { const e = document.getElementById(id); if (e) { e.textContent = v; if (style) e.style.color = style; } };
    el('stat-sessions', active);
    el('stat-patrols', activePatrols);
    el('stat-approvals', pendingApprovals, pendingApprovals > 0 ? '#da3633' : '#c9d1d9');
    el('stat-wiki', wikiCount);
    el('stat-cost', '$' + totalCost.toFixed(2));

    if (typeof window.updateApprovalsBadge === 'function') window.updateApprovalsBadge();
  } catch (e) { console.error('loadHomeStats:', e); }
}

async function loadHomePatrolsAndApprovals() {
  // Fetch patrols and approvals if not already loaded
  const patrolsCache = window.patrolsCache;
  const approvalsCache = window.approvalsCache;
  await Promise.all([
    ((!patrolsCache || patrolsCache.length === 0) && typeof window.fetchPatrols === 'function')
      ? window.fetchPatrols().catch(function(){}) : Promise.resolve(),
    ((!approvalsCache || approvalsCache.length === 0) && typeof window.fetchApprovals === 'function')
      ? window.fetchApprovals('pending').catch(function(){}) : Promise.resolve()
  ]);
  var pc = document.getElementById('homePatrolWidget');
  if (pc) renderHomePatrolWidgetContent(pc);
  var ac = document.getElementById('homeApprovalWidget');
  if (ac) renderHomeApprovalWidgetContent(ac);
}

function loadHomeActivity() {
  renderActivityFeed();
}

function renderActivityFeed() {
  const feed = document.getElementById('homeFeed');
  if (!feed) return;

  // Collect recent events from allSessionsCache and other caches
  const activities = [];
  const allSessionsCache = window.allSessionsCache || [];
  const patrolsCache = window.patrolsCache || [];
  const wvPages = window.wvPages || [];
  const approvalsCache = window.approvalsCache || [];

  // Session events (created / running)
  allSessionsCache.forEach(s => {
    if (s.last_active) {
      const isRunning = s.state === 'running';
      activities.push({
        type: isRunning ? 'session_running' : 'session_created',
        icon: isRunning ? '\uD83D\uDFE2' : (s.source === 'terminal' ? '\uD83D\uDCBB' : '\uD83D\uDCAC'),
        title: (isRunning ? 'Running: ' : '') + (s.name || s.summary || s.last_prompt || s.key),
        meta: s.state || '',
        time: s.last_active,
        onclick: "switchView('chat',document.querySelector('[data-view=chat]'));selectSession('" + escJs(s.key) + "','" + escJs(s.node || 'local') + "')"
      });
    }
  });

  // Patrol completed events
  patrolsCache.forEach(p => {
    if (p.last_run) {
      const ts = typeof p.last_run === 'string' ? new Date(p.last_run).getTime() : p.last_run;
      activities.push({
        type: 'patrol_completed',
        icon: '\uD83D\uDEE1\uFE0F',
        title: 'Patrol completed: ' + (p.name || p.id || 'unknown'),
        meta: (p.state || 'done'),
        time: ts,
        onclick: "switchView('patrols',document.querySelector('[data-view=patrols]'))"
      });
    }
  });

  // Wiki compiled events (from wvPages if loaded)
  wvPages.forEach(p => {
    if (p.compiled_at) {
      activities.push({
        type: 'wiki_compiled',
        icon: '\uD83D\uDCD6',
        title: 'Wiki compiled: ' + (p.title || p.name),
        meta: (p.sources || 0) + ' sources',
        time: new Date(p.compiled_at).getTime(),
        onclick: "switchView('wiki',document.querySelector('[data-view=wiki]'));setTimeout(function(){loadWikiPage('" + escJs(p.name) + "')},100)"
      });
    }
  });

  // Approval events
  approvalsCache.forEach(a => {
    const ts = a.created_at ? (typeof a.created_at === 'string' ? new Date(a.created_at).getTime() : a.created_at) : 0;
    if (ts) {
      activities.push({
        type: 'approval_pending',
        icon: '\uD83D\uDCE5',
        title: 'Approval: ' + (a.action || a.summary || 'pending review'),
        meta: a.status || 'pending',
        time: ts,
        onclick: "switchView('approvals',document.querySelector('[data-view=approvals]'))"
      });
    }
  });

  // Sort by time descending, take last 10
  activities.sort((a, b) => (b.time || 0) - (a.time || 0));
  const recent = activities.slice(0, 10);

  if (recent.length === 0) {
    feed.innerHTML = '<div class="feed-item"><div class="feed-icon">-</div><div class="feed-body"><div class="feed-title" style="color:#6e7681">No recent activity</div></div></div>';
    return;
  }
  feed.innerHTML = recent.map(a => {
    const ago = a.time ? timeAgo(a.time) : '';
    return '<div class="feed-item" style="cursor:pointer" onclick="' + escAttr(a.onclick || '') + '">' +
      '<div class="feed-icon">' + a.icon + '</div>' +
      '<div class="feed-body"><div class="feed-title">' + esc((a.title || '').substring(0, 80)) + '</div>' +
      '<div class="feed-meta">' + esc(a.meta) + (ago ? ' \u00b7 ' + ago : '') + '</div></div></div>';
  }).join('');
}

async function loadHomeWiki() {
  const container = document.getElementById('homeWikiList');
  if (!container) return;
  try {
    const pages = await fetch('/api/wiki', { headers: authHeaders() }).then(r => r.ok ? r.json() : []).catch(() => []);
    const wikiPages = Array.isArray(pages) ? pages : [];
    if (wikiPages.length === 0) {
      container.innerHTML = '<div style="color:#6e7681;font-size:13px;grid-column:1/-1">No wiki pages yet. Run Ingest from the Wiki view.</div>';
      return;
    }
    const sorted = wikiPages.sort((a, b) => (b.mod_time || 0) - (a.mod_time || 0)).slice(0, 5);
    container.innerHTML = sorted.map(p =>
      '<div class="home-wiki-card" onclick="switchView(\'wiki\',document.querySelector(\'[data-view=wiki]\'));setTimeout(function(){loadWikiPage(\'' + escJs(p.name) + '\')},100)">' +
        '<div class="hwc-name">' + esc(p.title || p.name) + '</div>' +
        '<div class="hwc-meta">' + (p.compiled_at ? timeAgo(new Date(p.compiled_at).getTime()) : '') +
        (p.sources ? ' \u00b7 ' + p.sources + ' sources' : '') + '</div>' +
      '</div>'
    ).join('');
  } catch (e) { container.innerHTML = ''; }
}

// ------- Home widget content (called by patrol/approval events) ----

function updateHomePatrolWidget() {
  var container = document.getElementById('homePatrolWidget');
  if (!container) return;
  renderHomePatrolWidgetContent(container);
}

function updateHomeApprovalWidget() {
  var container = document.getElementById('homeApprovalWidget');
  if (!container) return;
  renderHomeApprovalWidgetContent(container);
}

function renderHomePatrolWidgetContent(container) {
  const patrolsCache = window.patrolsCache || [];
  if (patrolsCache.length === 0) {
    container.innerHTML = '<div style="padding:12px 14px;font-size:12px;color:#6e7681">No patrols configured</div>';
    return;
  }
  var nonDisabled = patrolsCache.filter(function(p) { return (p.state || '').toLowerCase() !== 'disabled'; });
  if (nonDisabled.length === 0) {
    container.innerHTML = '<div style="padding:12px 14px;font-size:12px;color:#6e7681">All patrols disabled</div>';
    return;
  }
  container.innerHTML = nonDisabled.slice(0, 6).map(function(p) {
    var stateClass = (p.state || 'active').toLowerCase();
    var lastTime = '';
    if (p.last_run && p.last_run.timestamp) {
      lastTime = timeAgo(typeof p.last_run.timestamp === 'number' ? p.last_run.timestamp : new Date(p.last_run.timestamp).getTime());
    }
    return '<div class="hw-patrol-row">' +
      '<span class="patrol-dot ' + stateClass + '" style="width:7px;height:7px"></span>' +
      '<span class="hw-patrol-name">' + esc(p.name) + '</span>' +
      '<span class="hw-patrol-state">' + esc(p.state || 'Active') + '</span>' +
      (lastTime ? '<span class="hw-patrol-time">' + esc(lastTime) + '</span>' : '') +
    '</div>';
  }).join('');
}

function renderHomeApprovalWidgetContent(container) {
  const approvalsCache = window.approvalsCache || [];
  var pending = approvalsCache.filter(function(a) { return (a.status || '').toLowerCase() === 'pending'; });
  if (pending.length === 0) {
    container.innerHTML = '<div class="hw-clear">&#10003; All clear</div>';
    return;
  }
  container.innerHTML = pending.slice(0, 3).map(function(a) {
    var urgencyClass = a.urgency === 'urgent' ? ' urgent' : '';
    var ts = a.created_at ? timeAgo(typeof a.created_at === 'number' ? a.created_at : new Date(a.created_at).getTime()) : '';
    return '<div class="hw-approval-card' + urgencyClass + '" onclick="switchView(\'approvals\',document.querySelector(\'[data-view=approvals]\'))">' +
      '<div class="hw-approval-info">' +
        '<div class="hw-approval-title">' + esc(a.action || a.summary || 'Approval') + '</div>' +
        '<div class="hw-approval-meta">' + esc(a.patrol || '') + (ts ? ' \u00b7 ' + esc(ts) : '') + '</div>' +
      '</div>' +
      '<button class="hw-approval-btn" onclick="event.stopPropagation();switchView(\'approvals\',document.querySelector(\'[data-view=approvals]\'))">Review</button>' +
    '</div>';
  }).join('');
}

// ------- legacy window.* bridges -----------------------------------
// These must stay as long as the inline <script> block (or any
// pre-module view) calls them as bare identifiers. Removed in Phase 2
// once every caller is a module.

if (typeof window !== 'undefined') {
  window.renderHomeView = renderHome;
  window.loadHomeStats = loadHomeStats;
  window.loadHomeActivity = loadHomeActivity;
  window.loadHomeWiki = loadHomeWiki;
  window.loadHomePatrolsAndApprovals = loadHomePatrolsAndApprovals;
  window.renderActivityFeed = renderActivityFeed;
  window.updateHomePatrolWidget = updateHomePatrolWidget;
  window.updateHomeApprovalWidget = updateHomeApprovalWidget;
  window.renderHomePatrolWidgetContent = renderHomePatrolWidgetContent;
  window.renderHomeApprovalWidgetContent = renderHomeApprovalWidgetContent;
}
