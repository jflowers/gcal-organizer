# Research: Decision Extraction from Meeting Transcripts

**Feature**: `008-decision-extraction` | **Date**: 2026-02-26

## R1: Google Docs API Tab Creation

**Decision**: Use `AddDocumentTabRequest` via `Documents.BatchUpdate` to create the Decisions tab programmatically.

**Rationale**: The Google Docs API v1 (Go SDK `google.golang.org/api@v0.264.0`) fully supports tab operations. `AddDocumentTabRequest` accepts a `TabProperties` struct with `Title`, `Index`, `IconEmoji`, and `ParentTabId`. The response (`AddDocumentTabResponse`) returns the server-assigned `TabId` needed for targeting subsequent content insertion requests.

**Alternatives considered**:
- Playwright browser automation (used for task assignment): Rejected — slower, more fragile, requires Chrome process. The Docs API has native write support and the OAuth scope (`documents`) already authorizes write operations.
- Google Apps Script: Rejected — requires separate deployment and execution infrastructure.

**Key finding**: Tab creation and content insertion require separate BatchUpdate calls because the `TabId` is only available in the response. A 2-write-call pattern is required: (1) create tab → get TabId, (2) insert content targeting that TabId.

## R2: Cross-Tab Heading Links

**Decision**: Use `Link.Heading` with `HeadingLink{Id, TabId}` to create clickable cross-tab links from the Decisions tab to Transcript timestamp headings.

**Rationale**: The Docs API `Link` type supports 5 link targets. The `Heading` field (`*HeadingLink`) accepts both a heading ID and a target tab ID, enabling cross-tab navigation. Heading IDs are read-only (`ParagraphStyle.HeadingId`) and auto-assigned when a paragraph is styled as a heading. Since transcript H3 headings already exist, their IDs can be read from the initial `Documents.Get` call.

**Alternatives considered**:
- `Link.TabId` (tab-level link only): Rejected — navigates to the tab but not to a specific heading within it. Users need to land on the exact timestamp.
- `Link.Bookmark` with `BookmarkLink`: Rejected — bookmarks cannot be created programmatically via the Docs API (no `CreateBookmarkRequest` exists).
- `Link.Url` (external URL with anchor): Rejected — Google Docs internal navigation is better UX than opening a browser tab.

**Key finding**: The H3 timestamp headings in Google Gemini transcripts have server-assigned `HeadingId` values accessible via `tab.DocumentTab.Body.Content[].Paragraph.ParagraphStyle.HeadingId`. These can be read in Pass 1 (GET) and used in Pass 3 (BatchUpdate) for link insertion.

## R3: API Call Pattern

**Decision**: 3 Google API calls per document: 1 GET + 2 BatchUpdates, plus 1 Gemini call.

**Rationale**: The Docs API does not support forward-referencing response values within a single BatchUpdate. The server-assigned `TabId` from `AddDocumentTabRequest` is only available in `BatchUpdateDocumentResponse.Replies`, which is returned after the entire call completes. Therefore, content insertion targeting the new tab must be a separate call.

**Call sequence**:
1. `Documents.Get(docID)` — read document, extract Transcript tab content + H3 heading IDs, check for Decisions tab
2. `Gemini.GenerateContent` — send transcript text, receive structured decisions JSON
3. `Documents.BatchUpdate` — `AddDocumentTab` (create Decisions tab) → extract `TabId` from response
4. `Documents.BatchUpdate` — `InsertText` + `UpdateParagraphStyle` + `CreateParagraphBullets` + `UpdateTextStyle` (all targeting new `TabId`)

**Alternatives considered**:
- Single BatchUpdate for everything: Not possible — see rationale above.
- 4-call pattern (separate GET for heading IDs after styling): Not needed — transcript headings already exist from the initial GET. Only new headings in the Decisions tab would need a re-read, but we don't need to link *to* Decisions headings, only *from* them.

## R4: Gemini Prompt Design for Decision Extraction

**Decision**: Send the full transcript text with a structured prompt requesting JSON output with category, decision text, and associated timestamp.

**Rationale**: Gemini 2.0 Flash supports 1M token context window, sufficient for any meeting transcript (a 2-hour meeting transcript is typically 20-40K tokens). The existing codebase pattern (`ExtractAssigneesFromCheckboxes`) sends structured prompts and parses JSON responses — the same pattern applies here.

**Response format**: JSON array of objects:
```json
[
  {
    "category": "made",
    "text": "Team will adopt the new CI/CD pipeline starting next sprint",
    "timestamp": "12:34",
    "context": "After discussing the three pipeline options, the team voted unanimously for GitHub Actions"
  }
]
```

**Alternatives considered**:
- Two-pass summarize-then-extract: Rejected — adds latency and loses nuance. Full transcript analysis is more accurate.
- Pattern-matching only (regex for "we decided", "let's defer"): Rejected — too brittle. Natural language is varied; AI analysis captures implicit decisions.

## R5: Idempotency and Concurrency

**Decision**: Check for existing "Decisions" tab via document read (primary gate), plus optimistic concurrency on tab creation (secondary gate).

**Rationale**: The primary idempotency check reads all tabs from `doc.Tabs` and checks for `tab.TabProperties.Title == "Decisions"`. If found, the document is skipped. For concurrent instances, the `AddDocumentTab` call will fail if a tab with the same name was created by another instance between the read and write — this error is caught and treated as "already processed".

**Alternatives considered**:
- Distributed lock (Redis, file lock): Rejected — adds infrastructure complexity for a rare race condition in a single-user CLI tool.
- Emoji marker in document: Rejected — less clean than tab presence check; the tab itself is the artifact.

## R6: Attachment Title Matching

**Decision**: Exact match for "Notes by Gemini"; suffix match for "- Transcript" (case-sensitive).

**Rationale**: Google Gemini always names its meeting notes attachment exactly "Notes by Gemini". Standalone transcript documents follow the pattern `"[Meeting Title] - [Date] [Time] [TZ] - Transcript"` (e.g., "ComplyTime Standup - 2026/02/25 14:00 WET - Transcript"). Case-sensitive suffix matching with `strings.HasSuffix(title, "- Transcript")` is precise and avoids false positives.

**Alternatives considered**:
- Substring `strings.Contains`: Rejected — would match "Transcript Review Notes" or other unrelated documents.
- Case-insensitive matching: Not needed — Google Gemini generates consistent casing.

## R7: Document Collection and Deduplication

**Decision**: Collect eligible documents during Step 2 (calendar sync) using a new `decisionDocIDs` map with per-event deduplication preferring "Notes by Gemini".

**Rationale**: Follows the existing `notesDocIDs` pattern in the organizer (line 60 of organizer.go). During `SyncCalendarAttachments`, after iterating attachments for each event, the system checks for matching titles. If both "Notes by Gemini" and "- Transcript" are found on the same event, only the "Notes by Gemini" document ID is collected.

**Alternatives considered**:
- Separate scan pass over all documents: Rejected — wasteful; the calendar sync already iterates all events and attachments.
- Reuse `notesDocIDs`: Not appropriate — different filter criteria and different downstream processing.
