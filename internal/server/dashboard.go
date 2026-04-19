package server

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/naozhi/naozhi/internal/project"
	"github.com/naozhi/naozhi/internal/session"
)

// dashboardETag is computed once at init from the rendered HTML output
// (template + asset manifest), so any hash change busts the shell cache.
var (
	dashboardETag  string
	dashboardTmpl  *template.Template
	staticManifest map[string]string // logical path -> hashed path, e.g. "js/app.js" -> "js/app.abc12345.js"
	dashboardBody  []byte            // pre-rendered HTML (template output) for hot-path serving
)

//go:embed all:static
var staticFS embed.FS

// asset returns the URL for a logical static asset. When a hashed copy exists
// in the manifest we emit the immutable /static/dist/<hashed> URL; otherwise
// we fall back to the unhashed /static/<logical> path (dev mode, before
// `make static` has been run).
func asset(logical string) string {
	if h, ok := staticManifest[logical]; ok {
		return "/static/dist/" + h
	}
	return "/static/" + logical
}

func init() {
	// Load the asset manifest first so the template's asset helper can use it.
	if data, err := staticFS.ReadFile("static/dist/manifest.json"); err == nil {
		_ = json.Unmarshal(data, &staticManifest)
	} else {
		slog.Warn("static asset manifest missing; serving unhashed paths (run `make static` to enable immutable cache)",
			"err", err)
	}

	tmplBytes, err := staticFS.ReadFile("static/dashboard.html")
	if err != nil {
		return
	}
	dashboardTmpl = template.Must(template.New("dashboard").
		Funcs(template.FuncMap{"asset": asset}).
		Parse(string(tmplBytes)))

	var buf bytes.Buffer
	if err := dashboardTmpl.Execute(&buf, nil); err != nil {
		slog.Error("render dashboard template", "err", err)
		return
	}
	dashboardBody = buf.Bytes()

	// ETag covers both the template source and every manifest entry so any
	// rebuild with a changed asset hash invalidates the cached shell.
	h := sha256.New()
	h.Write(dashboardBody)
	keys := make([]string, 0, len(staticManifest))
	for k := range staticManifest {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(staticManifest[k]))
		h.Write([]byte{0})
	}
	sum := h.Sum(nil)
	dashboardETag = `"` + hex.EncodeToString(sum[:8]) + `"`
}

const authCookieName = "naozhi_auth"

// writeJSON encodes v as JSON to w. Logs errors at debug level since HTTP write
// failures are common after client disconnects, but JSON marshal failures indicate bugs.
func writeJSON(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Debug("write json response", "err", err)
	}
}

