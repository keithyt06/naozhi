// js/app.js — dashboard ES module entry point.
//
// Importing the core modules here is load-bearing: each of them
// installs its `window.*` bridges as a side effect of first evaluation,
// which is what the pre-split inline <script> block below relies on.
// Keep this file minimal — it is the bootstrap only.
//
// Task 15: only core/*, ws, and chat (plus home, imported transitively
// by chat.js) are eagerly shipped in the first-paint bundle. Every
// other view (knowledge / wiki / patrols / approvals / graph) is loaded
// on demand via the router's dynamic-import path — see core/router.js
// `ensureViewLoaded`. This keeps the initial JS payload small; the
// lazy chunks resolve through window.__resolveAsset so hashed URLs are
// picked up when the manifest is present.

import './core/html.js';
import './core/utils.js';
import './core/api.js';
import './core/state.js';
import './core/router.js';

// Task 9: WebSocket manager + chat view. Order matters — ws.js must
// load first because chat.js references `wsm` (via window bridge) for
// the send flow. Both are eagerly imported so the first-paint chat
// experience has no extra round trip. chat.js imports views/home.js
// statically (ESM), so the home renderer is bundled alongside chat
// without another network round trip.
import './core/ws.js';
import './views/chat.js';

// Mark bootstrap complete so the legacy inline script (and any future
// consumers) can detect readiness.
if (typeof window !== 'undefined') {
  window.__naozhiAppReady = true;
}
