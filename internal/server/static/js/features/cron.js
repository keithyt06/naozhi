// internal/server/static/js/features/cron.js
// Cron job management: create/list/pause/resume/delete cron jobs.
import { apiFetch } from '../core/api.js';
import { esc, html } from '../core/html.js';

let initialized = false;
function ensureInit() {
  if (initialized) return;
  initialized = true;
}

// --- Begin extracted from legacy.js:760-1070 ---
let cronJobs = [];

function createNewCronJob() {
  const presets = [
    { label: 'Every 30 min', value: '@every 30m' },
    { label: 'Every hour', value: '@every 1h' },
    { label: 'Daily 9:00', value: '0 9 * * *' },
    { label: 'Weekdays 9:00', value: '0 9 * * 1-5' },
    { label: 'Every Monday 9:00', value: '0 9 * * 1' },
  ];
  let selectedSchedule = '';
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  let scheduleHtml =
    '<ul class="proj-pick" id="cron-schedule-list">' +
    presets.map(p =>
      '<li data-value="' + escAttr(p.value) + '" onclick="cronSelectSchedule(this, \'' + escJs(p.value) + '\')">' +
        '<div class="pp-name">' + esc(p.label) + '</div>' +
        '<div class="pp-path">' + esc(p.value) + '</div>' +
      '</li>'
    ).join('') +
    '<li id="cron-custom-toggle" onclick="toggleCronCustom()">' +
      '<div class="pp-custom"><span class="pp-custom-icon">&#9881;</span> Custom expression</div>' +
    '</li>' +
    '</ul>' +
    '<div id="cron-custom-form" style="display:none;margin-top:8px">' +
      '<input id="cron-schedule" placeholder="@every 30m or 0 9 * * 1-5">' +
      '<div id="cron-preview-hint" style="font-size:11px;color:#8b949e;margin-top:4px;min-height:16px"></div>' +
    '</div>';

  // Workspace picker
  let wsHtml = '<div style="margin-top:12px"><div style="font-size:12px;color:#8b949e;margin-bottom:6px">Workspace (optional)</div>';
  if (projectsData.length > 0) {
    wsHtml += '<ul class="proj-pick" id="cron-ws-list">' +
      projectsData.map(p =>
        '<li data-path="' + escAttr(p.path) + '" onclick="cronSelectWorkspace(this, \'' + escJs(p.path) + '\')">' +
          '<div class="pp-name">' + esc(p.name) + '</div>' +
          '<div class="pp-path">' + esc(shortPath(p.path)) + '</div>' +
        '</li>'
      ).join('') +
      '<li id="cron-ws-custom-toggle" onclick="toggleCronWsCustom()">' +
        '<div class="pp-custom"><span class="pp-custom-icon">+</span> Custom path</div>' +
      '</li>' +
      '</ul>';
  }
  wsHtml += '<div id="cron-ws-custom-form" style="' + (projectsData.length > 0 ? 'display:none;' : '') + 'margin-top:4px">' +
    '<input id="cron-workdir" placeholder="' + escAttr(defaultWorkspace || '/home/user/project') + '">' +
    '</div></div>';

  overlay.innerHTML =
    '<div class="modal">' +
      '<h3>New Cron Job</h3>' +
      '<div style="margin-bottom:12px">' +
        '<div style="font-size:12px;color:#8b949e;margin-bottom:6px">Prompt</div>' +
        '<textarea id="cron-prompt" placeholder="what should this job do?" style="width:100%;min-height:60px;max-height:120px;padding:8px 12px;background:#0d1117;border:1px solid #30363d;border-radius:6px;color:#c9d1d9;font-size:14px;font-family:inherit;resize:vertical;outline:none"></textarea>' +
      '</div>' +
      '<div style="font-size:12px;color:#8b949e;margin-bottom:6px">Schedule</div>' +
      scheduleHtml + wsHtml +
      '<div class="modal-btns" style="margin-top:12px"><button onclick="this.closest(\'.modal-overlay\').remove()">cancel</button><button class="primary" onclick="doCreateCronJob()">create</button></div>' +
    '</div>';
  document.body.appendChild(overlay);
  overlay.addEventListener('keydown', function(e) {
    if (e.key === 'Escape') overlay.remove();
  });
  overlay._cronSchedule = '';
  overlay._cronWorkDir = '';
}

