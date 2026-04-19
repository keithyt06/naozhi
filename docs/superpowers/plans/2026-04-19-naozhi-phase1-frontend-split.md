# Phase 1: Frontend Dashboard Split — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Cut `internal/server/static/dashboard.html` from a single 8143-line monolith (961 LOC inline CSS + 7026 LOC inline JS + HTML shell) into a lean HTML shell (< 500 lines) plus lazily-loaded ES-module view chunks, so first-paint JS ships < 50 KB gzip and view switches stay < 50 ms after warmup.

**Architecture:** Pure refactor — zero behaviour change. CSS moves to `static/css/base.css` + `static/css/views/*.css`; JS moves to `static/js/core/*.js` + `static/js/views/<name>.js` with `export { mount, unmount }`. A new `js/core/router.js` registers a view table, dynamic `import()` lazy-loads view modules, and the old `switchView()` bridges via `window.switchView` so `onclick="switchView(...)"` attributes keep working during the transition. Go `embed.FS` is switched from per-file to `//go:embed all:static` + a generated `manifest.json` that maps logical names (`app.js`, `base.css`, `views/chat.js`) to content-hashed filenames (`app.abc123.js`). The dashboard HTML is served as a Go template that reads the manifest and renders `<link>` / `<script type="module">` URLs with one-year immutable cache headers.

**Tech Stack:** Go 1.22 `net/http` + `embed`, vanilla ES modules (Safari 10.1+ native), no Node build. A single bash/Go script under `tools/hashstatic/` walks `internal/server/static/`, writes hashed copies to `internal/server/static/dist/`, and emits `manifest.json`. Dynamic `import()` is what drives lazy-loading; no bundler.

**TDD note:** This is a refactor with no new observable behaviour. The safety net is (a) `go build ./... && go test ./...` green at every commit, (b) `go vet ./...` clean, (c) manual smoke test after every view migration (Task 10 onwards), (d) content grep assertions confirming nothing was left behind or duplicated. Each view migration Task repeats the same smoke checklist.

---

## File Structure

### New directories

```
internal/server/static/
  dashboard.html                 # thinned shell, rendered via html/template
  css/
    base.css                     # globals, reset, layout, sidebar, main, inputs
    components.css               # toast, modal, buttons, popovers, dropdowns
    views/
      home.css
      chat.css
      knowledge.css
      wiki.css
      patrols.css
      approvals.css
      graph.css
  js/
    app.js                       # bootstrap: auth, ws, router, sidebar
    core/
      router.js                  # view registry + switchView + history API
      ws.js                      # WebSocket hub client (subscribe/send/event)
      api.js                     # fetch + authHeaders + retry
      state.js                   # window-scoped mutable app state (sessionsData, selectedKey...)
      html.js                    # esc/escAttr/escJs/html tagged template
      utils.js                   # debounce, isToday, isYesterday, matchesTagFilter
    views/
      home.js                    # export { mount, unmount }
      chat.js                    # main/sidebar chat rendering + WS event fanout
      knowledge.js
      wiki.js
      patrols.js
      approvals.js
      graph.js                   # d3-force lazy-loaded here, not globally
  dist/                          # BUILD OUTPUT — generated, gitignored
    manifest.json
    app.<hash>.js
    css/base.<hash>.css
    css/views/chat.<hash>.css
    js/core/router.<hash>.js
    js/views/chat.<hash>.js
    ...
```

### Go files touched

| Path | Change |
|---|---|
| `internal/server/dashboard.go` | Replace single-file embed with `//go:embed all:static` FS; add `handleStatic` + `handleDashboardTmpl`; parse manifest once at init; serve dashboard from `html/template` |
| `internal/server/dashboard.go` | Update `Cache-Control` policy: `dashboard.html` short-cache, everything in `/static/dist/` `public, max-age=31536000, immutable` |
| `tools/hashstatic/main.go` | New ~120-line tool: walks `internal/server/static/{css,js,sw.js}`, sha256-trims to 8 hex, writes hashed copies to `internal/server/static/dist/`, emits `manifest.json` |
| `Makefile` (new, or `scripts/build.sh`) | Target `static` runs hashstatic; `build` depends on `static` |
| `.gitignore` | Add `internal/server/static/dist/` |
| `internal/server/static/sw.js` | Update cache key to include current manifest hash so old bundles get evicted on upgrade |

### Build pipeline

```
make static      -> go run ./tools/hashstatic    -> writes static/dist/ + manifest.json
make build       -> make static && CGO_ENABLED=0 go build -o bin/naozhi ./cmd/naozhi
make deploy      -> make build && ssh+replace binary (unchanged)
```

### Source-of-truth for "what view owns what"

| View | CSS source sections (current line numbers in `dashboard.html`) | JS source sections |
|---|---|---|
| base      | 14-334 (globals, sidebar, main, chat scaffolding, mobile breakpoints, code, markdown) | 1077-1112 globals, 1119-1157 util, 1210-1305 token/fetch, 1375-1503 history popover, 5507-5553 switchView |
| components| 335-574 (modals, toast, notifications, cron, file hub, cmd-k) | 1555-2500 ws + auth + notifications (approx — confirm via grep) |
| home      | 581-609 + 695-717 home widgets | 5557-5775 |
| chat      | 14-334 shares with base; 52-100 message bubbles | 1558-5526 main + sidebar + event stream (largest block) |
| knowledge | 718-845 | 6264-6668 |
| wiki      | 846-930 | 6669-7761 (includes decision journal + sources panel) |
| patrols   | 610-655 + 695-717 | 5776-5953 + 6215-6239 |
| approvals | 656-694 + 695-717 | 5954-6214 + 6240-6263 |
| graph     | (none — inline SVG styles in renderGraphView) | 7762-end + d3 CDN import |

These ranges are starting points. During each migration task the agent greps for the section markers (`/* ===== ... ===== */`) to pin exact boundaries before cutting.

---

## Task 1: Baseline & safety net

**Files:** none modified.

- [ ] **Step 1.1: Confirm on correct branch**

Run: `git -C /root/keith-space/AWS/EC2-Workload/naozhi status`
Expected: `On branch naozhi-2.0`, working tree clean (Phase 0 already committed + tagged `phase-0-meeting-removed`). Any pre-existing `M`/untracked from unrelated work: note in commit-skip list.

- [ ] **Step 1.2: Baseline build + test**

Run from `/root/keith-space/AWS/EC2-Workload/naozhi`:
```bash
go build ./... && go test ./... 2>&1 | tail -20
```
Expected: green. If any test was already red, record the name; Phase 1 steps must not add new reds.

- [ ] **Step 1.3: Create rollback branch (Risk 1)**

Run:
```bash
git -C /root/keith-space/AWS/EC2-Workload/naozhi branch naozhi-2.0-pre-split
git -C /root/keith-space/AWS/EC2-Workload/naozhi rev-parse naozhi-2.0-pre-split
```
Expected: prints a commit hash matching current `HEAD`. This branch is the "panic revert" target — if any point post-split produces a regression nobody can fix in a day, we hard-reset `naozhi-2.0` back to this.

- [ ] **Step 1.4: Snapshot size baseline**

Run:
```bash
wc -l internal/server/static/dashboard.html
awk 'NR==14,NR==974' internal/server/static/dashboard.html | gzip -c | wc -c
awk 'NR==1077,NR==8102' internal/server/static/dashboard.html | gzip -c | wc -c
```
Expected: ~8143 total lines, ~14.6 KB gzipped CSS, ~72 KB gzipped JS. These numbers define "before" in the Phase 1 acceptance table.

Record in commit message for Task 26 (final commit).

- [ ] **Step 1.5: Verify iOS Safari target (Risk 3)**

Run: `grep -c "type=\"module\"" internal/server/static/dashboard.html`
Expected: `1` (the existing shiki `<script type="module">`). ES modules are already used in production; native Safari 10.1+ support is proven. No polyfill work required. Record decision.

---

## Task 2: Scaffold the new directory layout

**Files:** Create empty files so every later task is a pure edit.

- [ ] **Step 2.1: Make directories**

```bash
cd /root/keith-space/AWS/EC2-Workload/naozhi/internal/server/static
mkdir -p css/views js/core js/views
```

- [ ] **Step 2.2: Create empty module stubs**

Create each of the following with a one-line JSDoc so `go:embed` has non-empty files (Go embed is OK with empty files, but the stubs document intent):

