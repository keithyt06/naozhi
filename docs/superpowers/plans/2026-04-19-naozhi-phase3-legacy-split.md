# Naozhi Phase 3: legacy.js Feature Split Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring first-paint JS gzip under 50 KB by splitting `internal/server/static/js/legacy.js` (2728 lines, 27.9 KB gzip) into eight user-triggered feature modules loaded on demand.

**Architecture:** Move feature implementations out of `legacy.js` into `internal/server/static/js/features/*.js`. Keep the eager `legacy.js` holding shared helpers, mobile navigation, discovery auto-poll, notification base, and a set of `window.*` shims that lazy-`import()` the relevant feature module on first call. Hashed asset pipeline from Phase 1 Task 14 resolves the chunk URLs via `window.__resolveAsset`.

**Tech Stack:** Vanilla ES modules, dynamic `import()`, Go `html/template`, Go `embed`, content-hash asset pipeline (`tools/hashstatic`), gzip compression on `/static/`.

**Reference spec:** `docs/superpowers/specs/2026-04-19-naozhi-phase3-legacy-split-design.md`
**Phase 1 baseline:** `docs/superpowers/plans/2026-04-19-phase1-perf-baseline.md`

---

## Pre-work: orientation

Before touching code, the implementing agent must:

- [ ] Read the spec (`docs/superpowers/specs/2026-04-19-naozhi-phase3-legacy-split-design.md`) end-to-end.
- [ ] Read `internal/server/static/js/core/router.js` to understand `__resolveAsset`.
- [ ] Read `internal/server/dashboard.go:338-386` (`handleStatic`) to understand `/static/dist/*` fallback.
- [ ] Verify working tree is clean: `git status`. If dirty, stop and ask the user.
- [ ] Confirm you are on branch `naozhi-2.0`: `git branch --show-current`.

---

## Task 1: Scaffold the features directory and module contract

**Files:**
- Create: `internal/server/static/js/features/.gitkeep`
- Create: `internal/server/static/js/features/README.md`

- [ ] **Step 1: Create the features directory marker**

```bash
mkdir -p internal/server/static/js/features
: > internal/server/static/js/features/.gitkeep
```

- [ ] **Step 2: Create the module contract README**

Write `internal/server/static/js/features/README.md`:

```markdown
# features/

Lazy-loaded feature modules. Each file conforms to this contract:

```javascript
import { apiFetch } from '../core/api.js';
import { esc, html } from '../core/html.js';
// (state, utils as needed)

let initialized = false;
function ensureInit() {
  if (initialized) return;
  initialized = true;
  // Inject overlay DOM, bind module-scoped listeners.
}

export async function open(...args) {
  ensureInit();
  // ... body copied verbatim from the original legacy.js function.
}
```

Feature modules are imported by `legacy.js` via `window.*` shims:

```javascript
const FEAT = (name) => window.__resolveAsset('js/features/' + name + '.js');
window.openFileHub = async (...a) => (await import(FEAT('file-hub'))).open(...a);
```

Do NOT import feature modules from other feature modules. Cross-feature
calls go through the `window.*` shim so the bundle graph stays flat.
```

- [ ] **Step 3: Commit scaffold**

```bash
git add internal/server/static/js/features/
git commit -m "naozhi: scaffold js/features/ directory for Phase 3 lazy loading"
```

---

## Task 2: Extract context-panel into a lazy module

**Target:** Move `toggleCtxPanel`, `switchCtxTab`, `refreshCtxPanel`, `loadCtxSaved`, `loadCtxRelated`, `deleteBookmark` (approximately lines 1874-1976 in `legacy.js`) into `features/context-panel.js`.

**Files:**
- Create: `internal/server/static/js/features/context-panel.js`
- Modify: `internal/server/static/js/legacy.js` (remove functions, add shim)

- [ ] **Step 1: Locate the precise boundary**

```bash
grep -n '^function toggleCtxPanel\|^function switchCtxTab\|^function refreshCtxPanel\|^async function loadCtxSaved\|^async function loadCtxRelated\|^async function deleteBookmark\|^async function loadSessionBookmarks' internal/server/static/js/legacy.js
```

Note the start line of `toggleCtxPanel` and the line immediately before `loadSessionBookmarks`. That is the extract region.

- [ ] **Step 2: Create `features/context-panel.js`**

Skeleton (fill the body with the exact source lines from Step 1):

```javascript
// internal/server/static/js/features/context-panel.js
// Context Panel: session saved/related/bookmark sidebar.
import { apiFetch } from '../core/api.js';
import { esc, html } from '../core/html.js';

let initialized = false;
function ensureInit() {
  if (initialized) return;
  initialized = true;
}

// --- Begin extracted from legacy.js:1874-1976 ---
function toggleCtxPanel() { /* paste verbatim */ }
function switchCtxTab(tab, el) { /* paste verbatim */ }
async function refreshCtxPanel() { /* paste verbatim */ }
async function loadCtxSaved(body) { /* paste verbatim */ }
async function loadCtxRelated(body) { /* paste verbatim */ }
async function deleteBookmark(id) { /* paste verbatim */ }
// --- End extracted ---

export async function open(...args) {
  ensureInit();
  return toggleCtxPanel(...args);
}

// Internal-only helpers exposed for other feature modules that may need them:
export { refreshCtxPanel, deleteBookmark };
```

Copy the function bodies verbatim from `legacy.js` — do not rewrite or reformat. If a function uses a helper like `esc`, `html`, `apiFetch`, make sure it is imported.

- [ ] **Step 3: Delete extracted functions from `legacy.js`**

Remove the extracted region. Leave everything else untouched.

- [ ] **Step 4: Register the shim in `legacy.js`**

Add this to the shims section near the end of `legacy.js` (or create a new section if none exists):

```javascript
// Lazy feature module shims
const FEAT = (name) => window.__resolveAsset('js/features/' + name + '.js');

window.openContextPanel = async (...a) =>
  (await import(FEAT('context-panel'))).open(...a);
```

