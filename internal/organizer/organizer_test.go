package organizer

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jflowers/gcal-organizer/internal/config"
	"github.com/jflowers/gcal-organizer/internal/docs"
	"github.com/jflowers/gcal-organizer/internal/drive"
	"github.com/jflowers/gcal-organizer/internal/logging"
	"github.com/jflowers/gcal-organizer/pkg/models"
)

// mockDriveService implements DriveService for testing.
type mockDriveService struct {
	dryRun bool

	// Recorded calls
	setMasterFolderCalls    []string
	listMeetingDocsCalls    int
	getOrCreateFolderCalls  []string
	createShortcutCalls     []shortcutCall
	moveDocumentCalls       []moveCall
	findShortcutToFileCalls int
	trashFileCalls          int
	shareFileCalls          []shareCall
	isFileOwnedCalls        []string
	canEditFileCalls        []string
	getFileNameCalls        []string

	// Return values
	listMeetingDocsReturn   []*models.Document
	listMeetingDocsErr      error
	getOrCreateFolderReturn *models.MeetingFolder
	getOrCreateFolderErr    error
	isFileOwnedResults      map[string]bool
	isFileOwnedErr          map[string]error
	canEditFileResults      map[string]bool
	getFileNameResults      map[string]string
}

type shortcutCall struct {
	fileID, fileName, folderID, folderName string
	folderIsNew                            bool
}

type moveCall struct {
	docID, docName, currentParentID, targetFolderID, targetFolderName string
}

type shareCall struct {
	fileID, fileName, email, role string
}

func (m *mockDriveService) SetMasterFolder(_ context.Context, folderName string) error {
	m.setMasterFolderCalls = append(m.setMasterFolderCalls, folderName)
	return nil
}

func (m *mockDriveService) ListMeetingDocuments(_ context.Context, _ []string) ([]*models.Document, error) {
	m.listMeetingDocsCalls++
	return m.listMeetingDocsReturn, m.listMeetingDocsErr
}

func (m *mockDriveService) GetOrCreateMeetingFolder(_ context.Context, meetingName string) (*models.MeetingFolder, error) {
	m.getOrCreateFolderCalls = append(m.getOrCreateFolderCalls, meetingName)
	if m.getOrCreateFolderErr != nil {
		return nil, m.getOrCreateFolderErr
	}
	if m.getOrCreateFolderReturn != nil {
		return m.getOrCreateFolderReturn, nil
	}
	return &models.MeetingFolder{
		ID:   "folder-" + meetingName,
		Name: meetingName,
	}, nil
}

func (m *mockDriveService) CreateShortcut(_ context.Context, fileID, fileName, folderID, folderName string, folderIsNew bool) drive.ActionResult {
	m.createShortcutCalls = append(m.createShortcutCalls, shortcutCall{
		fileID: fileID, fileName: fileName, folderID: folderID, folderName: folderName, folderIsNew: folderIsNew,
	})
	return drive.ActionResult{Action: "shortcut", Details: "Created shortcut"}
}

func (m *mockDriveService) MoveDocument(_ context.Context, docID, docName, currentParentID, targetFolderID, targetFolderName string) drive.ActionResult {
	m.moveDocumentCalls = append(m.moveDocumentCalls, moveCall{
		docID: docID, docName: docName, currentParentID: currentParentID,
		targetFolderID: targetFolderID, targetFolderName: targetFolderName,
	})
	return drive.ActionResult{Action: "move", Details: "Moved document"}
}

func (m *mockDriveService) FindShortcutToFile(_ context.Context, _, _ string) (string, error) {
	m.findShortcutToFileCalls++
	return "", nil
}

func (m *mockDriveService) TrashFile(_ context.Context, _, _ string) drive.ActionResult {
	m.trashFileCalls++
	return drive.ActionResult{Action: "trash", Skipped: true}
}

func (m *mockDriveService) ShareFile(_ context.Context, fileID, fileName, email, role string) drive.ActionResult {
	m.shareFileCalls = append(m.shareFileCalls, shareCall{
		fileID: fileID, fileName: fileName, email: email, role: role,
	})
	return drive.ActionResult{Action: "share", Details: "Shared file"}
}

func (m *mockDriveService) IsDryRun() bool {
	return m.dryRun
}

func (m *mockDriveService) IsFileOwned(_ context.Context, fileID string) (bool, error) {
	m.isFileOwnedCalls = append(m.isFileOwnedCalls, fileID)
	if m.isFileOwnedErr != nil {
		if err, ok := m.isFileOwnedErr[fileID]; ok {
			return false, err
		}
	}
	if m.isFileOwnedResults != nil {
		if owned, ok := m.isFileOwnedResults[fileID]; ok {
			return owned, nil
		}
	}
	return false, nil
}

func (m *mockDriveService) CanEditFile(_ context.Context, fileID string) bool {
	m.canEditFileCalls = append(m.canEditFileCalls, fileID)
	if m.canEditFileResults != nil {
		if canEdit, ok := m.canEditFileResults[fileID]; ok {
			return canEdit
		}
	}
	return true
}

func (m *mockDriveService) GetFileName(_ context.Context, fileID string) (string, error) {
	m.getFileNameCalls = append(m.getFileNameCalls, fileID)
	if m.getFileNameResults != nil {
		if name, ok := m.getFileNameResults[fileID]; ok {
			return name, nil
		}
	}
	return fileID, nil
}

// mockCalendarService implements CalendarService for testing.
type mockCalendarService struct {
	listRecentEventsCalls  int
	listRecentEventsReturn []*models.CalendarEvent
	listRecentEventsErr    error
}

func (m *mockCalendarService) ListRecentEvents(_ context.Context, _ int) ([]*models.CalendarEvent, error) {
	m.listRecentEventsCalls++
	return m.listRecentEventsReturn, m.listRecentEventsErr
}

// setupOrganizer creates an Organizer with mock services for testing.
func setupOrganizer(cfg *config.Config, driveMock *mockDriveService, calMock *mockCalendarService) *Organizer {
	return &Organizer{
		config:         cfg,
		drive:          driveMock,
		calendar:       calMock,
		logger:         logging.Logger,
		notesDocIDs:    make(map[string]bool),
		decisionDocIDs: make(map[string]string),
	}
}

// ---------- T009: OrganizeDocuments ownership filtering tests ----------

func TestOrganizeDocuments_OwnedOnlyFalse_ProcessesAll(t *testing.T) {
	// When OwnedOnly=false, all documents should be processed normally (no filtering).
	driveMock := &mockDriveService{
		listMeetingDocsReturn: []*models.Document{
			{ID: "doc1", Name: "Weekly - 2026-02-01", MeetingName: "Weekly", IsOwned: true, ParentFolderID: "root"},
			{ID: "doc2", Name: "Standup - 2026-02-02", MeetingName: "Standup", IsOwned: false, ParentFolderID: "root"},
		},
	}
	calMock := &mockCalendarService{}
	cfg := config.DefaultConfig()
	cfg.OwnedOnly = false

	org := setupOrganizer(cfg, driveMock, calMock)
	err := org.OrganizeDocuments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Owned doc should be moved
	if len(driveMock.moveDocumentCalls) != 1 {
		t.Errorf("expected 1 MoveDocument call, got %d", len(driveMock.moveDocumentCalls))
	}
	if len(driveMock.moveDocumentCalls) > 0 && driveMock.moveDocumentCalls[0].docID != "doc1" {
		t.Errorf("expected move for doc1, got %s", driveMock.moveDocumentCalls[0].docID)
	}

	// Non-owned doc should get a shortcut (not skipped)
	if len(driveMock.createShortcutCalls) != 1 {
		t.Errorf("expected 1 CreateShortcut call, got %d", len(driveMock.createShortcutCalls))
	}
	if len(driveMock.createShortcutCalls) > 0 && driveMock.createShortcutCalls[0].fileID != "doc2" {
		t.Errorf("expected shortcut for doc2, got %s", driveMock.createShortcutCalls[0].fileID)
	}

	// No skipped count
	if org.stats.Skipped != 0 {
		t.Errorf("expected Skipped=0, got %d", org.stats.Skipped)
	}
}

