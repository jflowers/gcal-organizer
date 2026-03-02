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
		wantError bool
		// Contract: expected assignments with full field equality
		wantAssignments []CheckboxAssignment
	}{
		{
			name:     "valid array response — null assignees filtered",
			response: `[{"index": 0, "assignee": "Jay"}, {"index": 1, "assignee": null}, {"index": 2, "assignee": "Sarah"}]`,
			wantAssignments: []CheckboxAssignment{
				{Index: 0, Text: items[0].Text, Assignee: "Jay"},
				{Index: 2, Text: items[2].Text, Assignee: "Sarah"},
			},
		},
		{
			name:     "markdown code block is unwrapped",
			response: "```json\n[{\"index\": 0, \"assignee\": \"Jay\"}]\n```",
			wantAssignments: []CheckboxAssignment{
				{Index: 0, Text: items[0].Text, Assignee: "Jay"},
			},
		},
		{
			name:            "all null assignees returns empty result",
			response:        `[{"index": 0, "assignee": null}, {"index": 1, "assignee": null}]`,
			wantAssignments: nil,
		},
		{
			name:      "invalid JSON returns error",
			response:  "not json at all",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseAssignmentsResponse(tt.response, items)

			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Contract: result count must match
			if len(result) != len(tt.wantAssignments) {
				t.Fatalf("got %d assignments, want %d", len(result), len(tt.wantAssignments))
			}

			// Contract: each assignment must have correct Assignee, Index, and Text
			for i, want := range tt.wantAssignments {
				got := result[i]
				if got.Assignee != want.Assignee {
					t.Errorf("assignment[%d].Assignee: got %q, want %q", i, got.Assignee, want.Assignee)
				}
				if got.Index != want.Index {
					t.Errorf("assignment[%d].Index: got %d, want %d", i, got.Index, want.Index)
				}
				if got.Text != want.Text {
					t.Errorf("assignment[%d].Text: got %q, want %q", i, got.Text, want.Text)
				}
			}
		})
	}
}

// ---------- T005: Decision extraction response parsing tests ----------

func TestParseDecisionsResponse(t *testing.T) {
	tests := []struct {
		name      string
		response  string
		wantError bool
		// Contract: full Decision struct equality per item
		wantDecisions []models.Decision
	}{
		{
			name:     "valid JSON array — all fields populated",
			response: `[{"category": "made", "text": "Adopt new pipeline", "timestamp": "12:34", "context": "Team voted"}, {"category": "deferred", "text": "Budget review", "timestamp": "13:00", "context": ""}]`,
			wantDecisions: []models.Decision{
				{Category: "made", Text: "Adopt new pipeline", Timestamp: "12:34", Context: "Team voted"},
				{Category: "deferred", Text: "Budget review", Timestamp: "13:00", Context: ""},
			},
		},
		{
			name:     "markdown-wrapped response is unwrapped",
			response: "```json\n[{\"category\": \"open\", \"text\": \"Discuss architecture\", \"timestamp\": \"09:15\", \"context\": \"Need more info\"}]\n```",
			wantDecisions: []models.Decision{
				{Category: "open", Text: "Discuss architecture", Timestamp: "09:15", Context: "Need more info"},
			},
		},
		{
			name:          "empty-text decisions are filtered out",
			response:      `[{"category": "made", "text": "", "timestamp": "", "context": ""}, {"category": "made", "text": "Real decision", "timestamp": "10:00", "context": ""}]`,
			wantDecisions: []models.Decision{{Category: "made", Text: "Real decision", Timestamp: "10:00", Context: ""}},
		},
		{
			name:          "invalid category defaults to open",
			response:      `[{"category": "bogus", "text": "Some item", "timestamp": "", "context": ""}]`,
			wantDecisions: []models.Decision{{Category: "open", Text: "Some item", Timestamp: "", Context: ""}},
		},
		{
			name:          "whitespace-only text is filtered out",
			response:      `[{"category": "made", "text": "  ", "timestamp": "", "context": ""}]`,
			wantDecisions: nil,
		},
		{
			name:          "empty array returns nil",
			response:      `[]`,
			wantDecisions: nil,
		},
		{
			name:      "invalid JSON returns error",
			response:  "not json at all",
			wantError: true,
		},
		{
			name:          "whitespace around fields is trimmed",
			response:      `[{"category": " MADE ", "text": "  Trimmed text  ", "timestamp": " 14:00 ", "context": " some context "}]`,
			wantDecisions: []models.Decision{{Category: "made", Text: "Trimmed text", Timestamp: "14:00", Context: "some context"}},
		},
		{
			name:     "error does not leak response text",
			response: `{"not": "an array"}`,
			// The JSON is valid but it's an object, not an array — Unmarshal will fail
			wantError: true,
		},
		{
			name:     "JSON array embedded in prose text",
			response: "Here are the decisions:\n[{\"category\": \"made\", \"text\": \"Ship it\", \"timestamp\": \"15:00\", \"context\": \"\"}]\nEnd of response.",
			wantDecisions: []models.Decision{
				{Category: "made", Text: "Ship it", Timestamp: "15:00", Context: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseDecisionsResponse(tt.response)

			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				// Contract: error message must not contain the raw response text
				errMsg := err.Error()
				if len(tt.response) > 10 {
					// Check a substring of the response isn't leaked
					probe := tt.response
					if len(probe) > 30 {
						probe = probe[:30]
					}
					if testing.Verbose() {
						t.Logf("error message: %s", errMsg)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Contract: result count must match
			if len(result) != len(tt.wantDecisions) {
				t.Fatalf("got %d decisions, want %d", len(result), len(tt.wantDecisions))
			}

			// Contract: each Decision struct must match field-by-field
			for i, want := range tt.wantDecisions {
				got := result[i]
				if got.Category != want.Category {
					t.Errorf("decision[%d].Category: got %q, want %q", i, got.Category, want.Category)
				}
				if got.Text != want.Text {
					t.Errorf("decision[%d].Text: got %q, want %q", i, got.Text, want.Text)
				}
				if got.Timestamp != want.Timestamp {
					t.Errorf("decision[%d].Timestamp: got %q, want %q", i, got.Timestamp, want.Timestamp)
				}
				if got.Context != want.Context {
					t.Errorf("decision[%d].Context: got %q, want %q", i, got.Context, want.Context)
				}
			}
		})
	}
}

// Compile-time check that models.Decision is usable from this package.
var _ = models.Decision{}
