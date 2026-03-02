package docs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jflowers/gcal-organizer/pkg/models"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/option"
)

// ---------- T010: ExtractTranscriptContent tests ----------

// buildTestDoc creates a docs.Document with the given tabs for testing.
func buildTestDoc(tabs []*docs.Tab) *docs.Document {
	return &docs.Document{
		DocumentId: "test-doc-id",
		Tabs:       tabs,
	}
}

// buildTab creates a tab with title, content elements, and a tabID.
func buildTab(title, tabID string, elements []*docs.StructuralElement) *docs.Tab {
	return &docs.Tab{
		TabProperties: &docs.TabProperties{
			Title: title,
			TabId: tabID,
		},
		DocumentTab: &docs.DocumentTab{
			Body: &docs.Body{
				Content: elements,
			},
		},
	}
}

// buildParagraphElement creates a structural element with paragraph text and optional heading style.
func buildParagraphElement(text string, startIndex, endIndex int64, headingStyle string, headingID string) *docs.StructuralElement {
	para := &docs.Paragraph{
		Elements: []*docs.ParagraphElement{
			{
				TextRun: &docs.TextRun{
					Content: text,
				},
			},
		},
		ParagraphStyle: &docs.ParagraphStyle{},
	}
	if headingStyle != "" {
		para.ParagraphStyle.NamedStyleType = headingStyle
	}
	if headingID != "" {
		para.ParagraphStyle.HeadingId = headingID
	}

	return &docs.StructuralElement{
		StartIndex: startIndex,
		EndIndex:   endIndex,
		Paragraph:  para,
	}
}

func TestExtractTranscriptContent(t *testing.T) {
	tests := []struct {
		name             string
		doc              *docs.Document
		wantTabID        string
		wantFullText     string
		wantHeadingCount int
		wantNil          bool
		// Contract: per-heading field assertions
		wantHeadings []models.TranscriptHeading
	}{
		{
			name: "finds Transcript tab in multi-tab doc",
			doc: buildTestDoc([]*docs.Tab{
				buildTab("Notes", "tab-notes", []*docs.StructuralElement{
					buildParagraphElement("Some notes\n", 0, 11, "", ""),
				}),
				buildTab("Transcript", "tab-transcript", []*docs.StructuralElement{
					buildParagraphElement("12:00\n", 0, 6, "HEADING_3", "h.abc123"),
					buildParagraphElement("Hello everyone\n", 6, 21, "", ""),
					buildParagraphElement("12:15\n", 21, 27, "HEADING_3", "h.def456"),
					buildParagraphElement("Moving on to the next topic\n", 27, 55, "", ""),
				}),
			}),
			wantTabID:        "tab-transcript",
			wantFullText:     "12:00\nHello everyone\n12:15\nMoving on to the next topic\n",
			wantHeadingCount: 2,
			wantHeadings: []models.TranscriptHeading{
				{HeadingID: "h.abc123", Text: "12:00", Index: 0},
				{HeadingID: "h.def456", Text: "12:15", Index: 21},
			},
		},
		{
			name: "uses first tab content for single-tab doc",
			doc: buildTestDoc([]*docs.Tab{
				buildTab("", "tab-only", []*docs.StructuralElement{
					buildParagraphElement("10:00\n", 0, 6, "HEADING_3", "h.single1"),
					buildParagraphElement("Discussion content\n", 6, 25, "", ""),
				}),
			}),
			wantTabID:        "tab-only",
			wantFullText:     "10:00\nDiscussion content\n",
			wantHeadingCount: 1,
			wantHeadings: []models.TranscriptHeading{
				{HeadingID: "h.single1", Text: "10:00", Index: 0},
			},
		},
		{
			name: "returns nil for doc with no transcript tab in multi-tab doc",
			doc: buildTestDoc([]*docs.Tab{
				buildTab("Notes", "tab-notes", []*docs.StructuralElement{
					buildParagraphElement("Some notes\n", 0, 11, "", ""),
				}),
				buildTab("Action Items", "tab-actions", []*docs.StructuralElement{
					buildParagraphElement("Do something\n", 0, 13, "", ""),
				}),
			}),
			wantNil: true,
		},
		{
			name:    "returns nil for nil document",
			doc:     nil,
			wantNil: true,
		},
		{
			name:    "returns nil for document with zero tabs",
			doc:     buildTestDoc([]*docs.Tab{}),
			wantNil: true,
		},
		{
			name: "extracts H3 heading metadata with HeadingID, Text, and Index",
			doc: buildTestDoc([]*docs.Tab{
				buildTab("Transcript", "tab-t", []*docs.StructuralElement{
					buildParagraphElement("09:30\n", 0, 6, "HEADING_3", "h.head1"),
					buildParagraphElement("Content here\n", 6, 19, "", ""),
					buildParagraphElement("09:45\n", 19, 25, "HEADING_3", "h.head2"),
					buildParagraphElement("More content\n", 25, 38, "", ""),
					buildParagraphElement("10:00\n", 38, 44, "HEADING_3", "h.head3"),
				}),
			}),
			wantTabID:        "tab-t",
			wantHeadingCount: 3,
			wantHeadings: []models.TranscriptHeading{
				{HeadingID: "h.head1", Text: "09:30", Index: 0},
				{HeadingID: "h.head2", Text: "09:45", Index: 19},
				{HeadingID: "h.head3", Text: "10:00", Index: 38},
			},
		},
		{
			name: "skips non-H3 headings and headings without HeadingId",
			doc: buildTestDoc([]*docs.Tab{
				buildTab("Transcript", "tab-skip", []*docs.StructuralElement{
					buildParagraphElement("Big Title\n", 0, 10, "HEADING_1", "h.big"),
					buildParagraphElement("09:30\n", 10, 16, "HEADING_3", "h.valid"),
					buildParagraphElement("No ID\n", 16, 22, "HEADING_3", ""), // no HeadingId
				}),
			}),
			wantTabID:        "tab-skip",
			wantHeadingCount: 1,
			wantHeadings: []models.TranscriptHeading{
				{HeadingID: "h.valid", Text: "09:30", Index: 10},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTranscriptContentFromDoc(tt.doc)

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil TranscriptContent, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("expected non-nil TranscriptContent, got nil")
			}

			// Contract: TabID must match the selected tab
			if result.TabID != tt.wantTabID {
				t.Errorf("TabID: got %q, want %q", result.TabID, tt.wantTabID)
			}

			// Contract: FullText must concatenate all paragraph text with newlines
			if tt.wantFullText != "" && result.FullText != tt.wantFullText {
				t.Errorf("FullText: got %q, want %q", result.FullText, tt.wantFullText)
			}

			// Contract: Headings count must match
			if len(result.Headings) != tt.wantHeadingCount {
				t.Errorf("Headings count: got %d, want %d", len(result.Headings), tt.wantHeadingCount)
			}

			// Contract: each heading must have correct HeadingID, Text, and Index
			for i, want := range tt.wantHeadings {
				if i >= len(result.Headings) {
					t.Errorf("heading[%d]: missing (only %d headings returned)", i, len(result.Headings))
					continue
				}
				got := result.Headings[i]
				if got.HeadingID != want.HeadingID {
					t.Errorf("heading[%d].HeadingID: got %q, want %q", i, got.HeadingID, want.HeadingID)
				}
				if got.Text != want.Text {
					t.Errorf("heading[%d].Text: got %q, want %q", i, got.Text, want.Text)
				}
				if got.Index != want.Index {
					t.Errorf("heading[%d].Index: got %d, want %d", i, got.Index, want.Index)
				}
			}
		})
	}
}

