# Feature Specification: Meeting Organizer & Action Item Tracker CLI

**Feature Branch**: `001-gcal-organizer-cli`  
**Created**: 2026-02-01  
**Status**: Implemented  
**Input**: Rewrite of Google Apps Script to Go CLI with GCP Gemini API key support

## Overview

A command-line tool that automates the lifecycle of meeting notes by:
1. Organizing meeting documents into a structured Drive folder hierarchy
2. Syncing calendar event attachments to corresponding meeting folders
3. Sharing meeting folders with calendar event attendees

> [!NOTE]
> Task extraction and assignment is handled separately via browser automation (see spec 002).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Organize Meeting Documents (Priority: P1)

As a user, I want to run a command that scans my Google Drive for meeting notes and automatically organizes them into topic-based subfolders within my Master Folder.

**Why this priority**: This is the core organizational function. Without it, the other features have no context.

**Independent Test**: Can be fully tested by creating a test document with a meeting name pattern and verifying it moves to the correct subfolder.

**Acceptance Scenarios**:

1. **Given** a document named "Team Standup - 2026-01-30" exists in Drive, **When** I run `gcal-organizer organize`, **Then** a subfolder "Team Standup" is created (or located) in the Master Folder and the document is moved there.

2. **Given** a document I don't own named "External Meeting - 2026-01-30", **When** I run `gcal-organizer organize`, **Then** a shortcut to the document is created in the "External Meeting" subfolder instead of moving it.

3. **Given** a document that doesn't match the naming pattern (e.g., "Random Notes"), **When** I run `gcal-organizer organize`, **Then** the document is skipped and logged appropriately.

---

### User Story 2 - Sync Calendar Attachments (Priority: P2)

As a user, I want the tool to scan my recent calendar events and sync any attached documents or Drive links to the corresponding meeting folders.

**Why this priority**: Extends organization capability to calendar-linked resources.

**Independent Test**: Can be tested by creating a calendar event with an attachment and running the sync command.

**Acceptance Scenarios**:

1. **Given** a calendar event "Weekly Planning" with a Drive attachment, **When** I run `gcal-organizer sync-calendar --days 8`, **Then** a shortcut to the attachment appears in the "Weekly Planning" subfolder.

2. **Given** a calendar event with a Drive URL in the description (not as attachment), **When** I run `gcal-organizer sync-calendar`, **Then** the URL is detected and a shortcut is created.

3. **Given** a calendar event with no attachments or Drive links, **When** I run `gcal-organizer sync-calendar`, **Then** the event is skipped gracefully.

---

> [!NOTE]
> **Task extraction and assignment moved to spec 002** (Browser Automation)
> The `assign-tasks` command uses Gemini AI + Playwright to assign tasks via native Google Docs UI.

---

### User Story 3 - Run Full Workflow (Priority: P1)

As a user, I want a single command to run the complete workflow: organize, sync, share folders, and assign tasks.

**Why this priority**: Primary user interaction mode for daily use.

**Independent Test**: Can be tested with a sample setup of docs and calendar events.

**Acceptance Scenarios**:

1. **Given** configured environment variables and credentials, **When** I run `gcal-organizer run`, **Then** all sub-workflows execute in sequence: Step 1 (organize), Step 2 (sync calendar), Step 3 (assign tasks from Notes by Gemini docs).

2. **Given** `--dry-run` flag is provided, **When** I run `gcal-organizer run --dry-run`, **Then** Steps 1 & 2 log actions without changes, and Step 3 reports how many docs would be scanned without launching browser.

3. **Given** calendar sync discovers Notes by Gemini documents, **When** Step 3 runs, **Then** each Notes doc is scanned for unassigned checkboxes and browser automation assigns tasks.

---

### User Story 4 - Share Folder with Calendar Attendees (Priority: P2)

As a user, I want my meeting folders to be automatically shared with all attendees from the corresponding calendar event so they can access the notes and attachments.

**Why this priority**: Enhances collaboration by ensuring all meeting participants have access to meeting materials.

**Independent Test**: Can be tested by creating a meeting folder and verifying all calendar event attendees receive view access.

**Acceptance Scenarios**:

1. **Given** a meeting folder "Team Standup" exists and a calendar event "Team Standup - 2026-02-06" has attendees alice@example.com and bob@example.com, **When** I run `gcal-organizer organize` or `gcal-organizer sync-calendar`, **Then** the folder is shared with alice@example.com and bob@example.com with view access.

2. **Given** an attendee already has access to the folder, **When** I run the organizer, **Then** the sharing is skipped (idempotent).

3. **Given** `--dry-run` flag is provided, **When** I run the command, **Then** output shows "Would share [folder] with [email]" without actually sharing.

4. **Given** an attendee email fails validation (e.g., external domain blocked by org policy), **When** sharing fails, **Then** the error is logged and processing continues.

---

### User Story 7 - Share Attachments with Attendees (Priority: P2)

As a user, I want calendar event attachments to be automatically shared with all attendees so they can access the files linked via shortcuts in the meeting folder.

**Why this priority**: Shortcuts to files the user can't access are useless; sharing the underlying files makes the folder structure actually functional.

**Independent Test**: Can be tested by running sync-calendar on an event with an owned attachment and verifying attendees receive edit access.

**Acceptance Scenarios**:

1. **Given** a calendar event attachment that I own, **When** I run `sync-calendar`, **Then** the attachment is shared with all attendees with edit access.

