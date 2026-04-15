package knowledge

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TwinConfig holds configuration for the CTO Digital Twin.
type TwinConfig struct {
	Enabled         bool     `json:"enabled" yaml:"enabled"`
	Name            string   `json:"name" yaml:"name"`                         // CTO name, e.g. "Keith"
	Role            string   `json:"role" yaml:"role"`                         // e.g. "AWS Solutions Architect"
	Style           string   `json:"style" yaml:"style"`                       // "formal" or "casual"
	DelegatePatrols []string `json:"delegate_patrols" yaml:"delegate_patrols"` // patrol names to delegate to
	MaxPromptTokens int      `json:"max_prompt_tokens" yaml:"max_prompt_tokens"`
	AutoReplyThreshold float64 `json:"auto_reply_threshold" yaml:"auto_reply_threshold"` // >= this auto-reply
	ReviewThreshold    float64 `json:"review_threshold" yaml:"review_threshold"`         // >= this go to review queue
}

// DefaultTwinConfig returns the default TwinConfig.
func DefaultTwinConfig() TwinConfig {
	return TwinConfig{
		Enabled:            false,
		Name:               "Keith",
		Role:               "AWS Solutions Architect",
		Style:              "formal",
		MaxPromptTokens:    8000,
		AutoReplyThreshold: 0.8,
		ReviewThreshold:    0.3,
	}
}

// ConfidenceScore holds the multi-dimensional confidence assessment.
type ConfidenceScore struct {
	Coverage    float64 `json:"coverage"`    // what % of query terms appear in wiki
	Recency     float64 `json:"recency"`     // how recent are matching wiki sources
	Specificity float64 `json:"specificity"` // exact match vs fuzzy match
	Overall     float64 `json:"overall"`     // weighted average
}

// TwinManager manages the CTO Digital Twin configuration and prompt assembly.
type TwinManager struct {
	mu       sync.RWMutex
	config   TwinConfig
	wiki     *WikiManager
	configPath string
}

// NewTwinManager creates a TwinManager with the given wiki manager and data directory.
func NewTwinManager(wiki *WikiManager, dataDir string) *TwinManager {
	tm := &TwinManager{
		config:     DefaultTwinConfig(),
		wiki:       wiki,
		configPath: filepath.Join(dataDir, "twin-config.json"),
	}
	tm.loadConfig()
	return tm
}

// Config returns the current twin configuration.
func (tm *TwinManager) Config() TwinConfig {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.config
}

// UpdateConfig sets a new twin configuration and persists it.
func (tm *TwinManager) UpdateConfig(cfg TwinConfig) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.config = cfg
	return tm.saveConfig()
}

func (tm *TwinManager) loadConfig() {
	data, err := os.ReadFile(tm.configPath)
	if err != nil {
		return // use defaults
	}
	var cfg TwinConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return
	}
	tm.config = cfg
}