```bash
cd /root/keith-space/AWS/EC2-Workload/naozhi/internal/server/static
for f in css/base.css css/components.css \
         css/views/home.css css/views/chat.css css/views/knowledge.css \
         css/views/wiki.css css/views/patrols.css css/views/approvals.css \
         css/views/graph.css; do
  printf "/* %s — Phase 1 split placeholder */\n" "$f" > "$f"
done
for f in js/app.js js/core/router.js js/core/ws.js js/core/api.js \
         js/core/state.js js/core/html.js js/core/utils.js \
         js/views/home.js js/views/chat.js js/views/knowledge.js \
         js/views/wiki.js js/views/patrols.js js/views/approvals.js \
         js/views/graph.js; do
  printf "// %s — Phase 1 split placeholder\n" "$f" > "$f"
done
```

- [ ] **Step 2.3: Verify tree**

Run: `find internal/server/static -type f | sort`
Expected: lists `dashboard.html`, `manifest.json`, `sw.js`, plus all the new stubs under `css/` and `js/`.

- [ ] **Step 2.4: Commit the scaffold**

```bash
git add internal/server/static/css internal/server/static/js
git commit -m "$(cat <<'EOF'
chore(frontend): scaffold css/js module tree for Phase 1 split

Empty placeholders for base/components/views CSS and core/views JS.
Content will be moved out of dashboard.html in subsequent commits.

Phase 1.1/1.2 of 2026-04-18-naozhi-ddd-refactor-design.md.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Extract CSS — base.css + components.css

**Files:** Modify `internal/server/static/dashboard.html`; write `css/base.css`, `css/components.css`.

- [ ] **Step 3.1: Read the full `<style>` block**

Run: `sed -n '14,974p' internal/server/static/dashboard.html | head -5 && echo '---' && sed -n '14,974p' internal/server/static/dashboard.html | tail -5`

Expected: block starts at `*{margin:0;padding:0;box-sizing:border-box}` and ends with the last rule before `</style>` (mobile media query tail or knowledge CSS tail depending on current file). Confirm the two boundary lines.

- [ ] **Step 3.2: Split the style block into sections**

Using the section-markers table in File Structure above, cut the following ranges into the new files (paste content, do not paraphrase):

- Lines 14-334 (globals + chat bubbles + mobile breakpoints + code/diff/markdown) → `css/base.css`
- Lines 335-574 (modal, toast, cron, file hub, cmd-k, notification, new-session modal, tag filter) → `css/components.css`

Keep the `/* ===== ... ===== */` section banners intact — they're navigation aids inside each file.

- [ ] **Step 3.3: Replace the inline block with a single `<link>` placeholder**

In `dashboard.html` replace lines 14-974 (`<style>…</style>`) with:
```html
<link rel="stylesheet" href="/static/css/base.css">
<link rel="stylesheet" href="/static/css/components.css">
<!-- per-view CSS links injected by app.js dynamic loader -->
```

Do not delete the `<script type="module">` Shiki block that currently lives at lines 976-1004 — it stays in the shell for now (it's imported from ESM CDN and runs before app.js).

- [ ] **Step 3.4: Verify line counts**

```bash
wc -l internal/server/static/css/base.css internal/server/static/css/components.css internal/server/static/dashboard.html
```
Expected rough split: base ~320 LOC, components ~240 LOC, dashboard.html drops by ~955 LOC.

- [ ] **Step 3.5: Smoke test (temporary — `/static/` route doesn't exist yet)**

This step will fail until Task 6 wires the static FS. Proceed to Task 4 first; the full build + browser smoke test happens at the end of Task 6.

- [ ] **Step 3.6: Commit**

```bash
git add internal/server/static/css/base.css \
        internal/server/static/css/components.css \
        internal/server/static/dashboard.html
git commit -m "$(cat <<'EOF'
refactor(frontend): extract base + components CSS from dashboard.html

Moves globals, layout, modals, toast, notifications, cmd-k overlay
into css/base.css + css/components.css. View-specific CSS stays in
dashboard.html for now (extracted per-view in Tasks 4-5).

Static files still inlined by gzip path; new /static/ route lands in
Task 6. Smoke test deferred.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Extract per-view CSS

**Files:** Modify `dashboard.html`; write `css/views/*.css`.

- [ ] **Step 4.1: Map remaining `<style>` content to views**

Run:
```bash
grep -n "^/\* =====" internal/server/static/dashboard.html | head -40
```
Expected: you see the section banners still in the file post-Task 3 — for Home, Patrols, Approvals, Home Widgets, Knowledge, Wiki.

- [ ] **Step 4.2: Extract each block**

For each view, cut the matching `/* ===== <View> ===== */` ... up to the line before the next `/* ===== */` into the corresponding `css/views/<name>.css`:

- `Home View (Task 13)` + `Home Patrol/Approval Widgets (Task 20)` → `css/views/home.css`
- `Patrols View (Task 17)` → `css/views/patrols.css`
- `Approvals View (Task 18)` → `css/views/approvals.css`
- `Knowledge View (Task 11)` + obsidian checkbox + foldable callouts + Knowledge AI Chat Panel → `css/views/knowledge.css`
- `Wiki View (Task 12)` + Wiki Sources Panel → `css/views/wiki.css`
- `Context Panel (Task 14)` + `Bookmark Button on Messages (Task 15)` → `css/views/chat.css`

Graph view has no dedicated style block — graph styles are inline in `renderGraphView()`. Create `css/views/graph.css` with only the section banner comment; the inline styles move into this file in Task 13 (graph migration).

- [ ] **Step 4.3: Replace the entire remaining `<style>…</style>` with per-view link tags**

The shell now looks like:
```html
<link rel="stylesheet" href="/static/css/base.css">
<link rel="stylesheet" href="/static/css/components.css">
<link rel="stylesheet" href="/static/css/views/home.css">
<link rel="stylesheet" href="/static/css/views/chat.css">
<link rel="stylesheet" href="/static/css/views/knowledge.css">
<link rel="stylesheet" href="/static/css/views/wiki.css">
<link rel="stylesheet" href="/static/css/views/patrols.css">
<link rel="stylesheet" href="/static/css/views/approvals.css">
<link rel="stylesheet" href="/static/css/views/graph.css">
```

Note: all view CSS is in the shell for now (i.e. not lazy-loaded yet). Task 15 switches non-first-paint views to dynamic `<link>` injection. Eager loading all CSS is fine — total is ~15 KB gzip.

- [ ] **Step 4.4: Verify no CSS left in HTML**

Run:
```bash
grep -n "<style>\|</style>" internal/server/static/dashboard.html
```
Expected: no output.

- [ ] **Step 4.5: Verify nothing was dropped**

```bash
wc -l internal/server/static/css/base.css internal/server/static/css/components.css internal/server/static/css/views/*.css
```
Sum should be approximately 961 (original style block size). Tolerance ±5 lines for comment headers and blank-line normalisation.

- [ ] **Step 4.6: Commit**

```bash
git add internal/server/static/css/views/ internal/server/static/dashboard.html
git commit -m "$(cat <<'EOF'
refactor(frontend): extract per-view CSS to css/views/*.css

Moves home/chat/knowledge/wiki/patrols/approvals/graph style blocks
out of dashboard.html and into css/views/. Graph view gets an empty
placeholder — its inline styles will move during Task 13 graph split.

Phase 1.1 of 2026-04-18-naozhi-ddd-refactor-design.md.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Write the static file-server Go path

**Files:** Modify `internal/server/dashboard.go`.

We need `/static/` serving CSS/JS before the browser can load the links in Task 4. No hashing yet (that's Task 14); this task just switches from per-file embed to directory embed and adds a route.

- [ ] **Step 5.1: Replace the three `//go:embed` directives with a single directory embed**

Open `internal/server/dashboard.go`. Replace lines 29-36:

```go
//go:embed static/dashboard.html
var dashboardHTML embed.FS

//go:embed static/manifest.json
var manifestJSON embed.FS

//go:embed static/sw.js
var swJS embed.FS
```

with:

```go
//go:embed all:static
var staticFS embed.FS
```

- [ ] **Step 5.2: Update `handleDashboard` to read from `staticFS`**

Change `dashboardHTML.ReadFile("static/dashboard.html")` to `staticFS.ReadFile("static/dashboard.html")`. Same for the `init()` function that computes ETag.

- [ ] **Step 5.3: Update `handleManifest` and `handleSW`**

Change `manifestJSON.ReadFile(...)` to `staticFS.ReadFile(...)` and `swJS.ReadFile(...)` to `staticFS.ReadFile(...)`.

- [ ] **Step 5.4: Add `handleStatic` for `/static/*`**

Append after `handleSW`:

```go
// handleStatic serves files from internal/server/static under /static/*.
// Phase 1: short cache (1h). Task 14 switches hashed files to immutable 1y.
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/static/")
	if path == "" || strings.Contains(path, "..") {
		http.NotFound(w, r)
		return
	}
	data, err := staticFS.ReadFile("static/" + path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case strings.HasSuffix(path, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(path, ".js"):
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case strings.HasSuffix(path, ".json"):
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") &&
		(strings.HasSuffix(path, ".css") || strings.HasSuffix(path, ".js")) {
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		_, _ = gz.Write(data)
		return
	}
	_, _ = w.Write(data)
}
```

- [ ] **Step 5.5: Register the route**

In `registerDashboard()` add, right below the existing `/sw.js` line:

```go
s.mux.HandleFunc("GET /static/", s.handleStatic)
```

- [ ] **Step 5.6: Update CSP to allow `/static/` self-source**

The existing CSP already has `'self'` in `style-src` and `script-src`, so `/static/` URLs work. No change needed — but verify by reading the `Content-Security-Policy` header line and confirming `'self'` in both.

- [ ] **Step 5.7: Build and run locally**

```bash
go build ./... && go vet ./...
CGO_ENABLED=0 go build -o bin/naozhi ./cmd/naozhi/
./bin/naozhi --config config.yaml &
sleep 2
curl -sI http://localhost:8080/static/css/base.css | head -3
curl -sI http://localhost:8080/static/js/app.js | head -3
kill %1 2>/dev/null
```
Expected: `HTTP/1.1 200 OK` for both. If 404, re-verify file paths and the embed directive.

- [ ] **Step 5.8: Browser smoke test — does the dashboard still render with external CSS?**

From your local browser (or SSH-tunnel from production EC2):
1. Load `http://localhost:8080/dashboard` (assuming local naozhi bound to :8080 with auth disabled or bearer provided)
2. Open DevTools → Network → filter `static` — confirm `base.css`, `components.css`, and all 7 view CSS files load with status 200
3. The dashboard should render **identically** to before (all 7 nav buttons, chat view layout, approvals button, etc.)
4. No CSP violations in console

If any view looks unstyled: you left a `<style>` fragment behind or dropped rules during Task 4. Diff the old `<style>` range against the concatenated `css/*.css` to find the gap.

- [ ] **Step 5.9: Commit**

```bash
git add internal/server/dashboard.go
git commit -m "$(cat <<'EOF'
feat(server): serve /static/* from embedded FS (Phase 1.1)

Replaces per-file embed with //go:embed all:static. Adds handleStatic
with short cache + gzip for css/js. Dashboard.html links to
/static/css/*.css now work end-to-end.

Phase 1.1 of 2026-04-18-naozhi-ddd-refactor-design.md.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Extract core JS — utils, html, api, state

**Files:** Modify `dashboard.html`; write `js/core/utils.js`, `js/core/html.js`, `js/core/api.js`, `js/core/state.js`.

- [x] **Step 6.1: Write `js/core/html.js`**

Replace the placeholder with:

```js
// js/core/html.js — HTML escaping + tagged template helper.
// Kept under 50 LOC, no dependencies.