function cronSelectSchedule(el, value) {
  const overlay = el.closest('.modal-overlay');
  overlay._cronSchedule = value;
  document.querySelectorAll('#cron-schedule-list li').forEach(li => li.style.background = '');
  el.style.background = '#1f6feb33';
  // Hide custom form and clear its value when preset selected
  const customForm = document.getElementById('cron-custom-form');
  if (customForm) customForm.style.display = 'none';
  const customInput = document.getElementById('cron-schedule');
  if (customInput) customInput.value = '';
  const toggle = document.getElementById('cron-custom-toggle');
  if (toggle) toggle.style.display = '';
}

function cronSelectWorkspace(el, path) {
  const overlay = el.closest('.modal-overlay');
  overlay._cronWorkDir = path;
  document.querySelectorAll('#cron-ws-list li').forEach(li => li.style.background = '');
  el.style.background = '#1f6feb33';
  const customForm = document.getElementById('cron-ws-custom-form');
  if (customForm) customForm.style.display = 'none';
  const toggle = document.getElementById('cron-ws-custom-toggle');
  if (toggle) toggle.style.display = '';
}

function toggleCronWsCustom() {
  const form = document.getElementById('cron-ws-custom-form');
  const toggle = document.getElementById('cron-ws-custom-toggle');
  if (form.style.display === 'none') {
    form.style.display = '';
    if (toggle) toggle.style.display = 'none';
    // Clear project selection
    const overlay = form.closest('.modal-overlay');
    if (overlay) overlay._cronWorkDir = '';
    document.querySelectorAll('#cron-ws-list li').forEach(li => li.style.background = '');
    document.getElementById('cron-workdir').focus();
  } else {
    form.style.display = 'none';
    if (toggle) toggle.style.display = '';
  }
}

function toggleCronCustom() {
  const form = document.getElementById('cron-custom-form');
  const toggle = document.getElementById('cron-custom-toggle');
  if (form.style.display === 'none') {
    form.style.display = '';
    toggle.style.display = 'none';
    // Clear preset selection
    const overlay = form.closest('.modal-overlay');
    if (overlay) overlay._cronSchedule = '';
    document.querySelectorAll('#cron-schedule-list li').forEach(li => li.style.background = '');
    const input = document.getElementById('cron-schedule');
    input.focus();
    if (!input._cronPreviewBound) {
      let previewTimer;
      input.addEventListener('input', function() {
        clearTimeout(previewTimer);
        previewTimer = setTimeout(() => previewCronSchedule(input.value.trim()), 300);
      });
      input._cronPreviewBound = true;
    }
  } else {
    form.style.display = 'none';
    toggle.style.display = '';
  }
}

async function previewCronSchedule(schedule) {
  const hint = document.getElementById('cron-preview-hint');
  if (!hint) return;
  if (!schedule) { hint.textContent = ''; return; }
  try {
    const headers = {};
    const t = getToken();
    if (t) headers['Authorization'] = 'Bearer ' + t;
    const r = await fetch('/api/cron/preview?schedule=' + encodeURIComponent(schedule), { headers });
    const data = await r.json();
    if (data.valid) {
      hint.style.color = '#7ee787';
      hint.textContent = 'next run: ' + timeAgo(data.next_run, true);
    } else {
      hint.style.color = '#da3633';
      hint.textContent = data.error || 'invalid schedule';
    }
  } catch (e) {
    hint.style.color = '#da3633';
    hint.textContent = 'preview error';
  }
}

async function doCreateCronJob() {
  const overlay = document.querySelector('.modal-overlay');
  if (!overlay) return;
  // Resolve schedule: preset selection or custom input
  let schedule = overlay._cronSchedule || '';
  const customInput = document.getElementById('cron-schedule');
  if (customInput && customInput.value.trim()) schedule = customInput.value.trim();
  if (!schedule) { showToast('schedule is required', 'warning'); return; }
  // Resolve prompt
  const promptInput = document.getElementById('cron-prompt');
  const prompt = promptInput ? promptInput.value.trim() : '';
  // Resolve work_dir: project selection or custom input
  let workDir = overlay._cronWorkDir || '';
  const wdInput = document.getElementById('cron-workdir');
  if (wdInput && wdInput.value.trim()) workDir = wdInput.value.trim();
  try {
    const headers = {'Content-Type': 'application/json'};
    const t = getToken();
    if (t) headers['Authorization'] = 'Bearer ' + t;
    const body = {schedule};
    if (prompt) body.prompt = prompt;
    if (workDir) body.work_dir = workDir;
    const r = await fetch('/api/cron', {method: 'POST', headers, body: JSON.stringify(body)});
    if (!r.ok) { showToast('create failed: ' + await r.text()); return; }
    const data = await r.json();
    if (overlay) overlay.remove();
    showToast('cron job created', 'success');
    fetchCronJobs();
    if (data.id) {
      const key = 'cron:' + data.id;
      sessionWorkspaces[key] = workDir || defaultWorkspace || '/tmp';
      lastVersion = 0;
      await fetchSessions();
      selectSession(key, 'local');
    }
  } catch (e) { showToast('error: ' + e.message); }
}

