# Feature Specification: YAML Config Migration & Decision Export Enhancements

**Feature Branch**: `013-yaml-config-decision-export`
**Created**: 2026-04-20
**Status**: Draft
**Input**: Migrate .env to YAML config, add meeting allowlist with exact title match, organize decision exports into per-meeting folders with time-based filenames, and include Google Doc source links in frontmatter for Dewey traceability.

## Overview

gcal-organizer currently uses a flat `.env` file (`~/.gcal-organizer/.env`) for configuration. This format cannot express lists or nested structures, making it unsuitable for features like a meeting allowlist. Additionally, the decision markdown export (feature 012) writes all files into a single flat directory with date-only filenames, which does not support multiple meetings per day or browsable per-meeting organization.

This feature makes four coordinated changes:

1. **YAML config migration**: Replace the `.env` file with a structured `config.yaml` that supports lists and nested configuration. When the application starts and finds a `.env` file, it automatically converts it to `config.yaml` and deletes the `.env` file.

2. **Meeting allowlist**: Add a configuration option that lets users list specific meeting titles whose decisions should be exported. Only meetings whose titles exactly match an entry in the list are exported. When the list is empty or absent, all meetings are exported (backward compatible).

3. **Per-meeting folder structure**: Organize exported decision files into subdirectories named after the meeting topic, with filenames that include the meeting time (not just the date), supporting multiple occurrences of the same meeting on a single day.

4. **Google Doc source link**: Include a link to the original Google Doc in each exported markdown file's frontmatter, enabling traceability from decision files back to the source document via Dewey semantic search.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Automatic Config Migration (Priority: P1)

As an existing user who has configured gcal-organizer via the `.env` file, I want the application to automatically convert my `.env` configuration to the new YAML format on startup so that I don't have to manually recreate my settings.

**Why this priority**: Without migration, existing users would lose their configuration or need to manually recreate it. This is the foundation that all other stories depend on -- the YAML config must exist before the new configuration options can be used.

**Independent Test**: Can be tested by placing a `.env` file with known settings in the config directory, running any gcal-organizer command, and verifying that a `config.yaml` file is created with equivalent settings and the `.env` file is deleted.

**Acceptance Scenarios**:

1. **Given** a `~/.gcal-organizer/.env` file with standard configuration values (master folder name, days to look back, keywords, Gemini model), **When** any gcal-organizer command is run, **Then** a `config.yaml` file is created in the same directory containing equivalent settings in YAML format, and the `.env` file is deleted.
2. **Given** a `config.yaml` already exists in the config directory, **When** any gcal-organizer command is run, **Then** the application loads configuration from `config.yaml` and does not look for or attempt to migrate a `.env` file.
3. **Given** neither `.env` nor `config.yaml` exists, **When** the user runs `gcal-organizer init`, **Then** a `config.yaml` is generated with default values (not a `.env` file).
4. **Given** a `.env` file contains secrets that have already been migrated to the OS keychain, **When** the config migration runs, **Then** the secrets are NOT written to `config.yaml` -- they remain in the keychain only.
5. **Given** a `.env` file with a custom value for `CHROME_PROFILE_PATH` or other path settings, **When** migration occurs, **Then** path values are preserved exactly as specified in the resulting `config.yaml`.

---

### User Story 2 - Filter Decisions by Meeting Title (Priority: P2)

As a meeting organizer who attends many meetings but only wants to track decisions from specific recurring meetings, I want to list which meetings should have their decisions exported so that my decisions directory contains only the meetings I care about.

**Why this priority**: The meeting allowlist is the primary new capability that motivated the config format change. Without it, users export decisions from all meetings, creating noise in their decisions directory. This delivers immediate value once the YAML config is in place.

**Independent Test**: Can be tested by configuring a list of meeting titles, running `gcal-organizer run` against a calendar with meetings both on and off the list, and verifying that only matching meetings produce decision files.

**Acceptance Scenarios**:

1. **Given** a `config.yaml` with a meetings list containing "Sprint Planning" and "Design Review", **When** the user processes calendar events that include "Sprint Planning", "Design Review", and "Weekly Standup", **Then** decision files are created only for "Sprint Planning" and "Design Review".
2. **Given** a `config.yaml` with an empty meetings list or no meetings key at all, **When** the user processes calendar events, **Then** decisions are exported for all meetings that produce decisions (backward compatible with feature 012 behavior).
3. **Given** a meetings list containing "Sprint Planning", **When** a calendar event titled "Sprint Planning - Q3 Kickoff" is processed, **Then** no decision file is created because the title does not exactly match "Sprint Planning".
4. **Given** a meetings list containing "Sprint Planning", **When** a calendar event titled "sprint planning" (different case) is processed, **Then** a decision file IS created because matching is case-insensitive.

---