// ---------- T011: CreateDecisionsTab tests ----------

func TestCreateDecisionsTab_RequestStructure(t *testing.T) {
	// Test that CreateDecisionsTab builds correct batch update requests.
	// Since we can't easily test actual API calls, we test the request building logic.

	tests := []struct {
		name       string
		decisions  []models.Decision
		transcript *models.TranscriptContent
		wantTitle  string
	}{
		{
			name: "creates tab with correct title",
			decisions: []models.Decision{
				{Category: "made", Text: "Adopt new pipeline", Timestamp: "12:34"},
			},
			transcript: &models.TranscriptContent{
				TabID:    "tab-transcript",
				FullText: "Some transcript text",
			},
			wantTitle: "Decisions",
		},
		{
			name:      "handles empty decisions list",
			decisions: []models.Decision{},
			transcript: &models.TranscriptContent{
				TabID:    "tab-transcript",
				FullText: "Some transcript text",
			},
			wantTitle: "Decisions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the request building helper
			req := buildAddTabRequest(tt.wantTitle)
			if req.AddDocumentTab == nil {
				t.Fatal("expected AddDocumentTab request")
			}
			if req.AddDocumentTab.TabProperties.Title != tt.wantTitle {
				t.Errorf("tab title: got %q, want %q", req.AddDocumentTab.TabProperties.Title, tt.wantTitle)
			}
		})
	}
}

func TestBuildDecisionsContent(t *testing.T) {
	tests := []struct {
		name      string
		decisions []models.Decision
		// Contract: structural checks on the returned contentLine slice
		wantHeadingTexts []string // ordered H2 section headings
		wantBulletTexts  []string // ordered bullet items
		wantTimestamps   []string // timestamp field for each bullet (empty if none)
	}{
		{
			name: "three categorized sections with decisions",
			decisions: []models.Decision{
				{Category: "made", Text: "Adopt new pipeline", Timestamp: "12:34"},
				{Category: "deferred", Text: "Budget review", Timestamp: "13:00"},
				{Category: "open", Text: "API migration", Timestamp: "13:45"},
			},
			wantHeadingTexts: []string{"Decisions Made", "Decisions Deferred", "Open Items"},
			wantBulletTexts:  []string{"[12:34] Adopt new pipeline", "[13:00] Budget review", "[13:45] API migration"},
			wantTimestamps:   []string{"12:34", "13:00", "13:45"},
		},
		{
			name:             "empty decisions shows placeholder in every section",
			decisions:        []models.Decision{},
			wantHeadingTexts: []string{"Decisions Made", "Decisions Deferred", "Open Items"},
			wantBulletTexts:  []string{},
		},
		{
			name: "decisions in single category — empty sections get placeholder",
			decisions: []models.Decision{
				{Category: "made", Text: "First decision"},
				{Category: "made", Text: "Second decision"},
			},
			wantHeadingTexts: []string{"Decisions Made", "Decisions Deferred", "Open Items"},
			wantBulletTexts:  []string{"First decision", "Second decision"},
			wantTimestamps:   []string{"", ""},
		},
		{
			name: "decision without timestamp has no bracket prefix",
			decisions: []models.Decision{
				{Category: "open", Text: "Review metrics", Timestamp: ""},
			},
			wantHeadingTexts: []string{"Decisions Made", "Decisions Deferred", "Open Items"},
			wantBulletTexts:  []string{"Review metrics"},
			wantTimestamps:   []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := buildDecisionsContent(tt.decisions)

			// Contract: must always produce exactly 3 section headings
			var gotHeadings []contentLine
			var gotBullets []contentLine
			var gotPlain []contentLine
			for _, line := range content {
				if line.isHeading {
					gotHeadings = append(gotHeadings, line)
				} else if line.isBullet {
					gotBullets = append(gotBullets, line)
				} else {
					gotPlain = append(gotPlain, line)
				}
			}

			if len(gotHeadings) != len(tt.wantHeadingTexts) {
				t.Fatalf("heading count: got %d, want %d", len(gotHeadings), len(tt.wantHeadingTexts))
			}
			for i, want := range tt.wantHeadingTexts {
				if gotHeadings[i].text != want {
					t.Errorf("heading[%d].text: got %q, want %q", i, gotHeadings[i].text, want)
				}
				// Contract: heading lines must have isHeading=true, isBullet=false
				if !gotHeadings[i].isHeading {
					t.Errorf("heading[%d].isHeading: got false, want true", i)
				}
				if gotHeadings[i].isBullet {
					t.Errorf("heading[%d].isBullet: got true, want false", i)
				}
			}

			// Contract: bullet items must have isBullet=true and correct text
			if len(gotBullets) != len(tt.wantBulletTexts) {
				t.Fatalf("bullet count: got %d, want %d", len(gotBullets), len(tt.wantBulletTexts))
			}
			for i, want := range tt.wantBulletTexts {
				if gotBullets[i].text != want {
					t.Errorf("bullet[%d].text: got %q, want %q", i, gotBullets[i].text, want)
				}
				if !gotBullets[i].isBullet {
					t.Errorf("bullet[%d].isBullet: got false, want true", i)
				}
			}

			// Contract: timestamp field must be populated for timestamped decisions
			for i, wantTS := range tt.wantTimestamps {
				if i >= len(gotBullets) {
					break
				}
				if gotBullets[i].timestamp != wantTS {
					t.Errorf("bullet[%d].timestamp: got %q, want %q", i, gotBullets[i].timestamp, wantTS)
				}
			}

			// Contract: sections without decisions must produce a plain "No decisions identified" line
			if len(tt.decisions) == 0 {
				placeholderCount := 0
				for _, line := range gotPlain {
					if line.text == "No decisions identified" {
						placeholderCount++
					}
				}
				if placeholderCount != 3 {
					t.Errorf("empty decisions: want 3 'No decisions identified' placeholders, got %d", placeholderCount)
				}
			}
		})
	}
}