Note: `openContextPanel` is called from `internal/server/static/js/views/chat.js`. Search confirms no internal call in `legacy.js`.

- [ ] **Step 5: Rebuild static assets and the binary**

```bash
go run ./tools/hashstatic
go build ./...
go vet ./...
```

Expected: all commands exit 0. `internal/server/static/dist/manifest.json` contains an entry for `js/features/context-panel.js`.

- [ ] **Step 6: Run Go tests**

```bash
go test ./internal/server/...
```

Expected: PASS (no Go code changed, tests continue to pass).

- [ ] **Step 7: Smoke-test the hashed chunk**

```bash
./bin/naozhi --config test-config.yaml >/tmp/naozhi-smoke.log 2>&1 &
NAOZHI_PID=$!
sleep 2
HASH=$(jq -r '."js/features/context-panel.js"' internal/server/static/dist/manifest.json)
curl -sI "http://localhost:8180/static/dist/$HASH" | head -5
kill $NAOZHI_PID
```

Expected: `HTTP/1.1 200 OK`, `Cache-Control: public, max-age=31536000, immutable`.

(Use the existing test config. If none exists, reuse `config.yaml.example` or ask the user for the dev port.)

- [ ] **Step 8: Verify legacy.js shrank**

```bash
wc -l internal/server/static/js/legacy.js
grep -c '^function toggleCtxPanel' internal/server/static/js/legacy.js   # must print 0
grep -c '^window\.openContextPanel' internal/server/static/js/legacy.js  # must print 1
```

Record the new line count — it must be strictly less than 2728.

- [ ] **Step 9: Commit**

```bash
git add internal/server/static/js/features/context-panel.js internal/server/static/js/legacy.js internal/server/static/dist/manifest.json
git commit -m "naozhi: lazy-load context-panel feature (Phase 3/8)"
```

---

## Task 3: Extract notif-enhance into a lazy module

**Target:** Move `fetchNotifications`, `fetchNotifCount`, `onWsNotification` (approximately lines 1757-1872) into `features/notif-enhance.js`.

**Files:**
- Create: `internal/server/static/js/features/notif-enhance.js`
- Modify: `internal/server/static/js/legacy.js`

- [ ] **Step 1: Locate precise boundaries**

```bash
grep -n '^async function fetchNotifications\|^async function fetchNotifCount\|^function onWsNotification\|^function toggleCtxPanel' internal/server/static/js/legacy.js
```

The extract region starts at `fetchNotifications` and ends immediately before `toggleCtxPanel` (the next non-notif function).

- [ ] **Step 2: Create `features/notif-enhance.js`**

```javascript
// internal/server/static/js/features/notif-enhance.js
// Notification enhancement: push setup + WS incoming handler + fetch loop.
import { apiFetch } from '../core/api.js';
import { esc, html } from '../core/html.js';

let initialized = false;
function ensureInit() {
  if (initialized) return;
  initialized = true;
}

// --- Begin extracted from legacy.js:1757-1872 ---
async function fetchNotifications() { /* paste verbatim */ }
async function fetchNotifCount() { /* paste verbatim */ }
function onWsNotification(msg) { /* paste verbatim */ }
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
```

- [ ] **Step 3: Delete extracted region from `legacy.js`**

- [ ] **Step 4: Register shims**

```javascript
window.setupPushNotifications = async (...a) =>
  (await import(FEAT('notif-enhance'))).setup(...a);
window.handleIncomingNotif = async (...a) =>
  (await import(FEAT('notif-enhance'))).handle(...a);
```

- [ ] **Step 5: Check internal callers**

```bash
grep -n 'fetchNotifications\|fetchNotifCount\|onWsNotification' internal/server/static/js/legacy.js
```

Any internal caller inside `legacy.js` that survives extraction must be converted to `window.setupPushNotifications()` / `window.handleIncomingNotif()`. Record every such rewrite.

- [ ] **Step 6: Rebuild + vet + test**

```bash
go run ./tools/hashstatic
go build ./... && go vet ./... && go test ./internal/server/...
```

- [ ] **Step 7: Protocol smoke**

```bash
./bin/naozhi --config test-config.yaml >/tmp/naozhi-smoke.log 2>&1 &
sleep 2
HASH=$(jq -r '."js/features/notif-enhance.js"' internal/server/static/dist/manifest.json)
curl -sI "http://localhost:8180/static/dist/$HASH" | head -5
kill %1
```

Expected: 200 + immutable cache.

- [ ] **Step 8: Verify structural invariants**

```bash
wc -l internal/server/static/js/legacy.js
grep -c '^async function fetchNotifications' internal/server/static/js/legacy.js      # 0
grep -c '^window\.setupPushNotifications' internal/server/static/js/legacy.js         # 1
grep -c '^window\.handleIncomingNotif' internal/server/static/js/legacy.js            # 1
```

- [ ] **Step 9: Commit**

```bash
git add internal/server/static/js/features/notif-enhance.js internal/server/static/js/legacy.js internal/server/static/dist/manifest.json
git commit -m "naozhi: lazy-load notif-enhance feature (Phase 3/8)"
```

---

## Task 4: Extract bookmark into a lazy module

**Target:** Move `loadSessionBookmarks`, `injectBookmarkButtons`, `showBookmarkPopover` (approximately lines 1981-2085) into `features/bookmark.js`.

**Files:**
- Create: `internal/server/static/js/features/bookmark.js`
- Modify: `internal/server/static/js/legacy.js`

- [ ] **Step 1: Locate boundaries**

```bash
grep -n '^async function loadSessionBookmarks\|^function injectBookmarkButtons\|^function showBookmarkPopover\|^function navigateSearchResult' internal/server/static/js/legacy.js
```

Region: from `loadSessionBookmarks` to the line before `navigateSearchResult`.

- [ ] **Step 2: Create `features/bookmark.js`**

