// js/views/wiki.js — Wiki view + Decision Journal (ADRs).
//
// Moved verbatim from dashboard.html's inline <script> (Task 12:
// Wiki View and Phase 4D: Decision Journal banners). Owns:
// category-grouped page list, page + sources panel rendering,
// Ingest/Lint actions, and the decision-journal sub-tab (list,
// detail, new-decision form). Shares the `#wvContent` slot with
// both pages and decisions — `switchWikiSubTab` flips between them.
//
// Helpers still living in the legacy inline script (esc* come from
// core/html.js; renderMd, showToast, authHeaders, timeAgo remain on
// window) are referenced via window during Phase 1.

import { esc, escAttr, escJs } from '../core/html.js';

// Module-local state (bridged to window below for legacy consumers —
// home.js reads window.wvPages to surface wiki-compiled activity).
var wvCurrentPage = '';
var wvPages = [];
var wikiSubTab = 'pages';
var decisionsCache = [];

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

function renderMd(s) {
  if (typeof window !== 'undefined' && typeof window.renderMd === 'function') {
    return window.renderMd(s);
  }
  return s || '';
}

// ------- Wiki pages -----------------------------------------------

export function renderWikiView() {
  const main = document.getElementById('main');
  main.innerHTML =
    '<div class="wv-container" id="wvContainer">' +
      '<div class="wv-list" id="wvList">' +
        '<div class="wv-list-header">' +
          '<div class="wv-sub-tabs">' +
            '<button class="wv-sub-tab active" data-subtab="pages" onclick="switchWikiSubTab(\'pages\',this)">Pages</button>' +
            '<button class="wv-sub-tab" data-subtab="decisions" onclick="switchWikiSubTab(\'decisions\',this)">Decisions</button>' +
          '</div>' +
          '<div class="wv-list-actions">' +
            '<button class="wv-action-btn" id="wvIngestBtn" onclick="triggerIngest(this)">Ingest</button>' +
            '<button class="wv-action-btn" id="wvLintBtn" onclick="triggerLint(this)">Lint</button>' +
          '</div>' +
        '</div>' +
        '<div id="wvPageList"><div class="kv-loading">Loading...</div></div>' +
        '<div id="wvDecisionList" style="display:none"><div class="kv-loading">Loading...</div></div>' +
      '</div>' +
      '<div class="wv-content" id="wvContent">' +
        '<div class="wv-empty"><span style="font-size:28px;opacity:.3">&#128214;</span><span>Select a wiki page</span><button class="wv-empty-action" onclick="triggerIngest(document.getElementById(\'wvIngestBtn\'))">Run your first Ingest to compile knowledge</button></div>' +
      '</div>' +
      '<div class="wv-sources-panel" id="wvSourcesPanel">' +
        '<div class="wv-sources-header">Sources</div>' +
        '<div class="wv-sources-body" id="wvSourcesBody"><div class="ctx-empty">Select a page to see sources</div></div>' +
      '</div>' +
      '<div class="wv-lint-panel" id="wvLintPanel">' +
        '<div class="wv-lint-header">Lint Results</div>' +
        '<div id="wvLintContent"><div class="ctx-empty">Run Lint to check wiki health</div></div>' +
      '</div>' +
    '</div>';
  loadWikiPages();
}

