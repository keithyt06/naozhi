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