// Verify the extractTranscriptContentFromDoc function is accessible (compile check)
func TestExtractTranscriptContentFromDoc_NilDoc(t *testing.T) {
	result := extractTranscriptContentFromDoc(nil)
	if result != nil {
		t.Errorf("expected nil for nil doc, got %+v", result)
	}
}

// Verify buildAddTabRequest exists and returns correct structure
func TestBuildAddTabRequest(t *testing.T) {
	req := buildAddTabRequest("Decisions")
	if req.AddDocumentTab == nil {
		t.Fatal("expected AddDocumentTab to be non-nil")
	}
	if req.AddDocumentTab.TabProperties == nil {
		t.Fatal("expected TabProperties to be non-nil")
	}
	if req.AddDocumentTab.TabProperties.Title != "Decisions" {
		t.Errorf("expected title 'Decisions', got %q", req.AddDocumentTab.TabProperties.Title)
	}
}

// Compile-time check that Service type exists and is constructible.
// Interface satisfaction (organizer.DocsService) is verified in cmd/gcal-organizer/main.go
// to avoid circular imports.
var _ = (*Service)(nil)

// ---------- T024: HasDecisionsTab tests ----------

func TestHasDecisionsTab(t *testing.T) {
	tests := []struct {
		name    string
		doc     *docs.Document
		wantHas bool
	}{
		{
			name: "returns true when Decisions tab exists among others",
			doc: buildTestDoc([]*docs.Tab{
				buildTab("Notes", "tab-notes", nil),
				buildTab("Decisions", "tab-decisions", nil),
			}),
			wantHas: true,
		},
		{
			name: "returns false when no Decisions tab present",
			doc: buildTestDoc([]*docs.Tab{
				buildTab("Notes", "tab-notes", nil),
				buildTab("Transcript", "tab-transcript", nil),
			}),
			wantHas: false,
		},
		{
			name: "returns true when Decisions is the only tab",
			doc: buildTestDoc([]*docs.Tab{
				buildTab("Decisions", "tab-manual-decisions", nil),
			}),
			wantHas: true,
		},
		{
			name:    "returns false for empty tabs slice",
			doc:     buildTestDoc([]*docs.Tab{}),
			wantHas: false,
		},
		{
			name:    "returns false for nil document",
			doc:     nil,
			wantHas: false,
		},
		{
			name: "case-sensitive: 'decisions' lowercase does not match",
			doc: buildTestDoc([]*docs.Tab{
				buildTab("decisions", "tab-lower", nil),
			}),
			wantHas: false,
		},
		{
			name: "tab with nil TabProperties is safely skipped",
			doc: &docs.Document{
				Tabs: []*docs.Tab{
					{TabProperties: nil},
					buildTab("Decisions", "tab-ok", nil),
				},
			},
			wantHas: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasDecisionsTabInDoc(tt.doc)
			// Contract: boolean return must exactly match title comparison
			if got != tt.wantHas {
				t.Errorf("hasDecisionsTabInDoc: got %v, want %v", got, tt.wantHas)
			}
		})
	}
}

// ---------- T025: Optimistic concurrency tests ----------

func TestErrDecisionsTabExists(t *testing.T) {
	// Verify that the sentinel error exists and has the expected message
	if ErrDecisionsTabExists == nil {
		t.Fatal("ErrDecisionsTabExists should not be nil")
	}
	if ErrDecisionsTabExists.Error() != "decisions tab already exists" {
		t.Errorf("expected error message 'decisions tab already exists', got %q", ErrDecisionsTabExists.Error())
	}
}

// ---------- parseTimestampMinutes tests ----------

func TestParseTimestampMinutes(t *testing.T) {
	tests := []struct {
		name string
		ts   string
		want int
	}{
		{"standard time", "12:34", 754},
		{"midnight", "00:00", 0},
		{"end of day", "23:59", 1439},
		{"morning", "09:00", 540},
		{"embedded in text", "Meeting at 10:30 today", 630},
		{"empty string", "", -1},
		{"invalid", "not a time", -1},
		{"too short", "12:", -1},
		{"just digits", "1234", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTimestampMinutes(tt.ts)
			if got != tt.want {
				t.Errorf("parseTimestampMinutes(%q): got %d, want %d", tt.ts, got, tt.want)
			}
		})
	}
}

// ---------- T019: Timestamp to heading matching tests ----------

func TestTimestampToHeadingMatch(t *testing.T) {
	headings := []models.TranscriptHeading{
		{HeadingID: "h.head1", Text: "09:30", Index: 0},
		{HeadingID: "h.head2", Text: "09:45", Index: 19},
		{HeadingID: "h.head3", Text: "10:00", Index: 38},
		{HeadingID: "h.head4", Text: "10:15", Index: 57},
	}

	tests := []struct {
		name      string
		timestamp string
		headings  []models.TranscriptHeading
		wantNil   bool
		// Contract: full heading struct equality (not just HeadingID)
		wantHeading *models.TranscriptHeading
	}{
		{
			name:        "exact timestamp match returns correct heading struct",
			timestamp:   "09:45",
			headings:    headings,
			wantHeading: &models.TranscriptHeading{HeadingID: "h.head2", Text: "09:45", Index: 19},
		},
		{
			name:        "nearest preceding heading when no exact match",
			timestamp:   "09:50",
			headings:    headings,
			wantHeading: &models.TranscriptHeading{HeadingID: "h.head2", Text: "09:45", Index: 19},
		},
		{
			name:      "no headings returns nil",
			timestamp: "10:00",
			headings:  nil,
			wantNil:   true,
		},
		{
			name:      "empty headings returns nil",
			timestamp: "10:00",
			headings:  []models.TranscriptHeading{},
			wantNil:   true,
		},
		{
			name:        "exact match at first heading",
			timestamp:   "09:30",
			headings:    headings,
			wantHeading: &models.TranscriptHeading{HeadingID: "h.head1", Text: "09:30", Index: 0},
		},
		{
			name:        "exact match at last heading",
			timestamp:   "10:15",
			headings:    headings,
			wantHeading: &models.TranscriptHeading{HeadingID: "h.head4", Text: "10:15", Index: 57},
		},
		{
			name:        "timestamp before any heading uses first heading",
			timestamp:   "08:00",
			headings:    headings,
			wantHeading: &models.TranscriptHeading{HeadingID: "h.head1", Text: "09:30", Index: 0},
		},
		{
			name:        "timestamp after all headings uses last heading",
			timestamp:   "11:00",
			headings:    headings,
			wantHeading: &models.TranscriptHeading{HeadingID: "h.head4", Text: "10:15", Index: 57},
		},
		{
			name:      "empty timestamp returns nil",
			timestamp: "",
			headings:  headings,
			wantNil:   true,
		},
		{
			name:        "unparseable timestamp falls back to first heading",
			timestamp:   "not-a-time",
			headings:    headings,
			wantHeading: &models.TranscriptHeading{HeadingID: "h.head1", Text: "09:30", Index: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchTimestampToHeading(tt.timestamp, tt.headings)

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("expected non-nil result, got nil")
			}

			// Contract: returned heading must match all fields
			if result.HeadingID != tt.wantHeading.HeadingID {
				t.Errorf("HeadingID: got %q, want %q", result.HeadingID, tt.wantHeading.HeadingID)
			}
			if result.Text != tt.wantHeading.Text {
				t.Errorf("Text: got %q, want %q", result.Text, tt.wantHeading.Text)
			}
			if result.Index != tt.wantHeading.Index {
				t.Errorf("Index: got %d, want %d", result.Index, tt.wantHeading.Index)
			}
		})
	}
}

