package export

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/jflowers/gcal-organizer/pkg/models"
)

// testLogger returns a logger suitable for tests (writes to discard).
func testLogger() *log.Logger {
	return log.NewWithOptions(os.Stderr, log.Options{})
}

// ---------- T022: renderMarkdown tests ----------

func TestRenderMarkdown(t *testing.T) {
	tests := []struct {
		name      string
		decisions []models.Decision
		topic     string
		date      time.Time
		attendees []string
		docID     string
		wantParts []string // substrings that must appear
		wantNot   []string // substrings that must NOT appear
	}{
		{
			name: "all three categories populated with time and source",
			decisions: []models.Decision{
				{Category: "made", Text: "Adopt GitHub Actions for CI/CD"},
				{Category: "deferred", Text: "Budget review postponed"},
				{Category: "open", Text: "API migration needs benchmarks"},
			},
			topic:     "Weekly Engineering Sync",
			date:      time.Date(2026, 4, 18, 14, 30, 0, 0, time.UTC),
			attendees: []string{"alice@example.com", "bob@example.com"},
			docID:     "doc-abc123",
			wantParts: []string{
				"topic: Weekly Engineering Sync",
				"date: \"2026-04-18\"",
				`time: "14:30"`,
				"attendees:",
				"  - alice@example.com",
				"  - bob@example.com",
				"source: https://docs.google.com/document/d/doc-abc123/edit",
				"## Decisions Made",
				"- Adopt GitHub Actions for CI/CD",
				"## Decisions Deferred",
				"- Budget review postponed",
				"## Open Items",
				"- API migration needs benchmarks",
			},
		},
		{
			name: "single category only - made",
			decisions: []models.Decision{
				{Category: "made", Text: "Use Go 1.24"},
			},
			topic: "Tech Review",
			date:  time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC),
			docID: "doc-xyz",
			wantParts: []string{
				"## Decisions Made",
				"- Use Go 1.24",
				`time: "09:00"`,
			},
			wantNot: []string{
				"## Decisions Deferred",
				"## Open Items",
				"attendees:",
			},
		},
		{
			name: "empty categories omitted (FR-015)",
			decisions: []models.Decision{
				{Category: "open", Text: "Need to decide on database"},
			},
			topic: "Architecture Review",
			date:  time.Date(2026, 5, 1, 16, 45, 0, 0, time.UTC),
			docID: "doc-456",
			wantParts: []string{
				"## Open Items",
				"- Need to decide on database",
				`time: "16:45"`,
			},
			wantNot: []string{
				"## Decisions Made",
				"## Decisions Deferred",
			},
		},
		{
			name: "decisions with special characters",
			decisions: []models.Decision{
				{Category: "made", Text: "Use `fmt.Errorf` with %w for error wrapping"},
				{Category: "open", Text: "Should we use JSON or YAML? (TBD)"},
			},
			topic: "Code Review: API v2",
			date:  time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC),
			docID: "doc-special",
			wantParts: []string{
				"- Use `fmt.Errorf` with %w for error wrapping",
				"- Should we use JSON or YAML? (TBD)",
			},
		},
		{
			name: "topic with Transcript suffix is cleaned in frontmatter",
			decisions: []models.Decision{
				{Category: "made", Text: "Ship it"},
			},
			topic: "Weekly Sync - Transcript",
			date:  time.Date(2026, 4, 20, 11, 0, 0, 0, time.UTC),
			docID: "doc-transcript",
			wantParts: []string{
				"topic: Weekly Sync",
			},
			wantNot: []string{
				"topic: Weekly Sync - Transcript",
			},
		},
		{
			name: "topic with Notes by Gemini prefix is cleaned in frontmatter",
			decisions: []models.Decision{
				{Category: "made", Text: "Approved"},
			},
			topic: "Notes by Gemini - Project Alpha",
			date:  time.Date(2026, 4, 20, 13, 15, 0, 0, time.UTC),
			docID: "doc-gemini",
			wantParts: []string{
				"topic: Project Alpha",
			},
			wantNot: []string{
				"topic: Notes by Gemini",
			},
		},
		{
			name: "no docID omits source field",
			decisions: []models.Decision{
				{Category: "made", Text: "Approved"},
			},
			topic: "Quick Sync",
			date:  time.Date(2026, 4, 20, 8, 0, 0, 0, time.UTC),
			docID: "",
			wantNot: []string{
				"source:",
			},
		},
		{
			name: "source URL uses correct doc ID (T24)",
			decisions: []models.Decision{
				{Category: "made", Text: "OK"},
			},
			topic: "Test Meeting",
			date:  time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC),
			docID: "1a2b3c4d5e",
			wantParts: []string{
				"source: https://docs.google.com/document/d/1a2b3c4d5e/edit",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(renderMarkdown(tt.decisions, tt.topic, tt.date, tt.attendees, tt.docID))

			for _, want := range tt.wantParts {
				if !strings.Contains(got, want) {
					t.Errorf("output missing expected substring %q\n\nGot:\n%s", want, got)
				}
			}

			for _, notWant := range tt.wantNot {
				if strings.Contains(got, notWant) {
					t.Errorf("output should NOT contain %q\n\nGot:\n%s", notWant, got)
				}
			}
		})
	}
}

