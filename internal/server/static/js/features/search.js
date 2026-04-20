// internal/server/static/js/features/search.js
// Cmd+K search overlay + result rendering.
import { apiFetch } from '../core/api.js';
import { esc, html } from '../core/html.js';

let initialized = false;
function ensureInit() {
  if (initialized) return;
  initialized = true;
}

// --- Begin extracted from legacy.js:2087-2376 ---
function navigateSearchResult(source, pathOrKey) {
  closeSearch();
  if (source === 'vault') {
    switchView('knowledge', document.querySelector('[data-view=knowledge]'));
    setTimeout(function() { loadVaultFile(pathOrKey); }, 150);
  } else if (source === 'wiki') {
    switchView('wiki', document.querySelector('[data-view=wiki]'));
    setTimeout(function() { loadWikiPage(pathOrKey); }, 150);
  } else if (pathOrKey) {
    switchView('chat', document.querySelector('[data-view=chat]'));
    selectSession(pathOrKey, 'local');
  }
}

/* ── Cmd+K Global Search ── */
var _searchSelectedIdx = -1;
var _searchResults = []; // [{key, node, type}]
var _searchInputBound = false;

function openSearch() {
  var ov = document.getElementById('searchOverlay');
  if (!ov) return;
  ov.classList.add('show');
  var inp = document.getElementById('searchInput');
  if (inp) {
    inp.value = '';
    inp.focus();
    if (!_searchInputBound) {
      _searchInputBound = true;
      var _searchDebounce = null;
      inp.addEventListener('input', function() {
        clearTimeout(_searchDebounce);
        _searchDebounce = setTimeout(function() {
          _performSearch(inp.value.trim());
        }, 300);
      });
    }
  }
  _searchSelectedIdx = -1;
  _searchResults = [];
  var sr = document.getElementById('searchResults');
  if (sr) sr.innerHTML = '<div class="sm-hint">Type to search across sessions and messages</div>';
}

function closeSearch() {
  var ov = document.getElementById('searchOverlay');
  if (ov) ov.classList.remove('show');
  _searchSelectedIdx = -1;
  _searchResults = [];
}

function _searchHighlight(text, query) {
  if (!text || !query) return esc(text || '');
  var escaped = esc(text);
  var qEsc = query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  try {
    return escaped.replace(new RegExp('(' + qEsc + ')', 'gi'), '<mark>$1</mark>');
  } catch(e) {
    return escaped;
  }
}

function _searchContextSnippet(text, query, maxLen) {
  maxLen = maxLen || 80;
  if (!text || !query) return '';
  var idx = text.toLowerCase().indexOf(query.toLowerCase());
  if (idx === -1) return text.substring(0, maxLen);
  var start = Math.max(0, idx - 30);
  var end = Math.min(text.length, idx + query.length + 50);
  var snippet = (start > 0 ? '...' : '') + text.substring(start, end) + (end < text.length ? '...' : '');
  return snippet;
}

