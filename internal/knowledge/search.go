package knowledge

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// SearchDocument represents a document in the search index.
type SearchDocument struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"` // "dashboard", "cli", "vault", "wiki", "bookmarks"
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	Tags      []string  `json:"tags,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Meta      string    `json:"meta,omitempty"` // extra metadata (path, session key, etc.)
}

// SearchResult represents a search hit.
type SearchResult struct {
	Source string  `json:"source"`
	Title  string  `json:"title"`
	Match  string  `json:"match"`
	Score  float64 `json:"score"`
	Meta   string  `json:"meta,omitempty"`
	DocID  string  `json:"doc_id"`
}

// SearchEngine provides full-text search over indexed documents.
// This is a lightweight in-memory implementation. Can be replaced with bleve later.
type SearchEngine struct {
	mu   sync.RWMutex
	docs map[string]SearchDocument
}

// NewSearchEngine creates a new in-memory search engine.
func NewSearchEngine() *SearchEngine {
	return &SearchEngine{
		docs: make(map[string]SearchDocument),
	}
}

// IndexDocument adds or updates a document in the index.
func (se *SearchEngine) IndexDocument(doc SearchDocument) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.docs[doc.ID] = doc
}

// DeleteDocument removes a document from the index.
func (se *SearchEngine) DeleteDocument(id string) {
	se.mu.Lock()
	defer se.mu.Unlock()
	delete(se.docs, id)
}

// Search performs a text search across indexed documents.
func (se *SearchEngine) Search(query string, source string, limit int) []SearchResult {
	if query == "" {
		return nil
	}
	if limit <= 0 {
		limit = 20
	}

	se.mu.RLock()
	defer se.mu.RUnlock()

	queryLower := strings.ToLower(query)
	terms := strings.Fields(queryLower)
	if len(terms) == 0 {
		return nil
	}

	var results []SearchResult
	for _, doc := range se.docs {
		if source != "" && source != "all" && doc.Source != source {
			continue
		}

		score := scoreDocument(doc, terms)
		if score <= 0 {
			continue
		}

		match := extractMatch(doc.Text, terms[0], 80)
		if match == "" {
			match = extractMatch(doc.Title, terms[0], 80)
		}

		results = append(results, SearchResult{
			Source: doc.Source,
			Title:  doc.Title,
			Match:  match,
			Score:  score,
			Meta:   doc.Meta,
			DocID:  doc.ID,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

// DocumentCount returns the number of indexed documents.
func (se *SearchEngine) DocumentCount() int {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return len(se.docs)
}

// scoreDocument computes a relevance score for a document against query terms.
func scoreDocument(doc SearchDocument, terms []string) float64 {
	score := 0.0
	titleLower := strings.ToLower(doc.Title)
	textLower := strings.ToLower(doc.Text)
	tagsLower := strings.ToLower(strings.Join(doc.Tags, " "))

	for _, term := range terms {
		termScore := 0.0
		// Title match (highest weight)
		if strings.Contains(titleLower, term) {
			termScore += 3.0
		}
		// Tag match
		if strings.Contains(tagsLower, term) {
			termScore += 2.0
		}
		// Text match
		if strings.Contains(textLower, term) {
			termScore += 1.0
			// Bonus for multiple occurrences
			count := strings.Count(textLower, term)
			if count > 1 {
				termScore += float64(min(count-1, 5)) * 0.2
			}
		}
		if termScore == 0 {
			return 0 // all terms must match
		}
		score += termScore
	}

	// Recency bonus: newer documents score slightly higher
	age := time.Since(doc.Timestamp).Hours()
	if age < 24 {
		score *= 1.2
	} else if age < 168 { // 1 week
		score *= 1.1
	}

	return score
}

// extractMatch returns a snippet of text around the first occurrence of term.
func extractMatch(text, term string, maxLen int) string {
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
