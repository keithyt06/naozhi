// js/views/chat.js — chat view module.
//
// Owns the chat experience: sidebar session list, time-grouped card
// rendering, history popover, sidebar status bar, main shell (transcript
// + composer), WebSocket send flow, tool/banner rendering, turn-state
// machine, voice hold-to-talk, file attachments, auth modal, "new
// session" flows, rename/pin, sidebar resizer. Moved verbatim from the
// pre-split dashboard.html (many separate banner blocks merged).
//
// The router (js/core/router.js) falls through to `renderHomeView()` or
// `renderMainShell()` via the window bridges we install below. We also
// import home.js directly so the empty-state render can call
// `renderHome(container)` without relying on the fallback chain (the
// legacy `if (!selectedKey) renderHomeView()` branch inside
// `renderMainShell` is now a direct ES import).
//
// Every helper that the legacy inline <script> or `onclick="..."` HTML
// attributes reference is bridged onto `window` at the bottom. Phase 2
// will drop the bridges once each consumer is a module.
//
// Shared mutable state (selectedKey, selectedNode, sessionsData, …) is
// read/written via window — state.js exposes every field as a
// getter/setter so bare-identifier assignments still mutate the central
// store.

import { esc, escAttr, escJs } from '../core/html.js';
import { renderHome } from './home.js';

// -------------------------------------------------------------------
// Module-local stores that pre-split lived as bare globals in the
// inline <script>. Bridged to window at the bottom so legacy callers
// (including themselves, during the migration) resolve them the same
// way the old `const sessionWorkspaces = {}` did.
// -------------------------------------------------------------------
const sessionWorkspaces = {};
const sessionNodes = {};
const sessionDrafts = {};

/* ===== Session list: render + fetch ================================ */

// Render session list with time grouping (called by renderSidebar and filterByTag)
export function renderSessionList() {
  const list = document.getElementById('session-list');
  if (!list) return;
  const scrollTop = list.scrollTop;

  // Apply tag filter
  const filtered = allSessionsCache.filter(s => matchesTagFilter(s));

  // Time grouping definitions
  const groups = [
    { key: 'pinned', label: '\uD83D\uDCCC Pinned', test: s => s.pinned },
    { key: 'today', label: 'Today', test: s => isToday(s) && !s.pinned && s.source !== 'terminal' },
    { key: 'discovered', label: '\uD83D\uDCBB Discovered', test: s => s.source === 'terminal' && !s.pinned },
    { key: 'yesterday', label: 'Yesterday', test: s => isYesterday(s) && !s.pinned && s.source !== 'terminal' },
    { key: 'earlier', label: 'Earlier', test: s => true }
  ];

  // Sort: running > ready > new > suspended, then by last_active desc
  const order = {running: 0, ready: 1, new: 2, suspended: 3};
  filtered.sort((a, b) => {
    const d = (order[a.state] ?? 9) - (order[b.state] ?? 9);
    return d !== 0 ? d : (b.last_active || 0) - (a.last_active || 0);
  });

  // Distribute sessions into groups (each session goes to the first matching group)
  const buckets = {};
  groups.forEach(g => { buckets[g.key] = []; });
  const placed = new Set();
  groups.forEach(g => {
    filtered.forEach(s => {
      if (!placed.has(s.key) && g.test(s)) {
        buckets[g.key].push(s);
        placed.add(s.key);
      }
    });
  });

  let html = '';
  groups.forEach(g => {
    if (buckets[g.key].length === 0) return;
    html += '<div class="section-header">' + g.label + '</div>';
    const isEarlier = g.key === 'earlier';
    if (isEarlier) html += '<div class="time-group-earlier">';
    buckets[g.key].forEach(s => { html += sessionCardHtml(s); });
    if (isEarlier) html += '</div>';
  });

  if (!html) html = '<div class="no-sessions">no sessions</div>';
  list.innerHTML = html;
  list.scrollTop = scrollTop;
}

export function removePendingSession(key) {
  delete sessionWorkspaces[key];
  delete sessionNodes[key];
}

export async function fetchSessions() {
  try {
    const headers = {};
    const t = getToken();
    if (t) headers['Authorization'] = 'Bearer ' + t;
    const r = await fetch('/api/sessions', { headers });
    if (r.status === 401 || r.status === 403) {
      if (!document.querySelector('.modal-overlay')) showAuthModal();
      return;
    }
    if (!r.ok) return;
    const data = await r.json();
    // Use server-side version counter for efficient change detection.
    // Falls back to JSON comparison for nodes/history which lack a version.
    const version = (data.stats && data.stats.version) || 0;
    const nodesHash = JSON.stringify(data.nodes || {});
    const historyHash = JSON.stringify(data.history_sessions || []);
    if (version === lastVersion && version > 0 && nodesHash === lastNodesJSON && historyHash === lastHistoryJSON) return;
    lastVersion = version;
    lastNodesJSON = nodesHash;
    lastHistoryJSON = historyHash;
    if (data.nodes) nodesData = data.nodes;
    if (data.stats.agents) availableAgents = data.stats.agents;
    if (data.stats.default_workspace) defaultWorkspace = data.stats.default_workspace;
    if (data.stats.projects) projectsData = data.stats.projects;
    historySessionsData = data.history_sessions || [];

    // Track which keys the backend knows about
    const backendKeys = new Set();
    (data.sessions || []).forEach(s => {
      const n = s.node || 'local';
      sessionsData[sid(s.key, n)] = s;
      backendKeys.add(s.key);
    });

    // Remove pending sessions that now exist in backend
    for (const key of Object.keys(sessionWorkspaces)) {
      if (backendKeys.has(key)) {
        delete sessionWorkspaces[key];
        delete sessionNodes[key];
      }
    }

    // Merge pending dashboard sessions into data for sidebar rendering
    const pendingKeys = Object.keys(sessionWorkspaces);
    if (pendingKeys.length > 0) {
      if (!data.sessions) data.sessions = [];
      for (const key of pendingKeys) {
        if (!backendKeys.has(key)) {
          const parts = key.split(':');
          data.sessions.push({
            key: key,
            state: 'new',
            platform: parts[0] || 'dashboard',
            agent: 'general',
            workspace: sessionWorkspaces[key],
            last_active: 0,
            last_prompt: '',
            node: sessionNodes[key] || 'local',
            project: matchProject(sessionWorkspaces[key]),
          });
        }
      }
    }

    renderSidebar(data);
  } catch (e) {
    console.error('fetchSessions:', e);
  }
}

// Debounced variant: coalesces multiple calls within 300ms into a single fetch.
// Returns a Promise that resolves after the actual fetch completes.
let _fetchDbTimer = null;
let _fetchDbResolvers = [];
export function debouncedFetchSessions() {
  return new Promise(resolve => {
    _fetchDbResolvers.push(resolve);
    if (_fetchDbTimer) clearTimeout(_fetchDbTimer);
    _fetchDbTimer = setTimeout(() => {
      _fetchDbTimer = null;
      const resolvers = _fetchDbResolvers;
      _fetchDbResolvers = [];
      fetchSessions().then(() => resolvers.forEach(r => r()));
    }, 300);
  });
}

export function renderSidebar(data) {
  const st = data.stats;
  localWsInfo = { name: st.workspace_name || st.workspace_id || '', sys: '' };
  if (st.system) {
    const sys = st.system;
    let memStr = sys.memory_mb > 0 ? (sys.memory_mb >= 1024 ? (sys.memory_mb / 1024).toFixed(1) + 'G' : sys.memory_mb + 'M') : '';
    const ipStr = sys.ips && sys.ips.length > 0 ? sys.ips.join(', ') : '';
    localWsInfo.sys = sys.os + '/' + sys.arch + ' \u00b7 ' + sys.cpus + 'C' + (memStr ? '/' + memStr : '') + (ipStr ? ' \u00b7 ' + ipStr : '');
  }
  updateStatusBar();
  if (st.agents) availableAgents = st.agents;
  if (st.default_workspace) defaultWorkspace = st.default_workspace;
  if (st.projects) projectsData = st.projects;

  // Merge discovered into sessions — tag them as source=terminal
  const allItems = (data.sessions || []).map(s => {
    if (!s.source) s.source = 'managed';
    return s;
  });
  discoveredItems.forEach(d => {
    allItems.push({
      key: '_discovered:' + d.pid,
      state: d.state || 'ready',
      cli_name: d.cli_name || 'cli',
      last_active: d.last_active || d.started_at,
      last_prompt: d.last_prompt || d.summary || '',
      workspace: d.cwd,
      project: d.project || matchProject(d.cwd),
      node: d.node || 'local',
      source: 'terminal',
      _discovered: d,
    });
  });

  // Workspace sidebar: managed + discovered only (no filesystem recent sessions).
  // Suspended sessions stay in the workspace (with distinct styling).
  allSessionsCache = allItems;

  // Delegate list rendering to renderSessionList() which handles time grouping + tag filtering
  renderSessionList();

  // Update history badge (filesystem history sessions, deduplicated against workspace)
  const hBadge = document.getElementById('history-badge');
  if (hBadge) {
    const workspaceIDs = new Set(allSessionsCache.filter(s => s.session_id).map(s => s.session_id));
    const historyCount = historySessionsData.filter(r => !workspaceIDs.has(r.session_id)).length;
    hBadge.textContent = historyCount > 0 ? historyCount : '';
    hBadge.style.display = historyCount > 0 ? '' : 'none';
  }
}

// Match a workspace path to a project from projectsData (longest prefix wins)
export function matchProject(workspace) {
  if (!workspace || !projectsData || projectsData.length === 0) return '';
  const ws = workspace.endsWith('/') ? workspace : workspace + '/';
  let best = '', bestLen = 0;
  for (const p of projectsData) {
    const prefix = p.path.endsWith('/') ? p.path : p.path + '/';
    if (ws.startsWith(prefix) && p.path.length > bestLen) {
      best = p.name; bestLen = p.path.length;
    }
  }
  return best;
}

/* ===== History popover ============================================= */

let activePopover = null;

export function closeHistoryPopover() {
  if (activePopover) { activePopover.remove(); activePopover = null; }
}

document.addEventListener('click', function(e) {
  if (activePopover && !activePopover.contains(e.target) && !e.target.closest('#btn-history')) {
    closeHistoryPopover();
  }
});

export function toggleHistory() {
  if (activePopover) { closeHistoryPopover(); return; }

  // Show all filesystem history sessions, deduplicated against workspace.
  const workspaceIDs = new Set(
    allSessionsCache.filter(s => s.session_id).map(s => s.session_id)
  );
  const merged = historySessionsData
    .filter(r => !workspaceIDs.has(r.session_id))
    .map(r => ({
      key: '_history:' + r.session_id, node: 'local', source: 'recent',
      session_id: r.session_id, last_active: r.last_active || 0,
      prompt: r.last_prompt || r.summary || '',
      project: r.project || matchProject(r.workspace), tool: '',
    }));
  merged.sort((a, b) => b.last_active - a.last_active);

  let itemsHtml;
  if (merged.length === 0) {
    itemsHtml = '<div class="history-popover-empty">no history</div>';
  } else {
    // Group by day.
    let currentDay = '';
    itemsHtml = merged.map(s => {
      let dayHeader = '';
      if (s.last_active) {
        const d = new Date(s.last_active);
        const dayStr = d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', weekday: 'short' });
        if (dayStr !== currentDay) {
          currentDay = dayStr;
          dayHeader = '<div class="hp-day-header">' + esc(dayStr) + '</div>';
        }
      }
      const ago = s.last_active ? timeAgo(s.last_active) : '';
      const onclick = 'resumeRecentSession(this.dataset.sid);closeHistoryPopover()';
      return dayHeader +
        '<div class="history-popover-item" data-sid="' + escAttr(s.session_id) + '" onclick="' + onclick + '">' +
        (s.prompt ? '<div class="hp-prompt" title="' + escAttr(s.prompt) + '">' + esc(s.prompt) + '</div>' : '<div class="hp-prompt" style="color:#6e7681">(no prompt)</div>') +
        '<div class="hp-meta">' +
          (s.project ? '<span class="hp-project">' + esc(s.project) + '</span><span class="hp-dot">&middot;</span>' : '') +
          (ago ? '<span>' + ago + '</span>' : '') +
        '</div>' +
        '</div>';
    }).join('');
  }

  const popover = document.createElement('div');
  popover.className = isMobile() ? 'history-sheet' : 'history-popover';
  popover.innerHTML = '<div class="history-popover-header">History (' + merged.length + ')</div>' + itemsHtml;
  if (isMobile()) {
    popover.innerHTML = '<div class="sheet-handle"></div>' + popover.innerHTML;
  }
  activePopover = popover;
  document.body.appendChild(popover);

  if (!isMobile()) {
    const btn = document.getElementById('btn-history');
    const rect = btn.getBoundingClientRect();
    popover.style.position = 'fixed';
    popover.style.top = (rect.bottom + 4) + 'px';
    popover.style.right = (window.innerWidth - rect.right) + 'px';
    popover.style.maxHeight = Math.min(500, window.innerHeight - rect.bottom - 16) + 'px';
  }
}

