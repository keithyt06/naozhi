// js/core/html.js — HTML escaping + tagged template helper.
// Kept under 50 LOC, no dependencies.
//
// Implementations preserved verbatim from pre-split dashboard.html to
// guarantee zero behaviour change. The plan template proposed a slightly
// different `esc`/`escAttr`/`escJs` (regex-based `esc`, `escAttr = esc`,
// shorter `escJs`); using the pre-split versions avoids any risk of
// output drift in tag/attr/inline-JS contexts until Phase 2 unifies them.

const _escEl = (typeof document !== 'undefined') ? document.createElement('div') : null;

export function esc(s) {
  if (!s) return '';
  if (!_escEl) {
    return String(s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');
  }
  _escEl.textContent = s;
  return _escEl.innerHTML;
}

export function escAttr(s) { return esc(s).replace(/"/g, '&quot;'); }

export function escJs(s) {
  if (!s) return '';
  return String(s).replace(/\\/g, '\\\\').replace(/'/g, "\\'").replace(/"/g, '\\"').replace(/\n/g, '\\n').replace(/\r/g, '\\r');
}

export function html(strings, ...values) {
  let out = strings[0];
  for (let i = 0; i < values.length; i++) {
    out += esc(values[i]) + strings[i + 1];
  }
  return out;
}

// Legacy globals — the migrated view modules import these, but existing
// inline onclick="switchView('chat',this)" handlers still resolve via window.
// Removed after Task 13 (graph migration completes).
if (typeof window !== 'undefined') {
  window.esc = esc;
  window.escAttr = escAttr;
  window.escJs = escJs;
}