```javascript
// internal/server/static/js/features/bookmark.js
// Per-session bookmark list + popover + button injection.
import { apiFetch } from '../core/api.js';
import { esc, html } from '../core/html.js';

let initialized = false;
function ensureInit() {
  if (initialized) return;
  initialized = true;
}

// --- Begin extracted from legacy.js:1981-2085 ---
async function loadSessionBookmarks() { /* paste verbatim */ }
function injectBookmarkButtons() { /* paste verbatim */ }
function showBookmarkPopover(anchor, text, eventIdx) { /* paste verbatim */ }
// --- End extracted ---

export async function load(...a) { ensureInit(); return loadSessionBookmarks(...a); }
export async function inject(...a) { ensureInit(); return injectBookmarkButtons(...a); }
export async function showPopover(...a) { ensureInit(); return showBookmarkPopover(...a); }
```

- [ ] **Step 3: Delete extracted region from `legacy.js`**

- [ ] **Step 4: Register shims**

```javascript
window.loadSessionBookmarks = async (...a) =>
  (await import(FEAT('bookmark'))).load(...a);
window.injectBookmarkButtons = async (...a) =>
  (await import(FEAT('bookmark'))).inject(...a);
window.showBookmarkPopover = async (...a) =>
  (await import(FEAT('bookmark'))).showPopover(...a);
```

- [ ] **Step 5: Rewrite internal `showBookmarkPopover` call**

Inside the extracted bookmark block, `showBookmarkPopover` is called from `injectBookmarkButtons`. Because both now live in `features/bookmark.js`, that call stays local (no rewrite needed). But any call inside `legacy.js` that survives extraction must go through `window.showBookmarkPopover`. Grep confirm:

```bash
grep -n 'showBookmarkPopover\|injectBookmarkButtons\|loadSessionBookmarks' internal/server/static/js/legacy.js
```

Rewrite any surviving calls to the `window.*` form.

- [ ] **Step 6: Rebuild + vet + test + smoke**

```bash
go run ./tools/hashstatic
go build ./... && go vet ./... && go test ./internal/server/...
./bin/naozhi --config test-config.yaml >/tmp/naozhi-smoke.log 2>&1 &
sleep 2
HASH=$(jq -r '."js/features/bookmark.js"' internal/server/static/dist/manifest.json)
curl -sI "http://localhost:8180/static/dist/$HASH" | head -5
kill %1
```

- [ ] **Step 7: Verify invariants**

```bash
wc -l internal/server/static/js/legacy.js
grep -c '^async function loadSessionBookmarks' internal/server/static/js/legacy.js    # 0
grep -c '^window\.loadSessionBookmarks' internal/server/static/js/legacy.js           # 1
```

- [ ] **Step 8: Commit**

```bash
git add internal/server/static/js/features/bookmark.js internal/server/static/js/legacy.js internal/server/static/dist/manifest.json
git commit -m "naozhi: lazy-load bookmark feature (Phase 3/8)"
```

---

## Task 5: Extract twin into a lazy module

**Target:** Move `openTwinPanel`, `showTwinOverlay`, `closeTwinOverlay`, `updateTwinField`, `testTwinQuery` (approximately lines 2543-2672) into `features/twin.js`.

**Files:**
- Create: `internal/server/static/js/features/twin.js`
- Modify: `internal/server/static/js/legacy.js`

- [ ] **Step 1: Locate boundaries**

```bash
grep -n '^function openTwinPanel\|^function showTwinOverlay\|^function closeTwinOverlay\|^function updateTwinField\|^function testTwinQuery\|^function keyboardShortcuts\|^document.addEventListener' internal/server/static/js/legacy.js
```

Identify the last line of `testTwinQuery` (or the function that closes out the twin block) — the extract ends there, before the keyboard-shortcut dispatcher.

- [ ] **Step 2: Create `features/twin.js`**

```javascript
// internal/server/static/js/features/twin.js
// CTO Digital Twin overlay + config editor + test query.
import { apiFetch } from '../core/api.js';
import { esc, html } from '../core/html.js';

let initialized = false;
function ensureInit() {
  if (initialized) return;
  initialized = true;
}

// --- Begin extracted from legacy.js:2543-2672 ---
function openTwinPanel() { /* paste verbatim */ }
function showTwinOverlay(cfg) { /* paste verbatim */ }
function closeTwinOverlay() { /* paste verbatim */ }
function updateTwinField(field, value) { /* paste verbatim */ }
async function testTwinQuery() { /* paste verbatim */ }
// --- End extracted ---

export async function open(...a) { ensureInit(); return openTwinPanel(...a); }
```

- [ ] **Step 3: Delete extracted region from `legacy.js`**

- [ ] **Step 4: Register shim**

```javascript
window.openTwinPanel = async (...a) =>
  (await import(FEAT('twin'))).open(...a);
```

- [ ] **Step 5: Check inline onclick handlers inside the overlay HTML**

Inside `showTwinOverlay`, the template may use `onclick="closeTwinOverlay()"` etc. These are resolved at click time against `window`. After extraction, `window.closeTwinOverlay` does not exist unless we expose it. Two options — pick exactly one:

a) Add additional shims for each inline handler referenced inside the overlay HTML, OR

b) Rewrite the overlay HTML to use a module-scoped handler (e.g., add `data-action` attributes and a single delegated listener bound in `ensureInit`).

Prefer (a) for minimum behavior change. Add:

```javascript
window.closeTwinOverlay = async (...a) =>
  (await import(FEAT('twin'))).closeOverlay(...a);
window.updateTwinField = async (...a) =>
  (await import(FEAT('twin'))).updateField(...a);
window.testTwinQuery = async (...a) =>
  (await import(FEAT('twin'))).testQuery(...a);
```

And extend the twin module exports accordingly:

```javascript
export async function closeOverlay(...a) { ensureInit(); return closeTwinOverlay(...a); }
export async function updateField(...a) { ensureInit(); return updateTwinField(...a); }
export async function testQuery(...a) { ensureInit(); return testTwinQuery(...a); }
```

If you chose (b) instead, document the change in the commit message.

- [ ] **Step 6: Rebuild + vet + test + smoke**

