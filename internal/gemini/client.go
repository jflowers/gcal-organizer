// Package gemini provides Gemini AI integration for action item extraction.
package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/jflowers/gcal-organizer/internal/retry"
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

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
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
		return nil, fmt.Errorf("failed to parse Gemini response as JSON array: %w\nResponse was: %s", err, responseText)
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
