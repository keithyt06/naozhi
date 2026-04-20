// internal/server/static/js/features/notif-enhance.js
// Notification enhancement: push setup + WS incoming handler + fetch loop.
import { authHeaders } from '../core/api.js';

let initialized = false;
function ensureInit() {
  if (initialized) return;
  initialized = true;
}

// --- Begin extracted from legacy.js:1757-1852 ---
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
// --- End extracted ---

export async function setup(...args) {
  ensureInit();
  return fetchNotifications(...args);
}

export async function handle(...args) {
  ensureInit();
  return onWsNotification(...args);
}

export { fetchNotifCount };
