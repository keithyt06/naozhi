// js/legacy.js — remaining "plumbing" lifted verbatim from
// dashboard.html's inline <script> (Plan 1 Task 13.4/13.5).
//
// Holds: shared helpers (showToast, renderDiff, renderMd,
// renderTable, loadMermaid/Katex), discovery & takeover, mobile nav
// + tab bar + copy-tap, cron tab, notification center, DOMContentLoaded
// bootstrap, file hub, /ls enhanced rendering, view-switching stub,
// context panel overlays, bookmark UI, replay overlay, twin overlay,
// cmd-K search. This file is temporary — Phase 4 splits it further
// into components/*. Loaded with `defer` from dashboard.html.

window.onerror = function(msg, url, line, col, err) {
  var d = document.createElement('div');
  d.style.cssText = 'position:fixed;top:0;left:0;right:0;z-index:99999;background:#da3633;color:#fff;padding:8px 16px;font-size:12px;font-family:monospace';
  d.textContent = 'JS ERROR: ' + msg + ' (line ' + line + ')';
  document.body.appendChild(d);
  console.error('NAOZHI ERROR:', msg, 'line:', line, 'col:', col, err);
};
// State globals now live in js/core/state.js (window bridge).

// --- Utilities ---

function showToast(msg, type, duration) {
  const el = document.getElementById('toast');
  el.textContent = msg;
  el.className = 'toast show' + (type ? ' ' + type : '');
  clearTimeout(el._tid);
  el._tid = setTimeout(() => { el.className = 'toast'; }, duration || 3000);
}

function fallbackCopy(text) {
  const ta = document.createElement('textarea');
  ta.value = text;
  ta.style.cssText = 'position:fixed;left:-9999px';
  document.body.appendChild(ta);
  ta.select();
  document.execCommand('copy');
  document.body.removeChild(ta);
}

function copyText(text) {
  if (navigator.clipboard) {
    navigator.clipboard.writeText(text).then(() => showToast('copied', 'success')).catch(() => { fallbackCopy(text); showToast('copied', 'success'); });
  } else {
    fallbackCopy(text);
    showToast('copied', 'success');
  }
}

function copyCodeBlock(btn) {
  const code = btn.closest('.md-code-wrap').querySelector('code').textContent;
  const done = () => { btn.textContent = 'copied!'; btn.classList.add('copied'); setTimeout(() => { btn.textContent = 'copy'; btn.classList.remove('copied'); }, 1500); };
  if (navigator.clipboard) {
    navigator.clipboard.writeText(code).then(done).catch(() => { fallbackCopy(code); done(); });
  } else {
    fallbackCopy(code);
    done();
  }
}

function highlightBlock(preEl) {
  if (!window.shikiHighlighter) return;
  const lang = preEl.getAttribute('data-lang') || 'text';
  const code = preEl.textContent;
  try {
    const html = window.shikiHighlighter.codeToHtml(code, { lang, theme: 'github-dark' });
    const wrapper = preEl.closest('.code-block');
    if (wrapper) {
      const codeArea = wrapper.querySelector('.code-area');
      if (codeArea) codeArea.innerHTML = html;
    }
  } catch(e) { /* fallback: keep plain text */ }
}

function copyCode(btn) {
  const block = btn.closest('.code-block');
  const code = block.querySelector('pre')?.textContent || block.getAttribute('data-code') || '';
  const done = () => { btn.textContent = '\u2713 Copied'; btn.classList.add('copied'); setTimeout(() => { btn.textContent = 'Copy'; btn.classList.remove('copied'); }, 1500); };
  if (navigator.clipboard) {
    navigator.clipboard.writeText(code).then(done).catch(() => { fallbackCopy(code); done(); });
  } else {
    fallbackCopy(code);
    done();
  }
}

function shortPath(p) {
  const home = '/home/';
  const i = p.indexOf(home);
  if (i >= 0) {
    const rest = p.substring(i + home.length);
    const slash = rest.indexOf('/');
    if (slash >= 0) return '~' + rest.substring(slash);
  }
  return p.length > 40 ? '...' + p.substring(p.length - 37) : p;
}

function timeAgo(ms, future) {
  if (!ms) return '\u2014';
  const d = future ? ms - Date.now() : Date.now() - ms;
  if (d < 0) return future ? 'now' : 'just now';
  const suffix = future ? '' : ' ago';
  if (d < 5000) return future ? 'now' : 'just now';
  if (d < 60000) return Math.floor(d/1000) + 's' + suffix;
  if (d < 3600000) return Math.floor(d/60000) + 'm' + suffix;
  if (d < 86400000) return Math.floor(d/3600000) + 'h' + suffix;
  return Math.floor(d/86400000) + 'd' + suffix;
}

function sessionTimeHint(key) {
  const m = (key || '').match(/:(\d{4})-(\d{2})-(\d{2})-(\d{2})(\d{2})(\d{2})/);
  if (m) return m[2] + '/' + m[3] + ' ' + m[4] + ':' + m[5];
  return '\u2014';
}

/* Diff rendering for Edit tool (Task 3) */
function renderDiff(oldStr, newStr, filePath) {
  if (!oldStr && !newStr) return '';
  var oldLines = oldStr ? oldStr.split('\n') : [];
  var newLines = newStr ? newStr.split('\n') : [];
  var added = 0, removed = 0, html = '';
  oldLines.forEach(function(l) { removed++; html += '<div class="diff-line del">- ' + esc(l) + '</div>'; });
  newLines.forEach(function(l) { added++; html += '<div class="diff-line add">+ ' + esc(l) + '</div>'; });
  var hdr = '<div class="diff-hdr"><span class="diff-file">' + esc(filePath || '') + '</span><span class="diff-stat"><span class="add">+' + added + '</span> <span class="del">-' + removed + '</span></span></div>';
  return '<div class="diff-block">' + hdr + html + '</div>';
}

/* Long output collapse toggle (Task 4) */
function toggleCollapse(btn) {
  var wrap = btn.closest('.collapse-wrap');
  wrap.classList.toggle('expanded');
  btn.textContent = wrap.classList.contains('expanded') ? '\u25B2 Show less' : '\u25BC Show more';
}

let mermaidLoading = false;
let mermaidReady = false;

function loadMermaid() {
  if (mermaidReady || mermaidLoading) return;
  mermaidLoading = true;
  const s = document.createElement('script');
  s.src = 'https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.min.js';
  s.integrity = 'sha384-tI0sDqjGJcqrQ8e/XKiQGS+ee11v5knTNWx2goxMBxe4DO9U0uKlfxJtYB9ILZ4j';
  s.crossOrigin = 'anonymous';
  s.onload = () => {
    mermaid.initialize({ startOnLoad: false, theme: 'dark', securityLevel: 'strict' });
    mermaidReady = true;
    mermaidLoading = false;
    runMermaid();
  };
  s.onerror = () => { mermaidLoading = false; };
  document.head.appendChild(s);
}

function runMermaid() {
  if (Object.keys(mermaidPending).length === 0) return;
  if (!mermaidReady) { loadMermaid(); return; }
  let hasNew = false;
  Object.entries(mermaidPending).forEach(([id, code]) => {
    const el = document.getElementById(id);
    if (!el) { delete mermaidPending[id]; return; }
    el.textContent = code;
    el.className = 'mermaid';
    delete mermaidPending[id];
    hasNew = true;
  });
  if (hasNew) mermaid.run({ nodes: document.querySelectorAll('.mermaid') });
}

let mermaidCounter = 0;
const mermaidPending = {};

let katexLoading = false;
let katexReady = false;
let katexCounter = 0;
const katexPending = {};

function loadKatex() {
  if (katexReady || katexLoading) return;
  katexLoading = true;
  const s = document.createElement('script');
  s.src = 'https://cdn.jsdelivr.net/npm/katex@0.16.21/dist/katex.min.js';
  s.integrity = 'sha384-Rma6DA2IPUwhNxmrB/7S3Tno0YY7sFu9WSYMCuulLhIqYSGZ2gKCJWIqhBWqMQfh';
  s.crossOrigin = 'anonymous';
  s.onload = () => {
    katexReady = true;
    katexLoading = false;
    runKatex();
  };
  s.onerror = (e) => { katexLoading = false; console.error('[KaTeX] failed to load:', e); };
  document.head.appendChild(s);
}

