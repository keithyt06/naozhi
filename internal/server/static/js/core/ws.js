// js/core/ws.js — WebSocket Connection Manager + WS Helper Functions.
//
// Owns the dashboard's WS lifecycle: connect/auth, subscribe/unsubscribe,
// and message dispatch for `history`, `event`, `send_ack`, `session_state`,
// `sessions_update`, `cron_result`, `patrol_event`, `approval_created`,
// `approval_resolved`, `notification`. Moved verbatim from the pre-split
// dashboard.html (banners `/* ===== WebSocket Connection Manager ===== */`
// and `/* ===== WS Helper Functions ===== */`).
//
// Dependencies not yet modularized — read off `window` as bare identifiers
// because state.js exposes every legacy mutable as `window.<name>` via
// getter/setters. The functions this module invokes (getToken,
// debouncedFetchSessions, sid, renameSession, processEventsForDisplay,
// eventHtml, resetTurnState, applyEventToTurnState, refreshBanner,
// isHiddenEvent, runMermaid, runKatex, enhanceLsOutput, esc, addNotification,
// onPatrolEvent, onApprovalCreated, onApprovalResolved, onWsNotification,
// fetchCronJobs, fetchEvents, fetchSessions, updateStatusBar, scanDiscovered,
// updateSendButton, showToast) are installed on window by the inline
// legacy <script>, by core/api.js, or by js/views/chat.js.
//
// Exported:
//   WS_STATES          — enum of transport states
//   wsm                — the manager object (subscribe/unsubscribe/send/etc.)
//   connectWebSocket() — thin wrapper invoking wsm.connect()
//   updateMainState, updateHeaderCost, flashSendBtn, stopPreviewPolling
//
// Side effect: bridges wsm, WS_STATES, connectWebSocket, and each helper
// onto window so pre-module callers continue to resolve them as globals.

/* ===== WebSocket Connection Manager ===== */

export const WS_STATES = { OFF: 'off', CONNECTING: 'connecting', AUTH: 'authenticating', CONNECTED: 'connected', DISCONNECTED: 'disconnected' };

