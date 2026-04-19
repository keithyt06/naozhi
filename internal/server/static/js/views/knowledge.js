// js/views/knowledge.js — Knowledge (Obsidian vault) view.
//
// Moved verbatim from dashboard.html's inline <script> (Task 11:
// Knowledge View + Knowledge AI Chat banners). Owns: vault tree
// rendering + filter, file load + KaTeX/Shiki post-processing,
// wikilink resolution, breadcrumb navigation, and the note-scoped
// AI chat compose flow.
//
// Shared globals still live on `window.*` via the state.js
// defineProperty bridge (sessionCounter, defaultWorkspace,
// sessionWorkspaces, lastVersion). Helpers that stay in the legacy
// inline script (showToast, loadKatex, runKatex, renderKatex, esc,
// highlightBlock, debouncedFetchSessions, switchView,
// selectSession, authHeaders) are accessed via window during Phase
// 1 and will migrate to core modules in Phase 2.

import { esc, escAttr, escJs } from '../core/html.js';

// Module-local state that pre-split lived as bare `var` globals.
// Bridged onto window below so legacy callers keep working.
var kvCurrentPath = '';
var kvCachedTree = null;

function authHeaders() {
  return (typeof window !== 'undefined' && typeof window.authHeaders === 'function')
    ? window.authHeaders() : {};
}

function showToast(msg, type, duration) {
  if (typeof window !== 'undefined' && typeof window.showToast === 'function') {
    window.showToast(msg, type, duration);
  }
}

function loadKatex() {
  if (typeof window !== 'undefined' && typeof window.loadKatex === 'function') {
    return window.loadKatex();
  }
}

function runKatex() {
  if (typeof window !== 'undefined' && typeof window.runKatex === 'function') {
    return window.runKatex();
  }
}

function renderKatex(tex, displayMode) {
  if (typeof window !== 'undefined' && typeof window.renderKatex === 'function') {
    return window.renderKatex(tex, displayMode);
  }
  return '';
}

function highlightBlock(el) {
  if (typeof window !== 'undefined' && typeof window.highlightBlock === 'function') {
    try { window.highlightBlock(el); } catch (e) {}
  }
}

// ------- entry point ----------------------------------------------

export function renderKnowledgeView() {
  const main = document.getElementById('main');
  main.innerHTML =
    '<div class="kv-container show-tree" id="kvContainer">' +
      '<div class="kv-tree" id="kvTree">' +
        '<div class="kv-tree-header">' +
          '<span>Vault</span>' +
          '<input class="kv-search" id="kvSearchInput" placeholder="Search files..." oninput="filterVaultTree(this.value)">' +
        '</div>' +
        '<div id="kvTreeContent"><div class="kv-loading">Loading vault...</div></div>' +
      '</div>' +
      '<div class="kv-content-area">' +
        '<button class="kv-mobile-back" id="kvMobileBack" onclick="kvShowTree()">\u2190 Back to vault tree</button>' +
        '<div class="kv-breadcrumb" id="kvBreadcrumb"></div>' +
        '<div class="kv-rendered" id="kvContent"><div class="kv-empty"><span style="font-size:28px;opacity:.3">&#128218;</span><span>Select a file from the vault tree</span></div></div>' +
        '<div class="kv-ai-response" id="kvAiResponse"></div>' +
        '<div class="kv-ai-chat" id="kvAiChat">' +
          '<div class="kv-ai-label">Ask about this note...</div>' +
          '<div class="kv-ai-row">' +
            '<input class="kv-ai-input" id="kvAiInput" type="text" placeholder="Ask a question about the current note..." onkeydown="if(event.key===\'Enter\'&&!event.shiftKey){event.preventDefault();kvAiAsk()}">' +
            '<button class="kv-ai-send" id="kvAiSendBtn" onclick="kvAiAsk()">Ask</button>' +
          '</div>' +
        '</div>' +
      '</div>' +
    '</div>';
  loadVaultTree();
}

export function kvShowTree() {
  var container = document.getElementById('kvContainer');
  if (container) container.classList.add('show-tree');
}

