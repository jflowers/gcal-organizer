# Data Model: Decision Markdown Export

**Feature**: `012-decision-markdown-export` | **Date**: 2026-04-19

## Entities

### DecisionDocContext (new)

Replaces the existing `map[string]string` (`decisionDocIDs`) in the organizer with a richer struct that carries meeting metadata needed for markdown export.

| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| DocID | string | Google Drive file ID | Non-empty |
| Source | string | Which attachment pattern matched | `"notes-by-gemini"` or `"transcript"` |
| EventTitle | string | Calendar event title | Non-empty; used to derive topic slug and frontmatter `topic` |
| EventDate | time.Time | Calendar event start time | Used for filename date component and frontmatter `date` |
| Attendees | []string | Attendee email addresses (excluding self and resources) | May be empty; omitted from frontmatter when empty |

**Validation rules**:
- `DocID` must be non-empty
- `Source` must be one of the two allowed values
- `EventTitle` must be non-empty (calendar events always have titles)
- Per-event deduplication: if both sources match on the same event, only `"notes-by-gemini"` is kept (existing behavior preserved)

**Notes**: This struct is internal to the organizer package. It is populated during `SyncCalendarAttachments` and consumed by `ExtractDecisionsForDoc`. It is transient — not persisted.

### DecisionExportFile (conceptual — output artifact)

Represents the markdown file written to disk. Not a Go struct — it's the rendered output.

| Component | Format | Description | Constraints |
|-----------|--------|-------------|-------------|
| Filename | `<topic-slug>-<YYYY-MM-DD>.md` | Kebab-case topic + ISO date | FR-002 |
| Frontmatter `topic` | string | Clean meeting topic (prefixes/suffixes stripped) | FR-005, FR-013 |
| Frontmatter `date` | string | ISO 8601 date (`YYYY-MM-DD`) | FR-005 |
| Frontmatter `attendees` | list of strings | Attendee emails | FR-006; omitted when empty |
| Section: Decisions Made | `## Decisions Made` | Level-2 heading + bullet items | FR-003, FR-004; omitted if no "made" decisions (FR-015) |
| Section: Decisions Deferred | `## Decisions Deferred` | Level-2 heading + bullet items | FR-003, FR-004; omitted if no "deferred" decisions (FR-015) |
| Section: Open Items | `## Open Items` | Level-2 heading + bullet items | FR-003, FR-004; omitted if no "open" decisions (FR-015) |

**Example output**:

```markdown
---
topic: Weekly Engineering Sync
date: "2026-04-18"
attendees:
  - alice@example.com
  - bob@example.com
---

## Decisions Made

- Team will adopt GitHub Actions for CI/CD
- Budget approved for Q3 infrastructure upgrade

## Open Items

- Whether to migrate to new API version — need benchmarks first
```

### Exporter (new Go struct)

The service responsible for rendering decisions as markdown and writing them to disk.

| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| outputDir | string | Resolved absolute path to the export directory | Expanded from `~`; created on first write |
| writeFile | func(string, []byte, os.FileMode) error | Injected file writer | Defaults to `os.WriteFile` in production |
| mkdirAll | func(string, os.FileMode) error | Injected directory creator | Defaults to `os.MkdirAll` in production |
| logger | *log.Logger | Structured logger | From `logging.Logger` |

**Methods**:

| Method | Signature | Description |
|--------|-----------|-------------|
| Export | `Export(ctx context.Context, decisions []models.Decision, meta DecisionDocContext, dryRun bool) error` | Renders markdown and writes to disk. Returns nil on success or dry-run. Logs warning on write failure (FR-012). |

**Notes**: The `Exporter` is created once per run and reused for all documents. The `outputDir` is resolved at construction time.

### ExportConfiguration (addition to existing Config)

| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| DecisionsExportDir | string | Output directory for decision markdown files | Default: `~/.gcal-organizer/decisions/`. Configurable via config file or `GCAL_DECISIONS_EXPORT_DIR` env var. `~` expanded at load time. |

## Entity Relationships

```text
CalendarEvent (existing)
  └── Attachment (existing)
        └── DecisionDocContext (new, replaces map entry)
              ├── EventTitle → TopicSlug() → filename
              ├── EventDate → filename + frontmatter
              ├── Attendees → frontmatter
              └── Decision[] (existing, from Gemini)
                    └── DecisionExportFile (new, rendered output)
```

## Modified Entities

### Config (existing — `internal/config/config.go`)

**Added field**:
- `DecisionsExportDir string` — default `~/.gcal-organizer/decisions/`

**Added viper binding**:
- `mustBindEnv("decisions_export_dir", "GCAL_DECISIONS_EXPORT_DIR")`

### Organizer (existing — `internal/organizer/organizer.go`)

**Changed field**:
- `decisionDocIDs map[string]string` → `decisionDocContexts map[string]DecisionDocContext`

**Changed methods**:
- `GetDecisionDocIDs() map[string]string` → `GetDecisionDocContexts() map[string]DecisionDocContext`
- `ExtractDecisionsForDoc()` — gains `exporter` parameter or receives it at construction

### Stats (existing — `internal/organizer/organizer.go`)

**Added fields**:
- `DecisionsExported int` — count of successfully written markdown files
- `DecisionsExportFailed int` — count of failed markdown writes (logged as warnings)

## Slug Generation Rules

The `TopicSlug(title string) string` function in `internal/export/slug.go`:

1. Strip known prefixes (case-insensitive): `"Notes by Gemini"`, `"Notes by Gemini - "`
2. Strip known suffixes (case-insensitive): `"- Transcript"`, `" - Transcript"`
3. Trim whitespace
4. Convert to lowercase
5. Replace any character not in `[a-z0-9-]` with `-`
6. Collapse consecutive hyphens to single hyphen
7. Trim leading/trailing hyphens

**Examples**:

| Input | Output |
|-------|--------|
| `"Weekly Engineering Sync"` | `weekly-engineering-sync` |
| `"Notes by Gemini"` | (empty after stripping — use fallback `"meeting"`) |
| `"Project Alpha - Transcript"` | `project-alpha` |
| `"Q3 Budget / Planning"` | `q3-budget-planning` |
| `"Design Review: API v2"` | `design-review-api-v2` |
| `""` | `meeting` (fallback for empty input) |
