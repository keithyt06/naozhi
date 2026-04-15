package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/naozhi/naozhi/internal/knowledge"
)

// KnowledgeHandlers holds handlers for knowledge-related API endpoints.
type KnowledgeHandlers struct {
	vault     *knowledge.Vault
	wiki      *knowledge.WikiManager
	bookmarks *knowledge.BookmarkStore
	search    *knowledge.SearchEngine
	ingest    *knowledge.IngestEngine
	lint      *knowledge.LintEngine
	cliSync   *knowledge.CLISyncManager
}

// NewKnowledgeHandlers creates a new KnowledgeHandlers instance.
func NewKnowledgeHandlers(vault *knowledge.Vault, wiki *knowledge.WikiManager, bookmarks *knowledge.BookmarkStore, search *knowledge.SearchEngine) *KnowledgeHandlers {
	return &KnowledgeHandlers{
		vault:     vault,
		wiki:      wiki,
		bookmarks: bookmarks,
		search:    search,
	}
}

// --- Vault API ---

func (kh *KnowledgeHandlers) handleVaultTree(w http.ResponseWriter, r *http.Request) {
	if kh.vault == nil || !kh.vault.Configured() {
		writeJSONStatus(w, map[string]string{"error": "vault not configured"}, http.StatusServiceUnavailable)
		return
	}
	tree, err := kh.vault.BuildTree()
	if err != nil {
		writeJSONStatus(w, map[string]string{"error": err.Error()}, http.StatusInternalServerError)
		return
	}
	writeJSONStatus(w, tree, http.StatusOK)
}

func (kh *KnowledgeHandlers) handleVaultRead(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeJSONStatus(w, map[string]string{"error": "path required"}, http.StatusBadRequest)
		return
	}
	if kh.vault == nil || !kh.vault.Configured() {
		writeJSONStatus(w, map[string]string{"error": "vault not configured"}, http.StatusServiceUnavailable)
		return
	}
	html, fm, err := kh.vault.RenderFile(path)
	if err != nil {
		writeJSONStatus(w, map[string]string{"error": err.Error()}, http.StatusNotFound)
		return
	}
	writeJSONStatus(w, map[string]interface{}{
		"html":        html,
		"frontmatter": fm,
		"path":        path,
	}, http.StatusOK)
}

func (kh *KnowledgeHandlers) handleVaultRaw(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeJSONStatus(w, map[string]string{"error": "path required"}, http.StatusBadRequest)
		return
	}
	if kh.vault == nil || !kh.vault.Configured() {
		writeJSONStatus(w, map[string]string{"error": "vault not configured"}, http.StatusServiceUnavailable)
		return
	}
	data, err := kh.vault.ReadFile(path)
	if err != nil {
		writeJSONStatus(w, map[string]string{"error": err.Error()}, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Write(data)
}

// --- Wiki API ---

func (kh *KnowledgeHandlers) handleWikiList(w http.ResponseWriter, r *http.Request) {
	if kh.wiki == nil {
		writeJSONStatus(w, []struct{}{}, http.StatusOK)
		return
	}
	pages, err := kh.wiki.ListPages()
	if err != nil {
		writeJSONStatus(w, map[string]string{"error": err.Error()}, http.StatusInternalServerError)
		return
	}
	if pages == nil {
		pages = []knowledge.WikiPage{}
	}
	writeJSONStatus(w, pages, http.StatusOK)
}

func (kh *KnowledgeHandlers) handleWikiRead(w http.ResponseWriter, r *http.Request) {
	// Extract name from path: /api/wiki/{name}
	name := strings.TrimPrefix(r.URL.Path, "/api/wiki/")
	name = strings.TrimSuffix(name, "/")
	if name == "" {
		writeJSONStatus(w, map[string]string{"error": "name required"}, http.StatusBadRequest)
		return
	}
	if kh.wiki == nil {
		writeJSONStatus(w, map[string]string{"error": "wiki not configured"}, http.StatusServiceUnavailable)
		return
	}
	page, err := kh.wiki.ReadPage(name)
	if err != nil {
		writeJSONStatus(w, map[string]string{"error": err.Error()}, http.StatusNotFound)
		return
	}
	// Render wiki markdown content via vault renderer
	if kh.vault != nil {
		html, fm, renderErr := kh.vault.Render([]byte(page.Content))
		if renderErr == nil {
			writeJSONStatus(w, map[string]interface{}{
				"page":        page,
				"html":        html,
				"frontmatter": fm,
			}, http.StatusOK)
			return
		}
	}
	writeJSONStatus(w, page, http.StatusOK)
}

func (kh *KnowledgeHandlers) handleWikiIngest(w http.ResponseWriter, r *http.Request) {
	if kh.ingest == nil {
		writeJSONStatus(w, map[string]string{"error": "ingest engine not configured"}, http.StatusServiceUnavailable)
		return
	}

	// Run ingest from vault and wiki in a goroutine to avoid blocking.
	go func() {
		if err := kh.ingest.IngestFromVault(); err != nil {
			slog.Warn("ingest from vault", "err", err)
		}
		if err := kh.ingest.IngestWikiPages(); err != nil {
			slog.Warn("ingest wiki pages", "err", err)
		}
		if kh.bookmarks != nil {
			kh.ingest.IndexBookmarks(kh.bookmarks.List())
		}
	}()

	writeJSONStatus(w, map[string]string{"status": "accepted", "message": "ingest started"}, http.StatusAccepted)
}

func (kh *KnowledgeHandlers) handleWikiLint(w http.ResponseWriter, r *http.Request) {
	if kh.lint == nil {
		writeJSONStatus(w, map[string]string{"error": "lint engine not configured"}, http.StatusServiceUnavailable)
		return
	}

	result := kh.lint.RunLint()
	writeJSONStatus(w, result, http.StatusOK)
}

func (kh *KnowledgeHandlers) handleWikiLintResult(w http.ResponseWriter, r *http.Request) {
	if kh.lint == nil {
		writeJSONStatus(w, map[string]string{"error": "lint engine not configured"}, http.StatusServiceUnavailable)
		return
	}

	last := kh.lint.LastResult()
	if last == nil {
		writeJSONStatus(w, map[string]string{"status": "no results", "message": "lint has not been run yet"}, http.StatusOK)
		return
	}
	writeJSONStatus(w, last, http.StatusOK)
}

// --- Search API ---

func (kh *KnowledgeHandlers) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	source := r.URL.Query().Get("source")
	if source == "" {
		source = "all"
	}
	if query == "" {
		writeJSONStatus(w, map[string]interface{}{"results": []struct{}{}}, http.StatusOK)
		return
	}
	if kh.search == nil {
		writeJSONStatus(w, map[string]interface{}{"results": []struct{}{}}, http.StatusOK)
		return
	}
	results := kh.search.Search(query, source, 20)
	if results == nil {
		results = []knowledge.SearchResult{}
	}
	writeJSONStatus(w, map[string]interface{}{
		"results": results,
		"count":   len(results),
	}, http.StatusOK)
}

