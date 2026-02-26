// Package organizer provides the main orchestration logic.
package organizer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/jflowers/gcal-organizer/internal/config"
	"github.com/jflowers/gcal-organizer/internal/docs"
	"github.com/jflowers/gcal-organizer/internal/drive"
	"github.com/jflowers/gcal-organizer/internal/logging"
	"github.com/jflowers/gcal-organizer/pkg/models"
)

// DocsService defines the Docs operations used for decision extraction.
type DocsService interface {
	ExtractTranscriptContent(ctx context.Context, docID string) (*models.TranscriptContent, error)
	HasDecisionsTab(ctx context.Context, docID string) (bool, error)
	CreateDecisionsTab(ctx context.Context, docID string, decisions []models.Decision, transcript *models.TranscriptContent) error
}

// DriveService defines the Drive operations used by the Organizer.
type DriveService interface {
	SetMasterFolder(ctx context.Context, folderName string) error
	ListMeetingDocuments(ctx context.Context, keywords []string) ([]*models.Document, error)
	GetOrCreateMeetingFolder(ctx context.Context, meetingName string) (*models.MeetingFolder, error)
	CreateShortcut(ctx context.Context, fileID, fileName, targetFolderID, targetFolderName string, folderIsNew bool) drive.ActionResult
	MoveDocument(ctx context.Context, docID, docName, currentParentID, targetFolderID, targetFolderName string) drive.ActionResult
	FindShortcutToFile(ctx context.Context, targetFileID, folderID string) (string, error)
	TrashFile(ctx context.Context, fileID, description string) drive.ActionResult
	ShareFile(ctx context.Context, fileID, fileName, email, role string) drive.ActionResult
	IsDryRun() bool
	IsFileOwned(ctx context.Context, fileID string) (bool, error)
	CanEditFile(ctx context.Context, fileID string) bool
	GetFileName(ctx context.Context, fileID string) (string, error)
}

// CalendarService defines the Calendar operations used by the Organizer.
type CalendarService interface {
	ListRecentEvents(ctx context.Context, daysBack int) ([]*models.CalendarEvent, error)
}

// GeminiService defines the Gemini operations used for decision extraction.
type GeminiService interface {
	ExtractDecisions(ctx context.Context, transcriptText string) ([]models.Decision, error)
}

// Stats tracks operation counts for summary reporting.
type Stats struct {
	DocumentsFound     int
	DocumentsMoved     int
	ShortcutsCreated   int
	ShortcutsTrashed   int
	EventsProcessed    int
	EventsWithAttach   int
	AttachmentsShared  int
	TasksAssigned      int
	TasksFailed        int
	DecisionsProcessed int
	DecisionsSkipped   int
	DecisionsFailed    int
	Skipped            int
	Errors             int
}

// Organizer orchestrates all the services.
type Organizer struct {
	config   *config.Config
	drive    DriveService
	calendar CalendarService
	logger   *log.Logger

	stats          Stats
	notesDocIDs    map[string]bool   // Google Doc IDs with "Notes" attachments
	decisionDocIDs map[string]string // Google Doc IDs for decision extraction (docID→source)
}

// New creates a new Organizer with all services initialized.
func New(cfg *config.Config, driveSvc DriveService, calSvc CalendarService) *Organizer {
	return &Organizer{
		config:         cfg,
		drive:          driveSvc,
		calendar:       calSvc,
		logger:         logging.Logger,
		notesDocIDs:    make(map[string]bool),
		decisionDocIDs: make(map[string]string),
	}
}

// GetNotesDocIDs returns the list of Google Doc IDs with "Notes" attachments.
func (o *Organizer) GetNotesDocIDs() []string {
	var ids []string
	for id := range o.notesDocIDs {
		ids = append(ids, id)
	}
	return ids
}

// GetDecisionDocIDs returns a copy of the map of Google Doc IDs eligible for decision extraction.
// Keys are doc IDs, values are the source pattern ("notes-by-gemini" or "transcript").
func (o *Organizer) GetDecisionDocIDs() map[string]string {
	result := make(map[string]string, len(o.decisionDocIDs))
	for k, v := range o.decisionDocIDs {
		result[k] = v
	}
	return result
}

