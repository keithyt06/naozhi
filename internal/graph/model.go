package graph

// Node represents an entity in the knowledge graph.
type Node struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Type     string `json:"type"` // hub, service, project, infra, security
	Size     int    `json:"size"` // proportional to number of connections
	Color    string `json:"color"`
	WikiPage string `json:"wiki_page,omitempty"` // associated wiki page name
}

// Edge represents a relationship between two nodes.
type Edge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Weight int    `json:"weight"` // number of times this link appears
	Label  string `json:"label"`  // relationship description
}

// GraphData holds the complete knowledge graph.
type GraphData struct {
	Nodes []Node     `json:"nodes"`
	Edges []Edge     `json:"edges"`
	Stats GraphStats `json:"stats"`
}

// GraphStats provides summary counts for the graph.
type GraphStats struct {
	TotalNodes int `json:"total_nodes"`
	TotalEdges int `json:"total_edges"`
}

// NodeType constants for knowledge graph node classification.
const (
	NodeTypeHub      = "hub"
	NodeTypeService  = "service"
	NodeTypeProject  = "project"
	NodeTypeInfra    = "infra"
	NodeTypeSecurity = "security"
)

// nodeColors maps node types to their display colors.
var nodeColors = map[string]string{
	NodeTypeHub:      "#8B5CF6", // purple
	NodeTypeService:  "#3B82F6", // blue
	NodeTypeProject:  "#10B981", // green
	NodeTypeInfra:    "#F59E0B", // yellow
	NodeTypeSecurity: "#EF4444", // red
}

// ColorForType returns the hex color for a given node type.
// Returns a default grey if the type is unknown.
func ColorForType(nodeType string) string {
	if c, ok := nodeColors[nodeType]; ok {
		return c
	}
	return "#6B7280"
}
