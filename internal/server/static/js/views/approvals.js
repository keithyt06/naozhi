// js/views/approvals.js — Approvals view.
//
// Moved verbatim from dashboard.html's inline <script> (Task 18:
// Approvals View banner). Owns: fetchApprovals, updateApprovalsBadge,
// renderApprovalsView, switchApprovalFilter, renderApprovalCards,
// approvalIcon, approvalAction, onApprovalCreated, onApprovalResolved,
// fetchApprovalsBadge.
//
// Shared globals (`approvalsCache`, `approvalsStatsCache`,
// `approvalsFilter`, `currentView`) are still declared in the
// legacy inline <script>'s /* View Switching (Tasks 11-13) */
// block and reached via window bridges — leave them there until
// Phase 2 migrates them into state.js.

import { esc, escAttr, escJs } from '../core/html.js';

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

function addNotification(title, desc, urgency, sessionKey, sessionNode) {
  if (typeof window !== 'undefined' && typeof window.addNotification === 'function') {
    window.addNotification(title, desc, urgency, sessionKey, sessionNode);
  }
}

function updateHomeApprovalWidget() {
  if (typeof window !== 'undefined' && typeof window.updateHomeApprovalWidget === 'function') {
    window.updateHomeApprovalWidget();
  }
}

// ------- entry points ---------------------------------------------

export async function fetchApprovals(status) {
  try {
    var url = '/api/approvals';
    if (status) url += '?status=' + encodeURIComponent(status);
    var resp = await fetch(url, { headers: authHeaders() });
    if (!resp.ok) return;
    var data = await resp.json();
    approvalsCache = data.approvals || [];
    approvalsStatsCache = data.stats || {};
    updateApprovalsBadge();
  } catch (e) { console.error('fetchApprovals:', e); }
}

export function updateApprovalsBadge() {
  var pending = approvalsStatsCache.pending || 0;
  var btn = document.getElementById('vbApprovalsBtn');
  if (!btn) return;
  var existing = btn.querySelector('.vb-badge');
  if (pending > 0) {
    if (!existing) {
      var badge = document.createElement('span');
      badge.className = 'vb-badge';
      btn.appendChild(badge);
      existing = badge;
    }
    existing.textContent = pending > 99 ? '99+' : pending;
  } else if (existing) {
    existing.remove();
  }
}

export function renderApprovalsView() {
  var main = document.getElementById('main');
  main.innerHTML =
    '<div class="approvals-view" id="approvalsView">' +
      '<div class="approvals-header"><h2>Approvals</h2></div>' +
      '<div class="approvals-tabs" id="approvalsTabs">' +
        '<button class="approvals-tab' + (approvalsFilter === 'pending' ? ' active' : '') + '" onclick="switchApprovalFilter(\'pending\',this)">Pending</button>' +
        '<button class="approvals-tab' + (approvalsFilter === 'approved' ? ' active' : '') + '" onclick="switchApprovalFilter(\'approved\',this)">Approved</button>' +
        '<button class="approvals-tab' + (approvalsFilter === 'rejected' ? ' active' : '') + '" onclick="switchApprovalFilter(\'rejected\',this)">Rejected</button>' +
        '<button class="approvals-tab' + (approvalsFilter === '' ? ' active' : '') + '" onclick="switchApprovalFilter(\'\',this)">All</button>' +
      '</div>' +
      '<div class="approvals-list" id="approvalsList"><div style="color:#6e7681;font-size:13px;text-align:center;padding:40px 0">Loading approvals...</div></div>' +
    '</div>';
  fetchApprovals(approvalsFilter || '').then(function() { renderApprovalCards(); });
}

export function switchApprovalFilter(status, el) {
  approvalsFilter = status;
  document.querySelectorAll('#approvalsTabs .approvals-tab').forEach(function(b) { b.classList.remove('active'); });
  if (el) el.classList.add('active');
  fetchApprovals(status).then(function() { renderApprovalCards(); });
}