/* ===== Session card + resume recent =============================== */

export function sessionCardHtml(s) {
  const sNode = s.node || 'local';
  const isActive = selectedKey === s.key && selectedNode === sNode;
  const isNew = s.state === 'new';
  const isSuspended = s.state === 'suspended';
  const cls = 'session-card' + (isActive ? ' active' : '') + (isNew ? ' new-card' : '') +
    (isSuspended ? ' suspended-card' : '');

  // Line 1: prompt (prefer name if set, also check pending names for new sessions)
  const pendingName = pendingSessionNames[s.key];
  const effectiveName = pendingName || (s.name && s.name.trim() ? s.name : '');
  const hasName = !!effectiveName;
  const prompt = hasName ? effectiveName : (s.summary || s.last_prompt || (isNew ? '(new session)' : '(no prompt)'));
  const icon = cliIcon(s.cli_name || 'cli');

  // Line 2: status dot + meta
  const dotCls = s.state === 'running' ? 'dot-running' : (s.state === 'ready' ? 'dot-ready' : (s.state === 'suspended' ? 'dot-suspended' : 'dot-new'));
  const ago = s.last_active ? timeAgo(s.last_active) : '';
  const nodeBadge = isMultiNode() && sNode !== 'local'
    ? '<span class="sc-node" style="background:' + nodeColor(sNode) + '">' + esc(sNode) + '</span>' : '';

  const dismissBtn = '<button class="btn-dismiss" data-key="' + escAttr(s.key) + '" data-node="' + escAttr(sNode) + '" onclick="event.stopPropagation();dismissSession(this.dataset.key,this.dataset.node)" title="remove">&times;</button>';

  const metaHtml = '<span class="sc-dot ' + dotCls + '"></span>' +
    '<span>' + esc(s.state) + '</span>' +
    nodeBadge;

  return '<div class="' + cls + '" data-key="' + escAttr(s.key) + '" data-node="' + escAttr(sNode) + '" onclick="selectSession(this.dataset.key,this.dataset.node)">' +
    dismissBtn +
    icon +
    '<div class="sc-body">' +
      '<div class="sc-header">' +
        (s.pinned ? '<span class="sc-pin" title="取消置顶" onclick="event.stopPropagation();togglePin(\'' + escAttr(s.key) + '\',\'' + escAttr(sNode) + '\',false)">📌</span>' :
          '<span class="sc-pin sc-pin-hover" title="置顶" onclick="event.stopPropagation();togglePin(\'' + escAttr(s.key) + '\',\'' + escAttr(sNode) + '\',true)">📌</span>') +
        '<div class="sc-prompt" title="' + escAttr(prompt) + '"' + (hasName ? '' : ' style="color:#8b949e"') + '>' + esc(prompt) + '</div>' +
        '<span class="sc-edit" title="重命名" onclick="event.stopPropagation();startRename(\'' + escAttr(s.key) + '\',\'' + escAttr(sNode) + '\')">✏️</span>' +
        (ago ? '<span class="sc-time">' + ago + '</span>' : '') +
      '</div>' +
      '<div class="sc-meta">' + metaHtml + '</div>' +
    '</div>' +
  '</div>';
}

export function resumeRecentSession(sessionId) {
  const found = historySessionsData.find(r => r.session_id === sessionId);
  resumeRecentById(sessionId, found ? found.workspace : '', found ? (found.last_prompt || found.summary || '') : '');
}

export async function resumeRecentById(sessionId, workspace, lastPrompt) {
  // Guard: if already resuming this session, find the managed key and select it
  for (const s of allSessionsCache) {
    if (s.session_id === sessionId) { selectSession(s.key, s.node || 'local'); return; }
  }

  try {
    const headers = {'Content-Type': 'application/json'};
    const token = getToken();
    if (token) headers['Authorization'] = 'Bearer ' + token;
    const r = await fetch('/api/sessions/resume', {
      method: 'POST', headers,
      body: JSON.stringify({session_id: sessionId, workspace: workspace || '', last_prompt: lastPrompt || ''})
    });
    if (!r.ok) { showToast('resume failed'); return; }
    const data = await r.json();
    const key = data.key;
    if (!key) return;

    // Force sidebar refresh to pick up the new suspended entry
    lastVersion = 0;
    await fetchSessions();

    selectSession(key, 'local');
    previewRecentSession(key, sessionId);
  } catch (e) {
    showToast('resume error: ' + e.message);
  }
}

export async function previewRecentSession(expectedKey, sessionId) {
  try {
    const headers = {};
    const token = getToken();
    if (token) headers['Authorization'] = 'Bearer ' + token;
    const r = await fetch('/api/discovered/preview?session_id=' + encodeURIComponent(sessionId), { headers });
    if (!r.ok) return;
    if (selectedKey !== expectedKey) return; // user navigated away
    const entries = await r.json();
    if (!entries || entries.length === 0) return;
    renderEvents(entries);
  } catch (e) {
    console.error('previewRecentSession:', e);
  }
}

/* ===== Sidebar status bar ========================================= */

const STATUS_LABELS = { off: 'offline', connecting: 'connecting...', authenticating: 'authenticating...', connected: 'connected', disconnected: 'polling', disconnected_retry: 'reconnecting...' };
const REMOTE_LABELS = { ok: 'connected', error: 'error', offline: 'offline', unreachable: 'unreachable' };
const VALID_DOT_CLASSES = { ok: 'ok', error: 'error', offline: 'offline', connecting: 'connecting', off: 'off', connected: 'connected', disconnected: 'disconnected', authenticating: 'authenticating' };

export function updateStatusBar() {
  const container = document.getElementById('sidebar-status');
  if (!container) return;
  const wsUp = wsm.state === WS_STATES.CONNECTED;

  // Local node row (always first)
  const localName = localWsInfo.name || 'workspace';
  // Distinguish short reconnect vs stable polling mode
  const statusKey = (wsm.state === WS_STATES.DISCONNECTED && wsm.backoff > 8000) ? 'disconnected' : (wsm.state === WS_STATES.DISCONNECTED ? 'disconnected_retry' : wsm.state);
  const localLabel = localName + ' \u00b7 ' + (STATUS_LABELS[statusKey] || wsm.state);
  const dotKey = statusKey === 'disconnected' ? 'connecting' : wsm.state; // polling = yellow dot
  const localSys = localWsInfo.sys || '';

  let html = '<div class="status-row">' +
    '<span class="status-dot ' + (VALID_DOT_CLASSES[dotKey] || 'off') + '"></span>' +
    '<div class="status-info">' +
      '<div class="status-ws">' + esc(localLabel) + '</div>' +
      (localSys ? '<div class="status-sys">' + esc(localSys) + '</div>' : '') +
    '</div></div>';

  // Remote node rows (from last known nodesData)
  const nodeIds = Object.keys(nodesData).filter(id => id !== 'local').sort();
  for (const id of nodeIds) {
    const nd = nodesData[id];
    const name = (nd.display_name || id);
    // When local WS is down, remote status is unknown — show as unreachable
    const status = wsUp ? (nd.status || 'offline') : 'unreachable';
    const dotCls = VALID_DOT_CLASSES[status] || 'offline';
    const label = REMOTE_LABELS[status] || status;
    const addr = nd.remote_addr || '';

    html += '<div class="status-row">' +
      '<span class="status-dot ' + dotCls + '"></span>' +
      '<div class="status-info">' +
        '<div class="status-ws">' + esc(name) + ' \u00b7 ' + esc(label) + '</div>' +
        (addr ? '<div class="status-sys">' + esc(addr) + '</div>' : '') +
      '</div></div>';
  }

  container.innerHTML = html;
}

/* ===== Select + dismiss session =================================== */

export function selectSession(key, node) {
  node = node || 'local';
  // Ensure we're in chat view when selecting a session
  if (currentView !== 'chat') {
    currentView = 'chat';
    document.querySelectorAll('#viewBar .vb-btn').forEach(b => b.classList.toggle('active', b.dataset.view === 'chat'));
    var tagFilter = document.getElementById('tagFilter');
    var sessionList = document.getElementById('session-list');
    if (tagFilter) tagFilter.style.display = '';
    if (sessionList) sessionList.style.display = '';
  }
  resetTurnState();
  // Recent session card click → trigger resume flow
  // Discovered session card click → trigger preview flow
  // Save draft for current session before switching
  if (selectedKey) {
    const inp = document.getElementById('msg-input');
    const draft = getMsgValue(inp);
    if (draft) sessionDrafts[selectedKey] = draft;
    else delete sessionDrafts[selectedKey];
  }
  if (key.startsWith('_discovered:')) {
    const pid = parseInt(key.split(':')[1]);
    const d = discoveredItems.find(x => x.pid === pid);
    if (d) {
      previewDiscovered(d.session_id, d.cwd, d.pid, d.proc_start_time || 0, d.node || '');
      return;
    }
  }
  pendingDiscovered = null;
  const prevKey = selectedKey;
  const prevNode = selectedNode;
  selectedKey = key;
  selectedNode = node;
  lastEventTime = 0;
  lastRenderedEventTime = 0;
  mobileEnterChat();
  stopPreviewPolling();
  document.querySelectorAll('.session-card').forEach(el => {
    el.classList.toggle('active', el.dataset.key === key && (el.dataset.node || 'local') === node);
  });
  renderMainShell();
  // Restore draft for the target session
  const draftInput = document.getElementById('msg-input');
  if (draftInput && sessionDrafts[key]) {
    setMsgValue(draftInput, sessionDrafts[key]);
  }

  const changed = prevKey !== key || prevNode !== node;
  if (wsm.isConnected()) {
    if (changed) wsm.unsubscribe();
    wsm.lastEventTimeWs = 0;
    wsm.subscribe(key, node);
    if (eventTimer) { clearInterval(eventTimer); eventTimer = null; }
  } else {
    fetchEvents(true);
    if (eventTimer) clearInterval(eventTimer);
    eventTimer = setInterval(() => fetchEvents(false), 1000);
  }
  // Refresh context panel when session changes
  if (ctxPanelOpen && changed) refreshCtxPanel();
}

export async function dismissSession(key, node) {
  node = node || 'local';
  delete sessionDrafts[key];

  // If it's a pending (never-sent) session, just remove from localStorage
  if (sessionWorkspaces[key] !== undefined) {
    removePendingSession(key);
    delete sessionsData[sid(key, node)];
    if (selectedKey === key) {
      selectedKey = null;
      renderHome(document.getElementById('main'));
    }
    lastVersion = 0;
    debouncedFetchSessions();
    return;
  }

  try {
    const headers = {'Content-Type': 'application/json'};
    const token = getToken();
    if (token) headers['Authorization'] = 'Bearer ' + token;
    const r = await fetch('/api/sessions', {method: 'DELETE', headers, body: JSON.stringify({key: key})});
    if (!r.ok && r.status !== 404) {
      const text = await r.text().catch(() => r.status);
      showToast('remove failed: ' + text);
      return;
    }
    delete sessionsData[sid(key, node)];
    if (selectedKey === key) {
      selectedKey = null;
      if (wsm.subscribedKey === key) wsm.unsubscribe();
      renderHome(document.getElementById('main'));
    }
    lastVersion = 0;
    debouncedFetchSessions();
  } catch (e) { showToast('remove error: ' + e.message); }
}

/* ===== Main shell (header + events + composer) =================== */

