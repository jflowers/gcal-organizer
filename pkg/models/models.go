// Package models provides data structures used throughout the application.
package models

import "time"

// Document represents a Google Drive document with parsed meeting information.
type Document struct {
	// ID is the Google Drive file ID
	ID string

	// Name is the original file name
	Name string

	// MeetingName is the extracted meeting topic from the filename
	MeetingName string

	// Date is the parsed date from the filename
	Date time.Time

	// MimeType is the MIME type of the document
	MimeType string

	// IsOwned indicates if the current user owns this document
	IsOwned bool

	// IsFallback indicates this was parsed via the fallback pattern (Notes - [name])
	IsFallback bool

	// ParentFolderID is the current parent folder ID
	ParentFolderID string

	// WebViewLink is the URL to view the document
	WebViewLink string
}

// MeetingFolder represents a subfolder in the master folder for a meeting topic.
type MeetingFolder struct {
	// ID is the Google Drive folder ID
	ID string

	// Name is the folder name (meeting topic)
	Name string

	// ParentID is the parent folder ID (master folder)
	ParentID string

	// IsNew indicates this folder was just created (or would be in dry-run)
	// Used to skip shortcut-exists checks since a new folder can't have existing shortcuts
	IsNew bool

	// DocumentCount is the number of documents in this folder
	DocumentCount int
}

// CalendarEvent represents a Google Calendar event with attachments.
type CalendarEvent struct {
	// ID is the calendar event ID
	ID string

	// Title is the event title
	Title string

	// Start is the event start time
	Start time.Time

	// End is the event end time
	End time.Time

	// Description is the event description (may contain Drive links)
	Description string

	// Attachments are the file attachments from the event
	Attachments []Attachment

	// Attendees are the event attendees
	Attendees []Attendee
}

// Attachment represents a file attached to a calendar event.
type Attachment struct {
	// FileID is the Google Drive file ID
	FileID string

	// Title is the attachment title
	Title string

	// MimeType is the MIME type
	MimeType string

	// FileURL is the URL to the file
	FileURL string
}

// Decision represents a single decision extracted from a meeting transcript.
type Decision struct {
	// Category is the decision classification: "made", "deferred", or "open"
	Category string

	// Text is the description of the decision
	Text string

	// Timestamp is the associated meeting timestamp (HH:MM format, or empty)
	Timestamp string

	// Context is a brief excerpt from the transcript providing context
	Context string
}

// TranscriptHeading represents an H3 timestamp heading in a Transcript tab.
type TranscriptHeading struct {
	// HeadingID is the server-assigned heading identifier (e.g., "h.xxxxxxxx")
	HeadingID string

	// Text is the display text of the heading (e.g., "12:34")
	Text string

	// Index is the zero-based position in the document body
	Index int64
}

// TranscriptContent aggregates the full parsed content of a transcript tab.
type TranscriptContent struct {
	// TabID is the Transcript tab identifier
	TabID string

	// FullText is the complete text content of the transcript
	FullText string

	// Headings is the ordered list of H3 timestamp headings
	Headings []TranscriptHeading
}

// Attendee represents a calendar event attendee.
type Attendee struct {
	// Email is the attendee's email address
	Email string

	// DisplayName is the attendee's display name (if available)
	DisplayName string

	// IsSelf indicates if this attendee is the current user
	IsSelf bool

	// IsOrganizer indicates if this attendee is the event organizer
	IsOrganizer bool
}