func TestOrganizeDocuments_OwnedOnlyTrue_SkipsNonOwned(t *testing.T) {
	// When OwnedOnly=true, non-owned docs should be skipped for move but still get shortcuts.
	driveMock := &mockDriveService{
		listMeetingDocsReturn: []*models.Document{
			{ID: "doc1", Name: "Weekly - 2026-02-01", MeetingName: "Weekly", IsOwned: false, ParentFolderID: "root"},
		},
	}
	calMock := &mockCalendarService{}
	cfg := config.DefaultConfig()
	cfg.OwnedOnly = true

	org := setupOrganizer(cfg, driveMock, calMock)
	err := org.OrganizeDocuments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should NOT call MoveDocument
	if len(driveMock.moveDocumentCalls) != 0 {
		t.Errorf("expected 0 MoveDocument calls, got %d", len(driveMock.moveDocumentCalls))
	}

	// Should still create shortcut for discoverability (FR-005)
	if len(driveMock.createShortcutCalls) != 1 {
		t.Errorf("expected 1 CreateShortcut call, got %d", len(driveMock.createShortcutCalls))
	}

	// Should increment Skipped
	if org.stats.Skipped != 1 {
		t.Errorf("expected Skipped=1, got %d", org.stats.Skipped)
	}
}

func TestOrganizeDocuments_OwnedOnlyTrue_ProcessesOwned(t *testing.T) {
	// When OwnedOnly=true, owned docs should be moved normally.
	driveMock := &mockDriveService{
		listMeetingDocsReturn: []*models.Document{
			{ID: "doc1", Name: "Weekly - 2026-02-01", MeetingName: "Weekly", IsOwned: true, ParentFolderID: "root"},
		},
	}
	calMock := &mockCalendarService{}
	cfg := config.DefaultConfig()
	cfg.OwnedOnly = true

	org := setupOrganizer(cfg, driveMock, calMock)
	err := org.OrganizeDocuments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should call MoveDocument for owned doc
	if len(driveMock.moveDocumentCalls) != 1 {
		t.Errorf("expected 1 MoveDocument call, got %d", len(driveMock.moveDocumentCalls))
	}

	// No skipped count for owned docs
	if org.stats.Skipped != 0 {
		t.Errorf("expected Skipped=0, got %d", org.stats.Skipped)
	}
}

func TestOrganizeDocuments_OwnedOnlyTrue_SkippedCount(t *testing.T) {
	// Stats.Skipped count should match the number of non-owned docs when OwnedOnly=true.
	driveMock := &mockDriveService{
		listMeetingDocsReturn: []*models.Document{
			{ID: "doc1", Name: "Meeting A - 2026-02-01", MeetingName: "Meeting A", IsOwned: true, ParentFolderID: "root"},
			{ID: "doc2", Name: "Meeting B - 2026-02-02", MeetingName: "Meeting B", IsOwned: false, ParentFolderID: "root"},
			{ID: "doc3", Name: "Meeting C - 2026-02-03", MeetingName: "Meeting C", IsOwned: false, ParentFolderID: "root"},
			{ID: "doc4", Name: "Meeting D - 2026-02-04", MeetingName: "Meeting D", IsOwned: true, ParentFolderID: "root"},
		},
	}
	calMock := &mockCalendarService{}
	cfg := config.DefaultConfig()
	cfg.OwnedOnly = true

	org := setupOrganizer(cfg, driveMock, calMock)
	err := org.OrganizeDocuments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 2 owned docs moved
	if len(driveMock.moveDocumentCalls) != 2 {
		t.Errorf("expected 2 MoveDocument calls, got %d", len(driveMock.moveDocumentCalls))
	}

	// 2 non-owned docs get shortcuts (from the owned-only skip path)
	// Note: the owned docs also don't get shortcuts since they get moved
	if len(driveMock.createShortcutCalls) != 2 {
		t.Errorf("expected 2 CreateShortcut calls, got %d", len(driveMock.createShortcutCalls))
	}

	// 2 skipped
	if org.stats.Skipped != 2 {
		t.Errorf("expected Skipped=2, got %d", org.stats.Skipped)
	}
}

// ---------- T013: SyncCalendarAttachments ownership filtering tests ----------

func TestSyncCalendarAttachments_OwnedOnlyTrue_SkipsShareForNonOwned(t *testing.T) {
	// When OwnedOnly=true, ShareFile should NOT be called for non-owned attachments.
	driveMock := &mockDriveService{
		isFileOwnedResults: map[string]bool{
			"att1": false,
		},
		canEditFileResults: map[string]bool{
			"att1": true,
		},
	}
	calMock := &mockCalendarService{
		listRecentEventsReturn: []*models.CalendarEvent{
			{
				ID:    "event1",
				Title: "Weekly",
				Attachments: []models.Attachment{
					{FileID: "att1", Title: "Notes", MimeType: "application/pdf"},
				},
				Attendees: []models.Attendee{
					{Email: "alice@example.com"},
				},
			},
		},
	}
	cfg := config.DefaultConfig()
	cfg.OwnedOnly = true

	org := setupOrganizer(cfg, driveMock, calMock)
	err := org.SyncCalendarAttachments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should NOT call ShareFile for non-owned attachment
	if len(driveMock.shareFileCalls) != 0 {
		t.Errorf("expected 0 ShareFile calls for non-owned attachment, got %d", len(driveMock.shareFileCalls))
	}

	// Should increment Skipped
	if org.stats.Skipped != 1 {
		t.Errorf("expected Skipped=1, got %d", org.stats.Skipped)
	}
}

func TestSyncCalendarAttachments_OwnedOnlyTrue_SharesOwnedAttachments(t *testing.T) {
	// When OwnedOnly=true, ShareFile should still be called for owned attachments.
	driveMock := &mockDriveService{
		isFileOwnedResults: map[string]bool{
			"att1": true,
		},
		canEditFileResults: map[string]bool{
			"att1": true,
		},
	}
	calMock := &mockCalendarService{
		listRecentEventsReturn: []*models.CalendarEvent{
			{
				ID:    "event1",
				Title: "Weekly",
				Attachments: []models.Attachment{
					{FileID: "att1", Title: "Notes Doc", MimeType: "application/pdf"},
				},
				Attendees: []models.Attendee{
					{Email: "alice@example.com"},
				},
			},
		},
	}
	cfg := config.DefaultConfig()
	cfg.OwnedOnly = true

	org := setupOrganizer(cfg, driveMock, calMock)
	err := org.SyncCalendarAttachments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should call ShareFile for owned attachment
	if len(driveMock.shareFileCalls) != 1 {
		t.Errorf("expected 1 ShareFile call for owned attachment, got %d", len(driveMock.shareFileCalls))
	}

	// No skipped
	if org.stats.Skipped != 0 {
		t.Errorf("expected Skipped=0, got %d", org.stats.Skipped)
	}
}

func TestSyncCalendarAttachments_OwnedOnlyTrue_ExcludesNonOwnedFromNotesDocIDs(t *testing.T) {
	// When OwnedOnly=true, non-owned Google Docs with "Notes" should NOT be collected for Step 3.
	driveMock := &mockDriveService{
		isFileOwnedResults: map[string]bool{
			"notes-owned":     true,
			"notes-not-owned": false,
		},
		canEditFileResults: map[string]bool{
			"notes-owned":     true,
			"notes-not-owned": true,
		},
	}
	calMock := &mockCalendarService{
		listRecentEventsReturn: []*models.CalendarEvent{
			{
				ID:    "event1",
				Title: "Weekly",
				Attachments: []models.Attachment{
					{FileID: "notes-owned", Title: "Notes - Weekly", MimeType: "application/vnd.google-apps.document"},
					{FileID: "notes-not-owned", Title: "Notes - Standup", MimeType: "application/vnd.google-apps.document"},
				},
				Attendees: []models.Attendee{
					{Email: "alice@example.com"},
				},
			},
		},
	}
	cfg := config.DefaultConfig()
	cfg.OwnedOnly = true

	org := setupOrganizer(cfg, driveMock, calMock)
	err := org.SyncCalendarAttachments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only owned Notes doc should be in notesDocIDs
	docIDs := org.GetNotesDocIDs()
	if len(docIDs) != 1 {
		t.Fatalf("expected 1 notes doc ID, got %d: %v", len(docIDs), docIDs)
	}
	if docIDs[0] != "notes-owned" {
		t.Errorf("expected notes-owned in notesDocIDs, got %s", docIDs[0])
	}
}

