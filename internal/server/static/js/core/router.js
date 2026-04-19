// js/core/router.js — view registry + switchView.
//
// Owns the top-nav view switching. Provides a tiny registry so view
// modules (Tasks 8-13) can self-register with { mount, unmount }. For
// any view without a registered module, we fall back to the legacy
// inline render functions still attached to `window` (renderHomeView,
// renderKnowledgeView, renderWikiView, renderPatrolsView,
// renderApprovalsView, renderGraphView, renderMainShell). This keeps
// all existing `onclick="switchView(...)"` attributes working during
// the migration.

import { state } from './state.js';

// name -> { mount(slot), unmount() }
const views = {};

export function register(name, mod) {
  views[name] = mod;
}

// Task 15: lazy-loaded view modules. chat (+ home, imported by chat)
// ships in the first-paint bundle; everything else is fetched on the
// first switchView('<name>'). Modules self-register via registerView()
// or install a window.render<Name>View bridge as a side effect of
// evaluation, so we don't need to inspect the resolved module object.
const LAZY_VIEWS = {
  knowledge: 'js/views/knowledge.js',
  wiki:      'js/views/wiki.js',
  patrols:   'js/views/patrols.js',
  approvals: 'js/views/approvals.js',
  graph:     'js/views/graph.js',
};

const loadedViews = new Set();
const loadingViews = new Map(); // name -> Promise, dedupes concurrent imports

function resolveAsset(rel) {
  if (typeof window !== 'undefined' && typeof window.__resolveAsset === 'function') {
    return window.__resolveAsset(rel);
  }
  return '/static/' + rel;
}

export function ensureViewLoaded(name) {
  if (loadedViews.has(name)) return Promise.resolve();
  const rel = LAZY_VIEWS[name];
  if (!rel) return Promise.resolve(); // not a lazy view (e.g. chat)
  if (loadingViews.has(name)) return loadingViews.get(name);
  const url = resolveAsset(rel);
  const p = import(url).then(() => {
    loadedViews.add(name);
    loadingViews.delete(name);
  }).catch(err => {
    loadingViews.delete(name);
    console.warn('ensureViewLoaded', name, err);
    throw err;
  });
  loadingViews.set(name, p);
  return p;
}

// switchView stays synchronous for callers — we kick off the lazy load
// and run the actual render on a microtask once the module is present.
// This matches the original UX because existing call sites never awaited
// the return value anyway (onclick handlers + legacy.js fire-and-forget).
export function switchView(view, el) {
  if (state.currentView === view) return;
  const prev = state.currentView;
  state.currentView = view;

  // Nav button highlight — synchronous so the clicked button lights up
  // immediately even while the lazy chunk is still downloading.
  document.querySelectorAll('#viewBar .vb-btn').forEach(b =>
    b.classList.toggle('active', b.dataset.view === view));

  // Teardown previous module (if any). Done before the new module
  // loads so the outgoing view always unmounts, even on slow networks.
  const prevMod = views[prev];
  if (prevMod && typeof prevMod.unmount === 'function') {
    try { prevMod.unmount(); } catch (e) { console.warn('unmount', prev, e); }
  }

  // Chat view shares the sidebar (session list + tag filter) with the
  // existing layout; every other view hides them. This mirrors the old
  // inline switchView body bit-for-bit.
  const tagFilter = document.getElementById('tagFilter');
  const sessionList = document.getElementById('session-list');

  if (view === 'chat') {
    if (tagFilter) tagFilter.style.display = '';
    if (sessionList) sessionList.style.display = '';
  } else {
    if (tagFilter) tagFilter.style.display = 'none';
    if (sessionList) sessionList.style.display = 'none';
    // On mobile, hide sidebar so #main content is visible
    document.body.classList.remove('mobile-list-view');
  }

  ensureViewLoaded(view).then(() => doMount(view)).catch(() => doMount(view));
}

function doMount(view) {
  // If another switchView superseded us before the lazy chunk arrived,
  // bail out — doMount for the newer view will take over.
  if (state.currentView !== view) return;

  // Prefer registered module; otherwise fall back to legacy window.*
  // render functions.
  const mod = views[view];
  if (mod && typeof mod.mount === 'function') {
    const slot = document.getElementById('main');
    try {
      const r = mod.mount(slot);
      if (r && typeof r.then === 'function') r.catch(e => console.warn('mount', view, e));
    } catch (e) {
      console.warn('mount', view, e);
    }
    return;
  }

  // Legacy fallback — matches the original inline switchView behavior.
  if (view === 'chat') {
    if (state.selectedKey) {
      if (typeof window.renderMainShell === 'function') window.renderMainShell();
    } else {
      if (typeof window.renderHomeView === 'function') window.renderHomeView();
    }
  } else if (view === 'knowledge') {
    if (typeof window.renderKnowledgeView === 'function') window.renderKnowledgeView();
  } else if (view === 'wiki') {
    if (typeof window.renderWikiView === 'function') window.renderWikiView();
  } else if (view === 'patrols') {
    if (typeof window.renderPatrolsView === 'function') window.renderPatrolsView();
  } else if (view === 'approvals') {
    if (typeof window.renderApprovalsView === 'function') window.renderApprovalsView();
  } else if (view === 'graph') {
    if (typeof window.renderGraphView === 'function') window.renderGraphView();
  }
}

// Legacy bridge — existing inline onclick="switchView(...)" attributes
// and any remaining legacy callers resolve to this module. We also
// expose the lazy loader so non-module scripts (legacy.js) can trigger
// a view fetch without going through switchView.
if (typeof window !== 'undefined') {
  window.switchView = switchView;
  window.registerView = register;
  window.__ensureViewLoaded = ensureViewLoaded;
}

export { views };
