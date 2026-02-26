// Package docs provides Google Docs operations.
package docs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/jflowers/gcal-organizer/internal/retry"
	"github.com/jflowers/gcal-organizer/pkg/models"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// ProcessedEmoji is the emoji used to mark processed action items.
const ProcessedEmoji = "🆔"

// Service provides Google Docs operations.
type Service struct {
	client *docs.Service
}

// CheckboxItem represents a checkbox item found in a document.
type CheckboxItem struct {
	// Text is the text content of the checkbox item
	Text string

	// StartIndex is the position in the document
	StartIndex int64

	// EndIndex is the end position in the document
	EndIndex int64

	// IsChecked indicates if the checkbox is checked
	IsChecked bool

	// IsProcessed indicates if the item has already been processed (contains 🆔)
	IsProcessed bool
}

// NewService creates a new Docs service.
func NewService(ctx context.Context, httpClient *http.Client) (*Service, error) {
	srv, err := docs.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create Docs service: %w", err)
	}

	return &Service{client: srv}, nil
}

// GetDocument retrieves a document by ID.
func (s *Service) GetDocument(ctx context.Context, docID string) (*docs.Document, error) {
	var doc *docs.Document
	err := retry.Do(ctx, retry.DefaultConfig(), func() error {
		var e error
		doc, e = s.client.Documents.Get(docID).Context(ctx).Do()
		return e
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}
	return doc, nil
}

// SuggestedNextStepsHeading is the section heading to look for.
const SuggestedNextStepsHeading = "Suggested next steps"

// NotesTabName is the tab name to target.
const NotesTabName = "Notes"

// ExtractCheckboxItems finds checkbox items in the "Suggested next steps" section of the "Notes" tab.
func (s *Service) ExtractCheckboxItems(ctx context.Context, docID string) ([]*CheckboxItem, error) {
	doc, err := s.GetDocument(ctx, docID)
	if err != nil {
		return nil, err
	}

	// Find the content to process - either from the "Notes" tab or the main body
	var content []*docs.StructuralElement

	if len(doc.Tabs) > 0 {
		// Multi-tab document - look for "Notes" tab
		for _, tab := range doc.Tabs {
			if tab.TabProperties != nil && tab.TabProperties.Title == NotesTabName {
				if tab.DocumentTab != nil && tab.DocumentTab.Body != nil {
					content = tab.DocumentTab.Body.Content
				}
				break
			}
		}
		if content == nil {
			// Notes tab not found
			return nil, nil
		}
	} else if doc.Body != nil {
		// Single-tab or legacy document - use main body
		content = doc.Body.Content
	} else {
		return nil, nil
	}

	return s.extractItemsFromSection(content)
}

// extractItemsFromSection extracts checkbox items from the "Suggested next steps" section.
func (s *Service) extractItemsFromSection(content []*docs.StructuralElement) ([]*CheckboxItem, error) {
	var items []*CheckboxItem
	inSuggestedSection := false

	for _, elem := range content {
		if elem.Paragraph == nil {
			continue
		}

		para := elem.Paragraph

		// Check if this is the "Suggested next steps" heading
		paraText := extractParagraphText(para)
		if strings.Contains(strings.ToLower(paraText), strings.ToLower(SuggestedNextStepsHeading)) {
			inSuggestedSection = true
			continue
		}

		// If we haven't found the section yet, skip
		if !inSuggestedSection {
			continue
		}

		// Check if this is a list item (bullet or checkbox)
		if para.Bullet == nil {
			continue
		}

		content := strings.TrimSpace(paraText)
		if content == "" {
			continue
		}

		// Check if already processed
		isProcessed := strings.Contains(content, ProcessedEmoji)

		items = append(items, &CheckboxItem{
			Text:        content,
			StartIndex:  elem.StartIndex,
			EndIndex:    elem.EndIndex,
			IsChecked:   false,
			IsProcessed: isProcessed,
		})
	}

	return items, nil
}

// extractParagraphText extracts all text from a paragraph.
func extractParagraphText(para *docs.Paragraph) string {
	var text strings.Builder
	for _, textElem := range para.Elements {
		if textElem.TextRun != nil {
			text.WriteString(textElem.TextRun.Content)
		}
	}
	// Strip non-printable characters (vertical tabs, carriage returns, zero-width spaces, etc.)
	// that Google Docs API may include in TextRun.Content
	cleaned := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r == ' ' {
			return r // keep normal whitespace
		}
		if unicode.IsControl(r) || !unicode.IsPrint(r) {
			return -1 // strip
		}
		return r
	}, text.String())
	return strings.TrimSpace(cleaned)
}