// RunFullWorkflow executes all operations in sequence.
func (o *Organizer) RunFullWorkflow(ctx context.Context) error {
	if o.drive.IsDryRun() {
		o.logger.Warn("DRY RUN MODE — no changes will be made")
	}
	o.logger.Info("Starting full workflow")

	// Step 1: Organize documents
	o.logger.Info("STEP 1: Organizing Documents")
	if err := o.OrganizeDocuments(ctx); err != nil {
		return fmt.Errorf("organize documents failed: %w", err)
	}

	// Step 2: Sync calendar
	o.logger.Info("STEP 2: Syncing Calendar Attachments")
	if err := o.SyncCalendarAttachments(ctx); err != nil {
		return fmt.Errorf("sync calendar failed: %w", err)
	}

	// Note: Step 3 (Assign Tasks) is handled by the caller if needed,
	// since it requires browser automation that lives outside the organizer.

	return nil
}

// PrintSummary outputs the final statistics.
func (o *Organizer) PrintSummary() {
	o.printSummary()
}

// AddTaskStats updates the task assignment statistics.
func (o *Organizer) AddTaskStats(assigned, failed int) {
	o.stats.TasksAssigned += assigned
	o.stats.TasksFailed += failed
}

// AddDecisionStats updates the decision extraction statistics.
func (o *Organizer) AddDecisionStats(processed, skipped, failed int) {
	o.stats.DecisionsProcessed += processed
	o.stats.DecisionsSkipped += skipped
	o.stats.DecisionsFailed += failed
}

// ExtractDecisionsForDoc orchestrates decision extraction for a single document.
// It extracts the transcript, calls Gemini for decision extraction, and creates
// the Decisions tab. Skips on AI failure with a warning (FR-017).
func (o *Organizer) ExtractDecisionsForDoc(ctx context.Context, docID string, docsSvc DocsService, geminiSvc GeminiService, dryRun bool) error {
	// Check for existing Decisions tab (idempotency — FR-005)
	hasTab, err := docsSvc.HasDecisionsTab(ctx, docID)
	if err != nil {
		return fmt.Errorf("check decisions tab: %w", err)
	}
	if hasTab {
		o.logger.Info("Document already has Decisions tab, skipping", "docID", docID)
		o.stats.DecisionsSkipped++
		return nil
	}

	// Extract transcript content
	transcript, err := docsSvc.ExtractTranscriptContent(ctx, docID)
	if err != nil {
		return fmt.Errorf("extract transcript: %w", err)
	}
	if transcript == nil || transcript.FullText == "" {
		o.logger.Warn("No transcript content found, skipping", "docID", docID)
		o.stats.DecisionsSkipped++
		return nil
	}

	// Dry-run: log what would happen and return
	if dryRun {
		o.logger.Info("Would extract decisions from document",
			"docID", docID,
			"transcript_chars", len(transcript.FullText),
			"headings", len(transcript.Headings),
		)
		o.stats.DecisionsProcessed++
		return nil
	}

	// Call Gemini for decision extraction
	decisions, err := geminiSvc.ExtractDecisions(ctx, transcript.FullText)
	if err != nil {
		// Skip on AI failure with warning (FR-017)
		o.logger.Warn("AI decision extraction failed, skipping document",
			"docID", docID,
			"error", err,
		)
		o.stats.DecisionsFailed++
		return nil
	}

	o.logger.Info("Extracted decisions from transcript",
		"docID", docID,
		"decisions", len(decisions),
	)

	// Create the Decisions tab
	err = docsSvc.CreateDecisionsTab(ctx, docID, decisions, transcript)
	if err != nil {
		// Check for sentinel error indicating tab already exists (optimistic concurrency — FR-018)
		if errors.Is(err, docs.ErrDecisionsTabExists) {
			o.logger.Info("Decisions tab created by concurrent instance, skipping", "docID", docID)
			o.stats.DecisionsSkipped++
			return nil
		}
		o.stats.DecisionsFailed++
		return fmt.Errorf("create decisions tab: %w", err)
	}

	o.stats.DecisionsProcessed++
	return nil
}