export async function loadWikiPages() {
  const container = document.getElementById('wvPageList');
  if (!container) return;
  try {
    const pages = await fetch('/api/wiki', { headers: authHeaders() }).then(r => r.ok ? r.json() : []).catch(() => []);
    wvPages = Array.isArray(pages) ? pages : [];
    if (typeof window !== 'undefined') window.wvPages = wvPages;
    if (wvPages.length === 0) {
      container.innerHTML = '<div class="ctx-empty">No wiki pages yet</div>';
      return;
    }
    // Group by category
    const cats = {};
    wvPages.forEach(p => {
      const cat = p.category || 'Uncategorized';
      if (!cats[cat]) cats[cat] = [];
      cats[cat].push(p);
    });
    let html = '';
    for (const [cat, pages] of Object.entries(cats).sort()) {
      html += '<div class="wv-cat-header">' + esc(cat) + ' (' + pages.length + ')</div>';
      pages.sort((a, b) => (b.mod_time || 0) - (a.mod_time || 0));
      for (const p of pages) {
        const active = wvCurrentPage === p.name ? ' active' : '';
        html += '<div class="wv-page-item' + active + '" data-name="' + escAttr(p.name) + '" onclick="loadWikiPage(\'' + escJs(p.name) + '\')">' +
          '<div class="wv-page-name">' + esc(p.title || p.name) + '</div>' +
          '<div class="wv-page-meta">' +
            (p.compiled_at ? '<span>' + timeAgo(new Date(p.compiled_at).getTime()) + '</span>' : '') +
            (p.sources ? '<span>' + p.sources + ' sources</span>' : '') +
          '</div>' +
        '</div>';
      }
    }
    container.innerHTML = html;
  } catch (e) {
    container.innerHTML = '<div class="ctx-empty">' + esc(e.message) + '</div>';
  }
}

export async function loadWikiPage(name) {
  wvCurrentPage = name;
  // Highlight active page
  document.querySelectorAll('#wvPageList .wv-page-item').forEach(i => i.classList.toggle('active', i.dataset.name === name));
  const content = document.getElementById('wvContent');
  if (!content) return;
  content.innerHTML = '<div class="kv-loading">Loading...</div>';
  try {
    const data = await fetch('/api/wiki/' + encodeURIComponent(name), { headers: authHeaders() }).then(r => {
      if (!r.ok) throw new Error('Page not found');
      return r.json();
    });
    let html = '';
    // Meta bar
    const page = data.page || data;
    const compiled = page.compiled_at ? timeAgo(new Date(page.compiled_at).getTime()) : 'unknown';
    const sources = page.sources || 0;
    const entities = page.entities || [];
    html += '<div class="wv-meta-bar">' +
      '<div class="wv-meta-item">Compiled: ' + esc(compiled) + '</div>' +
      '<div class="wv-meta-item">Sources: ' + sources + '</div>' +
      (entities.length > 0 ? '<div class="wv-meta-item">Entities: ' + entities.map(e => '<span class="wv-entity-tag">' + esc(e) + '</span>').join('') + '</div>' : '') +
    '</div>';
    // Rendered content
    html += '<div class="wv-rendered">' + (data.html || renderMd(page.content || '')) + '</div>';
    content.innerHTML = html;
    // Wire wikilink clicks in wiki content
    content.querySelectorAll('a[data-wiki]').forEach(a => {
      a.addEventListener('click', function(e) {
        e.preventDefault();
        const target = this.getAttribute('data-wiki');
        if (target) loadWikiPage(target);
      });
    });
    // Populate sources panel (Gap 4)
    renderWikiSourcesPanel(page);
  } catch (e) {
    content.innerHTML = '<div class="wv-empty"><span>' + esc(e.message) + '</span></div>';
  }
}

export function renderWikiSourcesPanel(page) {
  const panel = document.getElementById('wvSourcesPanel');
  const body = document.getElementById('wvSourcesBody');
  if (!panel || !body) return;

  const sources = page.sources || 0;
  const entities = page.entities || [];
  const sourceTypes = page.source_types || [];

  // Infer source badges from entities, source_types, or use common defaults
  const knownSources = ['Dashboard', 'CLI', 'Feishu', 'Obsidian'];
  let badges = [];
  if (sourceTypes.length > 0) {
    badges = sourceTypes;
  } else if (sources > 0) {
    // Heuristic: show all known source badges when types are not specified
    badges = knownSources;
  }

  let html = '';
  html += '<div class="wv-src-stat"><strong>' + sources + '</strong> source' + (sources !== 1 ? 's' : '') + ' compiled into this page</div>';

  if (badges.length > 0) {
    html += '<div class="wv-src-badges">';
    badges.forEach(function(b) {
      const cls = 'src-' + b.toLowerCase().replace(/[^a-z]/g, '');
      html += '<span class="wv-src-badge ' + cls + '"><span class="wsb-dot"></span>' + esc(b) + '</span>';
    });
    html += '</div>';
  }

  if (entities.length > 0) {
    html += '<div class="wv-src-entities">';
    html += '<div class="wv-src-entities-title">Entities</div>';
    entities.forEach(function(e) {
      html += '<span class="wv-src-entity">' + esc(e) + '</span>';
    });
    html += '</div>';
  }

  if (!sources && entities.length === 0) {
    html = '<div class="ctx-empty">No source information available</div>';
  }

  body.innerHTML = html;
  panel.classList.add('show');
}

