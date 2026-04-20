# Feature Specification: Decision Markdown Export

**Feature Branch**: `012-decision-markdown-export`
**Created**: 2026-04-19
**Status**: Draft
**Input**: GitHub Issue #12 — Export extracted decisions as local markdown for Dewey indexing

## Overview

gcal-organizer extracts structured decisions from meeting transcripts using Gemini AI and writes them to a "Decisions" tab in Google Docs. These decisions contain valuable project context — what was decided, what was deferred, and what remains open — but they are locked in Google Docs and not discoverable by AI agents or semantic search tools.

This feature adds a local markdown export step to the existing decision extraction pipeline. After writing decisions to Google Docs, the system also writes a markdown copy to a local directory. Each file uses YAML frontmatter and standard markdown headings, making it natively indexable by tools like Dewey for cross-repo semantic search.

The design follows a zero-coupling composability principle: gcal-organizer writes files; external tools read files. No API dependency exists between them.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Export Decisions as Local Markdown (Priority: P1)

As a meeting organizer who uses gcal-organizer, I want meeting decisions automatically saved as local markdown files so that they are available for offline reference and discoverable by AI-powered search tools without opening Google Docs.

**Why this priority**: This is the core value proposition. Without local file export, decisions remain locked in Google Docs. Writing a markdown file is the minimum viable increment that unlocks all downstream use cases (semantic search, team knowledge sharing, offline access).

**Independent Test**: Can be fully tested by running `gcal-organizer run` against a calendar with transcript-bearing events and verifying that markdown files appear in the configured output directory with correct content and structure.

**Acceptance Scenarios**:

1. **Given** a calendar event with a transcript that contains extractable decisions, **When** the user runs the standard `gcal-organizer run` command, **Then** a markdown file is created in the decisions output directory containing the same decisions that were written to the Google Docs "Decisions" tab.
2. **Given** the decisions output directory does not yet exist, **When** decision export runs for the first time, **Then** the directory is created automatically and the file is written successfully.
3. **Given** a markdown file already exists for the same meeting topic and date, **When** the transcript is reprocessed, **Then** the existing file is overwritten with the updated decisions (idempotent behavior).
4. **Given** the `--dry-run` flag is set, **When** decision extraction runs, **Then** no markdown files are written to disk and the user sees a message indicating what would have been exported.

---

### User Story 2 - Configure Export Directory (Priority: P2)

As a user who manages multiple machines or has a preferred file organization, I want to configure where decision markdown files are saved so that I can control the output location to suit my environment.

**Why this priority**: A configurable output path allows users to point the export at a Dewey-indexed directory, a shared folder, or any location that fits their workflow. Without this, the feature works but with a fixed path that may not suit all users.

**Independent Test**: Can be tested by setting the configuration value to a custom path and verifying files are written to that location instead of the default.

**Acceptance Scenarios**:

1. **Given** no export directory is configured, **When** decisions are exported, **Then** files are written to the default directory (`~/.gcal-organizer/decisions/`).
2. **Given** the user has configured a custom export directory, **When** decisions are exported, **Then** files are written to the configured directory.
3. **Given** the configured directory path uses a home directory shorthand (`~`), **When** decisions are exported, **Then** the path is expanded correctly and files are written to the resolved location.

---

### User Story 3 - Include Meeting Metadata in Frontmatter (Priority: P3)

As a user who searches across decision files, I want each markdown file to include structured metadata (date, topic, attendees) in YAML frontmatter so that search tools can filter and rank results by meeting context.

**Why this priority**: Frontmatter enriches the exported files with queryable metadata. Without it, the files still contain decisions (US1 delivers value), but search and filtering capabilities are limited.

**Independent Test**: Can be tested by exporting a decision file and verifying the YAML frontmatter contains the correct date, topic, and attendee list parsed from the calendar event.

**Acceptance Scenarios**:

1. **Given** a calendar event with attendees and a meeting title, **When** decisions are exported, **Then** the markdown file includes YAML frontmatter with `topic`, `date`, and `attendees` fields.
2. **Given** a calendar event with no attendees listed, **When** decisions are exported, **Then** the frontmatter includes `topic` and `date` but omits the `attendees` field rather than writing an empty list.
3. **Given** a meeting title contains the suffix "- Transcript" or is prefixed with "Notes by Gemini", **When** the topic is derived, **Then** these suffixes/prefixes are stripped to produce a clean topic name.

---

### Edge Cases

