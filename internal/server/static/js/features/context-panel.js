// internal/server/static/js/features/context-panel.js
// Context Panel: session saved/related/bookmark sidebar.
import { authHeaders } from '../core/api.js';
import { esc, escAttr, escJs } from '../core/html.js';

let initialized = false;
function ensureInit() {
  if (initialized) return;
  initialized = true;
}

// --- Begin extracted from legacy.js:1869-1975 ---
var ctxPanelOpen = false;
var ctxActiveTab = 'saved';

function toggleCtxPanel() {
  ctxPanelOpen = !ctxPanelOpen;
  const panel = document.getElementById('ctxPanel');
  const toggle = document.getElementById('ctxToggle');
  if (panel) panel.classList.toggle('open', ctxPanelOpen);
  if (toggle) toggle.innerHTML = ctxPanelOpen ? '&#9664;' : '&#9654;';
  if (ctxPanelOpen) refreshCtxPanel();
}

function switchCtxTab(tab, el) {
  ctxActiveTab = tab;
  document.querySelectorAll('#ctxPanel .ctx-tab').forEach(t => t.classList.toggle('active', t.dataset.tab === tab));
  refreshCtxPanel();
}

async function refreshCtxPanel() {
  const body = document.getElementById('ctxBody');
  if (!body) return;
  if (ctxActiveTab === 'saved') {
    await loadCtxSaved(body);
  } else if (ctxActiveTab === 'related') {
    await loadCtxRelated(body);
  } else {
    body.innerHTML = '<div class="ctx-ai-placeholder"><div style="font-size:24px;opacity:.3;margin-bottom:8px">&#129302;</div>Ask about this session<br><span style="font-size:11px;color:#484f58">(Coming in Phase 3)</span></div>';
  }
}

async function loadCtxSaved(body) {
  if (!selectedKey) {
    body.innerHTML = '<div class="ctx-empty">Select a session to see bookmarks</div>';
    return;
  }
  try {
    const bms = await fetch('/api/bookmarks?session=' + encodeURIComponent(selectedKey), { headers: authHeaders() }).then(r => r.ok ? r.json() : []).catch(() => []);
    const list = Array.isArray(bms) ? bms : [];
    if (list.length === 0) {
      body.innerHTML = '<div class="ctx-empty">No bookmarks for this session.<br><span style="font-size:11px">Hover AI messages and click the bookmark icon to save.</span></div>';
      return;
    }
    body.innerHTML = list.map(b =>
      '<div class="ctx-bm-card" data-bm-id="' + escAttr(b.id || '') + '">' +
        '<button class="ctx-bm-del" onclick="deleteBookmark(\'' + escJs(b.id || '') + '\')" title="Remove">&times;</button>' +
        '<div class="ctx-bm-text">' + esc((b.content || b.summary || '').substring(0, 300)) + '</div>' +
        '<div class="ctx-bm-meta">' +
          '<span class="ctx-source-badge src-' + esc(b.source || 'dashboard') + '">' + esc(b.source || 'dashboard') + '</span>' +
          '<span>' + (b.created_at ? timeAgo(b.created_at) : '') + '</span>' +
        '</div>' +
        (b.tags && b.tags.length > 0 ? '<div class="ctx-bm-tags">' + b.tags.map(t => '<span class="ctx-bm-tag">' + esc(t) + '</span>').join('') + '</div>' : '') +
      '</div>'
    ).join('');
  } catch (e) {
    body.innerHTML = '<div class="ctx-empty">Failed to load bookmarks</div>';
  }
}

async function loadCtxRelated(body) {
  if (!selectedKey) {
    body.innerHTML = '<div class="ctx-empty">Select a session to find related content</div>';
    return;
  }
  // Extract keywords from current session
  const s = sessionsData[sid(selectedKey, selectedNode)] || {};
  const keywords = (s.name || s.last_prompt || s.summary || '').substring(0, 100);
  if (!keywords) {
    body.innerHTML = '<div class="ctx-empty">No keywords to match</div>';
    return;
  }
  try {
    const data = await fetch('/api/search?q=' + encodeURIComponent(keywords) + '&limit=8', { headers: authHeaders() }).then(r => r.ok ? r.json() : { results: [] }).catch(() => ({ results: [] }));
    const results = data.results || [];
    if (results.length === 0) {
      body.innerHTML = '<div class="ctx-empty">No related content found</div>';
      return;
    }
    body.innerHTML = results.map(r =>
      '<div class="ctx-bm-card" style="cursor:pointer" onclick="navigateSearchResult(\'' + escJs(r.source || '') + '\',\'' + escJs(r.path || r.session_key || '') + '\')">' +
        '<div class="ctx-bm-text">' + esc((r.title || '').substring(0, 100)) + '</div>' +
        '<div class="ctx-bm-meta">' +
          '<span class="ctx-source-badge src-' + esc(r.source || '') + '">' + esc(r.source || '') + '</span>' +
          (r.match ? '<span>' + esc(r.match.substring(0, 60)) + '</span>' : '') +
        '</div>' +
      '</div>'
    ).join('');
  } catch (e) {
    body.innerHTML = '<div class="ctx-empty">Search failed</div>';
  }
}

async function deleteBookmark(id) {
  if (!id) return;
  try {
    await fetch('/api/bookmarks/' + encodeURIComponent(id), { method: 'DELETE', headers: authHeaders() });
    showToast('Bookmark removed', 'success');
    if (ctxPanelOpen && ctxActiveTab === 'saved') refreshCtxPanel();
    // Reset saved state on message buttons
    document.querySelectorAll('.bm-hover-btn.saved[data-bm-id="' + id + '"]').forEach(b => {
      b.classList.remove('saved');
      b.removeAttribute('data-bm-id');
    });
  } catch (e) { showToast('Delete failed'); }
}
// --- End extracted ---

export async function open(...args) {
  ensureInit();
  return toggleCtxPanel(...args);
}

// Internal-only helpers exposed for other feature modules that may need them:
export { refreshCtxPanel, deleteBookmark };
