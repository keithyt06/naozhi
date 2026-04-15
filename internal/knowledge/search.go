package knowledge

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
)

// SearchDocument represents a document to be indexed in the search engine.
type SearchDocument struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"` // "dashboard", "cli", "vault", "wiki", "bookmarks"
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	Tags      []string  `json:"tags,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Meta      string    `json:"meta,omitempty"` // extra metadata (path, session key, etc.)
}

// SearchResult represents a single search hit.
type SearchResult struct {
	Source string  `json:"source"`
	Title  string  `json:"title"`
	Match  string  `json:"match"`
	Score  float64 `json:"score"`
	Meta   string  `json:"meta,omitempty"`
	DocID  string  `json:"doc_id"`
}

// SearchEngine provides full-text search backed by a bleve index.
// The index is stored on disk at the configured path (e.g. ~/.naozhi/search.bleve).
type SearchEngine struct {
	mu    sync.RWMutex
	index bleve.Index
	path  string
}

// NewSearchEngine opens an existing bleve index at path, or creates a new one
// if the path does not exist. If the existing index is corrupted, it is removed
// and recreated. The index mapping uses:
//   - standard analyzer for title, text, tags (included in _all composite)
//   - keyword analyzer for source, meta (exact match, excluded from _all)
//   - datetime for timestamp
//
// All fields are stored for retrieval in search results.
func NewSearchEngine(path string) (*SearchEngine, error) {
	var idx bleve.Index
	var err error

	idx, err = bleve.Open(path)
	if err != nil {
		// Open failed: path does not exist, or index is corrupted.
		// Remove stale data if present, then create fresh.
		if _, statErr := os.Stat(path); statErr == nil {
			slog.Warn("removing stale search index", "path", path, "open_err", err)
			if removeErr := os.RemoveAll(path); removeErr != nil {
				return nil, fmt.Errorf("remove stale index at %s: %w", path, removeErr)
			}
		}
		idx, err = bleve.New(path, newSearchMapping())
		if err != nil {
			return nil, fmt.Errorf("create search index at %s: %w", path, err)
		}
	}

	return &SearchEngine{
		index: idx,
		path:  path,
	}, nil
}

// Close closes the bleve index, flushing any pending writes to disk.
// Must be called during graceful shutdown to prevent data loss.
func (se *SearchEngine) Close() error {
	se.mu.Lock()
	defer se.mu.Unlock()
	if se.index != nil {
		return se.index.Close()
	}
	return nil
}

// IndexDocument adds or updates a document in the search index.
// The document ID is used as the bleve document key.
func (se *SearchEngine) IndexDocument(doc SearchDocument) error {
	se.mu.Lock()
	defer se.mu.Unlock()
	return se.index.Index(doc.ID, doc)
}

// DeleteDocument removes a document from the search index by ID.
func (se *SearchEngine) DeleteDocument(id string) error {
	se.mu.Lock()
	defer se.mu.Unlock()
	return se.index.Delete(id)
}

// Search executes a full-text query across indexed documents.
// The query is matched against title, text, and tags fields.
//
// Parameters:
//   - query: search terms (empty returns nil)
//   - source: filter by source type; "all" or "" searches everything
//   - limit: max results (default 20, capped at 100)
//
// Results are sorted by BM25 relevance score descending.
func (se *SearchEngine) Search(query string, source string, limit int) ([]SearchResult, error) {
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	se.mu.RLock()
	defer se.mu.RUnlock()

	// Build per-field queries with boost weights matching the original
	// scoring strategy: title (3x), tags (2x), text (1x).
	titleQ := bleve.NewMatchQuery(query)
	titleQ.SetField("title")
	titleQ.SetBoost(3.0)

	tagsQ := bleve.NewMatchQuery(query)
	tagsQ.SetField("tags")
	tagsQ.SetBoost(2.0)

	textQ := bleve.NewMatchQuery(query)
	textQ.SetField("text")
	textQ.SetBoost(1.0)

	// Any field matching is sufficient (disjunction).
	mainQ := bleve.NewDisjunctionQuery(titleQ, tagsQ, textQ)

	var req *bleve.SearchRequest
	if source != "" && source != "all" {
		// Combine text search with source filter.
		sourceQ := bleve.NewTermQuery(source)
		sourceQ.SetField("source")
		req = bleve.NewSearchRequest(bleve.NewConjunctionQuery(mainQ, sourceQ))
	} else {
		req = bleve.NewSearchRequest(mainQ)
	}

	req.Size = limit
	req.Fields = []string{"source", "title", "text", "meta", "tags"}

	result, err := se.index.Search(req)
	if err != nil {
		return nil, fmt.Errorf("execute search: %w", err)
	}

	results := make([]SearchResult, 0, len(result.Hits))
	queryTerms := strings.Fields(strings.ToLower(query))

	for _, hit := range result.Hits {
		sr := SearchResult{
			Source: getStringField(hit.Fields, "source"),
			Title:  getStringField(hit.Fields, "title"),
			Score:  hit.Score,
			Meta:   getStringField(hit.Fields, "meta"),
			DocID:  hit.ID,
		}

		// Extract a match snippet from the full text (or title as fallback).
		text := getStringField(hit.Fields, "text")
		if len(queryTerms) > 0 {
			sr.Match = extractSnippet(text, queryTerms[0], 120)
			if sr.Match == "" {
				sr.Match = extractSnippet(sr.Title, queryTerms[0], 120)
			}
		}

		results = append(results, sr)
	}

	return results, nil
}

// DocumentCount returns the total number of indexed documents.
func (se *SearchEngine) DocumentCount() (uint64, error) {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return se.index.DocCount()
}

// DocCountBySource returns document counts grouped by source field,
// using a bleve faceted search on the source keyword field.
func (se *SearchEngine) DocCountBySource() (map[string]int, error) {
	se.mu.RLock()
	defer se.mu.RUnlock()

	req := bleve.NewSearchRequest(bleve.NewMatchAllQuery())
	req.Size = 0
	req.AddFacet("sources", bleve.NewFacetRequest("source", 10))

	result, err := se.index.Search(req)
	if err != nil {
		return nil, fmt.Errorf("facet search: %w", err)
	}

	counts := make(map[string]int)
	if f := result.Facets["sources"]; f != nil && f.Terms != nil {
		for _, tf := range f.Terms.Terms() {
			counts[tf.Term] = tf.Count
		}
	}
	return counts, nil
}

// --- Index mapping ---

// newSearchMapping builds the bleve index mapping for SearchDocument.
// Field configuration:
//   - title:     text, stored, in _all (default from NewTextFieldMapping)
//   - text:      text, stored, in _all
//   - tags:      text, stored, in _all
//   - source:    keyword, stored, NOT in _all (for exact filtering)
//   - meta:      keyword, stored, NOT in _all (stored for retrieval only)
//   - timestamp: datetime, stored, NOT in _all
func newSearchMapping() *mapping.IndexMappingImpl {
	im := bleve.NewIndexMapping()
	dm := bleve.NewDocumentMapping()

	// Text fields: stored + included in _all (defaults from NewTextFieldMapping).
	dm.AddFieldMappingsAt("title", bleve.NewTextFieldMapping())
	dm.AddFieldMappingsAt("text", bleve.NewTextFieldMapping())
	dm.AddFieldMappingsAt("tags", bleve.NewTextFieldMapping())

	// Source: keyword for exact-match filtering, excluded from _all
	// so that source values like "cli" don't pollute full-text search.
	sf := bleve.NewKeywordFieldMapping()
	sf.IncludeInAll = false
	dm.AddFieldMappingsAt("source", sf)

	// Meta: keyword, stored for retrieval in results but not searched.
	mf := bleve.NewKeywordFieldMapping()
	mf.IncludeInAll = false
	dm.AddFieldMappingsAt("meta", mf)

	// Timestamp: datetime, stored for retrieval but not in _all.
	tf := bleve.NewDateTimeFieldMapping()
	tf.IncludeInAll = false
	dm.AddFieldMappingsAt("timestamp", tf)

	im.DefaultMapping = dm
	return im
}

// --- Helpers ---

// getStringField extracts a string value from a bleve hit's stored fields map.
// For slice fields (e.g. tags), elements are joined with spaces.
func getStringField(fields map[string]interface{}, key string) string {
	if fields == nil {
		return ""
	}
	v, ok := fields[key]
	if !ok {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []interface{}:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	default:
		return fmt.Sprintf("%v", val)
	}
}

// extractSnippet returns a text fragment around the first occurrence of term,
// with configurable context length. Used to populate SearchResult.Match.
func extractSnippet(text, term string, maxLen int) string {
	if text == "" || term == "" {
		return ""
	}
	lower := strings.ToLower(text)
	termLower := strings.ToLower(term)
	idx := strings.Index(lower, termLower)
	if idx < 0 {
		return ""
	}

	start := idx - maxLen/2
	if start < 0 {
		start = 0
	}
	end := idx + len(term) + maxLen/2
	if end > len(text) {
		end = len(text)
	}

	snippet := text[start:end]
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(text) {
		snippet = snippet + "..."
	}
	return strings.TrimSpace(snippet)
}
