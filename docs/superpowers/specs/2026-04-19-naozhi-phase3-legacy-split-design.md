# Naozhi Phase 3: legacy.js Feature Split Design

**Date:** 2026-04-19
**Status:** Draft — awaiting user review before plan
**Phase 1 baseline:** `docs/superpowers/plans/2026-04-19-phase1-perf-baseline.md`
**Previous phase:** `docs/superpowers/plans/2026-04-19-naozhi-phase1-frontend-split.md`

## Goal

Bring first-paint JS gzip under 50 KB (from current 70.1 KB) by splitting
`internal/server/static/js/legacy.js` (2728 lines, 27.9 KB gzip) into eight
user-triggered feature modules loaded on demand.

## Non-goals

- Splitting `views/chat.js` (25.3 KB gzip) — deferred to a later phase.
- Introducing a framework (React/Vue) or custom-element rewrite.
- Changing any visible behavior. Pure structural refactor.
- Adding E2E browser test infrastructure.

## Architecture

```
js/legacy.js (eager, target ~7 KB gzip)
├── shared helpers: showToast, copyText, copyCode, copyCodeBlock,
│   highlightBlock, renderMd, inlineMd, renderTable, renderDiff,
│   toggleCollapse, shortPath, timeAgo, sessionTimeHint,
│   processEventsForDisplay, sid, isMultiNode, nodeColor
├── CDN loaders: loadMermaid/runMermaid, loadKatex/runKatex, renderKatex
│   (already lazy; stay in legacy.js because used by renderMd)
├── Discovery & Takeover (home autopoll; always running)
├── Mobile Navigation + Tab Bar + Copy-Tap (viewport-scoped, always needed)
├── Notification Center base (permission request + display)
├── /ls Enhanced Rendering (called from chat message handler)
├── View Switching stub (defers to router.js)
├── Initialization (DOMContentLoaded bootstrap)
├── Keyboard shortcuts dispatcher (cmd-K, cmd-N, ESC)
│   — listens globally, lazy-imports search.js on first cmd-K
└── window.* shims (end-oil): openFileHub, openCronPanel,
    openSearch, closeSearch, openReplayViewer, openTwinPanel,
    openContextPanel, loadSessionBookmarks, injectBookmarkButtons,
    showBookmarkPopover, setupPushNotifications, handleIncomingNotif

js/features/ (new directory, all lazy-loaded)
├── context-panel.js    ~1.6 KB gzip  (was lines 1869-1976)
├── notif-enhance.js    ~1.6 KB gzip  (was lines 1755-1869)
├── bookmark.js         ~1.4 KB gzip  (was ~lines 1976-2050, incl. showBookmarkPopover)
├── twin.js             ~1.3 KB gzip  (was ~lines 2543-2670, openTwinPanel)
├── replay.js           ~2.0 KB gzip  (was ~lines 2379-2540, openReplayViewer)
├── cron.js             ~4.0 KB gzip  (was lines 760-1074, openCronPanel)
├── file-hub.js         ~4.1 KB gzip  (was lines 1241-1674, openFileHub)
└── search.js           ~3.5 KB gzip  (was ~lines 2088-2345, openSearch/closeSearch)
```

**Total moved to lazy:** ~19.5 KB gzip.
**Predicted first-paint JS:** 70.1 − 19.5 ≈ **50.6 KB gzip** (at the target line).

If the result misses <50 KB by a small margin, the keyboard-shortcut
dispatcher and/or the `/ls Enhanced Rendering` helpers are candidate
follow-up splits. The Phase 3 plan will re-measure and open a follow-up
issue if needed.

## Module interface

Every feature module conforms to this shape:

```javascript
// js/features/<name>.js
import { apiFetch } from '../core/api.js';
import { esc, html } from '../core/html.js';
// (state, utils as needed)

let initialized = false;
function ensureInit() {
  if (initialized) return;
  initialized = true;
  // Inject overlay DOM, bind module-scoped listeners
}

export async function open(...args) {
  ensureInit();
  // ... body copied verbatim from the original legacy.js function
}

// Secondary entry points if the feature has multiple window-visible fns.
export async function close(...args) { ... }
```

**End-oil registration in legacy.js:**