export function renderApprovalCards() {
  var list = document.getElementById('approvalsList');
  if (!list) return;

  var filtered = approvalsCache;
  if (approvalsFilter) {
    filtered = approvalsCache.filter(function(a) { return (a.status || '').toLowerCase() === approvalsFilter; });
  }

  if (filtered.length === 0) {
    if (approvalsFilter === 'pending' || !approvalsFilter) {
      list.innerHTML = '<div class="approvals-empty"><div class="ae-check">&#10003;</div><div class="ae-title">All caught up!</div><div class="ae-sub">No pending approvals</div></div>';
    } else {
      list.innerHTML = '<div class="approvals-empty"><div class="ae-sub">No ' + esc(approvalsFilter) + ' approvals</div></div>';
    }
    return;
  }

  list.innerHTML = filtered.map(function(a) {
    var urgencyClass = (a.urgency === 'urgent') ? ' urgent' : '';
    var isPending = (a.status || '').toLowerCase() === 'pending';
    var levelClass = (a.level === 'critical') ? 'critical' : 'high';
    var icon = approvalIcon(a.action || a.summary || '');
    var ts = a.created_at ? timeAgo(typeof a.created_at === 'number' ? a.created_at : new Date(a.created_at).getTime()) : '';
    var detailId = 'approval-detail-' + (a.id || '');

    return '<div class="approval-card' + urgencyClass + '" id="approval-card-' + escAttr(a.id || '') + '">' +
      '<div class="approval-card-header">' +
        '<span class="approval-card-icon">' + icon + '</span>' +
        '<div class="approval-card-info">' +
          '<div class="approval-card-title">' + esc(a.action || a.summary || 'Approval Request') + '</div>' +
          '<div class="approval-card-source">' +
            (a.patrol ? 'from <strong>' + esc(a.patrol) + '</strong>' : '') +
            (a.level ? ' <span class="approval-level-badge ' + levelClass + '">' + esc(a.level) + '</span>' : '') +
          '</div>' +
        '</div>' +
        '<span class="approval-card-time">' + esc(ts) + '</span>' +
      '</div>' +
      (a.detail || a.summary ? '<div class="approval-card-detail">' + esc(a.detail || a.summary || '') + '</div>' : '') +
      (a.detail ? '<div class="approval-card-full-detail" id="' + detailId + '">' + esc(a.detail) + '</div>' : '') +
      '<div class="approval-card-actions">' +
        (isPending ? '<button class="approval-btn approve" onclick="approvalAction(\'' + escJs(a.id) + '\',\'approve\')">Approve</button>' +
          '<button class="approval-btn reject" onclick="approvalAction(\'' + escJs(a.id) + '\',\'reject\')">Reject</button>' : '') +
        (a.detail ? '<button class="approval-btn details" onclick="var d=document.getElementById(\'' + detailId + '\');if(d)d.classList.toggle(\'show\')">Details</button>' : '') +
      '</div>' +
    '</div>';
  }).join('');
}

export function approvalIcon(action) {
  var a = (action || '').toLowerCase();
  if (a.indexOf('terraform') >= 0) return '&#127959;';
  if (a.indexOf('delete') >= 0 || a.indexOf('drop') >= 0 || a.indexOf('rm ') >= 0) return '&#128465;';
  if (a.indexOf('deploy') >= 0 || a.indexOf('push') >= 0) return '&#128640;';
  if (a.indexOf('kubectl') >= 0) return '&#9784;';
  return '&#128275;';
}

export async function approvalAction(id, action) {
  var card = document.getElementById('approval-card-' + id);
  // Optimistic animation
  if (card) {
    card.classList.add(action === 'approve' ? 'approved-flash' : '');
    setTimeout(function() { if (card) card.classList.add('sliding-out'); }, 200);
  }
  try {
    var resp = await fetch('/api/approvals/' + encodeURIComponent(id) + '/' + action, {
      method: 'POST',
      headers: Object.assign({ 'Content-Type': 'application/json' }, authHeaders()),
      body: JSON.stringify(action === 'approve' ? { approved_by: 'dashboard' } : { rejected_by: 'dashboard' })
    });
    if (!resp.ok) {
      // Rollback animation
      if (card) { card.classList.remove('sliding-out', 'approved-flash'); }
      var e = await resp.json().catch(function() { return {}; });
      showToast(e.error || 'Failed to ' + action);
      return;
    }
    // Remove card after animation
    setTimeout(function() {
      fetchApprovals(approvalsFilter || '').then(function() { renderApprovalCards(); });
    }, 400);
  } catch (e) {
    if (card) { card.classList.remove('sliding-out', 'approved-flash'); }
    showToast('Network error');
  }
}

export function onApprovalCreated(msg) {
  var approval = msg.approval || msg;
  addNotification('Approval needed: ' + (approval.action || approval.summary || ''), 'from ' + (approval.patrol || 'patrol'), approval.urgency === 'urgent' ? 'urgent' : 'warning', '_none', 'local');
  // Refresh if on approvals view
  if (currentView === 'approvals') {
    fetchApprovals(approvalsFilter || '').then(function() { renderApprovalCards(); });
  }
  // Update badge
  fetchApprovalsBadge();
  updateHomeApprovalWidget();
}

export function onApprovalResolved(msg) {
  var status = msg.status || 'resolved';
  // Remove card with animation if on approvals view
  if (currentView === 'approvals') {
    var cardId = msg.id || (msg.approval && msg.approval.id);
    var card = cardId ? document.getElementById('approval-card-' + cardId) : null;
    if (card) card.classList.add('sliding-out');
    setTimeout(function() {
      fetchApprovals(approvalsFilter || '').then(function() { renderApprovalCards(); });
    }, 400);
  }
  fetchApprovalsBadge();
  updateHomeApprovalWidget();
}

export async function fetchApprovalsBadge() {
  try {
    var resp = await fetch('/api/approvals?status=pending', { headers: authHeaders() });
    if (!resp.ok) return;
    var data = await resp.json();
    approvalsStatsCache = data.stats || {};
    updateApprovalsBadge();
  } catch (e) { /* ignore */ }
}

// ------- Legacy bridges — removed after Phase 2 -------------------

if (typeof window !== 'undefined') {
  window.fetchApprovals = fetchApprovals;
  window.updateApprovalsBadge = updateApprovalsBadge;
  window.renderApprovalsView = renderApprovalsView;
  window.switchApprovalFilter = switchApprovalFilter;
  window.renderApprovalCards = renderApprovalCards;
  window.approvalIcon = approvalIcon;
  window.approvalAction = approvalAction;
  window.onApprovalCreated = onApprovalCreated;
  window.onApprovalResolved = onApprovalResolved;
  window.fetchApprovalsBadge = fetchApprovalsBadge;
}
