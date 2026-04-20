// internal/server/static/js/features/file-hub.js
// File Hub: browse/upload/download/mkdir overlay.
import { apiFetch } from '../core/api.js';
import { esc, html } from '../core/html.js';

let initialized = false;
function ensureInit() {
  if (initialized) return;
  initialized = true;
}

// --- Begin extracted from legacy.js:1241-1675 ---
var fhOpen = false;
var fhPath = '';
var fhEntries = [];
var fhSelected = new Set();
var fhSessionContext = null;
var fhShowHidden = false;

async function fhFetch(endpoint, options) {
  const headers = options && options.headers ? {...options.headers} : {};
  const t = getToken();
  if (t) headers['Authorization'] = 'Bearer ' + t;
  const opts = {...(options || {}), headers};
  const r = await fetch(endpoint, opts);
  if (!r.ok) throw new Error(await r.text());
  return r.json();
}

function openFileHub(initialPath, sessionCtx) {
  if (fhOpen) return;
  fhOpen = true;
  fhSessionContext = sessionCtx || null;
  fhSelected = new Set();

  const overlay = document.createElement('div');
  overlay.className = 'fh-overlay';
  overlay.id = 'fh-overlay';
  overlay.onclick = function(e) { if (e.target === overlay) closeFileHub(); };

  const modal = document.createElement('div');
  modal.className = 'fh-modal';
  modal.innerHTML =
    '<div class="fh-header">' +
      '<div class="fh-breadcrumb" id="fh-breadcrumb"></div>' +
      '<button class="fh-toggle-hidden" id="fh-toggle-hidden" onclick="fhToggleHidden()" title="Show hidden files">.*</button>' +
      '<button class="fh-close" onclick="closeFileHub()" title="Close">&times;</button>' +
    '</div>' +
    '<div class="fh-list" id="fh-list"><div class="fh-row-empty">loading...</div></div>' +
    '<div class="fh-toolbar" id="fh-toolbar"></div>';

  overlay.appendChild(modal);
  document.body.appendChild(overlay);

  overlay._keyHandler = function(e) {
    if (e.key === 'Escape') { e.stopPropagation(); closeFileHub(); }
  };
  document.addEventListener('keydown', overlay._keyHandler, true);

  fhNavigate(initialPath || defaultWorkspace || '/home');
}

function closeFileHub() {
  const overlay = document.getElementById('fh-overlay');
  if (overlay) {
    if (overlay._keyHandler) document.removeEventListener('keydown', overlay._keyHandler, true);
    overlay.remove();
  }
  fhOpen = false;
  fhSessionContext = null;
  fhEntries = [];
  fhSelected = new Set();
}

async function fhNavigate(path) {
  fhPath = path || '/';
  fhSelected = new Set();
  const list = document.getElementById('fh-list');
  if (list) list.innerHTML = '<div class="fh-row-empty">loading...</div>';

  try {
    let url = '/api/files/list?path=' + encodeURIComponent(fhPath);
    if (fhShowHidden) url += '&hidden=true';
    const data = await fhFetch(url);
    fhPath = data.path || fhPath;
    fhEntries = data.entries || [];
    fhRenderBreadcrumb();
    fhRenderList();
    fhRenderToolbar();
  } catch (e) {
    if (list) list.innerHTML = '<div class="fh-row-empty">Error: ' + esc(e.message) + '</div>';
    fhRenderBreadcrumb();
    fhRenderToolbar();
  }
}

function fhToggleHidden() {
  fhShowHidden = !fhShowHidden;
  const btn = document.getElementById('fh-toggle-hidden');
  if (btn) btn.classList.toggle('active', fhShowHidden);
  fhNavigate(fhPath);
}

function fhRenderBreadcrumb() {
  const el = document.getElementById('fh-breadcrumb');
  if (!el) return;
  const parts = fhPath.split('/').filter(Boolean);
  let html = '<span class="fh-crumb" onclick="fhNavigate(\'/\')">/</span>';
  let accumulated = '';
  for (let i = 0; i < parts.length; i++) {
    accumulated += '/' + parts[i];
    const isLast = i === parts.length - 1;
    html += '<span class="fh-crumb-sep">/</span>';
    if (isLast) {
      html += '<span class="fh-crumb fh-crumb-current">' + esc(parts[i]) + '</span>';
    } else {
      html += '<span class="fh-crumb" onclick="fhNavigate(\'' + escJs(accumulated) + '\')">' + esc(parts[i]) + '</span>';
    }
  }
  el.innerHTML = html;
  el.scrollLeft = el.scrollWidth;
}