export function renderMainShell() {
  const main = document.getElementById('main');
  // Empty-state: no session selected → render the Home view directly.
  // Pre-split this was a window.renderHomeView fallback chain; chat.js
  // now depends on home.js as an ES module and calls renderHome().
  if (selectedKey == null) { renderHome(main); return; }

  const s = sessionsData[sid(selectedKey, selectedNode)] || {};

  const keyParts = (selectedKey || '').split(':');
  const agentIsGeneric = !s.agent || s.agent === 'general';
  // Primary title: prefer pending name, then server name, then prompt.
  const displayName = pendingSessionNames[selectedKey] || s.name || s.summary || s.last_prompt || (agentIsGeneric ? '' : s.agent) || keyParts[keyParts.length - 1] || selectedKey || '';

  // Detail line: left = CLI name + version, right = cost (hidden for kiro)
  const cliLabel = s.cli_name ? esc(s.cli_name) + (s.cli_version ? ' v' + esc(s.cli_version) : '') : '';
  const showCost = s.cli_name !== 'kiro';
  const cost = s.total_cost || 0;
  const costText = '$' + (cost < 0.01 && cost > 0 ? cost.toFixed(4) : cost.toFixed(2));
  const costClass = 'detail-cost' + (cost >= 1 ? ' high-cost' : cost > 0 ? ' has-cost' : '');
  // Session ID row (shown when session_id is available)
  let sidHtml = '';
  if (s.session_id) {
    const shortId = s.session_id.length > 16 ? s.session_id.substring(0, 16) : s.session_id;
    sidHtml = '<div style="display:flex;align-items:center;gap:8px;margin-top:4px;font-size:12px;color:#8b949e">' +
      '<span style="font-family:monospace;color:#7d8590;background:#21262d;padding:2px 8px;border-radius:4px;font-size:11px">' + esc(shortId) + '</span>' +
      '<button onclick="copySessionId(\'' + escAttr(s.session_id) + '\')" style="background:none;border:1px solid #30363d;color:#8b949e;font-size:11px;padding:2px 8px;border-radius:4px;cursor:pointer">📋 复制 ID</button>';
    if (s.state === 'suspended' || s.state === 'dead') {
      sidHtml += '<button onclick="resumeCurrentSession()" style="background:#238636;border:none;color:#fff;font-size:11px;padding:3px 10px;border-radius:4px;cursor:pointer;font-weight:500">⟳ Resume</button>';
    }
    sidHtml += '</div>';
  }

  const workspace = s.workspace || sessionWorkspaces[selectedKey] || '';
  const filesBtn = workspace
    ? '<button class="btn-session-files" onclick="openFileHub(\'' + escJs(workspace) + '\')" title="Browse files"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg></button>'
    : '';

  main.innerHTML =
    '<div class="main-header">' +
      '<button class="btn-mobile-back" onclick="mobileBack()" title="back">&#8592;</button>' +
      '<div class="main-header-content">' +
      '<h2>' + esc(displayName) + '</h2>' +
      '<div class="detail">' +
        '<span class="detail-left">' + cliLabel + '</span>' +
        (showCost ? '<span class="' + costClass + '" id="header-cost">' + costText + '</span>' : '') +
      '</div>' +
      sidHtml +
      '</div>' +
      '<button onclick="openReplayViewer(\'' + escJs(selectedKey) + '\')" title="Replay session" style="background:none;border:none;color:#8b949e;cursor:pointer;padding:4px;font-size:16px">&#9654;</button>' +
      filesBtn +
      '<button class="hdr-btn" onclick="toggleCtxPanel()" title="Context Panel">&#9776;</button>' +
    '</div>' +
    '<div class="events" id="events-scroll"></div>' +
    '<div class="running-banner" id="running-banner" style="display:none">' +
      '<div class="rb-tool-row">' +
        '<span class="running-status"><span class="running-dot"></span><span id="tool-activity">Working...</span></span>' +
        '<span class="rb-elapsed" id="rb-elapsed"></span>' +
      '</div>' +
      '<div class="rb-thinking-summary" id="rb-thinking-summary" style="display:none"></div>' +
      '<div class="rb-agents" id="rb-agents"></div>' +
      '<div class="rb-stats" id="rb-stats" style="display:none"></div>' +
    '</div>' +
    '<div class="input-area' + (voiceInputMode ? ' voice-mode' : '') + '" id="input-area">' +
      '<div class="file-preview" id="file-preview"></div>' +
      '<div class="input-row">' +
        '<button class="btn-icon" onclick="openFilePicker()" title="upload image">&#x1f4ce;</button>' +
        '<button class="btn-icon btn-mic" id="btn-mic" onclick="toggleInputMode()" title="' + (voiceInputMode ? '\u5207\u6362\u952e\u76d8' : '\u5207\u6362\u8bed\u97f3') + '">' + (voiceInputMode ? '&#x2328;' : '&#x1f3a4;') + '</button>' +
        '<div id="msg-input" contenteditable="true" role="textbox" data-placeholder="send a message..." onkeydown="handleKey(event)" oncompositionend="lastCompositionEnd=Date.now()"></div>' +
        '<button class="btn-hold-talk" id="btn-hold-talk">\u6309\u4f4f\u8bf4\u8bdd</button>' +
        '<button class="btn-icon btn-send" id="btn-send" onclick="sendMessage()" title="send">&#x27a4;</button>' +
        '<button class="btn-icon btn-stop" id="btn-stop" onclick="interruptSession()" title="stop">&#x25A0;</button>' +
      '</div>' +
      '<div class="input-hints">Enter send &middot; Shift+Enter newline &middot; Esc interrupt</div>' +
      '<input type="file" id="file-input" accept="image/*" multiple style="display:none" onchange="handleFiles(this.files)">' +
    '</div>';

  // Enable drag-drop
  const ia = document.getElementById('input-area');
  ia.addEventListener('dragover', e => { e.preventDefault(); ia.style.borderColor='#58a6ff'; });
  ia.addEventListener('dragleave', () => { ia.style.borderColor=''; });
  ia.addEventListener('drop', e => { e.preventDefault(); ia.style.borderColor=''; handleFiles(e.dataTransfer.files); });

  // Voice hold-to-talk: only touchstart on button; move/end on document (see voiceTouchStart)
  const holdBtn = document.getElementById('btn-hold-talk');
  if (holdBtn) {
    holdBtn.addEventListener('touchstart', voiceTouchStart, {passive: false});
    holdBtn.addEventListener('mousedown', voiceMouseDown);
  }

  updateSendButton(s.state || '');
  // Suspended sessions can be resumed by sending a new message
  // Double-tap events feed → focus input (mobile)
  let lastTapMs = 0;
  document.getElementById('events-scroll').addEventListener('touchend', e => {
    if (!isMobile() || e.target.closest('a,button,code,pre')) return;
    const now = Date.now();
    if (now - lastTapMs < 300) { document.getElementById('msg-input')?.focus(); lastTapMs = 0; }
    else lastTapMs = now;
  }, {passive:true});
}

/* ===== Event fetch + render ====================================== */

export async function fetchEvents(full) {
  if (!selectedKey) return;
  try {
    let url = '/api/sessions/events?key=' + encodeURIComponent(selectedKey);
    if (selectedNode && selectedNode !== 'local') url += '&node=' + encodeURIComponent(selectedNode);
    if (!full && lastEventTime > 0) url += '&after=' + lastEventTime;

    const headers = {};
    const t = getToken();
    if (t) headers['Authorization'] = 'Bearer ' + t;
    const r = await fetch(url, { headers });
    const events = await r.json();
    if (!events || events.length === 0) return;

    if (full) {
      renderEvents(events);
    } else {
      appendEvents(events);
    }

    const last = events[events.length - 1];
    if (last && last.time > lastEventTime) lastEventTime = last.time;
  } catch (e) { console.error('fetch events:', e); }
}

export function renderEvents(events) {
  const el = document.getElementById('events-scroll');
  if (!el) return;
  const display = processEventsForDisplay(events);
  el.innerHTML = display.map(eventHtml).filter(Boolean).join('') || '<div class="empty-state">no events yet</div>';
  el.scrollTop = el.scrollHeight;
  // Task 15: Inject bookmark buttons on AI messages
  loadSessionBookmarks().then(() => injectBookmarkButtons());
  // Track the latest rendered event time for deduplication
  if (events.length > 0) {
    const last = events[events.length - 1];
    if (last.time) lastRenderedEventTime = last.time;
  }
  runMermaid();
  runKatex();
}

export function appendEvents(events) {
  const el = document.getElementById('events-scroll');
  if (!el) return;
  const empty = el.querySelector('.empty-state');
  if (empty) empty.remove();
  const wasBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 30;
  events.forEach(e => {
    if (isHiddenEvent(e)) return;
    // Deduplicate: skip events at or before the last rendered time
    if (e.time && e.time <= lastRenderedEventTime) return;
    const h = eventHtml(e); if (h) el.insertAdjacentHTML('beforeend', h);
    if (e.time && e.time > lastRenderedEventTime) lastRenderedEventTime = e.time;
  });
  if (wasBottom) el.scrollTop = el.scrollHeight;
  runMermaid();
  runKatex();
  // Task 15: Inject bookmark buttons on newly appended AI messages
  injectBookmarkButtons();
}

/* Check if a tool_use event is an Edit/Replace that should render as diff */
export function isEditToolEvent(e) {
  return e.type === 'tool_use' && (e.tool === 'Edit' || e.tool === 'Replace');
}

/* Check if an event should be hidden from the event list */
export function isHiddenEvent(e) {
  if (e.type === 'tool_use') return false; // Task 2: all tool_use events render as cards
  return e.type === 'result' || e.type === 'agent' || e.type === 'task_start' || e.type === 'task_progress' || e.type === 'task_done';
}

/* Task 2: Tool icon mapping by tool name */
var toolIconMap = {Read:'\uD83D\uDCC4',Edit:'\u270F\uFE0F',Replace:'\u270F\uFE0F',Bash:'\uD83D\uDCBB',Grep:'\uD83D\uDD0D',Glob:'\uD83D\uDCC2',Agent:'\uD83E\uDD16',Write:'\uD83D\uDCDD',NotebookEdit:'\uD83D\uDCD3',Skill:'\u2728',ToolSearch:'\uD83D\uDD0E',WebFetch:'\uD83C\uDF10'};

/* Task 2: Extract a short parameter summary for the tool card header */
export function toolParamSummary(e) {
  var detail = e.detail || '';
  // detail is formatted by Go backend as "ToolName path/pattern/command..."
  // Strip the tool name prefix to get just the parameter summary
  var toolName = e.tool || e.summary || '';
  if (detail.indexOf(toolName + ' ') === 0) {
    return detail.substring(toolName.length + 1).substring(0, 60);
  }
  if (detail.indexOf(toolName + ':') === 0) {
    return detail.substring(toolName.length + 1).trim().substring(0, 60);
  }
  return detail.substring(0, 60);
}

/* Task 2: Build the body content for a tool card */
export function toolCardBody(e) {
  // Edit/Replace: render diff inside card body
  if (isEditToolEvent(e) && e.tool_input) {
    try {
      var inp = (typeof e.tool_input === 'string') ? JSON.parse(e.tool_input) : e.tool_input;
      var diffHtml = renderDiff(inp.old_string || '', inp.new_string || '', inp.file_path || '');
      if (diffHtml) return diffHtml;
    } catch (_) {}
  }
  // Default: show full detail text
  return '<div class="tool-body-inner">' + esc(e.detail || e.summary || '') + '</div>';
}

/* Task 2: Render a tool_use event as a collapsible card */
export function toolCardHtml(e) {
  var toolName = e.tool || e.summary || 'Tool';
  var icon = toolIconMap[toolName] || '\uD83D\uDD27';
  var summary = esc(toolParamSummary(e));
  var status = '\u2713';
  // Edit/Replace: show +N -M in status
  if (isEditToolEvent(e) && e.tool_input) {
    try {
      var inp2 = (typeof e.tool_input === 'string') ? JSON.parse(e.tool_input) : e.tool_input;
      var added = (inp2.new_string || '').split('\n').length;
      var removed = (inp2.old_string || '').split('\n').length;
      status = '\u2713 +' + added + ' -' + removed;
    } catch (_) {}
  }
  var body = toolCardBody(e);
  return '<div class="event tool_use"><span class="event-icon" style="display:none"></span><div class="event-content">' +
    '<div class="tool-card">' +
      '<div class="tool-hdr" onclick="toggleToolCard(this)">' +
        '<span class="t-icon">' + icon + '</span>' +
        '<span class="t-name">' + esc(toolName) + '</span>' +
        '<span class="t-detail">' + summary + '</span>' +
        '<span class="t-status">' + status + '</span>' +
        '<span class="t-expand">\u25BC</span>' +
      '</div>' +
      '<div class="tool-body">' + body + '</div>' +
    '</div>' +
  '</div></div>';
}

