// Package export provides markdown file export for extracted meeting decisions.
//
// It renders decisions as markdown files with YAML frontmatter and writes them
// to a configurable local directory. Each file contains categorized decision
// sections (Decisions Made, Decisions Deferred, Open Items) and metadata
// (topic, date, attendees) for indexing by semantic search tools.
package export

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/jflowers/gcal-organizer/pkg/models"
)

// ShouldExportDecisions returns true if the given meeting title should have
// its decisions exported, based on the configured allowlist.
// When the allowlist is empty or nil, all meetings are exported (backward
// compatible, FR-011). When non-empty, only exact matches (case-insensitive)
// are exported (FR-010). No substring matching (US2.3).
func ShouldExportDecisions(title string, allowlist []string) bool {
	if len(allowlist) == 0 {
		return true
	}
	titleLower := strings.ToLower(title)
	for _, allowed := range allowlist {
		if strings.ToLower(allowed) == titleLower {
			return true
		}
	}
	return false
}

// Exporter writes decision markdown files to a local directory.
// File I/O functions are injectable for testability (research.md D4).
type Exporter struct {
	writeFile func(string, []byte, os.FileMode) error
	mkdirAll  func(string, os.FileMode) error
	logger    *log.Logger
	outputDir string
}

// NewExporter creates an Exporter that writes to outputDir.
// Production defaults: os.WriteFile for writes, os.MkdirAll for directory creation.
func NewExporter(outputDir string, logger *log.Logger) *Exporter {
	return &Exporter{
		writeFile: os.WriteFile,
		mkdirAll:  os.MkdirAll,
		logger:    logger,
		outputDir: outputDir,
	}
}

// Export renders decisions as markdown and writes to disk.
// When dryRun is true, logs the intended path without writing (FR-011).
// Returns nil on success or dry-run. Returns an error on write failure;
// callers are responsible for handling the error per FR-012 (export failure
// must not block the pipeline).
func (e *Exporter) Export(ctx context.Context, decisions []models.Decision, meta models.DecisionDocContext, dryRun bool) error {
	// No decisions means no file to write (edge case from spec)
	if len(decisions) == 0 {
		return nil
	}

	// FR-012: per-meeting subdirectory named by TopicSlug
	// FR-013: filename uses YYYY-MM-DDTHH-MM.md format
	slug := TopicSlug(meta.EventTitle)
	timeStr := meta.EventDate.Format("2006-01-02T15-04")
	subDir := filepath.Join(e.outputDir, slug)
	filename := timeStr + ".md"
	fullPath := filepath.Join(subDir, filename)

	// Dry-run: log what would happen and return (FR-011)
	if dryRun {
		e.logger.Info("Would export decisions to file", "path", fullPath)
		return nil
	}

	// Render markdown content (includes time and source in frontmatter)
	content := renderMarkdown(decisions, meta.EventTitle, meta.EventDate, meta.Attendees, meta.DocID)

	// FR-014: create per-meeting subdirectory if needed
	if err := e.mkdirAll(subDir, 0o755); err != nil {
		e.logger.Warn("Failed to create decisions export subdirectory",
			"path", subDir,
			"error", err,
		)
		return fmt.Errorf("create export subdirectory: %w", err)
	}

	// FR-015: overwrite existing file (idempotent)
	if err := e.writeFile(fullPath, content, 0o644); err != nil {
		e.logger.Warn("Failed to write decisions markdown file",
			"path", fullPath,
			"error", err,
		)
		return fmt.Errorf("write decisions file: %w", err)
	}

	e.logger.Info("Exported decisions to markdown",
		"path", fullPath,
		"decisions", len(decisions),
	)
	return nil
}

// CleanTopic strips common prefixes and suffixes from a meeting title
// to produce a clean topic name for frontmatter (FR-013).
// This is distinct from TopicSlug which produces a filename-safe slug.
func CleanTopic(title string) string {
	topic := title

	// Strip known prefixes (case-insensitive)
	prefixes := []string{"Notes by Gemini - ", "Notes by Gemini"}
	for _, prefix := range prefixes {
		if len(topic) >= len(prefix) && strings.EqualFold(topic[:len(prefix)], prefix) {
			topic = topic[len(prefix):]
			break
		}
	}

	// Strip known suffixes (case-insensitive)
	suffixes := []string{" - Transcript", "- Transcript"}
	for _, suffix := range suffixes {
		if len(topic) >= len(suffix) && strings.EqualFold(topic[len(topic)-len(suffix):], suffix) {
			topic = topic[:len(topic)-len(suffix)]
			break
		}
	}

	topic = strings.TrimSpace(topic)
	if topic == "" {
		return "Meeting"
	}
	return topic
}

// renderMarkdown produces a markdown document with YAML frontmatter and
// categorized decision sections. Empty categories are omitted (FR-015).
// The docID parameter is used to generate the Google Doc source URL (FR-016).
func renderMarkdown(decisions []models.Decision, topic string, date time.Time, attendees []string, docID string) []byte {
	var b strings.Builder

	cleanedTopic := CleanTopic(topic)

	// YAML frontmatter
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("topic: %s\n", cleanedTopic))
	b.WriteString(fmt.Sprintf("date: \"%s\"\n", date.Format("2006-01-02")))
	b.WriteString(fmt.Sprintf("time: \"%s\"\n", date.Format("15:04")))
	if len(attendees) > 0 {
		b.WriteString("attendees:\n")
		for _, a := range attendees {
			b.WriteString(fmt.Sprintf("  - %s\n", a))
		}
	}
	// FR-016: Google Doc source link for traceability
	if docID != "" {
		b.WriteString(fmt.Sprintf("source: https://docs.google.com/document/d/%s/edit\n", docID))
	}
	b.WriteString("---\n")

	// Categorize decisions
	type category struct {
		heading string
		items   []string
	}
	categories := []category{
		{heading: "Decisions Made"},
		{heading: "Decisions Deferred"},
		{heading: "Open Items"},
	}

	categoryMap := map[string]int{
		"made":     0,
		"deferred": 1,
		"open":     2,
	}

	for _, d := range decisions {
		idx, ok := categoryMap[d.Category]
		if !ok {
			continue
		}
		categories[idx].items = append(categories[idx].items, d.Text)
	}

	// Render non-empty categories (FR-015: omit empty)
	for _, cat := range categories {
		if len(cat.items) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("\n## %s\n\n", cat.heading))
		for _, item := range cat.items {
			b.WriteString(fmt.Sprintf("- %s\n", item))
		}
	}

	return []byte(b.String())
}
