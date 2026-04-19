// js/app.js — dashboard ES module entry point.
//
// Importing the core modules here is load-bearing: each of them
// installs its `window.*` bridges as a side effect of first evaluation,
// which is what the pre-split inline <script> block below relies on.
// Keep this file minimal — it is the bootstrap only. View modules
// (Tasks 8-13) will add their own imports here as they are extracted.

import './core/html.js';
import './core/utils.js';
import './core/api.js';
import './core/state.js';
import './core/router.js';

// Home is the first-paint view (rendered when no session is
// selected) — load eagerly so the landing page has no jank.
import './views/home.js';

// Task 9: WebSocket manager + chat view. Order matters — ws.js must
// load first because chat.js references `wsm` (via window bridge) for
// the send flow. Both are eagerly imported so the first-paint chat
// experience has no extra round trip.
import './core/ws.js';
import './views/chat.js';

// Task 10-12: non-chat view modules. Eager-imported so their window.*
// bridges are installed before the legacy bootstrap IIFE runs.
import './views/knowledge.js';
import './views/wiki.js';
import './views/patrols.js';

// Mark bootstrap complete so the legacy inline script (and any future
// consumers) can detect readiness.
if (typeof window !== 'undefined') {
  window.__naozhiAppReady = true;
}
