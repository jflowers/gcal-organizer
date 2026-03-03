package calendar

import (
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"
)

// ---------- T037-T044: parseEvent pure function tests ----------

func TestParseEvent_DateTimeFormat(t *testing.T) {
	s := &Service{}
	event := &calendar.Event{
		Id:      "evt1",
		Summary: "Weekly Standup",
		Start:   &calendar.EventDateTime{DateTime: "2026-03-01T10:00:00Z"},
		End:     &calendar.EventDateTime{DateTime: "2026-03-01T11:00:00Z"},
	}

	result, err := s.parseEvent(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: RFC3339 DateTime parsed correctly
	wantStart := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	if !result.Start.Equal(wantStart) {
		t.Errorf("Start: got %v, want %v", result.Start, wantStart)
	}
	wantEnd := time.Date(2026, 3, 1, 11, 0, 0, 0, time.UTC)
	if !result.End.Equal(wantEnd) {
		t.Errorf("End: got %v, want %v", result.End, wantEnd)
	}
	if result.Title != "Weekly Standup" {
		t.Errorf("Title: got %q, want %q", result.Title, "Weekly Standup")
	}
	if result.ID != "evt1" {
		t.Errorf("ID: got %q, want %q", result.ID, "evt1")
	}
}

func TestParseEvent_DateOnlyFormat(t *testing.T) {
	s := &Service{}
	event := &calendar.Event{
		Id:      "evt-allday",
		Summary: "All Day Event",
		Start:   &calendar.EventDateTime{Date: "2026-03-01"},
		End:     &calendar.EventDateTime{Date: "2026-03-02"},
	}

	result, err := s.parseEvent(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: Date-only format parsed as midnight UTC
	wantStart := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if !result.Start.Equal(wantStart) {
		t.Errorf("Start: got %v, want %v", result.Start, wantStart)
	}
	wantEnd := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	if !result.End.Equal(wantEnd) {
		t.Errorf("End: got %v, want %v", result.End, wantEnd)
	}
}

func TestParseEvent_InvalidDateTime(t *testing.T) {
	s := &Service{}
	tests := []struct {
		name  string
		event *calendar.Event
	}{
		{
			name: "invalid start datetime",
			event: &calendar.Event{
				Id:      "evt-bad-start",
				Summary: "Bad Event",
				Start:   &calendar.EventDateTime{DateTime: "not-a-date"},
				End:     &calendar.EventDateTime{DateTime: "2026-03-01T11:00:00Z"},
			},
		},
		{
			name: "invalid end datetime",
			event: &calendar.Event{
				Id:      "evt-bad-end",
				Summary: "Bad Event",
				Start:   &calendar.EventDateTime{DateTime: "2026-03-01T10:00:00Z"},
				End:     &calendar.EventDateTime{DateTime: "garbage"},
			},
		},
		{
			name: "invalid start date",
			event: &calendar.Event{
				Id:      "evt-bad-date",
				Summary: "Bad Event",
				Start:   &calendar.EventDateTime{Date: "March-1"},
				End:     &calendar.EventDateTime{Date: "2026-03-02"},
			},
		},
		{
			name: "invalid end date",
			event: &calendar.Event{
				Id:      "evt-bad-end-date",
				Summary: "Bad Event",
				Start:   &calendar.EventDateTime{Date: "2026-03-01"},
				End:     &calendar.EventDateTime{Date: "bad-date"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.parseEvent(tt.event)
			if err == nil {
				t.Error("expected error for invalid date/time")
			}
		})
	}
}

func TestParseEvent_Attachments(t *testing.T) {
	s := &Service{}
	event := &calendar.Event{
		Id:      "evt-att",
		Summary: "Meeting with Attachments",
		Start:   &calendar.EventDateTime{DateTime: "2026-03-01T10:00:00Z"},
		End:     &calendar.EventDateTime{DateTime: "2026-03-01T11:00:00Z"},
		Attachments: []*calendar.EventAttachment{
			{
				FileId:   "file-123",
				Title:    "Meeting Notes",
				MimeType: "application/vnd.google-apps.document",
				FileUrl:  "https://docs.google.com/document/d/file-123/edit",
			},
			{
				FileId:   "file-456",
				Title:    "Slides",
				MimeType: "application/vnd.google-apps.presentation",
				FileUrl:  "https://docs.google.com/presentation/d/file-456/edit",
			},
		},
	}

	result, err := s.parseEvent(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: all attachment fields mapped correctly
	if len(result.Attachments) < 2 {
		t.Fatalf("expected at least 2 attachments, got %d", len(result.Attachments))
	}
	att := result.Attachments[0]
	if att.FileID != "file-123" {
		t.Errorf("Attachment[0].FileID: got %q, want %q", att.FileID, "file-123")
	}
	if att.Title != "Meeting Notes" {
		t.Errorf("Attachment[0].Title: got %q, want %q", att.Title, "Meeting Notes")
	}
	if att.MimeType != "application/vnd.google-apps.document" {
		t.Errorf("Attachment[0].MimeType: got %q", att.MimeType)
	}
	if att.FileURL != "https://docs.google.com/document/d/file-123/edit" {
		t.Errorf("Attachment[0].FileURL: got %q", att.FileURL)
	}
}

func TestParseEvent_DescriptionDriveLinks(t *testing.T) {
	s := &Service{}
	event := &calendar.Event{
		Id:          "evt-desc",
		Summary:     "Meeting",
		Start:       &calendar.EventDateTime{DateTime: "2026-03-01T10:00:00Z"},
		End:         &calendar.EventDateTime{DateTime: "2026-03-01T11:00:00Z"},
		Description: "See the doc: https://docs.google.com/document/d/desc-doc-id/edit",
		Attachments: []*calendar.EventAttachment{
			{FileId: "explicit-att", Title: "Explicit", MimeType: "application/pdf"},
		},
	}

	result, err := s.parseEvent(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: explicit attachment + description link both extracted
	if len(result.Attachments) != 2 {
		t.Fatalf("expected 2 attachments (1 explicit + 1 from description), got %d", len(result.Attachments))
	}

	// First should be explicit
	if result.Attachments[0].FileID != "explicit-att" {
		t.Errorf("Attachment[0]: expected explicit attachment, got FileID=%q", result.Attachments[0].FileID)
	}
	// Second should be from description
	if result.Attachments[1].FileID != "desc-doc-id" {
		t.Errorf("Attachment[1]: expected description link, got FileID=%q", result.Attachments[1].FileID)
	}
}

func TestParseEvent_Attendees(t *testing.T) {
	s := &Service{}
	event := &calendar.Event{
		Id:      "evt-att",
		Summary: "Meeting",
		Start:   &calendar.EventDateTime{DateTime: "2026-03-01T10:00:00Z"},
		End:     &calendar.EventDateTime{DateTime: "2026-03-01T11:00:00Z"},
		Attendees: []*calendar.EventAttendee{
			{Email: "alice@example.com", DisplayName: "Alice", Self: false, Organizer: true},
			{Email: "bob@example.com", DisplayName: "Bob", Self: true, Organizer: false},
			{Email: "room@resource.calendar.google.com", DisplayName: "Room A"},
		},
	}

	result, err := s.parseEvent(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: all attendee fields mapped
	if len(result.Attendees) != 3 {
		t.Fatalf("expected 3 attendees, got %d", len(result.Attendees))
	}

	alice := result.Attendees[0]
	if alice.Email != "alice@example.com" {
		t.Errorf("Attendee[0].Email: got %q", alice.Email)
	}
	if alice.DisplayName != "Alice" {
		t.Errorf("Attendee[0].DisplayName: got %q", alice.DisplayName)
	}
	if !alice.IsOrganizer {
		t.Error("Attendee[0] should be organizer")
	}
	if alice.IsSelf {
		t.Error("Attendee[0] should not be self")
	}

	bob := result.Attendees[1]
	if !bob.IsSelf {
		t.Error("Attendee[1] should be self")
	}
}

func TestParseEvent_EmptyEvent(t *testing.T) {
	s := &Service{}
	event := &calendar.Event{
		Id:      "evt-empty",
		Summary: "Minimal",
		Start:   &calendar.EventDateTime{DateTime: "2026-03-01T10:00:00Z"},
		End:     &calendar.EventDateTime{DateTime: "2026-03-01T11:00:00Z"},
		// No attachments, no attendees, no description
	}

	result, err := s.parseEvent(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: no panics, empty slices
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Attachments) != 0 {
		t.Errorf("expected 0 attachments, got %d", len(result.Attachments))
	}
	if len(result.Attendees) != 0 {
		t.Errorf("expected 0 attendees, got %d", len(result.Attendees))
	}
	if result.Description != "" {
		t.Errorf("expected empty description, got %q", result.Description)
	}
}

func TestParseEvent_MixedDateFormats(t *testing.T) {
	// DateTime start with Date-only end shouldn't happen in practice
	// but parseEvent should handle it without panicking
	s := &Service{}
	event := &calendar.Event{
		Id:      "evt-mixed",
		Summary: "Mixed",
		Start:   &calendar.EventDateTime{DateTime: "2026-03-01T10:00:00Z"},
		End:     &calendar.EventDateTime{Date: "2026-03-02"},
	}

	result, err := s.parseEvent(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: both dates parse correctly
	if result.Start.IsZero() {
		t.Error("Start should not be zero")
	}
	if result.End.IsZero() {
		t.Error("End should not be zero")
	}
}

// ---------- extractDriveLinks tests ----------

func TestExtractDriveLinks(t *testing.T) {
	s := &Service{}

	tests := []struct {
		name      string
		text      string
		wantCount int
		wantIDs   []string
	}{
		{
			name:      "single doc link",
			text:      "Check out this doc: https://docs.google.com/document/d/1abc123XYZ/edit",
			wantCount: 1,
			wantIDs:   []string{"1abc123XYZ"},
		},
		{
			name:      "multiple links",
			text:      "Doc: https://docs.google.com/document/d/doc1 and Sheet: https://docs.google.com/spreadsheets/d/sheet2",
			wantCount: 2,
			wantIDs:   []string{"doc1", "sheet2"},
		},
		{
			name:      "drive file link",
			text:      "File: https://drive.google.com/file/d/fileID123/view",
			wantCount: 1,
			wantIDs:   []string{"fileID123"},
		},
		{
			name:      "presentation link",
			text:      "Slides: https://docs.google.com/presentation/d/slides456",
			wantCount: 1,
			wantIDs:   []string{"slides456"},
		},
		{
			name:      "no links",
			text:      "Just some regular text without any links",
			wantCount: 0,
			wantIDs:   nil,
		},
		{
			name:      "non-drive link",
			text:      "Check https://example.com/document/d/notadoc",
			wantCount: 0,
			wantIDs:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attachments := s.extractDriveLinks(tt.text)

			if len(attachments) != tt.wantCount {
				t.Fatalf("got %d attachments, want %d", len(attachments), tt.wantCount)
			}

			for i, wantID := range tt.wantIDs {
				if attachments[i].FileID != wantID {
					t.Errorf("attachment[%d].FileID = %q, want %q", i, attachments[i].FileID, wantID)
				}
			}
		})
	}
}