// printSummary outputs the final statistics.
func (o *Organizer) printSummary() {
	if o.drive.IsDryRun() {
		o.logger.Info("DRY RUN SUMMARY",
			"docs_found", o.stats.DocumentsFound,
			"docs_moved", o.stats.DocumentsMoved,
			"shortcuts_created", o.stats.ShortcutsCreated,
			"events_processed", o.stats.EventsProcessed,
			"tasks_assigned", o.stats.TasksAssigned,
		)
		if o.stats.DecisionsProcessed > 0 {
			o.logger.Info("Would extract decisions", "documents", o.stats.DecisionsProcessed)
		}
		if o.stats.Skipped > 0 {
			o.logger.Info("Would skip non-owned files (--owned-only active)", "count", o.stats.Skipped)
		}
		o.logger.Info("Dry run complete — no changes were made")
	} else {
		o.logger.Info("WORKFLOW SUMMARY",
			"docs_found", o.stats.DocumentsFound,
			"docs_moved", o.stats.DocumentsMoved,
			"shortcuts_created", o.stats.ShortcutsCreated,
			"events_processed", o.stats.EventsProcessed,
			"events_with_attachments", o.stats.EventsWithAttach,
			"tasks_assigned", o.stats.TasksAssigned,
		)
		if o.stats.DecisionsProcessed > 0 || o.stats.DecisionsSkipped > 0 || o.stats.DecisionsFailed > 0 {
			o.logger.Info("Decision extraction",
				"processed", o.stats.DecisionsProcessed,
				"skipped", o.stats.DecisionsSkipped,
				"failed", o.stats.DecisionsFailed,
			)
		}
		if o.stats.ShortcutsTrashed > 0 {
			o.logger.Info("Cleanup", "shortcuts_trashed", o.stats.ShortcutsTrashed)
		}
		if o.stats.AttachmentsShared > 0 {
			o.logger.Info("Sharing", "attachments_shared", o.stats.AttachmentsShared)
		}
		if o.stats.TasksFailed > 0 {
			o.logger.Warn("Task failures", "failed", o.stats.TasksFailed)
		}
		if o.stats.Skipped > 0 {
			o.logger.Info("Skipped non-owned files (--owned-only active)", "count", o.stats.Skipped)
		}
		if o.stats.Errors > 0 {
			o.logger.Warn("Errors encountered", "count", o.stats.Errors)
		}
		o.logger.Info("Workflow complete")
	}
}

// OrganizeDocuments finds meeting documents and organizes them into folders.
func (o *Organizer) OrganizeDocuments(ctx context.Context) error {
	// Set up master folder
	if err := o.drive.SetMasterFolder(ctx, o.config.MasterFolderName); err != nil {
		return err
	}

	// Find documents
	documents, err := o.drive.ListMeetingDocuments(ctx, o.config.FilenameKeywords)
	if err != nil {
		return err
	}

	o.stats.DocumentsFound = len(documents)
	o.logger.Info("Found meeting documents", "count", len(documents))

	for _, doc := range documents {
		// Skip non-owned documents when --owned-only is active
		if o.config.OwnedOnly && !doc.IsOwned {
			o.stats.Skipped++
			if o.config.DryRun {
				o.logger.Info("Would skip non-owned document", "name", doc.Name)
			} else if o.config.Verbose {
				o.logger.Info("Skipping non-owned document", "name", doc.Name)
			}
			// Still create shortcut for discoverability (FR-005)
			folder, err := o.drive.GetOrCreateMeetingFolder(ctx, doc.MeetingName)
			if err != nil {
				o.logger.Error("Failed to get/create folder", "meeting", doc.MeetingName, "err", err)
				o.stats.Errors++
				continue
			}
			result := o.drive.CreateShortcut(ctx, doc.ID, doc.Name, folder.ID, folder.Name, folder.IsNew)
			o.logActionResult(result, false)
			continue
		}

		// Get or create meeting folder
		folder, err := o.drive.GetOrCreateMeetingFolder(ctx, doc.MeetingName)
		if err != nil {
			o.logger.Error("Failed to get/create folder", "meeting", doc.MeetingName, "err", err)
			o.stats.Errors++
			continue
		}

		var result drive.ActionResult
		if doc.IsOwned {
			// For owned fallback files, also clean up any redundant shortcut
			if doc.IsFallback && folder.ID != "" {
				shortcutID, err := o.drive.FindShortcutToFile(ctx, doc.ID, folder.ID)
				if err != nil {
					o.logger.Debug("Could not check for shortcuts", "err", err)
				} else if shortcutID != "" {
					// Found a shortcut pointing to this file - trash it
					trashResult := o.drive.TrashFile(ctx, shortcutID,
						fmt.Sprintf("Trash redundant shortcut to %s (file being moved)", doc.Name))
					if !trashResult.Skipped || trashResult.Reason == "dry-run" {
						o.stats.ShortcutsTrashed++
						o.logger.Info("Trashed redundant shortcut", "details", trashResult.Details)
					}
				}
			}
			result = o.drive.MoveDocument(ctx, doc.ID, doc.Name, doc.ParentFolderID, folder.ID, folder.Name)
		} else {
			result = o.drive.CreateShortcut(ctx, doc.ID, doc.Name, folder.ID, folder.Name, folder.IsNew)
		}

		o.logActionResult(result, doc.IsOwned)
	}

	return nil
}

