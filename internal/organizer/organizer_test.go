package organizer

import (
	"context"
	"fmt"
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

// newTestOrganizer creates an Organizer with mock services for testing.
func newTestOrganizer(cfg *config.Config, driveMock *mockDriveService, calMock *mockCalendarService) *Organizer {
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

	org := newTestOrganizer(cfg, driveMock, calMock)
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

	org := newTestOrganizer(cfg, driveMock, calMock)
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

	org := newTestOrganizer(cfg, driveMock, calMock)
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

	org := newTestOrganizer(cfg, driveMock, calMock)
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

	org := newTestOrganizer(cfg, driveMock, calMock)
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

	org := newTestOrganizer(cfg, driveMock, calMock)
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

	org := newTestOrganizer(cfg, driveMock, calMock)
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

	org := newTestOrganizer(cfg, driveMock, calMock)
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

			org := newTestOrganizer(cfg, driveMock, calMock)
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
			org := newTestOrganizer(cfg, driveMock, calMock)

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
