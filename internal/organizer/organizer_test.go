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

	// Contract: DocumentsFound stat matches input length
	if org.stats.DocumentsFound != 2 {
		t.Errorf("DocumentsFound: got %d, want 2", org.stats.DocumentsFound)
	}

	// Contract: SetMasterFolder called with config folder name
	if len(driveMock.setMasterFolderCalls) != 1 {
		t.Fatalf("expected 1 SetMasterFolder call, got %d", len(driveMock.setMasterFolderCalls))
	}
	if driveMock.setMasterFolderCalls[0] != cfg.MasterFolderName {
		t.Errorf("SetMasterFolder called with %q, want %q", driveMock.setMasterFolderCalls[0], cfg.MasterFolderName)
	}

	// Contract: GetOrCreateMeetingFolder called with correct meeting names
	if len(driveMock.getOrCreateFolderCalls) != 2 {
		t.Fatalf("expected 2 GetOrCreateMeetingFolder calls, got %d", len(driveMock.getOrCreateFolderCalls))
	}
	if driveMock.getOrCreateFolderCalls[0] != "Weekly" {
		t.Errorf("first folder call: got %q, want %q", driveMock.getOrCreateFolderCalls[0], "Weekly")
	}
	if driveMock.getOrCreateFolderCalls[1] != "Standup" {
		t.Errorf("second folder call: got %q, want %q", driveMock.getOrCreateFolderCalls[1], "Standup")
	}

	// Owned doc should be moved
	if len(driveMock.moveDocumentCalls) != 1 {
		t.Errorf("expected 1 MoveDocument call, got %d", len(driveMock.moveDocumentCalls))
	}
	if len(driveMock.moveDocumentCalls) > 0 {
		mc := driveMock.moveDocumentCalls[0]
		// Contract: MoveDocument receives correct doc ID, name, parent, and target folder
		if mc.docID != "doc1" {
			t.Errorf("MoveDocument docID: got %q, want %q", mc.docID, "doc1")
		}
		if mc.docName != "Weekly - 2026-02-01" {
			t.Errorf("MoveDocument docName: got %q, want %q", mc.docName, "Weekly - 2026-02-01")
		}
		if mc.currentParentID != "root" {
			t.Errorf("MoveDocument currentParentID: got %q, want %q", mc.currentParentID, "root")
		}
		if mc.targetFolderID != "folder-Weekly" {
			t.Errorf("MoveDocument targetFolderID: got %q, want %q", mc.targetFolderID, "folder-Weekly")
		}
	}

	// Non-owned doc should get a shortcut (not skipped)
	if len(driveMock.createShortcutCalls) != 1 {
		t.Errorf("expected 1 CreateShortcut call, got %d", len(driveMock.createShortcutCalls))
	}
	if len(driveMock.createShortcutCalls) > 0 {
		sc := driveMock.createShortcutCalls[0]
		// Contract: CreateShortcut receives correct file ID, name, and folder
		if sc.fileID != "doc2" {
			t.Errorf("CreateShortcut fileID: got %q, want %q", sc.fileID, "doc2")
		}
		if sc.fileName != "Standup - 2026-02-02" {
			t.Errorf("CreateShortcut fileName: got %q, want %q", sc.fileName, "Standup - 2026-02-02")
		}
		if sc.folderID != "folder-Standup" {
			t.Errorf("CreateShortcut folderID: got %q, want %q", sc.folderID, "folder-Standup")
		}
	}

	// No skipped count
	if org.stats.Skipped != 0 {
		t.Errorf("expected Skipped=0, got %d", org.stats.Skipped)
	}
	// Contract: Errors stat is zero on success
	if org.stats.Errors != 0 {
		t.Errorf("expected Errors=0, got %d", org.stats.Errors)
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

	// Contract: DocumentsFound stat reflects input
	if org.stats.DocumentsFound != 1 {
		t.Errorf("DocumentsFound: got %d, want 1", org.stats.DocumentsFound)
	}

	// Should NOT call MoveDocument
	if len(driveMock.moveDocumentCalls) != 0 {
		t.Errorf("expected 0 MoveDocument calls, got %d", len(driveMock.moveDocumentCalls))
	}

	// Contract: GetOrCreateMeetingFolder still called (for shortcut creation)
	if len(driveMock.getOrCreateFolderCalls) != 1 {
		t.Errorf("expected 1 GetOrCreateMeetingFolder call, got %d", len(driveMock.getOrCreateFolderCalls))
	}
	if len(driveMock.getOrCreateFolderCalls) > 0 && driveMock.getOrCreateFolderCalls[0] != "Weekly" {
		t.Errorf("GetOrCreateMeetingFolder: got %q, want %q", driveMock.getOrCreateFolderCalls[0], "Weekly")
	}

	// Should still create shortcut for discoverability (FR-005)
	if len(driveMock.createShortcutCalls) != 1 {
		t.Errorf("expected 1 CreateShortcut call, got %d", len(driveMock.createShortcutCalls))
	}
	if len(driveMock.createShortcutCalls) > 0 {
		sc := driveMock.createShortcutCalls[0]
		// Contract: shortcut created for the correct doc in the correct folder
		if sc.fileID != "doc1" {
			t.Errorf("CreateShortcut fileID: got %q, want %q", sc.fileID, "doc1")
		}
		if sc.folderID != "folder-Weekly" {
			t.Errorf("CreateShortcut folderID: got %q, want %q", sc.folderID, "folder-Weekly")
		}
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

func TestOrganizeDocuments_ListMeetingDocsError(t *testing.T) {
	// Contract: ListMeetingDocuments error propagates to caller
	driveMock := &mockDriveService{
		listMeetingDocsErr: fmt.Errorf("Drive API unavailable"),
	}
	calMock := &mockCalendarService{}
	cfg := config.DefaultConfig()

	org := newTestOrganizer(cfg, driveMock, calMock)
	err := org.OrganizeDocuments(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Drive API unavailable") {
		t.Errorf("error should propagate Drive error, got: %v", err)
	}
	// Contract: no documents processed on error
	if org.stats.DocumentsFound != 0 {
		t.Errorf("DocumentsFound: got %d, want 0", org.stats.DocumentsFound)
	}
}

func TestOrganizeDocuments_EmptyDocumentList(t *testing.T) {
	// Contract: empty document list processes cleanly with correct stats
	driveMock := &mockDriveService{
		listMeetingDocsReturn: []*models.Document{},
	}
	calMock := &mockCalendarService{}
	cfg := config.DefaultConfig()

	org := newTestOrganizer(cfg, driveMock, calMock)
	err := org.OrganizeDocuments(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Contract: stats correctly reflect zero documents
	if org.stats.DocumentsFound != 0 {
		t.Errorf("DocumentsFound: got %d, want 0", org.stats.DocumentsFound)
	}
	if len(driveMock.moveDocumentCalls) != 0 {
		t.Errorf("expected 0 MoveDocument calls, got %d", len(driveMock.moveDocumentCalls))
	}
	if len(driveMock.createShortcutCalls) != 0 {
		t.Errorf("expected 0 CreateShortcut calls, got %d", len(driveMock.createShortcutCalls))
	}
	// Contract: SetMasterFolder still called even with no docs
	if len(driveMock.setMasterFolderCalls) != 1 {
		t.Errorf("expected 1 SetMasterFolder call, got %d", len(driveMock.setMasterFolderCalls))
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

	// Contract: EventsProcessed stat reflects input
	if org.stats.EventsProcessed != 1 {
		t.Errorf("EventsProcessed: got %d, want 1", org.stats.EventsProcessed)
	}
	// Contract: EventsWithAttach counts events with non-empty Attachments
	if org.stats.EventsWithAttach != 1 {
		t.Errorf("EventsWithAttach: got %d, want 1", org.stats.EventsWithAttach)
	}

	// Should NOT call ShareFile for non-owned attachment
	if len(driveMock.shareFileCalls) != 0 {
		t.Errorf("expected 0 ShareFile calls for non-owned attachment, got %d", len(driveMock.shareFileCalls))
	}

	// Contract: shortcut still created for the attachment regardless of ownership
	if len(driveMock.createShortcutCalls) != 1 {
		t.Errorf("expected 1 CreateShortcut call, got %d", len(driveMock.createShortcutCalls))
	}
	if len(driveMock.createShortcutCalls) > 0 {
		sc := driveMock.createShortcutCalls[0]
		if sc.fileID != "att1" {
			t.Errorf("CreateShortcut fileID: got %q, want %q", sc.fileID, "att1")
		}
		if sc.fileName != "Notes" {
			t.Errorf("CreateShortcut fileName: got %q, want %q", sc.fileName, "Notes")
		}
	}

	// Should increment Skipped
	if org.stats.Skipped != 1 {
		t.Errorf("expected Skipped=1, got %d", org.stats.Skipped)
	}
	// Contract: AttachmentsShared is zero when nothing was shared
	if org.stats.AttachmentsShared != 0 {
		t.Errorf("AttachmentsShared: got %d, want 0", org.stats.AttachmentsShared)
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
		t.Fatalf("expected 1 ShareFile call for owned attachment, got %d", len(driveMock.shareFileCalls))
	}
	// Contract: ShareFile receives correct file ID, title, email, and role
	sc := driveMock.shareFileCalls[0]
	if sc.fileID != "att1" {
		t.Errorf("ShareFile fileID: got %q, want %q", sc.fileID, "att1")
	}
	if sc.fileName != "Notes Doc" {
		t.Errorf("ShareFile fileName: got %q, want %q", sc.fileName, "Notes Doc")
	}
	if sc.email != "alice@example.com" {
		t.Errorf("ShareFile email: got %q, want %q", sc.email, "alice@example.com")
	}
	if sc.role != "writer" {
		t.Errorf("ShareFile role: got %q, want %q", sc.role, "writer")
	}

	// Contract: AttachmentsShared stat incremented
	if org.stats.AttachmentsShared != 1 {
		t.Errorf("AttachmentsShared: got %d, want 1", org.stats.AttachmentsShared)
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

	// Contract: EventsProcessed stat reflects input
	if org.stats.EventsProcessed != 1 {
		t.Errorf("EventsProcessed: got %d, want 1", org.stats.EventsProcessed)
	}
	// Contract: EventsWithAttach counts events with attachments
	if org.stats.EventsWithAttach != 1 {
		t.Errorf("EventsWithAttach: got %d, want 1", org.stats.EventsWithAttach)
	}

	// Should NOT call IsFileOwned when OwnedOnly=false
	if len(driveMock.isFileOwnedCalls) != 0 {
		t.Errorf("expected 0 IsFileOwned calls when OwnedOnly=false, got %d", len(driveMock.isFileOwnedCalls))
	}

	// Contract: CanEditFile called for both attachments
	if len(driveMock.canEditFileCalls) != 2 {
		t.Errorf("expected 2 CanEditFile calls, got %d", len(driveMock.canEditFileCalls))
	}

	// Contract: shortcuts created for both attachments (sharing is separate from shortcuts)
	if len(driveMock.createShortcutCalls) != 2 {
		t.Errorf("expected 2 CreateShortcut calls, got %d", len(driveMock.createShortcutCalls))
	}

	// Should call ShareFile only for the editable attachment
	if len(driveMock.shareFileCalls) != 1 {
		t.Fatalf("expected 1 ShareFile call, got %d", len(driveMock.shareFileCalls))
	}
	if driveMock.shareFileCalls[0].fileID != "att1" {
		t.Errorf("expected ShareFile for att1, got %s", driveMock.shareFileCalls[0].fileID)
	}

	// Contract: AttachmentsShared incremented for shared file
	if org.stats.AttachmentsShared != 1 {
		t.Errorf("AttachmentsShared: got %d, want 1", org.stats.AttachmentsShared)
	}
	// Contract: Skipped is zero when OwnedOnly=false
	if org.stats.Skipped != 0 {
		t.Errorf("Skipped: got %d, want 0", org.stats.Skipped)
	}
}

func TestSyncCalendarAttachments_ListEventsError(t *testing.T) {
	// Contract: ListRecentEvents error propagates to caller
	driveMock := &mockDriveService{}
	calMock := &mockCalendarService{
		listRecentEventsErr: fmt.Errorf("Calendar API unavailable"),
	}
	cfg := config.DefaultConfig()

	org := newTestOrganizer(cfg, driveMock, calMock)
	err := org.SyncCalendarAttachments(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Calendar API unavailable") {
		t.Errorf("error should propagate Calendar error, got: %v", err)
	}
}

func TestSyncCalendarAttachments_EmptyEventsList(t *testing.T) {
	// Contract: empty events list processes cleanly with correct stats
	driveMock := &mockDriveService{}
	calMock := &mockCalendarService{
		listRecentEventsReturn: []*models.CalendarEvent{},
	}
	cfg := config.DefaultConfig()

	org := newTestOrganizer(cfg, driveMock, calMock)
	err := org.SyncCalendarAttachments(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if org.stats.EventsProcessed != 0 {
		t.Errorf("EventsProcessed: got %d, want 0", org.stats.EventsProcessed)
	}
	if org.stats.EventsWithAttach != 0 {
		t.Errorf("EventsWithAttach: got %d, want 0", org.stats.EventsWithAttach)
	}
}

func TestSyncCalendarAttachments_SkipsCalendarResources(t *testing.T) {
	// Contract: calendar resource and group emails are not shared with
	driveMock := &mockDriveService{
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
					{Email: "room@resource.calendar.google.com"},
					{Email: "team@group.calendar.google.com"},
					{Email: "", IsSelf: false},              // empty email
					{Email: "me@example.com", IsSelf: true}, // self
				},
			},
		},
	}
	cfg := config.DefaultConfig()

	org := newTestOrganizer(cfg, driveMock, calMock)
	err := org.SyncCalendarAttachments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: only alice@example.com receives a share — resources, groups, empty, and self are filtered
	if len(driveMock.shareFileCalls) != 1 {
		t.Fatalf("expected 1 ShareFile call, got %d", len(driveMock.shareFileCalls))
	}
	if driveMock.shareFileCalls[0].email != "alice@example.com" {
		t.Errorf("ShareFile email: got %q, want %q", driveMock.shareFileCalls[0].email, "alice@example.com")
	}
}

func TestSyncCalendarAttachments_OwnershipCacheAvoidsRedundantCalls(t *testing.T) {
	// Contract: per-event ownership cache avoids N+1 IsFileOwned API calls
	// when the same fileID appears in both the notes-doc collection and sharing loops.
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
					{FileID: "att1", Title: "Notes by Gemini", MimeType: "application/vnd.google-apps.document"},
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

	// Contract: IsFileOwned called exactly once for "att1" despite being checked
	// in both the notes/decision collection loop and the sharing loop.
	// The ownership cache should prevent the second call.
	if len(driveMock.isFileOwnedCalls) != 1 {
		t.Errorf("expected 1 IsFileOwned call (cached), got %d: %v",
			len(driveMock.isFileOwnedCalls), driveMock.isFileOwnedCalls)
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

			// Contract: returned map must have exactly the expected count
			if len(gotDocIDs) != tt.wantDocCount {
				t.Fatalf("expected %d decision doc IDs, got %d: %v", tt.wantDocCount, len(gotDocIDs), gotDocIDs)
			}

			// Contract: each expected doc ID must be present with correct source pattern
			for wantID, wantSource := range tt.wantDocIDs {
				gotSource, ok := gotDocIDs[wantID]
				if !ok {
					t.Errorf("expected doc ID %q in decisionDocIDs, but not found", wantID)
					continue
				}
				if gotSource != wantSource {
					t.Errorf("doc %q: source got %q, want %q", wantID, gotSource, wantSource)
				}
				// Contract: source must be one of the two valid values
				if gotSource != "notes-by-gemini" && gotSource != "transcript" {
					t.Errorf("doc %q: invalid source %q (must be 'notes-by-gemini' or 'transcript')", wantID, gotSource)
				}
			}

			// Contract: no unexpected doc IDs in the result
			for gotID := range gotDocIDs {
				if _, expected := tt.wantDocIDs[gotID]; !expected {
					t.Errorf("unexpected doc ID %q in decisionDocIDs", gotID)
				}
			}
		})
	}
}

// ---------- TestGetDecisionDocIDs — return value contract ----------

func TestGetDecisionDocIDs_ReturnsCopy(t *testing.T) {
	// Contract: GetDecisionDocIDs returns a defensive copy, not a reference to internal state
	driveMock := &mockDriveService{}
	calMock := &mockCalendarService{
		listRecentEventsReturn: []*models.CalendarEvent{
			{
				ID:    "event1",
				Title: "Weekly",
				Attachments: []models.Attachment{
					{FileID: "doc1", Title: "Notes by Gemini", MimeType: "application/vnd.google-apps.document"},
				},
			},
		},
	}
	cfg := config.DefaultConfig()
	org := newTestOrganizer(cfg, driveMock, calMock)

	err := org.SyncCalendarAttachments(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Get a copy and mutate it
	copy1 := org.GetDecisionDocIDs()
	if len(copy1) != 1 {
		t.Fatalf("expected 1 doc ID, got %d", len(copy1))
	}

	// Contract: source value must be "notes-by-gemini" for exact match
	if copy1["doc1"] != "notes-by-gemini" {
		t.Errorf("expected source 'notes-by-gemini', got %q", copy1["doc1"])
	}

	// Mutate the copy
	copy1["doc1"] = "tampered"
	copy1["doc-injected"] = "attack"

	// Contract: second call returns pristine internal state
	copy2 := org.GetDecisionDocIDs()
	if copy2["doc1"] != "notes-by-gemini" {
		t.Errorf("internal state was mutated: got %q, want 'notes-by-gemini'", copy2["doc1"])
	}
	if _, found := copy2["doc-injected"]; found {
		t.Error("injected key found in internal state — GetDecisionDocIDs does not return a defensive copy")
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

	// Contract verification: captured arguments
	hasDecisionsTabDocID         string
	extractTranscriptDocID       string
	createDecisionsTabDocID      string
	createDecisionsTabDecisions  []models.Decision
	createDecisionsTabTranscript *models.TranscriptContent
}

func (m *mockDocsService) ExtractTranscriptContent(_ context.Context, docID string) (*models.TranscriptContent, error) {
	m.extractTranscriptDocID = docID
	return m.transcriptContent, m.transcriptContentErr
}

func (m *mockDocsService) HasDecisionsTab(_ context.Context, docID string) (bool, error) {
	m.hasDecisionsTabDocID = docID
	return m.hasDecisionsTab, m.hasDecisionsTabErr
}

func (m *mockDocsService) CreateDecisionsTab(_ context.Context, docID string, decisions []models.Decision, transcript *models.TranscriptContent) error {
	m.createDecisionsTabCalls++
	m.createDecisionsTabDocID = docID
	m.createDecisionsTabDecisions = decisions
	m.createDecisionsTabTranscript = transcript
	return m.createDecisionsTabErr
}

type mockGeminiService struct {
	decisions    []models.Decision
	extractErr   error
	extractCalls int

	// Contract verification: captured arguments
	lastTranscriptText string
}

func (m *mockGeminiService) ExtractDecisions(_ context.Context, transcriptText string) ([]models.Decision, error) {
	m.extractCalls++
	m.lastTranscriptText = transcriptText
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
		wantErrContains    string
		wantProcessed      int
		wantSkipped        int
		wantFailed         int
		wantCreateTabCalls int
		wantGeminiCalls    int
	}{
		{
			name: "already has Decisions tab — increments skip, no downstream calls",
			docsSvc: &mockDocsService{
				hasDecisionsTab: true,
			},
			geminiSvc:          &mockGeminiService{},
			wantSkipped:        1,
			wantGeminiCalls:    0,
			wantCreateTabCalls: 0,
		},
		{
			name: "no transcript content — increments skip",
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
			name: "empty transcript text — increments skip",
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
			name:   "dry-run — increments processed, no API calls at all (FR-013)",
			dryRun: true,
			docsSvc: &mockDocsService{
				// These values should NOT be accessed in dry-run mode
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
			name: "Gemini failure — increments failed, no error returned (FR-017)",
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
			name: "concurrent tab creation — sentinel error treated as skip (FR-018)",
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
			name: "CreateDecisionsTab non-sentinel error — returns wrapped error",
			docsSvc: &mockDocsService{
				hasDecisionsTab: false,
				transcriptContent: &models.TranscriptContent{
					TabID:    "tab1",
					FullText: "Meeting transcript text",
				},
				createDecisionsTabErr: fmt.Errorf("API timeout"),
			},
			geminiSvc: &mockGeminiService{
				decisions: []models.Decision{
					{Category: "made", Text: "Test decision"},
				},
			},
			wantErr:            true,
			wantErrContains:    "create decisions tab",
			wantFailed:         1,
			wantGeminiCalls:    1,
			wantCreateTabCalls: 1,
		},
		{
			name: "HasDecisionsTab error — returns wrapped error",
			docsSvc: &mockDocsService{
				hasDecisionsTabErr: fmt.Errorf("API unavailable"),
			},
			geminiSvc:       &mockGeminiService{},
			wantErr:         true,
			wantErrContains: "check decisions tab",
		},
		{
			name: "ExtractTranscriptContent error — returns wrapped error",
			docsSvc: &mockDocsService{
				hasDecisionsTab:      false,
				transcriptContentErr: fmt.Errorf("document not found"),
			},
			geminiSvc:       &mockGeminiService{},
			wantErr:         true,
			wantErrContains: "extract transcript",
		},
		{
			name: "happy path — decisions extracted, tab created, processed incremented",
			docsSvc: &mockDocsService{
				hasDecisionsTab: false,
				transcriptContent: &models.TranscriptContent{
					TabID:    "tab1",
					FullText: "Meeting transcript with decisions",
					Headings: []models.TranscriptHeading{
						{HeadingID: "h.1", Text: "12:00", Index: 0},
					},
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
					t.Fatal("expected error, got nil")
				}
				// Contract: errors must be wrapped with contextual prefix
				if tt.wantErrContains != "" {
					errMsg := err.Error()
					if !strings.Contains(errMsg, tt.wantErrContains) {
						t.Errorf("error %q does not contain %q", errMsg, tt.wantErrContains)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Contract: stats must reflect the exact operation outcome
			if org.stats.DecisionsProcessed != tt.wantProcessed {
				t.Errorf("DecisionsProcessed: got %d, want %d", org.stats.DecisionsProcessed, tt.wantProcessed)
			}
			if org.stats.DecisionsSkipped != tt.wantSkipped {
				t.Errorf("DecisionsSkipped: got %d, want %d", org.stats.DecisionsSkipped, tt.wantSkipped)
			}
			if org.stats.DecisionsFailed != tt.wantFailed {
				t.Errorf("DecisionsFailed: got %d, want %d", org.stats.DecisionsFailed, tt.wantFailed)
			}

			// Contract: downstream call counts must match
			if tt.docsSvc.createDecisionsTabCalls != tt.wantCreateTabCalls {
				t.Errorf("CreateDecisionsTab calls: got %d, want %d", tt.docsSvc.createDecisionsTabCalls, tt.wantCreateTabCalls)
			}
			if tt.geminiSvc.extractCalls != tt.wantGeminiCalls {
				t.Errorf("ExtractDecisions calls: got %d, want %d", tt.geminiSvc.extractCalls, tt.wantGeminiCalls)
			}

			// Contract: docID must be passed through to all downstream calls (except dry-run, which makes no API calls)
			if !tt.dryRun && tt.docsSvc.hasDecisionsTabDocID != "test-doc-id" {
				t.Errorf("HasDecisionsTab docID: got %q, want %q", tt.docsSvc.hasDecisionsTabDocID, "test-doc-id")
			}
			if tt.wantGeminiCalls > 0 && tt.docsSvc.transcriptContent != nil {
				// Contract: Gemini receives the full transcript text
				if tt.geminiSvc.lastTranscriptText != tt.docsSvc.transcriptContent.FullText {
					t.Errorf("Gemini received transcript text %q, want %q",
						tt.geminiSvc.lastTranscriptText, tt.docsSvc.transcriptContent.FullText)
				}
			}
			if tt.wantCreateTabCalls > 0 {
				// Contract: CreateDecisionsTab receives correct docID
				if tt.docsSvc.createDecisionsTabDocID != "test-doc-id" {
					t.Errorf("CreateDecisionsTab docID: got %q, want %q",
						tt.docsSvc.createDecisionsTabDocID, "test-doc-id")
				}
				// Contract: decisions passed to CreateDecisionsTab match Gemini output
				if len(tt.docsSvc.createDecisionsTabDecisions) != len(tt.geminiSvc.decisions) {
					t.Errorf("CreateDecisionsTab decisions count: got %d, want %d",
						len(tt.docsSvc.createDecisionsTabDecisions), len(tt.geminiSvc.decisions))
				}
				for i, want := range tt.geminiSvc.decisions {
					if i >= len(tt.docsSvc.createDecisionsTabDecisions) {
						break
					}
					got := tt.docsSvc.createDecisionsTabDecisions[i]
					if got.Category != want.Category || got.Text != want.Text {
						t.Errorf("CreateDecisionsTab decision[%d]: got {%s, %s}, want {%s, %s}",
							i, got.Category, got.Text, want.Category, want.Text)
					}
				}
				// Contract: transcript passed to CreateDecisionsTab matches extracted transcript
				if tt.docsSvc.createDecisionsTabTranscript != tt.docsSvc.transcriptContent {
					t.Error("CreateDecisionsTab transcript does not match extracted transcript")
				}
			}
		})
	}
}
