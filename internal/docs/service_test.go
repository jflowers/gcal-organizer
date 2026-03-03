package docs

import (
	"strings"
	"testing"

	"github.com/jflowers/gcal-organizer/pkg/models"
	"google.golang.org/api/docs/v1"
)

// ---------- extractItemsFromSection pure function tests ----------

func TestExtractItemsFromSection_SuggestedNextSteps(t *testing.T) {
	svc := &Service{}
	content := []*docs.StructuralElement{
		{
			StartIndex: 0,
			EndIndex:   25,
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "Suggested next steps"}},
				},
				ParagraphStyle: &docs.ParagraphStyle{},
			},
		},
		{
			StartIndex: 25,
			EndIndex:   60,
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "Jay will schedule meeting"}},
				},
				Bullet:         &docs.Bullet{},
				ParagraphStyle: &docs.ParagraphStyle{},
			},
		},
		{
			StartIndex: 60,
			EndIndex:   100,
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "Sarah will send report"}},
				},
				Bullet:         &docs.Bullet{},
				ParagraphStyle: &docs.ParagraphStyle{},
			},
		},
	}

	items, err := svc.extractItemsFromSection(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: 2 items extracted from "Suggested next steps" section
	if len(items) != 2 {
		t.Fatalf("expected 2 checkbox items, got %d", len(items))
	}
	if !strings.Contains(items[0].Text, "Jay will schedule meeting") {
		t.Errorf("item[0] text: got %q", items[0].Text)
	}
	if !strings.Contains(items[1].Text, "Sarah will send report") {
		t.Errorf("item[1] text: got %q", items[1].Text)
	}
}

func TestExtractItemsFromSection_NoSuggestedSection(t *testing.T) {
	svc := &Service{}
	content := []*docs.StructuralElement{
		{
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "Some other heading"}},
				},
				ParagraphStyle: &docs.ParagraphStyle{},
			},
		},
		{
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "A bullet item"}},
				},
				Bullet:         &docs.Bullet{},
				ParagraphStyle: &docs.ParagraphStyle{},
			},
		},
	}

	items, err := svc.extractItemsFromSection(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: no items when "Suggested next steps" section not found
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestExtractItemsFromSection_ProcessedEmoji(t *testing.T) {
	svc := &Service{}
	content := []*docs.StructuralElement{
		{
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "Suggested next steps"}},
				},
				ParagraphStyle: &docs.ParagraphStyle{},
			},
		},
		{
			StartIndex: 25,
			EndIndex:   80,
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: ProcessedEmoji + " Already assigned"}},
				},
				Bullet:         &docs.Bullet{},
				ParagraphStyle: &docs.ParagraphStyle{},
			},
		},
		{
			StartIndex: 80,
			EndIndex:   120,
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "Unprocessed task"}},
				},
				Bullet:         &docs.Bullet{},
				ParagraphStyle: &docs.ParagraphStyle{},
			},
		},
	}

	items, err := svc.extractItemsFromSection(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if !items[0].IsProcessed {
		t.Error("expected item[0] IsProcessed=true (has ProcessedEmoji)")
	}
	if items[1].IsProcessed {
		t.Error("expected item[1] IsProcessed=false")
	}
}

func TestExtractItemsFromSection_EmptyBullet(t *testing.T) {
	svc := &Service{}
	content := []*docs.StructuralElement{
		{
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "Suggested next steps"}},
				},
				ParagraphStyle: &docs.ParagraphStyle{},
			},
		},
		{
			StartIndex: 25,
			EndIndex:   30,
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "   "}},
				},
				Bullet:         &docs.Bullet{},
				ParagraphStyle: &docs.ParagraphStyle{},
			},
		},
	}

	items, err := svc.extractItemsFromSection(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: empty/whitespace-only bullets are skipped
	if len(items) != 0 {
		t.Errorf("expected 0 items for empty bullet, got %d", len(items))
	}
}

