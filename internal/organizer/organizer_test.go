package organizer

import (
	"context"
	"testing"

	"github.com/jflowers/gcal-organizer/internal/config"
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
		config:      cfg,
		drive:       driveMock,
		calendar:    calMock,
		logger:      logging.Logger,
		notesDocIDs: make(map[string]bool),
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
