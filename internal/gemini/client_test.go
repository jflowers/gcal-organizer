package gemini

import (
	"testing"

	"github.com/jflowers/gcal-organizer/pkg/models"
)

func TestParseAssignmentsResponse(t *testing.T) {
	items := []CheckboxItem{
		{Index: 0, Text: "Jay will schedule the follow-up meeting"},
		{Index: 1, Text: "The group will discuss Martin's proposal"},
		{Index: 2, Text: "Sarah will send the summary email"},
	}

	tests := []struct {
		name      string
		response  string
		wantCount int
		wantError bool
	}{
		{
			name:      "valid array response",
			response:  `[{"index": 0, "assignee": "Jay"}, {"index": 1, "assignee": null}, {"index": 2, "assignee": "Sarah"}]`,
			wantCount: 2,
			wantError: false,
		},
		{
			name:      "with markdown code block",
			response:  "```json\n[{\"index\": 0, \"assignee\": \"Jay\"}]\n```",
			wantCount: 1,
			wantError: false,
		},
		{
			name:      "all null assignees",
			response:  `[{"index": 0, "assignee": null}, {"index": 1, "assignee": null}]`,
			wantCount: 0,
			wantError: false,
		},
		{
			name:      "invalid json",
			response:  "not json at all",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseAssignmentsResponse(tt.response, items)

			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result) != tt.wantCount {
				t.Errorf("got %d assignments, want %d", len(result), tt.wantCount)
			}
		})
	}
}

// ---------- T005: Decision extraction response parsing tests ----------

func TestParseDecisionsResponse(t *testing.T) {
	tests := []struct {
		name           string
		response       string
		wantCount      int
		wantError      bool
		wantCategories []string
		wantTexts      []string
	}{
		{
			name:           "valid JSON array",
			response:       `[{"category": "made", "text": "Adopt new pipeline", "timestamp": "12:34", "context": "Team voted"}, {"category": "deferred", "text": "Budget review", "timestamp": "13:00", "context": ""}]`,
			wantCount:      2,
			wantCategories: []string{"made", "deferred"},
			wantTexts:      []string{"Adopt new pipeline", "Budget review"},
		},
		{
			name:           "markdown-wrapped response",
			response:       "```json\n[{\"category\": \"open\", \"text\": \"Discuss architecture\", \"timestamp\": \"09:15\", \"context\": \"Need more info\"}]\n```",
			wantCount:      1,
			wantCategories: []string{"open"},
			wantTexts:      []string{"Discuss architecture"},
		},
		{
			name:      "empty decisions filtered",
			response:  `[{"category": "made", "text": "", "timestamp": "", "context": ""}, {"category": "made", "text": "Real decision", "timestamp": "10:00", "context": ""}]`,
			wantCount: 1,
			wantTexts: []string{"Real decision"},
		},
		{
			name:           "invalid category defaults to open",
			response:       `[{"category": "bogus", "text": "Some item", "timestamp": "", "context": ""}]`,
			wantCount:      1,
			wantCategories: []string{"open"},
		},
		{
			name:      "empty text filtered",
			response:  `[{"category": "made", "text": "  ", "timestamp": "", "context": ""}]`,
			wantCount: 0,
		},
		{
			name:      "empty array",
			response:  `[]`,
			wantCount: 0,
		},
		{
			name:      "invalid JSON",
			response:  "not json at all",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseDecisionsResponse(tt.response)

			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result) != tt.wantCount {
				t.Errorf("got %d decisions, want %d", len(result), tt.wantCount)
			}

			for i, wantCat := range tt.wantCategories {
				if i >= len(result) {
					break
				}
				if result[i].Category != wantCat {
					t.Errorf("decision[%d]: got category %q, want %q", i, result[i].Category, wantCat)
				}
			}

			for i, wantText := range tt.wantTexts {
				if i >= len(result) {
					break
				}
				if result[i].Text != wantText {
					t.Errorf("decision[%d]: got text %q, want %q", i, result[i].Text, wantText)
				}
			}
		})
	}
}

// Verify the Decision type from models is used correctly
func TestDecisionModelFields(t *testing.T) {
	d := models.Decision{
		Category:  "made",
		Text:      "Test decision",
		Timestamp: "12:34",
		Context:   "Some context",
	}
	if d.Category != "made" {
		t.Errorf("expected category 'made', got %q", d.Category)
	}
	if d.Timestamp != "12:34" {
		t.Errorf("expected timestamp '12:34', got %q", d.Timestamp)
	}
}