// ---------- T023: Exporter.Export tests ----------

func TestExporterExport(t *testing.T) {
	baseDate := time.Date(2026, 4, 18, 14, 0, 0, 0, time.UTC)
	baseMeta := models.DecisionDocContext{
		DocID:      "doc-123",
		Source:     "notes-by-gemini",
		EventTitle: "Weekly Sync",
		EventDate:  baseDate,
		Attendees:  []string{"alice@example.com"},
	}
	baseDecisions := []models.Decision{
		{Category: "made", Text: "Adopt new pipeline"},
	}

	tests := []struct {
		name           string
		decisions      []models.Decision
		meta           models.DecisionDocContext
		dryRun         bool
		writeFileErr   error
		mkdirAllErr    error
		wantErr        bool
		wantWriteCalls int
		wantMkdirCalls int
	}{
		{
			name:           "successful write",
			decisions:      baseDecisions,
			meta:           baseMeta,
			wantWriteCalls: 1,
			wantMkdirCalls: 1,
		},
		{
			name:           "directory creation",
			decisions:      baseDecisions,
			meta:           baseMeta,
			wantMkdirCalls: 1,
			wantWriteCalls: 1,
		},
		{
			name:           "overwrite existing file (FR-008)",
			decisions:      baseDecisions,
			meta:           baseMeta,
			wantWriteCalls: 1,
			wantMkdirCalls: 1,
		},
		{
			name:           "write failure logs warning (FR-012)",
			decisions:      baseDecisions,
			meta:           baseMeta,
			writeFileErr:   fmt.Errorf("permission denied"),
			wantErr:        true,
			wantWriteCalls: 1,
			wantMkdirCalls: 1,
		},
		{
			name:           "mkdir failure logs warning (FR-012)",
			decisions:      baseDecisions,
			meta:           baseMeta,
			mkdirAllErr:    fmt.Errorf("read-only filesystem"),
			wantErr:        true,
			wantWriteCalls: 0,
			wantMkdirCalls: 1,
		},
		{
			name:           "dry-run suppresses write (FR-011)",
			decisions:      baseDecisions,
			meta:           baseMeta,
			dryRun:         true,
			wantWriteCalls: 0,
			wantMkdirCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeCalls := 0
			mkdirCalls := 0
			var lastWrittenPath string
			var lastWrittenData []byte

			mockWriteFile := func(name string, data []byte, perm os.FileMode) error {
				writeCalls++
				lastWrittenPath = name
				lastWrittenData = data
				return tt.writeFileErr
			}
			var lastMkdirPath string
			mockMkdirAll := func(path string, perm os.FileMode) error {
				mkdirCalls++
				lastMkdirPath = path
				return tt.mkdirAllErr
			}

			exporter := &Exporter{
				writeFile: mockWriteFile,
				mkdirAll:  mockMkdirAll,
				logger:    testLogger(),
				outputDir: "/tmp/test-decisions",
			}

			err := exporter.Export(context.Background(), tt.decisions, tt.meta, tt.dryRun)

			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if writeCalls != tt.wantWriteCalls {
				t.Errorf("writeFile calls: got %d, want %d", writeCalls, tt.wantWriteCalls)
			}
			if mkdirCalls != tt.wantMkdirCalls {
				t.Errorf("mkdirAll calls: got %d, want %d", mkdirCalls, tt.wantMkdirCalls)
			}

			// Verify mkdirAll is called with per-meeting subdirectory path
			if mkdirCalls > 0 && tt.mkdirAllErr == nil {
				expectedDir := "/tmp/test-decisions/weekly-sync"
				if lastMkdirPath != expectedDir {
					t.Errorf("mkdirAll path: got %q, want %q", lastMkdirPath, expectedDir)
				}
			}

			// Verify file path format for successful writes (FR-012, FR-013)
			if writeCalls > 0 && tt.writeFileErr == nil {
				expectedPath := "/tmp/test-decisions/weekly-sync/2026-04-18T14-00.md"
				if lastWrittenPath != expectedPath {
					t.Errorf("write path: got %q, want %q", lastWrittenPath, expectedPath)
				}
				// Verify content contains frontmatter
				content := string(lastWrittenData)
				if !strings.Contains(content, "topic: Weekly Sync") {
					t.Errorf("written content missing topic frontmatter")
				}
				if !strings.Contains(content, `time: "14:00"`) {
					t.Errorf("written content missing time frontmatter")
				}
				if !strings.Contains(content, "source: https://docs.google.com/document/d/doc-123/edit") {
					t.Errorf("written content missing source frontmatter")
				}
			}
		})
	}
}

