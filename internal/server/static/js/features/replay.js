// internal/server/static/js/features/replay.js
// Session replay overlay + playback controls + share.
import { apiFetch } from '../core/api.js';
import { esc, html } from '../core/html.js';

let initialized = false;
let replayTimer = null;  // module-scoped state (was window._replayTimer)

function ensureInit() {
  if (initialized) return;
  initialized = true;
}

// --- Begin extracted from legacy.js:2379-2541 ---
var _replayTimeline = null;
var _replayTimer = null;
var _replayIdx = 0;
var _replaySpeed = 1;
var _replayPlaying = false;

function openReplayViewer(sessionKey) {
  if (!sessionKey) return;
  fetch('/api/sessions/replay?key=' + encodeURIComponent(sessionKey))
    .then(function(r) { return r.json(); })
    .then(function(data) {
      _replayTimeline = data;
      _replayIdx = 0;
      _replaySpeed = 1;
      _replayPlaying = false;
      showReplayOverlay();
    })
    .catch(function() { alert('Failed to load replay data'); });
}

function showReplayOverlay() {
  var existing = document.getElementById('replayOverlay');
  if (existing) existing.remove();
  var tl = _replayTimeline;
  if (!tl) return;
  var totalSec = Math.round((tl.total_duration_ms || 0) / 1000);
  var totalTime = Math.floor(totalSec/60) + ':' + String(totalSec%60).padStart(2,'0');

  var ov = document.createElement('div');
  ov.id = 'replayOverlay';
  ov.style.cssText = 'position:fixed;inset:0;z-index:9999;background:rgba(0,0,0,.85);display:flex;flex-direction:column;color:#c9d1d9;font-family:monospace';
  ov.innerHTML =
    '<div style="padding:12px 20px;border-bottom:1px solid #30363d;display:flex;align-items:center;justify-content:space-between">' +
      '<div><b>Session Replay</b> <span style="color:#8b949e;font-size:12px">' + esc(tl.session_key) + '</span></div>' +
      '<div style="display:flex;gap:8px;align-items:center">' +
        '<button onclick="shareReplaySession(\'' + escJs(tl.session_key) + '\')" style="background:#238636;border:none;color:#fff;padding:4px 12px;border-radius:4px;cursor:pointer;font-size:12px">Share</button>' +
        '<button onclick="closeReplayOverlay()" style="background:none;border:none;color:#8b949e;cursor:pointer;font-size:18px">&#x2715;</button>' +
      '</div>' +
    '</div>' +
    '<div id="replayContent" style="flex:1;overflow-y:auto;padding:16px 24px;max-width:900px;margin:0 auto;width:100%"></div>' +
    '<div style="padding:10px 20px;border-top:1px solid #30363d;display:flex;align-items:center;gap:12px">' +
      '<button id="replayPlayBtn" onclick="toggleReplayPlay()" style="background:none;border:none;color:#58a6ff;cursor:pointer;font-size:18px">&#9654;</button>' +
      '<input type="range" id="replayScrub" min="0" max="' + Math.max(tl.events.length-1,0) + '" value="0" style="flex:1;accent-color:#58a6ff" oninput="scrubReplay(this.value)">' +
      '<span id="replayTime" style="font-size:12px;color:#8b949e;min-width:80px;text-align:right">0:00 / ' + totalTime + '</span>' +
      '<div style="display:flex;gap:4px">' +
        '<button onclick="setReplaySpeed(1)" class="rs-btn" data-speed="1" style="background:#21262d;border:1px solid #30363d;color:#c9d1d9;padding:2px 6px;border-radius:3px;cursor:pointer;font-size:11px">1x</button>' +
        '<button onclick="setReplaySpeed(2)" class="rs-btn" data-speed="2" style="background:#21262d;border:1px solid #30363d;color:#c9d1d9;padding:2px 6px;border-radius:3px;cursor:pointer;font-size:11px">2x</button>' +
        '<button onclick="setReplaySpeed(4)" class="rs-btn" data-speed="4" style="background:#21262d;border:1px solid #30363d;color:#c9d1d9;padding:2px 6px;border-radius:3px;cursor:pointer;font-size:11px">4x</button>' +
      '</div>' +
      '<span style="font-size:11px;color:#6e7681">' + (tl.stats ? tl.stats.event_count + ' events, ' + tl.stats.tool_call_count + ' tools' : '') + '</span>' +
    '</div>';
  document.body.appendChild(ov);
  renderReplayUpTo(0);
}

function closeReplayOverlay() {
  stopReplayTimer();
  var ov = document.getElementById('replayOverlay');
  if (ov) ov.remove();
  _replayTimeline = null;
}