### User Story 3 - Per-Meeting Folders with Time-Based Filenames (Priority: P3)

As a user who browses exported decision files, I want decisions organized into folders named after each meeting with filenames that include the meeting time so that I can quickly navigate to a specific meeting's history and distinguish between multiple meetings on the same day.

**Why this priority**: This improves the browsability and organization of the decisions directory. Feature 012 already exports files -- this story enhances the layout. The feature is independently valuable but builds on the export mechanism.

**Independent Test**: Can be tested by running `gcal-organizer run` against a calendar with multiple meetings (including two on the same day) and verifying the folder structure and filename format.

**Acceptance Scenarios**:

1. **Given** a meeting titled "Sprint Planning" at 9:00 AM on 2026-04-11, **When** decisions are exported, **Then** the file is created at `<export_dir>/sprint-planning/2026-04-11T09-00.md`.
2. **Given** two meetings titled "Sprint Planning" on the same day at 9:00 AM and 2:30 PM, **When** both are processed, **Then** two separate files are created: `sprint-planning/2026-04-11T09-00.md` and `sprint-planning/2026-04-11T14-30.md`.
3. **Given** a meeting titled "Q3 Budget / Planning Review", **When** decisions are exported, **Then** the folder is named `q3-budget-planning-review` (filename-safe slug, same as feature 012's `TopicSlug` behavior).
4. **Given** the per-meeting subdirectory does not yet exist, **When** the first decision file for that meeting is exported, **Then** the subdirectory is created automatically.
5. **Given** the `--dry-run` flag is set, **When** decision export runs, **Then** the logged path reflects the new folder structure (e.g., "Would export decisions to sprint-planning/2026-04-11T09-00.md").

---

### User Story 4 - Google Doc Source Link in Frontmatter (Priority: P3)

As a user or AI agent searching decision files via Dewey, I want each exported markdown file to include a link back to the original Google Doc so that I can trace a decision to its full meeting context and verify it against the original transcript.

**Why this priority**: Source attribution makes the exported files more useful for both human review and machine indexing. Dewey can follow the link to connect decision exports to other Google Docs content. This is additive metadata -- the export works without it.

**Independent Test**: Can be tested by exporting a decision file and verifying the YAML frontmatter contains a `source` field with a valid Google Docs URL.

**Acceptance Scenarios**:

1. **Given** a meeting with decisions extracted from a Google Doc with a known document ID, **When** the decision file is exported, **Then** the YAML frontmatter includes a `source` field containing `https://docs.google.com/document/d/<docID>/edit`.
2. **Given** an exported decision file is indexed by Dewey, **When** a user searches for "decisions about authentication", **Then** the search results include the source link, allowing the user to navigate to the original document.

---

### Edge Cases

- What happens when the `.env` file contains malformed lines (no `=` sign, empty key)? Malformed lines are skipped during migration. A warning is logged for each skipped line. Non-secret, non-config lines (comments, blanks) are discarded since YAML uses its own comment syntax.
- What happens when a meeting title in the allowlist contains special characters (quotes, commas, unicode)? YAML natively handles these as quoted strings. The matching comparison uses the raw string value.
- What happens when the same meeting occurs multiple times on the same day at the exact same time? The second file overwrites the first (last-write-wins, same as feature 012 idempotency behavior). This is an unlikely edge case since calendar events at the same time for the same meeting represent the same event.
- What happens when the `--config` CLI flag points to a `.env` file explicitly? The file is migrated to `config.yaml` in the same directory, the `.env` is deleted, and the `--config` flag value is effectively ignored on subsequent runs (the application finds `config.yaml`).
- What happens when the config directory is missing entirely? The application creates the directory and a default `config.yaml` when `gcal-organizer init` is run, same as the current behavior with `.env`.
- What happens to the service mode wrapper script and systemd unit that reference `.env`? They are updated to no longer depend on sourcing the `.env` file. The application binary handles all configuration loading internally from `config.yaml`.

## Requirements *(mandatory)*

### Functional Requirements

**Config Migration**:

- **FR-001**: On startup, if `~/.gcal-organizer/.env` exists and `~/.gcal-organizer/config.yaml` does not exist, the system MUST parse the `.env` file, create an equivalent `config.yaml`, and delete the `.env` file.
- **FR-002**: The migration MUST map `.env` keys to YAML keys: `GCAL_MASTER_FOLDER_NAME` to `master_folder_name`, `GCAL_DAYS_TO_LOOK_BACK` to `days_to_look_back`, `GCAL_FILENAME_KEYWORDS` to `filename_keywords` (as a YAML list), `GEMINI_MODEL` to `gemini_model`, `CHROME_PROFILE_PATH` to `chrome_profile_path`.
- **FR-003**: The migration MUST NOT write secret values (`GEMINI_API_KEY`, `GOOGLE_CREDENTIALS_FILE`) to `config.yaml`. Secrets remain in the OS keychain.
- **FR-004**: If `config.yaml` already exists, the system MUST load configuration from it and skip `.env` detection entirely.
- **FR-005**: Environment variables MUST continue to override `config.yaml` values (preserving the existing override precedence).
- **FR-006**: The `gcal-organizer init` command MUST generate a `config.yaml` file (not `.env`) for new installations.
- **FR-007**: The `gcal-organizer doctor` command MUST check for `config.yaml` (not `.env`) and report its status.
- **FR-008**: The `--config` CLI flag MUST accept a path to a YAML config file. If pointed at a `.env` file, the system MUST migrate it before proceeding.

**Meeting Allowlist**:

- **FR-009**: The system MUST support a `decisions.meetings` configuration key containing a list of meeting title strings.
- **FR-010**: When `decisions.meetings` is non-empty, the system MUST export decisions only for calendar events whose title exactly matches one of the listed meeting titles (case-insensitive comparison).
- **FR-011**: When `decisions.meetings` is empty or absent, the system MUST export decisions for all meetings that produce decisions (backward compatible).

**Per-Meeting Folders & Time Filenames**:

- **FR-012**: Exported decision files MUST be written to a subdirectory named after the meeting topic slug (using the existing `TopicSlug()` function) within the decisions export directory.
- **FR-013**: Exported decision filenames MUST include the meeting time in the format `YYYY-MM-DDTHH-MM.md`, using the calendar event's start time.
- **FR-014**: The system MUST create the per-meeting subdirectory if it does not exist.
- **FR-015**: If a file with the same name already exists in the per-meeting subdirectory, the system MUST overwrite it (idempotent behavior).

**Google Doc Source Link**:

- **FR-016**: Each exported decision markdown file MUST include a `source` field in the YAML frontmatter containing the URL of the source Google Doc in the format `https://docs.google.com/document/d/<docID>/edit`.

**Service Mode**:

- **FR-017**: The service mode wrapper script MUST NOT depend on sourcing a `.env` file. Configuration is loaded by the application binary from `config.yaml`.
- **FR-018**: The systemd unit file template MUST NOT include an `EnvironmentFile` directive referencing `.env`.

### Key Entities

- **Config File (`config.yaml`)**: YAML-formatted configuration file containing all non-secret application settings. Supports nested structures (`decisions.export_dir`, `decisions.meetings`) and list values (`filename_keywords`, `decisions.meetings`). Located at `~/.gcal-organizer/config.yaml`.
- **Meeting Allowlist**: An ordered list of meeting title strings within the `decisions` configuration section. Used as an exact-match (case-insensitive) filter for decision export. Empty list means no filtering.
- **Decision Export File**: A markdown file with YAML frontmatter (topic, date, time, attendees, source) written to `<export_dir>/<topic-slug>/YYYY-MM-DDTHH-MM.md`. The `source` field links back to the original Google Doc.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Existing users with a `.env` file experience zero manual intervention -- the application migrates their configuration automatically on the next run, and all subsequent runs use `config.yaml` with identical behavior.
- **SC-002**: Users can configure a meeting allowlist and see decisions exported only for listed meetings, with 100% filtering accuracy (no false positives from substring matches, no false negatives from case differences).
- **SC-003**: Users with recurring meetings can browse decisions organized by meeting name, with multiple same-day meetings distinguishable by time in the filename.
- **SC-004**: Every exported decision file contains a clickable link to the original Google Doc, enabling one-click navigation from a decision to its full meeting context.
- **SC-005**: New installations via `gcal-organizer init` produce a `config.yaml` file that the user can edit with standard YAML syntax, including list values.

## Assumptions

- The `.env` file format used by gcal-organizer is consistent and well-structured (generated by `gcal-organizer init`). Hand-edited `.env` files with non-standard formatting are handled on a best-effort basis with warnings for unrecognized lines.
- Secrets have been migrated to the OS keychain in all production deployments. The `.env` file contains only non-secret configuration values at the time of migration.
- The existing `TopicSlug()` function produces suitable directory names (filesystem-safe, deterministic, human-readable). No changes to slug generation logic are needed.
- Calendar event start times are available with minute-level precision from the Calendar API, which is sufficient for the `HH-MM` component of filenames.
- The Google Doc ID is available in `DecisionDocContext.DocID` at the time of export, and the URL format `https://docs.google.com/document/d/<docID>/edit` is stable and correct for all Google Workspace documents.
- The `filename_keywords` value in existing `.env` files uses comma-separated format (e.g., `"Notes,Meeting"`). No other configuration values use comma-separated lists. The migration splits this value into a YAML list.
- Service mode (wrapper script, systemd unit) is not widely adopted. Updating the service templates to remove `.env` sourcing has minimal user impact.