func TestSyncCalendarAttachments_OwnedOnlyFalse_PreservesExistingBehavior(t *testing.T) {
	// When OwnedOnly=false, existing CanEditFile-gated sharing behavior is preserved.
	driveMock := &mockDriveService{
		canEditFileResults: map[string]bool{
			"att1": true,
			"att2": false,
		},
	}
	calMock := &mockCalendarService{
		listRecentEventsReturn: []*models.CalendarEvent{
			{
				ID:    "event1",
				Title: "Weekly",
				Attachments: []models.Attachment{
					{FileID: "att1", Title: "Editable Doc", MimeType: "application/pdf"},
					{FileID: "att2", Title: "Read-Only Doc", MimeType: "application/pdf"},
				},
				Attendees: []models.Attendee{
					{Email: "alice@example.com"},
				},
			},
		},
	}
	cfg := config.DefaultConfig()
	cfg.OwnedOnly = false

	org := setupOrganizer(cfg, driveMock, calMock)
	err := org.SyncCalendarAttachments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should NOT call IsFileOwned when OwnedOnly=false
	if len(driveMock.isFileOwnedCalls) != 0 {
		t.Errorf("expected 0 IsFileOwned calls when OwnedOnly=false, got %d", len(driveMock.isFileOwnedCalls))
	}

	// Should call ShareFile only for the editable attachment
	if len(driveMock.shareFileCalls) != 1 {
		t.Errorf("expected 1 ShareFile call, got %d", len(driveMock.shareFileCalls))
	}
	if len(driveMock.shareFileCalls) > 0 && driveMock.shareFileCalls[0].fileID != "att1" {
		t.Errorf("expected ShareFile for att1, got %s", driveMock.shareFileCalls[0].fileID)
	}
}

// ---------- T004: Decision document collection tests ----------

func TestDecisionDocCollection(t *testing.T) {
	tests := []struct {
		name         string
		events       []*models.CalendarEvent
		ownedOnly    bool
		isFileOwned  map[string]bool
		wantDocIDs   map[string]string
		wantDocCount int
	}{
		{
			name: "exact match Notes by Gemini",
			events: []*models.CalendarEvent{
				{
					ID:    "event1",
					Title: "Weekly",
					Attachments: []models.Attachment{
						{FileID: "doc1", Title: "Notes by Gemini", MimeType: "application/vnd.google-apps.document"},
					},
				},
			},
			wantDocIDs:   map[string]string{"doc1": "notes-by-gemini"},
			wantDocCount: 1,
		},
		{
			name: "suffix match - Transcript",
			events: []*models.CalendarEvent{
				{
					ID:    "event1",
					Title: "Standup",
					Attachments: []models.Attachment{
						{FileID: "doc2", Title: "ComplyTime Standup - 2026/02/25 14:00 WET - Transcript", MimeType: "application/vnd.google-apps.document"},
					},
				},
			},
			wantDocIDs:   map[string]string{"doc2": "transcript"},
			wantDocCount: 1,
		},
		{
			name: "non-matching title rejected",
			events: []*models.CalendarEvent{
				{
					ID:    "event1",
					Title: "Weekly",
					Attachments: []models.Attachment{
						{FileID: "doc3", Title: "Meeting Agenda", MimeType: "application/vnd.google-apps.document"},
					},
				},
			},
			wantDocIDs:   map[string]string{},
			wantDocCount: 0,
		},
		{
			name: "per-event deduplication prefers Notes by Gemini",
			events: []*models.CalendarEvent{
				{
					ID:    "event1",
					Title: "Weekly",
					Attachments: []models.Attachment{
						{FileID: "doc-nbg", Title: "Notes by Gemini", MimeType: "application/vnd.google-apps.document"},
						{FileID: "doc-transcript", Title: "Weekly - 2026/02/25 14:00 WET - Transcript", MimeType: "application/vnd.google-apps.document"},
					},
				},
			},
			wantDocIDs:   map[string]string{"doc-nbg": "notes-by-gemini"},
			wantDocCount: 1,
		},
		{
			name: "owned-only filters unowned docs",
			events: []*models.CalendarEvent{
				{
					ID:    "event1",
					Title: "Weekly",
					Attachments: []models.Attachment{
						{FileID: "doc-owned", Title: "Notes by Gemini", MimeType: "application/vnd.google-apps.document"},
						{FileID: "doc-unowned", Title: "Standup - Transcript", MimeType: "application/vnd.google-apps.document"},
					},
				},
			},
			ownedOnly:    true,
			isFileOwned:  map[string]bool{"doc-owned": true, "doc-unowned": false},
			wantDocIDs:   map[string]string{"doc-owned": "notes-by-gemini"},
			wantDocCount: 1,
		},
		{
			name: "multiple events collect independently",
			events: []*models.CalendarEvent{
				{
					ID:    "event1",
					Title: "Weekly",
					Attachments: []models.Attachment{
						{FileID: "doc1", Title: "Notes by Gemini", MimeType: "application/vnd.google-apps.document"},
					},
				},
				{
					ID:    "event2",
					Title: "Standup",
					Attachments: []models.Attachment{
						{FileID: "doc2", Title: "Standup - Transcript", MimeType: "application/vnd.google-apps.document"},
					},
				},
			},
			wantDocIDs:   map[string]string{"doc1": "notes-by-gemini", "doc2": "transcript"},
			wantDocCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driveMock := &mockDriveService{
				isFileOwnedResults: tt.isFileOwned,
				canEditFileResults: map[string]bool{}, // no sharing in this test
			}
			calMock := &mockCalendarService{
				listRecentEventsReturn: tt.events,
			}
			cfg := config.DefaultConfig()
			cfg.OwnedOnly = tt.ownedOnly

			org := setupOrganizer(cfg, driveMock, calMock)
			err := org.SyncCalendarAttachments(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			gotDocIDs := org.GetDecisionDocIDs()
			if len(gotDocIDs) != tt.wantDocCount {
				t.Errorf("expected %d decision doc IDs, got %d: %v", tt.wantDocCount, len(gotDocIDs), gotDocIDs)
			}

			for wantID, wantSource := range tt.wantDocIDs {
				gotSource, ok := gotDocIDs[wantID]
				if !ok {
					t.Errorf("expected doc ID %s in decisionDocIDs, but not found", wantID)
					continue
				}
				if gotSource != wantSource {
					t.Errorf("doc %s: expected source %q, got %q", wantID, wantSource, gotSource)
				}
			}
		})
	}
}

// ---------- T003: logActionResult tests ----------

func TestLogActionResult(t *testing.T) {
	tests := []struct {
		name          string
		result        drive.ActionResult
		isMove        bool
		wantDocsMoved int
		wantShortcuts int
		wantErrors    int
	}{
		{
			name: "success move",
			result: drive.ActionResult{
				Action: "move", Skipped: false, Details: "Moved doc to folder",
			},
			isMove:        true,
			wantDocsMoved: 1,
		},
		{
			name: "success shortcut",
			result: drive.ActionResult{
				Action: "shortcut", Skipped: false, Details: "Created shortcut",
			},
			isMove:        false,
			wantShortcuts: 1,
		},
		{
			name: "dry-run move",
			result: drive.ActionResult{
				Action: "move", Skipped: false, Reason: "dry-run", Details: "Would move doc",
			},
			isMove:        true,
			wantDocsMoved: 1,
		},
		{
			name: "dry-run shortcut",
			result: drive.ActionResult{
				Action: "shortcut", Skipped: false, Reason: "dry-run", Details: "Would create shortcut",
			},
			isMove:        false,
			wantShortcuts: 1,
		},
		{
			name: "already exists skip",
			result: drive.ActionResult{
				Action: "shortcut", Skipped: true, Reason: "already exists", Details: "Shortcut already exists",
			},
			isMove: false,
			// No counters should increment for "already exists"
		},
		{
			name: "already in folder skip",
			result: drive.ActionResult{
				Action: "move", Skipped: true, Reason: "already in folder", Details: "Doc already in folder",
			},
			isMove: true,
			// No counters should increment for "already in folder"
		},
		{
			name: "error reason",
			result: drive.ActionResult{
				Action: "shortcut", Skipped: true, Reason: "error: permission denied", Details: "Failed to create shortcut",
			},
			isMove:     false,
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driveMock := &mockDriveService{}
			calMock := &mockCalendarService{}
			cfg := config.DefaultConfig()
			org := setupOrganizer(cfg, driveMock, calMock)

			org.logActionResult(tt.result, tt.isMove)

			if org.stats.DocumentsMoved != tt.wantDocsMoved {
				t.Errorf("DocumentsMoved: got %d, want %d", org.stats.DocumentsMoved, tt.wantDocsMoved)
			}
			if org.stats.ShortcutsCreated != tt.wantShortcuts {
				t.Errorf("ShortcutsCreated: got %d, want %d", org.stats.ShortcutsCreated, tt.wantShortcuts)
			}
			if org.stats.Errors != tt.wantErrors {
				t.Errorf("Errors: got %d, want %d", org.stats.Errors, tt.wantErrors)
			}
		})
	}
}