export async function triggerIngest(btn) {
  if (!btn) return;
  btn.classList.add('running');
  btn.textContent = 'Compiling...';
  try {
    const r = await fetch('/api/wiki/ingest', { method: 'POST', headers: authHeaders() });
    if (!r.ok) throw new Error('ingest failed');
    showToast('Ingest started', 'success');
    setTimeout(() => { loadWikiPages(); btn.classList.remove('running'); btn.textContent = 'Ingest'; }, 3000);
  } catch (e) {
    showToast('Ingest failed: ' + e.message);
    btn.classList.remove('running');
    btn.textContent = 'Ingest';
  }
}

export async function triggerLint(btn) {
  if (!btn) return;
  btn.classList.add('running');
  btn.textContent = 'Checking...';
  try {
    const data = await fetch('/api/wiki/lint', { method: 'POST', headers: authHeaders() }).then(r => r.ok ? r.json() : null);
    btn.classList.remove('running');
    btn.textContent = 'Lint';
    if (!data) { showToast('Lint not configured'); return; }
    // Show lint panel
    const panel = document.getElementById('wvLintPanel');
    if (panel) panel.classList.add('show');
    const container = document.getElementById('wvLintContent');
    if (!container) return;
    const issues = data.issues || [];
    if (issues.length === 0) {
      container.innerHTML = '<div class="ctx-empty" style="color:#3fb950">All clear! No issues found.</div>';
      return;
    }
    container.innerHTML = issues.map(i => {
      const badge = '<span class="lint-badge ' + (i.severity || 'info') + '">' + esc(i.severity || 'info') + '</span>';
      return '<div class="wv-lint-item">' + badge + '<span class="lint-page">' + esc(i.page || '') + '</span>' +
        '<div class="lint-msg">' + esc(i.message || '') + '</div></div>';
    }).join('');
  } catch (e) {
    btn.classList.remove('running');
    btn.textContent = 'Lint';
    showToast('Lint failed: ' + e.message);
  }
}

// ------- Decision Journal (Phase 4D) ------------------------------

export function switchWikiSubTab(tab, el) {
  wikiSubTab = tab;
  document.querySelectorAll('.wv-sub-tab').forEach(function(b) {
    b.classList.toggle('active', b.dataset.subtab === tab);
  });
  var pageList = document.getElementById('wvPageList');
  var decisionList = document.getElementById('wvDecisionList');
  if (!pageList || !decisionList) return;
  if (tab === 'pages') {
    pageList.style.display = '';
    decisionList.style.display = 'none';
  } else {
    pageList.style.display = 'none';
    decisionList.style.display = '';
    loadDecisions();
  }
}

export async function loadDecisions() {
  var container = document.getElementById('wvDecisionList');
  if (!container) return;
  container.innerHTML = '<div class="kv-loading">Loading...</div>';
  try {
    var resp = await fetch('/api/decisions', { headers: authHeaders() });
    if (!resp.ok) throw new Error('Failed to load decisions');
    decisionsCache = await resp.json();
    if (!Array.isArray(decisionsCache)) decisionsCache = [];
    renderDecisionList();
  } catch (e) {
    container.innerHTML = '<div class="ctx-empty">' + esc(e.message) + '</div>';
  }
}

