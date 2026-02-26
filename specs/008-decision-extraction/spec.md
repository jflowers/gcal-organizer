# Feature Specification: Decision Extraction from Meeting Transcripts

**Feature Branch**: `008-decision-extraction`  
**Created**: 2026-02-26  
**Status**: Draft  
**Input**: User description: "For all docs named ~'Notes by Gemini', with a 'Transcript' tab, add a tab named 'Decisions' with Gemini-extracted decisions linked to transcript timestamps. For all docs named ~'Transcript', add a 'Decisions' tab with the same."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Extract Decisions from Meeting Transcript (Priority: P1)

As a meeting organizer, I want decisions from my meetings automatically extracted and categorized into a new "Decisions" tab so I can quickly review what was decided, what was deferred, and what remains open without re-reading the entire transcript.

**Why this priority**: This is the core value proposition. Without decision extraction, the feature has no purpose. It delivers immediate time savings for anyone who runs recurring meetings and needs to track outcomes.

**Independent Test**: Can be fully tested by running the workflow against a calendar event that has a "Notes by Gemini" attachment with a Transcript tab. The test verifies that a "Decisions" tab is created with categorized decisions.

**Acceptance Scenarios**:

1. **Given** a calendar event within the lookback window has a "Notes by Gemini" attachment containing a "Transcript" tab, **When** the workflow processes the event, **Then** a new "Decisions" tab is created in the same document with three sections: "Decisions Made", "Decisions Deferred", and "Open Items", each populated with relevant decisions extracted from the transcript.

2. **Given** a calendar event within the lookback window has a standalone "Transcript" attachment (flat or tabbed document), **When** the workflow processes the event, **Then** a new "Decisions" tab is created in that document with the same three-section structure.

3. **Given** a calendar event has both a "Notes by Gemini" and a standalone "Transcript" attachment, **When** the workflow processes the event, **Then** only the "Notes by Gemini" document is processed (deduplication).

---

### User Story 2 - Cross-Tab Timestamp Links (Priority: P2)

As a meeting participant, I want each extracted decision to link back to the relevant timestamp in the transcript so I can verify the context and nuance of what was actually said.

**Why this priority**: Links transform the Decisions tab from a static list into a navigable reference. Without them, users must manually search the transcript to find context, which significantly reduces the feature's value.

**Independent Test**: Can be tested by clicking a decision's timestamp link in the Decisions tab and verifying it navigates to the correct heading in the Transcript tab.

**Acceptance Scenarios**:

1. **Given** a Decisions tab has been created with extracted decisions, **When** the user clicks a timestamp reference on any decision, **Then** the document navigates to the corresponding timestamp heading in the Transcript tab.

2. **Given** the transcript uses H3 headings for timestamps (e.g., "12:34"), **When** decisions are extracted and linked, **Then** each decision is associated with the nearest preceding timestamp heading from the transcript.

3. **Given** a transcript heading has no decisions associated with it, **When** the Decisions tab is generated, **Then** that timestamp is not referenced (no empty links).

---

### User Story 3 - Idempotent Processing (Priority: P3)

As a user running the workflow on a schedule, I want documents that have already been processed to be skipped automatically so decisions are not duplicated or overwritten on subsequent runs.

**Why this priority**: The workflow runs as a background service (hourly via launchd/systemd). Without idempotency, every run would create duplicate tabs or fail, requiring manual cleanup.

**Independent Test**: Can be tested by running the workflow twice against the same document and verifying the second run skips it without errors.

**Acceptance Scenarios**:

1. **Given** a document already has a "Decisions" tab from a previous run, **When** the workflow encounters the same document again, **Then** it skips the document and logs that it was already processed.

2. **Given** a document has a manually-created tab named "Decisions", **When** the workflow encounters this document, **Then** it treats the document as already processed and skips it.

---

### User Story 4 - Dry-Run and Ownership Controls (Priority: P3)

As a cautious user, I want to preview what decisions would be extracted and which documents would be modified before committing changes, and I want to restrict processing to documents I own.

**Why this priority**: Consistent with existing CLI behavior. The `--dry-run` and `--owned-only` flags are established patterns that users rely on for safe operation.

**Independent Test**: Can be tested by running with `--dry-run` and verifying no documents are modified, and by running with `--owned-only` and verifying unowned documents are skipped.

**Acceptance Scenarios**:

1. **Given** the `--dry-run` flag is set, **When** the workflow identifies a transcript document eligible for processing, **Then** it logs the decisions that would be extracted and the tab that would be created without modifying the document.