// ---------- T21: Two meetings on same day at different times ----------

func TestExporterExport_SameDayDifferentTimes(t *testing.T) {
	var writtenPaths []string
	mockWriteFile := func(name string, data []byte, perm os.FileMode) error {
		writtenPaths = append(writtenPaths, name)
		return nil
	}
	mockMkdirAll := func(path string, perm os.FileMode) error {
		return nil
	}

	exporter := &Exporter{
		writeFile: mockWriteFile,
		mkdirAll:  mockMkdirAll,
		logger:    testLogger(),
		outputDir: "/tmp/test-decisions",
	}

	decisions := []models.Decision{{Category: "made", Text: "Decided"}}

	// Morning meeting at 9:00
	meta1 := models.DecisionDocContext{
		DocID:      "doc-morning",
		EventTitle: "Sprint Planning",
		EventDate:  time.Date(2026, 4, 11, 9, 0, 0, 0, time.UTC),
	}
	// Afternoon meeting at 14:30
	meta2 := models.DecisionDocContext{
		DocID:      "doc-afternoon",
		EventTitle: "Sprint Planning",
		EventDate:  time.Date(2026, 4, 11, 14, 30, 0, 0, time.UTC),
	}

	if err := exporter.Export(context.Background(), decisions, meta1, false); err != nil {
		t.Fatalf("Export morning: %v", err)
	}
	if err := exporter.Export(context.Background(), decisions, meta2, false); err != nil {
		t.Fatalf("Export afternoon: %v", err)
	}

	if len(writtenPaths) != 2 {
		t.Fatalf("expected 2 writes, got %d", len(writtenPaths))
	}

	// Verify different filenames
	wantPath1 := "/tmp/test-decisions/sprint-planning/2026-04-11T09-00.md"
	wantPath2 := "/tmp/test-decisions/sprint-planning/2026-04-11T14-30.md"
	if writtenPaths[0] != wantPath1 {
		t.Errorf("path 1: got %q, want %q", writtenPaths[0], wantPath1)
	}
	if writtenPaths[1] != wantPath2 {
		t.Errorf("path 2: got %q, want %q", writtenPaths[1], wantPath2)
	}
}

// ---------- T024: Zero-decisions edge case ----------

func TestExporterExport_ZeroDecisions(t *testing.T) {
	writeCalls := 0
	mockWriteFile := func(name string, data []byte, perm os.FileMode) error {
		writeCalls++
		return nil
	}
	mockMkdirAll := func(path string, perm os.FileMode) error {
		return nil
	}

	exporter := &Exporter{
		writeFile: mockWriteFile,
		mkdirAll:  mockMkdirAll,
		logger:    testLogger(),
		outputDir: "/tmp/test-decisions",
	}

	meta := models.DecisionDocContext{
		DocID:      "doc-empty",
		Source:     "transcript",
		EventTitle: "Empty Meeting",
		EventDate:  time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC),
	}

	err := exporter.Export(context.Background(), []models.Decision{}, meta, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Contract: no file created when decisions slice is empty
	if writeCalls != 0 {
		t.Errorf("writeFile should not be called for zero decisions, got %d calls", writeCalls)
	}
}