export function renderDecisionList() {
  var container = document.getElementById('wvDecisionList');
  if (!container) return;
  var html = '<div style="padding:8px 12px"><button class="dj-add-btn" onclick="showNewDecisionForm()">+ New Decision</button></div>';
  if (decisionsCache.length === 0) {
    html += '<div class="ctx-empty">No decisions recorded yet</div>';
  } else {
    for (var i = 0; i < decisionsCache.length; i++) {
      var d = decisionsCache[i];
      var dateStr = d.created_at ? new Date(d.created_at).toLocaleDateString() : '';
      html += '<div class="dj-card" data-id="' + escAttr(d.id) + '" onclick="showDecisionDetail(\'' + escJs(d.id) + '\')">' +
        '<div class="dj-card-title">' + esc(d.title) + '</div>' +
        '<div class="dj-card-meta"><span>' + esc(dateStr) + '</span>' +
        (d.source ? '<span>from ' + esc(d.source) + '</span>' : '') + '</div>';
      if (d.tags && d.tags.length > 0) {
        html += '<div class="dj-card-tags">';
        d.tags.forEach(function(t) { html += '<span class="dj-card-tag">' + esc(t) + '</span>'; });
        html += '</div>';
      }
      html += '</div>';
    }
  }
  container.innerHTML = html;
}

export function showDecisionDetail(id) {
  var d = decisionsCache.find(function(x) { return x.id === id; });
  if (!d) return;
  // Highlight active card
  document.querySelectorAll('#wvDecisionList .dj-card').forEach(function(c) {
    c.classList.toggle('active', c.dataset.id === id);
  });
  var content = document.getElementById('wvContent');
  if (!content) return;
  var dateStr = d.created_at ? new Date(d.created_at).toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' }) : '';
  var html = '<div style="padding:24px;max-width:720px">' +
    '<h2 style="margin:0 0 4px 0;font-size:20px;color:#e6edf3">' + esc(d.title) + '</h2>' +
    '<div style="font-size:12px;color:#8b949e;margin-bottom:20px">' + esc(dateStr) +
    (d.source ? ' &middot; Source: ' + esc(d.source) : '') + '</div>';
  if (d.tags && d.tags.length > 0) {
    html += '<div style="margin-bottom:16px;display:flex;gap:4px;flex-wrap:wrap">';
    d.tags.forEach(function(t) { html += '<span class="wv-entity-tag">' + esc(t) + '</span>'; });
    html += '</div>';
  }
  if (d.context) {
    html += '<div class="dj-detail-section"><h4>Context</h4><p>' + esc(d.context) + '</p></div>';
  }
  if (d.decision) {
    html += '<div class="dj-detail-section"><h4>Decision</h4><p>' + esc(d.decision) + '</p></div>';
  }
  if (d.consequences) {
    html += '<div class="dj-detail-section"><h4>Consequences</h4><p>' + esc(d.consequences) + '</p></div>';
  }
  html += '</div>';
  content.innerHTML = html;
}