// --- Bookmark API ---

func (kh *KnowledgeHandlers) handleBookmarkList(w http.ResponseWriter, r *http.Request) {
	session := r.URL.Query().Get("session")
	query := r.URL.Query().Get("q")

	if kh.bookmarks == nil {
		writeJSONStatus(w, []struct{}{}, http.StatusOK)
		return
	}

	var bms []knowledge.Bookmark
	if query != "" {
		bms = kh.bookmarks.Search(query)
	} else if session != "" {
		bms = kh.bookmarks.BySession(session)
	} else {
		bms = kh.bookmarks.List()
	}
	if bms == nil {
		bms = []knowledge.Bookmark{}
	}
	writeJSONStatus(w, bms, http.StatusOK)
}

func (kh *KnowledgeHandlers) handleBookmarkCreate(w http.ResponseWriter, r *http.Request) {
	if kh.bookmarks == nil {
		writeJSONStatus(w, map[string]string{"error": "bookmarks not configured"}, http.StatusServiceUnavailable)
		return
	}
	var bm knowledge.Bookmark
	if err := json.NewDecoder(r.Body).Decode(&bm); err != nil {
		writeJSONStatus(w, map[string]string{"error": "invalid json"}, http.StatusBadRequest)
		return
	}
	if err := kh.bookmarks.Add(bm); err != nil {
		writeJSONStatus(w, map[string]string{"error": err.Error()}, http.StatusInternalServerError)
		return
	}
	writeJSONStatus(w, bm, http.StatusCreated)
}

func (kh *KnowledgeHandlers) handleBookmarkDelete(w http.ResponseWriter, r *http.Request) {
	if kh.bookmarks == nil {
		writeJSONStatus(w, map[string]string{"error": "bookmarks not configured"}, http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/bookmarks/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		writeJSONStatus(w, map[string]string{"error": "id required"}, http.StatusBadRequest)
		return
	}
	if err := kh.bookmarks.Remove(id); err != nil {
		writeJSONStatus(w, map[string]string{"error": err.Error()}, http.StatusNotFound)
		return
	}
	writeJSONStatus(w, map[string]string{"status": "deleted"}, http.StatusOK)
}

// --- Search Stats API ---

func (kh *KnowledgeHandlers) handleSearchStats(w http.ResponseWriter, r *http.Request) {
	if kh.search == nil {
		writeJSONStatus(w, map[string]interface{}{
			"total":      0,
			"by_source":  map[string]int{},
		}, http.StatusOK)
		return
	}
	writeJSONStatus(w, map[string]interface{}{
		"total":     kh.search.DocumentCount(),
		"by_source": kh.search.DocCountBySource(),
	}, http.StatusOK)
}

func writeJSONStatus(w http.ResponseWriter, v interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
