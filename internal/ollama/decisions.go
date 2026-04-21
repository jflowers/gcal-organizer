package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/jflowers/gcal-organizer/pkg/models"
)

// DecisionExtractor extracts structured decisions from meeting transcripts
// using a local AI model. Used in local-only mode as a replacement for
// the cloud-based Gemini extraction.
type DecisionExtractor interface {
	// ExtractDecisions analyzes a transcript and returns categorized decisions
	// (made, deferred, open) with text, timestamp, and context fields.
	ExtractDecisions(ctx context.Context, transcriptText string) ([]models.Decision, error)
}

// LocalDecisionExtractor implements DecisionExtractor using a local Ollama model.
type LocalDecisionExtractor struct {
	client *Client
	model  string
}

// NewDecisionExtractor creates a LocalDecisionExtractor that uses the specified
// model via the given Ollama client.
func NewDecisionExtractor(client *Client, model string) *LocalDecisionExtractor {
	return &LocalDecisionExtractor{
		client: client,
		model:  model,
	}
}

// ExtractDecisions analyzes a transcript and returns categorized decisions.
// Reuses the same prompt as the Gemini implementation (model-agnostic).
// FR-017: produces same three categories (made, deferred, open).
// Retries once on malformed response, hard-stops on network errors.
func (d *LocalDecisionExtractor) ExtractDecisions(ctx context.Context, transcriptText string) ([]models.Decision, error) {
	if transcriptText == "" {
		return nil, nil
	}

	// Same prompt as internal/gemini/client.go:ExtractDecisions
	prompt := fmt.Sprintf(`You are a meeting decision extraction assistant. Analyze the following meeting transcript and extract all decisions into three categories:

1. "made" — Decisions that were explicitly agreed upon or committed to
2. "deferred" — Decisions that were explicitly postponed or tabled for later
3. "open" — Topics discussed but left unresolved, needing further discussion

For each decision, provide:
- "category": one of "made", "deferred", or "open"
- "text": a clear, concise description of the decision (one sentence)
- "timestamp": the HH:MM timestamp from the transcript where this was discussed (or empty string if not identifiable)
- "context": a brief excerpt from the transcript providing context (or empty string)

Return ONLY a JSON array. No other text.

Example response:
[
  {"category": "made", "text": "Team will adopt GitHub Actions for CI/CD", "timestamp": "12:34", "context": "After discussing three options, team voted unanimously"},
  {"category": "deferred", "text": "Budget allocation for Q3", "timestamp": "13:15", "context": "Waiting for finance team input"},
  {"category": "open", "text": "Whether to migrate to new API version", "timestamp": "13:45", "context": "Need performance benchmarks first"}
]

If no decisions are found, return an empty array: []

Transcript:
%s`, transcriptText)

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		response, err := d.client.Generate(ctx, d.model, prompt)
		if err != nil {
			// Network/timeout errors: hard-stop immediately (FR-009).
			return nil, fmt.Errorf("local decision extraction failed: %w", err)
		}

		decisions, parseErr := parseLocalDecisionsResponse(response)
		if parseErr != nil {
			lastErr = parseErr
			continue // Retry on malformed response
		}
		return decisions, nil
	}

	return nil, fmt.Errorf("%w: %v", ErrMalformedResponse, lastErr)
}

// parseLocalDecisionsResponse parses the JSON array response from the local model.
// Same parsing logic as internal/gemini/client.go:parseDecisionsResponse.
func parseLocalDecisionsResponse(responseText string) ([]models.Decision, error) {
	responseText = strings.TrimSpace(responseText)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	// Try to find JSON array in the response.
	jsonArrayRegex := regexp.MustCompile(`\[[\s\S]*\]`)
	matches := jsonArrayRegex.FindString(responseText)
	if matches != "" {
		responseText = matches
	}

	var rawDecisions []struct {
		Category  string `json:"category"`
		Text      string `json:"text"`
		Timestamp string `json:"timestamp"`
		Context   string `json:"context"`
	}

	if err := json.Unmarshal([]byte(responseText), &rawDecisions); err != nil {
		return nil, fmt.Errorf("parse decisions response: %w", err)
	}

	validCategories := map[string]bool{
		"made":     true,
		"deferred": true,
		"open":     true,
	}

	var decisions []models.Decision
	for _, raw := range rawDecisions {
		text := strings.TrimSpace(raw.Text)
		if text == "" {
			continue
		}

		category := strings.ToLower(strings.TrimSpace(raw.Category))
		if !validCategories[category] {
			category = "open"
		}

		decisions = append(decisions, models.Decision{
			Category:  category,
			Text:      text,
			Timestamp: strings.TrimSpace(raw.Timestamp),
			Context:   strings.TrimSpace(raw.Context),
		})
	}

	return decisions, nil
}