function fhRenderList() {
  const el = document.getElementById('fh-list');
  if (!el) return;

  if (fhEntries.length === 0) {
    el.innerHTML = '<div class="fh-row-empty">Empty directory</div>';
    return;
  }

  let html = '';
  if (fhPath !== '/') {
    html += '<div class="fh-row" onclick="fhGoUp()">' +
      '<span class="fh-row-icon">\ud83d\udcc1</span>' +
      '<span class="fh-row-name is-dir">..</span>' +
      '<span class="fh-row-meta"></span></div>';
  }

  fhEntries.forEach(function(entry, idx) {
    var isDir = entry.type === 'dir';
    var sel = fhSelected.has(idx) ? ' selected' : '';
    var icon = isDir ? '\ud83d\udcc1' : '\ud83d\udcc4';
    var nameCls = isDir ? 'fh-row-name is-dir' : 'fh-row-name';
    var sizeStr = isDir ? (entry.item_count !== undefined ? entry.item_count + ' items' : '') : fhFormatSize(entry.size || 0);
    var dateStr = entry.mod_time ? fhFormatDate(entry.mod_time) : '';

    html += '<div class="fh-row' + sel + '" data-idx="' + idx + '" onclick="fhRowClick(event,' + idx + ')" ondblclick="fhRowDblClick(' + idx + ')">' +
      '<span class="fh-row-check">\u2713</span>' +
      '<span class="fh-row-icon">' + icon + '</span>' +
      '<span class="' + nameCls + '" title="' + escAttr(entry.name) + '">' + esc(entry.name) + '</span>' +
      '<span class="fh-row-meta"><span>' + esc(sizeStr) + '</span><span>' + esc(dateStr) + '</span></span>' +
    '</div>';
  });

  el.innerHTML = html;
}

function fhRenderToolbar() {
  const el = document.getElementById('fh-toolbar');
  if (!el) return;

  const count = fhSelected.size;
  const hasSelection = count > 0;

  let html = '<div class="fh-toolbar-left">';
  html += '<button class="fh-btn primary" onclick="fhInsertPath()"' + (hasSelection ? '' : ' disabled') + '>\ud83d\udccb Insert Path</button>';
  html += '<button class="fh-btn" onclick="fhCopyPath()"' + (hasSelection ? '' : ' disabled') + '>\ud83d\udcc4 Copy</button>';
  html += '<button class="fh-btn" onclick="fhShowUpload()">\u2b06 Upload</button>';
  html += '<button class="fh-btn" onclick="fhDownloadSelected()"' + (hasSelection ? '' : ' disabled') + '>\u2b07 Download</button>';
  html += '<button class="fh-btn" onclick="fhPromptMkdir()">\ud83d\udcc1 New Folder</button>';
  html += '<button class="fh-btn danger" onclick="fhDeleteSelected()"' + (hasSelection ? '' : ' disabled') + '>\ud83d\uddd1 Delete</button>';
  html += '</div>';

  if (hasSelection) {
    html += '<span class="fh-sel-count">' + count + ' selected</span>';
  }

  el.innerHTML = html;
}

function fhRowClick(ev, idx) {
  if (ev.target.closest('.fh-row-check') || ev.ctrlKey || ev.metaKey || ev.shiftKey) {
    fhToggle(idx, ev);
    return;
  }
  var entry = fhEntries[idx];
  if (entry && entry.type === 'dir') {
    fhNavigate(fhPath + '/' + entry.name);
    return;
  }
  fhToggle(idx, ev);
}

function fhRowDblClick(idx) {
  var entry = fhEntries[idx];
  if (!entry) return;
  if (entry.type === 'dir') {
    fhNavigate(fhPath + '/' + entry.name);
  } else {
    fhSelected = new Set([idx]);
    fhInsertPath();
  }
}

function fhGoUp() {
  var parent = fhPath.replace(/\/[^/]+\/?$/, '') || '/';
  fhNavigate(parent);
}

function fhToggle(idx) {
  if (fhSelected.has(idx)) {
    fhSelected.delete(idx);
  } else {
    fhSelected.add(idx);
  }
  var rows = document.querySelectorAll('#fh-list .fh-row[data-idx]');
  rows.forEach(function(row) {
    var i = parseInt(row.dataset.idx);
    row.classList.toggle('selected', fhSelected.has(i));
  });
  fhRenderToolbar();
}

function fhFormatSize(bytes) {
  if (bytes === 0) return '0B';
  if (bytes < 1024) return bytes + 'B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + 'K';
  if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + 'M';
  return (bytes / (1024 * 1024 * 1024)).toFixed(1) + 'G';
}

