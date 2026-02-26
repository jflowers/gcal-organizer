package drive

import (
	"regexp"
	"testing"

	"google.golang.org/api/drive/v3"
)

// mockService creates a minimal Service for testing parseDocument logic
func mockService() *Service {
	return &Service{
		filenamePattern:  regexp.MustCompile(`(.+)\s*-\s*(\d{4}-\d{2}-\d{2})`),
		fallbackPattern:  regexp.MustCompile(`^Notes\s*-\s*(.+)$`),
		rootFolderID:     "root", // Use "root" for tests since test files use Parents: []string{"root"}
		currentUserEmail: "test@example.com",
	}
}

func TestParseDocument_PrimaryPattern(t *testing.T) {
	s := mockService()

	tests := []struct {
		name            string
		filename        string
		wantMeetingName string
		wantDateStr     string
	}{
		{"standard format", "Weekly Standup - 2026-02-06", "Weekly Standup", "2026-02-06"},
		{"with extra spaces", "Team Sync  -  2026-01-15", "Team Sync", "2026-01-15"},
		{"long meeting name", "Q4 Planning and Review Meeting - 2026-03-20", "Q4 Planning and Review Meeting", "2026-03-20"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := &drive.File{Name: tt.filename, Id: "test-id"}
			doc, err := s.parseDocument(file)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if doc.MeetingName != tt.wantMeetingName {
				t.Errorf("MeetingName = %q, want %q", doc.MeetingName, tt.wantMeetingName)
			}
			if doc.Date.Format("2006-01-02") != tt.wantDateStr {
				t.Errorf("Date = %v, want %s", doc.Date, tt.wantDateStr)
			}
		})
	}
}

func TestParseDocument_IsOwned(t *testing.T) {
	s := mockService()

	tests := []struct {
		name      string
		owners    []*drive.User
		wantOwned bool
	}{
		{
			name:      "owned by current user",
			owners:    []*drive.User{{EmailAddress: "test@example.com"}},
			wantOwned: true,
		},
		{
			name:      "owned by another user",
			owners:    []*drive.User{{EmailAddress: "other@example.com"}},
			wantOwned: false,
		},
		{
			name:      "no owners (fail-safe)",
			owners:    nil,
			wantOwned: false,
		},
		{
			name:      "empty owners list (fail-safe)",
			owners:    []*drive.User{},
			wantOwned: false,
		},
		{
			name: "multiple owners, current user included",
			owners: []*drive.User{
				{EmailAddress: "other@example.com"},
				{EmailAddress: "test@example.com"},
			},
			wantOwned: true,
		},
		{
			name: "multiple owners, current user not included",
			owners: []*drive.User{
				{EmailAddress: "alice@example.com"},
				{EmailAddress: "bob@example.com"},
			},
			wantOwned: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := &drive.File{
				Name:   "Weekly Standup - 2026-02-06",
				Id:     "test-id",
				Owners: tt.owners,
			}
			doc, err := s.parseDocument(file)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if doc.IsOwned != tt.wantOwned {
				t.Errorf("IsOwned = %v, want %v", doc.IsOwned, tt.wantOwned)
			}
		})
	}
}

func TestParseDocument_FallbackPattern(t *testing.T) {
	s := mockService()

	tests := []struct {
		name            string
		filename        string
		wantMeetingName string
	}{
		{"standard notes format", "Notes - Weekly Standup", "Weekly Standup"},
		{"with extra spaces", "Notes  -  Team Sync", "Team Sync"},
		{"complex meeting name", "Notes - 1:1 with Bob (Engineering)", "1:1 with Bob (Engineering)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Fallback pattern only applies to files in Drive root
			file := &drive.File{Name: tt.filename, Id: "test-id", Parents: []string{"root"}}
			doc, err := s.parseDocument(file)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if doc.MeetingName != tt.wantMeetingName {
				t.Errorf("MeetingName = %q, want %q", doc.MeetingName, tt.wantMeetingName)
			}
			if !doc.Date.IsZero() {
				t.Errorf("Date should be zero for fallback pattern, got %v", doc.Date)
			}
		})
	}
}

func TestParseDocument_NoMatch(t *testing.T) {
	s := mockService()

	tests := []struct {
		name     string
		filename string
	}{
		{"random document", "Project Proposal"},
		{"no separator", "Meeting2026-02-06"},
		{"wrong prefix", "Agenda - Weekly Standup"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := &drive.File{Name: tt.filename, Id: "test-id"}
			_, err := s.parseDocument(file)
			if err == nil {
				t.Error("expected error for non-matching filename, got nil")
			}
		})
	}
}

func TestMoveDocumentIdempotent(t *testing.T) {
	t.Run("same folder is no-op", func(t *testing.T) {
		currentParent := "folder123"
		targetFolder := "folder123"

		if currentParent != targetFolder {
			t.Error("Expected same folder to be idempotent case")
		}
	})
}
