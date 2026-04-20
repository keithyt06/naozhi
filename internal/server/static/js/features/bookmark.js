// internal/server/static/js/features/bookmark.js
// Per-session bookmark list + popover + button injection.
import { authHeaders } from '../core/api.js';
import { esc } from '../core/html.js';

let initialized = false;
function ensureInit() {
  if (initialized) return;
  initialized = true;
}

// --- Begin extracted from legacy.js:1771-1880 ---
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
// --- End extracted ---

export async function load(...a) { ensureInit(); return loadSessionBookmarks(...a); }
export async function inject(...a) { ensureInit(); return injectBookmarkButtons(...a); }
export async function showPopover(...a) { ensureInit(); return showBookmarkPopover(...a); }