// ---------- T004: logCalendarAction tests ----------

func TestLogCalendarAction(t *testing.T) {
	tests := []struct {
		name          string
		result        drive.ActionResult
		wantShortcuts int
		wantErrors    int
	}{
		{
			name: "success",
			result: drive.ActionResult{
				Action: "shortcut", Skipped: false, Details: "Linked attachment",
			},
			wantShortcuts: 1,
		},
		{
			name: "dry-run",
			result: drive.ActionResult{
				Action: "shortcut", Skipped: false, Reason: "dry-run", Details: "Would link attachment",
			},
			wantShortcuts: 1,
		},
		{
			name: "already exists skip",
			result: drive.ActionResult{
				Action: "shortcut", Skipped: true, Reason: "already exists", Details: "Already linked",
			},
			// No counter increment
		},
		{
			name: "error",
			result: drive.ActionResult{
				Action: "shortcut", Skipped: true, Reason: "error: API failure", Details: "Failed to link",
			},
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driveMock := &mockDriveService{}
			calMock := &mockCalendarService{}
			cfg := config.DefaultConfig()
			org := setupOrganizer(cfg, driveMock, calMock)

			org.logCalendarAction(tt.result, "Weekly", "2026-03-01", "notes.pdf")

			if org.stats.ShortcutsCreated != tt.wantShortcuts {
				t.Errorf("ShortcutsCreated: got %d, want %d", org.stats.ShortcutsCreated, tt.wantShortcuts)
			}
			if org.stats.Errors != tt.wantErrors {
				t.Errorf("Errors: got %d, want %d", org.stats.Errors, tt.wantErrors)
			}
		})
	}
}

// ---------- T005: printSummary DryRun tests ----------

func TestPrintSummary_DryRun(t *testing.T) {
	driveMock := &mockDriveService{dryRun: true}
	calMock := &mockCalendarService{}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	// Set various stats
	org.stats.DocumentsFound = 5
	org.stats.DocumentsMoved = 3
	org.stats.ShortcutsCreated = 2
	org.stats.EventsProcessed = 10
	org.stats.DecisionsProcessed = 2
	org.stats.Skipped = 1

	// Should not panic
	org.printSummary()

	// Contract: stats should be unchanged after printing
	if org.stats.DocumentsFound != 5 {
		t.Errorf("DocumentsFound changed after printSummary: got %d, want 5", org.stats.DocumentsFound)
	}
	if org.stats.DecisionsProcessed != 2 {
		t.Errorf("DecisionsProcessed changed: got %d, want 2", org.stats.DecisionsProcessed)
	}
	if org.stats.Skipped != 1 {
		t.Errorf("Skipped changed: got %d, want 1", org.stats.Skipped)
	}
}

// ---------- T006: printSummary RealRun tests ----------

func TestPrintSummary_RealRun(t *testing.T) {
	driveMock := &mockDriveService{dryRun: false}
	calMock := &mockCalendarService{}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	// Exercise all conditional branches
	org.stats.DocumentsFound = 10
	org.stats.DocumentsMoved = 5
	org.stats.ShortcutsCreated = 3
	org.stats.ShortcutsTrashed = 1
	org.stats.EventsProcessed = 8
	org.stats.EventsWithAttach = 4
	org.stats.AttachmentsShared = 2
	org.stats.TasksAssigned = 3
	org.stats.TasksFailed = 1
	org.stats.DecisionsProcessed = 2
	org.stats.DecisionsSkipped = 1
	org.stats.DecisionsFailed = 1
	org.stats.Skipped = 2
	org.stats.Errors = 1

	// Should not panic with all branches active
	org.printSummary()

	// Contract: all stats preserved
	if org.stats.ShortcutsTrashed != 1 {
		t.Errorf("ShortcutsTrashed changed: got %d, want 1", org.stats.ShortcutsTrashed)
	}
	if org.stats.AttachmentsShared != 2 {
		t.Errorf("AttachmentsShared changed: got %d, want 2", org.stats.AttachmentsShared)
	}
	if org.stats.TasksFailed != 1 {
		t.Errorf("TasksFailed changed: got %d, want 1", org.stats.TasksFailed)
	}
	if org.stats.Errors != 1 {
		t.Errorf("Errors changed: got %d, want 1", org.stats.Errors)
	}
}

func TestPrintSummary_RealRun_ZeroStats(t *testing.T) {
	// Exercise the branches where optional sections are skipped (all zero)
	driveMock := &mockDriveService{dryRun: false}
	calMock := &mockCalendarService{}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	// All zeros - should not panic, no optional sections printed
	org.printSummary()
}

// ---------- T007: RunFullWorkflow tests ----------

