package main

import (
	"testing"

	"github.com/jflowers/gcal-organizer/internal/docs"
)

func TestExtractUnassignedItems(t *testing.T) {
	tests := []struct {
		name       string
		checkboxes []*docs.CheckboxItem
		wantCount  int
		wantTexts  []string
	}{
		{
			name: "all processed",
			checkboxes: []*docs.CheckboxItem{
				{Text: "Task 1", IsProcessed: true},
				{Text: "Task 2", IsProcessed: true},
			},
			wantCount: 0,
		},
		{
			name: "none processed",
			checkboxes: []*docs.CheckboxItem{
				{Text: "Task A", IsProcessed: false},
				{Text: "Task B", IsProcessed: false},
			},
			wantCount: 2,
			wantTexts: []string{"Task A", "Task B"},
		},
		{
			name: "mixed",
			checkboxes: []*docs.CheckboxItem{
				{Text: "Done task", IsProcessed: true},
				{Text: "Pending task", IsProcessed: false},
				{Text: "Another done", IsProcessed: true},
				{Text: "Another pending", IsProcessed: false},
			},
			wantCount: 2,
			wantTexts: []string{"Pending task", "Another pending"},
		},
		{
			name:       "empty input",
			checkboxes: []*docs.CheckboxItem{},
			wantCount:  0,
		},
		{
			name:       "nil input",
			checkboxes: nil,
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractUnassignedItems(tt.checkboxes)

			if len(result) != tt.wantCount {
				t.Fatalf("extractUnassignedItems: got %d items, want %d", len(result), tt.wantCount)
			}

			for i, wantText := range tt.wantTexts {
				if i >= len(result) {
					break
				}
				if result[i].Text != wantText {
					t.Errorf("item[%d].Text: got %q, want %q", i, result[i].Text, wantText)
				}
			}

			// Contract: indices are correct (0-based position in original slice)
			for _, item := range result {
				if item.Index < 0 || item.Index >= len(tt.checkboxes) {
					t.Errorf("item index %d out of range [0, %d)", item.Index, len(tt.checkboxes))
				}
			}
		})
	}
}
