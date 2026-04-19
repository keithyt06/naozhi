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

export function switchView(view, el) {
  if (state.currentView === view) return;
  const prev = state.currentView;
  state.currentView = view;

  // Nav button highlight
  document.querySelectorAll('#viewBar .vb-btn').forEach(b =>
    b.classList.toggle('active', b.dataset.view === view));

  // Teardown previous module (if any)
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
// and any remaining legacy callers resolve to this module.
if (typeof window !== 'undefined') {
  window.switchView = switchView;
  window.registerView = register;
}

export { views };
