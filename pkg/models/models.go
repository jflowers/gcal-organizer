// Package models provides data structures used throughout the application.
package models

import "time"

// Document represents a Google Drive document with parsed meeting information.
type Document struct {
	// ID is the Google Drive file ID
	ID string `json:"id"`

	// Name is the original file name
	Name string `json:"name"`

	// MeetingName is the extracted meeting topic from the filename
	MeetingName string `json:"meetingName"`

	// Date is the parsed date from the filename
	Date time.Time `json:"date"`

	// MimeType is the MIME type of the document
	MimeType string `json:"mimeType"`

	// IsOwned indicates if the current user owns this document
	IsOwned bool `json:"isOwned"`

	// IsFallback indicates this was parsed via the fallback pattern (Notes - [name])
	IsFallback bool `json:"isFallback"`

	// ParentFolderID is the current parent folder ID
	ParentFolderID string `json:"parentFolderId"`

	// WebViewLink is the URL to view the document
	WebViewLink string `json:"webViewLink"`
}

// MeetingFolder represents a subfolder in the master folder for a meeting topic.
type MeetingFolder struct {
	// ID is the Google Drive folder ID
	ID string `json:"id"`

	// Name is the folder name (meeting topic)
	Name string `json:"name"`

	// ParentID is the parent folder ID (master folder)
	ParentID string `json:"parentId"`

	// IsNew indicates this folder was just created (or would be in dry-run)
	// Used to skip shortcut-exists checks since a new folder can't have existing shortcuts
	IsNew bool `json:"isNew"`
}

// CalendarEvent represents a Google Calendar event with attachments.
type CalendarEvent struct {
	// ID is the calendar event ID
	ID string `json:"id"`

	// Title is the event title
	Title string `json:"title"`

	// Start is the event start time
	Start time.Time `json:"start"`

	// End is the event end time
	End time.Time `json:"end"`

	// Description is the event description (may contain Drive links)
	Description string `json:"description"`

	// Attachments are the file attachments from the event
	Attachments []Attachment `json:"attachments"`

	// Attendees are the event attendees
	Attendees []Attendee `json:"attendees"`
}

// Attachment represents a file attached to a calendar event.
type Attachment struct {
	// FileID is the Google Drive file ID
	FileID string `json:"fileId"`

	// Title is the attachment title
	Title string `json:"title"`

	// MimeType is the MIME type
	MimeType string `json:"mimeType"`

	// FileURL is the URL to the file
	FileURL string `json:"fileUrl"`
}

// Decision represents a single decision extracted from a meeting transcript.
type Decision struct {
	// Category is the decision classification: "made", "deferred", or "open"
	Category string `json:"category"`

	// Text is the description of the decision
	Text string `json:"text"`

	// Timestamp is the associated meeting timestamp (HH:MM format, or empty)
	Timestamp string `json:"timestamp,omitempty"`

	// Context is a brief excerpt from the transcript providing context
	Context string `json:"context,omitempty"`
}

// TranscriptHeading represents an H3 timestamp heading in a Transcript tab.
type TranscriptHeading struct {
	// HeadingID is the server-assigned heading identifier (e.g., "h.xxxxxxxx")
	HeadingID string `json:"headingId"`

	// Text is the display text of the heading (e.g., "12:34")
	Text string `json:"text"`

	// Index is the zero-based position in the document body
	Index int64 `json:"index"`
}

// TranscriptContent aggregates the full parsed content of a transcript tab.
type TranscriptContent struct {
	// TabID is the Transcript tab identifier
	TabID string `json:"tabId"`

	// FullText is the complete text content of the transcript
	FullText string `json:"fullText"`

	// Headings is the ordered list of H3 timestamp headings
	Headings []TranscriptHeading `json:"headings"`
}

// Attendee represents a calendar event attendee.
type Attendee struct {
	// Email is the attendee's email address
	Email string `json:"email"`

	// DisplayName is the attendee's display name (if available)
	DisplayName string `json:"displayName,omitempty"`

	// IsSelf indicates if this attendee is the current user
	IsSelf bool `json:"isSelf"`

	// IsOrganizer indicates if this attendee is the event organizer
	IsOrganizer bool `json:"isOrganizer"`
}