/* Task 2: Toggle tool card expand/collapse */
export function toggleToolCard(hdr) {
  var body = hdr.nextElementSibling;
  var arrow = hdr.querySelector('.t-expand');
  body.classList.toggle('open');
  if (arrow) arrow.classList.toggle('open');
}

export function eventHtml(e) {
  if (e.type === 'thinking') return '';
  if (isHiddenEvent(e)) return '';
  // Task 2: Render all tool_use events as collapsible cards
  if (e.type === 'tool_use') return toolCardHtml(e);
  // Filter out Claude Code system XML injected as user messages
  const raw0 = e.detail || e.summary || '';
  if (e.type === 'user' && /^<(task-notification|system-reminder|local-command|command-name|available-deferred-tools)[\s>]/.test(raw0)) return '';
  const icons = {init:'\u2699',system:'\u2699',user:'\u{1f464}',text:'\u2726'};
  const icon = icons[e.type] || '';

  let content = '';
  if (e.type === 'system') {
    content = esc(e.summary || e.type);
  } else if (e.type === 'text' || e.type === 'user') {
    let raw = e.detail || e.summary || e.type;
    // Strip redundant "[+N image(s)]" suffix when thumbnails are present
    if (e.images && e.images.length > 0) raw = raw.replace(/ \[\+\d+ image\(s\)\]$/, '');
    // Enhanced /ls output rendering for AI responses
    if (e.type === 'text') {
      const lsHtml = enhanceLsOutput(raw);
      if (lsHtml) { content = lsHtml; } else { content = renderMd(raw); }
    } else {
      content = renderMd(raw);
    }
  } else {
    content = esc(e.detail || e.summary || e.type);
  }

  // Render image thumbnails for user messages
  let imgHtml = '';
  if (e.images && e.images.length > 0) {
    imgHtml = '<div class="event-images">' + e.images.map(src =>
      '<img src="' + escAttr(src) + '" loading="lazy" onclick="openLightbox(this.src)">'
    ).join('') + '</div>';
  }

  // Task 4: Long output collapse for AI text events
  if (e.type === 'text' && content) {
    var tmp = document.createElement('div');
    tmp.innerHTML = content;
    var lineEls = tmp.querySelectorAll('br, p, li, .md-pre, tr');
    var textLines = (tmp.textContent || '').split('\n').length;
    if (lineEls.length > 10 || textLines > 15) {
      content = '<div class="collapse-wrap">' + content +
        '<div class="collapse-grad"><button class="collapse-btn" onclick="toggleCollapse(this)">\u25BC Show more</button></div></div>';
    }
  }

  return '<div class="event ' + esc(e.type||'') + '">' +
    '<span class="event-icon">' + icon + '</span>' +
    '<div class="event-content">' + content + imgHtml + '</div></div>';
}

/* ===== Send message + composer helpers =========================== */

export function handleKey(e) {
  if (e.key === 'Escape') { e.preventDefault(); interruptSession(); return; }
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing && Date.now() - lastCompositionEnd > 30) { e.preventDefault(); sendMessage(); }
}

export function autoGrow(el) {} // no-op: contenteditable auto-sizes
export function getMsgValue(el) { return (el ? el.innerText : '').trim(); }
export function setMsgValue(el, v) { if (el) el.innerText = v; }
export function clearMsg(el) { if (el) el.textContent = ''; }

export async function sendMessage() {
  if (sending) return;

  // Auto-takeover: if viewing a discovered session, takeover first then send
  if (pendingDiscovered && !selectedKey) {
    const input = document.getElementById('msg-input');
    const text = getMsgValue(input);
    if (!text) return;
    sending = true;
    const btn = document.getElementById('btn-send');
    if (btn) btn.classList.add('sending');
    if (input) input.dataset.placeholder = 'taking over session...';
    if (input) input.contentEditable = 'false';
    const pd = pendingDiscovered;
    try {
      const headers = {'Content-Type': 'application/json'};
      const token = getToken();
      if (token) headers['Authorization'] = 'Bearer ' + token;
      const r = await fetch('/api/discovered/takeover', {
        method: 'POST', headers,
        body: JSON.stringify({pid: pd.pid, session_id: pd.sessionId, cwd: pd.cwd, proc_start_time: pd.procStartTime || 0, node: pd.node || ''})
      });
      if (!r.ok) {
        const errText = await r.text();
        showToast('takeover failed: ' + errText);
        if (input) { input.dataset.placeholder = 'send a message to take over...'; input.contentEditable = 'true'; }
        sending = false;
        if (btn) btn.classList.remove('sending');
        return;
      }
      const data = await r.json();
      if (!data.key) {
        showToast('takeover failed: no session key returned');
        if (input) { input.dataset.placeholder = 'send a message to take over...'; input.contentEditable = 'true'; }
        sending = false;
        if (btn) btn.classList.remove('sending');
        return;
      }
      // Remove the discovered card from sidebar
      const card = document.querySelector('.session-card[data-key="_discovered:' + pd.pid + '"]');
      if (card) card.remove();
      pendingDiscovered = null;
      // Poll until the session appears in managed sessions (up to 10s)
      const takenKey = data.key;
      const takenNode = pd.node || 'local';
      let ready = false;
      for (let i = 0; i < 20; i++) {
        await new Promise(resolve => setTimeout(resolve, 500));
        lastVersion = 0;
        await fetchSessions();
        if (sessionsData[sid(takenKey, takenNode)]) { ready = true; break; }
      }
      if (!ready) {
        showToast('takeover timed out — session not ready');
        if (input) { input.dataset.placeholder = 'send a message...'; input.contentEditable = 'true'; }
        sending = false;
        if (btn) btn.classList.remove('sending');
        return;
      }
      // Session is ready — switch to it and send the message
      sending = false;
      selectSession(takenKey, takenNode);
      // Restore the message text and send
      const newInput = document.getElementById('msg-input');
      if (newInput) setMsgValue(newInput, text);
      await sendMessage();
      return;
    } catch (e) {
      showToast('takeover error: ' + e.message);
      if (input) { input.dataset.placeholder = 'send a message to take over...'; input.contentEditable = 'true'; }
      sending = false;
      if (btn) btn.classList.remove('sending');
      return;
    }
  }

  if (!selectedKey) return;
  const input = document.getElementById('msg-input');
  const text = getMsgValue(input);
  if (!text && pendingFiles.length === 0) return;

  sending = true;
  const btn = document.getElementById('btn-send');
  if (btn) btn.classList.add('sending');

  // WS path: send via WebSocket when connected and no files
  if (wsm.isConnected() && pendingFiles.length === 0) {
    const id = 'r' + (++wsm.sendCounter);
    const sendMsg = { type: 'send', key: selectedKey, text: text, id: id };
    if (selectedNode && selectedNode !== 'local') sendMsg.node = selectedNode;
    if (sessionWorkspaces[selectedKey]) {
      sendMsg.workspace = sessionWorkspaces[selectedKey];
      delete sessionWorkspaces[selectedKey];
      delete sessionNodes[selectedKey];
    }
    if (wsm.send(sendMsg)) {
      // Optimistic render: show user message immediately without waiting
      // for the CLI to echo it back as a "user" event.
      const el = document.getElementById('events-scroll');
      if (el && text) {
        const wasBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 30;
        const html = eventHtml({type: 'user', detail: text, time: Date.now()});
        if (html) {
          el.insertAdjacentHTML('beforeend', html);
          el.lastElementChild.classList.add('optimistic-msg');
          if (wasBottom) el.scrollTop = el.scrollHeight;
        }
      }
      if (input) clearMsg(input);
      delete sessionDrafts[selectedKey];
      sending = false;
      if (btn) btn.classList.remove('sending');
      return;
    }
    // WS send failed, fall through to HTTP path below
  }

  // HTTP POST fallback
  try {
    const headers = {};
    const token = getToken();
    if (token) headers['Authorization'] = 'Bearer ' + token;

    let body;
    if (pendingFiles.length > 0) {
      const fd = new FormData();
      fd.append('key', selectedKey);
      if (selectedNode && selectedNode !== 'local') fd.append('node', selectedNode);
      if (text) fd.append('text', text);
      if (sessionWorkspaces[selectedKey]) {
        fd.append('workspace', sessionWorkspaces[selectedKey]);
        delete sessionWorkspaces[selectedKey];
        delete sessionNodes[selectedKey];
      }
      pendingFiles.forEach(f => fd.append('files', f));
      body = fd;
    } else {
      headers['Content-Type'] = 'application/json';
      const payload = {key: selectedKey, text: text};
      if (selectedNode && selectedNode !== 'local') payload.node = selectedNode;
      if (sessionWorkspaces[selectedKey]) {
        payload.workspace = sessionWorkspaces[selectedKey];
        delete sessionWorkspaces[selectedKey];
        delete sessionNodes[selectedKey];
      }
      body = JSON.stringify(payload);
    }

    const r = await fetch('/api/sessions/send', {method:'POST', headers, body});

    if (r.status === 401 || r.status === 403) {
      if (input) setMsgValue(input, text);
      showAuthModal();
      return;
    }
    if (r.status === 429) {
      if (input) setMsgValue(input, text);
      showToast('message queue full, please wait');
      return;
    }
    if (!r.ok) {
      if (input) setMsgValue(input, text);
      const d = await r.json().catch(() => ({}));
      showToast(d.error || 'send failed: ' + r.status);
      return;
    }

    // Parse response to check for command results (/ls, /help, /cd, /pwd)
    const resp = await r.json().catch(() => ({}));

    // Clear input only after confirmed success
    if (input) clearMsg(input);
    delete sessionDrafts[selectedKey];
    pendingFiles = [];
    renderFilePreviews();

    if (resp.status === 'command') {
      // Render command result the same way as WS path
      const el = document.getElementById('events-scroll');
      if (el) {
        const empty = el.querySelector('.empty-state');
        if (empty) empty.remove();
        const enhanced = enhanceLsOutput(resp.result || '');
        const content = enhanced || '<pre style="white-space:pre-wrap;margin:0">' + esc(resp.result || '') + '</pre>';
        const wasBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 30;
        el.insertAdjacentHTML('beforeend',
          '<div class="event-row" style="border-left:3px solid #484f58;padding:8px 12px;margin:4px 0">' +
          '<div style="display:flex;align-items:center;gap:6px;margin-bottom:4px">' +
          '<span style="background:#21262d;color:#484f58;padding:2px 8px;border-radius:8px;font-size:10px;border:1px solid #30363d">SYSTEM</span>' +
          '</div>' + content + '</div>');
        if (wasBottom) el.scrollTop = el.scrollHeight;
      }
      return;
    }

    // Speed up polling when WS not connected
    if (!wsm.isConnected()) {
      if (eventTimer) clearInterval(eventTimer);
      eventTimer = setInterval(() => fetchEvents(false), 500);
      setTimeout(() => {
        if (eventTimer) clearInterval(eventTimer);
        if (!wsm.isConnected()) {
          eventTimer = setInterval(() => fetchEvents(false), 1000);
        }
      }, 15000);
    }
  } catch (e) {
    if (input) input.value = text;
    showToast('send error: ' + e.message);
  } finally {
    sending = false;
    if (btn) btn.classList.remove('sending');
  }
}

/* ===== Running banner: tool activity + agent tracking ============ */

let turnState = {
  toolCount: 0, currentTool: null, agents: [], isThinking: false,
  thinkingSummary: '', toolCounts: {}, toolOrder: [], turnStartTime: 0, isWriting: false,
  timerId: null
};