// ---------- T033: Frontmatter with attendees ----------

func TestRenderMarkdown_WithAttendees(t *testing.T) {
	decisions := []models.Decision{
		{Category: "made", Text: "Ship it"},
	}
	attendees := []string{"alice@example.com", "bob@example.com", "carol@example.com"}
	date := time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC)

	got := string(renderMarkdown(decisions, "Team Sync", date, attendees, "doc-attendees"))

	// Verify YAML attendees list renders correctly
	if !strings.Contains(got, "attendees:\n  - alice@example.com\n  - bob@example.com\n  - carol@example.com\n") {
		t.Errorf("attendees list not rendered correctly\n\nGot:\n%s", got)
	}
}

// ---------- T034: Frontmatter without attendees ----------

func TestRenderMarkdown_WithoutAttendees(t *testing.T) {
	decisions := []models.Decision{
		{Category: "made", Text: "Ship it"},
	}
	date := time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC)

	// Test with nil attendees
	got := string(renderMarkdown(decisions, "Solo Meeting", date, nil, "doc-solo"))
	if strings.Contains(got, "attendees:") {
		t.Errorf("attendees field should be omitted when nil\n\nGot:\n%s", got)
	}

	// Test with empty slice
	got = string(renderMarkdown(decisions, "Solo Meeting", date, []string{}, "doc-solo"))
	if strings.Contains(got, "attendees:") {
		t.Errorf("attendees field should be omitted when empty\n\nGot:\n%s", got)
	}
}

// ---------- T18: ShouldExportDecisions tests ----------

func TestShouldExportDecisions(t *testing.T) {
	tests := []struct {
		name      string
		title     string
		allowlist []string
		want      bool
	}{
		{
			name:      "empty allowlist returns true for any title (FR-011)",
			title:     "Any Meeting",
			allowlist: []string{},
			want:      true,
		},
		{
			name:      "nil allowlist returns true for any title",
			title:     "Any Meeting",
			allowlist: nil,
			want:      true,
		},
		{
			name:      "exact match returns true (FR-010)",
			title:     "Sprint Planning",
			allowlist: []string{"Sprint Planning"},
			want:      true,
		},
		{
			name:      "case-insensitive match (US2.4)",
			title:     "sprint planning",
			allowlist: []string{"Sprint Planning"},
			want:      true,
		},
		{
			name:      "no substring match (US2.3)",
			title:     "Sprint Planning - Q3 Kickoff",
			allowlist: []string{"Sprint Planning"},
			want:      false,
		},
		{
			name:      "multiple allowlist entries",
			title:     "Design Review",
			allowlist: []string{"Sprint Planning", "Design Review", "Weekly Sync"},
			want:      true,
		},
		{
			name:      "title not in allowlist",
			title:     "Weekly Standup",
			allowlist: []string{"Sprint Planning", "Design Review"},
			want:      false,
		},
		{
			name:      "special characters in meeting title",
			title:     "Q3 Budget / Planning Review",
			allowlist: []string{"Q3 Budget / Planning Review"},
			want:      true,
		},
		{
			name:      "unicode in meeting title",
			title:     "会議 Planning",
			allowlist: []string{"会議 Planning"},
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldExportDecisions(tt.title, tt.allowlist)
			if got != tt.want {
				t.Errorf("ShouldExportDecisions(%q, %v) = %v, want %v", tt.title, tt.allowlist, got, tt.want)
			}
		})
	}
}

// ---------- T035: Topic cleaning tests ----------

func TestCleanTopic(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Weekly Sync - Transcript", "Weekly Sync"},
		{"Notes by Gemini - Project Alpha", "Project Alpha"},
		{"Notes by Gemini", "Meeting"},
		{"Regular Meeting Title", "Regular Meeting Title"},
		{"", "Meeting"},
		{"Design Review: API v2", "Design Review: API v2"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := CleanTopic(tt.input)
			if got != tt.want {
				t.Errorf("CleanTopic(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