function fhFormatDate(iso) {
  if (!iso) return '';
  var d = new Date(iso);
  var months = ['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'];
  return months[d.getMonth()] + ' ' + d.getDate();
}

function fhGetSelectedPaths() {
  var paths = [];
  fhSelected.forEach(function(idx) {
    var entry = fhEntries[idx];
    if (entry) {
      var p = fhPath.endsWith('/') ? fhPath + entry.name : fhPath + '/' + entry.name;
      paths.push(p);
    }
  });
  return paths;
}

function fhInsertPath() {
  var paths = fhGetSelectedPaths();
  if (paths.length === 0) return;
  closeFileHub();

  var input = document.getElementById('msg-input');
  if (!input) return;

  var pathStr = paths.join(' ');
  var current = getMsgValue(input);
  if (current) {
    setMsgValue(input, current + ' ' + pathStr);
  } else {
    setMsgValue(input, pathStr);
  }
  input.focus();
}

function fhCopyPath() {
  var paths = fhGetSelectedPaths();
  if (paths.length === 0) return;
  copyText(paths.join('\n'));
}

function fhShowUpload() {
  var list = document.getElementById('fh-list');
  if (!list) return;

  list.innerHTML =
    '<div class="fh-upload-zone" id="fh-upload-zone" onclick="document.getElementById(\'fh-upload-input\').click()">' +
      '<div class="fh-upload-zone-icon">\u2b06</div>' +
      '<div class="fh-upload-zone-text">Click or drag files here to upload</div>' +
      '<div class="fh-upload-zone-hint">Files will be uploaded to ' + esc(fhPath) + '</div>' +
      '<input type="file" id="fh-upload-input" class="fh-upload-input" multiple onchange="fhUploadFiles(this.files)">' +
    '</div>' +
    '<div class="fh-progress-list" id="fh-progress-list"></div>';

  var zone = document.getElementById('fh-upload-zone');
  zone.addEventListener('dragover', function(e) { e.preventDefault(); zone.classList.add('dragover'); });
  zone.addEventListener('dragleave', function() { zone.classList.remove('dragover'); });
  zone.addEventListener('drop', function(e) { e.preventDefault(); zone.classList.remove('dragover'); fhDrop(e); });

  var toolbar = document.getElementById('fh-toolbar');
  if (toolbar) {
    toolbar.innerHTML = '<button class="fh-btn" onclick="fhNavigate(fhPath)">\u2190 Back to files</button>';
  }
}

function fhDrop(ev) {
  var files = ev.dataTransfer.files;
  if (files && files.length > 0) fhUploadFiles(files);
}

async function fhUploadFiles(fileList) {
  if (!fileList || fileList.length === 0) return;

  var progressList = document.getElementById('fh-progress-list');
  if (!progressList) return;

  for (var i = 0; i < fileList.length; i++) {
    var file = fileList[i];
    var itemId = 'fh-progress-' + i;
    progressList.innerHTML +=
      '<div class="fh-progress-item" id="' + itemId + '">' +
        '<span class="fh-progress-name">' + esc(file.name) + '</span>' +
        '<div class="fh-progress-bar"><div class="fh-progress-bar-fill" id="' + itemId + '-bar" style="width:0%"></div></div>' +
        '<span class="fh-progress-status" id="' + itemId + '-status">0%</span>' +
      '</div>';
  }

  for (var i = 0; i < fileList.length; i++) {
    var file = fileList[i];
    var itemId = 'fh-progress-' + i;
    try {
      var fd = new FormData();
      fd.append('dest', fhPath);
      fd.append('file', file);

      var t = getToken();

      await new Promise(function(resolve, reject) {
        var capturedId = itemId;
        var xhr = new XMLHttpRequest();
        xhr.open('POST', '/api/files/upload');
        if (t) xhr.setRequestHeader('Authorization', 'Bearer ' + t);

        xhr.upload.onprogress = function(e) {
          if (e.lengthComputable) {
            var pct = Math.round(e.loaded / e.total * 100);
            var bar = document.getElementById(capturedId + '-bar');
            var status = document.getElementById(capturedId + '-status');
            if (bar) bar.style.width = pct + '%';
            if (status) status.textContent = pct + '%';
          }
        };

        xhr.onload = function() {
          var bar = document.getElementById(capturedId + '-bar');
          var status = document.getElementById(capturedId + '-status');
          if (xhr.status >= 200 && xhr.status < 300) {
            if (bar) { bar.style.background = '#238636'; bar.style.width = '100%'; }
            if (status) { status.textContent = '\u2713'; status.style.color = '#3fb950'; }
            resolve();
          } else {
            if (bar) bar.style.background = '#da3633';
            if (status) { status.textContent = 'error'; status.style.color = '#da3633'; }
            reject(new Error(xhr.responseText || 'upload failed'));
          }
        };

        xhr.onerror = function() {
          var status = document.getElementById(capturedId + '-status');
          if (status) { status.textContent = 'error'; status.style.color = '#da3633'; }
          reject(new Error('network error'));
        };

        xhr.send(fd);
      });
    } catch (e) {
      console.error('upload error:', e);
    }
  }

  showToast('Upload complete', 'success');
  setTimeout(function() { fhNavigate(fhPath); }, 500);
}

