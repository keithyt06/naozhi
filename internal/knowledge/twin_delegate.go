package knowledge

import (
	"fmt"
	"sync"
	"time"
)

// DelegateAction represents the routing decision for a team question.
type DelegateAction string

const (
	DelegateAutoReply DelegateAction = "auto_reply"   // confidence >= auto_reply_threshold
	DelegateReview    DelegateAction = "review"        // confidence between review and auto_reply threshold
	DelegateEscalate  DelegateAction = "escalate"      // confidence < review_threshold
)

// DelegateRequest represents an incoming team question for the Twin.
type DelegateRequest struct {
	ID        string    `json:"id"`
	Question  string    `json:"question"`
	Source    string    `json:"source"`     // chat ID or channel
	Platform  string    `json:"platform"`   // feishu, slack, etc.
	Asker     string    `json:"asker"`      // who asked
	Timestamp time.Time `json:"timestamp"`
}

// DelegateResult holds the Twin's response and routing decision.
type DelegateResult struct {
	Request    DelegateRequest `json:"request"`
	Action     DelegateAction  `json:"action"`
	Draft      string          `json:"draft"`       // draft answer text
	Confidence ConfidenceScore `json:"confidence"`
	Tag        string          `json:"tag"`          // "[via CTO Twin]" or "[needs Keith's confirmation]"
	CreatedAt  time.Time       `json:"created_at"`
}

// DelegateHandler routes team questions through the CTO Digital Twin.
type DelegateHandler struct {
	twin  *TwinManager
	mu    sync.RWMutex
	queue []DelegateResult // review queue for items needing CTO attention
}

// NewDelegateHandler creates a DelegateHandler.
func NewDelegateHandler(twin *TwinManager) *DelegateHandler {
	return &DelegateHandler{
		twin:  twin,
		queue: make([]DelegateResult, 0),
	}
}

// HandleQuestion processes a team question and returns the routing result.
// The caller is responsible for actually sending the response to the IM platform
// or creating a notification -- this function only determines the action and draft.
func (dh *DelegateHandler) HandleQuestion(req DelegateRequest) (*DelegateResult, error) {
	cfg := dh.twin.Config()
	if !cfg.Enabled {
		return nil, fmt.Errorf("twin is disabled")
	}

	// Get wiki pages for confidence scoring.
	var wikiPages []WikiPage
	if dh.twin.wiki != nil {
		pages, err := dh.twin.wiki.ListPages()
		if err == nil {
			wikiPages = pages
		}
	}

	// Score confidence.
	confidence := dh.twin.ScoreConfidence(req.Question, wikiPages)

	// Build the draft answer prompt context.
	prompt := dh.twin.BuildTwinPrompt(req.Question, nil)

	result := &DelegateResult{
		Request:    req,
		Confidence: confidence,
		CreatedAt:  time.Now(),
	}

	// Route based on confidence thresholds.
	autoThreshold := cfg.AutoReplyThreshold
	if autoThreshold <= 0 {
		autoThreshold = 0.8
	}
	reviewThreshold := cfg.ReviewThreshold
	if reviewThreshold <= 0 {
		reviewThreshold = 0.3
	}

	switch {
	case confidence.Overall >= autoThreshold:
		result.Action = DelegateAutoReply
		result.Tag = fmt.Sprintf("[via %s Twin]", cfg.Name)
		result.Draft = fmt.Sprintf("[AI answer based on %s's knowledge base]\n\n"+
			"(Prompt context length: %d chars, confidence: %.2f)\n\n"+
			"To get a definitive answer, please @%s directly.",
			cfg.Name, len(prompt), confidence.Overall, cfg.Name)

	case confidence.Overall >= reviewThreshold:
		result.Action = DelegateReview
		result.Tag = fmt.Sprintf("[needs %s's confirmation]", cfg.Name)
		result.Draft = fmt.Sprintf("[Draft - needs review]\n\n"+
			"(Confidence: %.2f - review required before sending)\n\n"+
			"Question: %s",
			confidence.Overall, req.Question)
		// Add to review queue.
		dh.mu.Lock()
		dh.queue = append(dh.queue, *result)
		dh.mu.Unlock()

	default:
		result.Action = DelegateEscalate
		result.Tag = ""
		result.Draft = fmt.Sprintf("This question needs %s's direct attention. "+
			"Confidence too low (%.2f) to provide a reliable answer.",
			cfg.Name, confidence.Overall)
		// Add to review queue as well.
		dh.mu.Lock()
		dh.queue = append(dh.queue, *result)
		dh.mu.Unlock()
	}

	return result, nil
}

// ReviewQueue returns the current review queue items.
func (dh *DelegateHandler) ReviewQueue() []DelegateResult {
	dh.mu.RLock()
	defer dh.mu.RUnlock()
	out := make([]DelegateResult, len(dh.queue))
	copy(out, dh.queue)
	return out
}

// DismissFromQueue removes an item from the review queue by request ID.
func (dh *DelegateHandler) DismissFromQueue(requestID string) bool {
	dh.mu.Lock()
	defer dh.mu.Unlock()
	for i, item := range dh.queue {
		if item.Request.ID == requestID {
			dh.queue = append(dh.queue[:i], dh.queue[i+1:]...)
			return true
		}
	}
	return false
}