export function resetTurnState() {
  if (turnState.timerId) clearInterval(turnState.timerId);
  turnState = {
    toolCount: 0, currentTool: null, agents: [], isThinking: false,
    thinkingSummary: '', toolCounts: {}, toolOrder: [], turnStartTime: 0, isWriting: false,
    timerId: null
  };
  refreshBanner();
}

export function startTurnTimer() {
  if (turnState.turnStartTime) return;
  turnState.turnStartTime = Date.now();
  turnState.timerId = setInterval(function() {
    const el = document.getElementById('rb-elapsed');
    if (!el || !turnState.turnStartTime) return;
    const s = Math.floor((Date.now() - turnState.turnStartTime) / 1000);
    el.textContent = Math.floor(s / 60) + ':' + String(s % 60).padStart(2, '0');
  }, 1000);
}

export function trackTool(name) {
  if (!name) return;
  if (!turnState.toolCounts[name]) {
    turnState.toolCounts[name] = 0;
    turnState.toolOrder.push(name);
  }
  turnState.toolCounts[name]++;
}

export function fmtDuration(ms) {
  if (ms < 1000) return ms + 'ms';
  var s = ms / 1000;
  return s < 60 ? s.toFixed(1) + 's' : Math.floor(s / 60) + 'm' + Math.floor(s % 60) + 's';
}

const toolVerbs = {
  Read: 'Reading', Edit: 'Editing', Write: 'Writing', Bash: 'Running',
  Grep: 'Searching', Glob: 'Finding files', Agent: 'Agent',
  Notebook: 'Editing notebook', WebFetch: 'Fetching'
};

export function toolVerb(tool, summary) {
  const verb = toolVerbs[tool] || ('Using ' + tool);
  if (!summary || summary === tool) return verb + '...';
  return verb + ' ' + summary;
}

export function refreshBanner() {
  const actEl = document.getElementById('tool-activity');
  const thinkEl = document.getElementById('rb-thinking-summary');
  const agEl = document.getElementById('rb-agents');
  const statsEl = document.getElementById('rb-stats');

  // Line 1: current activity
  if (actEl) {
    if (turnState.currentTool) {
      actEl.textContent = toolVerb(turnState.currentTool.tool, turnState.currentTool.summary);
    } else if (turnState.isThinking) {
      actEl.textContent = 'Thinking...';
    } else if (turnState.isWriting) {
      actEl.textContent = 'Writing...';
    } else {
      actEl.textContent = 'Working...';
    }
  }

  // Thinking summary line (only during thinking)
  if (thinkEl) {
    if (turnState.isThinking && turnState.thinkingSummary) {
      thinkEl.textContent = turnState.thinkingSummary;
      thinkEl.style.display = '';
    } else {
      thinkEl.style.display = 'none';
    }
  }

  // Agent rows
  if (agEl) {
    agEl.innerHTML = renderAgentRows();
  }

  // Stats line (hidden when agents are shown)
  if (statsEl) {
    var hasAgents = turnState.agents.length > 0;
    if (!hasAgents && turnState.toolOrder.length > 0) {
      statsEl.textContent = turnState.toolOrder.map(function(t) {
        return t + ' \u00d7' + turnState.toolCounts[t];
      }).join(' \u00b7 ');
      statsEl.style.display = '';
    } else {
      statsEl.style.display = 'none';
    }
  }

  // Auto-show/hide banner based on active content. When state is "running",
  // updateSendButton already forces display=''. For "ready" sessions (e.g.
  // after zero-downtime restart with background agents), show the banner if
  // there's active tool/thinking/agent activity; hide it when turn resets.
  const banner = document.getElementById('running-banner');
  if (banner) {
    const hasContent = turnState.currentTool || turnState.isThinking || turnState.isWriting || turnState.agents.length > 0 || turnState.toolOrder.length > 0;
    if (hasContent && banner.style.display === 'none') {
      banner.style.display = '';
    } else if (!hasContent && banner.style.display !== 'none') {
      const sKey = sid(selectedKey, selectedNode);
      const sess = sessionsData[sKey];
      if (!sess || sess.state !== 'running') {
        banner.style.display = 'none';
      }
    }
  }
}

export function renderAgentRows() {
  var agents = turnState.agents;
  if (agents.length === 0) return '';

  // Separate solo subagents from team members
  var solos = [];
  var teams = {}; // teamName -> [agent, ...]
  for (var i = 0; i < agents.length; i++) {
    var a = agents[i];
    if (a.teamName) {
      if (!teams[a.teamName]) teams[a.teamName] = [];
      teams[a.teamName].push(a);
    } else {
      solos.push(a);
    }
  }

  var html = '';
  // Solo subagents
  for (var j = 0; j < solos.length; j++) {
    html += agentRowHtml(solos[j]);
  }
  // Team groups
  var teamNames = Object.keys(teams);
  for (var k = 0; k < teamNames.length; k++) {
    var tn = teamNames[k];
    var members = teams[tn];
    html += '<div class="rb-team-header"><span class="team-icon">\u25c6</span>' + esc(tn) + '<span class="team-count">' + members.length + ' agents</span></div>';
    for (var m = 0; m < members.length; m++) {
      html += agentRowHtml(members[m]);
    }
  }
  return html;
}

export function agentRowHtml(a) {
  var isDone = a.status === 'completed' || a.status === 'error';
  var cls = 'rb-agent-row' + (isDone ? ' done' : '');
  var label = a.name || a.description || 'agent';
  var parts = '<div class="' + cls + '"><span class="sa-dot"></span>';
  if (a.background) parts += '<span class="sa-bg">[bg]</span>';
  parts += '<span class="sa-name">' + esc(label) + '</span>';
  // Detail: lastTool or description
  var detail = '';
  if (a.lastTool) detail = a.lastTool;
  else if (a.description && a.name) detail = a.description;
  if (detail) parts += '<span class="sa-detail">\u00b7 ' + esc(detail) + '</span>';
  // Stats
  var stat = '';
  if (a.toolUses > 0) stat += a.toolUses + ' calls';
  if (a.durationMs > 0) stat += (stat ? ' \u00b7 ' : '') + fmtDuration(a.durationMs);
  if (isDone) stat += (stat ? ' \u00b7 ' : '') + '\u2713';
  if (stat) parts += '<span class="sa-stat">\u00b7 ' + stat + '</span>';
  parts += '</div>';
  return parts;
}

export function findAgentByToolUseId(tuid) {
  for (var i = 0; i < turnState.agents.length; i++) {
    if (turnState.agents[i].toolUseId === tuid) return turnState.agents[i];
  }
  return null;
}

export function findAgentByTaskId(tid) {
  for (var i = 0; i < turnState.agents.length; i++) {
    if (turnState.agents[i].taskId === tid) return turnState.agents[i];
  }
  return null;
}

export function initAgentsFromSession() {
  const sd = sessionsData[sid(selectedKey, selectedNode || 'local')];
  if (sd && sd.subagents && sd.subagents.length > 0) {
    turnState.agents = sd.subagents.map(function(sa) {
      return {
        toolUseId: '', taskId: '', name: sa.name, teamName: '',
        description: sa.activity || '', background: !!sa.background,
        lastTool: '', toolUses: 0, totalTokens: 0, durationMs: 0, status: 'running'
      };
    });
  }
}

export function applyEventToTurnState(ev) {
  startTurnTimer();
  switch (ev.type) {
    case 'tool_use':
      turnState.toolCount++;
      trackTool(ev.tool || ev.summary);
      turnState.currentTool = { tool: ev.tool || ev.summary, summary: ev.detail ? ev.detail.split('\n')[0].substring(0, 60) : '' };
      turnState.isThinking = false;
      turnState.isWriting = false;
      turnState.thinkingSummary = '';
      break;
    case 'agent':
      turnState.toolCount++;
      trackTool('Agent');
      turnState.currentTool = null;
      turnState.isThinking = false;
      turnState.isWriting = false;
      turnState.thinkingSummary = '';
      turnState.agents.push({
        toolUseId: ev.tool_use_id || '', taskId: '',
        name: ev.subagent || '', teamName: ev.team_name || '',
        description: ev.summary || '', background: !!ev.background,
        lastTool: '', toolUses: 0, totalTokens: 0, durationMs: 0, status: 'spawned'
      });
      break;
    case 'task_start':
      var a1 = findAgentByToolUseId(ev.tool_use_id);
      if (a1) {
        a1.taskId = ev.task_id;
        a1.status = 'running';
      }
      break;
    case 'task_progress':
      var a2 = findAgentByTaskId(ev.task_id) || findAgentByToolUseId(ev.tool_use_id);
      if (a2) {
        if (!a2.taskId) a2.taskId = ev.task_id;
        a2.status = 'running';
        if (ev.summary) a2.description = ev.summary;
        if (ev.last_tool) a2.lastTool = ev.last_tool;
        if (ev.tool_uses) a2.toolUses = ev.tool_uses;
        if (ev.tokens) a2.totalTokens = ev.tokens;
        if (ev.duration_ms) a2.durationMs = ev.duration_ms;
      }
      break;
    case 'task_done':
      var a3 = findAgentByTaskId(ev.task_id) || findAgentByToolUseId(ev.tool_use_id);
      if (a3) {
        if (!a3.taskId) a3.taskId = ev.task_id;
        a3.status = ev.status || 'completed';
        if (ev.tool_uses) a3.toolUses = ev.tool_uses;
        if (ev.tokens) a3.totalTokens = ev.tokens;
        if (ev.duration_ms) a3.durationMs = ev.duration_ms;
      }
      break;
    case 'thinking':
      turnState.isThinking = true;
      turnState.isWriting = false;
      turnState.currentTool = null;
      turnState.thinkingSummary = ev.summary || '';
      break;
    case 'text':
      turnState.isThinking = false;
      turnState.isWriting = true;
      turnState.currentTool = null;
      turnState.thinkingSummary = '';
      break;
  }
}

export function interruptSession() {
  if (!selectedKey) return;
  const sd = sessionsData[sid(selectedKey, selectedNode || 'local')];
  if (!sd || sd.state !== 'running') return;
  if (wsm.isConnected()) {
    wsm.send({ type: 'interrupt', key: selectedKey, id: 'int' + Date.now() });
    showToast('interrupt sent', 'warning');
  } else {
    showToast('interrupt requires WebSocket', 'warning');
  }
}

export function scrollEventsToBottom() {
  const el = document.getElementById('events-scroll');
  if (el) el.scrollTop = el.scrollHeight;
}

export function updateSendButton(state) {
  const banner = document.getElementById('running-banner');
  const sendBtn = document.getElementById('btn-send');
  const stopBtn = document.getElementById('btn-stop');
  const inVoiceMode = document.getElementById('input-area')?.classList.contains('voice-mode');
  if (state === 'running') {
    if (banner) banner.style.display = '';
    if (sendBtn) sendBtn.style.display = 'none';
    if (stopBtn) stopBtn.style.display = 'flex';
    initAgentsFromSession();
    refreshBanner();
  } else {
    // Don't force-hide the banner — refreshBanner will show/hide it based on
    // whether there's active content (background agents, tools). This ensures
    // the banner stays visible after zero-downtime restart when background
    // agents are still running but the session state is "ready".
    if (sendBtn) sendBtn.style.display = inVoiceMode ? 'none' : 'flex';
    if (stopBtn) stopBtn.style.display = 'none';
    resetTurnState();
  }
  // Banner show/hide changes .events height — keep latest message visible
  scrollEventsToBottom();
}

/* ===== File handling (image attachments) ========================== */

export function openFilePicker() { document.getElementById('file-input').click(); }

export function handleFiles(fileList) {
  for (const f of fileList) {
    if (!f.type.startsWith('image/')) continue;
    if (f.size > 10 * 1024 * 1024) { showToast('file too large (max 10MB)'); continue; }
    if (pendingFiles.length >= 10) { showToast('max 10 files'); break; }
    pendingFiles.push(f);
  }
  // Reset input so the same file can be re-selected
  const fi = document.getElementById('file-input');
  if (fi) fi.value = '';
  renderFilePreviews();
}

export function removeFile(idx) {
  pendingFiles.splice(idx, 1);
  renderFilePreviews();
}