2. **Given** the `--owned-only` flag is set, **When** the workflow encounters a transcript document not owned by the authenticated user, **Then** it skips the document.

---

### Edge Cases

- What happens when the transcript is empty or contains no meaningful dialogue?
  - The system sends the transcript to the AI regardless. If the AI identifies no decisions, the Decisions tab is still created but with a note indicating no decisions were found.

- What happens when the transcript contains no timestamp headings?
  - Decisions are still extracted and listed, but without clickable timestamp links. A text-only timestamp approximation may be included if the AI can identify one from the transcript text.

- What happens when the AI fails to parse the transcript (API error, timeout)?
  - The document is skipped with a warning logged. The next run will attempt processing again since no Decisions tab was created (natural retry via idempotency).

- What happens when the document has a "Decisions" tab but it is empty?
  - The system treats the tab as existing and skips the document. The check is for tab presence, not content.

- What happens when the meeting was very short and had no substantive discussion?
  - The AI may return an empty list of decisions. The Decisions tab is created with a "No decisions identified" note. This marks the document as processed.

- What happens when the transcript language is not English?
  - The AI processes the transcript in whatever language it is written in. Decision extraction quality depends on the AI model's multilingual capabilities.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST detect documents attached to calendar events with titles containing "Notes by Gemini" or "Transcript"
- **FR-002**: System MUST deduplicate: when both a "Notes by Gemini" and a standalone "Transcript" document are attached to the same calendar event, only the "Notes by Gemini" document is processed
- **FR-003**: System MUST locate the "Transcript" tab within a multi-tab document, or use the document body content for flat/single-tab documents
- **FR-004**: System MUST extract the full text content from the transcript, including all timestamp headings and their positions
- **FR-005**: System MUST skip documents that already have a tab named "Decisions" (idempotency check)
- **FR-006**: System MUST send the full transcript text (no truncation) to the AI for decision extraction
- **FR-007**: System MUST categorize each extracted decision as one of: "made", "deferred", or "open"
- **FR-008**: System MUST associate each decision with the nearest preceding timestamp from the transcript
- **FR-009**: System MUST create a new tab named "Decisions" in the processed document
- **FR-010**: System MUST organize the Decisions tab into three sections with clear headings: "Decisions Made", "Decisions Deferred", and "Open Items"
- **FR-011**: System MUST render each decision as a bullet point within its appropriate section
- **FR-012**: System MUST create clickable links from each decision's timestamp reference to the corresponding timestamp heading in the Transcript tab
- **FR-013**: System MUST respect the `--dry-run` flag by logging intended actions without modifying any documents
- **FR-014**: System MUST respect the `--owned-only` flag by skipping documents not owned by the authenticated user
- **FR-015**: System MUST process only documents attached to calendar events within the configured lookback window (`--days`)
- **FR-016**: System MUST create a Decisions tab with a "No decisions identified" note when the AI finds no decisions in the transcript
- **FR-017**: System MUST log a warning and skip the document when the AI call fails, allowing natural retry on subsequent runs

### Assumptions

- Transcript documents generated by Google Gemini use H3 headings for timestamps
- The Google Docs write scope (`DocumentsScope`) is already authorized via existing OAuth configuration
- The AI model configured in `GEMINI_MODEL` (default: `gemini-2.0-flash`) has sufficient context window for full meeting transcripts
- Calendar event attachments include the document title, which is used for pattern matching

### Key Entities

- **Decision**: Represents a single decision extracted from a transcript. Attributes: category (made/deferred/open), description text, associated timestamp, brief context excerpt from the transcript
- **TranscriptHeading**: Represents a timestamp heading in the transcript. Attributes: heading identifier (for linking), display text (e.g., "12:34"), position in the document
- **TranscriptContent**: The full text of a transcript and its associated metadata (tab identifier, list of timestamp headings)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For every eligible transcript document processed, a "Decisions" tab is created containing categorized decisions within 30 seconds per document
- **SC-002**: At least 90% of decisions identified by the AI are correctly categorized (made vs. deferred vs. open) when compared against manual review of 10 sample transcripts
- **SC-003**: Every decision with an identifiable transcript timestamp includes a working clickable link to the correct heading in the Transcript tab
- **SC-004**: Running the workflow twice on the same set of documents produces zero duplicate Decisions tabs (100% idempotency)
- **SC-005**: Users can review meeting decisions in under 1 minute by reading the Decisions tab, compared to 10+ minutes scanning the full transcript
- **SC-006**: All existing workflow steps (organize, sync-calendar, assign-tasks) continue to function without regression
- **SC-007**: The feature processes documents only within the configured lookback window, with zero documents processed outside that window