function renderReplayUpTo(idx) {
  var el = document.getElementById('replayContent');
  if (!el || !_replayTimeline) return;
  var events = _replayTimeline.events;
  var html = '';
  for (var i = 0; i <= idx && i < events.length; i++) {
    var ev = events[i];
    var icon = '', cls = 'text';
    if (ev.type === 'tool_use') { icon = '\u{1f527}'; cls = 'tool_use'; }
    else if (ev.type === 'thinking') { icon = '\u{1f4ad}'; cls = 'thinking'; }
    else if (ev.type === 'result') { icon = '\u2705'; cls = 'result'; }
    else if (ev.type === 'text') { icon = '\u{1f916}'; cls = 'text'; }
    else { icon = '\u2022'; cls = 'system'; }
    var timeSec = Math.round(ev.delta_ms / 1000);
    var timeStr = Math.floor(timeSec/60) + ':' + String(timeSec%60).padStart(2,'0');
    var content = ev.content || ev.tool_name || '';
    if (content.length > 500) content = content.substring(0, 500) + '...';
    html += '<div style="display:flex;gap:10px;padding:4px 0;opacity:' + (i === idx ? '1' : '.7') + '">' +
      '<span style="font-size:13px;min-width:22px">' + icon + '</span>' +
      '<div style="flex:1;font-size:14px;line-height:1.5">' +
        (ev.tool_name ? '<span style="color:#d2a8ff;font-size:12px;background:#21262d;padding:1px 6px;border-radius:3px">' + esc(ev.tool_name) + '</span> ' : '') +
        '<span style="color:' + (cls==='thinking'?'#a5d6ff':cls==='result'?'#7ee787':'#c9d1d9') + '">' + esc(content) + '</span>' +
      '</div>' +
      '<span style="font-size:11px;color:#484f58;min-width:40px;text-align:right">' + timeStr + '</span>' +
    '</div>';
  }
  el.innerHTML = html;
  el.scrollTop = el.scrollHeight;
  var scrub = document.getElementById('replayScrub');
  if (scrub) scrub.value = idx;
  updateReplayTime(idx);
}

function updateReplayTime(idx) {
  var el = document.getElementById('replayTime');
  if (!el || !_replayTimeline) return;
  var events = _replayTimeline.events;
  var cur = idx < events.length ? events[idx].delta_ms : _replayTimeline.total_duration_ms;
  var curSec = Math.round(cur / 1000);
  var totalSec = Math.round((_replayTimeline.total_duration_ms || 0) / 1000);
  el.textContent = Math.floor(curSec/60) + ':' + String(curSec%60).padStart(2,'0') + ' / ' + Math.floor(totalSec/60) + ':' + String(totalSec%60).padStart(2,'0');
}

function toggleReplayPlay() {
  if (_replayPlaying) { stopReplayTimer(); return; }
  _replayPlaying = true;
  var btn = document.getElementById('replayPlayBtn');
  if (btn) btn.innerHTML = '&#9646;&#9646;';
  advanceReplay();
}

function stopReplayTimer() {
  _replayPlaying = false;
  if (_replayTimer) { clearTimeout(_replayTimer); _replayTimer = null; }
  var btn = document.getElementById('replayPlayBtn');
  if (btn) btn.innerHTML = '&#9654;';
}

function advanceReplay() {
  if (!_replayPlaying || !_replayTimeline) return;
  var events = _replayTimeline.events;
  if (_replayIdx >= events.length - 1) { stopReplayTimer(); return; }
  _replayIdx++;
  renderReplayUpTo(_replayIdx);
  var delay = 100;
  if (_replayIdx < events.length - 1) {
    delay = Math.max(50, (events[_replayIdx + 1].delta_ms - events[_replayIdx].delta_ms) / _replaySpeed);
    delay = Math.min(delay, 2000); // cap at 2s
  }
  _replayTimer = setTimeout(advanceReplay, delay);
}

function scrubReplay(val) {
  _replayIdx = parseInt(val, 10);
  renderReplayUpTo(_replayIdx);
}

function setReplaySpeed(s) {
  _replaySpeed = s;
  document.querySelectorAll('.rs-btn').forEach(function(b) {
    b.style.borderColor = parseInt(b.dataset.speed) === s ? '#58a6ff' : '#30363d';
  });
}

function shareReplaySession(sessionKey) {
  fetch('/api/sessions/share', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ key: sessionKey })
  })
  .then(function(r) { return r.json(); })
  .then(function(data) {
    if (data.share_url) {
      navigator.clipboard.writeText(data.share_url).then(function() {
        alert('Share link copied to clipboard!\n\nExpires: ' + data.expires_at);
      }).catch(function() {
        prompt('Share link (copy manually):', data.share_url);
      });
    } else {
      alert('Failed to generate share link');
    }
  })
  .catch(function() { alert('Failed to generate share link'); });
}
// --- End extracted ---

export async function open(...a) { ensureInit(); return openReplayViewer(...a); }
export async function share(...a) { ensureInit(); return shareReplaySession(...a); }
// Overlay inline handlers need window-level shims; re-exported for legacy.js:
export async function closeOverlay(...a) { ensureInit(); return closeReplayOverlay(...a); }
export async function togglePlay(...a) { ensureInit(); return toggleReplayPlay(...a); }
export async function scrub(...a) { ensureInit(); return scrubReplay(...a); }
export async function speed(...a) { ensureInit(); return setReplaySpeed(...a); }