export const wsm = {
  conn: null,
  state: WS_STATES.OFF,
  backoff: 1000,
  maxBackoff: 30000,
  reconnectTimer: null,
  pingTimer: null,
  subscribedKey: null,
  subscribedNode: null,
  lastEventTimeWs: 0,
  sendCounter: 0,
  _initialSubscribe: false,

  connect() {
    if (this.conn && (this.conn.readyState === WebSocket.OPEN || this.conn.readyState === WebSocket.CONNECTING)) return;

    this.setState(WS_STATES.CONNECTING);
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    this.conn = new WebSocket(proto + '//' + location.host + '/ws');

    this.conn.onopen = () => {
      this.setState(WS_STATES.AUTH);
      const token = getToken();
      this.conn.send(JSON.stringify({ type: 'auth', token: token }));
    };

    this.conn.onmessage = (evt) => {
      try { this.onMessage(JSON.parse(evt.data)); }
      catch (err) { console.error('ws parse error:', err); }
    };

    this.conn.onclose = () => {
      this.cleanup();
      this.setState(WS_STATES.DISCONNECTED);
      this.scheduleReconnect();
    };

    this.conn.onerror = () => {};
  },

  cleanup() {
    if (this.pingTimer) { clearInterval(this.pingTimer); this.pingTimer = null; }
  },

  disconnect() {
    if (this.reconnectTimer) { clearTimeout(this.reconnectTimer); this.reconnectTimer = null; }
    this.cleanup();
    if (this.conn) { this.conn.close(); this.conn = null; }
    this.subscribedKey = null;
    this.subscribedNode = null;
    this._pendingSubscribeKey = null;
    this._pendingSubscribeNode = null;
    this.setState(WS_STATES.OFF);
  },

  scheduleReconnect() {
    if (this.reconnectTimer) return;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, this.backoff);
    this.backoff = Math.min(this.backoff * 2, this.maxBackoff);
  },

  onMessage(msg) {
    switch (msg.type) {
      case 'auth_ok':
        this.setState(WS_STATES.CONNECTED);
        this.backoff = 1000;
        this.startPing();
        this.onConnected();
        break;
      case 'auth_fail':
        showToast('WS auth failed: ' + (msg.error || 'invalid token'));
        this.conn.close();
        break;
      case 'subscribed':
        // Server confirmed subscription
        this.subscribedKey = this._pendingSubscribeKey || msg.key;
        this.subscribedNode = this._pendingSubscribeNode || 'local';
        this._pendingSubscribeKey = null;
        this._pendingSubscribeNode = null;
        break;
      case 'error':
        // Subscribe failed (e.g. session not found yet) — reset pending
        this._pendingSubscribeKey = null;
        this._pendingSubscribeNode = null;
        break;
      case 'history':
        this.onHistory(msg);
        break;
      case 'event':
        this.onEvent(msg);
        break;
      case 'send_ack':
        this.onSendAck(msg);
        break;
      case 'interrupt_ack':
        break;
      case 'session_state':
        this.onSessionState(msg);
        break;
      case 'sessions_update':
        debouncedFetchSessions().then(() => {
          // Apply pending session names: the session now exists in the router
          for (const pKey of Object.keys(pendingSessionNames)) {
            if (sessionsData[sid(pKey, 'local')]) {
              renameSession(pKey, 'local', pendingSessionNames[pKey]);
              delete pendingSessionNames[pKey];
            }
          }
          // Auto-subscribe to newly created session (only if no active or pending subscription)
          if (selectedKey && !wsm.subscribedKey && !wsm._pendingSubscribeKey && sessionsData[sid(selectedKey, selectedNode)]) {
            wsm.subscribe(selectedKey, selectedNode);
          }
        });
        break;
      case 'cron_result':
        fetchCronJobs();
        { const cronName = msg.job_name || msg.name || 'Cron job';
          const cronStatus = msg.success === false ? 'failed' : 'completed';
          const cronUrgency = msg.success === false ? 'urgent' : 'info';
          addNotification('Cron ' + cronStatus + ': ' + cronName, msg.error || msg.summary || '', cronUrgency, '_none', 'local'); }
        break;
      case 'patrol_event':
        onPatrolEvent(msg);
        break;
      case 'approval_created':
        onApprovalCreated(msg);
        break;
      case 'approval_resolved':
      case 'approval_update':
        onApprovalResolved(msg);
        break;
      case 'notification':
        onWsNotification(msg);
        break;
      case 'pong':
        break;
    }
  },

  startPing() {
    if (this.pingTimer) clearInterval(this.pingTimer);
    this.pingTimer = setInterval(() => {
      if (this.conn && this.conn.readyState === WebSocket.OPEN) {
        this.conn.send(JSON.stringify({ type: 'ping' }));
      }
    }, 30000);
  },

  send(msg) {
    if (this.conn && this.conn.readyState === WebSocket.OPEN) {
      this.conn.send(JSON.stringify(msg));
      return true;
    }
    return false;
  },

  subscribe(key, node) {
    node = node || 'local';
    this._pendingSubscribeKey = key;
    this._pendingSubscribeNode = node;
    const msg = { type: 'subscribe', key: key };
    if (node && node !== 'local') msg.node = node;
    this._initialSubscribe = (this.lastEventTimeWs === 0);
    if (this.lastEventTimeWs > 0) msg.after = this.lastEventTimeWs;
    this.send(msg);
  },

  unsubscribe() {
    if (this.subscribedKey) {
      const msg = { type: 'unsubscribe', key: this.subscribedKey };
      if (this.subscribedNode && this.subscribedNode !== 'local') msg.node = this.subscribedNode;
      this.send(msg);
    }
    this.subscribedKey = null;
    this.subscribedNode = null;
    this._pendingSubscribeKey = null;
    this._pendingSubscribeNode = null;
    this.lastEventTimeWs = 0;
  },

  /* -- WS event handlers -- */

  onConnected() {
    if (eventTimer) { clearInterval(eventTimer); eventTimer = null; }
    if (selectedKey) {
      if (lastEventTime > 0 && this.lastEventTimeWs === 0) {
        this.lastEventTimeWs = lastEventTime;
      }
      this.subscribe(selectedKey, selectedNode);
    }
  },

  onHistory(msg) {
    if (msg.key !== selectedKey || (msg.node || 'local') !== selectedNode) return;
    const el = document.getElementById('events-scroll');
    if (!el) return;
    const events = msg.events || [];
    const isInitial = this._initialSubscribe;
    this._initialSubscribe = false;

    const display = processEventsForDisplay(events);

    if (isInitial) {
      // Full render replaces everything — remove any optimistic messages
      el.innerHTML = display.map(eventHtml).filter(Boolean).join('') || '<div class="empty-state">no events yet</div>';
      el.scrollTop = el.scrollHeight;
      // Reset dedup tracker on full render
      if (events.length > 0) { const last = events[events.length - 1]; if (last.time) lastRenderedEventTime = last.time; }
      runMermaid();
  runKatex();
    } else {
      const wasBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 30;
      display.forEach(e => {
        // Deduplicate: skip events at or before the last rendered time
        if (e.time && e.time <= lastRenderedEventTime) return;
        // When the real "user" event arrives, remove the optimistic version
        if (e.type === 'user') {
          const opt = el.querySelector('.optimistic-msg');
          if (opt) opt.remove();
        }
        const h = eventHtml(e); if (h) el.insertAdjacentHTML('beforeend', h);
        if (e.time && e.time > lastRenderedEventTime) lastRenderedEventTime = e.time;
      });
      if (wasBottom) el.scrollTop = el.scrollHeight;
      runMermaid();
  runKatex();
    }

    if (events.length > 0) {
      const last = events[events.length - 1];
      if (last.time > this.lastEventTimeWs) this.lastEventTimeWs = last.time;
    }
    // Build turnState from events
    if (isInitial) {
      // Full rebuild: scan backward to find the last turn boundary
      resetTurnState();
      let turnStart = events.length;
      for (let i = events.length - 1; i >= 0; i--) {
        if (events[i].type === 'user' || events[i].type === 'result') { turnStart = i + 1; break; }
        if (i === 0) turnStart = 0;
      }
      // Anchor timer to the actual turn start time, not Date.now()
      if (turnStart < events.length && events[turnStart].time) {
        turnState.turnStartTime = events[turnStart].time;
        turnState.timerId = setInterval(function() {
          var el = document.getElementById('rb-elapsed');
          if (!el || !turnState.turnStartTime) return;
          var s = Math.floor((Date.now() - turnState.turnStartTime) / 1000);
          el.textContent = Math.floor(s / 60) + ':' + String(s % 60).padStart(2, '0');
        }, 1000);
      }
      for (let i = turnStart; i < events.length; i++) {
        applyEventToTurnState(events[i]);
      }
    } else {
      // Incremental: accumulate additively, reset only on turn boundaries
      for (let i = 0; i < events.length; i++) {
        const ev = events[i];
        if (ev.type === 'user') {
          resetTurnState();
          const text = ev.detail || ev.summary || '';
          if (text) {
            const h2 = document.querySelector('.main-header h2');
            if (h2) h2.textContent = text;
          }
          continue;
        }
        if (ev.type === 'result') {
          if (ev.cost) {
            const sKey = sid(selectedKey, selectedNode);
            if (sessionsData[sKey]) sessionsData[sKey].total_cost = ev.cost;
          }
          resetTurnState(); continue;
        }
        applyEventToTurnState(ev);
      }
    }
    refreshBanner();
    updateHeaderCost();
  },

  onEvent(msg) {
    if (msg.key !== selectedKey || (msg.node || 'local') !== selectedNode) return;
    const ev = msg.event;
    if (!ev) return;
    if (ev.time > this.lastEventTimeWs) this.lastEventTimeWs = ev.time;
    // Turn boundaries: reset state, don't feed into applyEventToTurnState
    if (ev.type === 'user') {
      const text = ev.detail || ev.summary || '';
      if (text) {
        const h2 = document.querySelector('.main-header h2');
        if (h2) h2.textContent = text;
      }
      resetTurnState();
    } else if (ev.type === 'result') {
      if (ev.cost) {
        const sKey = sid(selectedKey, selectedNode);
        if (sessionsData[sKey]) sessionsData[sKey].total_cost = ev.cost;
        updateHeaderCost();
      }
      resetTurnState();
    } else {
      applyEventToTurnState(ev);
      refreshBanner();
    }
    if (isHiddenEvent(ev)) return;
    const html = eventHtml(ev);
    if (!html) return;
    const el = document.getElementById('events-scroll');
    if (!el) return;
    const empty = el.querySelector('.empty-state');
    if (empty) empty.remove();
    // When the real "user" event arrives, remove the optimistic version
    if (ev.type === 'user') {
      const opt = el.querySelector('.optimistic-msg');
      if (opt) opt.remove();
    }
    const wasBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 30;
    el.insertAdjacentHTML('beforeend', html);
    if (wasBottom) el.scrollTop = el.scrollHeight;
    runMermaid();
  runKatex();
  },

  onSendAck(msg) {
    if (msg.status === 'accepted') {
      flashSendBtn();
      // Pending session names are applied in sessions_update handler
      // (after the session actually exists in the router).
      const ackKey = msg.key || selectedKey;
      // Subscribe to the session we just sent to, unless we're already
      // subscribed or a subscribe is already pending for this exact key.
      // The old check (!subscribedKey && !_pendingSubscribeKey) failed when
      // the user was previously viewing a different session — subscribedKey
      // was set to the old key, blocking the subscribe for the new one.
      if (ackKey && wsm.subscribedKey !== ackKey && wsm._pendingSubscribeKey !== ackKey) {
        wsm.lastEventTimeWs = 0;
        wsm.subscribe(ackKey, selectedNode);
      }
      // Re-subscribe is NOT needed here for already-subscribed sessions.
      // The existing eventPushLoop is still connected to the process's event
      // log and will deliver new events (including the user message we just
      // sent). Re-subscribing would cause a history replay that overlaps with
      // events already pushed by the running eventPushLoop, resulting in
      // duplicate user messages in the UI.
      // For process restarts (dead/suspended → running), onSessionState
      // handles re-subscription exclusively.
    } else if (msg.status === 'command') {
      // Slash command result from server (/ls, /help, /cd, /pwd)
      const el = document.getElementById('events-scroll');
      if (el) {
        const empty = el.querySelector('.empty-state');
        if (empty) empty.remove();
        const enhanced = enhanceLsOutput(msg.reason || '');
        const content = enhanced || '<pre style="white-space:pre-wrap;margin:0">' + esc(msg.reason || '') + '</pre>';
        const wasBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 30;
        el.insertAdjacentHTML('beforeend',
          '<div class="event-row" style="border-left:3px solid #484f58;padding:8px 12px;margin:4px 0">' +
          '<div style="display:flex;align-items:center;gap:6px;margin-bottom:4px">' +
          '<span style="background:#21262d;color:#484f58;padding:2px 8px;border-radius:8px;font-size:10px;border:1px solid #30363d">SYSTEM</span>' +
          '</div>' + content + '</div>');
        if (wasBottom) el.scrollTop = el.scrollHeight;
      }
      // Remove optimistic message
      const opt = document.querySelector('.optimistic-msg');
      if (opt) opt.remove();
    } else if (msg.status === 'error') {
      showToast('send error: ' + (msg.error || 'unknown'), 'error');
      addNotification('Send failed', msg.error || 'Unknown error sending message', 'urgent', selectedKey, selectedNode);
      // Remove optimistic message on send failure
      const opt = document.querySelector('.optimistic-msg');
      if (opt) opt.remove();
    }
  },

  onSessionState(msg) {
    const msgNode = msg.node || 'local';
    const sKey = sid(msg.key, msgNode);
    const prevState = sessionsData[sKey] ? sessionsData[sKey].state : null;
    if (sessionsData[sKey]) {
      sessionsData[sKey].state = msg.state;
      if (msg.reason) sessionsData[sKey].death_reason = msg.reason;
    }
    let card = null;
    document.querySelectorAll('.session-card').forEach(c => {
      if (c.dataset.key === msg.key && (c.dataset.node || 'local') === msgNode) card = c;
    });
    if (card) {
      const badge = card.querySelector('.badge');
      if (badge) { badge.className = 'badge ' + msg.state; badge.textContent = msg.state; }
      card.classList.toggle('dead-card', msg.state === 'suspended');
    }
    if (msg.key === selectedKey && msgNode === selectedNode) updateMainState(msg.state, msg.reason);
    // Process restarted or reconnected after restart: re-subscribe to pick up
    // the new process's event log. Covers both suspended→running (new message)
    // and suspended→ready (shim reconnect after zero-downtime restart).
    if (msg.key === selectedKey && msgNode === selectedNode &&
        (msg.state === 'running' || msg.state === 'ready') && prevState === 'suspended' &&
        wsm.subscribedKey === msg.key) {
      wsm.subscribe(msg.key, selectedNode);
    }
    // New session became running: subscribe immediately if we're viewing it
    // but not yet subscribed (covers the new-session send flow).
    if (msg.key === selectedKey && msgNode === selectedNode &&
        msg.state === 'running' && wsm.subscribedKey !== msg.key &&
        wsm._pendingSubscribeKey !== msg.key) {
      wsm.lastEventTimeWs = 0;
      wsm.subscribe(msg.key, selectedNode);
    }
    // Session became suspended: re-render sidebar to update visual state
    if (msg.state === 'suspended') { lastVersion = 0; debouncedFetchSessions(); }
    // Notification: session died or errored
    if (msg.state === 'suspended' && msg.reason) {
      const sData = sessionsData[sKey];
      const sName = (sData && sData.name) || msg.key;
      addNotification('Session stopped: ' + sName, msg.reason, 'urgent', msg.key, msgNode);
    }
  },

  setState(s) {
    this.state = s;
    updateStatusBar();
    if (s === WS_STATES.CONNECTED) {
      // WS connected: stop session polling, rely on push
      if (sessionPollTimer) { clearInterval(sessionPollTimer); sessionPollTimer = null; }
      // Reduce discovered scan frequency
      if (discoveredPollTimer) { clearInterval(discoveredPollTimer); discoveredPollTimer = null; }
      discoveredPollTimer = setInterval(scanDiscovered, 30000);
      // Pull fresh node/session state immediately to clear stale data
      debouncedFetchSessions();
    } else if (s === WS_STATES.DISCONNECTED) {
      // WS lost: start fallback polling
      if (!sessionPollTimer) sessionPollTimer = setInterval(fetchSessions, 5000);
      if (discoveredPollTimer) { clearInterval(discoveredPollTimer); discoveredPollTimer = null; }
      discoveredPollTimer = setInterval(scanDiscovered, 5000);
      if (selectedKey && !eventTimer) {
        lastEventTime = this.lastEventTimeWs;
        eventTimer = setInterval(() => fetchEvents(false), 1000);
      }
    }
  },

  isConnected() { return this.state === WS_STATES.CONNECTED; }
};