func (tm *TwinManager) saveConfig() error {
	data, err := json.MarshalIndent(tm.config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal twin config: %w", err)
	}
	dir := filepath.Dir(tm.configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(tm.configPath, data, 0o644)
}

// BuildTwinPrompt assembles a system prompt for the Digital Twin from wiki knowledge
// and recent decisions.
func (tm *TwinManager) BuildTwinPrompt(query string, recentDecisions []Decision) string {
	tm.mu.RLock()
	cfg := tm.config
	tm.mu.RUnlock()

	var sb strings.Builder

	// [Role] section -- always present, never truncated.
	sb.WriteString("[Role]\n")
	sb.WriteString(fmt.Sprintf("You are an AI agent acting as %s's digital twin. ", cfg.Name))
	sb.WriteString(fmt.Sprintf("%s's role is %s. ", cfg.Name, cfg.Role))
	sb.WriteString(fmt.Sprintf("Answer questions as %s would, cite sources when available, ", cfg.Name))
	sb.WriteString("and escalate when you are unsure or the question is outside your knowledge.\n\n")

	// [Response Style] section -- always present, never truncated.
	sb.WriteString("[Response Style]\n")
	switch cfg.Style {
	case "casual":
		sb.WriteString("Use a casual, friendly tone. Be direct and concise. ")
		sb.WriteString("Use everyday language, avoid jargon unless necessary.\n\n")
	default: // formal
		sb.WriteString("Use a professional, formal tone. Be precise and thorough. ")
		sb.WriteString("Prefer data-driven answers, give conclusions first then explain.\n\n")
	}

	// Estimate token budget for knowledge + decisions.
	// Role + Style is roughly 200 tokens. Reserve the rest.
	budgetTokens := cfg.MaxPromptTokens
	if budgetTokens <= 0 {
		budgetTokens = 8000
	}
	remainingBudget := budgetTokens - 200

	// [Recent Decisions] section -- higher priority than wiki context.
	decisionText := ""
	if len(recentDecisions) > 0 {
		var dsb strings.Builder
		dsb.WriteString("[Recent Decisions]\n")
		for _, d := range recentDecisions {
			line := fmt.Sprintf("- %s: %s (%s)\n", d.Title, d.Decision, d.CreatedAt.Format("2006-01-02"))
			dsb.WriteString(line)
		}
		dsb.WriteString("\n")
		decisionText = dsb.String()
		// Rough token estimate: 1 token ~ 4 chars.
		decisionTokens := len(decisionText) / 4
		remainingBudget -= decisionTokens
	}

	// [Knowledge Context] section -- from wiki pages matching query.
	knowledgeText := ""
	if tm.wiki != nil && query != "" && remainingBudget > 100 {
		pages, err := tm.wiki.ListPages()
		if err == nil && len(pages) > 0 {
			queryTerms := strings.Fields(strings.ToLower(query))
			type scored struct {
				page  WikiPage
				score int
			}
			var matches []scored
			for _, p := range pages {
				s := 0
				lower := strings.ToLower(p.Title + " " + strings.Join(p.Entities, " "))
				for _, term := range queryTerms {
					if strings.Contains(lower, term) {
						s++
					}
				}
				if s > 0 {
					matches = append(matches, scored{page: p, score: s})
				}
			}
			// Sort by score descending, take top 5.
			if len(matches) > 1 {
				for i := 0; i < len(matches)-1; i++ {
					for j := i + 1; j < len(matches); j++ {
						if matches[j].score > matches[i].score {
							matches[i], matches[j] = matches[j], matches[i]
						}
					}
				}
			}
			if len(matches) > 5 {
				matches = matches[:5]
			}

			if len(matches) > 0 {
				var ksb strings.Builder
				ksb.WriteString("[Knowledge Context]\n")
				maxCharsPerPage := (remainingBudget * 4) / max(len(matches), 1) // ~500 tokens per page
				for _, m := range matches {
					page, err := tm.wiki.ReadPage(m.page.Name)
					if err != nil {
						continue
					}
					content := page.Content
					if len(content) > maxCharsPerPage {
						content = content[:maxCharsPerPage] + "..."
					}
					ksb.WriteString(fmt.Sprintf("### %s\n%s\n\n", page.Title, content))
				}
				knowledgeText = ksb.String()
			}
		}
	}

	// Assemble: Knowledge first (lower priority, may be truncated), then Decisions.
	if knowledgeText != "" {
		sb.WriteString(knowledgeText)
	}
	if decisionText != "" {
		sb.WriteString(decisionText)
	}

	return sb.String()
}

// ScoreConfidence evaluates how confident the Twin should be about answering a query.
func (tm *TwinManager) ScoreConfidence(query string, wikiPages []WikiPage) ConfidenceScore {
	if query == "" || len(wikiPages) == 0 {
		return ConfidenceScore{
			Coverage:    0,
			Recency:     0,
			Specificity: scoringSpecificity(query),
			Overall:     scoringSpecificity(query) * 0.2, // only specificity contributes
		}
	}

	coverage := scoringCoverage(query, wikiPages)
	recency := scoringRecency(wikiPages)
	specificity := scoringSpecificity(query)

	// Weighted average: coverage 0.5, recency 0.3, specificity 0.2.
	overall := coverage*0.5 + recency*0.3 + specificity*0.2

	return ConfidenceScore{
		Coverage:    coverage,
		Recency:     recency,
		Specificity: specificity,
		Overall:     overall,
	}
}

// scoringCoverage calculates what fraction of query terms appear in wiki page titles/entities.
func scoringCoverage(query string, pages []WikiPage) float64 {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return 0
	}

	// Build a combined text from all wiki pages for matching.
	var combined strings.Builder
	for _, p := range pages {
		combined.WriteString(strings.ToLower(p.Title))
		combined.WriteString(" ")
		for _, e := range p.Entities {
			combined.WriteString(strings.ToLower(e))
			combined.WriteString(" ")
		}
	}
	text := combined.String()

	matched := 0
	for _, term := range terms {
		if strings.Contains(text, term) {
			matched++
		}
	}

	coverage := float64(matched) / float64(len(terms))
	return math.Min(1.0, coverage*1.2) // slight boost, capped at 1.0
}

// scoringRecency evaluates how recent the matching wiki pages are.
// Uses exponential decay with 30-day half-life.
func scoringRecency(pages []WikiPage) float64 {
	if len(pages) == 0 {
		return 0
	}

	now := time.Now()
	var totalScore float64
	var counted int

	for _, p := range pages {
		t, err := time.Parse("2006-01-02T15:04:05Z", p.CompiledAt)
		if err != nil {
			continue
		}
		days := now.Sub(t).Hours() / 24
		// Exponential decay: exp(-days/30) gives 30-day half-life.
		score := math.Exp(-days / 30.0)
		totalScore += score
		counted++
	}

	if counted == 0 {
		return 0
	}
	return totalScore / float64(counted)
}

// scoringSpecificity evaluates how specific the query is.
// Longer queries with concrete terms score higher.
func scoringSpecificity(query string) float64 {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return 0
	}

	// Base score from term count: more terms = more specific.
	lengthScore := math.Min(1.0, float64(len(terms))/8.0)

	// Bonus for concrete entities (AWS service names, project names, etc.).
	concreteTerms := []string{
		"ec2", "s3", "lambda", "cloudfront", "rds", "eks", "ecs",
		"dynamodb", "sqs", "sns", "iam", "vpc", "alb", "nlb",
		"cdk", "terraform", "docker", "kubernetes", "bedrock",
		"aurora", "elasticache", "cloudwatch", "waf", "kms",
	}
	entityBonus := 0.0
	lowerQuery := strings.ToLower(query)
	for _, ct := range concreteTerms {
		if strings.Contains(lowerQuery, ct) {
			entityBonus += 0.15
		}
	}
	entityBonus = math.Min(entityBonus, 0.4)

	return math.Min(1.0, lengthScore*0.6+entityBonus+0.1)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
