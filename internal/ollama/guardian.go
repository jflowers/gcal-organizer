package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jflowers/gcal-organizer/pkg/models"
)

// defaultMaxTranscriptChars is the default maximum transcript length in
// characters before truncation. Approximately 6000 tokens for granite-guardian.
const defaultMaxTranscriptChars = 24000

// SensitivityClassifier classifies transcript content for sensitivity
// before any processing occurs. Implementations must be safe for
// concurrent use.
type SensitivityClassifier interface {
	// Classify analyzes a transcript and returns a sensitivity determination.
	// Returns an error if the classification fails (model unavailable,
	// malformed response after retry, timeout).
	Classify(ctx context.Context, transcript string) (*models.SensitivityResult, error)
}

// Guardian implements SensitivityClassifier using the granite-guardian model
// via the Ollama API.
type Guardian struct {
	client           *Client
	model            string
	maxTranscriptLen int
}

// NewGuardian creates a Guardian sensitivity classifier that uses the specified
// model via the given Ollama client.
func NewGuardian(client *Client, model string) *Guardian {
	return &Guardian{
		client:           client,
		model:            model,
		maxTranscriptLen: defaultMaxTranscriptChars,
	}
}

// sensitivityPrompt is the prompt template for sensitivity classification.
// Categories and rules from research.md section 3.
const sensitivityPrompt = `You are a workplace content sensitivity classifier. Analyze the following meeting transcript and determine if it contains sensitive content that should not be processed by cloud AI services or written to external files.

Classify the transcript into one of these categories:
- "hr": Performance reviews, disciplinary actions, employee complaints, accommodation requests, hiring/firing discussions about specific individuals
- "legal": Legal proceedings, compliance investigations, regulatory matters, contracts under negotiation, litigation strategy
- "financial": Individual compensation discussions, salary negotiations, M&A discussions, confidential budget allocations
- "health": Medical information, health accommodations, wellness concerns about specific individuals
- "termination": Layoff planning, termination decisions, severance negotiations, reduction-in-force discussions
- "none": No sensitive content detected

Return your analysis as a JSON object with these fields:
- "sensitive": boolean (true if the transcript contains sensitive content)
- "category": string (one of: hr, legal, financial, health, termination, none)
- "score": number between 0.0 and 1.0 (confidence in the classification)
- "reasoning": string (brief explanation of why the content is or is not sensitive)

Important rules:
1. Err on the side of caution — if uncertain, classify as sensitive with a moderate score
2. Focus on content about SPECIFIC INDIVIDUALS, not general policy discussions
3. A meeting about "updating the PTO policy" is NOT sensitive; a meeting about "Sarah's excessive absences" IS sensitive
4. Return ONLY the JSON object, no other text

Transcript:
%s`

// validCategories defines the accepted sensitivity categories.
var validCategories = map[string]bool{
	"hr":          true,
	"legal":       true,
	"financial":   true,
	"health":      true,
	"termination": true,
	"none":        true,
}

// Classify analyzes a transcript and returns a sensitivity determination.
// Retries once on malformed response (total 2 attempts). Hard-stops on
// network errors (FR-009).
func (g *Guardian) Classify(ctx context.Context, transcript string) (*models.SensitivityResult, error) {
	// Truncate transcript if it exceeds the max length.
	transcript = g.truncateTranscript(transcript)

	prompt := fmt.Sprintf(sensitivityPrompt, transcript)

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		response, err := g.client.Generate(ctx, g.model, prompt)
		if err != nil {
			// Network/timeout errors: hard-stop immediately (FR-009).
			return nil, fmt.Errorf("sensitivity classification failed: %w", err)
		}

		result, parseErr := parseSensitivityResponse(response)
		if parseErr != nil {
			lastErr = parseErr
			continue // Retry on malformed response
		}
		return result, nil
	}

	return nil, fmt.Errorf("%w: %v", ErrMalformedResponse, lastErr)
}

// truncateTranscript truncates a transcript that exceeds the max length,
// preserving the first 60% and last 40% with a separator.
func (g *Guardian) truncateTranscript(transcript string) string {
	if len(transcript) <= g.maxTranscriptLen {
		return transcript
	}

	keepFirst := int(float64(g.maxTranscriptLen) * 0.6)
	keepLast := g.maxTranscriptLen - keepFirst
	separator := "\n\n[... transcript truncated ...]\n\n"

	return transcript[:keepFirst] + separator + transcript[len(transcript)-keepLast:]
}

// parseSensitivityResponse parses the JSON response from the sensitivity model.
// Strips markdown fences, validates category, and clamps score to [0.0, 1.0].
func parseSensitivityResponse(response string) (*models.SensitivityResult, error) {
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var result models.SensitivityResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("parse sensitivity response: %w", err)
	}

	// Validate category — default to "none" if unknown.
	result.Category = strings.ToLower(strings.TrimSpace(result.Category))
	if !validCategories[result.Category] {
		result.Category = "none"
	}

	// Clamp score to [0.0, 1.0].
	if result.Score < 0.0 {
		result.Score = 0.0
	}
	if result.Score > 1.0 {
		result.Score = 1.0
	}

	return &result, nil
}