export async function loadVaultTree() {
  const container = document.getElementById('kvTreeContent');
  if (!container) return;
  try {
    const tree = await fetch('/api/vault/tree', { headers: authHeaders() }).then(r => {
      if (!r.ok) throw new Error('vault not configured');
      return r.json();
    });
    if (!tree || (!tree.children && !tree.name)) {
      container.innerHTML = '<div class="kv-loading" style="color:#6e7681">Vault not configured</div>';
      return;
    }
    kvCachedTree = tree.children || [tree];
    container._treeData = kvCachedTree;
    container.innerHTML = renderTreeNode(kvCachedTree, 0);
  } catch (e) {
    container.innerHTML = '<div class="kv-loading" style="color:#6e7681">' + esc(e.message || 'Failed to load vault') + '</div>';
  }
}

export function filterVaultTree(query) {
  var container = document.getElementById('kvTreeContent');
  if (!container || !kvCachedTree) return;
  query = (query || '').trim().toLowerCase();
  if (!query) {
    container.innerHTML = renderTreeNode(kvCachedTree, 0);
    return;
  }
  var filtered = filterTreeNodes(kvCachedTree, query);
  if (filtered.length === 0) {
    container.innerHTML = '<div class="kv-loading" style="color:#6e7681">No matching files</div>';
    return;
  }
  container.innerHTML = renderTreeNode(filtered, 0, query);
  // Auto-expand all directories in filtered results
  container.querySelectorAll('.kv-children').forEach(function(el) { el.classList.add('open'); });
  container.querySelectorAll('.kv-dir-arrow').forEach(function(el) { el.classList.add('open'); });
}

export function filterTreeNodes(nodes, query) {
  var results = [];
  for (var i = 0; i < nodes.length; i++) {
    var n = nodes[i];
    if (n.is_dir) {
      var childMatches = filterTreeNodes(n.children || [], query);
      var dirNameMatches = n.name.toLowerCase().indexOf(query) !== -1;
      if (childMatches.length > 0 || dirNameMatches) {
        results.push({ name: n.name, path: n.path, is_dir: true, children: childMatches.length > 0 ? childMatches : n.children, file_count: n.file_count });
      }
    } else {
      if (n.name.toLowerCase().indexOf(query) !== -1 || n.path.toLowerCase().indexOf(query) !== -1) {
        results.push(n);
      }
    }
  }
  return results;
}

export function highlightMatch(text, query) {
  if (!query) return esc(text);
  var lower = text.toLowerCase();
  var idx = lower.indexOf(query.toLowerCase());
  if (idx === -1) return esc(text);
  return esc(text.substring(0, idx)) + '<span class="kv-highlight">' + esc(text.substring(idx, idx + query.length)) + '</span>' + esc(text.substring(idx + query.length));
}

export function renderTreeNode(nodes, depth, query) {
  if (!nodes || nodes.length === 0) return '';
  let html = '<ul style="padding-left:' + (depth > 0 ? 12 : 0) + 'px">';
  // Sort: dirs first, then files, alphabetical
  const sorted = nodes.slice().sort((a, b) => {
    if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
    return (a.name || '').localeCompare(b.name || '');
  });
  for (const n of sorted) {
    if (n.is_dir) {
      const childHtml = renderTreeNode(n.children || [], depth + 1, query);
      html += '<li>' +
        '<div class="kv-dir" onclick="toggleKvDir(this)">' +
          '<span class="kv-dir-arrow">\u25B6</span>' +
          (query ? highlightMatch(n.name, query) : esc(n.name)) +
        '</div>' +
        '<div class="kv-children">' + childHtml + '</div>' +
      '</li>';
    } else {
      html += '<li><div class="kv-file" data-path="' + escAttr(n.path) + '" onclick="loadVaultFile(\'' + escJs(n.path) + '\')">' +
        (query ? highlightMatch(n.name.replace(/\.md$/, ''), query) : esc(n.name.replace(/\.md$/, ''))) + '</div></li>';
    }
  }
  html += '</ul>';
  return html;
}

