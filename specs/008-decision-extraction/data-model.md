# Data Model: Decision Extraction from Meeting Transcripts

**Feature**: `008-decision-extraction` | **Date**: 2026-02-26

## Entities

### Decision

Represents a single decision extracted from a meeting transcript by the AI.

| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| Category | string | Decision classification | One of: `"made"`, `"deferred"`, `"open"` |
| Text | string | Description of the decision | Non-empty |
| Timestamp | string | Associated meeting timestamp | Format: `"HH:MM"` or empty if not identifiable |
| Context | string | Brief excerpt from the transcript providing context | May be empty |

**Validation rules**:
- `Category` must be one of the three allowed values
- `Text` must not be empty
- `Timestamp` is optional; when present, must match a heading in the transcript

**Notes**: Decisions are transient — they exist only during the processing pipeline between Gemini response parsing and Docs API content insertion. They are not persisted to disk or database.

### TranscriptHeading

Represents an H3 timestamp heading found in the Transcript tab of a document.

| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| HeadingID | string | Server-assigned heading identifier | Read-only from Docs API, format: `"h.xxxxxxxx"` |
| Text | string | Display text of the heading | Typically a timestamp like `"12:34"` |
| Index | int64 | Zero-based position in the document body | >= 0 |

**Validation rules**:
- `HeadingID` must be non-empty (only populated for actual headings)
- `Text` extracted from `ParagraphStyle.HeadingId` parent paragraph content

**Notes**: HeadingIDs are assigned by the Google Docs server and are immutable. They are used to construct cross-tab `HeadingLink` references.

### TranscriptContent

Aggregates the full parsed content of a transcript tab.

| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| TabID | string | The Transcript tab identifier | Non-empty |
| FullText | string | Complete text content of the transcript | May be empty for edge case |
| Headings | []TranscriptHeading | Ordered list of H3 timestamp headings | May be empty if no headings found |

**Relationships**:
- Contains zero or more `TranscriptHeading` entries
- Referenced by `Decision` via `Timestamp` → nearest `TranscriptHeading.Text` match

### DecisionDocCandidate

Represents a document collected during calendar sync for decision processing in Step 4. Internal to the organizer — not a persisted entity.

| Field | Type | Description | Constraints |
|-------|------|-------------|-------------|
| DocID | string | Google Drive file ID | Non-empty |
| Source | string | Which pattern matched | `"notes-by-gemini"` or `"transcript"` |

**Validation rules**:
- Per-event deduplication: if both sources match on the same event, only `"notes-by-gemini"` is kept

## Entity Relationships

```text
CalendarEvent (existing)
  └── Attachment (existing)
        └── DecisionDocCandidate (new, transient)
              └── TranscriptContent (new, transient)
                    ├── TranscriptHeading[] (new, transient)
                    └── Decision[] (new, transient, from Gemini)
```

## State Transitions

Documents move through a simple linear state:

```text
[Unprocessed] → [Processing] → [Decisions Tab Created]
                     │
                     └─→ [Skipped] (on error or AI failure)
```

- **Unprocessed → Processing**: Document passes title match + no existing Decisions tab
- **Processing → Decisions Tab Created**: All 3 API calls succeed; tab with content exists
- **Processing → Skipped**: AI failure (FR-017), API error, or concurrent creation (FR-018)
- **Decisions Tab Created → (terminal)**: Idempotent; subsequent runs skip via FR-005

There is no reverse transition. Once a Decisions tab exists, it is permanent. Users can manually delete the tab to trigger reprocessing on the next run.

## Gemini Response Schema

The AI returns a JSON array conforming to this schema:

```json
[
  {
    "category": "made | deferred | open",
    "text": "Description of the decision",
    "timestamp": "HH:MM",
    "context": "Brief transcript excerpt"
  }
]
```

**Parsing rules**:
- Strip markdown code fences (```` ```json ````) if present
- Extract JSON array from response text using regex `\[[\s\S]*\]`
- Parse into `[]Decision` structs
- Filter out entries with empty `Text` field
- Validate `Category` is one of the three allowed values; default to `"open"` if invalid

## Timestamp-to-Heading Matching

When associating a `Decision.Timestamp` with a `TranscriptHeading`:

1. Parse the decision's timestamp as `HH:MM`
2. Find the `TranscriptHeading` whose `Text` contains the matching timestamp
3. If exact match found → use that heading's `HeadingID` for linking
4. If no exact match → find the nearest preceding heading by document position
5. If no headings at all → decision is rendered without a clickable link (text-only timestamp)
