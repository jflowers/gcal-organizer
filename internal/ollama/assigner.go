package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/jflowers/gcal-organizer/pkg/models"
)

// TaskAssigner extracts assignees from checkbox action items using
// a local AI model. Produces results in the same format as the
// existing cloud-based extraction (FR-014).
//
// Note: CheckboxItem and CheckboxAssignment types are defined in
// pkg/models/models.go so that both internal/ollama and internal/gemini
// can use them without cross-package dependency (review finding A-1).
type TaskAssigner interface {
	// ExtractAssignees identifies the responsible individual for each
	// checkbox item. Returns assignments only for items with a clear
	// single assignee; items with group/ambiguous assignees are omitted.
	ExtractAssignees(ctx context.Context, items []models.CheckboxItem) ([]models.CheckboxAssignment, error)
}

// Assigner implements TaskAssigner using a local Ollama model.
type Assigner struct {
	client *Client
	model  string
}

// NewAssigner creates a TaskAssigner that uses the specified model via
// the given Ollama client.
func NewAssigner(client *Client, model string) *Assigner {
	return &Assigner{
		client: client,
		model:  model,
	}
}

// ExtractAssigneesFromCheckboxes is an alias for ExtractAssignees that matches
// the same method signature as gemini.Client, allowing both to satisfy the
// same interface in the caller.
func (a *Assigner) ExtractAssigneesFromCheckboxes(ctx context.Context, items []models.CheckboxItem) ([]models.CheckboxAssignment, error) {
	return a.ExtractAssignees(ctx, items)
}

// ExtractAssignees identifies the responsible individual for each checkbox
// item using the local AI model. Reuses the same prompt as the Gemini
// implementation (model-agnostic). Retries once on malformed response,
// hard-stops on network errors.
func (a *Assigner) ExtractAssignees(ctx context.Context, items []models.CheckboxItem) ([]models.CheckboxAssignment, error) {
	if len(items) == 0 {
		return nil, nil
	}

	// Build the same prompt as internal/gemini/client.go:ExtractAssigneesFromCheckboxes
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

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		response, err := a.client.Generate(ctx, a.model, prompt)
		if err != nil {
			// Network/timeout errors: hard-stop immediately (FR-009).
			return nil, fmt.Errorf("local task assignment failed: %w", err)
		}

		assignments, parseErr := parseLocalAssignmentsResponse(response, items)
		if parseErr != nil {
			lastErr = parseErr
			continue // Retry on malformed response
		}
		return assignments, nil
	}

	return nil, fmt.Errorf("%w: %v", ErrMalformedResponse, lastErr)
}

// parseLocalAssignmentsResponse parses the JSON array response from the local model.
// Same parsing logic as internal/gemini/client.go:parseAssignmentsResponse.
func parseLocalAssignmentsResponse(responseText string, items []models.CheckboxItem) ([]models.CheckboxAssignment, error) {
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

	var rawAssignments []struct {
		Index    int     `json:"index"`
		Assignee *string `json:"assignee"` // Pointer to handle null
	}

	if err := json.Unmarshal([]byte(responseText), &rawAssignments); err != nil {
		return nil, fmt.Errorf("parse assignment response: %w", err)
	}

	// Map items by index for easy lookup.
	itemMap := make(map[int]models.CheckboxItem)
	for _, item := range items {
		itemMap[item.Index] = item
	}

	// Build result — only include items with a non-empty assignee (FR-015).
	var assignments []models.CheckboxAssignment
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
			assignments = append(assignments, models.CheckboxAssignment{
				Index:    raw.Index,
				Text:     item.Text,
				Assignee: assignee,
			})
		}
	}

	return assignments, nil
}
