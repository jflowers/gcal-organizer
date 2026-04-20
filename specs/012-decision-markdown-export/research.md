# Research: Decision Markdown Export

**Feature**: `012-decision-markdown-export` | **Date**: 2026-04-19

## Research Questions

### RQ-1: How does the existing decision extraction pipeline work, and where should the export hook be inserted?

**Finding**: The pipeline flows through these stages:

1. **Collection** (`organizer.SyncCalendarAttachments`): Calendar events are scanned. Attachments titled "Notes by Gemini" or ending with "- Transcript" are collected into `o.decisionDocIDs` (a `map[string]string` of docID→source).

2. **Orchestration** (`cmd/gcal-organizer/main.go`, Step 4): Iterates over `decisionDocIDs`, calling `org.ExtractDecisionsForDoc()` for each.

3. **Extraction** (`organizer.ExtractDecisionsForDoc`): For each document:
   - Checks for existing Decisions tab (idempotency)
   - Extracts transcript content via `docsSvc.ExtractTranscriptContent()`
   - Calls `geminiSvc.ExtractDecisions()` to get `[]models.Decision`
   - Calls `docsSvc.CreateDecisionsTab()` to write to Google Docs
   - Updates stats

**Hook point**: The markdown export should be called inside `ExtractDecisionsForDoc()`, **after** the Gemini extraction succeeds and **before or after** `CreateDecisionsTab()`. Placing it after `CreateDecisionsTab()` ensures the Google Docs write (the primary output) is not delayed by local I/O. Per FR-012, local export failure must not block the Docs tab creation, so the export call should be independent — a failure is logged as a warning but does not return an error.

**Decision**: Insert the export call after `CreateDecisionsTab()` succeeds. If the Docs tab creation fails, no local export is attempted (the decisions are considered "not processed"). If the Docs tab succeeds but local export fails, log a warning and continue.

### RQ-2: What metadata is available for YAML frontmatter at the export hook point?

**Finding**: At the point where `ExtractDecisionsForDoc` runs, the available data is:
- `docID` (string) — the Google Doc ID
- `decisions` (`[]models.Decision`) — extracted decisions with category, text, timestamp, context
- `transcript` (`*models.TranscriptContent`) — full text and headings

**Missing at this point**:
- **Meeting topic/title** — The calendar event title is known during `SyncCalendarAttachments` but is not passed through to `ExtractDecisionsForDoc`. The `decisionDocIDs` map only stores `docID→source`.
- **Meeting date** — Similarly, the event's `Start` time is available during calendar sync but not threaded through.
- **Attendees** — The `CalendarEvent.Attendees` list is available during calendar sync but not passed to the decision extraction step.

**Decision**: Enrich the `decisionDocIDs` map to carry meeting metadata alongside the doc ID. Instead of `map[string]string` (docID→source), use a new struct `DecisionDocContext` that includes `Source`, `EventTitle`, `EventDate`, and `Attendees`. This keeps the change localized to the organizer package without requiring new API calls.

### RQ-3: How should the topic slug be derived from the meeting title?

**Finding**: The spec requires (FR-013, FR-014):
- Strip common suffixes: "- Transcript"
- Strip common prefixes: "Notes by Gemini"
- Convert to kebab-case
- Replace filename-unsafe characters with hyphens
- Collapse consecutive hyphens

**Existing patterns**: The codebase already parses meeting names from filenames in `drive.ListMeetingDocuments()` using a regex pattern. However, the topic slug for export files is derived from the **calendar event title**, not the document filename.

**Decision**: Create a `slug.go` file in the `internal/export` package with a `TopicSlug(title string) string` function that:
1. Strips known prefixes/suffixes (case-insensitive)
2. Converts to lowercase
3. Replaces non-alphanumeric characters (except hyphens) with hyphens
4. Collapses consecutive hyphens
5. Trims leading/trailing hyphens

This is a pure function with no external dependencies, easily tested with table-driven tests.

### RQ-4: How should the markdown file be rendered?

**Finding**: The spec requires (FR-003, FR-004, FR-005, FR-015):
- YAML frontmatter with `topic`, `date`, and optionally `attendees`
- Three sections: "Decisions Made", "Decisions Deferred", "Open Items" (level-2 headings)
- Each decision as a bullet point
- Empty categories omitted (FR-015)

**Existing pattern**: `docs.buildDecisionsContent()` already constructs content lines with the same three categories. However, it includes "No decisions identified" for empty categories (for the Google Docs tab). The markdown export should **omit** empty categories entirely per FR-015.