function fhDownloadSelected() {
  var paths = fhGetSelectedPaths();
  if (paths.length === 0) return;

  fhSelected.forEach(function(idx) {
    var entry = fhEntries[idx];
    if (!entry || entry.type === 'dir') return;
    var p = fhPath.endsWith('/') ? fhPath + entry.name : fhPath + '/' + entry.name;
    var a = document.createElement('a');
    a.href = '/api/files/download?path=' + encodeURIComponent(p);
    a.download = entry.name;
    a.style.display = 'none';
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
  });
}

async function fhPromptMkdir() {
  var name = prompt('New folder name:');
  if (!name || !name.trim()) return;

  var newPath = fhPath.endsWith('/') ? fhPath + name.trim() : fhPath + '/' + name.trim();
  try {
    var headers = {'Content-Type': 'application/json'};
    var t = getToken();
    if (t) headers['Authorization'] = 'Bearer ' + t;
    var r = await fetch('/api/files/mkdir', {
      method: 'POST', headers: headers,
      body: JSON.stringify({path: newPath})
    });
    if (!r.ok) throw new Error(await r.text());
    showToast('Folder created', 'success');
    fhNavigate(fhPath);
  } catch (e) {
    showToast('mkdir failed: ' + e.message);
  }
}

async function fhDeleteSelected() {
  var paths = fhGetSelectedPaths();
  if (paths.length === 0) return;

  var names = paths.map(function(p) { return p.split('/').pop(); }).join(', ');
  if (!confirm('Delete ' + paths.length + ' item(s)?\n' + names)) return;

  var errors = 0;
  for (var i = 0; i < paths.length; i++) {
    try {
      var headers = {};
      var t = getToken();
      if (t) headers['Authorization'] = 'Bearer ' + t;
      var r = await fetch('/api/files/delete?path=' + encodeURIComponent(paths[i]), {method: 'DELETE', headers: headers});
      if (!r.ok) errors++;
    } catch (e) {
      errors++;
    }
  }

  if (errors > 0) showToast(errors + ' delete(s) failed', 'warning');
  else showToast('Deleted', 'success');
  fhNavigate(fhPath);
}
// --- End extracted ---

// --- Extracted from legacy.js:1716 ---
function fhLsNavigate(path) {
  var input = document.getElementById('msg-input');
  if (input) {
    setMsgValue(input, '/ls ' + path);
    sendMessage();
  }
}
// --- End extracted ---

export async function open(...a) { ensureInit(); return openFileHub(...a); }
export async function close(...a) { ensureInit(); return closeFileHub(...a); }
export async function lsNavigate(...a) { ensureInit(); return fhLsNavigate(...a); }
// Overlay inline handlers:
export async function navigate(...a) { ensureInit(); return fhNavigate(...a); }
export async function toggleHidden(...a) { ensureInit(); return fhToggleHidden(...a); }
export async function rowClick(...a) { ensureInit(); return fhRowClick(...a); }
export async function rowDblClick(...a) { ensureInit(); return fhRowDblClick(...a); }
export async function goUp(...a) { ensureInit(); return fhGoUp(...a); }
export async function toggle(...a) { ensureInit(); return fhToggle(...a); }
export async function insertPath(...a) { ensureInit(); return fhInsertPath(...a); }
export async function copyPath(...a) { ensureInit(); return fhCopyPath(...a); }
export async function showUpload(...a) { ensureInit(); return fhShowUpload(...a); }
export async function drop(...a) { ensureInit(); return fhDrop(...a); }
export async function downloadSelected(...a) { ensureInit(); return fhDownloadSelected(...a); }
export async function promptMkdir(...a) { ensureInit(); return fhPromptMkdir(...a); }
export async function deleteSelected(...a) { ensureInit(); return fhDeleteSelected(...a); }