```bash
go run ./tools/hashstatic
go build ./... && go vet ./... && go test ./internal/server/...
./bin/naozhi --config test-config.yaml >/tmp/naozhi-smoke.log 2>&1 &
sleep 2
HASH=$(jq -r '."js/features/twin.js"' internal/server/static/dist/manifest.json)
curl -sI "http://localhost:8180/static/dist/$HASH" | head -5
kill %1
```

- [ ] **Step 7: Verify invariants**

```bash
wc -l internal/server/static/js/legacy.js
grep -c '^function openTwinPanel' internal/server/static/js/legacy.js    # 0
grep -c '^window\.openTwinPanel' internal/server/static/js/legacy.js     # 1
```

- [ ] **Step 8: Commit**

```bash
git add internal/server/static/js/features/twin.js internal/server/static/js/legacy.js internal/server/static/dist/manifest.json
git commit -m "naozhi: lazy-load twin feature (Phase 3/8)"
```

---

## Task 6: Extract replay into a lazy module

**Target:** Move `openReplayViewer`, `showReplayOverlay`, `closeReplayOverlay`, `renderReplayUpTo`, `updateReplayTime`, `toggleReplayPlay`, `stopReplayTimer`, `advanceReplay`, `scrubReplay`, `setReplaySpeed`, `shareReplaySession` (approximately lines 2379-2541) into `features/replay.js`.

**Files:**
- Create: `internal/server/static/js/features/replay.js`
- Modify: `internal/server/static/js/legacy.js`

- [ ] **Step 1: Locate boundaries**

```bash
grep -n '^function openReplayViewer\|^function shareReplaySession\|^function openTwinPanel' internal/server/static/js/legacy.js
```

Region: from `openReplayViewer` to the line before `openTwinPanel` (note: by the time you run Task 6, twin may or may not already be extracted. If Task 5 ran first, the following function after the replay block is now the keyboard dispatcher — adjust the end marker accordingly).

- [ ] **Step 2: Create `features/replay.js`**

```javascript
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
function openReplayViewer(sessionKey) { /* paste verbatim */ }
function showReplayOverlay() { /* paste verbatim */ }
function closeReplayOverlay() { /* paste verbatim */ }
function renderReplayUpTo(idx) { /* paste verbatim */ }
function updateReplayTime(idx) { /* paste verbatim */ }
function toggleReplayPlay() { /* paste verbatim */ }
function stopReplayTimer() { /* paste verbatim */ }
function advanceReplay() { /* paste verbatim */ }
function scrubReplay(val) { /* paste verbatim */ }
function setReplaySpeed(s) { /* paste verbatim */ }
function shareReplaySession(sessionKey) { /* paste verbatim */ }
// --- End extracted ---

export async function open(...a) { ensureInit(); return openReplayViewer(...a); }
export async function share(...a) { ensureInit(); return shareReplaySession(...a); }
// Overlay inline handlers need window-level shims; re-exported for legacy.js:
export async function closeOverlay(...a) { ensureInit(); return closeReplayOverlay(...a); }
export async function togglePlay(...a) { ensureInit(); return toggleReplayPlay(...a); }
export async function scrub(...a) { ensureInit(); return scrubReplay(...a); }
export async function speed(...a) { ensureInit(); return setReplaySpeed(...a); }
```

- [ ] **Step 3: Delete extracted region from `legacy.js`**

- [ ] **Step 4: Register shims**

```javascript
window.openReplayViewer = async (...a) =>
  (await import(FEAT('replay'))).open(...a);
window.shareReplaySession = async (...a) =>
  (await import(FEAT('replay'))).share(...a);
window.closeReplayOverlay = async (...a) =>
  (await import(FEAT('replay'))).closeOverlay(...a);
window.toggleReplayPlay = async (...a) =>
  (await import(FEAT('replay'))).togglePlay(...a);
window.scrubReplay = async (...a) =>
  (await import(FEAT('replay'))).scrub(...a);
window.setReplaySpeed = async (...a) =>
  (await import(FEAT('replay'))).speed(...a);
```

- [ ] **Step 5: Rebuild + vet + test + smoke**

```bash
go run ./tools/hashstatic
go build ./... && go vet ./... && go test ./internal/server/...
./bin/naozhi --config test-config.yaml >/tmp/naozhi-smoke.log 2>&1 &
sleep 2
HASH=$(jq -r '."js/features/replay.js"' internal/server/static/dist/manifest.json)
curl -sI "http://localhost:8180/static/dist/$HASH" | head -5
kill %1
```

- [ ] **Step 6: Verify invariants**

```bash
wc -l internal/server/static/js/legacy.js
grep -c '^function openReplayViewer' internal/server/static/js/legacy.js      # 0
grep -c '^window\.openReplayViewer' internal/server/static/js/legacy.js       # 1
```

- [ ] **Step 7: Commit**

```bash
git add internal/server/static/js/features/replay.js internal/server/static/js/legacy.js internal/server/static/dist/manifest.json
git commit -m "naozhi: lazy-load replay feature (Phase 3/8)"
```

---

## Task 7: Extract cron into a lazy module

**Target:** Move `createNewCronJob`, `cronSelectSchedule`, `cronSelectWorkspace`, `toggleCronWsCustom`, `toggleCronCustom`, `previewCronSchedule`, `doCreateCronJob`, `openCronPanel`, `renderCronPanel`, `openCronSession`, `fetchCronJobs`, `cronPause`, `cronResume`, `cronDelete` (approximately lines 764-1074) into `features/cron.js`.

**Files:**
- Create: `internal/server/static/js/features/cron.js`
- Modify: `internal/server/static/js/legacy.js`

- [ ] **Step 1: Locate boundaries**

```bash
grep -n '^function createNewCronJob\|^function cron\|^async function cron\|^function openCronPanel\|^function renderCronPanel\|^function openCronSession\|^async function fetchCronJobs\|^async function doCreateCronJob\|^async function previewCronSchedule\|^function addNotification' internal/server/static/js/legacy.js
```

Region: from `createNewCronJob` to the line before `addNotification`.