**Decision**: The `internal/export` package will have its own rendering function that takes `[]models.Decision` and metadata, and produces a `[]byte` markdown output. It will reuse the same category names and ordering as `buildDecisionsContent()` but with markdown-specific formatting and the FR-015 omission rule. This avoids coupling to the Docs-specific rendering.

### RQ-5: How should the export directory configuration work?

**Finding**: The existing config system uses:
- `config.Config` struct with fields
- `viper` for environment variable binding and config file loading
- `config.DefaultConfig()` for defaults
- Pattern: `mustBindEnv("key", "ENV_VAR")` + `if v := viper.GetString("key"); v != "" { cfg.Field = v }`

**Decision**: Add a `DecisionsExportDir` field to `config.Config` with default `~/.gcal-organizer/decisions/`. Bind to `GCAL_DECISIONS_EXPORT_DIR` environment variable. The `~` expansion is already handled by the application for other paths (e.g., `CredentialsFile`, `TokenFile`). Follow the exact same pattern.

### RQ-6: How should `--dry-run` interact with the export?

**Finding**: The `dryRun` flag is already threaded through `ExtractDecisionsForDoc` as a parameter. When `dryRun` is true, the method logs "Would extract decisions" and returns early **before** calling Gemini or creating the Docs tab.

**Problem**: In dry-run mode, decisions are never extracted (Gemini is not called), so there's nothing to export. The dry-run log message should indicate that markdown export would also happen.

**Decision**: In dry-run mode, add a log line like "Would export decisions to <path>" alongside the existing "Would extract decisions" message. No file is written. This matches the existing dry-run pattern.

### RQ-7: What about the `--dry-run` case where decisions ARE extracted but we just don't want to write?

**Finding**: Re-reading the spec more carefully — FR-011 says "The `--dry-run` flag MUST suppress markdown file creation and log what would have been written." This implies decisions might be extracted (Gemini called) but the file write is suppressed.

However, the current implementation skips Gemini entirely in dry-run mode. The export dry-run behavior should match: if the organizer skips Gemini in dry-run, the export is also skipped with a "would export" message.

**Decision**: The export function receives a `dryRun` parameter. When true, it logs the file path that would be written and returns without writing. The organizer passes its own dry-run state to the exporter.

### RQ-8: How should the exporter be made testable?

**Finding**: The export function writes to the filesystem. For unit tests, we need to avoid actual filesystem writes.

**Options considered**:
1. **`afero` filesystem abstraction** — Already an indirect dependency (`github.com/spf13/afero`). Provides `afero.Fs` interface with `afero.MemMapFs` for testing.
2. **`io/fs` + custom writer** — Standard library, but `io/fs` is read-only. Would need a custom `WriteFS` interface.
3. **Inject `func(path string, data []byte, perm os.FileMode) error`** — Simple function injection.

**Decision**: Use option 3 — inject a `WriteFile` function. This is the simplest approach that satisfies testability without adding a new dependency or coupling to afero's API. The production code uses `os.WriteFile`; tests inject a mock. The `MkdirAll` function is similarly injectable.

```go
type Exporter struct {
    writeFile func(name string, data []byte, perm os.FileMode) error
    mkdirAll  func(path string, perm os.FileMode) error
    logger    *log.Logger
}
```

This follows the project's existing pattern of dependency injection (e.g., `organizer.New(cfg, driveSvc, calSvc)`).

## Summary of Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | Hook export after `CreateDecisionsTab()` in `ExtractDecisionsForDoc` | Primary output (Docs) completes first; local export failure doesn't block |
| D2 | Enrich `decisionDocIDs` to carry event metadata (title, date, attendees) | Avoids extra API calls; metadata already available during calendar sync |
| D3 | New `internal/export` package with `slug.go` + `export.go` | Separation of concerns; independently testable; follows existing package pattern |
| D4 | Inject `writeFile`/`mkdirAll` functions for testability | Simplest DI approach; no new dependencies; matches project patterns |
| D5 | Omit empty categories in markdown (FR-015) vs. Docs "No decisions identified" | Spec explicitly requires omission; different rendering rules for different outputs |
| D6 | Config key `decisions_export_dir` with default `~/.gcal-organizer/decisions/` | Follows existing config pattern; viper binding + env var |
| D7 | Dry-run logs "would export" without writing | Matches existing dry-run behavior throughout the pipeline |