export function esc(s) {
  if (s == null) return '';
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

export function escAttr(s) { return esc(s); }

export function escJs(s) {
  if (s == null) return '';
  return String(s).replace(/\\/g, '\\\\').replace(/'/g, "\\'").replace(/\n/g, '\\n');
}

export function html(strings, ...values) {
  let out = strings[0];
  for (let i = 0; i < values.length; i++) {
    out += esc(values[i]) + strings[i + 1];
  }
  return out;
}

// Legacy globals — the migrated view modules import these, but existing
// inline onclick="switchView('chat',this)" handlers still resolve via window.
// Removed after Task 13 (graph migration completes).
if (typeof window !== 'undefined') {
  window.esc = esc;
  window.escAttr = escAttr;
  window.escJs = escJs;
}
```

- [x] **Step 6.2: Write `js/core/utils.js`**

Grep the current `dashboard.html` for `function isToday`, `function isYesterday`, `function matchesTagFilter`, `function filterByTag`, `function debounce` (if any), `function majorMinor`, `function cliIcon`. Move each intact into `js/core/utils.js` and add `export` to every function. At the bottom, for each export, add `window.<name> = <name>` to keep existing inline handlers working. Target line count: ~80 LOC.

- [x] **Step 6.3: Write `js/core/api.js`**

Grep for `function getToken`, `function setToken`, `function authHeaders` (search — it's used everywhere). Move them plus any shared `fetchJSON`/`fetchWithRetry` helpers into `js/core/api.js` with named exports + `window.*` legacy bridge.

If a shared `fetchJSON` does not exist yet, create one:

```js
// js/core/api.js
export function getToken() { return ''; }
export function setToken(t) { /* HttpOnly cookie, no-op */ }

export function authHeaders() {
  // Auth is cookie-based today; this function returns an empty header map
  // as a compatibility shim for all fetch calls. Do not change until
  // Phase 2 server DDD unifies auth middleware.
  return {};
}

export async function apiFetch(url, opts = {}) {
  const merged = Object.assign({}, opts);
  merged.headers = Object.assign({}, authHeaders(), opts.headers || {});
  const resp = await fetch(url, merged);
  if (!resp.ok) throw new Error(`${url}: HTTP ${resp.status}`);
  return resp;
}

export async function apiJSON(url, opts) {
  const resp = await apiFetch(url, opts);
  return resp.json();
}

if (typeof window !== 'undefined') {
  window.getToken = getToken;
  window.setToken = setToken;
  window.authHeaders = authHeaders;
  window.apiFetch = apiFetch;
  window.apiJSON = apiJSON;
}
```

Only replace the legacy `authHeaders()` implementation if it's currently `function authHeaders() { return {}; }`. If it does more (e.g. attaches a token), preserve that logic verbatim.

- [x] **Step 6.4: Write `js/core/state.js`**

Move the `let selectedKey`, `let sessionsData`, `let allSessionsCache`, etc. globals (currently at dashboard.html lines 1085-1116) into `js/core/state.js`:

```js
// js/core/state.js — shared mutable app state.
// Exported as individual bindings. Because ES module bindings are
// live references, assigning to window.X is required to keep pre-split
// code reading/writing the same value. Removed in Phase 2 when all
// views are modules.

export const state = {
  selectedKey: null,
  eventTimer: null,
  lastEventTime: 0,
  lastRenderedEventTime: 0,
  lastCompositionEnd: 0,
  sessionsData: {},
  allSessionsCache: [],
  pendingFiles: [],
  sending: false,
  selectedNode: 'local',
  nodesData: {},
  lastVersion: 0,
  lastNodesJSON: '',
  lastHistoryJSON: '',
  sessionPollTimer: null,
  discoveredPollTimer: null,
  discoveredItems: [],
  notifications: [],
  notifIdCounter: 0,
  previewTimer: null,
  previewEventCount: 0,
  pendingDiscovered: null,
  sessionCounter: 0,
  availableAgents: ['general'],
  defaultWorkspace: '',
  projectsData: [],
  localWsInfo: { name: '', sys: '' },
  sessionWorkspaces: {},
  sessionNodes: {},
  sessionDrafts: {},
  historySessionsData: [],
  activeTagFilter: 'all',
  currentView: 'chat',
};

// Legacy bridge: keep window.selectedKey etc. as getters/setters so
// un-migrated code reads/writes the same store.
if (typeof window !== 'undefined') {
  for (const key of Object.keys(state)) {
    Object.defineProperty(window, key, {
      get() { return state[key]; },
      set(v) { state[key] = v; },
      configurable: true,
    });
  }
}
```

**Critical:** `let selectedKey = null` in the current code is a lexical binding; `window.selectedKey` won't shadow it. After this task, delete the `let selectedKey = ...` lines at dashboard.html:1085-1116. The `defineProperty` bridge makes `window.selectedKey = 'foo'` and `selectedKey === 'foo'` both work for legacy code, because the legacy code runs as a plain `<script>` (not a module) in the remaining inline block, so unqualified `selectedKey` resolves through the global object.

- [x] **Step 6.5: Delete the migrated globals + helpers from `dashboard.html`**

In `dashboard.html`:
1. Remove lines 1085-1116 (the `let ... ` globals now in `state.js`)
2. Remove the `function isToday`, `isYesterday`, `filterByTag`, `matchesTagFilter`, `majorMinor`, `cliIcon` definitions (now in `utils.js`)
3. Remove the `function getToken`, `setToken`, `authHeaders` definitions (now in `api.js`)
4. Remove any standalone `function esc`, `escAttr`, `escJs` (now in `html.js`)

- [x] **Step 6.6: Add the bootstrap module loader at the bottom of `<script>…</script>`**

**Do not** convert the entire remaining inline `<script>` to `type="module"` yet — that breaks `onclick="..."` resolution. Instead, leave the inline `<script>…</script>` as is, and **above it** (just after `</head>` or inside `<body>` top) inject a small preload:

```html
<script type="module">
  import '/static/js/core/html.js';
  import '/static/js/core/utils.js';
  import '/static/js/core/api.js';
  import '/static/js/core/state.js';
  // Signals to the legacy inline script that core is ready.
  window.__naozhiCoreReady = true;
</script>
```

Put this immediately before the legacy `<script>` that starts at line 1077. Because modules are deferred by default, and the inline `<script>` is parser-blocking, we need to gate the inline script. Option: wrap the body of the legacy script in:

```js
if (!window.__naozhiCoreReady) {
  window.addEventListener('DOMContentLoaded', () => { /* original body */ });
} else {
  /* original body */
}
```

Simpler: use `defer` on the inline block by **moving** the legacy inline `<script>` block into a new file `js/legacy.js` and loading it as `<script src="/static/js/legacy.js" defer></script>`. Modules and defer scripts both execute after HTML parsing in document order. This is the approach we take:

- Cut lines 1077-8102 (the entire inline `<script>…</script>` body) into `internal/server/static/js/legacy.js`
- Replace those lines in `dashboard.html` with `<script src="/static/js/legacy.js" defer></script>`
- The module imports of `html.js` / `utils.js` / `api.js` / `state.js` run before `legacy.js` because modules are deferred by default and registered earlier in the document

- [x] **Step 6.7: Build and smoke test**

```bash
go build ./... && CGO_ENABLED=0 go build -o bin/naozhi ./cmd/naozhi/
./bin/naozhi --config config.yaml &
sleep 2
curl -sI http://localhost:8080/static/js/legacy.js | head -3
curl -sI http://localhost:8080/static/js/core/state.js | head -3
kill %1 2>/dev/null
```
Expected: both return 200. In browser, reload `/dashboard`, open DevTools Console — no `ReferenceError`, session list renders, clicking a session opens chat. Any `selectedKey is not defined` → the `state.js` bridge didn't land — re-check Step 6.4.

- [x] **Step 6.8: Verify line counts**

```bash
wc -l internal/server/static/dashboard.html internal/server/static/js/legacy.js internal/server/static/js/core/*.js
```
Expected: dashboard.html ≲ 150 LOC (pure HTML shell + link/script tags), legacy.js ~6800 LOC, core/*.js totals ~250 LOC.

- [x] **Step 6.9: Commit**

```bash
git add internal/server/static/js/ internal/server/static/dashboard.html
git commit -m "$(cat <<'EOF'
refactor(frontend): extract html/utils/api/state core modules

Moves ~250 LOC of pure helpers (esc, authHeaders, shared app state,
date/tag utils) into js/core/*.js as ES modules with window.* legacy
bridges. Remaining 6800 LOC of dashboard code moves to js/legacy.js
loaded with defer — behaviour unchanged, but the HTML shell drops
from 8143 to ~150 lines.

Phase 1.2 of 2026-04-18-naozhi-ddd-refactor-design.md.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Extract core JS — ws.js

**Files:** Modify `js/legacy.js`; write `js/core/ws.js`.

- [ ] **Step 7.1: Identify the WS client surface**

Run: `grep -n "new WebSocket\|wsSend\|ws\.onmessage\|function ws" internal/server/static/js/legacy.js | head`

The WS code is the state machine around `const ws = new WebSocket(...)`, including `authWS`, `subscribe`, reconnection, `send_ack` + `history` handling. Approximate range: lines 1555-2200 in the pre-split file (grep to confirm in `legacy.js`).

- [ ] **Step 7.2: Cut the WS block into `js/core/ws.js`**

Export the client as a factory with injection points for the event callback:

```js
// js/core/ws.js — WebSocket hub client.
import { state } from './state.js';

let ws = null;
let reconnectDelay = 1000;
let eventHandler = null;
let statusHandler = null;

export function setEventHandler(fn) { eventHandler = fn; }
export function setStatusHandler(fn) { statusHandler = fn; }

export function connect() { /* paste original connect logic, replacing bare `ws` refs and bare `selectedKey` reads with `state.selectedKey` */ }
export function wsSend(msg) { /* paste */ }
export function subscribe(key, after) { /* paste */ }
export function interrupt(key) { /* paste */ }

// Legacy bridge
if (typeof window !== 'undefined') {
  window.wsConnect = connect;
  window.wsSend = wsSend;
  window.wsSubscribe = subscribe;
  window.wsInterrupt = interrupt;
  window.setWSEventHandler = setEventHandler;
  window.setWSStatusHandler = setStatusHandler;
}
```

- [ ] **Step 7.3: Delete the original WS block from `legacy.js`**

Remove the cut range. Add `import('/static/js/core/ws.js').then(m => { window.__ws = m; m.connect(); })` at the top of `legacy.js`, or preferably, move this `import` into the early module-loader block in `dashboard.html` (alongside html/utils/api/state) so it loads before `legacy.js` runs.

- [ ] **Step 7.4: Rewire event dispatch**

The old WS code calls functions like `renderEventToChat(ev)` directly. After this extraction, those functions still live in `legacy.js` and are on `window`. The module `ws.js` dispatches via a registered handler:

In `legacy.js` after its globals init, add:
```js
window.setWSEventHandler((ev) => {
  // existing event handler body, unchanged
});
```

- [ ] **Step 7.5: Build and smoke test**

Build, run local, open dashboard, confirm:
- WS status indicator in UI turns to `connected`
- Opening a session and sending a message gets a streamed reply (or a "not running" error — either is fine; what matters is **no** `ws is not defined` console error)
- DevTools Network → WS tab shows an active `/ws` connection

- [ ] **Step 7.6: Commit**

```bash
git add internal/server/static/js/
git commit -m "$(cat <<'EOF'
refactor(frontend): extract WebSocket client to js/core/ws.js

Move ~600 LOC of WS connect/reconnect/subscribe/send logic from
legacy.js into a module with event handler injection. legacy.js
registers its renderer callback via window.setWSEventHandler.

Phase 1.2 of 2026-04-18-naozhi-ddd-refactor-design.md.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Write the view router

**Files:** Write `js/core/router.js`; modify `js/app.js`; modify `dashboard.html`.

The router replaces the current `function switchView(view, el)` (line 5528). It keeps `window.switchView` so existing `onclick="switchView(...)"` attributes continue to resolve, but behind the scenes it dynamically `import()`s the view module.

- [x] **Step 8.1: Write `js/core/router.js`**

```js
// js/core/router.js — view registry + switchView + history API.
import { state } from './state.js';

const VIEWS = {
  // Populated lazily as view modules are extracted. Keys that aren't
  // here yet fall back to the legacy renderXxxView() on window.
  // After Tasks 9-13, all 7 views are here.
};

export function register(name, loader) { VIEWS[name] = loader; }

let current = { name: null, mod: null };

export async function switchView(name, el) {
  if (current.name === name) return;
  const loader = VIEWS[name];
  // Highlight the nav button
  document.querySelectorAll('#viewBar .vb-btn').forEach(b =>
    b.classList.toggle('active', b.dataset.view === name));
  state.currentView = name;

  // Teardown previous
  if (current.mod && typeof current.mod.unmount === 'function') {
    try { current.mod.unmount(); } catch (e) { console.warn('unmount', name, e); }
  }

  // Legacy fallback — route to the original switchView body for views
  // not yet migrated.
  if (!loader) {
    if (typeof window.__legacySwitchView === 'function') {
      window.__legacySwitchView(name, el);
    }
    current = { name, mod: null };
    return;
  }

  const mod = await loader();
  const slot = document.getElementById('main');
  if (typeof mod.mount === 'function') {
    await mod.mount(slot);
  }
  current = { name, mod };
}

if (typeof window !== 'undefined') {
  window.switchView = switchView;
  window.registerView = register;
}
```

- [x] **Step 8.2: Rewrite `js/app.js` as the bootstrap**

```js
// js/app.js — dashboard entrypoint.
import './core/html.js';
import './core/utils.js';
import './core/api.js';
import './core/state.js';
import * as ws from './core/ws.js';
import * as router from './core/router.js';

window.addEventListener('DOMContentLoaded', () => {
  ws.connect();
  // Initial view: chat (default). switchView will fall back to legacy
  // until the chat module is extracted in Task 10.
  router.switchView('chat');
});
```

- [x] **Step 8.3: In `legacy.js`, rename the existing `function switchView`**

Change `function switchView(view, el)` (line ~5528 of original) to `function __legacySwitchView(view, el)`, and expose it: `window.__legacySwitchView = __legacySwitchView;`. Do **not** re-export the name `switchView` from legacy — `router.js` owns that.

- [x] **Step 8.4: Update `dashboard.html` to load `app.js`**

Replace the four individual module imports from Task 6.6 with a single entry:

```html
<script type="module" src="/static/js/app.js"></script>
<script src="/static/js/legacy.js" defer></script>
```

The module and the defer script both run after parsing; the module's static imports execute first (by spec), so by the time `legacy.js` runs, `window.switchView` + `window.__naozhiCoreReady` are set.

- [x] **Step 8.5: Build and full smoke test**

```bash
go build ./... && CGO_ENABLED=0 go build -o bin/naozhi ./cmd/naozhi/
./bin/naozhi --config config.yaml &
```

In the browser at `/dashboard`:
1. All 7 nav buttons render
2. Click through **each** — Chat, Knowledge, Wiki, Patrols, Approvals, Graph; each one's view content loads (via `__legacySwitchView` fallback)
3. DevTools Console: **no errors**
4. DevTools Network: `app.js`, 4x core modules, `legacy.js` all 200

- [ ] **Step 8.6: Commit**

```bash
git add internal/server/static/js/ internal/server/static/dashboard.html
git commit -m "$(cat <<'EOF'
feat(frontend): add router + app.js bootstrap

router.js owns window.switchView, dispatches to registered view
modules (none yet) or falls back to window.__legacySwitchView. app.js
imports all core modules + ws.connect() + initial switchView('chat').

All 7 views still render via legacy fallback — view modules are
extracted one per commit in Tasks 9-13.

Phase 1.2 of 2026-04-18-naozhi-ddd-refactor-design.md.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## View migration template (Tasks 9-13 share this pattern)

For each view, the steps are:

1. **Identify source range** in `legacy.js` (grep for `renderXxxView` + `loadXxxList` + `renderXxxCards` etc.)
2. **Cut** that range into `js/views/<name>.js`
3. **Wrap** the cut content with:
   ```js
   import { state } from '../core/state.js';
   import { apiJSON, authHeaders } from '../core/api.js';
   import { esc, escAttr, escJs } from '../core/html.js';
   import { register } from '../core/router.js';

   export async function mount(slot) {
     render(slot);
     // any data loaders
   }
   export function unmount() { /* optional cleanup: clear timers, detach listeners */ }

   function render(slot) { /* paste renderXxxView body, replacing document.getElementById('main') with slot */ }
   // paste helpers: loadXxx, renderXxxCards, etc.

   register('<name>', () => import('/static/js/views/<name>.js'));
   // Self-register: ES modules execute once; importing the module
   // installs it in the router. The router then uses the cached module.
   ```
4. **Delete** the cut functions from `legacy.js` (leave helpers only if shared — if so, promote to `core/utils.js`)
5. **In `app.js`**, eagerly import the view so self-registration happens at bootstrap:
   ```js
   import('./views/<name>.js');
   ```
   (Task 15 replaces this with lazy-on-switch for views other than the initial one.)
6. **Build + smoke test**:
   - `go build ./... && CGO_ENABLED=0 go build -o bin/naozhi ./cmd/naozhi/`
   - Start locally, open `/dashboard`, click the `<name>` nav button
   - Verify the view renders identically, functionality works (list loads, filters, actions)
   - No console errors
   - `grep -n "renderXxxView\|loadXxx" internal/server/static/js/legacy.js` returns empty
7. **Commit** with message `refactor(frontend): migrate <name> view to js/views/<name>.js`

Each view task below fills in the specifics.

---

## Task 9: Migrate `home` view

**Files:** Modify `js/legacy.js`, `js/app.js`; write `js/views/home.js`.

- [x] **Step 9.1: Locate source**

Run: `grep -n "function renderHomeView\|function loadHomeStats\|function loadHomeActivity\|function loadHomeWiki\|function loadHomePatrolsAndApprovals\|function renderHomePatrolWidgetContent\|function renderHomeApprovalWidgetContent" internal/server/static/js/legacy.js`

Expected: 7 functions. Their total range is roughly contiguous, ~240 LOC.

- [x] **Step 9.2: Cut into `js/views/home.js`**

Create `js/views/home.js` per template. The `mount(slot)` body is the current `renderHomeView` body (with `document.getElementById('main')` replaced by `slot`). Keep the 4 `loadHome*` helpers as module-scoped functions (not exported). Keep `renderHomePatrolWidgetContent`/`renderHomeApprovalWidgetContent` as module-scoped (they were called from both Home and Patrols/Approvals — grep confirms; if they're also called from elsewhere, keep a copy via `window.renderHomePatrolWidgetContent = ...` compat).

- [x] **Step 9.3: Delete from `legacy.js`**

Remove the 7 cut functions.

- [x] **Step 9.4: Register in `app.js`**

```js
import('./views/home.js');
```

- [x] **Step 9.5: Smoke test**

Build, run locally, click **Home** (or load landing; Home renders when no session is selected). Checklist:
- Greeting + date + 5 stat cards + 4 action buttons render
- "Patrol Status" widget loads within 2s
- "Pending Approvals" widget loads within 2s
- "Recent Activity" feed populates
- "Recently Compiled" wiki list populates
- No console errors

`grep -n "renderHomeView" internal/server/static/js/legacy.js` → no output.

- [x] **Step 9.6: Commit**

```bash
git add internal/server/static/js/
git commit -m "$(cat <<'EOF'
refactor(frontend): migrate home view to js/views/home.js

~240 LOC cut from legacy.js: renderHomeView, loadHomeStats,
loadHomeActivity, loadHomeWiki, loadHomePatrolsAndApprovals, plus
the two widget renderers. Self-registers with router on import.

Phase 1.3 of 2026-04-18-naozhi-ddd-refactor-design.md.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Migrate `chat` view

**Files:** Modify `js/legacy.js`, `js/app.js`; write `js/views/chat.js`.

**Note:** Chat is the **largest** view — it owns `renderMainShell`, `renderSessionList`, `renderSidebar`, message rendering, tool-call cards, bookmark buttons, input handling, drag-drop, voice recording, and the context panel. Budget this task a full day; break into sub-steps freely.

- [x] **Step 10.1: Identify the chat boundary**

Grep:
```bash
grep -n "function renderMainShell\|function renderSessionList\|function renderSidebar\|function sessionCardHtml\|function resumeRecentSession\|function closeHistoryPopover\|function toggleHistory\|function renderEventToChat\|function renderToolCall\|function appendUserMessage\|function scrollChatToBottom" internal/server/static/js/legacy.js
```

Expected: ~20+ functions. Record the set.

- [x] **Step 10.2: Identify shared-by-chat helpers that other views call**

Run: `grep -n "renderMainShell\|renderSessionList\|renderSidebar" internal/server/static/js/legacy.js`

Anywhere these are called from **outside** the chat function cluster (e.g. from `switchView` itself, from globals init), keep a thin wrapper on `window` that re-imports the module. For example in `legacy.js`:

```js
window.renderMainShell = (...args) => import('/static/js/views/chat.js').then(m => m.renderMainShell(...args));
```

- [x] **Step 10.3: Cut into `js/views/chat.js`**

Export both `mount` / `unmount` and named helpers (`renderMainShell`, `renderSidebar`, etc.) so the `window.*` bridges in Step 10.2 resolve. `mount(slot)`:

```js
export async function mount(slot) {
  const tagFilter = document.getElementById('tagFilter');
  const sessionList = document.getElementById('session-list');
  if (tagFilter) tagFilter.style.display = '';
  if (sessionList) sessionList.style.display = '';
  if (state.selectedKey) renderMainShell();
  else {
    // Home view is the landing when no session selected
    const home = await import('./home.js');
    await home.mount(slot);
  }
}
```

`unmount()` clears any chat-specific timers. Leave it minimal; the aggressive cleanup can come later.

- [x] **Step 10.4: Delete from `legacy.js`**

Remove all the chat functions cut above.

- [x] **Step 10.5: Eager-import in `app.js`**

```js
import('./views/chat.js');
```

- [x] **Step 10.6: Smoke test — long checklist**

Build, run, open dashboard, then:
- Session list in sidebar loads (≥ 1 session visible if server has any)
- Click a session → main area renders chat shell with messages
- Type a message + Enter → streams a reply (or error if CLI not installed — that's fine; no JS error)
- Upload a file via paperclip → preview shows, send → attached
- Bookmark button on a message works
- Tool-call cards expand/collapse
- Code blocks highlight via Shiki
- Sidebar tag filter pills: All / Today / Yesterday / Pinned — click each, list filters
- History popover (clock icon): opens, lists older sessions
- `/new` command text message creates a new session ID
- Switch to Home → Chat → still works, state preserved

If any step fails, stop and diff the cut range against the module — you likely dropped a function.

- [x] **Step 10.7: Commit**

```bash
git add internal/server/static/js/
git commit -m "$(cat <<'EOF'
refactor(frontend): migrate chat view to js/views/chat.js

Largest single migration — ~3500 LOC covering session list, sidebar,
main chat shell, message rendering, tool cards, bookmark, input,
drag-drop, voice recording, context panel. Exports named helpers
plus mount/unmount. legacy.js drops from ~6500 to ~3000 LOC.

Phase 1.3 of 2026-04-18-naozhi-ddd-refactor-design.md.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Migrate `knowledge` view

**Files:** Modify `js/legacy.js`, `js/app.js`; write `js/views/knowledge.js`.

- [x] **Step 11.1: Locate source**

Run: `grep -n "function renderKnowledgeView\|function loadVaultTree\|function renderVaultTree\|function openVaultFile\|function knowledgeSearch\|function knowledgeAIChat" internal/server/static/js/legacy.js`

Range is approximately what was lines 6264-6668 in the pre-split HTML. In `legacy.js` the lines will differ — grep is the source of truth.

- [x] **Step 11.2: Cut + wrap + self-register**

Follow the view migration template.

- [x] **Step 11.3: Smoke test**

- Click **Knowledge** tab
- Vault tree renders on the left, content panel on the right
- Click a folder → expands
- Click a `.md` file → content renders with callouts + tasks + wikilinks
- Search box → type → results update
- AI Chat panel opens if clicked, accepts input

- [x] **Step 11.4: Commit**

```bash
git add internal/server/static/js/
git commit -m "refactor(frontend): migrate knowledge view to js/views/knowledge.js

Phase 1.3 of 2026-04-18-naozhi-ddd-refactor-design.md.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 12: Migrate `wiki` view

**Files:** Modify `js/legacy.js`, `js/app.js`; write `js/views/wiki.js`.

- [x] **Step 12.1: Locate source**

Run: `grep -n "function renderWikiView\|function loadWikiPages\|function loadWikiPage\|function renderWikiSourcesPanel\|function loadDecisions\|function saveDecisionFromForm" internal/server/static/js/legacy.js`

Wiki includes the Decision Journal (ADR) UI and Sources Panel — all part of this module.

- [x] **Step 12.2: Cut + wrap + self-register**

Follow template.

- [x] **Step 12.3: Smoke test**

- Click **Wiki** tab
- Page list on left, content on right
- Click a wiki page → renders compiled content + sources panel
- Decision Journal: view existing decisions, create new decision (fill title/context/decision/consequences → save → appears in list)
- Ingest button triggers `POST /api/wiki/ingest` (don't actually run in smoke test — just click once and confirm the spinner fires without error; cancel the operation if it's slow)

- [x] **Step 12.4: Commit**

```bash
git add internal/server/static/js/
git commit -m "refactor(frontend): migrate wiki + decision journal to js/views/wiki.js

Phase 1.3 of 2026-04-18-naozhi-ddd-refactor-design.md.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 13: Migrate `patrols`, `approvals`, `graph` views

**Files:** Modify `js/legacy.js`, `js/app.js`; write `js/views/patrols.js`, `js/views/approvals.js`, `js/views/graph.js`.

Do each as a separate commit (three commits total in this task).

- [x] **Step 13.1: Patrols — cut + wrap + register**

Grep: `function renderPatrolsView\|function renderPatrolCards\|function togglePatrolRule\|function editPatrolRule`. Template.

Smoke test: Patrols tab → rule cards render → toggle pause/resume → edit rule → save. No errors.

Commit: `refactor(frontend): migrate patrols view to js/views/patrols.js`.

- [ ] **Step 13.2: Approvals — cut + wrap + register**

Grep: `function renderApprovalsView\|function renderApprovalCards\|function approveItem\|function rejectItem\|function dismissApproval`. Template.

Smoke test: Approvals tab → queue renders → click Approve on an item (if queue non-empty) → toast + item removed. Or if queue empty → "No pending approvals" state.

Commit: `refactor(frontend): migrate approvals view to js/views/approvals.js`.

- [ ] **Step 13.3: Graph — cut + wrap + register + move d3 import inside**

Graph is special: it lazy-loads `d3` from CDN. Move the `import('https://cdn.jsdelivr.net/npm/d3@...')` call **inside** `js/views/graph.js` so d3 is only downloaded when the user clicks Graph.

Grep: `function renderGraphView\|function loadGraphData\|function drawGraph\|function filterGraphType\|GRAPH_COLORS\|GRAPH_LABELS\|getOrLoadD3`.

Move inline styles (graph doesn't currently have a CSS block — it has `element.style.*` assignments) into the previously-empty `css/views/graph.css`. Convert inline `style="..."` attributes in the graph template strings to class names.

Smoke test: Graph tab → triggers d3 download (visible in Network tab) → SVG renders with nodes + edges → filter pills work → click a node → detail panel shows.

Commit: `refactor(frontend): migrate graph view to js/views/graph.js (d3 now lazy-loaded on demand)`.

- [ ] **Step 13.4: Verify all views are in modules**

Run:
```bash
grep -n "function render.*View\b" internal/server/static/js/legacy.js
```
Expected: no output (all 7 `render<Name>View` functions are gone).

- [ ] **Step 13.5: Verify legacy.js is now small**

```bash
wc -l internal/server/static/js/legacy.js
```
Expected: < 1500 lines (only: global window.onerror handler, session helpers that weren't chat-specific, cron tab, files tab, discovered tab, cmd-K search overlay, notification center, new-session modal, auth modal, mobile tabs). These remain "plumbing" that's shared across views and will be broken up further in Phase 4.

---

## Task 14: Content-hash build pipeline

**Files:** Write `tools/hashstatic/main.go`; write `Makefile`; modify `.gitignore`; modify `internal/server/dashboard.go`.

This is the Risk-4 mitigation: content-hashed static files with `Cache-Control: immutable`, served via a `manifest.json` consumed by the HTML template.

- [ ] **Step 14.1: Write `tools/hashstatic/main.go`**

```go
// tools/hashstatic: reads internal/server/static/{css,js,sw.js,manifest.json}
// and writes hashed copies to internal/server/static/dist/ + manifest.json.
// Usage: go run ./tools/hashstatic
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	srcRoot  = "internal/server/static"
	distRoot = "internal/server/static/dist"
)

func main() {
	if err := os.RemoveAll(distRoot); err != nil {
		fatal(err)
	}
	manifest := map[string]string{}

	err := filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip the dist dir itself
			if path == distRoot {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(srcRoot, path)
		// Only hash CSS/JS/service-worker. HTML stays at a fixed path,
		// manifest.json stays at a fixed path.
		ext := strings.ToLower(filepath.Ext(rel))
		if ext != ".css" && ext != ".js" {
			return nil
		}
		// Skip anything already in dist/
		if strings.HasPrefix(rel, "dist/") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		h := sha256.Sum256(data)
		hash := hex.EncodeToString(h[:4]) // 8 hex chars
		// dist/js/core/router.<hash>.js
		base := strings.TrimSuffix(rel, ext)
		hashed := fmt.Sprintf("%s.%s%s", base, hash, ext)
		target := filepath.Join(distRoot, hashed)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
		manifest[filepath.ToSlash(rel)] = filepath.ToSlash(hashed)
		return nil
	})
	if err != nil {
		fatal(err)
	}

	manPath := filepath.Join(distRoot, "manifest.json")
	if err := os.MkdirAll(distRoot, 0o755); err != nil {
		fatal(err)
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(manPath, data, 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("hashstatic: wrote %d files + manifest.json\n", len(manifest))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
```

- [ ] **Step 14.2: Add `.gitignore`**

Append to `.gitignore` (create if absent):
```
internal/server/static/dist/
```

- [ ] **Step 14.3: Write `Makefile`**

If a Makefile doesn't exist, create at repo root:

```makefile
.PHONY: static build test deploy

static:
	go run ./tools/hashstatic

build: static
	CGO_ENABLED=0 go build -o bin/naozhi ./cmd/naozhi/

test: static
	go test ./...

deploy: build
	# existing deploy steps here
```

If a Makefile exists, add the `static` target and make `build` depend on it.

- [ ] **Step 14.4: Run the tool, verify output**

```bash
cd /root/keith-space/AWS/EC2-Workload/naozhi
go run ./tools/hashstatic
ls internal/server/static/dist/
cat internal/server/static/dist/manifest.json | head -20
```
Expected: `manifest.json` with entries like `"js/app.js": "js/app.abc12345.js"`; hashed files present.

- [ ] **Step 14.5: Update `dashboard.go` to read manifest + serve from `dist/`**

In `internal/server/dashboard.go`:

```go
import "html/template"

//go:embed all:static
var staticFS embed.FS

var (
	dashboardTmpl *template.Template
	staticManifest map[string]string // "js/app.js" -> "js/app.abc12345.js"
)

func init() {
	// Parse dashboard.html as a template
	tmplBytes, err := staticFS.ReadFile("static/dashboard.html")
	if err != nil { return }
	dashboardTmpl = template.Must(template.New("dashboard").Parse(string(tmplBytes)))

	// Load manifest if present (hashstatic run). Fallback: empty map → templates
	// serve unhashed paths via the /static/ route.
	if data, err := staticFS.ReadFile("static/dist/manifest.json"); err == nil {
		_ = json.Unmarshal(data, &staticManifest)
	}
	// ETag from manifest hash (so any module change busts the shell cache)
	h := sha256.Sum256(tmplBytes)
	for _, v := range staticManifest {
		h2 := sha256.Sum256([]byte(v))
		for i := range h { h[i] ^= h2[i] }
	}
	dashboardETag = `"` + hex.EncodeToString(h[:8]) + `"`
}

// asset returns the hashed path for a logical name, or the unhashed
// path if manifest is empty (dev mode).
func asset(name string) string {
	if h, ok := staticManifest[name]; ok {
		return "/static/dist/" + h
	}
	return "/static/" + name
}
```

In `dashboard.html`, replace the `<link>` and `<script>` tags with template calls:

```html
<link rel="stylesheet" href="{{ asset "css/base.css" }}">
<link rel="stylesheet" href="{{ asset "css/components.css" }}">
<link rel="stylesheet" href="{{ asset "css/views/home.css" }}">
<link rel="stylesheet" href="{{ asset "css/views/chat.css" }}">
<link rel="stylesheet" href="{{ asset "css/views/knowledge.css" }}">
<link rel="stylesheet" href="{{ asset "css/views/wiki.css" }}">
<link rel="stylesheet" href="{{ asset "css/views/patrols.css" }}">
<link rel="stylesheet" href="{{ asset "css/views/approvals.css" }}">
<link rel="stylesheet" href="{{ asset "css/views/graph.css" }}">
...
<script type="module" src="{{ asset "js/app.js" }}"></script>
<script src="{{ asset "js/legacy.js" }}" defer></script>
```

Register the `asset` func on the template:

```go
dashboardTmpl = template.Must(
  template.New("dashboard").
    Funcs(template.FuncMap{"asset": asset}).
    Parse(string(tmplBytes)))
```

Update `handleDashboard` to `dashboardTmpl.Execute(gzipOrPlainWriter, nil)` instead of writing raw bytes.

- [ ] **Step 14.6: Update `handleStatic` to serve `/static/dist/*` with immutable cache**

```go
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/static/")
	if path == "" || strings.Contains(path, "..") {
		http.NotFound(w, r)
		return
	}
	data, err := staticFS.ReadFile("static/" + path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// Hashed files under dist/ are immutable
	if strings.HasPrefix(path, "dist/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	// ...content-type + gzip as before
}
```

- [ ] **Step 14.7: Internal JS dynamic imports need hashed URLs too**

The router's `register('<name>', () => import('/static/js/views/<name>.js'))` imports an **unhashed** URL. To pick up the hashed dist path we inject a manifest into the page:

In `dashboard.html` add, right before `app.js` loads:

```html
<script>window.__MANIFEST = {{ manifestJSON }};</script>
```

Template func:
```go
"manifestJSON": func() template.JS {
  b, _ := json.Marshal(staticManifest)
  return template.JS(b)
},
```

In `js/core/router.js` + `app.js` + self-registering view modules, change:
```js
register('home', () => import('/static/js/views/home.js'));
```
to:
```js
register('home', () => import(window.__resolve('js/views/home.js')));
```

In `app.js` add:
```js
window.__resolve = (logical) => {
  const m = window.__MANIFEST || {};
  return m[logical] ? '/static/dist/' + m[logical] : '/static/' + logical;
};
```

- [ ] **Step 14.8: Build end-to-end**

```bash
make static
go build ./... && go vet ./...
CGO_ENABLED=0 go build -o bin/naozhi ./cmd/naozhi/
./bin/naozhi --config config.yaml &
sleep 2
curl -sI http://localhost:8080/static/dist/js/app.*.js | head -5
# In browser: check Network tab, confirm:
#   - app.<hash>.js loaded with Cache-Control: immutable
#   - dynamic import() of views/<name>.js resolves to hashed URL
```

- [ ] **Step 14.9: Update `sw.js` to version its cache**

In `internal/server/static/sw.js`, change the cache name to include a build-time version. Read the manifest from `/static/dist/manifest.json` at install time and use its hash as the cache version so old bundles are evicted.

Minimum viable change (full rewrite deferred to Phase 4): bump a `CACHE_VERSION = 'phase1-v1'` constant so any pre-Phase-1 cache is ignored.

- [ ] **Step 14.10: Commit**

```bash
git add tools/hashstatic/ Makefile .gitignore internal/server/dashboard.go \
        internal/server/static/dashboard.html internal/server/static/js/core/router.js \
        internal/server/static/js/app.js internal/server/static/sw.js
git commit -m "$(cat <<'EOF'
feat(frontend): content-hash pipeline for css/js + immutable cache

tools/hashstatic writes hashed copies of every css/js file into
internal/server/static/dist/ + a manifest.json. dashboard.html becomes
a html/template that reads the manifest and renders immutable URLs.
Dynamic import() resolves via window.__MANIFEST injected into the shell.

make static produces the dist/; make build depends on it. dist/ is
gitignored (rebuilt per deploy). Unhashed /static/ routes remain
functional as a dev fallback when the manifest is empty.

Closes Risk 4 of 2026-04-18-naozhi-ddd-refactor-design.md.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 15: Lazy-load non-first-paint views

**Files:** Modify `js/app.js`; verify all view modules.

Currently `app.js` eagerly imports all 7 view modules to install them with the router. This defeats the point of lazy loading. Fix:

- [ ] **Step 15.1: Replace eager imports with router-registered loaders**

In `app.js`:

```js
import './core/html.js';
import './core/utils.js';
import './core/api.js';
import './core/state.js';
import * as ws from './core/ws.js';
import * as router from './core/router.js';

window.__resolve = (logical) => {
  const m = window.__MANIFEST || {};
  return m[logical] ? '/static/dist/' + m[logical] : '/static/' + logical;
};

// Register views WITHOUT importing them. The loader closure does
// the dynamic import on first switchView.
const VIEWS = ['home', 'chat', 'knowledge', 'wiki', 'patrols', 'approvals', 'graph'];
for (const name of VIEWS) {
  router.register(name, () => import(/* @vite-ignore */ window.__resolve(`js/views/${name}.js`)));
}