/* ===== WS Helper Functions ===== */

export function updateMainState(state, reason) {
  const ia = document.getElementById('input-area');
  if (ia) ia.classList.toggle('disabled', false);
  updateSendButton(state);
}

export function updateHeaderCost() {
  const s = sessionsData[sid(selectedKey, selectedNode)] || {};
  const el = document.getElementById('header-cost');
  if (!el) return;
  const cost = s.total_cost || 0;
  el.textContent = '$' + (cost < 0.01 && cost > 0 ? cost.toFixed(4) : cost.toFixed(2));
  el.className = 'detail-cost' + (cost >= 1 ? ' high-cost' : cost > 0 ? ' has-cost' : '');
}

export function flashSendBtn() {
  const btn = document.getElementById('btn-send');
  const stop = document.getElementById('btn-stop');
  const target = (btn && btn.style.display !== 'none') ? btn : stop;
  if (!target) return;
  target.style.boxShadow = '0 0 8px #3fb950';
  setTimeout(() => { target.style.boxShadow = ''; }, 600);
}

export function stopPreviewPolling() {
  if (previewTimer) { clearInterval(previewTimer); previewTimer = null; }
  previewEventCount = 0;
}

// Thin wrapper for callers that prefer a plain function name over wsm.connect().
export function connectWebSocket() {
  wsm.connect();
}

// ------- legacy window.* bridges -----------------------------------
// Pre-module callers reference these as bare identifiers. Removed in
// Phase 2 when every consumer is a module.

if (typeof window !== 'undefined') {
  window.WS_STATES = WS_STATES;
  window.wsm = wsm;
  window.updateMainState = updateMainState;
  window.updateHeaderCost = updateHeaderCost;
  window.flashSendBtn = flashSendBtn;
  window.stopPreviewPolling = stopPreviewPolling;
  window.connectWebSocket = connectWebSocket;
}