// ---------- T020: CreateDecisionsTab with links tests ----------

func TestBuildDecisionsContentWithTimestamp(t *testing.T) {
	decisions := []models.Decision{
		{Category: "made", Text: "Adopt new pipeline", Timestamp: "12:34"},
		{Category: "made", Text: "No timestamp decision", Timestamp: ""},
	}

	content := buildDecisionsContent(decisions)

	// Collect bullets only
	var bullets []contentLine
	for _, line := range content {
		if line.isBullet {
			bullets = append(bullets, line)
		}
	}

	if len(bullets) != 2 {
		t.Fatalf("expected 2 bullets, got %d", len(bullets))
	}

	// Contract: timestamped decision formatted as "[HH:MM] text"
	if bullets[0].text != "[12:34] Adopt new pipeline" {
		t.Errorf("bullet[0].text: got %q, want %q", bullets[0].text, "[12:34] Adopt new pipeline")
	}
	if bullets[0].timestamp != "12:34" {
		t.Errorf("bullet[0].timestamp: got %q, want %q", bullets[0].timestamp, "12:34")
	}

	// Contract: decision without timestamp has plain text and empty timestamp field
	if bullets[1].text != "No timestamp decision" {
		t.Errorf("bullet[1].text: got %q, want %q", bullets[1].text, "No timestamp decision")
	}
	if bullets[1].timestamp != "" {
		t.Errorf("bullet[1].timestamp: got %q, want empty string", bullets[1].timestamp)
	}
}

// ---------- buildContentRequests tests (P0 — CRAP reduction for CreateDecisionsTab) ----------

func TestBuildContentRequests(t *testing.T) {
	transcript := &models.TranscriptContent{
		TabID: "tab-transcript",
		Headings: []models.TranscriptHeading{
			{HeadingID: "h.abc", Text: "12:30", Index: 0},
			{HeadingID: "h.def", Text: "13:00", Index: 100},
		},
	}

	tests := []struct {
		name       string
		content    []contentLine
		transcript *models.TranscriptContent
		tabID      string
		// Contract: expected request structure
		wantInsertText  bool   // should produce an InsertText request
		wantInsertTabID string // tabID on the InsertText.Location
		wantInsertIdx   int64  // start index for InsertText
		wantHeadingReqs int    // count of UpdateParagraphStyle requests
		wantBulletReqs  int    // count of CreateParagraphBullets requests
		wantLinkReqs    int    // count of UpdateTextStyle (link) requests
		wantTotalReqs   int    // total request count
	}{
		{
			name: "headings and bullets with timestamps produce correct request types",
			content: []contentLine{
				{text: "Decisions Made", isHeading: true},
				{text: "[12:34] Adopt pipeline", isBullet: true, timestamp: "12:34"},
				{text: "Decisions Deferred", isHeading: true},
				{text: "No decisions identified"},
				{text: "Open Items", isHeading: true},
				{text: "[13:05] Review API", isBullet: true, timestamp: "13:05"},
			},
			transcript:      transcript,
			tabID:           "tab-new",
			wantInsertText:  true,
			wantInsertTabID: "tab-new",
			wantInsertIdx:   1,
			wantHeadingReqs: 3,
			wantBulletReqs:  2,
			wantLinkReqs:    2,
			// 1 InsertText + 3 heading styles + 2 bullets + 2 links = 8
			wantTotalReqs: 8,
		},
		{
			name: "bullets without timestamps produce no link requests",
			content: []contentLine{
				{text: "Decisions Made", isHeading: true},
				{text: "Simple decision", isBullet: true, timestamp: ""},
			},
			transcript:      transcript,
			tabID:           "tab-x",
			wantInsertText:  true,
			wantInsertTabID: "tab-x",
			wantInsertIdx:   1,
			wantHeadingReqs: 1,
			wantBulletReqs:  1,
			wantLinkReqs:    0,
			wantTotalReqs:   3, // 1 insert + 1 heading + 1 bullet
		},
		{
			name: "nil transcript suppresses link requests even with timestamps",
			content: []contentLine{
				{text: "Decisions Made", isHeading: true},
				{text: "[12:34] Decision", isBullet: true, timestamp: "12:34"},
			},
			transcript:      nil,
			tabID:           "tab-y",
			wantInsertText:  true,
			wantHeadingReqs: 1,
			wantBulletReqs:  1,
			wantLinkReqs:    0,
			wantTotalReqs:   3,
		},
		{
			name: "transcript with empty headings suppresses link requests",
			content: []contentLine{
				{text: "[12:34] Decision", isBullet: true, timestamp: "12:34"},
			},
			transcript:     &models.TranscriptContent{TabID: "t", Headings: nil},
			tabID:          "tab-z",
			wantInsertText: true,
			wantBulletReqs: 1,
			wantLinkReqs:   0,
			wantTotalReqs:  2, // 1 insert + 1 bullet
		},
		{
			name:           "empty content produces no requests",
			content:        []contentLine{},
			transcript:     transcript,
			tabID:          "tab-empty",
			wantInsertText: false,
			wantTotalReqs:  0,
		},
		{
			name: "plain text lines produce only InsertText (no style requests)",
			content: []contentLine{
				{text: "No decisions identified"},
			},
			transcript:      transcript,
			tabID:           "tab-plain",
			wantInsertText:  true,
			wantHeadingReqs: 0,
			wantBulletReqs:  0,
			wantLinkReqs:    0,
			wantTotalReqs:   1, // just insert
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqs := buildContentRequests(tt.content, tt.transcript, tt.tabID)

			if len(reqs) != tt.wantTotalReqs {
				t.Fatalf("total requests: got %d, want %d", len(reqs), tt.wantTotalReqs)
			}

			var insertCount, headingCount, bulletCount, linkCount int
			for _, r := range reqs {
				switch {
				case r.InsertText != nil:
					insertCount++
					// Contract: InsertText always targets index 1 in the specified tab
					if r.InsertText.Location.Index != 1 {
						t.Errorf("InsertText.Location.Index: got %d, want 1", r.InsertText.Location.Index)
					}
					if r.InsertText.Location.TabId != tt.tabID {
						t.Errorf("InsertText.Location.TabId: got %q, want %q", r.InsertText.Location.TabId, tt.tabID)
					}
				case r.UpdateParagraphStyle != nil:
					headingCount++
					// Contract: heading style must be HEADING_2
					if r.UpdateParagraphStyle.ParagraphStyle.NamedStyleType != "HEADING_2" {
						t.Errorf("heading style: got %q, want HEADING_2", r.UpdateParagraphStyle.ParagraphStyle.NamedStyleType)
					}
					// Contract: range must use the correct tab ID
					if r.UpdateParagraphStyle.Range.TabId != tt.tabID {
						t.Errorf("heading range TabId: got %q, want %q", r.UpdateParagraphStyle.Range.TabId, tt.tabID)
					}
				case r.CreateParagraphBullets != nil:
					bulletCount++
					// Contract: bullet preset must be BULLET_DISC_CIRCLE_SQUARE
					if r.CreateParagraphBullets.BulletPreset != "BULLET_DISC_CIRCLE_SQUARE" {
						t.Errorf("bullet preset: got %q, want BULLET_DISC_CIRCLE_SQUARE", r.CreateParagraphBullets.BulletPreset)
					}
				case r.UpdateTextStyle != nil:
					linkCount++
					// Contract: link must reference the transcript tab
					if tt.transcript != nil && r.UpdateTextStyle.TextStyle.Link.Heading.TabId != tt.transcript.TabID {
						t.Errorf("link TabId: got %q, want %q", r.UpdateTextStyle.TextStyle.Link.Heading.TabId, tt.transcript.TabID)
					}
				}
			}

			if tt.wantInsertText && insertCount != 1 {
				t.Errorf("InsertText requests: got %d, want 1", insertCount)
			}
			if !tt.wantInsertText && insertCount != 0 {
				t.Errorf("InsertText requests: got %d, want 0", insertCount)
			}
			if headingCount != tt.wantHeadingReqs {
				t.Errorf("heading style requests: got %d, want %d", headingCount, tt.wantHeadingReqs)
			}
			if bulletCount != tt.wantBulletReqs {
				t.Errorf("bullet requests: got %d, want %d", bulletCount, tt.wantBulletReqs)
			}
			if linkCount != tt.wantLinkReqs {
				t.Errorf("link requests: got %d, want %d", linkCount, tt.wantLinkReqs)
			}
		})
	}
}