// SyncCalendarAttachments syncs calendar event attachments to meeting folders.
func (o *Organizer) SyncCalendarAttachments(ctx context.Context) error {
	events, err := o.calendar.ListRecentEvents(ctx, o.config.DaysToLookBack)
	if err != nil {
		return err
	}

	o.stats.EventsProcessed = len(events)
	o.logger.Info("Found calendar events", "count", len(events), "days", o.config.DaysToLookBack)

	for _, event := range events {
		if len(event.Attachments) == 0 {
			continue
		}

		o.stats.EventsWithAttach++

		// Get or create meeting folder
		folder, err := o.drive.GetOrCreateMeetingFolder(ctx, event.Title)
		if err != nil {
			o.logger.Error("Failed to get/create folder", "event", event.Title, "err", err)
			o.stats.Errors++
			continue
		}

		// Per-event tracking for decision doc deduplication
		var eventDecisionDocID string
		var eventDecisionSource string

		for _, att := range event.Attachments {
			if att.FileID == "" {
				continue
			}

			title := att.Title
			if title == "" {
				// Fetch actual filename from Drive
				fileName, err := o.drive.GetFileName(ctx, att.FileID)
				if err != nil {
					title = fmt.Sprintf("attachment (%s...)", att.FileID[:min(8, len(att.FileID))])
				} else {
					title = fileName
				}
			}

			result := o.drive.CreateShortcut(ctx, att.FileID, title, folder.ID, folder.Name, folder.IsNew)
			o.logCalendarAction(result, event.Title, event.Start.Format("2006-01-02"), title)

			// Track Google Docs with "Notes" in the title for task assignment
			if att.MimeType == "application/vnd.google-apps.document" &&
				strings.Contains(strings.ToLower(title), "notes") {
				// When --owned-only is active, only collect owned docs for Step 3
				if o.config.OwnedOnly {
					owned, err := o.drive.IsFileOwned(ctx, att.FileID)
					if err != nil || !owned {
						continue
					}
				}
				o.notesDocIDs[att.FileID] = true
			}

			// Track Google Docs eligible for decision extraction (Step 4)
			// Exact match "Notes by Gemini" or suffix "- Transcript"
			if att.MimeType == "application/vnd.google-apps.document" {
				if title == "Notes by Gemini" {
					// "Notes by Gemini" always wins in per-event deduplication
					eventDecisionDocID = att.FileID
					eventDecisionSource = "notes-by-gemini"
				} else if strings.HasSuffix(title, "- Transcript") && eventDecisionSource != "notes-by-gemini" {
					// Only use transcript if no "Notes by Gemini" found yet for this event
					eventDecisionDocID = att.FileID
					eventDecisionSource = "transcript"
				}
			}
		}

		// Collect the decision doc for this event (if any), applying --owned-only filter
		if eventDecisionDocID != "" {
			if o.config.OwnedOnly {
				owned, err := o.drive.IsFileOwned(ctx, eventDecisionDocID)
				if err != nil || !owned {
					eventDecisionDocID = ""
				}
			}
			if eventDecisionDocID != "" {
				o.decisionDocIDs[eventDecisionDocID] = eventDecisionSource
			}
		}

		// Share attachments with attendees (edit access)
		for _, att := range event.Attachments {
			if att.FileID == "" {
				continue
			}

			// When --owned-only is active, only share files we own
			if o.config.OwnedOnly {
				owned, err := o.drive.IsFileOwned(ctx, att.FileID)
				if err != nil || !owned {
					o.stats.Skipped++
					if o.config.DryRun {
						o.logger.Info("Would skip sharing non-owned attachment", "attachment", att.Title)
					} else if o.config.Verbose {
						o.logger.Info("Skipping share for non-owned attachment", "attachment", att.Title)
					}
					continue
				}
			}

			// Only share if we have edit access to the attachment
			if !o.drive.CanEditFile(ctx, att.FileID) {
				o.logger.Debug("Skipping share — no edit access", "attachment", att.Title)
				continue
			}

			for _, attendee := range event.Attendees {
				if attendee.IsSelf || attendee.Email == "" || isCalendarResource(attendee.Email) {
					continue
				}

				result := o.drive.ShareFile(ctx, att.FileID, att.Title, attendee.Email, "writer")
				if !result.Skipped || result.Reason == "dry-run" {
					o.stats.AttachmentsShared++
					o.logger.Info("Shared attachment", "file", att.Title, "email", attendee.Email)
				} else if result.Reason != "already shared" {
					o.logger.Warn("Share attachment failed", "details", result.Details)
					o.stats.Errors++
				}
			}
		}
	}

	return nil
}