func (s *Server) registerDashboard() {
	s.hub = NewHub(HubOptions{
		Router:        s.router,
		Agents:        s.agents,
		AgentCmds:     s.agentCommands,
		DashToken:     s.dashboardToken,
		CookieMAC:     s.auth.cookieMAC(),
		Guard:         s.sessionGuard,
		Nodes:         s.nodes,
		NodesMu:       &s.nodesMu,
		ProjectMgr:    s.projectMgr,
		AllowedRoot:   s.allowedRoot,
		TrustedProxy:  s.auth.trustedProxy,
		WSAuthLimiter: s.auth.loginLimiterFor,
	})
	s.hub.SetScheduler(s.scheduler)

	// Wire sendH now that hub exists
	s.sendH = &SendHandler{nodeAccess: s.nodeAccess, hub: s.hub}

	// Push session list changes to WS clients
	s.router.SetOnChange(func() { s.hub.BroadcastSessionsUpdate() })

	// Push cron execution results to WS clients
	if s.scheduler != nil {
		s.scheduler.SetOnExecute(func(jobID, result, errMsg string) {
			s.hub.BroadcastCronResult(jobID, result, errMsg)
		})
	}

	// Authenticated API routes
	auth := s.auth.requireAuth
	s.mux.HandleFunc("GET /api/sessions", auth(s.sessionH.handleList))
	s.mux.HandleFunc("GET /api/sessions/events", auth(s.sessionH.handleEvents))
	s.mux.HandleFunc("POST /api/sessions/send", auth(s.sendH.handleSend))
	s.mux.HandleFunc("DELETE /api/sessions", auth(s.sessionH.handleDelete))
	s.mux.HandleFunc("POST /api/sessions/resume", auth(s.sessionH.handleResume))
	s.mux.HandleFunc("GET /api/discovered", auth(s.discoveryH.handleList))
	s.mux.HandleFunc("GET /api/discovered/preview", auth(s.discoveryH.handlePreview))
	s.mux.HandleFunc("POST /api/discovered/takeover", auth(s.discoveryH.handleTakeover))
	s.mux.HandleFunc("GET /api/projects", auth(s.projectH.handleList))
	s.mux.HandleFunc("GET /api/projects/config", auth(s.projectH.handleConfigGet))
	s.mux.HandleFunc("PUT /api/projects/config", auth(s.projectH.handleConfigPut))
	s.mux.HandleFunc("POST /api/projects/planner/restart", auth(s.projectH.handlePlannerRestart))
	s.mux.HandleFunc("POST /api/transcribe", auth(s.transcribeH.handleTranscribe))
	s.mux.HandleFunc("GET /api/cron", auth(s.cronH.handleList))
	s.mux.HandleFunc("POST /api/cron", auth(s.cronH.handleCreate))
	s.mux.HandleFunc("DELETE /api/cron", auth(s.cronH.handleDelete))
	s.mux.HandleFunc("POST /api/cron/pause", auth(s.cronH.handlePause))
	s.mux.HandleFunc("POST /api/cron/resume", auth(s.cronH.handleResume))
	s.mux.HandleFunc("GET /api/cron/preview", auth(s.cronH.handlePreview))

	// Patrol API
	if s.patrolH != nil {
		s.patrolH.RegisterRoutes(s.mux, auth)
		s.patrolH.RegisterWebhookRoutes(s.mux) // webhooks are unauthenticated
		s.patrolH.RegisterAlertRoutes(s.mux)   // alert webhooks are unauthenticated
		if s.patrolMgr != nil {
			s.patrolMgr.SetHub(s.hub)
		}
	}

	// Approval API
	if s.approvalH != nil {
		s.approvalH.Hub = s.hub
		s.approvalH.RegisterApprovalRoutes(s.mux, auth)
	}

	// Notification API
	if s.notifH != nil {
		s.notifH.Hub = s.hub
		s.notifH.RegisterNotificationRoutes(s.mux, auth)
	}

	s.mux.HandleFunc("POST /api/auth/logout", auth(s.auth.handleLogout))

	// Session naming & pinning
	s.mux.HandleFunc("PATCH /api/sessions/rename", auth(s.hub.handleRename))
	s.mux.HandleFunc("PATCH /api/sessions/pin", auth(s.hub.handlePin))

	// Knowledge API
	if s.knowledgeH != nil {
		s.mux.HandleFunc("GET /api/vault/tree", auth(s.knowledgeH.handleVaultTree))
		s.mux.HandleFunc("GET /api/vault/read", auth(s.knowledgeH.handleVaultRead))
		s.mux.HandleFunc("GET /api/vault/raw", auth(s.knowledgeH.handleVaultRaw))
		s.mux.HandleFunc("GET /api/wiki", auth(s.knowledgeH.handleWikiList))
		s.mux.HandleFunc("GET /api/wiki/", auth(s.knowledgeH.handleWikiRead))
		s.mux.HandleFunc("POST /api/wiki/ingest", auth(s.knowledgeH.handleWikiIngest))
		s.mux.HandleFunc("POST /api/wiki/lint", auth(s.knowledgeH.handleWikiLint))
		s.mux.HandleFunc("GET /api/wiki/lint", auth(s.knowledgeH.handleWikiLintResult))
		s.mux.HandleFunc("GET /api/search", auth(s.knowledgeH.handleSearch))
		s.mux.HandleFunc("GET /api/search/stats", auth(s.knowledgeH.handleSearchStats))
		s.mux.HandleFunc("GET /api/bookmarks", auth(s.knowledgeH.handleBookmarkList))
		s.mux.HandleFunc("POST /api/bookmarks", auth(s.knowledgeH.handleBookmarkCreate))
		s.mux.HandleFunc("DELETE /api/bookmarks/", auth(s.knowledgeH.handleBookmarkDelete))
		s.mux.HandleFunc("GET /api/decisions", auth(s.knowledgeH.handleDecisionList))
		s.mux.HandleFunc("POST /api/decisions", auth(s.knowledgeH.handleDecisionCreate))
		s.mux.HandleFunc("GET /api/decisions/", auth(s.knowledgeH.handleDecisionGet))
	}

	// Graph API (Knowledge Graph)
	if s.graphH != nil {
		s.mux.HandleFunc("GET /api/graph", auth(s.graphH.HandleGraph))
		s.mux.HandleFunc("GET /api/graph/nodes", auth(s.graphH.HandleNodes))
		s.mux.HandleFunc("GET /api/graph/nodes/", auth(s.graphH.HandleNodeDetail))
	}

	// Replay API (Session Replay & Sharing)
	if s.replayH != nil {
		s.mux.HandleFunc("GET /api/sessions/replay", auth(s.replayH.handleReplay))
		s.mux.HandleFunc("POST /api/sessions/share", auth(s.replayH.handleShare))
	}

	// Twin API (CTO Digital Twin)
	if s.twinH != nil {
		s.mux.HandleFunc("GET /api/twin/config", auth(s.twinH.handleTwinConfigGet))
		s.mux.HandleFunc("PUT /api/twin/config", auth(s.twinH.handleTwinConfigPut))
		s.mux.HandleFunc("POST /api/twin/test", auth(s.twinH.handleTwinTest))
		s.mux.HandleFunc("GET /api/twin/queue", auth(s.twinH.handleTwinQueue))
		s.mux.HandleFunc("POST /api/twin/queue/dismiss", auth(s.twinH.handleTwinDismiss))
	}

	// File Hub API
	s.mux.HandleFunc("GET /api/files/list", auth(s.filesH.handleList))
	s.mux.HandleFunc("GET /api/files/stat", auth(s.filesH.handleStat))
	s.mux.HandleFunc("POST /api/files/upload", auth(s.filesH.handleUpload))
	s.mux.HandleFunc("GET /api/files/download", auth(s.filesH.handleDownload))
	s.mux.HandleFunc("POST /api/files/mkdir", auth(s.filesH.handleMkdir))
	s.mux.HandleFunc("DELETE /api/files/delete", auth(s.filesH.handleDelete))

	// Unauthenticated routes (login, static assets, WebSocket with own auth)
	if s.replayH != nil {
		s.mux.HandleFunc("GET /api/shared/", s.replayH.handleSharedReplay) // public, no auth
	}
	s.mux.HandleFunc("POST /api/auth/login", s.auth.handleLogin)
	s.mux.HandleFunc("GET /dashboard", s.handleDashboard)
	s.mux.HandleFunc("GET /manifest.json", s.handleManifest)
	s.mux.HandleFunc("GET /sw.js", s.handleSW)
	s.mux.HandleFunc("GET /static/", s.handleStatic)
	s.mux.HandleFunc("GET /ws", s.hub.HandleUpgrade)
	if s.reverseNodeServer != nil {
		s.mux.Handle("GET /ws-node", s.reverseNodeServer)
	}
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if s.dashboardToken != "" && !s.auth.isAuthenticated(r) {
		s.auth.serveLoginPage(w)
		return
	}
	data := dashboardBody
	if len(data) == 0 {
		// Template render failed at init; fall back to raw embedded HTML
		// so the dashboard still loads (unhashed asset URLs).
		raw, err := staticFS.ReadFile("static/dashboard.html")
		if err != nil {
			http.Error(w, "dashboard not found", http.StatusNotFound)
			return
		}
		data = raw
	}
	// ETag-based caching: browser must revalidate every time, but gets
	// a 304 if the content hasn't changed. This ensures code fixes are
	// picked up immediately while still allowing conditional caching.
	if dashboardETag != "" {
		if match := r.Header.Get("If-None-Match"); match == dashboardETag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", dashboardETag)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
	// TODO: 'unsafe-inline' in script-src weakens XSS protection. Moving inline
	// JS to static/dashboard.js would allow removing it (nonce or 'self' only).
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://esm.sh https://esm.run; connect-src 'self' wss: ws: https://esm.sh https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; font-src 'self' https://cdn.jsdelivr.net; img-src 'self' data: blob:; worker-src 'self' blob:; manifest-src 'self' https://*.amazoncognito.com")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")

	// Gzip compress the response if the client supports it.
	// The embedded dashboard HTML is ~370KB; gzip reduces it to ~100KB.
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Del("Content-Length")
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		if _, err := gz.Write(data); err != nil {
			slog.Debug("dashboard gzip write", "err", err)
		}
		return
	}
	if _, err := w.Write(data); err != nil {
		slog.Debug("dashboard write", "err", err)
	}
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	data, err := staticFS.ReadFile("static/manifest.json")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "max-age=3600")
	if _, err := w.Write(data); err != nil {
		slog.Debug("manifest write", "err", err)
	}
}