- [ ] **Step 2: Create `features/cron.js`**

Use the full template from Task 2, pasting every cron function verbatim. Export surface:

```javascript
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
```

- [ ] **Step 3: Delete extracted region from `legacy.js`**

- [ ] **Step 4: Register shims (one per inline-onclick target)**

```javascript
window.openCronPanel = async (...a) => (await import(FEAT('cron'))).open(...a);
window.createNewCronJob = async (...a) => (await import(FEAT('cron'))).createNew(...a);
window.doCreateCronJob = async (...a) => (await import(FEAT('cron'))).doCreate(...a);
window.cronSelectSchedule = async (...a) => (await import(FEAT('cron'))).selectSchedule(...a);
window.cronSelectWorkspace = async (...a) => (await import(FEAT('cron'))).selectWorkspace(...a);
window.toggleCronWsCustom = async (...a) => (await import(FEAT('cron'))).toggleWsCustom(...a);
window.toggleCronCustom = async (...a) => (await import(FEAT('cron'))).toggleCustom(...a);
window.previewCronSchedule = async (...a) => (await import(FEAT('cron'))).preview(...a);
window.openCronSession = async (...a) => (await import(FEAT('cron'))).openSession(...a);
window.cronPause = async (...a) => (await import(FEAT('cron'))).pause(...a);
window.cronResume = async (...a) => (await import(FEAT('cron'))).resume(...a);
window.cronDelete = async (...a) => (await import(FEAT('cron'))).remove(...a);
```

- [ ] **Step 5: Rebuild + vet + test + smoke**

```bash
go run ./tools/hashstatic
go build ./... && go vet ./... && go test ./internal/server/...
./bin/naozhi --config test-config.yaml >/tmp/naozhi-smoke.log 2>&1 &
sleep 2
HASH=$(jq -r '."js/features/cron.js"' internal/server/static/dist/manifest.json)
curl -sI "http://localhost:8180/static/dist/$HASH" | head -5
kill %1
```

- [ ] **Step 6: Verify invariants**

```bash
wc -l internal/server/static/js/legacy.js
grep -c '^function openCronPanel' internal/server/static/js/legacy.js    # 0
grep -c '^window\.openCronPanel' internal/server/static/js/legacy.js     # 1
```

- [ ] **Step 7: Commit**

```bash
git add internal/server/static/js/features/cron.js internal/server/static/js/legacy.js internal/server/static/dist/manifest.json
git commit -m "naozhi: lazy-load cron feature (Phase 3/8)"
```

---

## Task 8: Extract file-hub into a lazy module

**Target:** Move `fhFetch`, `openFileHub`, `closeFileHub`, `fhNavigate`, `fhToggleHidden`, `fhRenderBreadcrumb`, `fhRenderList`, `fhRenderToolbar`, `fhRowClick`, `fhRowDblClick`, `fhGoUp`, `fhToggle`, `fhFormatSize`, `fhFormatDate`, `fhGetSelectedPaths`, `fhInsertPath`, `fhCopyPath`, `fhShowUpload`, `fhDrop`, `fhUploadFiles`, `fhDownloadSelected`, `fhPromptMkdir`, `fhDeleteSelected`, `fhLsNavigate` (approximately lines 1250-1716) into `features/file-hub.js`. **Do NOT move `enhanceLsOutput`** (line 1676) — it is called from `views/chat.js` on every `/ls` render and must stay eager.

**Files:**
- Create: `internal/server/static/js/features/file-hub.js`
- Modify: `internal/server/static/js/legacy.js`

- [ ] **Step 1: Locate boundaries and identify the eager/lazy split point**

```bash
grep -n '^async function fhFetch\|^function openFileHub\|^function enhanceLsOutput\|^function fhLsNavigate\|^async function fetchNotifications' internal/server/static/js/legacy.js
```

Two extract regions:
1. `fhFetch` through the line before `enhanceLsOutput`.
2. `fhLsNavigate` only (not `enhanceLsOutput`).

`enhanceLsOutput` stays in `legacy.js`.

- [ ] **Step 2: Create `features/file-hub.js`**

```javascript
// internal/server/static/js/features/file-hub.js
// File Hub: browse/upload/download/mkdir overlay.
import { apiFetch } from '../core/api.js';
import { esc, html } from '../core/html.js';

let initialized = false;
function ensureInit() {
  if (initialized) return;
  initialized = true;
}

// --- Begin extracted from legacy.js:1250-1675 ---
async function fhFetch(endpoint, options) { /* paste verbatim */ }
function openFileHub(initialPath, sessionCtx) { /* paste verbatim */ }
function closeFileHub() { /* paste verbatim */ }
async function fhNavigate(path) { /* paste verbatim */ }
function fhToggleHidden() { /* paste verbatim */ }
function fhRenderBreadcrumb() { /* paste verbatim */ }
function fhRenderList() { /* paste verbatim */ }
function fhRenderToolbar() { /* paste verbatim */ }
function fhRowClick(ev, idx) { /* paste verbatim */ }
function fhRowDblClick(idx) { /* paste verbatim */ }
function fhGoUp() { /* paste verbatim */ }
function fhToggle(idx) { /* paste verbatim */ }
function fhFormatSize(bytes) { /* paste verbatim */ }
function fhFormatDate(iso) { /* paste verbatim */ }
function fhGetSelectedPaths() { /* paste verbatim */ }
function fhInsertPath() { /* paste verbatim */ }
function fhCopyPath() { /* paste verbatim */ }
function fhShowUpload() { /* paste verbatim */ }
function fhDrop(ev) { /* paste verbatim */ }
async function fhUploadFiles(fileList) { /* paste verbatim */ }
function fhDownloadSelected() { /* paste verbatim */ }
async function fhPromptMkdir() { /* paste verbatim */ }
async function fhDeleteSelected() { /* paste verbatim */ }
// --- End extracted ---

// --- Extracted from legacy.js:1716 ---
function fhLsNavigate(path) { /* paste verbatim */ }
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
```

- [ ] **Step 3: Delete extracted regions from `legacy.js`**