export function showNewDecisionForm() {
  var content = document.getElementById('wvContent');
  if (!content) return;
  content.innerHTML =
    '<div style="padding:24px;max-width:600px">' +
      '<h2 style="margin:0 0 16px;font-size:18px;color:#e6edf3">New Decision</h2>' +
      '<div style="margin-bottom:12px"><label style="display:block;font-size:12px;color:#8b949e;margin-bottom:4px">Title</label>' +
        '<input type="text" id="djTitle" style="width:100%;padding:8px;border-radius:6px;border:1px solid #30363d;background:#0d1117;color:#e6edf3;font-size:14px;font-family:inherit;box-sizing:border-box" placeholder="e.g. Use CDK over Terraform"></div>' +
      '<div style="margin-bottom:12px"><label style="display:block;font-size:12px;color:#8b949e;margin-bottom:4px">Context</label>' +
        '<textarea id="djContext" rows="3" style="width:100%;padding:8px;border-radius:6px;border:1px solid #30363d;background:#0d1117;color:#e6edf3;font-size:14px;font-family:inherit;resize:vertical;box-sizing:border-box" placeholder="Why was this decision needed?"></textarea></div>' +
      '<div style="margin-bottom:12px"><label style="display:block;font-size:12px;color:#8b949e;margin-bottom:4px">Decision</label>' +
        '<textarea id="djDecision" rows="3" style="width:100%;padding:8px;border-radius:6px;border:1px solid #30363d;background:#0d1117;color:#e6edf3;font-size:14px;font-family:inherit;resize:vertical;box-sizing:border-box" placeholder="What was decided?"></textarea></div>' +
      '<div style="margin-bottom:12px"><label style="display:block;font-size:12px;color:#8b949e;margin-bottom:4px">Consequences</label>' +
        '<textarea id="djConsequences" rows="3" style="width:100%;padding:8px;border-radius:6px;border:1px solid #30363d;background:#0d1117;color:#e6edf3;font-size:14px;font-family:inherit;resize:vertical;box-sizing:border-box" placeholder="Impact and trade-offs"></textarea></div>' +
      '<div style="margin-bottom:16px"><label style="display:block;font-size:12px;color:#8b949e;margin-bottom:4px">Tags (comma-separated)</label>' +
        '<input type="text" id="djTags" style="width:100%;padding:8px;border-radius:6px;border:1px solid #30363d;background:#0d1117;color:#e6edf3;font-size:14px;font-family:inherit;box-sizing:border-box" placeholder="aws, infrastructure, cdk"></div>' +
      '<button onclick="submitNewDecision()" style="padding:8px 20px;border-radius:6px;border:1px solid #1f6feb;background:rgba(31,111,235,.15);color:#58a6ff;cursor:pointer;font-size:13px;font-family:inherit">Save Decision</button>' +
    '</div>';
}

export async function submitNewDecision() {
  var title = (document.getElementById('djTitle') || {}).value || '';
  var context = (document.getElementById('djContext') || {}).value || '';
  var decision = (document.getElementById('djDecision') || {}).value || '';
  var consequences = (document.getElementById('djConsequences') || {}).value || '';
  var tagsStr = (document.getElementById('djTags') || {}).value || '';
  if (!title.trim()) { alert('Title is required'); return; }
  var tags = tagsStr.split(',').map(function(t) { return t.trim(); }).filter(Boolean);
  try {
    var resp = await fetch('/api/decisions', {
      method: 'POST',
      headers: Object.assign({ 'Content-Type': 'application/json' }, authHeaders()),
      body: JSON.stringify({ title: title, context: context, decision: decision, consequences: consequences, tags: tags, source: 'manual' })
    });
    if (!resp.ok) throw new Error('Failed to save decision');
    loadDecisions();
    document.getElementById('wvContent').innerHTML =
      '<div class="wv-empty"><span style="font-size:28px;opacity:.3">&#9989;</span><span>Decision saved</span></div>';
  } catch (e) {
    alert('Error: ' + e.message);
  }
}

// ------- Legacy bridges — removed after Phase 2 -------------------

if (typeof window !== 'undefined') {
  // Ensure home.js sees the latest wvPages even before loadWikiPages runs.
  window.wvPages = wvPages;

  window.renderWikiView = renderWikiView;
  window.loadWikiPages = loadWikiPages;
  window.loadWikiPage = loadWikiPage;
  window.renderWikiSourcesPanel = renderWikiSourcesPanel;
  window.triggerIngest = triggerIngest;
  window.triggerLint = triggerLint;
  window.switchWikiSubTab = switchWikiSubTab;
  window.loadDecisions = loadDecisions;
  window.renderDecisionList = renderDecisionList;
  window.showDecisionDetail = showDecisionDetail;
  window.showNewDecisionForm = showNewDecisionForm;
  window.submitNewDecision = submitNewDecision;
}