2. **Given** a calendar event attachment that I do NOT have edit access to, **When** I run `sync-calendar`, **Then** the attachment sharing is skipped with a verbose log message.

3. **Given** an attendee already has access to the attachment, **When** I run `sync-calendar`, **Then** the sharing is skipped (idempotent).

4. **Given** `--dry-run` flag is provided, **When** I run the command, **Then** output shows "Would share [attachment] with [email] (writer)" without actually sharing.

---

### User Story 5 - Authentication Management (Priority: P1)

As a user, I want to authenticate with Google Workspace APIs via OAuth2 and check my authentication status.

**Why this priority**: Required before any other functionality works.

**Independent Test**: Can be tested by running `auth login` and verifying token is saved.

**Acceptance Scenarios**:

1. **Given** no token exists, **When** I run `gcal-organizer auth login`, **Then** a browser opens for OAuth flow and token is saved locally.

2. **Given** a valid token exists, **When** I run `gcal-organizer auth status`, **Then** output shows "✅ OAuth token found (authenticated)".

3. **Given** an expired token, **When** I run `gcal-organizer auth status`, **Then** output shows "⚠️ Token expired" with instructions to re-authenticate.

---

### User Story 6 - Configuration Management (Priority: P3)

As a user, I want to view my current configuration to verify settings are correct.

**Why this priority**: Troubleshooting aid, not required for core functionality.

**Independent Test**: Run `config show` and verify output matches environment.

**Acceptance Scenarios**:

1. **Given** environment variables are set, **When** I run `gcal-organizer config show`, **Then** output displays all configuration values.

2. **Given** credentials file is missing, **When** I run `gcal-organizer config show`, **Then** output shows warning with setup instructions.

---

### Edge Cases

- What happens when OAuth token is expired?
  → Prompt for re-authentication or provide refresh instructions
- What happens when rate limits are exceeded?
  → Implement exponential backoff with clear status messages
- What happens when a document is trashed during processing?
  → Log the error and continue with remaining documents
- What happens when moving an owned document that has an existing shortcut in the target folder?
  → The redundant shortcut is trashed to avoid duplicates
- What happens when an attendee is a calendar resource or group calendar (e.g. `@resource.calendar.google.com`, `@group.calendar.google.com`)?
  → Automatically skipped during folder and attachment sharing (always fails)

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST authenticate with Google Workspace APIs using OAuth2 (Drive, Docs, Calendar)
- **FR-002**: System MUST parse document names using configurable regex pattern (default: `(.+) - (\d{4}-\d{2}-\d{2})`)
- **FR-003**: System MUST create subfolders based on extracted meeting names
- **FR-004**: System MUST move owned documents and create shortcuts for non-owned documents
- **FR-005**: System MUST scan calendar events within configurable lookback period (default: 8 days)
- **FR-006**: System MUST detect Drive attachments via Calendar API and URLs in event descriptions
- **FR-007**: System MUST share meeting folders with all attendees from the corresponding calendar event (writer access)
- **FR-008**: System MUST skip sharing for attendees who already have access (idempotent)
- **FR-009**: System MUST support `--dry-run` mode with detailed output showing:
  - **FR-009a**: For document organization: file name, source location, target folder, and action (move vs shortcut)
  - **FR-009b**: For calendar sync: event title, attachment name, target folder for shortcut
  - **FR-009c**: For folder sharing: folder name, attendee emails to share with
  - **FR-009d**: Summary counts at the end (documents processed, shortcuts created, shares added)
- **FR-010**: System MUST support `--verbose` mode for detailed logging
- **FR-011**: System MUST share calendar event attachments with all attendees (writer access) when the user has edit access to the attachment
- **FR-012**: System MUST skip sharing attachments the user does not have edit access to, with a verbose log message
- **FR-013**: System MUST use `charmbracelet/log` for leveled, colorful, structured logging with `--verbose` controlling debug output
- **FR-014**: `run` command MUST accept `--days` flag to override `GCAL_DAYS_TO_LOOK_BACK` for the current execution
- **FR-015**: `gcal-organizer --version` MUST display the git tag release version, injected at build time via Go `ldflags`

### Configuration Requirements

- **CR-001**: `GCAL_MASTER_FOLDER_NAME` - Name of the master folder (string)
- **CR-002**: `GCAL_DAYS_TO_LOOK_BACK` - Calendar lookback period (integer, default: 1)
- **CR-003**: `GCAL_FILENAME_KEYWORDS` - Keywords to filter documents (comma-separated list)
- **CR-004**: `GCAL_FILENAME_PATTERN` - Regex for parsing document names (string)

### Key Entities

- **Meeting Folder**: A subfolder within the Master Folder, named after a meeting topic
- **Meeting Document**: A Google Doc matching the naming pattern
- **Calendar Event**: A Google Calendar event with potential attachments and attendees

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: CLI responds to all commands within 2 seconds (excluding API calls)
- **SC-002**: Successfully organizes 100+ documents without errors in a single run
- **SC-003**: All operations are idempotent (safe to run multiple times)
- **SC-004**: Error messages are actionable (include next steps or troubleshooting hints)
- **SC-005**: `--dry-run` mode produces identical log output without side effects

## Clarifications

*To be filled in during /speckit.clarify phase*

## Review & Acceptance Checklist

- [ ] All user stories have acceptance scenarios
- [ ] All functional requirements are testable
- [ ] Edge cases are documented
- [ ] Configuration requirements are complete
- [ ] Success criteria are measurable
- [ ] Dependencies (OAuth, API key) are documented