Leave `enhanceLsOutput` in place. Confirm:

```bash
grep -n '^function enhanceLsOutput' internal/server/static/js/legacy.js
```

- [ ] **Step 4: Register shims**

```javascript
window.openFileHub = async (...a) => (await import(FEAT('file-hub'))).open(...a);
window.closeFileHub = async (...a) => (await import(FEAT('file-hub'))).close(...a);
window.fhLsNavigate = async (...a) => (await import(FEAT('file-hub'))).lsNavigate(...a);
window.fhNavigate = async (...a) => (await import(FEAT('file-hub'))).navigate(...a);
window.fhToggleHidden = async (...a) => (await import(FEAT('file-hub'))).toggleHidden(...a);
window.fhRowClick = async (...a) => (await import(FEAT('file-hub'))).rowClick(...a);
window.fhRowDblClick = async (...a) => (await import(FEAT('file-hub'))).rowDblClick(...a);
window.fhGoUp = async (...a) => (await import(FEAT('file-hub'))).goUp(...a);
window.fhToggle = async (...a) => (await import(FEAT('file-hub'))).toggle(...a);
window.fhInsertPath = async (...a) => (await import(FEAT('file-hub'))).insertPath(...a);
window.fhCopyPath = async (...a) => (await import(FEAT('file-hub'))).copyPath(...a);
window.fhShowUpload = async (...a) => (await import(FEAT('file-hub'))).showUpload(...a);
window.fhDrop = async (...a) => (await import(FEAT('file-hub'))).drop(...a);
window.fhDownloadSelected = async (...a) => (await import(FEAT('file-hub'))).downloadSelected(...a);
window.fhPromptMkdir = async (...a) => (await import(FEAT('file-hub'))).promptMkdir(...a);
window.fhDeleteSelected = async (...a) => (await import(FEAT('file-hub'))).deleteSelected(...a);
```

- [ ] **Step 5: Rewrite `enhanceLsOutput` call-site**

Inside `enhanceLsOutput`, the code generates HTML with `onclick="fhLsNavigate(...)"` etc. These inline handlers now resolve to the window shims — no source change needed inside `enhanceLsOutput`. Grep confirm no direct JS call to extracted names remains:

```bash
grep -n 'fhNavigate\|fhLsNavigate\|openFileHub\|closeFileHub' internal/server/static/js/legacy.js
```

Only hits should be inside `enhanceLsOutput` HTML strings (fine) and the shim block.

- [ ] **Step 6: Rebuild + vet + test + smoke**

```bash
go run ./tools/hashstatic
go build ./... && go vet ./... && go test ./internal/server/...
./bin/naozhi --config test-config.yaml >/tmp/naozhi-smoke.log 2>&1 &
sleep 2
HASH=$(jq -r '."js/features/file-hub.js"' internal/server/static/dist/manifest.json)
curl -sI "http://localhost:8180/static/dist/$HASH" | head -5
kill %1
```

- [ ] **Step 7: Verify invariants**

```bash
wc -l internal/server/static/js/legacy.js
grep -c '^function openFileHub' internal/server/static/js/legacy.js    # 0
grep -c '^window\.openFileHub' internal/server/static/js/legacy.js     # 1
grep -c '^function enhanceLsOutput' internal/server/static/js/legacy.js # 1 — STILL PRESENT
```

- [ ] **Step 8: Commit**

```bash
git add internal/server/static/js/features/file-hub.js internal/server/static/js/legacy.js internal/server/static/dist/manifest.json
git commit -m "naozhi: lazy-load file-hub feature (Phase 3/8)"
```

---

## Task 9: Extract search into a lazy module (keyboard dispatcher stays eager)

**Target:** Move `navigateSearchResult`, `openSearch`, `closeSearch`, `_searchHighlight`, `_searchContextSnippet`, `_performSearch`, `_searchSelect`, `_searchHover`, `_searchUpdateSelection` (approximately lines 2087-2376) into `features/search.js`. Keep the `document.addEventListener('keydown', ...)` block in `legacy.js` so Cmd+K is bound before any user interaction.

**Files:**
- Create: `internal/server/static/js/features/search.js`
- Modify: `internal/server/static/js/legacy.js`

- [ ] **Step 1: Locate boundaries**

```bash
grep -n '^function navigateSearchResult\|^function openSearch\|^function closeSearch\|^function _search\|^async function _performSearch\|^function openReplayViewer\|^document.addEventListener' internal/server/static/js/legacy.js
```

Extract region: from `navigateSearchResult` (2087) through the last `_search*` helper (before `openReplayViewer`). The keyboard dispatcher (`document.addEventListener('keydown', ...)` around line 2676) stays in `legacy.js`.

- [ ] **Step 2: Create `features/search.js`**

```javascript
// internal/server/static/js/features/search.js
// Cmd+K search overlay + result rendering.
import { apiFetch } from '../core/api.js';
import { esc, html } from '../core/html.js';

let initialized = false;
function ensureInit() {
  if (initialized) return;
  initialized = true;
}

// --- Begin extracted from legacy.js:2087-2376 ---
function navigateSearchResult(source, pathOrKey) { /* paste verbatim */ }
function openSearch() { /* paste verbatim */ }
function closeSearch() { /* paste verbatim */ }
function _searchHighlight(text, query) { /* paste verbatim */ }
function _searchContextSnippet(text, query, maxLen) { /* paste verbatim */ }
async function _performSearch(query) { /* paste verbatim */ }
function _searchSelect(idx) { /* paste verbatim */ }
function _searchHover(idx) { /* paste verbatim */ }
function _searchUpdateSelection() { /* paste verbatim */ }
// --- End extracted ---

export async function open(...a) { ensureInit(); return openSearch(...a); }
export async function close(...a) { ensureInit(); return closeSearch(...a); }
```

- [ ] **Step 3: Delete extracted region from `legacy.js`**

- [ ] **Step 4: Register shims**

