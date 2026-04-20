# Phase 1 First-Paint Performance Baseline

**Measurement date:** 2026-04-19
**Commit:** `b852dc86cc3e0f8a69440d34e1a7289dc60c5f77` (branch `naozhi-2.0`)
**Method:** real HTTP GET against locally-bound `:19871` with `Accept-Encoding: gzip`. Server serves gzip for all `/static/dist/*` and `/dashboard`. Bytes measured post-transfer via `wc -c` (exact compressed body length).
**Baseline target (from plan Task 16):** first-paint JS < 50 KB (51200 bytes) gzip. Pre-split dashboard.html monolith = **71555 bytes gzip** (69.9 KB).

## Dashboard HTML

| Asset | Bytes (gzip) | KB |
|---|---:|---:|
| `/dashboard` (HTML shell + `window.__MANIFEST`) | 3700 | 3.6 |

## First-paint JS (eager — transitive static-import closure from `app.js` + non-module `legacy.js`)

The browser must download all of the following before the initial chat view can render. Transitive closure walked via `grep -E "^import"` starting from `app.js` (module entry) and `legacy.js` (deferred). The 5 lazy views (knowledge, wiki, patrols, approvals, graph) and d3 are excluded — they load on nav click via dynamic `import()`.

| Logical path | Hashed filename | Bytes (gzip) |
|---|---|---:|
| `js/app.js` | `js/app.c2f077ac.js` | 843 |
| `js/legacy.js` | `js/legacy.1bf8e28b.js` | 28193 |
| `js/core/html.js` | `js/core/html.7d306602.js` | 817 |
| `js/core/utils.js` | `js/core/utils.3e644f12.js` | 2593 |
| `js/core/api.js` | `js/core/api.8202d541.js` | 686 |
| `js/core/state.js` | `js/core/state.ab86fffc.js` | 737 |
| `js/core/router.js` | `js/core/router.9c233460.js` | 2188 |
| `js/core/ws.js` | `js/core/ws.7864c345.js` | 6031 |
| `js/views/chat.js` | `js/views/chat.948770c7.js` | 25300 |
| `js/views/home.js` | `js/views/home.a2bd21fc.js` | 4416 |
| **Total JS first-paint** | | **71804** |

**71804 bytes ≈ 70.1 KB gzip.**

## First-paint CSS (all stylesheets in `<head>` — render-blocking)

All 9 CSS bundles are linked directly in `dashboard.html`, so every view's CSS is blocking for first paint. This is intentional (CSS is small, avoids FOUC on lazy-view nav).

| Hashed path | Bytes (gzip) |
|---|---:|
| `css/base.6c9ff3c0.css` | 7248 |
| `css/components.6df29555.css` | 3127 |
| `css/views/home.dc6e7e69.css` | 1231 |
| `css/views/chat.6b2086a5.css` | 1348 |
| `css/views/knowledge.31765d8b.css` | 2565 |
| `css/views/wiki.949c4df9.css` | 1469 |
| `css/views/patrols.9352f683.css` | 1269 |
| `css/views/approvals.5aa6c794.css` | 1134 |
| `css/views/graph.f2a31c5b.css` | 720 |
| **Total CSS first-paint** | **20111** |

**20111 bytes ≈ 19.6 KB gzip.**

## Totals

| Bucket | Bytes | KB |
|---|---:|---:|
| HTML | 3700 | 3.6 |
| JS (eager) | 71804 | 70.1 |
| CSS (eager) | 20111 | 19.6 |
| **Total first-paint transfer** | **95615** | **93.4** |

## Baseline comparison

| Metric | Pre-split | Post-split (Phase 1) | Delta |
|---|---:|---:|---:|
| First-paint JS gzip | 71555 B (69.9 KB, monolithic `dashboard.html`) | 71804 B (70.1 KB) | **+249 B (+0.3%)** |
| Target (<50 KB JS) | — | **NOT MET** | — |

**Target status: FAIL.** The plan's 50 KB target is not met by Phase 1 alone. Phase 1 was structural (split the monolith into modules + hashed bundles + lazy views). The byte count is essentially unchanged because:

- `legacy.js` (28.2 KB gzip) still carries all the un-migrated logic (approvals/patrols/wiki/knowledge/graph helpers that are referenced via global window handlers on the dashboard template).
- `views/chat.js` (25.3 KB gzip) holds the entire chat renderer, event log, and dispatch code — the largest single user-facing surface.
- Every other module is <7 KB.

Together those two files are **53.5 KB** — they alone exceed the 50 KB target.

## What IS lazy (not counted above)

Confirmed via `internal/server/static/js/core/router.js` dynamic `import()`:

- `views/knowledge.js`
- `views/wiki.js`
- `views/patrols.js`
- `views/approvals.js`
- `views/graph.js` (plus d3 loaded via CDN on graph nav)

