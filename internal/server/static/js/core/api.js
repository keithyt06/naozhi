// js/core/api.js — token + auth-header helpers and shared fetch utilities.
// Moved verbatim from pre-split dashboard.html. Current auth is cookie-based,
// so getToken/setToken are no-ops kept as compatibility shims; if a future
// phase re-introduces Bearer tokens, update both the helpers and the callers
// in one pass.

export function getToken() { return ''; }
export function setToken(t) { /* token stored in HttpOnly cookie only */ }

export function authHeaders() {
  const h = {};
  const t = getToken();
  if (t) h['Authorization'] = 'Bearer ' + t;
  return h;
}

// Shared fetch helpers (new — no pre-existing shared fetchJSON in dashboard.html).
// Legacy call sites keep their own `fetch(...)` calls for now; new code under
// js/views/* should prefer apiFetch/apiJSON.
export async function apiFetch(url, opts = {}) {
  const merged = Object.assign({}, opts);
  merged.headers = Object.assign({}, authHeaders(), opts.headers || {});
  const resp = await fetch(url, merged);
  if (!resp.ok) throw new Error(`${url}: HTTP ${resp.status}`);
  return resp;
}

export async function apiJSON(url, opts) {
  const resp = await apiFetch(url, opts);
  return resp.json();
}

if (typeof window !== 'undefined') {
  window.getToken = getToken;
  window.setToken = setToken;
  window.authHeaders = authHeaders;
  window.apiFetch = apiFetch;
  window.apiJSON = apiJSON;
}