export function toggleKvDir(el) {
  const arrow = el.querySelector('.kv-dir-arrow');
  const children = el.nextElementSibling;
  if (children) children.classList.toggle('open');
  if (arrow) arrow.classList.toggle('open');
}

// Find a vault file path matching a wikilink target name
export function findVaultFilePath(target) {
  if (!target) return null;
  var name = target.replace(/\.md$/, '').toLowerCase();
  var found = null;
  function search(nodes) {
    if (!nodes || found) return;
    for (var i = 0; i < nodes.length; i++) {
      var n = nodes[i];
      if (!n.is_dir) {
        var fname = (n.name || '').replace(/\.md$/, '').toLowerCase();
        if (fname === name) { found = n.path; return; }
      }
      if (n.children) search(n.children);
    }
  }
  var tree = document.getElementById('kvTreeContent');
  if (tree && tree._treeData) search(tree._treeData);
  return found;
}

export async function loadVaultFile(path) {
  kvCurrentPath = path;
  // Highlight active file
  document.querySelectorAll('#kvTree .kv-file').forEach(f => f.classList.toggle('active', f.dataset.path === path));
  // Update breadcrumb with clickable segments
  var bc = document.getElementById('kvBreadcrumb');
  if (bc) {
    if (!path) {
      bc.innerHTML = '<span class="kv-crumb-current">Vault</span>';
    } else {
      var parts = path.split('/');
      var bcHtml = '<span class="kv-crumb" onclick="kvShowTree();kvExpandToFolder(\'\')">Vault</span>';
      var cumPath = '';
      for (var i = 0; i < parts.length; i++) {
        cumPath += (i > 0 ? '/' : '') + parts[i];
        bcHtml += '<span class="kv-sep">/</span>';
        if (i < parts.length - 1) {
          bcHtml += '<span class="kv-crumb" onclick="kvExpandToFolder(\'' + escJs(cumPath) + '\')">' + esc(parts[i]) + '</span>';
        } else {
          bcHtml += '<span class="kv-crumb-current">' + esc(parts[i].replace(/\.md$/, '')) + '</span>';
        }
      }
      bc.innerHTML = bcHtml;
    }
  }
  // If path is empty, show empty state
  if (!path) {
    var content0 = document.getElementById('kvContent');
    if (content0) content0.innerHTML = '<div class="kv-empty"><span style="font-size:28px;opacity:.3">&#128218;</span><span>Select a file from the vault tree</span></div>';
    return;
  }
  // Load content
  var content = document.getElementById('kvContent');
  if (!content) return;
  content.innerHTML = '<div class="kv-loading">Loading...</div>';
  try {
    var data = await fetch('/api/vault/read?path=' + encodeURIComponent(path), { headers: authHeaders() }).then(function(r) {
      if (!r.ok) throw new Error('File not found');
      return r.json();
    });
    var html = '';
    // Frontmatter
    if (data.frontmatter && typeof data.frontmatter === 'object' && Object.keys(data.frontmatter).length > 0) {
      html += '<div class="kv-frontmatter">';
      for (var _k of Object.keys(data.frontmatter)) {
        var _v = data.frontmatter[_k];
        var val = Array.isArray(_v) ? _v.join(', ') : String(_v);
        html += '<div class="kv-fm-key">' + esc(_k) + '</div><div class="kv-fm-val">' + esc(val) + '</div>';
      }
      html += '</div>';
    }
    html += data.html || '<p style="color:#6e7681">Empty file</p>';
    content.innerHTML = html;
    // Wire wikilink clicks
    content.querySelectorAll('a.wikilink[data-target], a[data-wiki]').forEach(function(a) {
      a.addEventListener('click', function(e) {
        e.preventDefault();
        var target = this.getAttribute('data-target') || this.getAttribute('data-wiki');
        if (target) {
          // Search vault tree for matching file
          var filePath = findVaultFilePath(target);
          if (filePath) loadVaultFile(filePath);
          else loadVaultFile(target.endsWith('.md') ? target : target + '.md');
        }
      });
    });
    // Wire foldable callout click handlers (data-foldable instead of onclick to pass XSS sanitizer)
    content.querySelectorAll('.callout[data-foldable]').forEach(function(el) {
      el.style.cursor = 'pointer';
      el.addEventListener('click', function() { this.classList.toggle('folded'); });
    });
    // Apply Shiki syntax highlighting to code blocks in vault content
    if (window.shikiHighlighter) {
      content.querySelectorAll('pre code, .md-pre[data-lang]').forEach(function(el) {
        try { highlightBlock(el); } catch(ex) {}
      });
    }
    // Render KaTeX math formulas in vault content
    // Process display math ($$...$$) and inline math ($...$)
    kvRenderMath(content);
    // Mobile: hide tree, show content
    var container = document.getElementById('kvContainer');
    if (container) container.classList.remove('show-tree');
  } catch (e) {
    content.innerHTML = '<div class="kv-empty"><span>' + esc(e.message) + '</span></div>';
  }
}

