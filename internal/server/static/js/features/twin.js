// internal/server/static/js/features/twin.js
// CTO Digital Twin overlay + config editor + test query.
import { esc } from '../core/html.js';

let initialized = false;
function ensureInit() {
  if (initialized) return;
  initialized = true;
}

// --- Begin extracted from legacy.js:2224-2334 ---
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
// --- End extracted ---

export async function open(...a) { ensureInit(); return openTwinPanel(...a); }
export async function closeOverlay(...a) { ensureInit(); return closeTwinOverlay(...a); }
export async function updateField(...a) { ensureInit(); return updateTwinField(...a); }
export async function testQuery(...a) { ensureInit(); return testTwinQuery(...a); }