func TestBuildContentRequests_IndexArithmetic(t *testing.T) {
	// Verify the UTF-16 index ranges are computed correctly for known content.
	content := []contentLine{
		{text: "Decisions Made", isHeading: true},                     // 14 chars + 1 newline = 15 UTF-16 units
		{text: "[12:34] Ship it", isBullet: true, timestamp: "12:34"}, // 15 chars + 1 newline = 16 UTF-16 units
	}
	transcript := &models.TranscriptContent{
		TabID: "t1",
		Headings: []models.TranscriptHeading{
			{HeadingID: "h.1", Text: "12:30", Index: 0},
		},
	}

	reqs := buildContentRequests(content, transcript, "tab-new")

	// Expected request order: InsertText, UpdateParagraphStyle(heading), CreateParagraphBullets, UpdateTextStyle(link)
	if len(reqs) != 4 {
		t.Fatalf("expected 4 requests, got %d", len(reqs))
	}

	// Request 0: InsertText at index 1
	if reqs[0].InsertText == nil {
		t.Fatal("request[0] should be InsertText")
	}

	// Request 1: Heading style for "Decisions Made\n" — range [1, 16)
	headingReq := reqs[1].UpdateParagraphStyle
	if headingReq == nil {
		t.Fatal("request[1] should be UpdateParagraphStyle")
	}
	if headingReq.Range.StartIndex != 1 {
		t.Errorf("heading StartIndex: got %d, want 1", headingReq.Range.StartIndex)
	}
	if headingReq.Range.EndIndex != 16 {
		t.Errorf("heading EndIndex: got %d, want 16 (14 chars + 1 newline + offset 1)", headingReq.Range.EndIndex)
	}

	// Request 2: Bullet for "[12:34] Ship it\n" — range [16, 32)
	bulletReq := reqs[2].CreateParagraphBullets
	if bulletReq == nil {
		t.Fatal("request[2] should be CreateParagraphBullets")
	}
	if bulletReq.Range.StartIndex != 16 {
		t.Errorf("bullet StartIndex: got %d, want 16", bulletReq.Range.StartIndex)
	}
	if bulletReq.Range.EndIndex != 32 {
		t.Errorf("bullet EndIndex: got %d, want 32 (15 chars + 1 newline + offset 16)", bulletReq.Range.EndIndex)
	}

	// Request 3: Link for "[12:34]" — range [16, 23)
	linkReq := reqs[3].UpdateTextStyle
	if linkReq == nil {
		t.Fatal("request[3] should be UpdateTextStyle")
	}
	if linkReq.Range.StartIndex != 16 {
		t.Errorf("link StartIndex: got %d, want 16", linkReq.Range.StartIndex)
	}
	// "[12:34]" = 7 chars = 7 UTF-16 units, so endIdx = 16 + 7 = 23
	if linkReq.Range.EndIndex != 23 {
		t.Errorf("link EndIndex: got %d, want 23 (7 chars for '[12:34]')", linkReq.Range.EndIndex)
	}
	// Contract: link must point to the correct heading
	if linkReq.TextStyle.Link.Heading.Id != "h.1" {
		t.Errorf("link heading ID: got %q, want 'h.1'", linkReq.TextStyle.Link.Heading.Id)
	}
	if linkReq.TextStyle.Link.Heading.TabId != "t1" {
		t.Errorf("link heading TabId: got %q, want 't1'", linkReq.TextStyle.Link.Heading.TabId)
	}
}