export function renderFilePreviews() {
  const el = document.getElementById('file-preview');
  if (!el) return;
  // Revoke old blob URLs to prevent memory leaks
  el.querySelectorAll('img[src^="blob:"]').forEach(img => URL.revokeObjectURL(img.src));
  el.innerHTML = pendingFiles.map((f, i) => {
    const url = URL.createObjectURL(f);
    return '<div class="file-thumb"><img src="' + url + '"><button class="remove" onclick="removeFile(' + i + ')">\u00d7</button></div>';
  }).join('');
}

/* ===== Voice recording (WeChat-style hold-to-talk) ================ */

let mediaRecorder = null;
let audioChunks = [];
let isUnloading = false;
let voiceRecTimer = null;
let voiceRecStart = 0;
const MAX_REC_SECS = 60;
let pendingMic = false;
let voiceInputMode = false;
let voiceTouchStartY = 0;
let voiceCancelled = false;
let voiceActive = false; // true while hold gesture is in progress
let persistentMicStream = null; // keep mic stream alive to avoid repeated permission prompts

window.addEventListener('pagehide', () => {
  isUnloading = true;
  voiceActive = false;
  cleanupVoiceTouchListeners();
  if (mediaRecorder && mediaRecorder.state !== 'inactive') mediaRecorder.stop();
  if (persistentMicStream) { persistentMicStream.getTracks().forEach(t => t.stop()); persistentMicStream = null; }
});

export function acquireMicStream() {
  if (persistentMicStream && persistentMicStream.getAudioTracks().some(t => t.readyState === 'live')) {
    return Promise.resolve(persistentMicStream);
  }
  if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
    return Promise.reject(new Error('not supported'));
  }
  return navigator.mediaDevices.getUserMedia({ audio: true }).then(stream => {
    persistentMicStream = stream;
    return stream;
  });
}

export function releaseMicStream() {
  if (persistentMicStream) {
    persistentMicStream.getTracks().forEach(t => t.stop());
    persistentMicStream = null;
  }
}

export function toggleInputMode() {
  if (pendingMic) return;
  voiceInputMode = !voiceInputMode;
  const ia = document.getElementById('input-area');
  if (ia) ia.classList.toggle('voice-mode', voiceInputMode);
  const btn = document.getElementById('btn-mic');
  if (btn) {
    btn.innerHTML = voiceInputMode ? '&#x2328;' : '&#x1f3a4;';
    btn.title = voiceInputMode ? '\u5207\u6362\u952e\u76d8' : '\u5207\u6362\u8bed\u97f3';
  }
  if (voiceInputMode) {
    // Pre-acquire mic permission so hold-to-talk won't prompt again
    acquireMicStream().catch(() => {});
  } else {
    releaseMicStream();
  }
  // Sync send/stop button visibility after mode toggle
  const sd = sessionsData[sid(selectedKey, selectedNode || 'local')];
  updateSendButton(sd ? sd.state || '' : '');
}

// --- Touch handlers for hold-to-talk ---
// touchmove/touchend registered on document (not button) so the overlay cannot block them.

export function voiceTouchStart(e) {
  e.preventDefault();
  voiceTouchStartY = e.touches[0].clientY;
  voiceCancelled = false;
  voiceActive = true;
  document.addEventListener('touchmove', voiceTouchMove, {passive: false});
  document.addEventListener('touchend', voiceTouchEnd, {passive: false});
  document.addEventListener('touchcancel', voiceTouchCancel, {passive: false});
  startVoiceRecording();
}

export function voiceTouchMove(e) {
  if (!voiceActive) return;
  e.preventDefault();
  const touch = e.touches[0];
  if (!touch) return;
  const dy = voiceTouchStartY - touch.clientY;
  const overlay = document.getElementById('voice-overlay');
  const hint = document.getElementById('vo-hint');
  if (dy > 80) {
    voiceCancelled = true;
    if (overlay) overlay.classList.add('cancel');
    if (hint) hint.textContent = '\u677e\u5f00\u53d6\u6d88';
  } else {
    voiceCancelled = false;
    if (overlay) overlay.classList.remove('cancel');
    if (hint) hint.textContent = '\u677e\u5f00\u53d1\u9001 \u00b7 \u4e0a\u6ed1\u53d6\u6d88';
  }
}

export function voiceTouchEnd(e) {
  if (!voiceActive) return;
  e.preventDefault();
  voiceActive = false;
  cleanupVoiceTouchListeners();
  stopVoiceRecording(!voiceCancelled);
}

export function voiceTouchCancel() {
  voiceActive = false;
  cleanupVoiceTouchListeners();
  stopVoiceRecording(false);
}

export function cleanupVoiceTouchListeners() {
  document.removeEventListener('touchmove', voiceTouchMove);
  document.removeEventListener('touchend', voiceTouchEnd);
  document.removeEventListener('touchcancel', voiceTouchCancel);
}

export function voiceMouseDown(e) {
  e.preventDefault();
  voiceCancelled = false;
  voiceActive = true;
  startVoiceRecording();
  const startY = e.clientY;
  const onMove = (me) => {
    const dy = startY - me.clientY;
    const overlay = document.getElementById('voice-overlay');
    const hint = document.getElementById('vo-hint');
    if (dy > 80) {
      voiceCancelled = true;
      if (overlay) overlay.classList.add('cancel');
      if (hint) hint.textContent = '\u677e\u5f00\u53d6\u6d88';
    } else {
      voiceCancelled = false;
      if (overlay) overlay.classList.remove('cancel');
      if (hint) hint.textContent = '\u677e\u5f00\u53d1\u9001 \u00b7 \u4e0a\u6ed1\u53d6\u6d88';
    }
  };
  const onUp = () => {
    document.removeEventListener('mousemove', onMove);
    document.removeEventListener('mouseup', onUp);
    voiceActive = false;
    stopVoiceRecording(!voiceCancelled);
  };
  document.addEventListener('mousemove', onMove);
  document.addEventListener('mouseup', onUp);
}

export function startVoiceRecording() {
  if (pendingMic) return;
  pendingMic = true;
  const holdBtn = document.getElementById('btn-hold-talk');
  if (holdBtn) holdBtn.classList.add('active');

  acquireMicStream().then(stream => {
    pendingMic = false;
    // If finger was released during async acquireMicStream, abort immediately
    if (!voiceActive) {
      if (holdBtn) holdBtn.classList.remove('active');
      return;
    }
    audioChunks = [];
    const mimeType = MediaRecorder.isTypeSupported('audio/webm;codecs=opus') ? 'audio/webm;codecs=opus'
      : MediaRecorder.isTypeSupported('audio/ogg;codecs=opus') ? 'audio/ogg;codecs=opus' : '';
    mediaRecorder = mimeType ? new MediaRecorder(stream, { mimeType }) : new MediaRecorder(stream);
    mediaRecorder.ondataavailable = e => { if (e.data.size > 0) audioChunks.push(e.data); };
    mediaRecorder.onstop = () => {
      clearInterval(voiceRecTimer);
      // Do NOT stop persistent stream tracks — keep them alive for next recording
      if (holdBtn) holdBtn.classList.remove('active');
      if (isUnloading) return;

      if (voiceCancelled) {
        hideVoiceOverlay();
        showToast('\u5df2\u53d6\u6d88');
        audioChunks = [];
        return;
      }

      const blob = new Blob(audioChunks, { type: mediaRecorder.mimeType });
      audioChunks = [];
      if (blob.size < 1000) {
        hideVoiceOverlay();
        showToast('\u5f55\u97f3\u592a\u77ed');
        return;
      }
      // Show transcribing state on overlay
      const overlay = document.getElementById('voice-overlay');
      if (overlay) overlay.classList.add('transcribing');
      const hint = document.getElementById('vo-hint');
      if (hint) hint.textContent = '\u6b63\u5728\u8bc6\u522b...';
      transcribeAudio(blob, false);
    };
    mediaRecorder.start();
    voiceRecStart = Date.now();
    voiceRecTimer = setInterval(updateVoiceTimer, 200);
    updateVoiceTimer();
    // Show overlay
    const overlay = document.getElementById('voice-overlay');
    if (overlay) { overlay.classList.remove('cancel', 'transcribing'); overlay.classList.add('show'); }
    const hint = document.getElementById('vo-hint');
    if (hint) hint.textContent = '\u677e\u5f00\u53d1\u9001 \u00b7 \u4e0a\u6ed1\u53d6\u6d88';
  }).catch(err => {
    pendingMic = false;
    voiceActive = false;
    cleanupVoiceTouchListeners();
    if (holdBtn) holdBtn.classList.remove('active');
    showToast(err.message === 'not supported' ? '\u6d4f\u89c8\u5668\u4e0d\u652f\u6301\u5f55\u97f3' : '\u9ea6\u514b\u98ce\u6743\u9650\u88ab\u62d2\u7edd');
    console.warn('mic error:', err);
  });
}

export function stopVoiceRecording(shouldSend) {
  if (!shouldSend) voiceCancelled = true;
  const holdBtn = document.getElementById('btn-hold-talk');
  if (holdBtn) holdBtn.classList.remove('active');
  if (mediaRecorder && mediaRecorder.state === 'recording') {
    mediaRecorder.stop(); // triggers onstop handler
  } else {
    hideVoiceOverlay();
  }
}

export function hideVoiceOverlay() {
  const overlay = document.getElementById('voice-overlay');
  if (overlay) overlay.classList.remove('show', 'cancel', 'transcribing');
}

// Tap overlay to cancel (escape hatch for stuck states). Bound once the
// DOM is ready — module scripts defer, so the overlay element exists.
document.getElementById('voice-overlay')?.addEventListener('click', function(e) {
  // Normal flow: touchend/mouseup already stopped recording before click fires.
  // This only triggers when genuinely stuck (recording active or overlay visible).
  if (mediaRecorder && mediaRecorder.state === 'recording') {
    voiceActive = false;
    cleanupVoiceTouchListeners();
    stopVoiceRecording(false);
  } else if (this.classList.contains('show')) {
    // Stuck in transcribing state or overlay didn't dismiss
    hideVoiceOverlay();
  }
});

export function updateVoiceTimer() {
  const el = document.getElementById('vo-timer');
  if (!el) return;
  const secs = Math.floor((Date.now() - voiceRecStart) / 1000);
  el.textContent = secs + 's';
  if (secs >= MAX_REC_SECS) {
    stopVoiceRecording(true);
    showToast('\u5df2\u8fbe\u6700\u957f' + MAX_REC_SECS + '\u79d2');
  }
}

export function transcribeAudio(blob, autoSend) {
  const fd = new FormData();
  fd.append('audio', blob, 'recording.' + (blob.type.includes('webm') ? 'webm' : blob.type.includes('ogg') ? 'ogg' : 'mp4'));
  const headers = {};
  const token = getToken();
  if (token) headers['Authorization'] = 'Bearer ' + token;
  fetch('/api/transcribe', {
    method: 'POST',
    headers: headers,
    credentials: 'same-origin',
    body: fd
  }).then(r => {
    if (!r.ok) return r.text().then(t => { throw new Error('HTTP ' + r.status + ': ' + t); });
    return r.json();
  }).then(data => {
    hideVoiceOverlay();
    const input = document.getElementById('msg-input');
    if (input && data.text) {
      const cur = getMsgValue(input);
      setMsgValue(input, autoSend ? data.text : (cur ? cur + ' ' + data.text : data.text));
      if (autoSend) {
        sendMessage();
      } else {
        input.focus();
        showToast('\u8f6c\u5199: ' + data.text.substring(0, 50) + (data.text.length > 50 ? '...' : ''), 'success', 5000);
      }
    } else {
      showToast('\u672a\u68c0\u6d4b\u5230\u8bed\u97f3', '', 5000);
    }
  }).catch(err => {
    hideVoiceOverlay();
    showToast('\u8f6c\u5199\u5931\u8d25: ' + err.message, '', 5000);
  });
}

/* ===== Auth modal ================================================= */

export function showAuthModal() {
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  overlay.innerHTML =
    '<div class="modal">' +
      '<h3>Dashboard API Token</h3>' +
      '<input id="token-input" type="password" placeholder="enter dashboard token..." onkeydown="if(event.key===\'Enter\'){saveToken()}">' +
      '<div class="modal-btns">' +
        '<button onclick="this.closest(\'.modal-overlay\').remove()">cancel</button>' +
        '<button class="primary" onclick="saveToken()">save</button>' +
      '</div>' +
    '</div>';
  document.body.appendChild(overlay);
  setTimeout(() => document.getElementById('token-input').focus(), 100);
}

