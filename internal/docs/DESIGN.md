# Package docs — Design & Contracts

Package `docs` provides Google Docs operations for checkbox extraction and
decision tab creation.

## Contractual Side Effects

### ExtractTranscriptContent

**Reads** a Google Doc (with `IncludeTabsContent(true)`) and returns parsed
transcript content.

| Side Effect | Classification | Description |
|-------------|---------------|-------------|
| Docs API GET | Contractual | Fetches document with tab content via retry loop |
| Return `*models.TranscriptContent` | Contractual | Populated with `FullText`, `TabID`, and `Headings` extracted from the Transcript tab |
| Return `nil` | Contractual | When document has no Transcript tab or no content |

**Contract**: Single-tab documents use the sole tab; multi-tab documents
search for a tab titled `"Transcript"`. H3 headings with a `HeadingId` are
collected as `TranscriptHeading` entries.

### HasDecisionsTab

**Reads** a Google Doc to check whether a tab named `"Decisions"` exists.

| Side Effect | Classification | Description |
|-------------|---------------|-------------|
| Docs API GET | Contractual | Fetches document with tab content |
| Return `bool` | Contractual | `true` if any tab title equals `"Decisions"` (idempotency gate — FR-005) |

### CreateDecisionsTab

**Writes** a new `"Decisions"` tab to a Google Doc with categorized decision
content and cross-tab heading links.

| Side Effect | Classification | Description |
|-------------|---------------|-------------|
| Docs API BatchUpdate #1 | Contractual | Creates the new tab via `AddDocumentTab` request |
| Docs API GET | Contractual | Re-fetches document to discover the new tab's ID |
| Docs API BatchUpdate #2 | Contractual | Inserts formatted content (headings, bullets, heading links) into the new tab |
| Return `ErrDecisionsTabExists` | Contractual | On HTTP 409 or duplicate-tab error message (optimistic concurrency — FR-018) |
| Return `error` | Contractual | On any other API failure |

**Content structure**: Three H2 sections (`"Decisions Made"`,
`"Decisions Deferred"`, `"Open Items"`), each with bullet-pointed decisions.
Timestamps are rendered as `[HH:MM]` prefixes with `HeadingLink` references
pointing to the matching heading in the Transcript tab.

### extractTranscriptContentFromDoc (unexported)

Pure function that parses a `*docs.Document` into `*models.TranscriptContent`.

| Side Effect | Classification | Description |
|-------------|---------------|-------------|
| None (pure) | N/A | No I/O; transforms document structure to domain model |

### buildDecisionsContent (unexported)

Pure function that transforms `[]models.Decision` into `[]contentLine` for
insertion into the Decisions tab.

| Side Effect | Classification | Description |
|-------------|---------------|-------------|
| None (pure) | N/A | No I/O; builds content structure from decisions |

### matchTimestampToHeading (unexported)

Pure function that finds the transcript heading matching a decision's
timestamp. Uses exact minute match first, then falls back to nearest
preceding heading.

| Side Effect | Classification | Description |
|-------------|---------------|-------------|
| None (pure) | N/A | Matches `HH:MM` timestamp to heading list by parsed minutes |

### parseTimestampMinutes (unexported)

Pure function that parses a timestamp string into total minutes.

| Side Effect | Classification | Description |
|-------------|---------------|-------------|
| None (pure) | N/A | Returns `-1` for unparseable timestamps |

## Error Handling

All Docs API calls use the `retry` package for transient error recovery.
`CreateDecisionsTab` specifically checks for HTTP 409 / duplicate-tab errors
to support optimistic concurrency (FR-018).

## Specification Reference

Full requirements: `specs/008-decision-extraction/spec.md`
Data model: `specs/008-decision-extraction/data-model.md`
