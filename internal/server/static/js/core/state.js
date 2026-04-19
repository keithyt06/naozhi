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