```javascript
window.openSearch = async (...a) => (await import(FEAT('search'))).open(...a);
window.closeSearch = async (...a) => (await import(FEAT('search'))).close(...a);
```

- [ ] **Step 5: Verify keyboard dispatcher still compiles**

The dispatcher calls `openSearch()` and `closeSearch()`. Rewrite those to `window.openSearch()` / `window.closeSearch()` so the lazy path is taken:

```javascript
document.addEventListener('keydown', async function(e) {
  // ... Cmd+K detection ...
  var ov = document.getElementById('searchOverlay');
  if (ov && ov.classList.contains('show')) {
    await window.closeSearch();
  } else {
    await window.openSearch();
  }
  // ... Escape/ArrowDown/ArrowUp/Enter branches also call window.closeSearch / _searchSelect ...
});
```

The internal `_searchSelect`, `_searchUpdateSelection` calls inside the dispatcher's Arrow/Enter branches need a different approach — they access module-local state. Options:

a) Move the keyboard dispatcher into `features/search.js` and bind it inside `ensureInit`. Drawback: first-keydown race — if the user presses a search key before the overlay is ever opened, the dispatcher is not bound.

b) Keep the dispatcher in `legacy.js` and route arrow/enter through new window shims `window._searchArrow(dir)` / `window._searchEnter()` that lazy-import and delegate.

Prefer (b). Extend `features/search.js` exports:

```javascript
export async function arrow(dir) { ensureInit(); if (dir === 'down') { /* copy ArrowDown body */ } else { /* ArrowUp body */ } }
export async function enter() { ensureInit(); /* copy Enter body */ }
```

And register:

```javascript
window._searchArrow = async (dir) => (await import(FEAT('search'))).arrow(dir);
window._searchEnter = async () => (await import(FEAT('search'))).enter();
```

Rewrite the dispatcher's Arrow/Enter branches to `await window._searchArrow('down')` / `await window._searchEnter()`.

- [ ] **Step 6: Rebuild + vet + test + smoke**

```bash
go run ./tools/hashstatic
go build ./... && go vet ./... && go test ./internal/server/...
./bin/naozhi --config test-config.yaml >/tmp/naozhi-smoke.log 2>&1 &
sleep 2
HASH=$(jq -r '."js/features/search.js"' internal/server/static/dist/manifest.json)
curl -sI "http://localhost:8180/static/dist/$HASH" | head -5
kill %1
```

- [ ] **Step 7: Verify invariants**

```bash
wc -l internal/server/static/js/legacy.js
grep -c '^function openSearch' internal/server/static/js/legacy.js    # 0
grep -c '^window\.openSearch' internal/server/static/js/legacy.js     # 1
grep -c '^document.addEventListener' internal/server/static/js/legacy.js  # at least 1 — dispatcher still there
```

- [ ] **Step 8: Commit**

```bash
git add internal/server/static/js/features/search.js internal/server/static/js/legacy.js internal/server/static/dist/manifest.json
git commit -m "naozhi: lazy-load search feature (Phase 3/8)"
```

---

## Task 10: Re-measure first-paint JS and update baseline

**Files:**
- Modify: `docs/superpowers/plans/2026-04-19-phase1-perf-baseline.md`

- [ ] **Step 1: Build static assets and the binary**

```bash
go run ./tools/hashstatic
CGO_ENABLED=0 go build -o bin/naozhi ./cmd/naozhi/
```

- [ ] **Step 2: Start the server and measure gzipped transfer sizes**

```bash
./bin/naozhi --config test-config.yaml >/tmp/naozhi-smoke.log 2>&1 &
sleep 2

# Fetch the dashboard HTML and extract the JS URLs it references
HTML=$(curl -s -H 'Accept-Encoding: gzip' http://localhost:8180/dashboard | gunzip)

# Enumerate eager JS (all <script src=...> entries)
echo "$HTML" | grep -oE 'src="[^"]+\.js"' | sed 's/src="//;s/"//'

# Measure each eager JS chunk's gzip transfer size
for URL in $(echo "$HTML" | grep -oE 'src="[^"]+\.js"' | sed 's/src="//;s/"//'); do
  SIZE=$(curl -s -H 'Accept-Encoding: gzip' -o /dev/null -w '%{size_download}' "http://localhost:8180$URL")
  printf '%-60s %s bytes\n' "$URL" "$SIZE"
done

# Sum
TOTAL=0
for URL in $(echo "$HTML" | grep -oE 'src="[^"]+\.js"' | sed 's/src="//;s/"//'); do
  SIZE=$(curl -s -H 'Accept-Encoding: gzip' -o /dev/null -w '%{size_download}' "http://localhost:8180$URL")
  TOTAL=$((TOTAL + SIZE))
done
echo "First-paint JS gzip total: $TOTAL bytes"

kill %1
```

Record the total. Target: < 51200 (50 KB).

- [ ] **Step 3: Append a "Phase 3 — legacy.js split" section to the baseline doc**

Edit `docs/superpowers/plans/2026-04-19-phase1-perf-baseline.md`. Append:

```markdown
## Phase 3 — legacy.js split (2026-04-19)

Measurement after 8 feature-module extractions landed.

| Chunk | Before (Phase 1) | After (Phase 3) | Δ |
|---|---|---|---|
| legacy.js | 27.9 KB | <fill in> | <fill in> |
| app.js | 2.3 KB | 2.3 KB | 0 |
| core/* | 14.5 KB | 14.5 KB | 0 |
| Eager total (first-paint) | 70.1 KB | <fill in> | <fill in> |
| Lazy features/* (not first-paint) | 0 | ~19.5 KB | +19.5 KB |

**Target met:** <yes/no> (first-paint gzip must be < 50 KB).

Verification commands: see Task 10 Step 2 of
`docs/superpowers/plans/2026-04-19-naozhi-phase3-legacy-split.md`.
```