func (s *Server) handleSW(w http.ResponseWriter, r *http.Request) {
	data, err := staticFS.ReadFile("static/sw.js")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Service-Worker-Allowed", "/")
	if _, err := w.Write(data); err != nil {
		slog.Debug("sw write", "err", err)
	}
}

// handleStatic serves files from internal/server/static under /static/*.
// Files under /static/dist/ are content-hashed (see tools/hashstatic) and
// served with a 1-year immutable cache. Other /static/ paths remain as a
// dev-mode fallback with a 1h cache.
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
	if strings.HasPrefix(path, "dist/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
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

// strOrFallback extracts a string from a map, trying the primary key first then the fallback.
// Used to handle remote nodes that may send Go-default JSON keys (e.g. "Name") instead of
// tagged lowercase keys (e.g. "name").
func strOrFallback(m map[string]any, key, fallback string) string {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	v, _ := m[fallback].(string)
	return v
}

// buildSessionOpts resolves agent config and planner overrides for a session key.
func buildSessionOpts(key string, agents map[string]session.AgentOpts, projectMgr *project.Manager) session.AgentOpts {
	parts := strings.SplitN(key, ":", 4)
	agentID := "general"
	if len(parts) == 4 {
		agentID = parts[3]
	}

	opts := agents[agentID]
	if project.IsPlannerKey(key) {
		opts.Exempt = true
		if projectMgr != nil {
			pParts := strings.SplitN(key, ":", 3)
			if len(pParts) == 3 {
				if p := projectMgr.Get(pParts[1]); p != nil {
					opts.Workspace = p.Path
					if m := projectMgr.EffectivePlannerModel(p); m != "" {
						opts.Model = m
					}
					if prompt := projectMgr.EffectivePlannerPrompt(p); prompt != "" {
						opts.ExtraArgs = append(opts.ExtraArgs[:len(opts.ExtraArgs):len(opts.ExtraArgs)],
							"--append-system-prompt", prompt)
					}
				}
			}
		}
	}
	return opts
}