// isCalendarResource returns true for Google Calendar resource/group addresses
// that cannot be shared with (e.g. conference rooms, group calendars).
func isCalendarResource(email string) bool {
	return strings.HasSuffix(email, "@resource.calendar.google.com") ||
		strings.HasSuffix(email, "@group.calendar.google.com")
}

// logActionResult logs the result of a document action.
func (o *Organizer) logActionResult(result drive.ActionResult, isMove bool) {
	action := "shortcut"
	if isMove {
		action = "move"
	}

	if result.Skipped {
		if result.Reason == "already exists" || result.Reason == "already in folder" {
			o.logger.Debug("Skipped", "action", action, "details", result.Details)
		} else if result.Reason != "" && result.Reason != "dry-run" {
			o.stats.Errors++
			o.logger.Error("Action failed", "action", action, "details", result.Details, "reason", result.Reason)
		} else {
			// dry-run, still log what would happen
			if isMove {
				o.stats.DocumentsMoved++
			} else {
				o.stats.ShortcutsCreated++
			}
			o.logger.Info("Would "+action, "details", result.Details)
		}
	} else {
		if isMove {
			o.stats.DocumentsMoved++
		} else {
			o.stats.ShortcutsCreated++
		}
		o.logger.Debug("Completed", "action", action, "details", result.Details)
	}
}

// logCalendarAction logs the result of a calendar sync action.
func (o *Organizer) logCalendarAction(result drive.ActionResult, eventTitle, eventDate, attachmentName string) {
	event := fmt.Sprintf("%s (%s)", eventTitle, eventDate)

	if result.Skipped {
		if result.Reason == "already exists" {
			o.logger.Debug("Skipped attachment", "event", event, "attachment", attachmentName)
		} else if result.Reason != "" && result.Reason != "dry-run" {
			o.stats.Errors++
			o.logger.Error("Attachment sync failed", "event", event, "attachment", attachmentName, "reason", result.Reason)
		} else {
			// dry-run
			o.stats.ShortcutsCreated++
			o.logger.Info("Would link attachment", "event", event, "attachment", attachmentName)
		}
	} else {
		o.stats.ShortcutsCreated++
		o.logger.Debug("Linked attachment", "event", event, "attachment", attachmentName)
	}
}
