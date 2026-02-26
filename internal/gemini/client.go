// Package gemini provides Gemini AI integration for action item extraction.
package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/jflowers/gcal-organizer/internal/retry"
	"github.com/jflowers/gcal-organizer/pkg/models"
	"google.golang.org/genai"
)

// Client wraps the Gemini SDK for action item extraction.
type Client struct {
	client    *genai.Client
	modelName string
}

// NewClient creates a new Gemini client for action item extraction.
func NewClient(ctx context.Context, apiKey, modelName string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY is required")
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	return &Client{
		client:    client,
		modelName: modelName,
	}, nil
}

// CheckboxItem represents a checkbox item from a document with its index.
type CheckboxItem struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
}

// CheckboxAssignment represents an extracted assignment for a checkbox.
type CheckboxAssignment struct {
	Index    int    `json:"index"`
	Text     string `json:"text"`
	Assignee string `json:"assignee"`
	Email    string `json:"email"` // Populated after name-to-email resolution
}

// ExtractAssigneesFromCheckboxes extracts assignee names from multiple checkbox items.
func (c *Client) ExtractAssigneesFromCheckboxes(ctx context.Context, items []CheckboxItem) ([]CheckboxAssignment, error) {
	if len(items) == 0 {
		return nil, nil
	}

	// Build a single prompt with all checkbox items for efficiency
	var itemsList strings.Builder
	for _, item := range items {
		itemsList.WriteString(fmt.Sprintf("%d. %s\n", item.Index, item.Text))
	}

	prompt := fmt.Sprintf(`You are an action item extraction assistant. For each numbered task below, determine if there is a SINGLE, SPECIFIC individual who is clearly responsible.

Tasks:
%s

Return your response as a JSON array. Each element should have:
- "index": The task number (integer)
- "assignee": The full name of *one specific person* responsible (string), or null

CRITICAL Rules for determining the assignee:
1. ONLY return an assignee when a SINGLE, NAMED INDIVIDUAL is clearly the person who must do the task
2. The pattern must be "[Person's Name] will...", "[Person's Name] to...", or similar where the person is the SUBJECT performing the action
3. Return null in ALL of these cases:
   - A group or team is the subject: "The group will...", "The team will...", "We will..."
   - Multiple people share responsibility: "Alice and Bob will..."
   - A person is mentioned but is NOT the one doing the task: "approach Martin" (someone else approaches Martin)
   - The assignee is vague or unclear: "someone should...", "it was decided..."
   - No person is mentioned at all
4. Only return the JSON array, no other text

Example input tasks:
0. Jay will schedule the follow-up meeting
1. The group will discuss Martin's proposal
2. Alice and Bob will prepare the presentation
3. Sarah will send the summary email
4. Reach out to the vendor about pricing

Example response:
[
  {"index": 0, "assignee": "Jay"},
  {"index": 1, "assignee": null},
  {"index": 2, "assignee": null},
  {"index": 3, "assignee": "Sarah"},
  {"index": 4, "assignee": null}
]

Your response:`, itemsList.String())

	var result *genai.GenerateContentResponse
	err := retry.Do(ctx, retry.DefaultConfig(), func() error {
		var e error
		result, e = c.client.Models.GenerateContent(ctx, c.modelName, genai.Text(prompt), nil)
		return e
	})
	if err != nil {
		return nil, fmt.Errorf("Gemini API error: %w", err)
	}

	if len(result.Candidates) == 0 ||
		result.Candidates[0].Content == nil ||
		len(result.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no response from Gemini")
	}

	// Extract text from the response
	var responseText string
	for _, part := range result.Candidates[0].Content.Parts {
		if part.Text != "" {
			responseText += part.Text
		}
	}

	// Parse the JSON array response
	assignments, err := parseAssignmentsResponse(responseText, items)
	if err != nil {
		return nil, err
	}

	return assignments, nil
}

// ExtractDecisions sends a transcript to Gemini and returns structured decisions.
func (c *Client) ExtractDecisions(ctx context.Context, transcriptText string) ([]models.Decision, error) {
	if transcriptText == "" {
		return nil, nil
	}

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

	var result *genai.GenerateContentResponse
	err := retry.Do(ctx, retry.DefaultConfig(), func() error {
		var e error
		result, e = c.client.Models.GenerateContent(ctx, c.modelName, genai.Text(prompt), nil)
		return e
	})
	if err != nil {
		return nil, fmt.Errorf("Gemini API error: %w", err)
	}

	if len(result.Candidates) == 0 ||
		result.Candidates[0].Content == nil ||
		len(result.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no response from Gemini")
	}

	var responseText string
	for _, part := range result.Candidates[0].Content.Parts {
		if part.Text != "" {
			responseText += part.Text
		}
	}

	decisions, err := parseDecisionsResponse(responseText)
	if err != nil {
		return nil, err
	}

	return decisions, nil
}

// parseDecisionsResponse parses the JSON array response from Gemini for decision extraction.
func parseDecisionsResponse(responseText string) ([]models.Decision, error) {
	responseText = strings.TrimSpace(responseText)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	// Try to find JSON array in the response
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
		// Do not include raw response text in error — it may contain
		// confidential transcript content from the meeting.
		return nil, fmt.Errorf("failed to parse Gemini decisions response as JSON array: %w", err)
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

// parseAssignmentsResponse parses the JSON array response from Gemini.
func parseAssignmentsResponse(responseText string, items []CheckboxItem) ([]CheckboxAssignment, error) {
	// Clean up the response
	responseText = strings.TrimSpace(responseText)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	// Try to find JSON array in the response
	jsonArrayRegex := regexp.MustCompile(`\[[\s\S]*\]`)
	matches := jsonArrayRegex.FindString(responseText)
	if matches != "" {
		responseText = matches
	}

	var rawAssignments []struct {
		Index    int     `json:"index"`
		Assignee *string `json:"assignee"` // Pointer to handle null
	}

	if err := json.Unmarshal([]byte(responseText), &rawAssignments); err != nil {
		// Do not include raw response text in error — it may contain
		// confidential task content from the meeting document.
		return nil, fmt.Errorf("failed to parse Gemini response as JSON array: %w", err)
	}

	// Map items by index for easy lookup
	itemMap := make(map[int]CheckboxItem)
	for _, item := range items {
		itemMap[item.Index] = item
	}

	// Build result
	var assignments []CheckboxAssignment
	for _, raw := range rawAssignments {
		item, ok := itemMap[raw.Index]
		if !ok {
			continue
		}

		assignee := ""
		if raw.Assignee != nil {
			assignee = *raw.Assignee
		}

		if assignee != "" {
			assignments = append(assignments, CheckboxAssignment{
				Index:    raw.Index,
				Text:     item.Text,
				Assignee: assignee,
			})
		}
	}

	return assignments, nil
}