// First paint: eagerly load chat because it's the default view.
// Home is a sub-import from chat when no session is selected.
window.addEventListener('DOMContentLoaded', () => {
  ws.connect();
  router.switchView('chat');
});
```

- [ ] **Step 15.2: Remove self-registering `register('<name>', ...)` calls from view modules**

Each view module should no longer register itself — the registration lives in `app.js`. Remove the `register(...)` line from the bottom of `js/views/*.js`. This avoids the chicken-and-egg of the module needing to register itself before it runs.

- [ ] **Step 15.3: Lazy-load per-view CSS**

In `js/core/router.js`, inside `switchView`, before `mod.mount(slot)`:

```js
async function ensureViewCSS(name) {
  if (document.querySelector(`link[data-view-css="${name}"]`)) return;
  const link = document.createElement('link');
  link.rel = 'stylesheet';
  link.href = window.__resolve(`css/views/${name}.css`);
  link.dataset.viewCss = name;
  document.head.appendChild(link);
  // Wait for it to load so FOUC is minimised
  await new Promise((res) => { link.onload = res; link.onerror = res; });
}

export async function switchView(name, el) {
  // ...existing teardown...
  await ensureViewCSS(name);
  const mod = await loader();
  // ...
}
```

Remove the 7 per-view `<link>` tags from `dashboard.html` — keep only `base.css` + `components.css` as eager links. First-paint CSS drops to ~5 KB gzip.

- [ ] **Step 15.4: Build and verify lazy-load**

```bash
make build && ./bin/naozhi --config config.yaml &
```

In browser:
1. DevTools Network tab, clear
2. Hard reload `/dashboard`
3. Observe: `app.js` loads, `chat.js` loads, `chat.css` loads. No `graph.js`, `wiki.js`, `knowledge.js`, etc.
4. Click **Graph** → now see `graph.js` + `graph.css` + d3 arrive
5. Measure: right-click `app.js` → "Copy as cURL" → pipe through `gzip -c | wc -c`. Target: total first-paint JS (app + chat + core + legacy) < 50 KB gzip. Record the number.

- [ ] **Step 15.5: Commit**

```bash
git add internal/server/static/js/ internal/server/static/dashboard.html
git commit -m "$(cat <<'EOF'
feat(frontend): lazy-load per-view js + css via router

Views + per-view CSS are now dynamically imported on first switchView.
First-paint ships only app.js + chat.js + core modules + legacy.js;
Graph view (d3) and Knowledge/Wiki (shiki+katex already deferred) load
on demand. Target first-paint JS < 50 KB gzip met — see TASK 15.4.

Phase 1.4 of 2026-04-18-naozhi-ddd-refactor-design.md.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
EOF
)"
```

---

## Task 16: Performance verification

**Files:** none modified.

- [ ] **Step 16.1: Measure first-paint JS + CSS gzip**

```bash
make build && ./bin/naozhi --config config.yaml &
sleep 2
TOTAL=0
for f in /static/dist/js/app.*.js /static/dist/js/legacy.*.js /static/dist/js/core/*.js /static/dist/js/views/chat.*.js; do
  SIZE=$(curl -s --compressed http://localhost:8080${f} -o /dev/null -w '%{size_download}\n')
  TOTAL=$((TOTAL + SIZE))
  echo "$f: $SIZE"
done
echo "TOTAL first-paint JS: $TOTAL bytes"
kill %1
```
**Target:** < 51200 bytes (50 KB). Record the number.

If over target: inspect `legacy.js` — the goal is to continue shrinking it in Phase 3. If first-paint is blocked by `legacy.js`, consider deferring its load behind `DOMContentLoaded` + splitting its remaining content.

- [ ] **Step 16.2: Measure view-switch latency**

In Chrome DevTools Console on the dashboard:

```js
// Paste once
window.__switchTimings = [];
const origSwitch = window.switchView;
window.switchView = async function(name, el) {
  const t0 = performance.now();
  const r = await origSwitch(name, el);
  const dt = performance.now() - t0;
  window.__switchTimings.push({ name, ms: dt.toFixed(1) });
  console.log(`switch ${name}: ${dt.toFixed(1)}ms`);
  return r;
};
```

Then click each nav button **twice** (first click triggers the module download, second is the cached switch). Record the second-click time for each.

**Target:** second-switch < 50ms for every view. First switch (import time) is expected 100-300ms on 4G.

- [ ] **Step 16.3: Measure LCP (local)**

In DevTools Performance panel → "Reload and Record". Look at the LCP marker in the timeline.

**Target:** < 1.5s on local WiFi.

- [ ] **Step 16.4: Measure LCP (throttled to "Slow 4G")**

Same procedure, with Network throttling set to "Slow 4G". **Target:** < 2.5s.

- [ ] **Step 16.5: iOS Safari verification (Risk 3)**

Open the production dashboard URL on the iPhone already used for the existing `<script type="module">` (Shiki). Verify:
- All 7 nav buttons work
- DevTools via Safari desktop → iPhone → Console shows no errors
- Each view renders

If any view fails on iOS: stop. Likely cause: a cut/paste into a view module dropped `async`/`await` handling, or used a pre-2020 syntax that isn't supported. Fix and re-test.

- [ ] **Step 16.6: Commit performance numbers into the plan**

Edit this plan's Acceptance section below to fill in the measured numbers, then commit:

```bash
git add docs/superpowers/plans/2026-04-19-naozhi-phase1-frontend-split.md
git commit -m "docs(plan): record Phase 1 performance measurements

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Task 17: Deploy to EC2 + production smoke test

**Files:** none — deploy + verify.

- [ ] **Step 17.1: Build production binary**

```bash
cd /root/keith-space/AWS/EC2-Workload/naozhi
make static
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/naozhi ./cmd/naozhi/
ls -la bin/naozhi
```
Expected: arm64 linux binary, ~25-40 MB.

- [ ] **Step 17.2: Upload + restart**

```bash
PEM=/root/keith-space/AWS/AWS-Keith-Network/keith-secret.pem
EC2=ubuntu@10.0.11.189
scp -i $PEM bin/naozhi $EC2:/tmp/naozhi
ssh -i $PEM $EC2 "sudo systemctl stop naozhi && \
  sudo mv /tmp/naozhi /usr/local/bin/naozhi && \
  sudo chmod +x /usr/local/bin/naozhi && \
  sudo systemctl start naozhi && \
  sleep 2 && sudo systemctl is-active naozhi"
```
Expected: `active`.

- [ ] **Step 17.3: Health + static asset checks**

```bash
ssh -i $PEM $EC2 "curl -s http://localhost:8180/health"
ssh -i $PEM $EC2 "curl -sI http://localhost:8180/static/dist/js/app.*.js 2>&1 | head -5 | grep -E 'HTTP|Cache-Control'"
```
Expected: health OK; `Cache-Control: public, max-age=31536000, immutable` on the hashed JS.

- [ ] **Step 17.4: Browser smoke — all views + PWA**

User loads `https://naozhi.keithyu.cloud/dashboard` from phone + desktop. Checklist (same as Task 15.4 plus production-specifics):
- All 7 nav buttons work
- DevTools → Application → Service Workers — sw.js registered, new cache version
- Refresh: first-paint waterfall shows only `base.css`, `components.css`, `app.js`, `legacy.js`, `chat.js`, `chat.css`, core modules — nothing else
- Open Graph tab → d3 loads (from CDN) + graph.js + graph.css
- Everything keeps working through CloudFront (check Network tab origin)

- [ ] **Step 17.5: CloudFront cache sanity**

```bash
curl -sI https://naozhi.keithyu.cloud/static/dist/js/app.ANY_HASH.js | grep -E 'cache|age|via'
```
Expected: on second request, `x-cache: Hit from cloudfront`. The first request is a MISS that fetches from origin; subsequent hits serve from CloudFront edge.

- [ ] **Step 17.6: Tag the commit**

```bash
git -C /root/keith-space/AWS/EC2-Workload/naozhi tag -a phase-1-frontend-split \
  -m "Phase 1 complete: dashboard.html split into ES modules + content-hash cache"
```

- [ ] **Step 17.7: Update deployment changelog**

Add to `/root/keith-space/AWS/EC2-Workload/naozhi-deployment.md`:

```markdown
### 2026-04-19 — Phase 1 前端拆分上线

- `dashboard.html`: 8143 → ~120 行（HTML shell + template）
- 新增 `css/` + `js/core/` + `js/views/` 模块树
- `tools/hashstatic` 生成 content-hash + `dist/manifest.json`
- `/static/dist/*` `Cache-Control: immutable`
- 首屏 JS gzip: 120KB → <measured> KB
- 视图切换 (non-first): ~200ms → <measured> ms
- 回滚分支: `naozhi-2.0-pre-split`
```

Commit in parent repo:

```bash
cd /root/keith-space
git add AWS/EC2-Workload/naozhi-deployment.md
git commit -m "docs: naozhi deployment — record Phase 1 frontend split

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>"
```

---

## Acceptance

Phase 1 is complete when **all** of the following hold:

**Structural:**
- [ ] `wc -l internal/server/static/dashboard.html` reports **< 500** lines
- [ ] `ls internal/server/static/js/views/` lists all 7 views
- [ ] `grep -n "function render.*View\b" internal/server/static/js/legacy.js` returns empty
- [ ] `go build ./... && go test ./... && go vet ./...` green

**Performance (Task 16):**
- [ ] First-paint JS gzip < **50 KB** (measured: ____ KB)
- [ ] First-paint CSS gzip < **15 KB** (measured: ____ KB)
- [ ] View switch (second click, cached) < **50 ms** for every view (measured: home ___, chat ___, knowledge ___, wiki ___, patrols ___, approvals ___, graph ___ ms)
- [ ] LCP local < **1.5 s** (measured: ____ s)
- [ ] LCP 4G < **2.5 s** (measured: ____ s)

**Production:**
- [ ] EC2 `systemctl is-active naozhi` → `active` (Task 17.2)
- [ ] EC2 `/static/dist/js/app.*.js` returns `Cache-Control: immutable` (Task 17.3)
- [ ] All 7 nav tabs work on desktop + iPhone (Task 17.4)
- [ ] Service Worker registered + old cache evicted (Task 17.4)

**Safety:**
- [ ] Rollback branch `naozhi-2.0-pre-split` exists (Task 1.3)
- [ ] Tag `phase-1-frontend-split` exists (Task 17.6)
- [ ] Deployment changelog updated (Task 17.7)

## Next

On sign-off, Phase 2 (backend DDD skeleton) writing-plans session begins. `internal/server/handlers/` is next on the chopping block.
