package gemini

import (
	"testing"
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
