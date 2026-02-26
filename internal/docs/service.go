// Package docs provides Google Docs operations.
package docs

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/jflowers/gcal-organizer/internal/retry"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"
)

// ProcessedEmoji is the emoji used to mark processed action items.
const ProcessedEmoji = "🆔"

// ClaimEmoji is the emoji used to mark items being actively assigned.
// Includes a timestamp for stale claim detection.
const ClaimEmoji = "⏳"

// ClaimTTL is how long a claim is valid before it's considered stale.
const ClaimTTL = 10 * time.Minute

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

	// IsClaimed indicates if another instance has claimed this item (contains ⏳)
	IsClaimed bool
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

		// Check if already processed or claimed
		isProcessed := strings.Contains(content, ProcessedEmoji)
		isClaimed := isClaimActive(content)

		items = append(items, &CheckboxItem{
			Text:        content,
			StartIndex:  elem.StartIndex,
			EndIndex:    elem.EndIndex,
			IsChecked:   false,
			IsProcessed: isProcessed,
			IsClaimed:   isClaimed,
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

// isClaimActive checks if a line contains an active (non-expired) ⏳ claim.
func isClaimActive(text string) bool {
	idx := strings.Index(text, ClaimEmoji)
	if idx < 0 {
		return false
	}

	// Extract the timestamp after ⏳
	after := text[idx+len(ClaimEmoji):]
	// Trim any trailing whitespace or characters
	after = strings.TrimSpace(after)
	if after == "" {
		// Claim with no timestamp — treat as active (conservative)
		return true
	}

	// Try to parse RFC3339 timestamp
	// Take first 20 chars max (length of 2026-02-09T12:38:10Z)
	if len(after) > 25 {
		after = after[:25]
	}
	claimTime, err := time.Parse(time.RFC3339, after)
	if err != nil {
		// Can't parse timestamp — treat as active (conservative)
		return true
	}

	// Check if claim has expired
	return time.Since(claimTime) < ClaimTTL
}
