# Package organizer — Design & Contracts

Package `organizer` provides the main orchestration logic for the
gcal-organizer workflow: document organization, calendar attachment syncing,
and decision extraction.

## Contractual Side Effects

### ExtractDecisionsForDoc

**Orchestrates** decision extraction for a single document (Step 4).

| Side Effect | Classification | Description |
|-------------|---------------|-------------|
| `DocsService.HasDecisionsTab` call | Contractual | Idempotency check — skips if tab exists (FR-005) |
| `DocsService.ExtractTranscriptContent` call | Contractual | Reads transcript text and headings |
| `GeminiService.ExtractDecisions` call | Contractual | Sends transcript to AI for extraction (FR-006) |
| `DocsService.CreateDecisionsTab` call | Contractual | Creates Decisions tab with categorized content (FR-009) |
| Stats increment: `DecisionsSkipped` | Contractual | When tab exists or no transcript content found |
| Stats increment: `DecisionsProcessed` | Contractual | On successful tab creation |
| Stats increment: `DecisionsFailed` | Contractual | On AI or API failure |
| Dry-run: log only | Contractual | When `dryRun=true`, logs decisions without creating tab (FR-013) |
| Return `nil` on AI failure | Contractual | Logs warning, increments failed counter, does not propagate error (FR-017) |

**Error handling contract**: AI failures are intentionally swallowed with a
warning log. Since no Decisions tab is created, the next run will naturally
retry (idempotency-based retry — FR-017). API errors for `HasDecisionsTab`
and `ExtractTranscriptContent` are propagated.

### GetDecisionDocIDs

**Returns** a copy of the internal map of document IDs eligible for decision
extraction.

| Side Effect | Classification | Description |
|-------------|---------------|-------------|
| Return `map[string]string` | Contractual | Keys are doc IDs, values are source pattern (`"notes-by-gemini"` or `"transcript"`) |

**Contract**: Returns a defensive copy. The map is populated during
`SyncCalendarAttachments` based on attachment title matching (FR-001).

### GetNotesDocIDs

**Returns** the list of Google Doc IDs with "Notes" attachments.

| Side Effect | Classification | Description |
|-------------|---------------|-------------|
| Return `[]string` | Contractual | Doc IDs collected during `SyncCalendarAttachments` |

### SyncCalendarAttachments

**Syncs** calendar event attachments to meeting folders, shares with
attendees, and collects documents for decision extraction.

| Side Effect | Classification | Description |
|-------------|---------------|-------------|
| Drive shortcut creation | Contractual | Links attachments to meeting folders |
| Drive file sharing | Contractual | Shares folders/attachments with attendees |
| `decisionDocIDs` population | Contractual | Collects "Notes by Gemini" (exact) and "- Transcript" (suffix) docs (FR-001) |
| Per-event deduplication | Contractual | If both patterns match on same event, only "notes-by-gemini" is kept (FR-002) |
| `--owned-only` filtering | Contractual | Skips share operations for non-owned files (FR-014) |

### OrganizeDocuments

**Moves** meeting documents into topic-based subfolders under the master
folder.

| Side Effect | Classification | Description |
|-------------|---------------|-------------|
| Drive folder creation | Contractual | Creates meeting-name subfolders |
| Drive file move | Contractual | Moves documents to their topic folder |
| `--owned-only` filtering | Contractual | Skips non-owned documents |

## Interface Contracts

The organizer depends on three interfaces to allow testing without real
Google API calls:

- `DocsService` — transcript extraction, tab existence check, tab creation
- `DriveService` — folder/file operations, ownership checks
- `CalendarService` — event listing with attachments
- `GeminiService` — AI decision extraction

## Specification Reference

Full requirements: `specs/008-decision-extraction/spec.md`
Data model: `specs/008-decision-extraction/data-model.md`