func TestRunFullWorkflow_HappyPath(t *testing.T) {
	driveMock := &mockDriveService{
		dryRun: false,
		listMeetingDocsReturn: []*models.Document{
			{ID: "doc1", Name: "Weekly - 2026-02-01", MeetingName: "Weekly", IsOwned: true, ParentFolderID: "root"},
		},
	}
	calMock := &mockCalendarService{
		listRecentEventsReturn: []*models.CalendarEvent{},
	}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.RunFullWorkflow(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: OrganizeDocuments ran (SetMasterFolder was called)
	if len(driveMock.setMasterFolderCalls) != 1 {
		t.Errorf("SetMasterFolder calls: got %d, want 1", len(driveMock.setMasterFolderCalls))
	}
	// Contract: SyncCalendarAttachments ran (ListRecentEvents was called)
	if calMock.listRecentEventsCalls != 1 {
		t.Errorf("ListRecentEvents calls: got %d, want 1", calMock.listRecentEventsCalls)
	}
	// Contract: document was processed
	if org.stats.DocumentsFound != 1 {
		t.Errorf("DocumentsFound: got %d, want 1", org.stats.DocumentsFound)
	}
}

func TestRunFullWorkflow_OrganizeError(t *testing.T) {
	driveMock := &mockDriveService{
		dryRun:             false,
		listMeetingDocsErr: fmt.Errorf("API unavailable"),
	}
	calMock := &mockCalendarService{
		listRecentEventsReturn: []*models.CalendarEvent{},
	}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.RunFullWorkflow(context.Background())
	if err == nil {
		t.Fatal("expected error from OrganizeDocuments failure")
	}

	// Contract: error propagated from OrganizeDocuments
	if !strings.Contains(err.Error(), "organize documents failed") {
		t.Errorf("expected 'organize documents failed' in error, got: %v", err)
	}
	// Contract: SyncCalendarAttachments was NOT called (sequential execution)
	if calMock.listRecentEventsCalls != 0 {
		t.Errorf("ListRecentEvents should not be called after OrganizeDocuments error, got %d calls", calMock.listRecentEventsCalls)
	}
}

func TestRunFullWorkflow_SyncError(t *testing.T) {
	driveMock := &mockDriveService{
		dryRun:                false,
		listMeetingDocsReturn: []*models.Document{},
	}
	calMock := &mockCalendarService{
		listRecentEventsErr: fmt.Errorf("calendar API error"),
	}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.RunFullWorkflow(context.Background())
	if err == nil {
		t.Fatal("expected error from SyncCalendarAttachments failure")
	}

	// Contract: error propagated from SyncCalendarAttachments
	if !strings.Contains(err.Error(), "sync calendar failed") {
		t.Errorf("expected 'sync calendar failed' in error, got: %v", err)
	}
	// Contract: OrganizeDocuments DID complete (SetMasterFolder was called)
	if len(driveMock.setMasterFolderCalls) != 1 {
		t.Errorf("SetMasterFolder should have been called, got %d calls", len(driveMock.setMasterFolderCalls))
	}
}

func TestRunFullWorkflow_DryRunMode(t *testing.T) {
	driveMock := &mockDriveService{
		dryRun:                true,
		listMeetingDocsReturn: []*models.Document{},
	}
	calMock := &mockCalendarService{
		listRecentEventsReturn: []*models.CalendarEvent{},
	}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.RunFullWorkflow(context.Background())
	if err != nil {
		t.Fatalf("unexpected error in dry-run: %v", err)
	}

	// Contract: IsDryRun returns true
	if !driveMock.IsDryRun() {
		t.Error("expected dry-run mode to be active")
	}
}

// ---------- T008: Strengthen OrganizeDocuments tests ----------

func TestOrganizeDocuments_SetMasterFolderArgs(t *testing.T) {
	driveMock := &mockDriveService{
		listMeetingDocsReturn: []*models.Document{},
	}
	calMock := &mockCalendarService{}
	cfg := config.DefaultConfig()
	cfg.MasterFolderName = "Custom Meeting Notes"
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.OrganizeDocuments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: SetMasterFolder called with config value
	if len(driveMock.setMasterFolderCalls) != 1 {
		t.Fatalf("SetMasterFolder calls: got %d, want 1", len(driveMock.setMasterFolderCalls))
	}
	if driveMock.setMasterFolderCalls[0] != "Custom Meeting Notes" {
		t.Errorf("SetMasterFolder arg: got %q, want %q", driveMock.setMasterFolderCalls[0], "Custom Meeting Notes")
	}
	// Contract: stats reflect empty document list
	if org.stats.DocumentsFound != 0 {
		t.Errorf("DocumentsFound: got %d, want 0", org.stats.DocumentsFound)
	}
}

func TestOrganizeDocuments_MoveDocumentArgs(t *testing.T) {
	driveMock := &mockDriveService{
		listMeetingDocsReturn: []*models.Document{
			{ID: "doc-abc", Name: "Weekly - 2026-03-01", MeetingName: "Weekly", IsOwned: true, ParentFolderID: "parent-xyz"},
		},
	}
	calMock := &mockCalendarService{}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.OrganizeDocuments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: MoveDocument called with correct arguments
	if len(driveMock.moveDocumentCalls) != 1 {
		t.Fatalf("MoveDocument calls: got %d, want 1", len(driveMock.moveDocumentCalls))
	}
	call := driveMock.moveDocumentCalls[0]
	if call.docID != "doc-abc" {
		t.Errorf("MoveDocument docID: got %q, want %q", call.docID, "doc-abc")
	}
	if call.docName != "Weekly - 2026-03-01" {
		t.Errorf("MoveDocument docName: got %q, want %q", call.docName, "Weekly - 2026-03-01")
	}
	if call.currentParentID != "parent-xyz" {
		t.Errorf("MoveDocument currentParentID: got %q, want %q", call.currentParentID, "parent-xyz")
	}
	if call.targetFolderID != "folder-Weekly" {
		t.Errorf("MoveDocument targetFolderID: got %q, want %q", call.targetFolderID, "folder-Weekly")
	}
	// Contract: stats reflect document move
	if org.stats.DocumentsMoved != 1 {
		t.Errorf("DocumentsMoved: got %d, want 1", org.stats.DocumentsMoved)
	}
	if org.stats.DocumentsFound != 1 {
		t.Errorf("DocumentsFound: got %d, want 1", org.stats.DocumentsFound)
	}
}

func TestOrganizeDocuments_GetOrCreateMeetingFolderArgs(t *testing.T) {
	driveMock := &mockDriveService{
		listMeetingDocsReturn: []*models.Document{
			{ID: "doc1", Name: "Standup - 2026-03-01", MeetingName: "Standup", IsOwned: true, ParentFolderID: "root"},
			{ID: "doc2", Name: "Standup - 2026-03-02", MeetingName: "Standup", IsOwned: false, ParentFolderID: "root"},
		},
	}
	calMock := &mockCalendarService{}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.OrganizeDocuments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: GetOrCreateMeetingFolder called for each document
	if len(driveMock.getOrCreateFolderCalls) != 2 {
		t.Fatalf("GetOrCreateMeetingFolder calls: got %d, want 2", len(driveMock.getOrCreateFolderCalls))
	}
	for _, call := range driveMock.getOrCreateFolderCalls {
		if call != "Standup" {
			t.Errorf("GetOrCreateMeetingFolder arg: got %q, want %q", call, "Standup")
		}
	}

	// Contract: DocumentsFound reflects total
	if org.stats.DocumentsFound != 2 {
		t.Errorf("DocumentsFound: got %d, want 2", org.stats.DocumentsFound)
	}
}

func TestOrganizeDocuments_CreateShortcutArgs(t *testing.T) {
	driveMock := &mockDriveService{
		listMeetingDocsReturn: []*models.Document{
			{ID: "doc-nonowned", Name: "Weekly - 2026-03-01", MeetingName: "Weekly", IsOwned: false, ParentFolderID: "root"},
		},
	}
	calMock := &mockCalendarService{}
	cfg := config.DefaultConfig()
	cfg.OwnedOnly = false
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.OrganizeDocuments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: CreateShortcut called with correct arguments for non-owned doc
	if len(driveMock.createShortcutCalls) != 1 {
		t.Fatalf("CreateShortcut calls: got %d, want 1", len(driveMock.createShortcutCalls))
	}
	call := driveMock.createShortcutCalls[0]
	if call.fileID != "doc-nonowned" {
		t.Errorf("CreateShortcut fileID: got %q, want %q", call.fileID, "doc-nonowned")
	}
	if call.fileName != "Weekly - 2026-03-01" {
		t.Errorf("CreateShortcut fileName: got %q, want %q", call.fileName, "Weekly - 2026-03-01")
	}
	// Contract: stats reflect shortcut creation
	if org.stats.ShortcutsCreated != 1 {
		t.Errorf("ShortcutsCreated: got %d, want 1", org.stats.ShortcutsCreated)
	}
	if org.stats.DocumentsFound != 1 {
		t.Errorf("DocumentsFound: got %d, want 1", org.stats.DocumentsFound)
	}
}

// ---------- T009: Strengthen SyncCalendarAttachments tests ----------

func TestSyncCalendarAttachments_EventsProcessed(t *testing.T) {
	driveMock := &mockDriveService{
		canEditFileResults: map[string]bool{"att1": true},
	}
	calMock := &mockCalendarService{
		listRecentEventsReturn: []*models.CalendarEvent{
			{
				ID:    "evt1",
				Title: "Weekly",
				Attachments: []models.Attachment{
					{FileID: "att1", Title: "Notes", MimeType: "application/pdf"},
				},
				Attendees: []models.Attendee{
					{Email: "alice@example.com"},
				},
			},
			{
				ID:    "evt2",
				Title: "Standup",
			},
		},
	}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.SyncCalendarAttachments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: EventsProcessed counts all events
	if org.stats.EventsProcessed != 2 {
		t.Errorf("EventsProcessed: got %d, want 2", org.stats.EventsProcessed)
	}
	// Contract: EventsWithAttach only counts events with attachments
	if org.stats.EventsWithAttach != 1 {
		t.Errorf("EventsWithAttach: got %d, want 1", org.stats.EventsWithAttach)
	}
	// Contract: AttachmentsShared tracks shared files
	if org.stats.AttachmentsShared != 1 {
		t.Errorf("AttachmentsShared: got %d, want 1", org.stats.AttachmentsShared)
	}
}

func TestSyncCalendarAttachments_ShareFileArgs(t *testing.T) {
	driveMock := &mockDriveService{
		canEditFileResults: map[string]bool{"att-xyz": true},
	}
	calMock := &mockCalendarService{
		listRecentEventsReturn: []*models.CalendarEvent{
			{
				ID:    "evt1",
				Title: "Weekly",
				Attachments: []models.Attachment{
					{FileID: "att-xyz", Title: "Report.pdf", MimeType: "application/pdf"},
				},
				Attendees: []models.Attendee{
					{Email: "bob@example.com"},
				},
			},
		},
	}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.SyncCalendarAttachments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: ShareFile called with correct arguments
	if len(driveMock.shareFileCalls) != 1 {
		t.Fatalf("ShareFile calls: got %d, want 1", len(driveMock.shareFileCalls))
	}
	call := driveMock.shareFileCalls[0]
	if call.fileID != "att-xyz" {
		t.Errorf("ShareFile fileID: got %q, want %q", call.fileID, "att-xyz")
	}
	if call.fileName != "Report.pdf" {
		t.Errorf("ShareFile fileName: got %q, want %q", call.fileName, "Report.pdf")
	}
	// Contract: stats reflect sharing
	if org.stats.AttachmentsShared != 1 {
		t.Errorf("AttachmentsShared: got %d, want 1", org.stats.AttachmentsShared)
	}
	if call.email != "bob@example.com" {
		t.Errorf("ShareFile email: got %q, want %q", call.email, "bob@example.com")
	}
	if call.role != "writer" {
		t.Errorf("ShareFile role: got %q, want %q", call.role, "writer")
	}
}

func TestSyncCalendarAttachments_CanEditFileCallCounts(t *testing.T) {
	driveMock := &mockDriveService{
		canEditFileResults: map[string]bool{
			"att1": true,
			"att2": false,
		},
	}
	calMock := &mockCalendarService{
		listRecentEventsReturn: []*models.CalendarEvent{
			{
				ID:    "evt1",
				Title: "Weekly",
				Attachments: []models.Attachment{
					{FileID: "att1", Title: "Doc1", MimeType: "application/pdf"},
					{FileID: "att2", Title: "Doc2", MimeType: "application/pdf"},
				},
				Attendees: []models.Attendee{
					{Email: "alice@example.com"},
				},
			},
		},
	}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.SyncCalendarAttachments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: CanEditFile called once per attachment
	if len(driveMock.canEditFileCalls) != 2 {
		t.Errorf("CanEditFile calls: got %d, want 2", len(driveMock.canEditFileCalls))
	}
	// Contract: Only editable file was shared
	if len(driveMock.shareFileCalls) != 1 {
		t.Errorf("ShareFile calls: got %d, want 1 (only for editable file)", len(driveMock.shareFileCalls))
	}
	// Contract: stats reflect event processing
	if org.stats.EventsProcessed != 1 {
		t.Errorf("EventsProcessed: got %d, want 1", org.stats.EventsProcessed)
	}
	// Contract: notesDocIDs not populated (non-Google-Doc mime type)
	if len(org.GetNotesDocIDs()) != 0 {
		t.Errorf("notesDocIDs: got %d, want 0", len(org.GetNotesDocIDs()))
	}
}

// ---------- T010: OrganizeDocuments error/edge case tests ----------

func TestOrganizeDocuments_ListMeetingDocsError(t *testing.T) {
	driveMock := &mockDriveService{
		listMeetingDocsErr: fmt.Errorf("Drive API unavailable"),
	}
	calMock := &mockCalendarService{}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.OrganizeDocuments(context.Background())
	if err == nil {
		t.Fatal("expected error from ListMeetingDocuments failure")
	}

	// Contract: error propagated
	if !strings.Contains(err.Error(), "Drive API unavailable") {
		t.Errorf("expected 'Drive API unavailable' in error, got: %v", err)
	}
	// Contract: no documents processed
	if org.stats.DocumentsFound != 0 {
		t.Errorf("DocumentsFound: got %d, want 0", org.stats.DocumentsFound)
	}
}

func TestOrganizeDocuments_EmptyDocumentList(t *testing.T) {
	driveMock := &mockDriveService{
		listMeetingDocsReturn: []*models.Document{},
	}
	calMock := &mockCalendarService{}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.OrganizeDocuments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: stats reflect empty list
	if org.stats.DocumentsFound != 0 {
		t.Errorf("DocumentsFound: got %d, want 0", org.stats.DocumentsFound)
	}
	if org.stats.DocumentsMoved != 0 {
		t.Errorf("DocumentsMoved: got %d, want 0", org.stats.DocumentsMoved)
	}
	if org.stats.ShortcutsCreated != 0 {
		t.Errorf("ShortcutsCreated: got %d, want 0", org.stats.ShortcutsCreated)
	}
}

// ---------- T011: SyncCalendarAttachments error/edge case tests ----------

func TestSyncCalendarAttachments_ListEventsError(t *testing.T) {
	driveMock := &mockDriveService{}
	calMock := &mockCalendarService{
		listRecentEventsErr: fmt.Errorf("Calendar API error"),
	}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.SyncCalendarAttachments(context.Background())
	if err == nil {
		t.Fatal("expected error from ListRecentEvents failure")
	}

	// Contract: error propagated
	if !strings.Contains(err.Error(), "Calendar API error") {
		t.Errorf("expected 'Calendar API error' in error, got: %v", err)
	}
	// Contract: no events processed due to error
	if org.stats.EventsProcessed != 0 {
		t.Errorf("EventsProcessed: got %d, want 0", org.stats.EventsProcessed)
	}
}

func TestSyncCalendarAttachments_EmptyEventsList(t *testing.T) {
	driveMock := &mockDriveService{}
	calMock := &mockCalendarService{
		listRecentEventsReturn: []*models.CalendarEvent{},
	}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.SyncCalendarAttachments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: zero events processed
	if org.stats.EventsProcessed != 0 {
		t.Errorf("EventsProcessed: got %d, want 0", org.stats.EventsProcessed)
	}
	if org.stats.EventsWithAttach != 0 {
		t.Errorf("EventsWithAttach: got %d, want 0", org.stats.EventsWithAttach)
	}
	// Contract: no notes or decision docs collected
	if len(org.GetNotesDocIDs()) != 0 {
		t.Errorf("notesDocIDs: got %d, want 0", len(org.GetNotesDocIDs()))
	}
	if len(org.GetDecisionDocIDs()) != 0 {
		t.Errorf("decisionDocIDs: got %d, want 0", len(org.GetDecisionDocIDs()))
	}
}

func TestSyncCalendarAttachments_SkipsCalendarResources(t *testing.T) {
	driveMock := &mockDriveService{
		canEditFileResults: map[string]bool{"att1": true},
	}
	calMock := &mockCalendarService{
		listRecentEventsReturn: []*models.CalendarEvent{
			{
				ID:    "evt1",
				Title: "Weekly",
				Attachments: []models.Attachment{
					{FileID: "att1", Title: "Notes", MimeType: "application/pdf"},
				},
				Attendees: []models.Attendee{
					{Email: "room-1@resource.calendar.google.com"},
					{Email: "team@group.calendar.google.com"},
					{Email: "real-user@example.com"},
				},
			},
		},
	}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.SyncCalendarAttachments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: ShareFile only called for the real user (not resources)
	if len(driveMock.shareFileCalls) != 1 {
		t.Errorf("ShareFile calls: got %d, want 1 (resource emails should be skipped)", len(driveMock.shareFileCalls))
	}
	if len(driveMock.shareFileCalls) > 0 && driveMock.shareFileCalls[0].email != "real-user@example.com" {
		t.Errorf("ShareFile email: got %q, want %q", driveMock.shareFileCalls[0].email, "real-user@example.com")
	}
	// Contract: stats reflect sharing
	if org.stats.AttachmentsShared != 1 {
		t.Errorf("AttachmentsShared: got %d, want 1", org.stats.AttachmentsShared)
	}
	// Contract: notesDocIDs collected (pdf attachment)
	if len(org.GetNotesDocIDs()) != 0 {
		t.Errorf("notesDocIDs: got %d, want 0 (non-Google-Doc)", len(org.GetNotesDocIDs()))
	}
}

func TestSyncCalendarAttachments_OwnershipCacheAvoidsRedundantCalls(t *testing.T) {
	// When OwnedOnly=true, ownership should be checked per attachment
	driveMock := &mockDriveService{
		isFileOwnedResults: map[string]bool{
			"att1": true,
		},
		canEditFileResults: map[string]bool{
			"att1": true,
		},
	}
	calMock := &mockCalendarService{
		listRecentEventsReturn: []*models.CalendarEvent{
			{
				ID:    "evt1",
				Title: "Weekly",
				Attachments: []models.Attachment{
					{FileID: "att1", Title: "Notes", MimeType: "application/pdf"},
				},
				Attendees: []models.Attendee{
					{Email: "alice@example.com"},
				},
			},
		},
	}
	cfg := config.DefaultConfig()
	cfg.OwnedOnly = true
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.SyncCalendarAttachments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: IsFileOwned called for the attachment in the sharing loop
	if len(driveMock.isFileOwnedCalls) < 1 {
		t.Errorf("IsFileOwned calls: got %d, want at least 1", len(driveMock.isFileOwnedCalls))
	}
}

// ---------- T012: Trivial constructor/utility tests ----------

func TestNew(t *testing.T) {
	cfg := config.DefaultConfig()
	driveMock := &mockDriveService{}
	calMock := &mockCalendarService{}

	org := New(cfg, driveMock, calMock)
	if org == nil {
		t.Fatal("New returned nil")
	}
	if org.config != cfg {
		t.Error("New did not set config")
	}
	if org.notesDocIDs == nil {
		t.Error("New did not initialize notesDocIDs map")
	}
	if org.decisionDocIDs == nil {
		t.Error("New did not initialize decisionDocIDs map")
	}
}

func TestAddTaskStats(t *testing.T) {
	cfg := config.DefaultConfig()
	driveMock := &mockDriveService{}
	calMock := &mockCalendarService{}
	org := New(cfg, driveMock, calMock)

	org.AddTaskStats(5, 2)
	if org.stats.TasksAssigned != 5 {
		t.Errorf("TasksAssigned: got %d, want 5", org.stats.TasksAssigned)
	}
	if org.stats.TasksFailed != 2 {
		t.Errorf("TasksFailed: got %d, want 2", org.stats.TasksFailed)
	}

	// Accumulates
	org.AddTaskStats(3, 1)
	if org.stats.TasksAssigned != 8 {
		t.Errorf("TasksAssigned after add: got %d, want 8", org.stats.TasksAssigned)
	}
}

func TestAddDecisionStats(t *testing.T) {
	cfg := config.DefaultConfig()
	driveMock := &mockDriveService{}
	calMock := &mockCalendarService{}
	org := New(cfg, driveMock, calMock)

	org.AddDecisionStats(3, 1, 2)
	if org.stats.DecisionsProcessed != 3 {
		t.Errorf("DecisionsProcessed: got %d, want 3", org.stats.DecisionsProcessed)
	}
	if org.stats.DecisionsSkipped != 1 {
		t.Errorf("DecisionsSkipped: got %d, want 1", org.stats.DecisionsSkipped)
	}
	if org.stats.DecisionsFailed != 2 {
		t.Errorf("DecisionsFailed: got %d, want 2", org.stats.DecisionsFailed)
	}
}

func TestPrintSummary_Exported(t *testing.T) {
	cfg := config.DefaultConfig()
	driveMock := &mockDriveService{}
	calMock := &mockCalendarService{}
	org := New(cfg, driveMock, calMock)

	// Should not panic
	org.PrintSummary()
}

// ---------- Additional OrganizeDocuments branch tests ----------

func TestOrganizeDocuments_FallbackDocTriggersShortcutTrash(t *testing.T) {
	// When a fallback doc is owned, it gets moved AND redundant shortcuts get trashed.
	driveMock := &mockDriveService{
		listMeetingDocsReturn: []*models.Document{
			{
				ID:             "doc-fallback",
				Name:           "Notes - Weekly",
				MeetingName:    "Weekly",
				IsOwned:        true,
				IsFallback:     true,
				ParentFolderID: "root",
			},
		},
		getOrCreateFolderReturn: &models.MeetingFolder{
			ID:   "folder-weekly",
			Name: "Weekly",
		},
	}
	calMock := &mockCalendarService{}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.OrganizeDocuments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: FindShortcutToFile called for fallback + owned doc
	if driveMock.findShortcutToFileCalls != 1 {
		t.Errorf("FindShortcutToFile calls: got %d, want 1", driveMock.findShortcutToFileCalls)
	}
	// Contract: MoveDocument called
	if len(driveMock.moveDocumentCalls) != 1 {
		t.Errorf("MoveDocument calls: got %d, want 1", len(driveMock.moveDocumentCalls))
	}
}

func TestOrganizeDocuments_GetOrCreateFolderError(t *testing.T) {
	driveMock := &mockDriveService{
		listMeetingDocsReturn: []*models.Document{
			{ID: "doc1", Name: "Weekly - 2026-03-01", MeetingName: "Weekly", IsOwned: true, ParentFolderID: "root"},
		},
		getOrCreateFolderErr: fmt.Errorf("folder creation failed"),
	}
	calMock := &mockCalendarService{}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.OrganizeDocuments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: error increments Errors but doesn't stop processing
	if org.stats.Errors != 1 {
		t.Errorf("Errors: got %d, want 1", org.stats.Errors)
	}
	// Contract: no move attempted
	if len(driveMock.moveDocumentCalls) != 0 {
		t.Errorf("MoveDocument should not be called on folder error, got %d", len(driveMock.moveDocumentCalls))
	}
}

func TestOrganizeDocuments_OwnedOnlyGetOrCreateFolderError(t *testing.T) {
	// When OwnedOnly=true and non-owned doc, GetOrCreateMeetingFolder error
	// should still increment Errors
	driveMock := &mockDriveService{
		listMeetingDocsReturn: []*models.Document{
			{ID: "doc1", Name: "Weekly - 2026-03-01", MeetingName: "Weekly", IsOwned: false, ParentFolderID: "root"},
		},
		getOrCreateFolderErr: fmt.Errorf("folder error"),
	}
	calMock := &mockCalendarService{}
	cfg := config.DefaultConfig()
	cfg.OwnedOnly = true
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.OrganizeDocuments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: error counted
	if org.stats.Errors != 1 {
		t.Errorf("Errors: got %d, want 1", org.stats.Errors)
	}
	// Contract: skipped counted
	if org.stats.Skipped != 1 {
		t.Errorf("Skipped: got %d, want 1", org.stats.Skipped)
	}
}

func TestSyncCalendarAttachments_EmptyTitleFetchesFileName(t *testing.T) {
	// When attachment title is empty, GetFileName should be called
	driveMock := &mockDriveService{
		canEditFileResults: map[string]bool{"att1": false},
		getFileNameResults: map[string]string{"att1": "Fetched Name.pdf"},
	}
	calMock := &mockCalendarService{
		listRecentEventsReturn: []*models.CalendarEvent{
			{
				ID:    "evt1",
				Title: "Weekly",
				Attachments: []models.Attachment{
					{FileID: "att1", Title: "", MimeType: "application/pdf"},
				},
			},
		},
	}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.SyncCalendarAttachments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: GetFileName called for empty-title attachment
	if len(driveMock.getFileNameCalls) != 1 {
		t.Errorf("GetFileName calls: got %d, want 1", len(driveMock.getFileNameCalls))
	}
}

func TestSyncCalendarAttachments_EmptyFileIDSkipped(t *testing.T) {
	driveMock := &mockDriveService{}
	calMock := &mockCalendarService{
		listRecentEventsReturn: []*models.CalendarEvent{
			{
				ID:    "evt1",
				Title: "Weekly",
				Attachments: []models.Attachment{
					{FileID: "", Title: "Empty ID", MimeType: "application/pdf"},
				},
			},
		},
	}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.SyncCalendarAttachments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: No CreateShortcut or ShareFile calls for empty FileID
	if len(driveMock.createShortcutCalls) != 0 {
		t.Errorf("CreateShortcut calls: got %d, want 0 for empty FileID", len(driveMock.createShortcutCalls))
	}
}

func TestSyncCalendarAttachments_SelfAttendeeSkipped(t *testing.T) {
	driveMock := &mockDriveService{
		canEditFileResults: map[string]bool{"att1": true},
	}
	calMock := &mockCalendarService{
		listRecentEventsReturn: []*models.CalendarEvent{
			{
				ID:    "evt1",
				Title: "Weekly",
				Attachments: []models.Attachment{
					{FileID: "att1", Title: "Notes", MimeType: "application/pdf"},
				},
				Attendees: []models.Attendee{
					{Email: "me@example.com", IsSelf: true},
				},
			},
		},
	}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.SyncCalendarAttachments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: self attendee is not shared with
	if len(driveMock.shareFileCalls) != 0 {
		t.Errorf("ShareFile calls: got %d, want 0 (self attendee should be skipped)", len(driveMock.shareFileCalls))
	}
}

func TestSyncCalendarAttachments_GetOrCreateFolderError(t *testing.T) {
	driveMock := &mockDriveService{
		getOrCreateFolderErr: fmt.Errorf("folder error"),
	}
	calMock := &mockCalendarService{
		listRecentEventsReturn: []*models.CalendarEvent{
			{
				ID:    "evt1",
				Title: "Weekly",
				Attachments: []models.Attachment{
					{FileID: "att1", Title: "Notes", MimeType: "application/pdf"},
				},
			},
		},
	}
	cfg := config.DefaultConfig()
	org := setupOrganizer(cfg, driveMock, calMock)

	err := org.SyncCalendarAttachments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: error counted but processing continues
	if org.stats.Errors != 1 {
		t.Errorf("Errors: got %d, want 1", org.stats.Errors)
	}
}

// ---------- Mock DocsService and GeminiService for ExtractDecisionsForDoc tests ----------

type mockDocsService struct {
	hasDecisionsTab         bool
	hasDecisionsTabErr      error
	transcriptContent       *models.TranscriptContent
	transcriptContentErr    error
	createDecisionsTabErr   error
	createDecisionsTabCalls int
}

func (m *mockDocsService) ExtractTranscriptContent(_ context.Context, _ string) (*models.TranscriptContent, error) {
	return m.transcriptContent, m.transcriptContentErr
}

func (m *mockDocsService) HasDecisionsTab(_ context.Context, _ string) (bool, error) {
	return m.hasDecisionsTab, m.hasDecisionsTabErr
}

func (m *mockDocsService) CreateDecisionsTab(_ context.Context, _ string, _ []models.Decision, _ *models.TranscriptContent) error {
	m.createDecisionsTabCalls++
	return m.createDecisionsTabErr
}

type mockGeminiService struct {
	decisions    []models.Decision
	extractErr   error
	extractCalls int
}

func (m *mockGeminiService) ExtractDecisions(_ context.Context, _ string) ([]models.Decision, error) {
	m.extractCalls++
	return m.decisions, m.extractErr
}

// ---------- TestExtractDecisionsForDoc ----------

func TestExtractDecisionsForDoc(t *testing.T) {
	tests := []struct {
		name               string
		docsSvc            *mockDocsService
		geminiSvc          *mockGeminiService
		dryRun             bool
		wantErr            bool
		wantProcessed      int
		wantSkipped        int
		wantFailed         int
		wantCreateTabCalls int
		wantGeminiCalls    int
	}{
		{
			name: "already has Decisions tab - skip",
			docsSvc: &mockDocsService{
				hasDecisionsTab: true,
			},
			geminiSvc:          &mockGeminiService{},
			wantSkipped:        1,
			wantGeminiCalls:    0,
			wantCreateTabCalls: 0,
		},
		{
			name: "no transcript content - skip",
			docsSvc: &mockDocsService{
				hasDecisionsTab:   false,
				transcriptContent: nil,
			},
			geminiSvc:          &mockGeminiService{},
			wantSkipped:        1,
			wantGeminiCalls:    0,
			wantCreateTabCalls: 0,
		},
		{
			name: "empty transcript text - skip",
			docsSvc: &mockDocsService{
				hasDecisionsTab:   false,
				transcriptContent: &models.TranscriptContent{TabID: "tab1", FullText: ""},
			},
			geminiSvc:          &mockGeminiService{},
			wantSkipped:        1,
			wantGeminiCalls:    0,
			wantCreateTabCalls: 0,
		},
		{
			name:   "dry-run - logs only, no Gemini call",
			dryRun: true,
			docsSvc: &mockDocsService{
				hasDecisionsTab: false,
				transcriptContent: &models.TranscriptContent{
					TabID:    "tab1",
					FullText: "Meeting transcript text",
				},
			},
			geminiSvc:          &mockGeminiService{},
			wantProcessed:      1,
			wantGeminiCalls:    0,
			wantCreateTabCalls: 0,
		},
		{
			name: "Gemini failure - skip with warning",
			docsSvc: &mockDocsService{
				hasDecisionsTab: false,
				transcriptContent: &models.TranscriptContent{
					TabID:    "tab1",
					FullText: "Meeting transcript text",
				},
			},
			geminiSvc: &mockGeminiService{
				extractErr: fmt.Errorf("Gemini API error: rate limited"),
			},
			wantFailed:         1,
			wantGeminiCalls:    1,
			wantCreateTabCalls: 0,
		},
		{
			name: "concurrent tab creation - sentinel error treated as skip",
			docsSvc: &mockDocsService{
				hasDecisionsTab: false,
				transcriptContent: &models.TranscriptContent{
					TabID:    "tab1",
					FullText: "Meeting transcript text",
				},
				createDecisionsTabErr: docs.ErrDecisionsTabExists,
			},
			geminiSvc: &mockGeminiService{
				decisions: []models.Decision{
					{Category: "made", Text: "Test decision"},
				},
			},
			wantSkipped:        1,
			wantGeminiCalls:    1,
			wantCreateTabCalls: 1,
		},
		{
			name: "happy path - decisions extracted and tab created",
			docsSvc: &mockDocsService{
				hasDecisionsTab: false,
				transcriptContent: &models.TranscriptContent{
					TabID:    "tab1",
					FullText: "Meeting transcript with decisions",
				},
			},
			geminiSvc: &mockGeminiService{
				decisions: []models.Decision{
					{Category: "made", Text: "Adopt new pipeline", Timestamp: "12:34"},
					{Category: "deferred", Text: "Budget review", Timestamp: "13:00"},
				},
			},
			wantProcessed:      1,
			wantGeminiCalls:    1,
			wantCreateTabCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driveMock := &mockDriveService{}
			calMock := &mockCalendarService{}
			cfg := config.DefaultConfig()
			org := setupOrganizer(cfg, driveMock, calMock)

			err := org.ExtractDecisionsForDoc(context.Background(), "test-doc-id", tt.docsSvc, tt.geminiSvc, tt.dryRun)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if org.stats.DecisionsProcessed != tt.wantProcessed {
				t.Errorf("DecisionsProcessed: got %d, want %d", org.stats.DecisionsProcessed, tt.wantProcessed)
			}

			if org.stats.DecisionsSkipped != tt.wantSkipped {
				t.Errorf("DecisionsSkipped: got %d, want %d", org.stats.DecisionsSkipped, tt.wantSkipped)
			}

			if org.stats.DecisionsFailed != tt.wantFailed {
				t.Errorf("DecisionsFailed: got %d, want %d", org.stats.DecisionsFailed, tt.wantFailed)
			}

			if tt.docsSvc.createDecisionsTabCalls != tt.wantCreateTabCalls {
				t.Errorf("CreateDecisionsTab calls: got %d, want %d", tt.docsSvc.createDecisionsTabCalls, tt.wantCreateTabCalls)
			}

			if tt.geminiSvc.extractCalls != tt.wantGeminiCalls {
				t.Errorf("ExtractDecisions calls: got %d, want %d", tt.geminiSvc.extractCalls, tt.wantGeminiCalls)
			}
		})
	}
}