// Process math in vault-rendered HTML: find $$...$$ and $...$ in text nodes and render with KaTeX
export function kvRenderMath(container) {
  if (!container) return;
  // Load KaTeX JS if not already loaded
  loadKatex();
  var html = container.innerHTML;
  // Display math: $$...$$
  html = html.replace(/\$\$([\s\S]+?)\$\$/g, function(match, tex) {
    return '<div class="md-math-display">' + renderKatex(tex.trim(), true) + '</div>';
  });
  // Inline math: $...$ (but not $$)
  html = html.replace(/(?<!\$)\$(?!\$)([^\$\n]+?)\$(?!\$)/g, function(match, tex) {
    return renderKatex(tex.trim(), false);
  });
  if (html !== container.innerHTML) {
    container.innerHTML = html;
    // Re-wire wikilinks and callouts after innerHTML replacement
    container.querySelectorAll('a.wikilink[data-target]').forEach(function(a) {
      a.addEventListener('click', function(e) {
        e.preventDefault();
        var target = this.getAttribute('data-target');
        if (target) {
          var filePath = findVaultFilePath(target);
          if (filePath) loadVaultFile(filePath);
          else loadVaultFile(target.endsWith('.md') ? target : target + '.md');
        }
      });
    });
    container.querySelectorAll('.callout[data-foldable]').forEach(function(el) {
      el.style.cursor = 'pointer';
      el.addEventListener('click', function() { this.classList.toggle('folded'); });
    });
    // Run pending KaTeX renders
    runKatex();
  }
}

export function kvExpandToFolder(folderPath) {
  // Expand tree nodes to reach the specified folder path
  var treeContainer = document.getElementById('kvTreeContent');
  if (!treeContainer) return;
  // Open all dir nodes along the path
  var dirs = treeContainer.querySelectorAll('.kv-dir');
  dirs.forEach(function(dirEl) {
    var dirText = dirEl.textContent.trim();
    var children = dirEl.nextElementSibling;
    var arrow = dirEl.querySelector('.kv-dir-arrow');
    // Walk up through parents to build full path
    var pathParts = [];
    var node = dirEl;
    while (node) {
      var parentDir = node.closest('.kv-children');
      if (!parentDir) break;
      var parentDirEl = parentDir.previousElementSibling;
      if (parentDirEl && parentDirEl.classList.contains('kv-dir')) {
        pathParts.unshift(parentDirEl.textContent.trim());
        node = parentDirEl;
      } else {
        break;
      }
    }
    pathParts.push(dirText);
    var fullPath = pathParts.join('/');
    // If this dir is part of the target path, expand it
    if (folderPath.indexOf(fullPath) === 0 || fullPath.indexOf(folderPath) === 0) {
      if (children) children.classList.add('open');
      if (arrow) arrow.classList.add('open');
    }
  });
}

// ------- Knowledge AI Chat (Gap 5) --------------------------------

