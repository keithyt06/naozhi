package graph

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
)

// Handlers provides HTTP handlers for the Knowledge Graph API.
type Handlers struct {
	wikiDir string

	mu    sync.RWMutex
	cache *GraphData
}

// NewHandlers creates graph API handlers that read from the given wiki directory.
func NewHandlers(wikiDir string) *Handlers {
	return &Handlers{wikiDir: wikiDir}
}

// Refresh rebuilds the graph cache from the wiki directory.
// Safe to call concurrently; callers block until refresh completes.
func (h *Handlers) Refresh() error {
	g, err := ExtractFromWiki(h.wikiDir)
	if err != nil {
		return err
	}
	h.mu.Lock()
	h.cache = g
	h.mu.Unlock()
	return nil
}

// getGraph returns the cached graph, building it on first access.
func (h *Handlers) getGraph() (*GraphData, error) {
	h.mu.RLock()
	g := h.cache
	h.mu.RUnlock()
	if g != nil {
		return g, nil
	}
	// First access: build cache.
	if err := h.Refresh(); err != nil {
		return nil, err
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cache, nil
}

// HandleGraph returns the full graph data as JSON.
// GET /api/graph
func (h *Handlers) HandleGraph(w http.ResponseWriter, r *http.Request) {
	g, err := h.getGraph()
	if err != nil {
		writeErr(w, "failed to build graph: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeOK(w, g)
}

// HandleNodes returns nodes, optionally filtered by type query parameter.
// GET /api/graph/nodes?type=service
func (h *Handlers) HandleNodes(w http.ResponseWriter, r *http.Request) {
	g, err := h.getGraph()
	if err != nil {
		writeErr(w, "failed to build graph: "+err.Error(), http.StatusInternalServerError)
		return
	}
	typeFilter := r.URL.Query().Get("type")
	if typeFilter == "" {
		writeOK(w, g.Nodes)
		return
	}
	var filtered []Node
	for _, n := range g.Nodes {
		if n.Type == typeFilter {
			filtered = append(filtered, n)
		}
	}
	if filtered == nil {
		filtered = []Node{}
	}
	writeOK(w, filtered)
}

// HandleNodeDetail returns a single node with its connected edges.
// GET /api/graph/nodes/{id}
func (h *Handlers) HandleNodeDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/graph/nodes/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		writeErr(w, "node id required", http.StatusBadRequest)
		return
	}

	g, err := h.getGraph()
	if err != nil {
		writeErr(w, "failed to build graph: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Find the node.
	var found *Node
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			found = &g.Nodes[i]
			break
		}
	}
	if found == nil {
		writeErr(w, "node not found: "+id, http.StatusNotFound)
		return
	}

	// Collect connected edges and neighbor IDs.
	var connEdges []Edge
	neighbors := make(map[string]bool)
	for _, e := range g.Edges {
		if e.Source == id || e.Target == id {
			connEdges = append(connEdges, e)
			if e.Source == id {
				neighbors[e.Target] = true
			} else {
				neighbors[e.Source] = true
			}
		}
	}
	if connEdges == nil {
		connEdges = []Edge{}
	}

	neighborIDs := make([]string, 0, len(neighbors))
	for nid := range neighbors {
		neighborIDs = append(neighborIDs, nid)
	}

	writeOK(w, map[string]interface{}{
		"node":       found,
		"edges":      connEdges,
		"neighbors":  neighborIDs,
		"connection_count": len(connEdges),
	})
}

func writeOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