function runKatex() {
  if (Object.keys(katexPending).length === 0) return;
  if (!katexReady) { loadKatex(); return; }
  Object.entries(katexPending).forEach(([id, info]) => {
    const el = document.getElementById(id);
    if (!el) { delete katexPending[id]; return; }
    try {
      katex.render(info.tex, el, { displayMode: info.display, throwOnError: false });
    } catch(_) {
      el.textContent = (info.display ? '$$' : '$') + info.tex + (info.display ? '$$' : '$');
    }
    delete katexPending[id];
  });
}

function renderKatex(tex, displayMode) {
  if (katexReady) {
    try { return katex.renderToString(tex, { displayMode: displayMode, throwOnError: false }); }
    catch(_) { return esc(tex); }
  }
  const id = 'ktx-' + (++katexCounter);
  katexPending[id] = { tex: tex, display: displayMode };
  loadKatex();
  return '<span id="' + id + '" class="katex-pending">' + esc(tex) + '</span>';
}

/* Lightweight Markdown renderer for text/result events */

function renderMd(s) {
  if (!s) return '';
  // Split by fenced code blocks and display math blocks
  const parts = s.split(/(```[\s\S]*?```|\$\$[\s\S]*?\$\$|\\\[[\s\S]*?\\\])/g);
  return parts.map(part => {
    if (part.startsWith('```')) {
      const m = part.match(/^```(\w*)\n?([\s\S]*?)```$/);
      const lang = m ? m[1] : '';
      const code = m ? m[2].replace(/\n$/, '') : part.slice(3, -3);
      if (lang === 'mermaid') {
        const id = 'mmd-' + (++mermaidCounter);
        mermaidPending[id] = code;
        return '<div class="mermaid-wrap"><pre id="' + id + '" class="mermaid-pending"></pre></div>';
      }
      const langAttr = lang ? ' data-lang="' + escAttr(lang) + '"' : '';
      const langLabel = lang || 'text';
      // Try Shiki highlighting if available
      let codeAreaHtml;
      if (window.shikiHighlighter && lang) {
        try {
          codeAreaHtml = window.shikiHighlighter.codeToHtml(code, { lang: lang, theme: 'github-dark' });
        } catch(e) { codeAreaHtml = null; }
      }
      if (!codeAreaHtml) {
        codeAreaHtml = '<pre class="md-pre"' + langAttr + '><code' + langAttr + '>' + esc(code) + '</code></pre>';
      }
      return '<div class="code-block" data-code="' + escAttr(code) + '"><div class="code-hdr"><span class="code-lang">' + esc(langLabel) + '</span><div class="code-actions"><button class="code-btn" onclick="copyCode(this)">Copy</button></div></div><div class="code-area">' + codeAreaHtml + '</div></div>';
    }
    if (part.startsWith('$$') && part.endsWith('$$')) {
      return '<div class="md-math-display">' + renderKatex(part.slice(2, -2).trim(), true) + '</div>';
    }
    if (part.startsWith('\\[') && part.endsWith('\\]')) {
      return '<div class="md-math-display">' + renderKatex(part.slice(2, -2).trim(), true) + '</div>';
    }
    // Process line by line for block elements
    const lines = part.split('\n');
    let html = '';
    let inList = '';
    for (let i = 0; i < lines.length; i++) {
      let line = lines[i];
      // Headings
      const hm = line.match(/^(#{1,4})\s+(.+)$/);
      if (hm) {
        if (inList) { html += '</' + inList + '>'; inList = ''; }
        const level = hm[1].length;
        html += '<strong class="md-h' + level + '">' + inlineMd(hm[2]) + '</strong>\n';
        continue;
      }
      // Unordered list
      if (/^\s*[-*]\s+/.test(line)) {
        if (inList === 'ol') { html += '</ol>'; inList = ''; }
        if (!inList) { html += '<ul class="md-ul">'; inList = 'ul'; }
        html += '<li>' + inlineMd(line.replace(/^\s*[-*]\s+/, '')) + '</li>';
        continue;
      }
      // Ordered list
      if (/^\s*\d+\.\s+/.test(line)) {
        if (inList === 'ul') { html += '</ul>'; inList = ''; }
        if (!inList) { html += '<ol class="md-ol">'; inList = 'ol'; }
        html += '<li>' + inlineMd(line.replace(/^\s*\d+\.\s+/, '')) + '</li>';
        continue;
      }
      if (line === '') {
        if (inList) {
          // Look ahead: if next non-blank line continues the list, keep it open
          let peek = i + 1;
          while (peek < lines.length && lines[peek] === '') peek++;
          if (peek < lines.length) {
            let nl = lines[peek];
            if ((inList === 'ol' && /^\s*\d+\.\s+/.test(nl)) ||
                (inList === 'ul' && /^\s*[-*]\s+/.test(nl))) {
              continue;
            }
          }
          html += '</' + inList + '>'; inList = '';
        }
        html += '<div class="md-blank"></div>';
        continue;
      }
      if (inList) { html += '</' + inList + '>'; inList = ''; }
      if (/^\|.+\|$/.test(line.trim())) {
        let tbl = [line];
        while (i + 1 < lines.length && /^\|.+\|$/.test(lines[i + 1].trim())) { tbl.push(lines[++i]); }
        html += renderTable(tbl);
        continue;
      }
      html += inlineMd(line) + '<br>';
    }
    if (inList) html += '</' + inList + '>';
    return html;
  }).join('');
}

/* Inline markdown: bold, italic, code, links, math */
function inlineMd(s) {
  // Extract inline math before HTML escaping. Use \x00 delimiters to avoid collisions with user content.
  const mathTokens = [];
  s = s.replace(/\$([^\$\n]+?)\$/g, function(_, tex) {
    const idx = mathTokens.length;
    mathTokens.push(renderKatex(tex, false));
    return '\x00KTX' + idx + '\x00';
  });
  s = s.replace(/\\\((.+?)\\\)/g, function(_, tex) {
    const idx = mathTokens.length;
    mathTokens.push(renderKatex(tex, false));
    return '\x00KTX' + idx + '\x00';
  });
  s = esc(s);
  s = s.replace(/`([^`]+)`/g, '<code class="md-code">$1</code>');
  s = s.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
  s = s.replace(/\*(.+?)\*/g, '<em>$1</em>');
  s = s.replace(/\[([^\]]+)\]\(([^)]+)\)/g, function(_, text, url) {
    if (/^https?:\/\//i.test(url) || /^mailto:/i.test(url)) {
      return '<a href="' + url.replace(/"/g, '&quot;').replace(/'/g, '&#39;') + '" class="md-link" target="_blank" rel="noopener noreferrer">' + text + '</a>';
    }
    return text;
  });
  // Restore math tokens after escaping
  if (mathTokens.length > 0) {
    s = s.replace(/\x00KTX(\d+)\x00/g, function(_, idx) { return mathTokens[+idx]; });
  }
  return s;
}

function renderTable(lines) {
  if (lines.length < 2) return lines.map(l => inlineMd(l) + '\n').join('');
  if (!/^\|[\s\-:]+(\|[\s\-:]+)+\|$/.test(lines[1].trim())) return lines.map(l => inlineMd(l) + '\n').join('');
  const cells = l => l.trim().replace(/^\||\|$/g, '').split('|').map(c => c.trim());
  let h = '<table class="md-table"><thead><tr>' + cells(lines[0]).map(c => '<th>' + inlineMd(c) + '</th>').join('') + '</tr></thead><tbody>';
  for (let i = 2; i < lines.length; i++) h += '<tr>' + cells(lines[i]).map(c => '<td>' + inlineMd(c) + '</td>').join('') + '</tr>';
  return '<div class="md-table-wrap">' + h + '</tbody></table></div>';
}

function processEventsForDisplay(events) {
  return events.filter(e => !isHiddenEvent(e));
}

function sid(key, node) { return key + '\t' + (node || 'local'); }

function isMultiNode() {
  const keys = Object.keys(nodesData);
  return keys.length > 1 || (keys.length === 1 && keys[0] !== 'local');
}

const NODE_BADGE_COLORS = ['#1f6feb','#0550ae','#1a7f37','#6e40c9','#9a6700','#cf222e'];
function nodeColor(id) {
  let h = 0;
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) >>> 0;
  return NODE_BADGE_COLORS[h % NODE_BADGE_COLORS.length];
}


/* ===== Discovery & Takeover ===== */

let _knownDiscoveredPids = new Set();
async function scanDiscovered() {
  try {
    const headers = {};
    const t = getToken();
    if (t) headers['Authorization'] = 'Bearer ' + t;
    const r = await fetch('/api/discovered', { headers });
    const items = (await r.json()) || [];
    // Detect newly discovered sessions
    for (const d of items) {
      if (!_knownDiscoveredPids.has(d.pid)) {
        const cwdShort = (d.cwd || '').split('/').pop() || d.cwd || 'unknown';
        addNotification('Session discovered', 'External CLI session in ' + cwdShort + ' (PID ' + d.pid + ')', 'info', '_discovered:' + d.pid, d.node || 'local');
      }
    }
    _knownDiscoveredPids = new Set(items.map(d => d.pid));
    discoveredItems = items;
    // Trigger sidebar re-render to merge discovered into project groups
    lastVersion = 0;
    debouncedFetchSessions();
  } catch (e) {
    showToast('scan error: ' + e.message);
  }
}

function handleDiscoveredClick(el) {
  previewDiscovered(el.dataset.sessionId, el.dataset.cwd, Number(el.dataset.pid), Number(el.dataset.pst), el.dataset.node || '');
}

async function previewDiscovered(sessionId, cwd, pid, procStartTime, node) {
  stopPreviewPolling();
  // Deselect any managed session
  selectedKey = null;
  selectedNode = null;
  if (wsm.subscribedKey) wsm.unsubscribe();
  if (eventTimer) { clearInterval(eventTimer); eventTimer = null; }
  document.querySelectorAll('.session-card').forEach(el => el.classList.remove('active'));
  mobileEnterChat();

  // Highlight the discovered card
  document.querySelectorAll('.session-card').forEach(el => el.classList.remove('active'));
  const card = document.querySelector('.session-card[data-key="_discovered:' + pid + '"]');
  if (card) card.classList.add('active');

  const base = cwd.split('/').pop() || cwd;
  const main = document.getElementById('main');
  main.innerHTML =
    '<div class="main-header">' +
      '<button class="btn-mobile-back" onclick="mobileBack()" title="back">&#8592;</button>' +
      '<div class="main-header-content">' +
        '<h2>' + esc(base) + '</h2>' +
        '<div class="detail">' +
          '<span class="badge" style="background:#9e6a03;color:#fff">external</span>' +
        '</div>' +
      '</div>' +
    '</div>' +
    '<div class="events" id="events-scroll"><div class="empty-state">loading...</div></div>' +
    '<div class="input-area" id="input-area">' +
      '<div class="file-preview" id="file-preview"></div>' +
      '<div class="input-row">' +
        '<div id="msg-input" contenteditable="true" role="textbox" data-placeholder="send a message to take over..." onkeydown="handleKey(event)" oncompositionend="lastCompositionEnd=Date.now()"></div>' +
        '<button class="btn-icon btn-send" id="btn-send" onclick="sendMessage()" title="send">&#x27a4;</button>' +
      '</div>' +
    '</div>';
  pendingDiscovered = {pid: pid, sessionId: sessionId, cwd: cwd, procStartTime: procStartTime, node: node};

  try {
    const nodeParam = node ? '&node=' + encodeURIComponent(node) : '';
    const headers = {};
    const t = getToken();
    if (t) headers['Authorization'] = 'Bearer ' + t;
    const r = await fetch('/api/discovered/preview?session_id=' + encodeURIComponent(sessionId) + nodeParam, { headers });
    if (!r.ok) { showToast('preview failed'); return; }
    const events = await r.json();
    const el = document.getElementById('events-scroll');
    if (!el) return;
    const display = processEventsForDisplay(events);
    if (events.length === 0) {
      el.innerHTML = '<div class="empty-state">no conversation history</div>';
    } else {
      el.innerHTML = display.map(eventHtml).filter(Boolean).join('');
      el.scrollTop = el.scrollHeight;
    }
    previewEventCount = events.length;
    const capturedSid = sessionId;
    previewTimer = setInterval(async () => {
      try {
        const headers2 = {};
        const t2 = getToken();
        if (t2) headers2['Authorization'] = 'Bearer ' + t2;
        const r2 = await fetch('/api/discovered/preview?session_id=' + encodeURIComponent(capturedSid) + nodeParam, { headers: headers2 });
        if (!r2.ok) return;
        const all = await r2.json();
        if (all.length <= previewEventCount) return;
        const fresh = all.slice(previewEventCount);
        previewEventCount = all.length;
        const el2 = document.getElementById('events-scroll');
        if (!el2) { stopPreviewPolling(); return; }
        const empty = el2.querySelector('.empty-state');
        if (empty) empty.remove();
        const wasBottom = el2.scrollTop + el2.clientHeight >= el2.scrollHeight - 30;
        fresh.forEach(e => {
          if (isHiddenEvent(e)) return;
          const h = eventHtml(e); if (h) el2.insertAdjacentHTML('beforeend', h);
        });
        if (wasBottom) el2.scrollTop = el2.scrollHeight;
      } catch (_) {}
    }, 2000);
  } catch (e) {
    showToast('preview error: ' + e.message);
  }
}

function handleTakeoverClick(el) {
  takeover(el, Number(el.dataset.pid), el.dataset.sessionId, el.dataset.cwd, Number(el.dataset.pst), el.dataset.node || '');
}

async function takeover(btn, pid, sessionId, cwd, procStartTime, node) {
  btn.classList.add('taking');
  btn.textContent = 'taking over...';
  try {
    const headers = {'Content-Type': 'application/json'};
    const token = getToken();
    if (token) headers['Authorization'] = 'Bearer ' + token;
    const r = await fetch('/api/discovered/takeover', {
      method: 'POST', headers,
      body: JSON.stringify({pid: pid, session_id: sessionId, cwd: cwd, proc_start_time: procStartTime || 0, node: node || ''})
    });
    if (!r.ok) {
      const text = await r.text();
      showToast('takeover failed: ' + text);
      btn.classList.remove('taking');
      btn.textContent = 'takeover';
      return;
    }
    const data = await r.json();
    showToast('session taken over', 'success');
    // Immediately remove the discovered card from DOM
    const card = document.querySelector('.session-card[data-key="_discovered:' + pid + '"]');
    if (card) card.remove();
    // Force refresh (clear cache so renderSidebar runs)
    lastVersion = 0;
    await fetchSessions();
    if (data.key) {
      selectSession(data.key, node || 'local');
    }
  } catch (e) {
    showToast('takeover error: ' + e.message);
    btn.classList.remove('taking');
    btn.textContent = 'takeover';
  }
}

/* ===== Mobile Navigation ===== */

const mobileQuery = window.matchMedia('(max-width:768px)');
function isMobile() { return mobileQuery.matches; }

// Re-initialise when crossing the 768px breakpoint (e.g. orientation change)
mobileQuery.addEventListener('change', e => {
  if (!e.matches) {
    document.body.classList.remove('mobile-list-view', 'mobile-chat-view');
  } else {
    initMobile();
  }
});

function mobileEnterChat() {
  if (!isMobile()) return;
  history.pushState({ view: 'chat' }, '');
  document.body.classList.remove('mobile-list-view');
  document.body.classList.add('mobile-chat-view');
}

function mobileBack() {
  document.body.classList.remove('mobile-chat-view');
  document.body.classList.add('mobile-list-view');
  if (document.activeElement) document.activeElement.blur();
  syncMobileTab('sessions');
}

// Handle Android back button and iOS swipe-back gesture
window.addEventListener('popstate', () => {
  if (isMobile() && document.body.classList.contains('mobile-chat-view')) {
    mobileBack();
  }
});

function initMobile() {
  if (!isMobile()) return;
  const hasSession = !!selectedKey;
  document.body.classList.toggle('mobile-chat-view', hasSession);
  document.body.classList.toggle('mobile-list-view', !hasSession);
}

function initSwipeDelete() {
  const list = document.getElementById('session-list');
  if (!list) return;
  let card = null, startX = 0, startY = 0, tracking = false;
  let longPressTimer = null, longPressed = false;

  // Ensure swipe-bg element exists on a card
  function ensureSwipeBg(c) {
    if (!c.querySelector('.swipe-bg')) {
      const bg = document.createElement('div');
      bg.className = 'swipe-bg';
      bg.textContent = 'Delete';
      c.appendChild(bg);
    }
  }

  function clearLongPress() {
    if (longPressTimer) { clearTimeout(longPressTimer); longPressTimer = null; }
  }

  list.addEventListener('touchstart', e => {
    if (e.touches.length !== 1) { card = null; clearLongPress(); return; }
    const c = e.target.closest('.session-card[data-key]');
    if (!c) return;
    card = c; startX = e.touches[0].clientX; startY = e.touches[0].clientY; tracking = false; longPressed = false;
    // Long-press: 500ms to toggle pin
    clearLongPress();
    longPressTimer = setTimeout(() => {
      if (!card || tracking) return;
      longPressed = true;
      const key = card.dataset.key;
      const node = card.dataset.node || 'local';
      // Find session data to check current pin state
      const sData = allSessionsCache.find(s => s.key === key && (s.node || 'local') === node);
      const isPinned = sData ? !!sData.pinned : false;
      togglePin(key, node, !isPinned);
      if (navigator.vibrate) navigator.vibrate(10);
      showToast(isPinned ? 'Unpinned' : 'Pinned', 'success');
      card = null;
    }, 500);
  }, {passive:true});
  list.addEventListener('touchmove', e => {
    if (!card) return;
    const dx = e.touches[0].clientX - startX;
    const dy = e.touches[0].clientY - startY;
    if (!tracking) {
      if (Math.abs(dx) < 5 && Math.abs(dy) < 5) return;
      clearLongPress();
      if (Math.abs(dy) >= Math.abs(dx)) { card = null; return; }
      tracking = true;
    }
    if (dx >= 0) return;
    ensureSwipeBg(card);
    card.classList.add('swiping');
    card.style.transform = 'translateX(' + dx + 'px)';
    // Show red bg with swipe-bg reveal at 80px threshold
    const absDx = -dx;
    card.style.background = 'rgba(218,54,51,' + Math.min(0.35, absDx / card.offsetWidth * 0.6) + ')';
    const swipeBg = card.querySelector('.swipe-bg');
    if (swipeBg) swipeBg.style.opacity = absDx >= 80 ? '1' : String(Math.min(1, absDx / 80));
  }, {passive:true});
  list.addEventListener('touchend', e => {
    clearLongPress();
    if (longPressed) { longPressed = false; card = null; tracking = false; return; }
    if (!card || !tracking) { card = null; tracking = false; return; }
    const dx = e.changedTouches[0].clientX - startX;
    const c = card; card = null; tracking = false;
    c.classList.remove('swiping');
    const swipeBg = c.querySelector('.swipe-bg');
    if (dx < -80) {
      // Swipe past threshold: delete
      c.style.transition = 'transform .2s ease, opacity .2s ease';
      c.style.transform = 'translateX(-100%)';
      c.style.opacity = '0';
      setTimeout(() => dismissSession(c.dataset.key, c.dataset.node || 'local'), 180);
    } else {
      c.style.transition = 'transform .2s ease, background .2s ease';
      c.style.transform = '';
      c.style.background = '';
      if (swipeBg) swipeBg.style.opacity = '0';
      setTimeout(() => { c.style.transition = ''; }, 200);
    }
  }, {passive:true});
}

function initSwipeBack() {
  const main = document.getElementById('main');
  if (!main) return;
  let startX = 0, startY = 0, tracking = false, swiping = false;
  main.addEventListener('touchstart', e => {
    if (!isMobile() || e.touches.length !== 1) return;
    startX = e.touches[0].clientX; startY = e.touches[0].clientY;
    tracking = false; swiping = false;
    // Only trigger from left edge (within 40px)
    if (startX > 40) return;
    tracking = true;
  }, {passive:true});
  main.addEventListener('touchmove', e => {
    if (!tracking) return;
    const dx = e.touches[0].clientX - startX;
    const dy = e.touches[0].clientY - startY;
    if (!swiping) {
      if (Math.abs(dx) < 8 && Math.abs(dy) < 8) return;
      if (Math.abs(dy) > Math.abs(dx)) { tracking = false; return; }
      if (dx < 0) { tracking = false; return; }
      swiping = true;
    }
    const progress = Math.min(dx / window.innerWidth, 1);
    main.style.transform = 'translateX(' + dx + 'px)';
    main.style.opacity = String(1 - progress * 0.3);
  }, {passive:true});
  main.addEventListener('touchend', e => {
    if (!tracking || !swiping) { tracking = false; swiping = false; return; }
    const dx = e.changedTouches[0].clientX - startX;
    tracking = false; swiping = false;
    if (dx > window.innerWidth * 0.3) {
      main.style.transition = 'transform .2s ease, opacity .2s ease';
      main.style.transform = 'translateX(100%)';
      main.style.opacity = '0';
      setTimeout(() => {
        main.style.transition = ''; main.style.transform = ''; main.style.opacity = '';
        mobileBack();
      }, 200);
    } else {
      main.style.transition = 'transform .2s ease, opacity .2s ease';
      main.style.transform = ''; main.style.opacity = '';
      setTimeout(() => { main.style.transition = ''; }, 200);
    }
  }, {passive:true});
}

/* ===== Task 10: Mobile Tab Bar ===== */

function switchMobileTab(tab, el) {
  if (!isMobile()) return;
  // Update active tab styling
  document.querySelectorAll('#mobileTabs .m-tab').forEach(t => t.classList.remove('active'));
  if (el) el.classList.add('active');

  if (tab === 'sessions') {
    // Show session list view
    mobileBack();
  } else if (tab === 'cron') {
    openCronPanel();
  } else if (tab === 'files') {
    window.openFileHub();
  } else if (tab === 'discovered') {
    // Show sidebar in list view to display discovered sessions
    document.body.classList.remove('mobile-chat-view');
    document.body.classList.add('mobile-list-view');
    // Trigger a discovered scan refresh
    scanDiscovered();
  }
}

// Sync the active tab when navigating back to list view
function syncMobileTab(tab) {
  const tabs = document.getElementById('mobileTabs');
  if (!tabs) return;
  tabs.querySelectorAll('.m-tab').forEach(t => {
    t.classList.toggle('active', t.dataset.tab === tab);
  });
}

/* ===== Task 12: Mobile Copy Button Tap ===== */

function initMobileCopyBtnTap() {
  // On mobile, show copy/bookmark buttons on tap (since hover doesn't work)
  document.addEventListener('click', e => {
    if (!isMobile()) return;
    const eventEl = e.target.closest('.event.text');
    if (!eventEl) return;
    // Find all copy buttons in this event
    const btns = eventEl.querySelectorAll('.md-copy-btn');
    if (btns.length === 0) return;
    // Toggle visibility for 3 seconds
    btns.forEach(btn => {
      btn.style.opacity = '1';
    });
    // Clear any previous timer on this element
    if (eventEl._copyBtnTimer) clearTimeout(eventEl._copyBtnTimer);
    eventEl._copyBtnTimer = setTimeout(() => {
      btns.forEach(btn => {
        btn.style.opacity = '';
      });
    }, 3000);
  }, {passive: true});
}

/* Sidebar resizer moved to js/views/chat.js (installed as IIFE at module load). */

/* ===== Notification Center ===== */

function addNotification(title, desc, urgency, sessionKey, sessionNode) {
  urgency = urgency || 'info'; // 'info', 'warning', 'urgent'
  const n = {
    id: ++notifIdCounter,
    title: title,
    desc: desc || '',
    time: Date.now(),
    read: false,
    urgency: urgency,
    sessionKey: sessionKey || null,
    sessionNode: sessionNode || 'local'
  };
  notifications.unshift(n);
  // Cap at 50 notifications
  if (notifications.length > 50) notifications.length = 50;
  updateNotifBadge();
  renderNotifications();
}

function updateNotifBadge() {
  const badge = document.getElementById('notifBadge');
  if (!badge) return;
  const unread = notifications.filter(n => !n.read).length;
  if (unread > 0) {
    badge.textContent = unread > 99 ? '99+' : unread;
    badge.style.display = 'flex';
  } else {
    badge.style.display = 'none';
  }
}

function toggleNotifications() {
  const panel = document.getElementById('notifPanel');
  if (!panel) return;
  panel.classList.toggle('show');
}

function closeNotifPanel() {
  const panel = document.getElementById('notifPanel');
  if (panel) panel.classList.remove('show');
}

function clearAllNotifs() {
  notifications.forEach(n => n.read = true);
  updateNotifBadge();
  renderNotifications();
}

function notifTimeAgo(ts) {
  const diff = Date.now() - ts;
  const sec = Math.floor(diff / 1000);
  if (sec < 60) return 'just now';
  const min = Math.floor(sec / 60);
  if (min < 60) return min + 'm ago';
  const hr = Math.floor(min / 60);
  if (hr < 24) return hr + 'h ago';
  const d = Math.floor(hr / 24);
  return d + 'd ago';
}

function notifIcon(urgency) {
  if (urgency === 'urgent') return '\u26a0\ufe0f';
  if (urgency === 'warning') return '\u26a1';
  return '\ud83d\udd14';
}

function renderNotifications() {
  const list = document.getElementById('notifList');
  if (!list) return;
  if (notifications.length === 0) {
    list.innerHTML = '<div class="notif-empty">No notifications yet</div>';
    return;
  }
  let html = '';
  for (const n of notifications) {
    const cls = n.read ? '' : (n.urgency === 'urgent' ? ' urgent' : ' unread');
    html += '<div class="notif-item' + cls + '" data-notif-id="' + n.id + '" onclick="onNotifClick(' + n.id + ')">';
    html += '<span class="ni-icon">' + notifIcon(n.urgency) + '</span>';
    html += '<div class="ni-body">';
    html += '<div class="ni-title">' + esc(n.title) + '</div>';
    if (n.desc) html += '<div class="ni-desc">' + esc(n.desc) + '</div>';
    html += '<div class="ni-time">' + notifTimeAgo(n.time) + '</div>';
    html += '</div></div>';
  }
  list.innerHTML = html;
}

function onNotifClick(id) {
  const n = notifications.find(x => x.id === id);
  if (!n) return;
  n.read = true;
  updateNotifBadge();
  renderNotifications();
  closeNotifPanel();
  // Navigate to the session if available
  if (n.sessionKey && n.sessionKey !== '_none') {
    const card = document.querySelector('.session-card[data-key="' + n.sessionKey + '"]');
    if (card) card.click();
  }
}

// Close notification panel when clicking outside
document.addEventListener('click', function(e) {
  const panel = document.getElementById('notifPanel');
  const btn = document.getElementById('notifBtn');
  if (panel && panel.classList.contains('show') && !panel.contains(e.target) && !btn.contains(e.target)) {
    closeNotifPanel();
  }
});

// Render empty state on load
renderNotifications();

/* ===== Initialization ===== */

// Wait for deferred ES modules (js/app.js -> core/ws.js + views/chat.js +
// views/home.js) to install their window bridges before calling
// fetchSessions/wsm.connect/renderHomeView. The classic inline <script>
// runs during HTML parsing, before module scripts evaluate, so we have
// to gate on readiness.
function naozhiBootstrap() {
  fetchSessions();
  sessionPollTimer = setInterval(fetchSessions, 5000);
  scanDiscovered();
  discoveredPollTimer = setInterval(scanDiscovered, 30000);
  fetchCronJobs(); // load initial cron state for badge
  wsm.connect();
  initMobile();
  initSwipeDelete();
  initSwipeBack();
  initMobileCopyBtnTap();
  // Task 19: Fetch persistent notifications and badge count on load
  fetchNotifications();
  fetchApprovalsBadge();
  // Periodically refresh notification count and patrol status
  setInterval(fetchNotifCount, 60000);
  setInterval(function() { fetchApprovalsBadge(); }, 60000);
  // Task 13: Show Home view as default landing page.
  if (typeof window.renderHomeView === 'function') {
    window.renderHomeView();
  }
}
(function waitForModules(tries) {
  // chat.js installs window.fetchSessions; ws.js installs window.wsm.
  if (typeof window.fetchSessions === 'function' && typeof window.wsm === 'object') {
    naozhiBootstrap();
    return;
  }
  if (tries > 0) {
    setTimeout(function() { waitForModules(tries - 1); }, 10);
  } else {
    console.error('NAOZHI: module bridges not installed after 500ms');
  }
})(50);
(function(){
  const ov=document.createElement('div');ov.className='lightbox-overlay';
  ov.setAttribute('role','dialog');ov.setAttribute('aria-modal','true');ov.setAttribute('aria-label','Image preview');
  ov.innerHTML='<img alt="">';document.body.appendChild(ov);
  function close(){ov.classList.remove('active')}
  ov.addEventListener('click',close);
  ov.querySelector('img').addEventListener('click',function(e){e.stopPropagation()});
  window.openLightbox=function(src){ov.querySelector('img').src=src;ov.classList.add('active')};
  document.addEventListener('keydown',function(e){if(e.key==='Escape'&&ov.classList.contains('active'))close()});
})();

/* ===== /ls Enhanced Rendering ===== */

function enhanceLsOutput(text) {
  if (!text || text.indexOf('\ud83d\udcc2 ') !== 0) return null;

  var lines = text.split('\n');
  var headerMatch = lines[0].match(/^\ud83d\udcc2\s+(.+)$/);
  if (!headerMatch) return null;

  var basePath = headerMatch[1].trim();
  var html = '<div class="ls-enhanced">';
  html += '<div>\ud83d\udcc2 <strong>' + esc(basePath) + '</strong></div>';

  for (var i = 1; i < lines.length; i++) {
    var line = lines[i];
    if (!line.trim()) continue;

    // Actual /ls output format: "  📁 dirname/    6 items" or "  📄 file.txt    1.2K"
    var dirMatch = line.match(/^(\s+)\ud83d\udcc1\s+(\S+?)\/(.*)$/);
    var fileMatch = line.match(/^(\s+)\ud83d\udcc4\s+(.+)$/);
    var summaryMatch = line.match(/^\d+\s+items\s+\(/);

    if (dirMatch) {
      var prefix = dirMatch[1];
      var dirName = dirMatch[2];
      var suffix = dirMatch[3] || '';
      var fullPath = basePath.endsWith('/') ? basePath + dirName : basePath + '/' + dirName;
      html += '<div>' + esc(prefix) + '\ud83d\udcc1 <span class="ls-dir-link" onclick="fhLsNavigate(\'' + escJs(fullPath) + '\')">' + esc(dirName) + '/</span>' + esc(suffix) + '</div>';
    } else if (summaryMatch) {
      html += '<div style="margin-top:4px;padding-top:4px;border-top:1px solid #21262d;color:#484f58">' + esc(line) + '</div>';
    } else if (fileMatch) {
      html += '<div>' + esc(line) + '</div>';
    } else {
      html += '<div>' + esc(line) + '</div>';
    }
  }

  html += '<button class="ls-open-fh" onclick="openFileHub(\'' + escJs(basePath) + '\')">\ud83d\udcc1 Open in File Hub</button>';
  html += '</div>';
  return html;
}

/* ===== View Switching (Tasks 11-13) ===== */

var currentView = 'chat'; // 'chat', 'knowledge', 'wiki', 'patrols', 'approvals'
var patrolsCache = [];
var patrolsStatsCache = {};
var approvalsCache = [];
var approvalsStatsCache = {};
var approvalsFilter = 'pending';
var patrolRefreshTimer = null;

// switchView moved to js/core/router.js (bridged via window.switchView).

/* Task 13: Home Dashboard moved to js/views/home.js
   (renderHomeView/renderHome, loadHomeStats, loadHomeActivity,
   loadHomeWiki, loadHomePatrolsAndApprovals, renderActivityFeed).
   Bare `renderHomeView()` callsites in this legacy block resolve to
   window.renderHomeView installed by the home module. */

/* Patrols View (Task 17) moved to js/views/patrols.js. All entry
   points and the WS onPatrolEvent dispatcher bridged onto window.*;
   the shared patrolsCache/patrolsStatsCache/patrolRefreshTimer vars
   remain declared above in the /* View Switching */ block. */

/* Approvals View (Task 18) moved to js/views/approvals.js. All entry
   points (fetchApprovals, updateApprovalsBadge, renderApprovalsView,
   switchApprovalFilter, renderApprovalCards, approvalIcon,
   approvalAction, onApprovalCreated, onApprovalResolved,
   fetchApprovalsBadge) bridged onto window.*; the shared
   approvalsCache/approvalsStatsCache/approvalsFilter vars remain
   declared above in the /* View Switching */ block. */

/* ===== Task 19: Notification Center Enhancement ===== */

async function fetchNotifications() {
  try {
    var resp = await fetch('/api/notifications?limit=20', { headers: authHeaders() });
    if (!resp.ok) return;
    var data = await resp.json();
    var serverNotifs = data.notifications || [];
    // Merge server notifications into the local array
    for (var i = 0; i < serverNotifs.length; i++) {
      var sn = serverNotifs[i];
      var exists = notifications.find(function(n) { return n.serverId === sn.id; });
      if (!exists) {
        notifications.push({
          id: ++notifIdCounter,
          serverId: sn.id,
          title: sn.title || sn.type || 'Notification',
          desc: sn.summary || '',
          time: sn.created_at ? (typeof sn.created_at === 'number' ? sn.created_at : new Date(sn.created_at).getTime()) : Date.now(),
          read: !!sn.read,
          urgency: sn.urgency === 'urgent' ? 'urgent' : 'info',
          sourceType: sn.source_type || '',
          sourceRef: sn.source_ref || '',
          sessionKey: null,
          sessionNode: 'local'
        });
      }
    }
    // Sort by time descending
    notifications.sort(function(a, b) { return b.time - a.time; });
    if (notifications.length > 50) notifications.length = 50;
    updateNotifBadge();
    renderNotifications();
  } catch (e) { console.error('fetchNotifications:', e); }
}

async function fetchNotifCount() {
  try {
    var resp = await fetch('/api/notifications/count', { headers: authHeaders() });
    if (!resp.ok) return;
    var data = await resp.json();
    var serverUnread = data.unread_count || 0;
    // Use the max of server and local unread
    var localUnread = notifications.filter(function(n) { return !n.read; }).length;
    var totalUnread = Math.max(serverUnread, localUnread);
    var badge = document.getElementById('notifBadge');
    if (!badge) return;
    if (totalUnread > 0) {
      badge.textContent = totalUnread > 99 ? '99+' : totalUnread;
      badge.style.display = 'flex';
    } else {
      badge.style.display = 'none';
    }
  } catch (e) { /* ignore */ }
}

function onWsNotification(msg) {
  var n = msg.notification || msg;
  addNotification(
    n.title || n.type || 'Notification',
    n.summary || '',
    n.urgency === 'urgent' ? 'urgent' : 'info',
    '_none',
    'local'
  );
}

// Override clearAllNotifs to also call server-side mark-all-read
var _origClearAllNotifs = clearAllNotifs;
clearAllNotifs = function() {
  _origClearAllNotifs();
  fetch('/api/notifications/read-all', { method: 'POST', headers: authHeaders() }).catch(function() {});
};

// Enhanced onNotifClick to navigate to source views
var _origOnNotifClick = onNotifClick;
onNotifClick = function(id) {
  var n = notifications.find(function(x) { return x.id === id; });
  if (n) { n.read = true; }
  updateNotifBadge();
  renderNotifications();
  closeNotifPanel();
  // Navigate based on sourceType
  if (n && n.sourceType === 'patrol') {
    switchView('patrols', document.querySelector('[data-view=patrols]'));
    return;
  }
  if (n && n.sourceType === 'approval') {
    switchView('approvals', document.querySelector('[data-view=approvals]'));
    return;
  }
  // Fall back to session navigation
  if (n && n.sessionKey && n.sessionKey !== '_none') {
    switchView('chat', document.querySelector('[data-view=chat]'));
    var card = document.querySelector('.session-card[data-key="' + n.sessionKey + '"]');
    if (card) card.click();
  }
};

/* Task 20: Home Dashboard Integration moved to js/views/home.js
   (updateHomePatrolWidget, updateHomeApprovalWidget,
   renderHomePatrolWidgetContent, renderHomeApprovalWidgetContent).
   Accessed from this legacy block via window.* bridges. */

/* Knowledge View (Task 11) + Knowledge AI Chat (Gap 5) moved to
   js/views/knowledge.js. All helpers are bridged onto window.* for
   inline onclick callers. */

/* Wiki View (Task 12) and Decision Journal (Phase 4D) moved to
   js/views/wiki.js. renderWikiView, loadWikiPage(s), triggerIngest,
   triggerLint, switchWikiSubTab, loadDecisions/render/show/submit
   all bridged on window. wvPages mirrored on window so home.js can
   enumerate compiled pages. */

/* ===== Task 14: Context Panel ===== */

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

/* ===== Task 15: Bookmark UI ===== */

var _bmSessionBookmarks = {}; // session_key -> Set of event indices with bookmarks
var _bmIndexToId = {}; // session_key -> { eventIndex -> bookmark_id }

async function loadSessionBookmarks() {
  if (!selectedKey) return;
  try {
    const bms = await fetch('/api/bookmarks?session=' + encodeURIComponent(selectedKey), { headers: authHeaders() }).then(r => r.ok ? r.json() : []).catch(() => []);
    const set = new Set();
    const idMap = {};
    (Array.isArray(bms) ? bms : []).forEach(b => { if (b.event_index >= 0) { set.add(b.event_index); if (b.id) idMap[b.event_index] = b.id; } });
    _bmSessionBookmarks[selectedKey] = set;
    _bmIndexToId[selectedKey] = idMap;
  } catch (_) {}
}

function injectBookmarkButtons() {
  const events = document.getElementById('events-scroll');
  if (!events) return;
  let idx = 0;
  events.querySelectorAll('.event.text').forEach(ev => {
    const content = ev.querySelector('.event-content');
    if (!content || content.querySelector('.bm-hover-btn')) return;
    const eventIdx = idx++;
    const saved = _bmSessionBookmarks[selectedKey] && _bmSessionBookmarks[selectedKey].has(eventIdx);
    const btn = document.createElement('button');
    btn.className = 'bm-hover-btn' + (saved ? ' saved' : '');
    btn.textContent = '\uD83D\uDD16';
    btn.title = 'Save bookmark';
    btn.setAttribute('data-event-idx', eventIdx);
    btn.addEventListener('click', async function(e) {
      e.stopPropagation();
      if (this.classList.contains('saved')) {
        // Toggle: delete existing bookmark
        const bmId = (_bmIndexToId[selectedKey] || {})[eventIdx];
        if (bmId) {
          try {
            await fetch('/api/bookmarks/' + encodeURIComponent(bmId), { method: 'DELETE', headers: authHeaders() });
            this.classList.remove('saved');
            this.removeAttribute('data-bm-id');
            if (_bmSessionBookmarks[selectedKey]) _bmSessionBookmarks[selectedKey].delete(eventIdx);
            if (_bmIndexToId[selectedKey]) delete _bmIndexToId[selectedKey][eventIdx];
            showToast('Bookmark removed', 'success');
            if (ctxPanelOpen && ctxActiveTab === 'saved') refreshCtxPanel();
          } catch (err) { showToast('Delete failed'); }
        }
        return;
      }
      showBookmarkPopover(this, content.textContent || '', eventIdx);
    });
    content.appendChild(btn);
  });
}

function showBookmarkPopover(anchor, text, eventIdx) {
  // Remove any existing popover
  document.querySelectorAll('.bm-popover').forEach(p => p.remove());
  const preview = text.substring(0, 200) + (text.length > 200 ? '...' : '');
  const pop = document.createElement('div');
  pop.className = 'bm-popover';
  pop.innerHTML =
    '<textarea readonly>' + esc(preview) + '</textarea>' +
    '<input type="text" placeholder="Tags (comma-separated, e.g. security, waf)" id="bmTagInput">' +
    '<div class="bm-pop-btns">' +
      '<button onclick="this.closest(\'.bm-popover\').remove()">Cancel</button>' +
      '<button class="primary" id="bmSaveBtn">Save</button>' +
    '</div>';
  anchor.parentElement.appendChild(pop);
  const tagInput = pop.querySelector('#bmTagInput');
  tagInput.focus();
  pop.querySelector('#bmSaveBtn').addEventListener('click', async function() {
    const tags = tagInput.value.split(',').map(t => t.trim()).filter(Boolean);
    try {
      const r = await fetch('/api/bookmarks', {
        method: 'POST',
        headers: { ...authHeaders(), 'Content-Type': 'application/json' },
        body: JSON.stringify({
          session_key: selectedKey || '',
          source: 'dashboard',
          content: text.substring(0, 2000),
          tags: tags,
          event_index: eventIdx
        })
      });
      if (!r.ok) throw new Error('save failed');
      const saved = await r.json().catch(() => ({}));
      pop.remove();
      anchor.classList.add('saved');
      if (saved.id) { anchor.setAttribute('data-bm-id', saved.id); if (!_bmIndexToId[selectedKey]) _bmIndexToId[selectedKey] = {}; _bmIndexToId[selectedKey][eventIdx] = saved.id; }
      showToast('Bookmarked', 'success');
      if (!_bmSessionBookmarks[selectedKey]) _bmSessionBookmarks[selectedKey] = new Set();
      _bmSessionBookmarks[selectedKey].add(eventIdx);
      // Refresh context panel if open
      if (ctxPanelOpen && ctxActiveTab === 'saved') refreshCtxPanel();
    } catch (e) {
      showToast('Bookmark failed: ' + e.message);
    }
  });
  // Close on outside click
  setTimeout(() => {
    const handler = function(e) {
      if (!pop.contains(e.target) && e.target !== anchor) {
        pop.remove();
        document.removeEventListener('click', handler);
      }
    };
    document.addEventListener('click', handler);
  }, 0);
}

// ─── CTO Digital Twin UI ────────────────────────────────────────────────────
var _twinConfig = null;

function openTwinPanel() {
  fetch('/api/twin/config')
    .then(function(r) { return r.json(); })
    .then(function(cfg) {
      _twinConfig = cfg;
      showTwinOverlay(cfg);
    })
    .catch(function() { alert('Failed to load Twin config'); });
}

function showTwinOverlay(cfg) {
  var existing = document.getElementById('twinOverlay');
  if (existing) existing.remove();
  var ov = document.createElement('div');
  ov.id = 'twinOverlay';
  ov.style.cssText = 'position:fixed;inset:0;z-index:9998;background:rgba(0,0,0,.7);display:flex;align-items:center;justify-content:center';
  ov.onclick = function(e) { if(e.target===ov) closeTwinOverlay(); };
  ov.innerHTML =
    '<div style="background:#161b22;border:1px solid #30363d;border-radius:12px;width:520px;max-width:90vw;max-height:80vh;overflow-y:auto;color:#c9d1d9;font-family:monospace">' +
      '<div style="padding:16px 20px;border-bottom:1px solid #30363d;display:flex;justify-content:space-between;align-items:center">' +
        '<h3 style="font-size:16px;color:#f0f6fc;margin:0">CTO Digital Twin</h3>' +
        '<button onclick="closeTwinOverlay()" style="background:none;border:none;color:#8b949e;cursor:pointer;font-size:16px">&#x2715;</button>' +
      '</div>' +
      '<div style="padding:16px 20px">' +
        '<div style="margin-bottom:16px">' +
          '<label style="display:flex;align-items:center;gap:8px;cursor:pointer">' +
            '<input type="checkbox" id="twinEnabled" ' + (cfg.enabled ? 'checked' : '') + ' onchange="updateTwinField(\'enabled\',this.checked)" style="accent-color:#58a6ff">' +
            '<span>Enable Digital Twin</span>' +
          '</label>' +
        '</div>' +
        '<div style="margin-bottom:12px">' +
          '<div style="font-size:12px;color:#8b949e;margin-bottom:4px">Name</div>' +
          '<input id="twinName" value="' + esc(cfg.name || '') + '" style="width:100%;padding:6px 10px;background:#0d1117;border:1px solid #30363d;border-radius:4px;color:#c9d1d9;font-size:13px;box-sizing:border-box" onchange="updateTwinField(\'name\',this.value)">' +
        '</div>' +
        '<div style="margin-bottom:12px">' +
          '<div style="font-size:12px;color:#8b949e;margin-bottom:4px">Role</div>' +
          '<input id="twinRole" value="' + esc(cfg.role || '') + '" style="width:100%;padding:6px 10px;background:#0d1117;border:1px solid #30363d;border-radius:4px;color:#c9d1d9;font-size:13px;box-sizing:border-box" onchange="updateTwinField(\'role\',this.value)">' +
        '</div>' +
        '<div style="margin-bottom:16px">' +
          '<div style="font-size:12px;color:#8b949e;margin-bottom:4px">Response Style</div>' +
          '<select id="twinStyle" style="padding:6px 10px;background:#0d1117;border:1px solid #30363d;border-radius:4px;color:#c9d1d9;font-size:13px" onchange="updateTwinField(\'style\',this.value)">' +
            '<option value="formal"' + (cfg.style === 'formal' ? ' selected' : '') + '>Formal</option>' +
            '<option value="casual"' + (cfg.style === 'casual' ? ' selected' : '') + '>Casual</option>' +
          '</select>' +
        '</div>' +
        '<div style="border-top:1px solid #21262d;padding-top:16px;margin-bottom:12px">' +
          '<div style="font-size:14px;color:#f0f6fc;margin-bottom:8px">Test Query</div>' +
          '<div style="display:flex;gap:8px">' +
            '<input id="twinTestInput" placeholder="Ask a question to test the Twin..." style="flex:1;padding:6px 10px;background:#0d1117;border:1px solid #30363d;border-radius:4px;color:#c9d1d9;font-size:13px" onkeydown="if(event.key===\'Enter\')testTwinQuery()">' +
            '<button onclick="testTwinQuery()" style="background:#238636;border:none;color:#fff;padding:6px 12px;border-radius:4px;cursor:pointer;font-size:13px">Test</button>' +
          '</div>' +
        '</div>' +
        '<div id="twinTestResult" style="display:none;margin-top:12px;padding:12px;background:#0d1117;border:1px solid #30363d;border-radius:6px;font-size:13px"></div>' +
      '</div>' +
    '</div>';
  document.body.appendChild(ov);
}

function closeTwinOverlay() {
  var ov = document.getElementById('twinOverlay');
  if (ov) ov.remove();
}

function updateTwinField(field, value) {
  if (!_twinConfig) return;
  _twinConfig[field] = value;
  fetch('/api/twin/config', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(_twinConfig)
  }).catch(function() {});
}

function testTwinQuery() {
  var input = document.getElementById('twinTestInput');
  var resultEl = document.getElementById('twinTestResult');
  if (!input || !resultEl) return;
  var query = input.value.trim();
  if (!query) return;
  resultEl.style.display = 'block';
  resultEl.innerHTML = '<span style="color:#8b949e">Testing...</span>';
  fetch('/api/twin/test', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ query: query })
  })
  .then(function(r) { return r.json(); })
  .then(function(data) {
    var conf = data.confidence || {};
    var barColor = conf.overall >= 0.8 ? '#3fb950' : (conf.overall >= 0.3 ? '#d29922' : '#da3633');
    resultEl.innerHTML =
      '<div style="margin-bottom:8px"><b>Action:</b> <span style="color:#58a6ff">' + esc(data.action || 'N/A') + '</span></div>' +
      (data.tag ? '<div style="margin-bottom:8px;color:#8b949e">' + esc(data.tag) + '</div>' : '') +
      '<div style="margin-bottom:8px"><b>Confidence:</b> ' + (conf.overall != null ? conf.overall.toFixed(2) : 'N/A') + '</div>' +
      '<div style="display:flex;gap:16px;margin-bottom:8px;font-size:12px">' +
        '<span>Coverage: ' + (conf.coverage != null ? conf.coverage.toFixed(2) : '-') + '</span>' +
        '<span>Recency: ' + (conf.recency != null ? conf.recency.toFixed(2) : '-') + '</span>' +
        '<span>Specificity: ' + (conf.specificity != null ? conf.specificity.toFixed(2) : '-') + '</span>' +
      '</div>' +
      '<div style="height:6px;background:#21262d;border-radius:3px;overflow:hidden;margin-bottom:8px">' +
        '<div style="height:100%;width:' + ((conf.overall||0)*100) + '%;background:' + barColor + ';border-radius:3px"></div>' +
      '</div>' +
      (data.draft ? '<div style="margin-top:8px;padding:8px;background:#161b22;border-radius:4px;white-space:pre-wrap;font-size:12px;color:#c9d1d9">' + esc(data.draft) + '</div>' : '') +
      (data.error ? '<div style="color:#da3633;margin-top:8px">' + esc(data.error) + '</div>' : '') +
      (data.note ? '<div style="color:#d29922;margin-top:4px;font-size:12px">' + esc(data.note) + '</div>' : '');
  })
  .catch(function() { resultEl.innerHTML = '<span style="color:#da3633">Request failed</span>'; });
}

// Keyboard shortcuts: Cmd+K search, Cmd+N new session, ESC close
document.addEventListener('keydown', async function(e) {
  // Cmd+N or Ctrl+N to open new session modal
  if ((e.metaKey || e.ctrlKey) && e.key === 'n') {
    e.preventDefault();
    var nsOv = document.getElementById('nsOverlay');
    if (nsOv && nsOv.classList.contains('show')) {
      closeNewSessionModal();
    } else {
      openNewSessionModal();
    }
    return;
  }

  // ESC to close new session modal (before search ESC handler)
  if (e.key === 'Escape') {
    var nsOv = document.getElementById('nsOverlay');
    if (nsOv && nsOv.classList.contains('show')) {
      e.preventDefault();
      e.stopPropagation();
      closeNewSessionModal();
      return;
    }
  }

  // Cmd+K or Ctrl+K to open search
  if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
    e.preventDefault();
    var ov = document.getElementById('searchOverlay');
    if (ov && ov.classList.contains('show')) {
      await window.closeSearch();
    } else {
      await window.openSearch();
    }
    return;
  }

  // Only handle keys when search overlay is open
  var ov = document.getElementById('searchOverlay');
  if (!ov || !ov.classList.contains('show')) return;

  if (e.key === 'Escape') {
    e.preventDefault();
    e.stopPropagation();
    await window.closeSearch();
    return;
  }

  if (e.key === 'ArrowDown') {
    e.preventDefault();
    await window._searchArrow('down');
    return;
  }

  if (e.key === 'ArrowUp') {
    e.preventDefault();
    await window._searchArrow('up');
    return;
  }

  if (e.key === 'Enter') {
    e.preventDefault();
    await window._searchEnter();
    return;
  }
});

/* Phase 4A Knowledge Graph moved to js/views/graph.js (Plan Task 13.3). */

// ─── Lazy feature module shims (Phase 3) ─────────────────────────────────────
const FEAT = (name) => window.__resolveAsset('js/features/' + name + '.js');

window.openReplayViewer = async (...a) =>
  (await import(FEAT('replay'))).open(...a);
window.shareReplaySession = async (...a) =>
  (await import(FEAT('replay'))).share(...a);
window.closeReplayOverlay = async (...a) =>
  (await import(FEAT('replay'))).closeOverlay(...a);
window.toggleReplayPlay = async (...a) =>
  (await import(FEAT('replay'))).togglePlay(...a);
window.scrubReplay = async (...a) =>
  (await import(FEAT('replay'))).scrub(...a);
window.setReplaySpeed = async (...a) =>
  (await import(FEAT('replay'))).speed(...a);

window.openCronPanel = async (...a) => (await import(FEAT('cron'))).open(...a);
window.createNewCronJob = async (...a) => (await import(FEAT('cron'))).createNew(...a);
window.doCreateCronJob = async (...a) => (await import(FEAT('cron'))).doCreate(...a);
window.cronSelectSchedule = async (...a) => (await import(FEAT('cron'))).selectSchedule(...a);
window.cronSelectWorkspace = async (...a) => (await import(FEAT('cron'))).selectWorkspace(...a);
window.toggleCronWsCustom = async (...a) => (await import(FEAT('cron'))).toggleWsCustom(...a);
window.toggleCronCustom = async (...a) => (await import(FEAT('cron'))).toggleCustom(...a);
window.previewCronSchedule = async (...a) => (await import(FEAT('cron'))).preview(...a);
window.openCronSession = async (...a) => (await import(FEAT('cron'))).openSession(...a);
window.cronPause = async (...a) => (await import(FEAT('cron'))).pause(...a);
window.cronResume = async (...a) => (await import(FEAT('cron'))).resume(...a);
window.cronDelete = async (...a) => (await import(FEAT('cron'))).remove(...a);

window.openFileHub = async (...a) => (await import(FEAT('file-hub'))).open(...a);
window.closeFileHub = async (...a) => (await import(FEAT('file-hub'))).close(...a);
window.fhLsNavigate = async (...a) => (await import(FEAT('file-hub'))).lsNavigate(...a);
window.fhNavigate = async (...a) => (await import(FEAT('file-hub'))).navigate(...a);
window.fhToggleHidden = async (...a) => (await import(FEAT('file-hub'))).toggleHidden(...a);
window.fhRowClick = async (...a) => (await import(FEAT('file-hub'))).rowClick(...a);
window.fhRowDblClick = async (...a) => (await import(FEAT('file-hub'))).rowDblClick(...a);
window.fhGoUp = async (...a) => (await import(FEAT('file-hub'))).goUp(...a);
window.fhToggle = async (...a) => (await import(FEAT('file-hub'))).toggle(...a);
window.fhInsertPath = async (...a) => (await import(FEAT('file-hub'))).insertPath(...a);
window.fhCopyPath = async (...a) => (await import(FEAT('file-hub'))).copyPath(...a);
window.fhShowUpload = async (...a) => (await import(FEAT('file-hub'))).showUpload(...a);
window.fhDrop = async (...a) => (await import(FEAT('file-hub'))).drop(...a);
window.fhDownloadSelected = async (...a) => (await import(FEAT('file-hub'))).downloadSelected(...a);
window.fhPromptMkdir = async (...a) => (await import(FEAT('file-hub'))).promptMkdir(...a);
window.fhDeleteSelected = async (...a) => (await import(FEAT('file-hub'))).deleteSelected(...a);

window.openSearch = async (...a) => (await import(FEAT('search'))).open(...a);
window.closeSearch = async (...a) => (await import(FEAT('search'))).close(...a);
window._searchArrow = async (dir) => (await import(FEAT('search'))).arrow(dir);
window._searchEnter = async () => (await import(FEAT('search'))).enter();

