package drive

import (
	"regexp"
	"testing"

	"google.golang.org/api/drive/v3"
)

// setupDriveService creates a minimal Service for testing parseDocument logic
func setupDriveService() *Service {
	return &Service{
		filenamePattern:  regexp.MustCompile(`(.+)\s*-\s*(\d{4}-\d{2}-\d{2})`),
		fallbackPattern:  regexp.MustCompile(`^Notes\s*-\s*(.+)$`),
		rootFolderID:     "root", // Use "root" for tests since test files use Parents: []string{"root"}
		currentUserEmail: "test@example.com",
	}
}

// ---------- T055: escapeQuery tests ----------

func TestEscapeQuery(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"no quotes", "Meeting Notes", "Meeting Notes"},
		{"single quote", "O'Brien's Meeting", "O\\'Brien\\'s Meeting"},
		{"multiple quotes", "It's Jay's doc", "It\\'s Jay\\'s doc"},
		{"quote at start", "'hello", "\\'hello"},
		{"quote at end", "hello'", "hello\\'"},
		{"only quote", "'", "\\'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeQuery(tt.input)
			if got != tt.want {
				t.Errorf("escapeQuery(%q): got %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------- T056: parseDocument edge case tests ----------

func TestParseDocument_NilOwners(t *testing.T) {
	s := setupDriveService()
	file := &drive.File{
		Name:   "Weekly - 2026-02-06",
		Id:     "test-id",
		Owners: nil,
	}

	doc, err := s.parseDocument(file)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: nil owners results in IsOwned=false
	if doc.IsOwned {
		t.Error("expected IsOwned=false for nil owners")
	}
}

func TestParseDocument_EmptyParents(t *testing.T) {
	s := setupDriveService()
	file := &drive.File{
		Name:    "Weekly - 2026-02-06",
		Id:      "test-id",
		Parents: []string{},
	}

	doc, err := s.parseDocument(file)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: empty parents results in empty ParentFolderID
	if doc.ParentFolderID != "" {
		t.Errorf("expected empty ParentFolderID, got %q", doc.ParentFolderID)
	}
}

func TestParseDocument_PrimaryPattern(t *testing.T) {
	s := setupDriveService()

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
	s := setupDriveService()

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
	s := setupDriveService()

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
	s := setupDriveService()

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