// DecisionsTabTitle is the title of the Decisions tab.
const DecisionsTabTitle = "Decisions"

// contentLine represents a line of content to insert into a Decisions tab.
type contentLine struct {
	text      string
	isHeading bool   // H2 heading
	isBullet  bool   // bullet list item
	timestamp string // if non-empty, the [HH:MM] text to link to a heading
}

// extractTranscriptContentFromDoc extracts transcript content from a document structure.
// It looks for a tab titled "Transcript" in multi-tab docs, or uses the sole tab
// for single-tab docs. Returns nil if no transcript content is found.
func extractTranscriptContentFromDoc(doc *docs.Document) *models.TranscriptContent {
	if doc == nil {
		return nil
	}

	if len(doc.Tabs) == 0 {
		return nil
	}

	var targetTab *docs.Tab

	if len(doc.Tabs) == 1 {
		// Single-tab doc — use the only tab
		targetTab = doc.Tabs[0]
	} else {
		// Multi-tab doc — find "Transcript" tab
		for _, tab := range doc.Tabs {
			if tab.TabProperties != nil && tab.TabProperties.Title == "Transcript" {
				targetTab = tab
				break
			}
		}
	}

	if targetTab == nil {
		return nil
	}

	if targetTab.DocumentTab == nil || targetTab.DocumentTab.Body == nil {
		return nil
	}

	tabID := ""
	if targetTab.TabProperties != nil {
		tabID = targetTab.TabProperties.TabId
	}

	tc := &models.TranscriptContent{
		TabID: tabID,
	}

	var fullText strings.Builder
	for _, elem := range targetTab.DocumentTab.Body.Content {
		if elem.Paragraph == nil {
			continue
		}

		paraText := extractParagraphText(elem.Paragraph)

		fullText.WriteString(paraText)
		// Re-add the newline that extractParagraphText may have stripped
		if !strings.HasSuffix(paraText, "\n") {
			fullText.WriteString("\n")
		}

		// Check for H3 headings with HeadingId
		if elem.Paragraph.ParagraphStyle != nil &&
			elem.Paragraph.ParagraphStyle.NamedStyleType == "HEADING_3" &&
			elem.Paragraph.ParagraphStyle.HeadingId != "" {
			tc.Headings = append(tc.Headings, models.TranscriptHeading{
				HeadingID: elem.Paragraph.ParagraphStyle.HeadingId,
				Text:      strings.TrimSpace(paraText),
				Index:     elem.StartIndex,
			})
		}
	}

	tc.FullText = fullText.String()
	return tc
}