export async function saveToken() {
  const input = document.getElementById('token-input');
  const t = input && input.value.trim();
  if (!t) return;
  try {
    const r = await fetch('/api/auth/login', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({token: t})
    });
    if (r.ok) {
      const overlay = document.querySelector('.modal-overlay');
      if (overlay) overlay.remove();
      wsm.disconnect();
      wsm.connect();
      fetchSessions();
    } else {
      document.getElementById('token-input').value = '';
      document.getElementById('token-input').placeholder = 'invalid token — try again';
    }
  } catch(e) {
    showToast('network error', 'error');
  }
}

/* ===== Create new session (legacy + modal flows) ================== */

export function createNewSession() {
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  const ws = defaultWorkspace || '';

  if (projectsData.length > 0) {
    // Project picker: build DOM elements to avoid onclick attribute injection (XSS).
    const ul = document.createElement('ul');
    ul.className = 'proj-pick';
    projectsData.forEach(p => {
      const li = document.createElement('li');
      li.addEventListener('click', () => doCreateInProject(p.path, p.name, p.node || 'local'));
      li.innerHTML = '<div class="pp-name">' + esc(p.name) + '</div>' +
        '<div class="pp-path">' + esc(shortPath(p.path)) + '</div>';
      ul.appendChild(li);
    });
    const customLi = document.createElement('li');
    customLi.id = 'pp-custom-toggle';
    customLi.addEventListener('click', toggleCustomWorkspace);
    customLi.innerHTML = '<div class="pp-custom"><span class="pp-custom-icon">+</span> Custom workspace</div>';
    ul.appendChild(customLi);
    overlay.innerHTML =
      '<div class="modal">' +
        '<h3>New Session</h3>' +
        '<div id="pp-list-container"></div>' +
        '<div id="pp-custom-form" style="display:none;margin-top:8px">' +
          '<input id="new-workspace" placeholder="' + escAttr(ws) + '" value="" onkeydown="if(event.key===\'Enter\'){doCreateSession()}">' +
          '<div class="modal-btns"><button class="primary" onclick="doCreateSession()">create</button></div>' +
        '</div>' +
        '<div class="modal-btns"><button onclick="this.closest(\'.modal-overlay\').remove()">cancel</button></div>' +
      '</div>';
    document.body.appendChild(overlay);
    document.getElementById('pp-list-container').appendChild(ul);
  } else {
    // No projects: simple workspace input
    overlay.innerHTML =
      '<div class="modal">' +
        '<h3>New Session</h3>' +
        '<div style="margin-bottom:12px">' +
          '<label style="font-size:12px;color:#8b949e;display:block;margin-bottom:4px">Workspace</label>' +
          '<input id="new-workspace" placeholder="' + escAttr(ws) + '" value="' + escAttr(ws) + '" onkeydown="if(event.key===\'Enter\'){doCreateSession()}">' +
        '</div>' +
        '<div class="modal-btns">' +
          '<button onclick="this.closest(\'.modal-overlay\').remove()">cancel</button>' +
          '<button class="primary" onclick="doCreateSession()">create</button>' +
        '</div>' +
      '</div>';
  }
  if (!overlay.parentNode) document.body.appendChild(overlay);
  if (!projectsData.length) setTimeout(() => document.getElementById('new-workspace').focus(), 100);
}

export function toggleCustomWorkspace() {
  const form = document.getElementById('pp-custom-form');
  const toggle = document.getElementById('pp-custom-toggle');
  if (form.style.display === 'none') {
    form.style.display = '';
    toggle.style.display = 'none';
    setTimeout(() => document.getElementById('new-workspace').focus(), 100);
  }
}

export function doCreateInProject(projectPath, projectName, nodeId) {
  document.querySelector('.modal-overlay').remove();
  sessionCounter++;
  const now = new Date();
  const ts = now.toISOString().slice(0,10) + '-' +
    now.toTimeString().slice(0,8).replace(/:/g, '') + '-' + sessionCounter;
  const key = 'dashboard:direct:' + ts + ':' + projectName;

  sessionWorkspaces[key] = projectPath;
  if (nodeId && nodeId !== 'local') sessionNodes[key] = nodeId;

  stopPreviewPolling();
  wsm.unsubscribe();
  selectedKey = key;
  selectedNode = nodeId || 'local';
  lastEventTime = 0;
  mobileEnterChat();
  document.querySelectorAll('.session-card').forEach(el => el.classList.remove('active'));
  renderMainShell();
  lastVersion = 0;
  debouncedFetchSessions();
  showNewSessionNameDialog(key, nodeId || 'local');
  setTimeout(() => { const input = document.getElementById('msg-input'); if (input) input.focus(); }, 100);
}

export function doCreateSession() {
  const workspace = document.getElementById('new-workspace').value.trim();
  const folderName = workspace ? (workspace.replace(/\/+$/, '').split('/').pop() || 'session') : 'session';
  document.querySelector('.modal-overlay').remove();

  sessionCounter++;
  const now = new Date();
  const ts = now.toISOString().slice(0,10) + '-' +
    now.toTimeString().slice(0,8).replace(/:/g, '') + '-' + sessionCounter;
  const key = 'dashboard:direct:' + ts + ':' + folderName;

  if (workspace) sessionWorkspaces[key] = workspace;

  stopPreviewPolling();
  wsm.unsubscribe();
  selectedKey = key;
  selectedNode = 'local';
  lastEventTime = 0;
  mobileEnterChat();
  document.querySelectorAll('.session-card').forEach(el => el.classList.remove('active'));
  renderMainShell();
  lastVersion = 0;
  debouncedFetchSessions();
  showNewSessionNameDialog(key, 'local');
  setTimeout(() => { const input = document.getElementById('msg-input'); if (input) input.focus(); }, 100);
}

/* ===== New Session Modal (Cmd+N) ================================== */

var _nsAgentIcons = {
  general: '💬', 'code-reviewer': '🔍', reviewer: '🔍', review: '🔍',
  researcher: '📖', research: '📖', planner: '📋', plan: '📋',
  coder: '💻', code: '💻', writer: '✏️', default: '🤖'
};
var _nsAgentDescs = {
  general: 'General-purpose assistant',
  'code-reviewer': 'Code review & analysis',
  reviewer: 'Code review & analysis',
  review: 'Code review & analysis',
  researcher: 'Research & exploration',
  research: 'Research & exploration',
  planner: 'Planning & architecture',
  plan: 'Planning & architecture',
  coder: 'Code generation',
  code: 'Code generation',
  writer: 'Writing & documentation'
};

export function openNewSessionModal() {
  var ov = document.getElementById('nsOverlay');
  if (!ov) return;

  // Populate agents
  var agentsEl = document.getElementById('nsAgents');
  agentsEl.innerHTML = '';
  var agents = availableAgents && availableAgents.length > 0 ? availableAgents : ['general'];
  agents.forEach(function(a, i) {
    var icon = _nsAgentIcons[a] || _nsAgentIcons['default'];
    var desc = _nsAgentDescs[a] || 'Agent: ' + a;
    var div = document.createElement('div');
    div.className = 'ns-agent' + (i === 0 ? ' selected' : '');
    div.setAttribute('data-agent', a);
    div.innerHTML = '<div class="na-name">' + icon + ' ' + esc(a) + '</div><div class="na-desc">' + esc(desc) + '</div>';
    div.addEventListener('click', function() { selectNSAgent(this); });
    agentsEl.appendChild(div);
  });

  // Populate projects
  var projContainer = document.getElementById('nsProjects');
  var projLabel = document.getElementById('nsProjLabel');
  projContainer.innerHTML = '';
  if (projectsData && projectsData.length > 0) {
    projLabel.style.display = '';
    projContainer.style.display = '';
    projectsData.forEach(function(p) {
      var div = document.createElement('div');
      div.className = 'ns-proj';
      div.setAttribute('data-path', p.path);
      div.setAttribute('data-name', p.name);
      if (p.node) div.setAttribute('data-node', p.node);
      div.innerHTML = '<div class="np-name">' + esc(p.name) + '</div><div class="np-path">' + esc(shortPath(p.path)) + '</div>';
      div.addEventListener('click', function() { selectNSProject(this); });
      projContainer.appendChild(div);
    });
  } else {
    projLabel.style.display = 'none';
    projContainer.style.display = 'none';
  }

  // Reset workspace input
  var wsInput = document.getElementById('nsWorkdir');
  wsInput.value = defaultWorkspace || '';
  wsInput.placeholder = defaultWorkspace || '/home/user/project';

  ov.classList.add('show');
}

export function closeNewSessionModal() {
  var ov = document.getElementById('nsOverlay');
  if (ov) ov.classList.remove('show');
}

export function selectNSAgent(el) {
  var container = document.getElementById('nsAgents');
  container.querySelectorAll('.ns-agent').forEach(function(a) { a.classList.remove('selected'); });
  el.classList.add('selected');
}

export function selectNSProject(el) {
  // Toggle: click again to deselect
  if (el.classList.contains('selected')) {
    el.classList.remove('selected');
    return;
  }
  var container = document.getElementById('nsProjects');
  container.querySelectorAll('.ns-proj').forEach(function(p) { p.classList.remove('selected'); });
  el.classList.add('selected');
  // Auto-fill workspace from project path
  var wsInput = document.getElementById('nsWorkdir');
  if (wsInput && el.getAttribute('data-path')) {
    wsInput.value = el.getAttribute('data-path');
  }
}

export function confirmNewSession() {
  // Get selected agent
  var agentEl = document.querySelector('#nsAgents .ns-agent.selected');
  var agent = agentEl ? agentEl.getAttribute('data-agent') : 'general';

  // Get selected project (if any)
  var projEl = document.querySelector('#nsProjects .ns-proj.selected');
  var projPath = projEl ? projEl.getAttribute('data-path') : '';
  var projName = projEl ? projEl.getAttribute('data-name') : '';
  var projNode = projEl ? (projEl.getAttribute('data-node') || 'local') : 'local';

  // Get workspace
  var wsInput = document.getElementById('nsWorkdir');
  var workspace = wsInput ? wsInput.value.trim() : '';

  closeNewSessionModal();

  // If a project is selected, use doCreateInProject flow
  if (projPath && projName) {
    // Inject agent into the key by using doCreateInProject then letting the session key reflect the agent
    sessionCounter++;
    var now = new Date();
    var ts = now.toISOString().slice(0,10) + '-' +
      now.toTimeString().slice(0,8).replace(/:/g, '') + '-' + sessionCounter;
    var key = 'dashboard:direct:' + ts + ':' + projName;

    sessionWorkspaces[key] = projPath;
    if (projNode && projNode !== 'local') sessionNodes[key] = projNode;

    stopPreviewPolling();
    wsm.unsubscribe();
    selectedKey = key;
    selectedNode = projNode;
    lastEventTime = 0;
    mobileEnterChat();
    document.querySelectorAll('.session-card').forEach(function(el) { el.classList.remove('active'); });
    renderMainShell();
    lastVersion = 0;
    debouncedFetchSessions();
    showNewSessionNameDialog(key, projNode);
    setTimeout(function() { var input = document.getElementById('msg-input'); if (input) input.focus(); }, 100);
    return;
  }

  // No project: create with workspace and agent
  var folderName = workspace ? (workspace.replace(/\/+$/, '').split('/').pop() || 'session') : (agent !== 'general' ? agent : 'session');

  sessionCounter++;
  var now = new Date();
  var ts = now.toISOString().slice(0,10) + '-' +
    now.toTimeString().slice(0,8).replace(/:/g, '') + '-' + sessionCounter;
  var key = 'dashboard:direct:' + ts + ':' + folderName;

  if (workspace) sessionWorkspaces[key] = workspace;

  stopPreviewPolling();
  wsm.unsubscribe();
  selectedKey = key;
  selectedNode = 'local';
  lastEventTime = 0;
  mobileEnterChat();
  document.querySelectorAll('.session-card').forEach(function(el) { el.classList.remove('active'); });
  renderMainShell();
  lastVersion = 0;
  debouncedFetchSessions();
  showNewSessionNameDialog(key, 'local');
  setTimeout(function() { var input = document.getElementById('msg-input'); if (input) input.focus(); }, 100);
}