async function _performSearch(query) {
  var results = document.getElementById('searchResults');
  if (!results) return;
  if (!query || query.length < 1) {
    results.innerHTML = '<div class="sm-hint">Type to search across sessions and messages</div>';
    _searchResults = [];
    _searchSelectedIdx = -1;
    return;
  }

  var q = query.toLowerCase();
  var sessionMatches = [];
  var messageMatches = [];

  // Search sessions from allSessionsCache
  for (var i = 0; i < allSessionsCache.length; i++) {
    var s = allSessionsCache[i];
    var name = s.name || '';
    var prompt = s.last_prompt || s.summary || '';
    var key = s.key || '';
    var sNode = s.node || 'local';

    var nameMatch = name.toLowerCase().indexOf(q) !== -1;
    var promptMatch = prompt.toLowerCase().indexOf(q) !== -1;
    var keyMatch = key.toLowerCase().indexOf(q) !== -1;

    if (nameMatch || promptMatch || keyMatch) {
      sessionMatches.push({
        key: key,
        node: sNode,
        name: name || prompt || key,
        matchField: nameMatch ? 'name' : (promptMatch ? 'prompt' : 'key'),
        matchText: nameMatch ? name : (promptMatch ? prompt : key),
        state: s.state || '',
        lastActive: s.last_active || 0
      });
    }
    if (sessionMatches.length >= 10) break;
  }

  // Search message content from sessionsData (loaded events)
  var sKeys = Object.keys(sessionsData);
  for (var si = 0; si < sKeys.length && messageMatches.length < 15; si++) {
    var sd = sessionsData[sKeys[si]];
    if (!sd || !sd.key) continue;
    // sessionsData stores session objects; events are rendered from WS history
    // Check session-level fields that might contain message content
    var fields = [sd.last_prompt, sd.summary, sd.name];
    // Also check if there's any cached detail/content
    for (var fi = 0; fi < fields.length; fi++) {
      var f = fields[fi];
      if (!f) continue;
      if (f.toLowerCase().indexOf(q) !== -1) {
        // Avoid duplicate if already in session matches
        var isDupe = false;
        for (var di = 0; di < sessionMatches.length; di++) {
          if (sessionMatches[di].key === sd.key) { isDupe = true; break; }
        }
        if (!isDupe) {
          messageMatches.push({
            key: sd.key,
            node: sd.node || 'local',
            sessionName: sd.name || sd.last_prompt || sd.key,
            matchText: f,
            type: fi === 0 ? 'prompt' : (fi === 1 ? 'summary' : 'name')
          });
        }
        break;
      }
    }
  }

  // Also search historySessionsData for broader coverage
  for (var hi = 0; hi < historySessionsData.length && messageMatches.length < 15; hi++) {
    var h = historySessionsData[hi];
    var hPrompt = h.last_prompt || h.summary || '';
    if (hPrompt.toLowerCase().indexOf(q) !== -1) {
      // Check not already in session matches
      var hDupe = false;
      for (var hdi = 0; hdi < sessionMatches.length; hdi++) {
        if (sessionMatches[hdi].key === h.session_id) { hDupe = true; break; }
      }
      for (var hdi2 = 0; hdi2 < messageMatches.length; hdi2++) {
        if (messageMatches[hdi2].key === h.session_id) { hDupe = true; break; }
      }
      if (!hDupe) {
        messageMatches.push({
          key: h.session_id || '',
          node: 'local',
          sessionName: hPrompt.substring(0, 60),
          matchText: hPrompt,
          type: 'history',
          isHistory: true,
          workspace: h.workspace || ''
        });
      }
    }
  }

  // Task 16: Also query /api/search for knowledge results
  var knowledgeMatches = [];
  try {
    var apiResp = await fetch('/api/search?q=' + encodeURIComponent(query) + '&source=all&limit=10', { headers: authHeaders() });
    if (apiResp.ok) {
      var apiData = await apiResp.json();
      knowledgeMatches = (apiData.results || []).slice(0, 10);
    }
  } catch(_) {}

  // Build results HTML
  _searchResults = [];
  var html = '';

  if (sessionMatches.length === 0 && messageMatches.length === 0 && knowledgeMatches.length === 0) {
    results.innerHTML = '<div class="sm-empty">No results for "' + esc(query) + '"</div>';
    _searchSelectedIdx = -1;
    return;
  }

  if (sessionMatches.length > 0) {
    html += '<div class="sm-group">Sessions</div>';
    for (var ri = 0; ri < sessionMatches.length; ri++) {
      var r = sessionMatches[ri];
      var idx = _searchResults.length;
      _searchResults.push({ key: r.key, node: r.node, type: 'session' });
      var stateIcon = r.state === 'running' ? '\u{1f7e2}' : (r.state === 'ready' ? '\u{1f535}' : '\u26aa');
      var snippet = _searchContextSnippet(r.matchText, query);
      html += '<div class="sm-item" data-idx="' + idx + '" onclick="_searchSelect(' + idx + ')" onmouseenter="_searchHover(' + idx + ')">' +
        '<div class="si-icon">' + stateIcon + '</div>' +
        '<div class="si-body">' +
          '<div class="si-title">' + _searchHighlight(r.name.substring(0, 80), query) + '</div>' +
          (r.matchField !== 'name' && snippet ? '<div class="si-match">' + _searchHighlight(snippet, query) + '</div>' : '') +
          '<div class="si-meta">' + esc(r.state) + (r.lastActive ? ' \u00b7 ' + timeAgo(r.lastActive) : '') + '</div>' +
        '</div></div>';
    }
  }

  if (messageMatches.length > 0) {
    html += '<div class="sm-group">Messages</div>';
    for (var mi = 0; mi < messageMatches.length; mi++) {
      var m = messageMatches[mi];
      var midx = _searchResults.length;
      _searchResults.push({ key: m.key, node: m.node, type: m.isHistory ? 'history' : 'message' });
      var mSnippet = _searchContextSnippet(m.matchText, query);
      html += '<div class="sm-item" data-idx="' + midx + '" onclick="_searchSelect(' + midx + ')" onmouseenter="_searchHover(' + midx + ')">' +
        '<div class="si-icon">\u{1f4ac}</div>' +
        '<div class="si-body">' +
          '<div class="si-title">' + esc(m.sessionName.substring(0, 60)) + '</div>' +
          '<div class="si-match">' + _searchHighlight(mSnippet, query) + '</div>' +
          '<div class="si-meta">' + esc(m.type) + '</div>' +
        '</div></div>';
    }
  }

  // Task 16: Knowledge results section from /api/search
  if (knowledgeMatches.length > 0) {
    var srcIcons = { vault: '\u{1F4D6}', wiki: '\u{1F4DA}', bookmark: '\u{1F516}', dashboard: '\u{1F4AC}', cli: '\u{1F4BB}' };
    html += '<div class="sm-group">Knowledge</div>';
    for (var ki = 0; ki < knowledgeMatches.length; ki++) {
      var km = knowledgeMatches[ki];
      var kidx = _searchResults.length;
      _searchResults.push({ key: km.path || km.session_key || km.title || '', node: 'local', type: 'knowledge', source: km.source || '' });
      var kIcon = srcIcons[km.source] || '\u{1F50D}';
      var kSnippet = km.match || '';
      html += '<div class="sm-item" data-idx="' + kidx + '" onclick="_searchSelect(' + kidx + ')" onmouseenter="_searchHover(' + kidx + ')">' +
        '<div class="si-icon">' + kIcon + '</div>' +
        '<div class="si-body">' +
          '<div class="si-title">' + _searchHighlight((km.title || '').substring(0, 80), query) + '</div>' +
          (kSnippet ? '<div class="si-match">' + _searchHighlight(kSnippet.substring(0, 100), query) + '</div>' : '') +
          '<div class="si-meta">' + esc(km.source || '') + (km.timestamp ? ' \u00b7 ' + timeAgo(km.timestamp) : '') + '</div>' +
        '</div></div>';
    }
  }

  results.innerHTML = html;
  _searchSelectedIdx = -1;
}