- What happens when the export directory path is invalid or the filesystem is read-only? The system logs a warning and continues processing (decision extraction to Google Docs still succeeds; local export failure does not block the pipeline).
- What happens when the meeting title contains characters that are invalid in filenames (e.g., `/`, `\`, `:`)? Invalid filename characters are replaced with hyphens to produce a safe filename.
- What happens when two calendar events on the same date produce the same derived topic name? The second file overwrites the first (last-write-wins). This is acceptable because decisions from the same topic on the same date represent the same meeting context.
- What happens when Gemini returns zero decisions for a transcript? No markdown file is created. Only meetings with at least one extracted decision produce an export file.
- What happens when the user has disabled decision extraction entirely? No markdown files are produced. The export step is skipped along with the rest of decision processing.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST write a markdown file to the decisions output directory for each meeting that produces at least one extracted decision.
- **FR-002**: Each exported markdown file MUST use the naming convention `<topic-slug>-<YYYY-MM-DD>.md`, where `topic-slug` is a kebab-case version of the meeting topic and `YYYY-MM-DD` is the meeting date.
- **FR-003**: Each exported markdown file MUST contain three sections matching the existing decision categories: "Decisions Made", "Decisions Deferred", and "Open Items", using level-2 markdown headings.
- **FR-004**: Each decision item MUST be rendered as a markdown bullet point under its category heading.
- **FR-005**: Each exported markdown file MUST include YAML frontmatter with at minimum `topic` (string) and `date` (ISO 8601 date string) fields.
- **FR-006**: The frontmatter SHOULD include an `attendees` field (list of strings) when attendee information is available from the calendar event. The field is omitted when no attendees are available.
- **FR-007**: The system MUST create the output directory (including parent directories) if it does not exist when the first file is exported.
- **FR-008**: If a file with the same name already exists in the output directory, the system MUST overwrite it (idempotent reprocessing).
- **FR-009**: The default output directory MUST be `~/.gcal-organizer/decisions/`.
- **FR-010**: The output directory MUST be configurable via the existing configuration mechanism (configuration file).
- **FR-011**: The `--dry-run` flag MUST suppress markdown file creation and log what would have been written.
- **FR-012**: If the markdown file cannot be written (permissions error, disk full, invalid path), the system MUST log a warning and continue processing remaining events. The failure of local export MUST NOT prevent the Google Docs "Decisions" tab from being created.
- **FR-013**: The meeting topic MUST be derived from the calendar event title, with common suffixes ("- Transcript") and prefixes ("Notes by Gemini") stripped.
- **FR-014**: Filename-unsafe characters in the topic slug MUST be replaced with hyphens, and consecutive hyphens MUST be collapsed to a single hyphen.
- **FR-015**: Empty decision categories MUST be omitted from the exported file rather than rendered as empty sections.

### Key Entities

- **Decision Export File**: A markdown file representing all decisions extracted from a single meeting. Contains YAML frontmatter (topic, date, attendees) and categorized decision content (made, deferred, open items). Named by topic slug and date.
- **Export Configuration**: The output directory path for decision files. Stored in the existing application configuration. Defaults to `~/.gcal-organizer/decisions/`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Every meeting that produces decisions via Gemini extraction also produces a corresponding local markdown file (100% parity with Google Docs export, unless local write fails).
- **SC-002**: Exported markdown files are valid markdown with parseable YAML frontmatter, verifiable by standard markdown linting tools.
- **SC-003**: Reprocessing a meeting that was previously exported overwrites the existing file without creating duplicates, producing exactly one file per topic-date combination.
- **SC-004**: A user can configure a custom export directory and see files appear at that location on the next run, with zero files at the old location for newly processed meetings.
- **SC-005**: Local export failures (permission errors, disk issues) do not prevent the rest of the pipeline from completing successfully, including the Google Docs Decisions tab creation.

## Assumptions

- The meeting topic can be reliably derived from the calendar event title. Calendar events processed by gcal-organizer have titles set by Google Calendar, which are typically descriptive meeting names.
- Attendee information is available from the Calendar API response for events the user owns. For events the user is an attendee of (not the organizer), attendee lists may be restricted by organizational policies.
- The existing configuration mechanism (viper/config file) supports adding new configuration keys without breaking existing configurations.
- The home directory shorthand (`~`) is expanded by the application, consistent with how the existing `~/.gcal-organizer/` paths are handled.
- Users who want Dewey indexing will separately configure their Dewey sources to point at the decisions directory. This feature does not modify Dewey configuration.