```javascript
const FEAT = (name) => window.__resolveAsset('js/features/' + name + '.js');

window.openFileHub = async (...a) =>
  (await import(FEAT('file-hub'))).open(...a);
window.openCronPanel = async (...a) =>
  (await import(FEAT('cron'))).open(...a);
window.openSearch = async (...a) =>
  (await import(FEAT('search'))).open(...a);
window.closeSearch = async (...a) =>
  (await import(FEAT('search'))).close(...a);
window.openReplayViewer = async (...a) =>
  (await import(FEAT('replay'))).open(...a);
window.openTwinPanel = async (...a) =>
  (await import(FEAT('twin'))).open(...a);
window.openContextPanel = async (...a) =>
  (await import(FEAT('context-panel'))).open(...a);
window.loadSessionBookmarks = async (...a) =>
  (await import(FEAT('bookmark'))).load(...a);
window.injectBookmarkButtons = async (...a) =>
  (await import(FEAT('bookmark'))).inject(...a);
window.showBookmarkPopover = async (...a) =>
  (await import(FEAT('bookmark'))).showPopover(...a);
window.setupPushNotifications = async (...a) =>
  (await import(FEAT('notif-enhance'))).setup(...a);
window.handleIncomingNotif = async (...a) =>
  (await import(FEAT('notif-enhance'))).handle(...a);
```

**Key properties:**
1. First call pays one network round-trip + module execution (~50-200 ms).
2. Subsequent calls hit the browser's module cache synchronously.
3. Existing inline `onclick="openFileHub()"` attributes in HTML continue
   to work — the window-level shim is synchronously registered by
   `legacy.js` on DOMContentLoaded.
4. `window.__resolveAsset` comes from the manifest injection that Phase 1
   Task 15 already shipped. Falls back to `/static/js/features/<name>.js`
   (unhashed) when the manifest is missing (dev mode without `make static`).

## Shared state

Any feature-owned state currently on `window.*` migrates into
`internal/server/static/js/core/state.js` and uses the existing
`Object.defineProperty` bridge. The list (from a grep of legacy.js):

- `window.fileHubState` — File Hub open/scroll state
- `window.searchResults`, `window.searchIndex` — cmd-K cache
- `window.replayState` — Replay playback position/speed
- `window.twinConfig` — Twin overlay config draft
- `window.bookmarks` — Per-session bookmark list

Helpers used by multiple feature modules (`esc`, `html`, `apiFetch`,
`authHeaders`) are already in `core/*` and imported directly by each
feature module. No need for a separate `features/common.js`.

## Cross-feature call audit

Grep of legacy.js call-sites (2026-04-19):

| Function | Call-sites | Source context | Crosses module boundary? |
|---|---|---|---|
| `openFileHub` | 3 | 716 (mobile tab bar), 1260 (def), 1711 (/ls render button) | Yes, via `window.openFileHub` (shim) |
| `openCronPanel` | 2 | 714 (mobile tab bar), 959 (def) | Yes, via shim |
| `openSearch` | 2 | 2106 (def), 2684 (keyboard shortcut) | Keyboard dispatches via shim |
| `closeSearch` | 5 | 2088, 2131 (def), 2341 (internal), 2682, 2696 (keyboard) | Internal calls become module-local; external via shim |
| `openReplayViewer` | 1 | 2379 (def) — external callers in views/chat.js | Via shim |
| `openTwinPanel` | 1 | 2543 (def) — external caller in views/home.js | Via shim |
| `showBookmarkPopover` | 2 | 2025 (internal), 2031 (def) | Internal only |
| `injectBookmarkButtons` | 1 | 1993 (def) — external callers in views/chat.js | Via shim |
| `openContextPanel` | 0 in legacy | External caller in views/chat.js | Via shim |

All cross-module calls already go through `window.*`. No inter-feature
import dependencies exist — each feature is self-contained apart from
the shared `core/*` utilities.

## Rollout plan

Eight split commits (one feature each), ordered lowest-risk first:

| # | Feature | Notes | Expected Δ |
|---|---|---|---|
| 1 | context-panel | Self-contained overlay, 0 external callers from legacy | −1.6 KB |
| 2 | notif-enhance | Self-contained | −1.6 KB |
| 3 | bookmark | Internal-only + 1 external, safe | −1.4 KB |
| 4 | twin | Self-contained overlay | −1.3 KB |
| 5 | replay | Has WS event subscription; verify doesn't miss events during import | −2.0 KB |
| 6 | cron | Polling loop — ensure it starts only after first `openCronPanel` | −4.0 KB |
| 7 | file-hub | Multi-entry + DnD; largest chunk | −4.1 KB |
| 8 | search | Keyboard dispatcher stays in legacy; module owns UI | −3.5 KB |

Final commit: re-run Task 16 perf script, update
`docs/superpowers/plans/2026-04-19-phase1-perf-baseline.md` with Phase 3
numbers, EC2 deploy + git tag `phase-3-legacy-split`.

## Verification gates (per feature commit)

1. `go run ./tools/hashstatic` emits new `dist/manifest.json` with the
   feature's hashed entry.
2. `go build ./... && go vet ./... && go test ./internal/server/...`
3. `grep -cE 'function open<Feature>\\b' internal/server/static/js/legacy.js` = 0
   (old implementation removed)
4. `grep -cE 'window\\.open<Feature>\\s*=' internal/server/static/js/legacy.js` = 1
   (shim registered)
5. `wc -l internal/server/static/js/legacy.js` strictly decreases
6. `curl -sI http://localhost:8180/static/dist/js/features/<name>.<hash>.js`
   → 200 + `Cache-Control: public, max-age=31536000, immutable`

## Risks

1. **End-oil race:** Rapid double-click triggers two concurrent `import()`
   calls. Browser returns the same Promise, so only one module instance
   executes. Each module's `initialized` guard makes `open()` idempotent.
2. **First keyboard shortcut loss:** The `Cmd+K` handler must be bound
   synchronously by `legacy.js`, not by `search.js`. The handler calls
   `window.openSearch()` (the shim) which lazy-loads. First invocation
   adds 50-200 ms latency; acceptable for a one-time cost.
3. **Service Worker stale cache:** `sw.js` CACHE_VERSION already tracks
   content hash. Phase 3 triggers a cold fetch of every feature chunk on
   first visit post-deploy — expected and cache-normal.
4. **onclick attribute timing:** Inline `onclick="openFileHub()"` exists
   in pre-rendered HTML before `legacy.js` executes. This is the same
   pattern Phase 1 already validated — `legacy.js` loads with `defer`,
   runs before user interaction, installs shims before any click fires.

## Rollback

Each feature commit is independently revertable. If a single feature
breaks in production:

- **Single-feature rollback:** `git revert <feature-commit>`, re-run
  `make static`, redeploy.
- **Full Phase 3 rollback:** `git revert <first-commit>..<last-commit>`
  or `git checkout phase-1-frontend-split -- internal/server/static`.
- **Production hot rollback:**
  `ssh ubuntu@10.0.11.189 'sudo cp /usr/local/bin/naozhi.pre-phase3
  /usr/local/bin/naozhi && sudo systemctl restart naozhi'`
  (backup taken at the start of the Phase 3 deploy task).

## Testing strategy

1. **Go tests (existing):** `go test ./internal/server/...` continues to
   cover `handleStatic`, `handleDashboard`, manifest parsing. No new Go
   code — no new Go tests required.
2. **Protocol-layer smoke (per feature):** `curl` each hashed chunk URL;
   verify 200 + immutable cache headers. The curl matrix from Phase 1
   Task 17 is reused and extended with the eight feature paths.
3. **Browser smoke (per feature, manual by user after deploy):**
   - Context Panel: click session info icon
   - Notif Enhance: trigger a notification
   - Bookmark: add a bookmark, verify popover
   - Twin: open CTO Digital Twin config
   - Replay: session right-click → Replay
   - Cron: navigate to Cron tab
   - File Hub: open File Hub, upload a file
   - Search: press Cmd+K, search `test`, click result

## Performance target

- First-paint JS gzip: **< 50 KB** (currently 70.1 KB).
- legacy.js gzip after split: **< 8 KB** (currently 27.9 KB).
- No regression in first-paint CSS (19.6 KB gzip) or HTML (3.6 KB gzip).

Baseline doc update: append a "Phase 3 — legacy.js split" section to
`docs/superpowers/plans/2026-04-19-phase1-perf-baseline.md` with before
(Phase 1 post-deploy) vs after (Phase 3 post-deploy) numbers.