func TestBuildContentRequests_SurrogatePairIndexing(t *testing.T) {
	// Verify UTF-16 index arithmetic handles surrogate pairs (emoji) correctly.
	// The grinning face emoji (U+1F600) requires 2 UTF-16 code units (a surrogate pair).
	content := []contentLine{
		{text: "Made \U0001F600", isHeading: true}, // "Made " (5) + U+1F600 (2 UTF-16 units) = 7 UTF-16 + 1 newline = 8
		{text: "Item", isBullet: true},             // "Item" (4) + 1 newline = 5
	}

	reqs := buildContentRequests(content, nil, "tab-emoji")

	// InsertText + heading style + bullet = 3 requests
	if len(reqs) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(reqs))
	}

	// Heading range: [1, 9) — 7 UTF-16 units for "Made 😀" + 1 newline = 8
	headingReq := reqs[1].UpdateParagraphStyle
	if headingReq == nil {
		t.Fatal("request[1] should be UpdateParagraphStyle")
	}
	if headingReq.Range.StartIndex != 1 {
		t.Errorf("heading StartIndex: got %d, want 1", headingReq.Range.StartIndex)
	}
	if headingReq.Range.EndIndex != 9 {
		t.Errorf("heading EndIndex: got %d, want 9 (7 UTF-16 units + 1 newline + offset 1)", headingReq.Range.EndIndex)
	}

	// Bullet range: [9, 14) — 4 UTF-16 units for "Item" + 1 newline = 5
	bulletReq := reqs[2].CreateParagraphBullets
	if bulletReq == nil {
		t.Fatal("request[2] should be CreateParagraphBullets")
	}
	if bulletReq.Range.StartIndex != 9 {
		t.Errorf("bullet StartIndex: got %d, want 9", bulletReq.Range.StartIndex)
	}
	if bulletReq.Range.EndIndex != 14 {
		t.Errorf("bullet EndIndex: got %d, want 14", bulletReq.Range.EndIndex)
	}
}

// ---------- extractItemsFromSection tests (P1 — direct coverage) ----------

// buildBulletElement creates a structural element with bullet paragraph text.
func buildBulletElement(text string, startIndex, endIndex int64) *docs.StructuralElement {
	return &docs.StructuralElement{
		StartIndex: startIndex,
		EndIndex:   endIndex,
		Paragraph: &docs.Paragraph{
			Elements: []*docs.ParagraphElement{
				{TextRun: &docs.TextRun{Content: text}},
			},
			ParagraphStyle: &docs.ParagraphStyle{},
			Bullet:         &docs.Bullet{},
		},
	}
}

// buildPlainElement creates a structural element with plain (non-bullet) paragraph text.
func buildPlainElement(text string, startIndex, endIndex int64) *docs.StructuralElement {
	return &docs.StructuralElement{
		StartIndex: startIndex,
		EndIndex:   endIndex,
		Paragraph: &docs.Paragraph{
			Elements: []*docs.ParagraphElement{
				{TextRun: &docs.TextRun{Content: text}},
			},
			ParagraphStyle: &docs.ParagraphStyle{},
		},
	}
}

func TestExtractItemsFromSection(t *testing.T) {
	svc := &Service{}

	tests := []struct {
		name      string
		content   []*docs.StructuralElement
		wantItems int
		// Contract: per-item field assertions
		wantTexts      []string
		wantProcessed  []bool
		wantStartIndex []int64
	}{
		{
			name: "extracts bullets from Suggested next steps section",
			content: []*docs.StructuralElement{
				buildPlainElement("Introduction\n", 0, 13),
				buildParagraphElement("Suggested next steps\n", 13, 34, "HEADING_2", ""),
				buildBulletElement("Jay will schedule meeting\n", 34, 60),
				buildBulletElement("Sarah will send email\n", 60, 82),
			},
			wantItems:      2,
			wantTexts:      []string{"Jay will schedule meeting", "Sarah will send email"},
			wantProcessed:  []bool{false, false},
			wantStartIndex: []int64{34, 60},
		},
		{
			name: "stops at section boundary (next heading)",
			content: []*docs.StructuralElement{
				buildParagraphElement("Suggested next steps\n", 0, 21, "HEADING_2", ""),
				buildBulletElement("Action item 1\n", 21, 35),
				buildParagraphElement("Other Notes\n", 35, 47, "HEADING_2", ""),
				buildBulletElement("Not an action item\n", 47, 66),
			},
			wantItems:      1,
			wantTexts:      []string{"Action item 1"},
			wantProcessed:  []bool{false},
			wantStartIndex: []int64{21},
		},
		{
			name: "stops at H1 boundary",
			content: []*docs.StructuralElement{
				buildParagraphElement("Suggested next steps\n", 0, 21, "HEADING_3", ""),
				buildBulletElement("Task A\n", 21, 28),
				buildParagraphElement("Big Section\n", 28, 40, "HEADING_1", ""),
				buildBulletElement("Not a task\n", 40, 51),
			},
			wantItems: 1,
			wantTexts: []string{"Task A"},
		},
		{
			name: "detects processed emoji",
			content: []*docs.StructuralElement{
				buildParagraphElement("Suggested next steps\n", 0, 21, "HEADING_2", ""),
				buildBulletElement("🆔 Already processed\n", 21, 44),
				buildBulletElement("Not processed yet\n", 44, 62),
			},
			wantItems:     2,
			wantTexts:     []string{"🆔 Already processed", "Not processed yet"},
			wantProcessed: []bool{true, false},
		},
		{
			name: "skips non-bullet paragraphs within section",
			content: []*docs.StructuralElement{
				buildParagraphElement("Suggested next steps\n", 0, 21, "HEADING_2", ""),
				buildPlainElement("Some explanation text\n", 21, 43),
				buildBulletElement("Real task\n", 43, 53),
			},
			wantItems: 1,
			wantTexts: []string{"Real task"},
		},
		{
			name: "returns empty when section heading not found",
			content: []*docs.StructuralElement{
				buildParagraphElement("Meeting Notes\n", 0, 14, "HEADING_1", ""),
				buildBulletElement("Some item\n", 14, 24),
			},
			wantItems: 0,
		},
		{
			name:      "returns empty for nil content",
			content:   nil,
			wantItems: 0,
		},
		{
			name: "skips empty/whitespace bullet text",
			content: []*docs.StructuralElement{
				buildParagraphElement("Suggested next steps\n", 0, 21, "HEADING_2", ""),
				buildBulletElement("  \n", 21, 24),
				buildBulletElement("Valid item\n", 24, 35),
			},
			wantItems: 1,
			wantTexts: []string{"Valid item"},
		},
		{
			name: "case-insensitive heading match",
			content: []*docs.StructuralElement{
				buildParagraphElement("SUGGESTED NEXT STEPS\n", 0, 21, "HEADING_2", ""),
				buildBulletElement("Found it\n", 21, 30),
			},
			wantItems: 1,
			wantTexts: []string{"Found it"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := svc.extractItemsFromSection(tt.content)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(items) != tt.wantItems {
				t.Fatalf("item count: got %d, want %d", len(items), tt.wantItems)
			}

			for i, wantText := range tt.wantTexts {
				if i >= len(items) {
					break
				}
				if items[i].Text != wantText {
					t.Errorf("item[%d].Text: got %q, want %q", i, items[i].Text, wantText)
				}
			}

			for i, wantProcessed := range tt.wantProcessed {
				if i >= len(items) {
					break
				}
				if items[i].IsProcessed != wantProcessed {
					t.Errorf("item[%d].IsProcessed: got %v, want %v", i, items[i].IsProcessed, wantProcessed)
				}
			}

			for i, wantIdx := range tt.wantStartIndex {
				if i >= len(items) {
					break
				}
				if items[i].StartIndex != wantIdx {
					t.Errorf("item[%d].StartIndex: got %d, want %d", i, items[i].StartIndex, wantIdx)
				}
			}
		})
	}
}