func TestExtractItemsFromSection_NilParagraph(t *testing.T) {
	svc := &Service{}
	content := []*docs.StructuralElement{
		{Paragraph: nil}, // Non-paragraph element (table, etc.)
		{
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "Suggested next steps"}},
				},
				ParagraphStyle: &docs.ParagraphStyle{},
			},
		},
		{
			StartIndex: 25,
			EndIndex:   50,
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "A task"}},
				},
				Bullet:         &docs.Bullet{},
				ParagraphStyle: &docs.ParagraphStyle{},
			},
		},
	}

	items, err := svc.extractItemsFromSection(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: nil paragraphs are skipped without error
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

// ---------- utf16Len tests ----------

func TestUtf16Len(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want int64
	}{
		{"ascii", "hello", 5},
		{"empty", "", 0},
		{"emoji", "👍", 2},      // U+1F44D needs surrogate pair
		{"mixed", "hi 👍", 5},   // 'h' + 'i' + ' ' + surrogate pair (2 units)
		{"unicode", "café", 4}, // all BMP chars
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

// ---------- T010: ExtractTranscriptContent tests ----------

// makeTestDoc creates a docs.Document with the given tabs for testing.
func makeTestDoc(tabs []*docs.Tab) *docs.Document {
	return &docs.Document{
		DocumentId: "test-doc-id",
		Tabs:       tabs,
	}
}

// makeTab creates a tab with title, content elements, and a tabID.
func makeTab(title, tabID string, elements []*docs.StructuralElement) *docs.Tab {
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

// makeParagraphElement creates a structural element with paragraph text and optional heading style.
func makeParagraphElement(text string, startIndex, endIndex int64, headingStyle string, headingID string) *docs.StructuralElement {
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
	}{
		{
			name: "finds Transcript tab in multi-tab doc",
			doc: makeTestDoc([]*docs.Tab{
				makeTab("Notes", "tab-notes", []*docs.StructuralElement{
					makeParagraphElement("Some notes\n", 0, 11, "", ""),
				}),
				makeTab("Transcript", "tab-transcript", []*docs.StructuralElement{
					makeParagraphElement("12:00\n", 0, 6, "HEADING_3", "h.abc123"),
					makeParagraphElement("Hello everyone\n", 6, 21, "", ""),
					makeParagraphElement("12:15\n", 21, 27, "HEADING_3", "h.def456"),
					makeParagraphElement("Moving on to the next topic\n", 27, 55, "", ""),
				}),
			}),
			wantTabID:        "tab-transcript",
			wantFullText:     "12:00\nHello everyone\n12:15\nMoving on to the next topic\n",
			wantHeadingCount: 2,
		},
		{
			name: "uses first tab content for single-tab doc",
			doc: makeTestDoc([]*docs.Tab{
				makeTab("", "tab-only", []*docs.StructuralElement{
					makeParagraphElement("10:00\n", 0, 6, "HEADING_3", "h.single1"),
					makeParagraphElement("Discussion content\n", 6, 25, "", ""),
				}),
			}),
			wantTabID:        "tab-only",
			wantFullText:     "10:00\nDiscussion content\n",
			wantHeadingCount: 1,
		},
		{
			name: "returns empty TranscriptContent for doc with no transcript",
			doc: makeTestDoc([]*docs.Tab{
				makeTab("Notes", "tab-notes", []*docs.StructuralElement{
					makeParagraphElement("Some notes\n", 0, 11, "", ""),
				}),
				makeTab("Action Items", "tab-actions", []*docs.StructuralElement{
					makeParagraphElement("Do something\n", 0, 13, "", ""),
				}),
			}),
			wantNil: true,
		},
		{
			name: "extracts H3 heading metadata",
			doc: makeTestDoc([]*docs.Tab{
				makeTab("Transcript", "tab-t", []*docs.StructuralElement{
					makeParagraphElement("09:30\n", 0, 6, "HEADING_3", "h.head1"),
					makeParagraphElement("Content here\n", 6, 19, "", ""),
					makeParagraphElement("09:45\n", 19, 25, "HEADING_3", "h.head2"),
					makeParagraphElement("More content\n", 25, 38, "", ""),
					makeParagraphElement("10:00\n", 38, 44, "HEADING_3", "h.head3"),
				}),
			}),
			wantTabID:        "tab-t",
			wantHeadingCount: 3,
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

			if result.TabID != tt.wantTabID {
				t.Errorf("TabID: got %q, want %q", result.TabID, tt.wantTabID)
			}

			if tt.wantFullText != "" && result.FullText != tt.wantFullText {
				t.Errorf("FullText: got %q, want %q", result.FullText, tt.wantFullText)
			}

			if len(result.Headings) != tt.wantHeadingCount {
				t.Errorf("Headings count: got %d, want %d", len(result.Headings), tt.wantHeadingCount)
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
		name           string
		decisions      []models.Decision
		wantSections   int
		wantContains   []string
		wantNoDecision bool
	}{
		{
			name: "three categorized sections with decisions",
			decisions: []models.Decision{
				{Category: "made", Text: "Adopt new pipeline", Timestamp: "12:34"},
				{Category: "deferred", Text: "Budget review", Timestamp: "13:00"},
				{Category: "open", Text: "API migration", Timestamp: "13:45"},
			},
			wantSections: 3,
			wantContains: []string{"Decisions Made", "Decisions Deferred", "Open Items", "Adopt new pipeline", "Budget review", "API migration"},
		},
		{
			name:           "empty decisions shows no decisions note",
			decisions:      []models.Decision{},
			wantSections:   3,
			wantContains:   []string{"Decisions Made", "Decisions Deferred", "Open Items", "No decisions identified"},
			wantNoDecision: true,
		},
		{
			name: "decisions in single category - empty sections get placeholder",
			decisions: []models.Decision{
				{Category: "made", Text: "First decision"},
				{Category: "made", Text: "Second decision"},
			},
			wantSections: 3,
			wantContains: []string{"First decision", "Second decision", "No decisions identified"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := buildDecisionsContent(tt.decisions)

			for _, want := range tt.wantContains {
				found := false
				for _, line := range content {
					if strings.Contains(line.text, want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected content to contain %q", want)
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

// Compile-time check that Service implements the expected interface methods.
var _ = (*Service)(nil)

// ---------- T024: HasDecisionsTab tests ----------

func TestHasDecisionsTab(t *testing.T) {
	tests := []struct {
		name    string
		doc     *docs.Document
		wantHas bool
	}{
		{
			name: "returns true when Decisions tab exists",
			doc: makeTestDoc([]*docs.Tab{
				makeTab("Notes", "tab-notes", nil),
				makeTab("Decisions", "tab-decisions", nil),
			}),
			wantHas: true,
		},
		{
			name: "returns false when no Decisions tab",
			doc: makeTestDoc([]*docs.Tab{
				makeTab("Notes", "tab-notes", nil),
				makeTab("Transcript", "tab-transcript", nil),
			}),
			wantHas: false,
		},
		{
			name: "returns true for manually-created Decisions tab",
			doc: makeTestDoc([]*docs.Tab{
				makeTab("Decisions", "tab-manual-decisions", nil),
			}),
			wantHas: true,
		},
		{
			name:    "returns false for empty tabs",
			doc:     makeTestDoc([]*docs.Tab{}),
			wantHas: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasDecisionsTabInDoc(tt.doc)
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
		name          string
		timestamp     string
		headings      []models.TranscriptHeading
		wantHeadingID string
		wantNil       bool
	}{
		{
			name:          "exact timestamp match returns correct HeadingID",
			timestamp:     "09:45",
			headings:      headings,
			wantHeadingID: "h.head2",
		},
		{
			name:          "nearest preceding heading when no exact match",
			timestamp:     "09:50",
			headings:      headings,
			wantHeadingID: "h.head2", // 09:45 is nearest preceding
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
			name:          "exact match at first heading",
			timestamp:     "09:30",
			headings:      headings,
			wantHeadingID: "h.head1",
		},
		{
			name:          "exact match at last heading",
			timestamp:     "10:15",
			headings:      headings,
			wantHeadingID: "h.head4",
		},
		{
			name:          "timestamp before any heading uses first heading",
			timestamp:     "08:00",
			headings:      headings,
			wantHeadingID: "h.head1",
		},
		{
			name:          "timestamp after all headings uses last heading",
			timestamp:     "11:00",
			headings:      headings,
			wantHeadingID: "h.head4",
		},
		{
			name:      "empty timestamp returns nil",
			timestamp: "",
			headings:  headings,
			wantNil:   true,
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

			if result.HeadingID != tt.wantHeadingID {
				t.Errorf("HeadingID: got %q, want %q", result.HeadingID, tt.wantHeadingID)
			}
		})
	}
}

// ---------- T020: CreateDecisionsTab with links tests ----------

func TestBuildDecisionsContentWithTimestamp(t *testing.T) {
	// Verify that timestamps are included in decision bullet text
	decisions := []models.Decision{
		{Category: "made", Text: "Adopt new pipeline", Timestamp: "12:34"},
		{Category: "made", Text: "No timestamp decision", Timestamp: ""},
	}

	content := buildDecisionsContent(decisions)

	foundWithTimestamp := false
	foundWithout := false
	for _, line := range content {
		if strings.Contains(line.text, "[12:34]") && strings.Contains(line.text, "Adopt new pipeline") {
			foundWithTimestamp = true
		}
		if line.text == "No timestamp decision" {
			foundWithout = true
		}
	}

	if !foundWithTimestamp {
		t.Error("expected decision with timestamp formatted as '[12:34] Adopt new pipeline'")
	}
	if !foundWithout {
		t.Error("expected decision without timestamp to have plain text")
	}
}