function _searchSelect(idx) {
  if (idx < 0 || idx >= _searchResults.length) return;
  var r = _searchResults[idx];
  closeSearch();
  if (r.type === 'knowledge') {
    navigateSearchResult(r.source || '', r.key);
  } else if (r.type === 'history') {
    // History session — resume via session_id
    if (typeof resumeRecentSession === 'function') {
      resumeRecentSession(r.key);
    }
  } else {
    selectSession(r.key, r.node);
  }
}

function _searchHover(idx) {
  if (idx === _searchSelectedIdx) return;
  _searchSelectedIdx = idx;
  _searchUpdateSelection();
}

function _searchUpdateSelection() {
  var items = document.querySelectorAll('#searchResults .sm-item');
  for (var i = 0; i < items.length; i++) {
    if (parseInt(items[i].dataset.idx) === _searchSelectedIdx) {
      items[i].classList.add('selected');
      items[i].scrollIntoView({ block: 'nearest' });
    } else {
      items[i].classList.remove('selected');
    }
  }
}
// --- End extracted ---

export async function open(...a) { ensureInit(); return openSearch(...a); }
export async function close(...a) { ensureInit(); return closeSearch(...a); }

// Arrow/Enter exports for the keyboard dispatcher in legacy.js (plan Step 5 option b)
export async function arrow(dir) {
  ensureInit();
  if (_searchResults.length > 0) {
    if (dir === 'down') {
      _searchSelectedIdx = (_searchSelectedIdx + 1) % _searchResults.length;
    } else {
      _searchSelectedIdx = _searchSelectedIdx <= 0 ? _searchResults.length - 1 : _searchSelectedIdx - 1;
    }
    _searchUpdateSelection();
  }
}

export async function enter() {
  ensureInit();
  if (_searchSelectedIdx >= 0) {
    _searchSelect(_searchSelectedIdx);
  }
}