export async function kvAiAsk() {
  const input = document.getElementById('kvAiInput');
  const btn = document.getElementById('kvAiSendBtn');
  const responseEl = document.getElementById('kvAiResponse');
  if (!input || !btn) return;
  const question = input.value.trim();
  if (!question) return;
  if (!kvCurrentPath) {
    showToast('Select a note first');
    return;
  }

  // Disable input while processing
  btn.disabled = true;
  btn.textContent = 'Asking...';
  input.disabled = true;
  if (responseEl) {
    responseEl.classList.add('show');
    responseEl.innerHTML = '<span style="color:#8b949e">Thinking...</span>';
  }

  try {
    // Get the note content
    const noteData = await fetch('/api/vault/read?path=' + encodeURIComponent(kvCurrentPath), { headers: authHeaders() })
      .then(r => r.ok ? r.json() : null);
    const noteContent = noteData ? (noteData.raw || noteData.content || '') : '';
    const noteTitle = kvCurrentPath.split('/').pop().replace(/\.md$/, '');

    // Create a new chat session with the note as context
    var sessionCounter_ = ++sessionCounter;
    var now = new Date();
    var ts = now.toISOString().slice(0,10) + '-' +
      now.toTimeString().slice(0,8).replace(/:/g, '') + '-' + sessionCounter_;
    var key = 'dashboard:direct:' + ts + ':knowledge-qa';

    var workspace = defaultWorkspace || '/tmp';
    sessionWorkspaces[key] = workspace;

    // Build the contextual message
    var contextMsg = 'I am reading a note titled "' + noteTitle + '" from my Obsidian vault. Here is its content:\n\n---\n' +
      noteContent.substring(0, 4000) +
      (noteContent.length > 4000 ? '\n...(truncated)' : '') +
      '\n---\n\nMy question: ' + question;

    // Send via the sessions API
    var sendPayload = {
      key: key,
      text: contextMsg,
      workspace: workspace
    };
    delete sessionWorkspaces[key]; // consumed by send

    var resp = await fetch('/api/sessions/send', {
      method: 'POST',
      headers: { ...authHeaders(), 'Content-Type': 'application/json' },
      body: JSON.stringify(sendPayload)
    });

    if (!resp.ok) throw new Error('Failed to create chat session');

    // Show success and offer to open the session
    if (responseEl) {
      responseEl.innerHTML =
        '<div style="font-size:12px;color:#3fb950;margin-bottom:6px">Session created with note context.</div>' +
        '<div style="font-size:12px;color:#8b949e">The AI is processing your question. ' +
        '<a href="#" style="color:#58a6ff" onclick="event.preventDefault();switchView(\'chat\',document.querySelector(\'[data-view=chat]\'));setTimeout(function(){selectSession(\'' + escJs(key) + '\',\'local\')},100)">Open chat session</a> to see the response.</div>';
    }
    input.value = '';

    // Refresh sessions list to pick up the new session
    lastVersion = 0;
    debouncedFetchSessions();
  } catch (e) {
    if (responseEl) {
      responseEl.innerHTML = '<span style="color:#da3633">Error: ' + esc(e.message) + '</span>';
    }
    showToast('AI chat failed: ' + e.message);
  } finally {
    btn.disabled = false;
    btn.textContent = 'Ask';
    input.disabled = false;
    input.focus();
  }
}

// ------- Legacy bridges — removed after Phase 2 -------------------

if (typeof window !== 'undefined') {
  window.renderKnowledgeView = renderKnowledgeView;
  window.kvShowTree = kvShowTree;
  window.loadVaultTree = loadVaultTree;
  window.filterVaultTree = filterVaultTree;
  window.filterTreeNodes = filterTreeNodes;
  window.highlightMatch = highlightMatch;
  window.renderTreeNode = renderTreeNode;
  window.toggleKvDir = toggleKvDir;
  window.findVaultFilePath = findVaultFilePath;
  window.loadVaultFile = loadVaultFile;
  window.kvRenderMath = kvRenderMath;
  window.kvExpandToFolder = kvExpandToFolder;
  window.kvAiAsk = kvAiAsk;
}