/* ===== New Session Naming Dialog ================================== */

// Pending names for sessions that haven't been registered in the router yet.
// Key: session key, Value: desired name. Applied after first send_ack(accepted).
var pendingSessionNames = {};

export function showNewSessionNameDialog(key, node) {
  const overlay = document.createElement('div');
  overlay.style.cssText = 'position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,.5);z-index:1000;display:flex;align-items:center;justify-content:center';
  const dialog = document.createElement('div');
  dialog.style.cssText = 'background:#161b22;border:1px solid #30363d;border-radius:8px;padding:16px;width:320px;max-width:90vw';
  dialog.innerHTML = '<label style="font-size:13px;color:#8b949e;display:block;margin-bottom:6px">Session 名称（可选）</label>' +
    '<input id="new-session-name" type="text" placeholder="例如：CloudFront 调试、API 开发..." style="width:100%;background:#0d1117;border:1px solid #30363d;border-radius:6px;padding:8px 12px;color:#e6edf3;font-size:14px;outline:none;box-sizing:border-box">' +
    '<div style="display:flex;gap:8px;margin-top:12px">' +
    '<button id="new-session-ok" style="background:#238636;border:none;color:#fff;padding:6px 16px;border-radius:6px;cursor:pointer;font-size:13px">确定</button>' +
    '<button id="new-session-skip" style="background:none;border:1px solid #30363d;color:#8b949e;padding:6px 16px;border-radius:6px;cursor:pointer;font-size:13px">跳过</button></div>';
  overlay.appendChild(dialog);
  document.body.appendChild(overlay);
  const input = document.getElementById('new-session-name');
  input.focus();
  function finish(save) {
    if (save && input.value.trim()) {
      // Store as pending — session may not exist in router yet.
      // Will be applied via sessions_update when session is created on server.
      pendingSessionNames[key] = input.value.trim();
      // Also try immediately (works if session already exists)
      renameSession(key, node || 'local', input.value.trim());
      // Optimistic: update card text + header immediately so name shows
      const card = document.querySelector('.session-card[data-key="' + key + '"]');
      if (card) {
        const promptEl = card.querySelector('.sc-prompt');
        if (promptEl) { promptEl.textContent = input.value.trim(); promptEl.style.color = ''; }
      }
      renderMainShell();
    }
    overlay.remove();
  }
  document.getElementById('new-session-ok').onclick = () => finish(true);
  document.getElementById('new-session-skip').onclick = () => finish(false);
  overlay.onclick = (e) => { if (e.target === overlay) finish(false); };
  input.addEventListener('keydown', e => {
    if (e.key === 'Enter') { e.preventDefault(); finish(true); }
    if (e.key === 'Escape') { e.preventDefault(); finish(false); }
  });
}

/* ===== Session ID helpers ========================================= */

export function copySessionId(id) {
  navigator.clipboard.writeText(id).then(() => showToast('Session ID 已复制')).catch(() => {
    const ta = document.createElement('textarea');
    ta.value = id; document.body.appendChild(ta); ta.select();
    document.execCommand('copy'); document.body.removeChild(ta);
    showToast('Session ID 已复制');
  });
}

export function resumeCurrentSession() {
  if (!selectedKey) return;
  const sKey = sid(selectedKey, selectedNode);
  const s = sessionsData[sKey];
  if (!s || !s.session_id) return;
  const input = document.getElementById('msg-input');
  if (input && !getMsgValue(input)) setMsgValue(input, '继续');
  sendMessage();
}

/* ===== Session Rename & Pin ======================================= */

export function startRename(key, node) {
  const card = document.querySelector('.session-card[data-key="' + key + '"][data-node="' + (node || 'local') + '"]');
  if (!card) return;
  const promptEl = card.querySelector('.sc-prompt');
  if (!promptEl) return;
  const sKey = sid(key, node);
  const currentName = sessionsData[sKey] ? (sessionsData[sKey].name || '') : '';
  const input = document.createElement('input');
  input.value = currentName;
  input.placeholder = '输入 Session 名称...';
  input.style.cssText = 'background:#0d1117;border:1px solid #58a6ff;border-radius:4px;padding:2px 6px;color:#e6edf3;font-size:14px;width:100%;outline:none';
  const editIcon = card.querySelector('.sc-edit');
  if (editIcon) editIcon.style.display = 'none';
  promptEl.replaceWith(input);
  input.focus();
  input.select();
  function finish(save) {
    if (save) renameSession(key, node, input.value.trim());
    input.replaceWith(Object.assign(document.createElement('div'), {
      className: 'sc-prompt', textContent: input.value || '(no prompt)'
    }));
    if (editIcon) editIcon.style.display = '';
  }
  input.addEventListener('keydown', e => {
    if (e.key === 'Enter') { e.preventDefault(); finish(true); }
    if (e.key === 'Escape') { e.preventDefault(); finish(false); }
  });
  input.addEventListener('blur', () => finish(true));
}

export async function renameSession(key, node, name) {
  const headers = {'Content-Type': 'application/json'};
  const token = getToken();
  if (token) headers['Authorization'] = 'Bearer ' + token;
  try {
    await fetch('/api/sessions/rename', { method: 'PATCH', headers, body: JSON.stringify({key, name}) });
    const sKey = sid(key, node);
    if (sessionsData[sKey]) sessionsData[sKey].name = name;
  } catch (e) { showToast('rename failed: ' + e.message, 'error'); }
}

export async function togglePin(key, node, pinned) {
  const headers = {'Content-Type': 'application/json'};
  const token = getToken();
  if (token) headers['Authorization'] = 'Bearer ' + token;
  try {
    await fetch('/api/sessions/pin', { method: 'PATCH', headers, body: JSON.stringify({key, pinned}) });
    const sKey = sid(key, node);
    if (sessionsData[sKey]) sessionsData[sKey].pinned = pinned;
    lastVersion = 0;
    debouncedFetchSessions();
  } catch (e) { showToast('pin failed: ' + e.message, 'error'); }
}

/* ===== Sidebar resizer (desktop only) ============================= */
// IIFE moved from dashboard.html verbatim. Runs at module load time;
// module scripts defer, so DOMContentLoaded has fired by the time this
// executes and #resizer / .sidebar exist.
(function(){
  const resizer = document.getElementById('resizer');
  const sidebar = document.querySelector('.sidebar');
  if (!resizer || !sidebar) return;
  const LS_KEY = 'naozhi_sidebar_w';
  const saved = parseFloat(localStorage.getItem(LS_KEY));
  if (saved >= 200) sidebar.style.width = saved + 'px';

  let startX, startW;
  resizer.addEventListener('mousedown', function(e) {
    e.preventDefault();
    startX = e.clientX;
    startW = sidebar.getBoundingClientRect().width;
    resizer.classList.add('dragging');
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup', onUp);
  });
  function onMove(e) {
    const w = Math.min(Math.max(startW + e.clientX - startX, 200), window.innerWidth * 0.6);
    sidebar.style.width = w + 'px';
  }
  function onUp() {
    resizer.classList.remove('dragging');
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
    document.removeEventListener('mousemove', onMove);
    document.removeEventListener('mouseup', onUp);
    localStorage.setItem(LS_KEY, Math.round(sidebar.getBoundingClientRect().width));
  }
  resizer.addEventListener('dblclick', function() {
    sidebar.style.width = '360px';
    localStorage.removeItem(LS_KEY);
  });
})();

// ------- legacy window.* bridges -----------------------------------
// Pre-module callers (inline <script>, onclick="..." HTML, remaining
// un-migrated views) reference these as bare identifiers. Removed in
// Phase 2 when every consumer is a module.

if (typeof window !== 'undefined') {
  // Session list + sidebar
  window.renderSessionList = renderSessionList;
  window.removePendingSession = removePendingSession;
  window.fetchSessions = fetchSessions;
  window.debouncedFetchSessions = debouncedFetchSessions;
  window.renderSidebar = renderSidebar;
  window.matchProject = matchProject;
  window.sessionCardHtml = sessionCardHtml;
  window.sessionWorkspaces = sessionWorkspaces;
  window.sessionNodes = sessionNodes;
  window.sessionDrafts = sessionDrafts;

  // History popover
  window.closeHistoryPopover = closeHistoryPopover;
  window.toggleHistory = toggleHistory;

  // Resume recent
  window.resumeRecentSession = resumeRecentSession;
  window.resumeRecentById = resumeRecentById;
  window.previewRecentSession = previewRecentSession;

  // Status bar
  window.updateStatusBar = updateStatusBar;

  // Select / dismiss / main shell
  window.selectSession = selectSession;
  window.dismissSession = dismissSession;
  window.renderMainShell = renderMainShell;

  // Event fetch/render
  window.fetchEvents = fetchEvents;
  window.renderEvents = renderEvents;
  window.appendEvents = appendEvents;
  window.isEditToolEvent = isEditToolEvent;
  window.isHiddenEvent = isHiddenEvent;
  window.toolParamSummary = toolParamSummary;
  window.toolCardBody = toolCardBody;
  window.toolCardHtml = toolCardHtml;
  window.toggleToolCard = toggleToolCard;
  window.eventHtml = eventHtml;

  // Composer
  window.handleKey = handleKey;
  window.autoGrow = autoGrow;
  window.getMsgValue = getMsgValue;
  window.setMsgValue = setMsgValue;
  window.clearMsg = clearMsg;
  window.sendMessage = sendMessage;

  // Turn state + banner
  window.resetTurnState = resetTurnState;
  window.startTurnTimer = startTurnTimer;
  window.trackTool = trackTool;
  window.fmtDuration = fmtDuration;
  window.toolVerb = toolVerb;
  window.refreshBanner = refreshBanner;
  window.renderAgentRows = renderAgentRows;
  window.agentRowHtml = agentRowHtml;
  window.findAgentByToolUseId = findAgentByToolUseId;
  window.findAgentByTaskId = findAgentByTaskId;
  window.initAgentsFromSession = initAgentsFromSession;
  window.applyEventToTurnState = applyEventToTurnState;
  window.interruptSession = interruptSession;
  window.scrollEventsToBottom = scrollEventsToBottom;
  window.updateSendButton = updateSendButton;

  // File attachments
  window.openFilePicker = openFilePicker;
  window.handleFiles = handleFiles;
  window.removeFile = removeFile;
  window.renderFilePreviews = renderFilePreviews;

  // Voice
  window.acquireMicStream = acquireMicStream;
  window.releaseMicStream = releaseMicStream;
  window.toggleInputMode = toggleInputMode;
  window.voiceTouchStart = voiceTouchStart;
  window.voiceTouchMove = voiceTouchMove;
  window.voiceTouchEnd = voiceTouchEnd;
  window.voiceTouchCancel = voiceTouchCancel;
  window.cleanupVoiceTouchListeners = cleanupVoiceTouchListeners;
  window.voiceMouseDown = voiceMouseDown;
  window.startVoiceRecording = startVoiceRecording;
  window.stopVoiceRecording = stopVoiceRecording;
  window.hideVoiceOverlay = hideVoiceOverlay;
  window.updateVoiceTimer = updateVoiceTimer;
  window.transcribeAudio = transcribeAudio;

  // Auth modal
  window.showAuthModal = showAuthModal;
  window.saveToken = saveToken;

  // Create session
  window.createNewSession = createNewSession;
  window.toggleCustomWorkspace = toggleCustomWorkspace;
  window.doCreateInProject = doCreateInProject;
  window.doCreateSession = doCreateSession;
  window.openNewSessionModal = openNewSessionModal;
  window.closeNewSessionModal = closeNewSessionModal;
  window.selectNSAgent = selectNSAgent;
  window.selectNSProject = selectNSProject;
  window.confirmNewSession = confirmNewSession;
  window.showNewSessionNameDialog = showNewSessionNameDialog;
  window.pendingSessionNames = pendingSessionNames;

  // Session helpers
  window.copySessionId = copySessionId;
  window.resumeCurrentSession = resumeCurrentSession;
  window.startRename = startRename;
  window.renameSession = renameSession;
  window.togglePin = togglePin;
}