// ---------- utf16Len contract tests ----------

func TestUtf16Len(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want int64
	}{
		{"empty string", "", 0},
		{"ASCII only", "hello", 5},
		{"ASCII with newline", "hello\n", 6},
		{"BMP character (emoji)", "\u2714", 1},                          // check mark
		{"supplementary plane emoji (surrogate pair)", "\U0001F600", 2}, // grinning face
		{"mixed ASCII and emoji", "ok \U0001F44D", 5},                   // "ok " + thumbs up (surrogate pair)
		{"multiple surrogate pairs", "\U0001F600\U0001F600", 4},
		{"Decisions tab typical content", "Decisions Made\n", 15},
		{"bracket timestamp", "[12:34] Decision text\n", 22},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := utf16Len(tt.s)
			if got != tt.want {
				t.Errorf("utf16Len(%q): got %d, want %d", tt.s, got, tt.want)
			}
		})
	}
}

// ---------- P0: CreateDecisionsTab orchestration tests ----------

// newTestService creates a Service backed by a test HTTP server.
// The handler receives all BatchUpdate calls and can return different
// responses per invocation via the callCount pointer.
func newTestService(t *testing.T, handler http.HandlerFunc) (*Service, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(handler)
	docsSvc, err := docs.NewService(context.Background(),
		option.WithHTTPClient(ts.Client()),
		option.WithEndpoint(ts.URL),
	)
	if err != nil {
		t.Fatalf("failed to create docs service: %v", err)
	}
	return &Service{client: docsSvc}, ts
}

func TestCreateDecisionsTab_HappyPath(t *testing.T) {
	// Path 1: Tab created successfully, content inserted successfully.
	callCount := 0
	svc, ts := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			// First BatchUpdate: AddDocumentTab — return a tab ID
			json.NewEncoder(w).Encode(map[string]interface{}{
				"replies": []map[string]interface{}{
					{
						"addDocumentTab": map[string]interface{}{
							"tabProperties": map[string]interface{}{
								"tabId": "new-tab-123",
								"title": "Decisions",
							},
						},
					},
				},
			})
		} else {
			// Second BatchUpdate: InsertText + styles — success
			json.NewEncoder(w).Encode(map[string]interface{}{
				"replies": []map[string]interface{}{},
			})
		}
	})
	defer ts.Close()

	decisions := []models.Decision{
		{Category: "made", Text: "Adopt new pipeline", Timestamp: "12:34"},
		{Category: "deferred", Text: "Budget review", Timestamp: "13:00"},
	}
	transcript := &models.TranscriptContent{
		TabID:    "tab-transcript",
		FullText: "12:30\nHello\n12:45\nDiscussion",
		Headings: []models.TranscriptHeading{
			{HeadingID: "h.abc", Text: "12:30", Index: 0},
		},
	}

	err := svc.CreateDecisionsTab(context.Background(), "doc-123", decisions, transcript)

	// Contract: no error returned
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Contract: exactly 2 BatchUpdate calls (create tab + insert content)
	if callCount != 2 {
		t.Errorf("expected 2 BatchUpdate calls, got %d", callCount)
	}
}

func TestCreateDecisionsTab_DuplicateTab_GoogleAPIError(t *testing.T) {
	// Path 2a: First BatchUpdate returns a 409 googleapi.Error — sentinel error.
	svc, ts := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict) // 409
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"code":    409,
				"message": "Tab already exists",
				"status":  "ALREADY_EXISTS",
			},
		})
	})
	defer ts.Close()

	decisions := []models.Decision{{Category: "made", Text: "Ship it"}}

	err := svc.CreateDecisionsTab(context.Background(), "doc-dup", decisions, nil)

	// Contract: must return ErrDecisionsTabExists sentinel
	if err != ErrDecisionsTabExists {
		t.Errorf("expected ErrDecisionsTabExists, got %v", err)
	}
}

func TestCreateDecisionsTab_DuplicateTab_GoogleAPIStringMatch(t *testing.T) {
	// Path 2b: First BatchUpdate returns a googleapi.Error with code != 409
	// but message contains "already exists" — should still detect duplicate.
	svc, ts := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest) // 400, not 409
		w.Write([]byte(`{"error": {"code": 400, "message": "Tab already exists in document"}}`))
	})
	defer ts.Close()

	decisions := []models.Decision{{Category: "made", Text: "Ship it"}}

	err := svc.CreateDecisionsTab(context.Background(), "doc-dup2", decisions, nil)

	// Contract: "already exists" in googleapi.Error.Message triggers sentinel
	if err != ErrDecisionsTabExists {
		t.Errorf("expected ErrDecisionsTabExists, got %v", err)
	}
}

func TestCreateDecisionsTab_DuplicateTab_NonAPIErrorStringMatch(t *testing.T) {
	// Path 2c: Server closes connection producing a non-googleapi error.
	// We simulate this by having the handler close the connection after
	// writing partial/malformed data.
	svc, ts := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		// Hijack the connection to simulate a network-level error that
		// produces a non-googleapi error containing "already exists".
		// However, it's difficult to inject arbitrary error text this way.
		// Instead, we test the string-match path indirectly through the
		// "duplicate" keyword.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": {"code": 400, "message": "duplicate tab title not allowed"}}`))
	})
	defer ts.Close()

	decisions := []models.Decision{{Category: "made", Text: "Ship it"}}

	err := svc.CreateDecisionsTab(context.Background(), "doc-dup3", decisions, nil)

	// Contract: "duplicate" in error message triggers sentinel
	if err != ErrDecisionsTabExists {
		t.Errorf("expected ErrDecisionsTabExists, got %v", err)
	}
}

