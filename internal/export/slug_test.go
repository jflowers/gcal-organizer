package export

import "testing"

// ---------- T012: TopicSlug table-driven tests ----------

func TestTopicSlug(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "standard meeting title",
			input: "Weekly Engineering Sync",
			want:  "weekly-engineering-sync",
		},
		{
			name:  "Notes by Gemini prefix only",
			input: "Notes by Gemini",
			want:  "meeting",
		},
		{
			name:  "Transcript suffix",
			input: "Project Alpha - Transcript",
			want:  "project-alpha",
		},
		{
			name:  "slash in title",
			input: "Q3 Budget / Planning",
			want:  "q3-budget-planning",
		},
		{
			name:  "colon in title",
			input: "Design Review: API v2",
			want:  "design-review-api-v2",
		},
		{
			name:  "empty string fallback",
			input: "",
			want:  "meeting",
		},
		{
			name:  "Notes by Gemini with dash prefix",
			input: "Notes by Gemini - Weekly Standup",
			want:  "weekly-standup",
		},
		{
			name:  "long transcript title",
			input: "ComplyTime Standup - 2026/02/25 14:00 WET - Transcript",
			want:  "complytime-standup-2026-02-25-14-00-wet",
		},
		{
			name:  "multiple special characters",
			input: "Q&A: Team @All — Review #42",
			want:  "q-a-team-all-review-42",
		},
		{
			name:  "leading and trailing spaces",
			input: "  Spaced Title  ",
			want:  "spaced-title",
		},
		{
			name:  "only special characters",
			input: "///",
			want:  "meeting",
		},
		{
			name:  "case insensitive prefix stripping",
			input: "notes by gemini - Test",
			want:  "test",
		},
		{
			name:  "case insensitive suffix stripping",
			input: "Meeting - transcript",
			want:  "meeting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TopicSlug(tt.input)
			if got != tt.want {
				t.Errorf("TopicSlug(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