Plus each view's CSS (wait — CSS is already eager, see above).

## Caveats

- Measurement taken against `/tmp/naozhi-test` (local build, debug log level) with an empty `$HOME=/tmp/naozhi-empty-home` to bypass the session-discovery startup stall caused by this repo's large `~/.claude/sessions/` backlog. Server behavior (static serving, gzip, manifest injection) is identical.
- Task 15 retained a fallback 1h cache header for any `/static/*` path that isn't in `dist/` — N/A here since all eager assets resolve through the hashed `dist/` path (verified: 404 for unhashed `/static/dist/js/chat.948770c7.js`, 200 + `immutable` for `/static/dist/js/views/chat.948770c7.js`).
- `Content-Encoding: gzip` confirmed on hashed JS (`HEAD` check).
- EC2 production is behind CloudFront; first-request latency for a new hash will be origin fetch + compress, then cached at the edge.

## Phase 3 follow-up (not in this phase)

To hit <50 KB JS first-paint, candidate work items:

1. **Shrink `legacy.js`** — move view-specific handlers (approvals actions, wiki editor, patrol runner) into their respective lazy view modules so the initial download drops from 28 KB toward ~5 KB of true cross-view glue.
2. **Split `views/chat.js`** — extract the Shiki-dependent renderer and event-log virtualization into a secondary chunk loaded after the initial message paints. Initial render only needs the input composer + WS hookup (~5-8 KB).
3. **Defer `legacy.js` past `DOMContentLoaded`** and measure whether the initial paint no longer blocks on it (it's already `defer`, but ordering vs module graph matters).

## Acceptance vs plan target

Per plan Task 16 guidance: "If over target: inspect legacy.js — the goal is to continue shrinking it in Phase 3." Phase 1 structural goal is met; the byte reduction is a Phase 3 task. Proceeding to Task 17 (EC2 deploy) with this baseline recorded.

---

## Phase 3 — legacy.js feature split (2026-04-19)

Measurement method: walked static ES6 `import`/`export ... from` chains
starting from `js/app.js`, plus `js/legacy.js` (included via plain
`<script>`). Dynamic `import()` calls (the 19 lazy feature shims) are
excluded — which is the whole point.

### First-paint JS gzip

| File | raw | gzip |
|---|---:|---:|
| js/app.js | 1,535 | 847 |
| js/core/api.js | 1,379 | 681 |
| js/core/html.js | 1,538 | 811 |
| js/core/router.js | 5,859 | 2,197 |
| js/core/state.js | 1,425 | 731 |
| js/core/utils.js | 5,471 | 2,570 |
| js/core/ws.js | 20,872 | 6,017 |
| js/legacy.js | 45,173 | 13,164 |
| js/views/chat.js | 94,885 | 25,177 |
| js/views/home.js | 16,501 | 4,399 |
| **Total** | **194,638** | **56,594 (55.3 KB)** |

### Delta vs Phase 1 baseline (70.1 KB)

- **Reduction: −14.8 KB gzip** (−21%).
- `legacy.js` shrank from 27.9 KB → 13.2 KB gzip (−14.7 KB).
- All 8 feature modules correctly absent from first-paint (`features/*.js`
  gzip reachable only via dynamic `import()`).

### Target status

- **Target:** < 50 KB.
- **Actual:** 55.3 KB.
- **Gap:** 5.3 KB over target.

### Why we missed

`chat.js` at 25.2 KB gzip is now the dominant contributor (45% of
first-paint bytes) — Phase 3 non-goal. A follow-up `chat.js` split
(deferring Shiki highlighter + event-log virtualization) is the next
lever; design spec §"Non-goals" explicitly deferred it.

### Decision

Phase 3 structural goal is met: legacy.js halved, all 8 features lazy.
Byte budget overage (5.3 KB) is a `views/chat.js` problem, not a
legacy.js problem. Accepting the result and proceeding to deploy.
Followup tracked as a Phase 4 item.

### Lazy chunks verified on disk (via manifest)

| Feature | Hashed path |
|---|---|
| context-panel | `js/features/context-panel.524c7e20.js` |
| notif-enhance | `js/features/notif-enhance.950bfac4.js` |
| bookmark | `js/features/bookmark.edeee16e.js` |
| twin | `js/features/twin.c7331f55.js` |
| replay | `js/features/replay.3460f1b8.js` |
| cron | `js/features/cron.af77f2ae.js` |
| file-hub | `js/features/file-hub.0204eb85.js` |
| search | `js/features/search.f48b0a19.js` |

Served by `handleStatic` under `/static/dist/` prefix → 1-year immutable
cache header (`public, max-age=31536000, immutable`), gzipped.