func TestCreateDecisionsTab_ContentInsertionFails_CleanupSucceeds(t *testing.T) {
	// Path 3: Tab created, content insertion fails, cleanup (delete tab) succeeds.
	callCount := 0
	svc, ts := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		switch callCount {
		case 1:
			// First BatchUpdate: AddDocumentTab — success
			json.NewEncoder(w).Encode(map[string]interface{}{
				"replies": []map[string]interface{}{
					{
						"addDocumentTab": map[string]interface{}{
							"tabProperties": map[string]interface{}{
								"tabId": "new-tab-456",
								"title": "Decisions",
							},
						},
					},
				},
			})
		case 2:
			// Second BatchUpdate: InsertText — 400 failure (non-retryable)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"code":    400,
					"message": "Invalid request: index out of bounds",
					"status":  "INVALID_ARGUMENT",
				},
			})
		case 3:
			// Third BatchUpdate: DeleteTab (cleanup) — success
			json.NewEncoder(w).Encode(map[string]interface{}{
				"replies": []map[string]interface{}{},
			})
		}
	})
	defer ts.Close()

	decisions := []models.Decision{
		{Category: "made", Text: "Adopt pipeline", Timestamp: "12:34"},
	}

	err := svc.CreateDecisionsTab(context.Background(), "doc-fail", decisions, nil)

	// Contract: error returned mentioning insertion failure
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Contract: error message indicates cleanup succeeded ("empty tab cleaned up")
	if !strings.Contains(err.Error(), "empty tab cleaned up") {
		t.Errorf("error should mention cleanup success, got: %v", err)
	}
	// Contract: error wraps the original content insertion error
	if !strings.Contains(err.Error(), "failed to insert content") {
		t.Errorf("error should mention insertion failure, got: %v", err)
	}
	// Contract: exactly 3 calls — create, insert (fail), cleanup (success)
	if callCount != 3 {
		t.Errorf("expected 3 BatchUpdate calls, got %d", callCount)
	}
}

func TestCreateDecisionsTab_ContentInsertionFails_CleanupFails(t *testing.T) {
	// Path 4: Tab created, content insertion fails, cleanup also fails.
	callCount := 0
	svc, ts := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")

		switch callCount {
		case 1:
			// First BatchUpdate: AddDocumentTab — success
			json.NewEncoder(w).Encode(map[string]interface{}{
				"replies": []map[string]interface{}{
					{
						"addDocumentTab": map[string]interface{}{
							"tabProperties": map[string]interface{}{
								"tabId": "new-tab-789",
								"title": "Decisions",
							},
						},
					},
				},
			})
		default:
			// All subsequent calls fail with 400 (non-retryable)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"code":    400,
					"message": "Invalid request",
					"status":  "INVALID_ARGUMENT",
				},
			})
		}
	})
	defer ts.Close()

	decisions := []models.Decision{
		{Category: "open", Text: "API migration"},
	}

	err := svc.CreateDecisionsTab(context.Background(), "doc-double-fail", decisions, nil)

	// Contract: error returned
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Contract: error mentions both failures
	if !strings.Contains(err.Error(), "failed to insert content") {
		t.Errorf("error should mention insertion failure, got: %v", err)
	}
	if !strings.Contains(err.Error(), "cleanup of empty tab also failed") {
		t.Errorf("error should mention cleanup failure, got: %v", err)
	}
}

func TestCreateDecisionsTab_NoTabIDReturned(t *testing.T) {
	// Edge case: BatchUpdate succeeds but returns no TabId.
	svc, ts := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"replies": []map[string]interface{}{},
		})
	})
	defer ts.Close()

	decisions := []models.Decision{{Category: "made", Text: "Ship it"}}

	err := svc.CreateDecisionsTab(context.Background(), "doc-no-tab", decisions, nil)

	// Contract: error about missing TabId
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no TabId returned") {
		t.Errorf("error should mention missing TabId, got: %v", err)
	}
}

func TestCreateDecisionsTab_EmptyDecisions(t *testing.T) {
	// Edge case: Empty decisions list — should still create the tab with placeholder content.
	callCount := 0
	var capturedBodies []string
	svc, ts := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++

		// Capture request body for the content insertion call
		if callCount == 2 {
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			if bodyBytes, err := json.Marshal(body); err == nil {
				capturedBodies = append(capturedBodies, string(bodyBytes))
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"replies": []map[string]interface{}{
					{
						"addDocumentTab": map[string]interface{}{
							"tabProperties": map[string]interface{}{
								"tabId": "tab-empty-dec",
								"title": "Decisions",
							},
						},
					},
				},
			})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"replies": []map[string]interface{}{},
			})
		}
	})
	defer ts.Close()

	err := svc.CreateDecisionsTab(context.Background(), "doc-empty", []models.Decision{}, nil)

	// Contract: no error — empty decisions still produces the tab
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Contract: 2 BatchUpdate calls (create tab + insert placeholder content)
	if callCount != 2 {
		t.Errorf("expected 2 BatchUpdate calls, got %d", callCount)
	}
}

func TestCreateDecisionsTab_RequestBodyValidation(t *testing.T) {
	// Validates that the first BatchUpdate contains an AddDocumentTab request
	// and the second contains InsertText + style requests.
	callCount := 0
	svc, ts := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++

		var body struct {
			Requests []json.RawMessage `json:"requests"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		w.Header().Set("Content-Type", "application/json")

		if callCount == 1 {
			// Contract: first call must have exactly 1 request (AddDocumentTab)
			if len(body.Requests) != 1 {
				t.Errorf("first BatchUpdate: expected 1 request, got %d", len(body.Requests))
			}
			// Verify it's an addDocumentTab request
			var req map[string]interface{}
			json.Unmarshal(body.Requests[0], &req)
			if _, ok := req["addDocumentTab"]; !ok {
				t.Error("first request should be addDocumentTab")
			}

			json.NewEncoder(w).Encode(map[string]interface{}{
				"replies": []map[string]interface{}{
					{
						"addDocumentTab": map[string]interface{}{
							"tabProperties": map[string]interface{}{
								"tabId": "tab-validated",
								"title": "Decisions",
							},
						},
					},
				},
			})
		} else {
			// Contract: second call must contain InsertText as first request
			if len(body.Requests) > 0 {
				var firstReq map[string]interface{}
				json.Unmarshal(body.Requests[0], &firstReq)
				if _, ok := firstReq["insertText"]; !ok {
					t.Error("second BatchUpdate's first request should be insertText")
				}
			}

			json.NewEncoder(w).Encode(map[string]interface{}{
				"replies": []map[string]interface{}{},
			})
		}
	})
	defer ts.Close()

	decisions := []models.Decision{
		{Category: "made", Text: "Ship it", Timestamp: "12:00"},
	}

	err := svc.CreateDecisionsTab(context.Background(), "doc-validate", decisions, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