Fill in the `<fill in>` fields from Step 2's measurement.

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/plans/2026-04-19-phase1-perf-baseline.md
git commit -m "naozhi: record Phase 3 perf measurements in baseline doc"
```

---

## Task 11: Cross-browser sanity via curl matrix

**Files:** None (verification only)

- [ ] **Step 1: Start the server locally**

```bash
CGO_ENABLED=0 go build -o bin/naozhi ./cmd/naozhi/
./bin/naozhi --config test-config.yaml >/tmp/naozhi-smoke.log 2>&1 &
sleep 2
```

- [ ] **Step 2: Run the hashed-asset smoke matrix**

```bash
check() {
  local URL="$1"
  local CODE=$(curl -sI "http://localhost:8180$URL" | awk 'NR==1 {print $2}')
  local CC=$(curl -sI "http://localhost:8180$URL" | awk -F': ' '/^Cache-Control/ {print $2}' | tr -d '\r')
  printf '%-70s code=%s cache=%s\n' "$URL" "$CODE" "$CC"
}

# Manifest + dashboard
check /dashboard
check /static/dist/manifest.json

# Eight feature chunks (path derived from manifest)
for F in context-panel notif-enhance bookmark twin replay cron file-hub search; do
  HASH=$(jq -r ".\"js/features/$F.js\"" internal/server/static/dist/manifest.json)
  check "/static/dist/$HASH"
done

# Eager legacy.js
HASH=$(jq -r '."js/legacy.js"' internal/server/static/dist/manifest.json)
check "/static/dist/$HASH"
```

Expected: every feature chunk returns `code=200 cache=public, max-age=31536000, immutable`.

- [ ] **Step 3: Stop the local server**

```bash
kill %1
```

- [ ] **Step 4: Commit the curl script if you want to keep it (optional)**

If you saved the commands into a script, commit under `tools/smoke-phase3.sh`. Otherwise skip.

---

## Task 12: Deploy to EC2 and tag the release

**Files:**
- Modify: `docs/naozhi-deployment.md`

- [ ] **Step 1: Cross-compile for production (amd64 Linux — EC2 is t3.small amd64, not ARM)**

Re-confirm the EC2 architecture before cross-compiling:

```bash
ssh ubuntu@10.0.11.189 'uname -m'
```

If output is `x86_64`, build amd64:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/naozhi-linux-amd64 ./cmd/naozhi/
```

If output is `aarch64`, build arm64 instead:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin/naozhi-linux-arm64 ./cmd/naozhi/
```

- [ ] **Step 2: Backup current production binary**

```bash
ssh ubuntu@10.0.11.189 'sudo cp /usr/local/bin/naozhi /usr/local/bin/naozhi.pre-phase3'
ssh ubuntu@10.0.11.189 'ls -la /usr/local/bin/naozhi.pre-phase3'
```

- [ ] **Step 3: Upload the new binary**

```bash
scp bin/naozhi-linux-amd64 ubuntu@10.0.11.189:/tmp/naozhi.new
ssh ubuntu@10.0.11.189 'sudo install -m 0755 /tmp/naozhi.new /usr/local/bin/naozhi && rm /tmp/naozhi.new'
```

- [ ] **Step 4: Restart the service**

```bash
ssh ubuntu@10.0.11.189 'sudo systemctl restart naozhi && sleep 2 && sudo systemctl status naozhi | head -15'
```

Expected: `active (running)`.

- [ ] **Step 5: Remote smoke**

```bash
ssh ubuntu@10.0.11.189 'curl -sI http://localhost:8180/dashboard | head -3'
ssh ubuntu@10.0.11.189 'curl -s http://localhost:8180/static/dist/manifest.json | jq "keys | length"'
```

Expected: `HTTP/1.1 200` and a count that matches the local manifest's key count.

- [ ] **Step 6: Tag the release**

```bash
git tag -a phase-3-legacy-split -m "Phase 3: legacy.js split into 8 lazy feature modules, first-paint JS < 50 KB"
git push origin phase-3-legacy-split
```

- [ ] **Step 7: Append a "Phase 3" changelog section to `docs/naozhi-deployment.md`**

Add under the existing changelog:

```markdown
### Phase 3 — legacy.js feature split (2026-04-19)

**Tag:** `phase-3-legacy-split`
**Binary backup:** `/usr/local/bin/naozhi.pre-phase3`

**What changed:**
- `internal/server/static/js/legacy.js`: 2728 lines → <fill in>.
- `internal/server/static/js/features/`: eight new lazy modules (context-panel, notif-enhance, bookmark, twin, replay, cron, file-hub, search).
- First-paint JS gzip: 70.1 KB → <fill in>.

**Rollback:**
```bash
ssh ubuntu@10.0.11.189 'sudo cp /usr/local/bin/naozhi.pre-phase3 /usr/local/bin/naozhi && sudo systemctl restart naozhi'
```

Fill in the two `<fill in>` fields from Task 10 Step 2.

- [ ] **Step 8: Commit the deployment manual update**

```bash
git add docs/naozhi-deployment.md
git commit -m "naozhi: document Phase 3 deployment in manual"
git push origin naozhi-2.0
```

- [ ] **Step 9: Ask the user for browser smoke**

Post in chat: the production URL (`https://naozhi.keithyu.cloud/dashboard`) is now on Phase 3. Please exercise each feature (context panel, notifications, bookmark, twin, replay, cron, file hub, cmd+K search) and report any regressions. If any feature breaks, run the rollback from Step 7. If all features work, Phase 3 is complete.

---

## Verification summary

After Task 12 completes, these must all hold:

- `wc -l internal/server/static/js/legacy.js` strictly less than 2728.
- `ls internal/server/static/js/features/ | grep -c '\.js$'` = 8.
- `jq 'keys | length' internal/server/static/dist/manifest.json` incremented by 8 (plus re-hashed legacy.js).
- First-paint JS gzip total < 50 KB (Task 10 Step 2).
- `go build ./... && go vet ./... && go test ./internal/server/...` all pass.
- All eight feature chunks return 200 + `public, max-age=31536000, immutable`.
- `git tag --list | grep phase-3-legacy-split` present.
- Production service `active (running)` and dashboard returns 200.

If any invariant fails, STOP and ask the user before proceeding.
