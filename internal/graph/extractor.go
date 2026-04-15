package graph

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// wikiLinkRe matches [[wikilinks]] in markdown content.
var wikiLinkRe = regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]+)?\]\]`)

// serviceKeywords maps keywords to node types for classification.
var serviceKeywords = map[string][]string{
	NodeTypeService: {
		"aws", "eks", "ecs", "cloudfront", "s3", "lambda", "rds",
		"dynamodb", "sqs", "sns", "iam", "vpc", "ec2", "bedrock",
		"cloudwatch", "route53", "alb", "elb", "api gateway",
		"kinesis", "glue", "athena", "redshift", "aurora",
		"codepipeline", "codebuild", "codedeploy",
	},
	NodeTypeInfra: {
		"terraform", "cdk", "cloudformation", "docker", "kubernetes",
		"helm", "ansible", "jenkins", "cicd", "ci/cd", "pipeline",
		"deploy", "infra", "infrastructure", "monitoring", "observability",
	},
	NodeTypeSecurity: {
		"security", "waf", "firewall", "iam", "rbac", "encryption",
		"ssl", "tls", "certificate", "vulnerability", "compliance",
		"audit", "guard", "guardduty", "securityhub", "shield",
	},
	NodeTypeProject: {
		"project", "customer", "client", "poc", "demo",
		"migration", "tap", "engagement",
	},
}

// frontmatter is a minimal struct for parsing wiki page YAML frontmatter.
type frontmatter struct {
	Title    string   `yaml:"title"`
	Entities []string `yaml:"entities"`
}

// ExtractFromWiki scans all .md files in wikiDir and builds a GraphData.
// It extracts entities from frontmatter and [[wikilinks]] from content.
func ExtractFromWiki(wikiDir string) (*GraphData, error) {
	entries, err := os.ReadDir(wikiDir)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyGraph(), nil
		}
		return nil, err
	}

	nodeMap := make(map[string]*Node)   // id -> node
	edgeMap := make(map[string]*Edge)   // "src->tgt" -> edge
	connCount := make(map[string]int)   // node id -> connection count

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "CLAUDE.md" {
			continue
		}
		pageName := strings.TrimSuffix(e.Name(), ".md")
		data, readErr := os.ReadFile(filepath.Join(wikiDir, e.Name()))
		if readErr != nil {
			continue
		}

		fm, content := parseFM(data)

		// Create a node for this wiki page itself.
		pageID := normalizeID(pageName)
		if _, exists := nodeMap[pageID]; !exists {
			nodeType := inferType(pageName, content)
			nodeMap[pageID] = &Node{
				ID:       pageID,
				Label:    pageLabel(pageName, fm.Title),
				Type:     nodeType,
				Color:    ColorForType(nodeType),
				WikiPage: pageName,
			}
		}

		// Extract entity nodes from frontmatter.
		for _, entity := range fm.Entities {
			entID := normalizeID(entity)
			if _, exists := nodeMap[entID]; !exists {
				entType := inferTypeFromName(entity)
				nodeMap[entID] = &Node{
					ID:    entID,
					Label: entity,
					Type:  entType,
					Color: ColorForType(entType),
				}
			}
			// Create edge from page to entity.
			addEdge(edgeMap, connCount, pageID, entID, "contains")
		}

		// Extract [[wikilinks]] from content.
		matches := wikiLinkRe.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			linkTarget := strings.TrimSpace(m[1])
			if linkTarget == "" {
				continue
			}
			targetID := normalizeID(linkTarget)
			if _, exists := nodeMap[targetID]; !exists {
				tType := inferTypeFromName(linkTarget)
				nodeMap[targetID] = &Node{
					ID:    targetID,
					Label: linkTarget,
					Type:  tType,
					Color: ColorForType(tType),
				}
			}
			addEdge(edgeMap, connCount, pageID, targetID, "links")
		}
	}

	// Set node sizes from connection counts.
	for id, n := range nodeMap {
		n.Size = connCount[id]
	}

	// Build result slices.
	nodes := make([]Node, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, *n)
	}
	edges := make([]Edge, 0, len(edgeMap))
	for _, e := range edgeMap {
		edges = append(edges, *e)
	}

	return &GraphData{
		Nodes: nodes,
		Edges: edges,
		Stats: GraphStats{
			TotalNodes: len(nodes),
			TotalEdges: len(edges),
		},
	}, nil
}

// addEdge creates or increments an edge between src and tgt.
func addEdge(edgeMap map[string]*Edge, connCount map[string]int, src, tgt, label string) {
	if src == tgt {
		return // skip self-links
	}
	key := src + "->" + tgt
	reverseKey := tgt + "->" + src
	// Check if reverse edge exists to avoid duplicates in undirected sense.
	if e, ok := edgeMap[reverseKey]; ok {
		e.Weight++
		connCount[src]++
		connCount[tgt]++
		return
	}
	if e, ok := edgeMap[key]; ok {
		e.Weight++
	} else {
		edgeMap[key] = &Edge{
			Source: src,
			Target: tgt,
			Weight: 1,
			Label:  label,
		}
	}
	connCount[src]++
	connCount[tgt]++
}

// normalizeID converts a string to a stable node ID (lowercase, hyphens).
func normalizeID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	return s
}

// pageLabel returns the display label for a wiki page node.
func pageLabel(pageName, fmTitle string) string {
	if fmTitle != "" {
		return fmTitle
	}
	return pageName
}

// inferType determines node type from page name and content.
func inferType(pageName, content string) string {
	combined := strings.ToLower(pageName + " " + content)
	return classifyText(combined)
}

// inferTypeFromName determines node type from entity name alone.
func inferTypeFromName(name string) string {
	return classifyText(strings.ToLower(name))
}

// classifyText scores text against keyword lists and returns the best-matching type.
func classifyText(text string) string {
	bestType := NodeTypeHub
	bestScore := 0
	for nodeType, keywords := range serviceKeywords {
		score := 0
		for _, kw := range keywords {
			if strings.Contains(text, kw) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestType = nodeType
		}
	}
	return bestType
}

// parseFM extracts YAML frontmatter and body content from raw markdown bytes.
func parseFM(data []byte) (frontmatter, string) {
	var fm frontmatter
	s := string(data)
	if !strings.HasPrefix(s, "---\n") {
		return fm, s
	}
	end := strings.Index(s[4:], "\n---")
	if end < 0 {
		return fm, s
	}
	_ = yaml.Unmarshal([]byte(s[4:4+end]), &fm)
	content := strings.TrimSpace(s[4+end+4:])
	return fm, content
}

// emptyGraph returns a valid but empty GraphData.
func emptyGraph() *GraphData {
	return &GraphData{
		Nodes: []Node{},
		Edges: []Edge{},
		Stats: GraphStats{},
	}
}