function openCronPanel() {
  selectedKey = null; selectedNode = null;
  if (wsm.subscribedKey) wsm.unsubscribe();
  if (eventTimer) { clearInterval(eventTimer); eventTimer = null; }
  document.querySelectorAll('.session-card').forEach(el => el.classList.remove('active'));
  mobileEnterChat();
  fetchCronJobs().then(() => renderCronPanel());
}

function renderCronPanel() {
  const main = document.getElementById('main');
  let html = '<div class="cron-detail">' +
    '<div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:12px">' +
      '<h3 style="margin:0">Cron Jobs</h3>' +
      '<button onclick="createNewCronJob()" style="padding:5px 14px;border-radius:6px;border:1px solid #30363d;background:#21262d;color:#c9d1d9;cursor:pointer;font-size:12px;display:flex;align-items:center;gap:4px"><svg width="14" height="14" viewBox="0 0 24 24" stroke="currentColor" fill="none" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg> New</button>' +
    '</div>';
  if (cronJobs.length === 0) {
    html += '<div style="text-align:center;padding:40px 20px">' +
      '<div style="font-size:36px;opacity:.3;margin-bottom:12px">&#9201;</div>' +
      '<div style="color:#8b949e;margin-bottom:16px">No cron jobs yet</div>' +
      '<button onclick="createNewCronJob()" style="padding:8px 20px;border-radius:6px;border:1px solid #1f6feb;background:#1f6feb22;color:#58a6ff;cursor:pointer;font-size:13px">Create your first cron job</button>' +
    '</div>';
  } else {
    cronJobs.sort((a, b) => b.created_at - a.created_at);
    html += cronJobs.map(j => {
      const status = j.paused ? '<span class="badge suspended">paused</span>' : '<span class="badge running">active</span>';
      const nextStr = j.next_run ? timeAgo(j.next_run, true) : '';
      const lastStr = j.last_run_at ? timeAgo(j.last_run_at) : '';
      const wdStr = j.work_dir ? '<span style="color:#a5d6ff" title="' + escAttr(j.work_dir) + '">' + esc(shortPath(j.work_dir)) + '</span>' : '';
      const result = j.last_error
        ? '<div style="color:#da3633;font-size:12px;margin-top:4px">\u2716 ' + esc(j.last_error) + '</div>'
        : (j.last_result ? '<div style="color:#7ee787;font-size:12px;margin-top:4px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">\u2714 ' + esc(j.last_result.substring(0, 150)) + '</div>' : '');
      return '<div style="padding:12px;border:1px solid #30363d;border-radius:8px;margin-bottom:8px;background:#161b22;cursor:pointer" onclick="openCronSession(\'' + escJs(j.id) + '\')">' +
        '<div style="font-size:14px;color:#f0f6fc;font-weight:500">' + (j.prompt ? esc(j.prompt) : '<span style="color:#6e7681">(no prompt — click to set)</span>') + '</div>' +
        '<div style="font-size:12px;color:#a5d6ff;margin-top:4px">' + esc(j.schedule) + '</div>' +
        '<div style="font-size:12px;color:#8b949e;margin-top:4px;display:flex;gap:8px;align-items:center;flex-wrap:wrap">' +
        status +
        wdStr +
        (lastStr ? '<span>ran ' + lastStr + '</span>' : '') +
        (nextStr ? '<span>next ' + nextStr + '</span>' : '') +
        '</div>' +
        result +
        '<div style="display:flex;gap:6px;margin-top:8px" onclick="event.stopPropagation()">' +
        (j.paused
          ? '<button onclick="cronResume(\'' + escJs(j.id) + '\')" style="padding:3px 10px;border-radius:4px;border:1px solid #30363d;background:#21262d;color:#c9d1d9;cursor:pointer;font-size:11px">resume</button>'
          : '<button onclick="cronPause(\'' + escJs(j.id) + '\')" style="padding:3px 10px;border-radius:4px;border:1px solid #30363d;background:#21262d;color:#c9d1d9;cursor:pointer;font-size:11px">pause</button>') +
        '<button onclick="cronDelete(\'' + escJs(j.id) + '\')" style="padding:3px 10px;border-radius:4px;border:1px solid #da3633;color:#da3633;background:transparent;cursor:pointer;font-size:11px">delete</button>' +
        '</div></div>';
    }).join('');
  }
  html += '</div>';
  main.innerHTML = html;
}