// ExtractTranscriptContent retrieves a document and extracts transcript content.
func (s *Service) ExtractTranscriptContent(ctx context.Context, docID string) (*models.TranscriptContent, error) {
	var doc *docs.Document
	err := retry.Do(ctx, retry.DefaultConfig(), func() error {
		var e error
		doc, e = s.client.Documents.Get(docID).IncludeTabsContent(true).Context(ctx).Do()
		return e
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get document for transcript extraction: %w", err)
	}

	return extractTranscriptContentFromDoc(doc), nil
}

// hasDecisionsTabInDoc checks if a document structure contains a "Decisions" tab.
func hasDecisionsTabInDoc(doc *docs.Document) bool {
	if doc == nil {
		return false
	}
	for _, tab := range doc.Tabs {
		if tab.TabProperties != nil && tab.TabProperties.Title == DecisionsTabTitle {
			return true
		}
	}
	return false
}

// HasDecisionsTab checks if a document already has a "Decisions" tab.
func (s *Service) HasDecisionsTab(ctx context.Context, docID string) (bool, error) {
	var doc *docs.Document
	err := retry.Do(ctx, retry.DefaultConfig(), func() error {
		var e error
		doc, e = s.client.Documents.Get(docID).Context(ctx).Do()
		return e
	})
	if err != nil {
		return false, fmt.Errorf("failed to get document for decisions tab check: %w", err)
	}

	return hasDecisionsTabInDoc(doc), nil
}

// matchTimestampToHeading finds the transcript heading that best matches a decision's timestamp.
// Returns nil if no headings exist or timestamp is empty.
func matchTimestampToHeading(timestamp string, headings []models.TranscriptHeading) *models.TranscriptHeading {
	if timestamp == "" || len(headings) == 0 {
		return nil
	}

	// First try exact match: heading text contains the timestamp
	for i := range headings {
		if strings.Contains(headings[i].Text, timestamp) {
			return &headings[i]
		}
	}

	// No exact match — find nearest heading by comparing timestamps
	// Parse decision timestamp as minutes since midnight for comparison
	decMinutes := parseTimestampMinutes(timestamp)
	if decMinutes < 0 {
		// Can't parse, return first heading as fallback
		return &headings[0]
	}

	// Find the nearest preceding heading (or first if timestamp is before all)
	var best *models.TranscriptHeading
	bestMinutes := -1

	for i := range headings {
		hMinutes := parseTimestampMinutes(headings[i].Text)
		if hMinutes < 0 {
			continue
		}

		if hMinutes <= decMinutes {
			if best == nil || hMinutes > bestMinutes {
				best = &headings[i]
				bestMinutes = hMinutes
			}
		}
	}

	if best != nil {
		return best
	}

	// Timestamp is before all headings — return the first heading
	// Or timestamp is after all headings — return the last heading
	if decMinutes < parseTimestampMinutes(headings[0].Text) {
		return &headings[0]
	}
	return &headings[len(headings)-1]
}

// parseTimestampMinutes parses "HH:MM" format and returns minutes since midnight.
// Returns -1 if parsing fails.
// Operates on the byte representation but validates that each character is an
// ASCII digit before arithmetic, making it safe for strings that contain
// multi-byte UTF-8 characters before the timestamp.
func parseTimestampMinutes(ts string) int {
	// Convert to bytes explicitly for safe indexing
	b := []byte(ts)
	for i := 0; i+4 < len(b); i++ {
		if b[i+2] != ':' {
			continue
		}
		h1, h2 := b[i], b[i+1]
		m1, m2 := b[i+3], b[i+4]
		// Verify all four positions are ASCII digits
		if h1 < '0' || h1 > '9' || h2 < '0' || h2 > '9' ||
			m1 < '0' || m1 > '5' || m2 < '0' || m2 > '9' {
			continue
		}
		hours := int(h1-'0')*10 + int(h2-'0')
		minutes := int(m1-'0')*10 + int(m2-'0')
		if hours <= 23 && minutes <= 59 {
			return hours*60 + minutes
		}
	}
	return -1
}

// utf16Len returns the number of UTF-16 code units needed to represent s.
// Google Docs API indices are measured in UTF-16 code units, not bytes or runes.
func utf16Len(s string) int64 {
	var n int64
	for _, r := range s {
		if r >= 0x10000 {
			n += 2 // surrogate pair
		} else {
			n++
		}
	}
	return n
}

// ErrDecisionsTabExists is returned when trying to create a Decisions tab that already exists.
var ErrDecisionsTabExists = errors.New("decisions tab already exists")

// buildAddTabRequest creates a BatchUpdate request to add a new tab.
func buildAddTabRequest(title string) *docs.Request {
	return &docs.Request{
		AddDocumentTab: &docs.AddDocumentTabRequest{
			TabProperties: &docs.TabProperties{
				Title: title,
			},
		},
	}
}

// buildDecisionsContent constructs the content lines for the Decisions tab.
func buildDecisionsContent(decisions []models.Decision) []contentLine {
	var lines []contentLine

	// Section headings
	sections := []struct {
		heading  string
		category string
	}{
		{"Decisions Made", "made"},
		{"Decisions Deferred", "deferred"},
		{"Open Items", "open"},
	}

	hasAnyDecisions := len(decisions) > 0

	for _, section := range sections {
		lines = append(lines, contentLine{text: section.heading, isHeading: true})

		found := false
		if hasAnyDecisions {
			for _, d := range decisions {
				if d.Category == section.category {
					bulletText := d.Text
					ts := ""
					if d.Timestamp != "" {
						bulletText = fmt.Sprintf("[%s] %s", d.Timestamp, d.Text)
						ts = d.Timestamp
					}
					lines = append(lines, contentLine{text: bulletText, isBullet: true, timestamp: ts})
					found = true
				}
			}
		}

		if !found {
			lines = append(lines, contentLine{text: "No decisions identified", isBullet: false})
		}
	}

	return lines
}

// CreateDecisionsTab creates a new "Decisions" tab in a document with categorized decisions.
func (s *Service) CreateDecisionsTab(ctx context.Context, docID string, decisions []models.Decision, transcript *models.TranscriptContent) error {
	// BatchUpdate #1: Create the Decisions tab
	addTabReq := buildAddTabRequest(DecisionsTabTitle)

	var batchResp *docs.BatchUpdateDocumentResponse
	err := retry.Do(ctx, retry.DefaultConfig(), func() error {
		var e error
		batchResp, e = s.client.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
			Requests: []*docs.Request{addTabReq},
		}).Context(ctx).Do()
		return e
	})
	if err != nil {
		// Check if the error indicates a duplicate tab (optimistic concurrency).
		// Use structured error inspection via googleapi.Error when possible.
		var apiErr *googleapi.Error
		if errors.As(err, &apiErr) {
			if apiErr.Code == 409 || strings.Contains(apiErr.Message, "already exists") || strings.Contains(apiErr.Message, "duplicate") {
				return ErrDecisionsTabExists
			}
		} else if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "duplicate") {
			return ErrDecisionsTabExists
		}
		return fmt.Errorf("failed to create Decisions tab: %w", err)
	}

	// Extract the new tab ID from the response
	var newTabID string
	if batchResp != nil {
		for _, reply := range batchResp.Replies {
			if reply.AddDocumentTab != nil && reply.AddDocumentTab.TabProperties != nil {
				newTabID = reply.AddDocumentTab.TabProperties.TabId
				break
			}
		}
	}
	if newTabID == "" {
		return fmt.Errorf("no TabId returned from AddDocumentTab response")
	}

	// Build content for the Decisions tab
	content := buildDecisionsContent(decisions)

	// BatchUpdate #2: Insert content into the new tab
	var requests []*docs.Request

	// Build text content — insert from bottom to top so indices don't shift.
	// Google Docs new tab starts with index 1 (empty paragraph).
	// We insert all text at index 1 in reverse order.
	var fullText strings.Builder
	for _, line := range content {
		fullText.WriteString(line.text)
		fullText.WriteString("\n")
	}

	// Insert all text at once at position 1
	textToInsert := fullText.String()
	if textToInsert != "" {
		requests = append(requests, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Text: textToInsert,
				Location: &docs.Location{
					Index: 1,
					TabId: newTabID,
				},
			},
		})
	}

	// Now apply styles — calculate positions based on inserted text.
	// Google Docs API uses UTF-16 code unit indices, not byte indices.
	offset := int64(1) // Starting position after insert
	for _, line := range content {
		lineLen := utf16Len(line.text) + 1 // +1 for newline
		startIdx := offset
		endIdx := offset + lineLen

		if line.isHeading {
			requests = append(requests, &docs.Request{
				UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
					Range: &docs.Range{
						StartIndex: startIdx,
						EndIndex:   endIdx,
						TabId:      newTabID,
					},
					ParagraphStyle: &docs.ParagraphStyle{
						NamedStyleType: "HEADING_2",
					},
					Fields: "namedStyleType",
				},
			})
		}

		if line.isBullet {
			requests = append(requests, &docs.Request{
				CreateParagraphBullets: &docs.CreateParagraphBulletsRequest{
					Range: &docs.Range{
						StartIndex: startIdx,
						EndIndex:   endIdx,
						TabId:      newTabID,
					},
					BulletPreset: "BULLET_DISC_CIRCLE_SQUARE",
				},
			})

			// Add cross-tab heading link for timestamp text (US2 — FR-012)
			if line.timestamp != "" && transcript != nil && len(transcript.Headings) > 0 {
				heading := matchTimestampToHeading(line.timestamp, transcript.Headings)
				if heading != nil {
					// The timestamp text is formatted as "[HH:MM]" at the start of the line
					tsText := fmt.Sprintf("[%s]", line.timestamp)
					tsEndIdx := startIdx + utf16Len(tsText)
					if tsEndIdx <= endIdx {
						requests = append(requests, &docs.Request{
							UpdateTextStyle: &docs.UpdateTextStyleRequest{
								Range: &docs.Range{
									StartIndex: startIdx,
									EndIndex:   tsEndIdx,
									TabId:      newTabID,
								},
								TextStyle: &docs.TextStyle{
									Link: &docs.Link{
										Heading: &docs.HeadingLink{
											Id:    heading.HeadingID,
											TabId: transcript.TabID,
										},
									},
								},
								Fields: "link",
							},
						})
					}
				}
			}
		}

		offset += lineLen
	}

	if len(requests) > 0 {
		err = retry.Do(ctx, retry.DefaultConfig(), func() error {
			_, e := s.client.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
				Requests: requests,
			}).Context(ctx).Do()
			return e
		})
		if err != nil {
			return fmt.Errorf("failed to insert content into Decisions tab: %w", err)
		}
	}

	return nil
}