function openCronSession(cronId) {
  const key = 'cron:' + cronId;
  // Ensure the session appears in the sidebar (may be pending if never sent)
  if (!sessionsData[sid(key, 'local')] && !sessionWorkspaces[key]) {
    sessionWorkspaces[key] = defaultWorkspace || '/tmp';
    lastVersion = 0;
    debouncedFetchSessions();
  }
  selectSession(key, 'local');
}

async function fetchCronJobs() {
  try {
    const headers = {};
    const t = getToken();
    if (t) headers['Authorization'] = 'Bearer ' + t;
    const r = await fetch('/api/cron', { headers });
    if (!r.ok) return;
    const data = await r.json();
    cronJobs = data.jobs || [];
    const cronBadge = document.getElementById('cron-badge');
    if (cronBadge) { cronBadge.textContent = cronJobs.length; cronBadge.style.display = cronJobs.length > 0 ? '' : 'none'; }
  } catch (e) { console.error('fetch cron:', e); }
}

async function cronPause(id) {
  try {
    const headers = { 'Content-Type': 'application/json' };
    const t = getToken();
    if (t) headers['Authorization'] = 'Bearer ' + t;
    const r = await fetch('/api/cron/pause', { method: 'POST', headers, body: JSON.stringify({ id }) });
    if (!r.ok) { showToast('pause failed'); return; }
    fetchCronJobs().then(() => renderCronPanel());
  } catch (e) { showToast('error: ' + e.message); }
}

async function cronResume(id) {
  try {
    const headers = { 'Content-Type': 'application/json' };
    const t = getToken();
    if (t) headers['Authorization'] = 'Bearer ' + t;
    const r = await fetch('/api/cron/resume', { method: 'POST', headers, body: JSON.stringify({ id }) });
    if (!r.ok) { showToast('resume failed'); return; }
    fetchCronJobs().then(() => renderCronPanel());
  } catch (e) { showToast('error: ' + e.message); }
}

async function cronDelete(id) {
  if (!confirm('Delete cron job ' + id + '?')) return;
  try {
    const headers = {};
    const t = getToken();
    if (t) headers['Authorization'] = 'Bearer ' + t;
    const r = await fetch('/api/cron?id=' + encodeURIComponent(id), { method: 'DELETE', headers });
    if (!r.ok) { showToast('delete failed'); return; }
    fetchCronJobs().then(() => renderCronPanel());
  } catch (e) { showToast('error: ' + e.message); }
}
// --- End extracted ---

export async function open(...a) { ensureInit(); return openCronPanel(...a); }
export async function createNew(...a) { ensureInit(); return createNewCronJob(...a); }
export async function doCreate(...a) { ensureInit(); return doCreateCronJob(...a); }
export async function selectSchedule(...a) { ensureInit(); return cronSelectSchedule(...a); }
export async function selectWorkspace(...a) { ensureInit(); return cronSelectWorkspace(...a); }
export async function toggleWsCustom(...a) { ensureInit(); return toggleCronWsCustom(...a); }
export async function toggleCustom(...a) { ensureInit(); return toggleCronCustom(...a); }
export async function preview(...a) { ensureInit(); return previewCronSchedule(...a); }
export async function openSession(...a) { ensureInit(); return openCronSession(...a); }
export async function pause(...a) { ensureInit(); return cronPause(...a); }
export async function resume(...a) { ensureInit(); return cronResume(...a); }
export async function remove(...a) { ensureInit(); return cronDelete(...a); }
