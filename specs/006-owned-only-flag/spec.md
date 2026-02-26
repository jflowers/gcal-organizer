# Feature Specification: Owned-Only Mode for File Mutation Protection

**Feature Branch**: `006-owned-only-flag`  
**Created**: 2026-02-26  
**Status**: Draft  
**Input**: User description: "Add --owned-only global flag to prevent mutations on files not owned by the authenticated user, allowing only read-only operations like shortcut creation on non-owned files"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Protect Non-Owned Files During Organization (Priority: P1)

As a user who collaborates on shared documents, I want to ensure that when I run the organizer, it does not modify files I don't own — no moving, sharing, or trashing files belonging to other people. The system should still create shortcuts to those files in my own folder structure so I can find them, but leave the original files untouched.

**Why this priority**: This is the core value proposition. Users operating in shared workspaces risk unintended side effects on colleagues' files (moving them out of their expected location, sharing them with unintended recipients). Preventing mutations on non-owned files is the primary safety guarantee.

**Independent Test**: Can be tested by running `gcal-organizer organize --owned-only` against a set of documents where some are owned by the user and some are shared. Verify owned files are moved normally and non-owned files only receive shortcuts — no moves, no shares, no trashes.

**Acceptance Scenarios**:

1. **Given** a mix of owned and non-owned meeting documents, **When** I run `organize --owned-only`, **Then** owned documents are moved into their meeting folder and non-owned documents only get shortcuts created in my folder
2. **Given** a non-owned document that would normally be moved, **When** `--owned-only` is active, **Then** the document remains in its original location and a shortcut is created instead
3. **Given** `--owned-only` is not set, **When** I run `organize`, **Then** the existing behavior is preserved (non-owned files get shortcuts, owned files get moved) — no regression

---

### User Story 2 - Skip Sharing Non-Owned Calendar Attachments (Priority: P1)

As a user syncing calendar attachments, I want the system to skip the sharing step for attachments I don't own when `--owned-only` is active. The system should still create shortcuts in my folder for discoverability, but should not modify permissions on files belonging to others.

**Why this priority**: Sharing someone else's file with additional people without their knowledge is a significant privacy and governance concern. This is equally critical to the organize protection.

**Independent Test**: Run `gcal-organizer sync-calendar --owned-only` with calendar events containing attachments owned by other users. Verify shortcuts are created but no permission changes are made to non-owned attachments.

**Acceptance Scenarios**:

1. **Given** a calendar event with an attachment owned by another user, **When** I run `sync-calendar --owned-only`, **Then** a shortcut is created in my meeting folder but the file's sharing permissions are not modified
2. **Given** a calendar event with an attachment I own, **When** I run `sync-calendar --owned-only`, **Then** the file is shared with attendees as normal
3. **Given** `--owned-only` is not set, **When** I run `sync-calendar`, **Then** sharing behavior is unchanged from current behavior (shares files where user has edit access)

---

### User Story 3 - Block Task Assignment on Non-Owned Documents (Priority: P2)

As a user running task assignment, I want the system to refuse to assign tasks in documents I don't own when `--owned-only` is active. For the standalone `assign-tasks --doc` command, this should produce a clear error. For the automated `run` workflow, non-owned docs should be silently skipped with a verbose log.

**Why this priority**: Task assignment involves browser automation that modifies document content (assigning checkboxes). While important, it builds on the ownership concept established by P1 stories and is a narrower use case.

**Independent Test**: Run `assign-tasks --doc <non-owned-id> --owned-only` and verify it exits with an error. Run `gcal-organizer run --owned-only` and verify non-owned Notes documents are skipped in Step 3.

**Acceptance Scenarios**:

1. **Given** a document I do not own, **When** I run `assign-tasks --doc <id> --owned-only`, **Then** the system exits with a non-zero status and a clear error message stating the document is not owned by me
2. **Given** the `run` workflow discovers Notes documents from calendar events where some are non-owned, **When** `--owned-only` is active, **Then** non-owned documents are excluded from Step 3 task assignment processing
3. **Given** a document I own, **When** I run `assign-tasks --doc <id> --owned-only`, **Then** task assignment proceeds normally

---

### User Story 4 - Visibility Into Skipped Files (Priority: P3)

As a user running with `--owned-only`, I want clear feedback about which files were skipped due to ownership, especially in verbose mode, so I can understand what the system did and didn't do.

**Why this priority**: Observability feature that builds on top of the core protection. Users need to trust the system is working correctly and understand its decisions.

**Independent Test**: Run any command with `--owned-only --verbose` and verify that skipped files are logged with their name and owner information.

**Acceptance Scenarios**:

1. **Given** `--owned-only` and `--verbose` are both active, **When** a non-owned file is skipped, **Then** the system logs a message identifying the file name and its owner's email address
2. **Given** `--owned-only` and `--dry-run` are both active, **When** the system encounters non-owned files, **Then** the dry-run output clearly reports which files would be skipped due to ownership and what operations were suppressed
3. **Given** `--owned-only` is active without `--verbose`, **When** non-owned files are skipped, **Then** the system operates silently (no extra output) but includes a summary count of skipped files at the end

---

### User Story 5 - Persist Owned-Only as a Configuration Default (Priority: P3)

As a user who always wants owned-only protection, I want to set it once in my configuration file rather than passing the flag every time.

**Why this priority**: Convenience feature. Users who adopt this as a standard practice should not need to remember the flag on every invocation.

**Independent Test**: Set `owned-only: true` in configuration, run any command without the flag, and verify owned-only behavior is active.

**Acceptance Scenarios**:

1. **Given** `owned-only` is set to `true` in the configuration file, **When** I run any command without `--owned-only`, **Then** owned-only protection is active
2. **Given** `owned-only` is set to `true` in configuration, **When** I explicitly pass `--owned-only=false` on the command line, **Then** the CLI flag overrides the configuration and owned-only protection is disabled

---

### Edge Cases

- What happens when the ownership API call fails or returns empty owner data for a file?
  → The file is treated as non-owned (fail-safe). When `--owned-only` is active, mutations are blocked. A warning is logged in verbose mode.
- What happens with files on a Shared Drive where the "owner" is the organization, not an individual?
  → Shared Drive files have organizational ownership. They are treated as non-owned by individual users. Users who need to process Shared Drive files should not use `--owned-only`.
- What happens when a file has multiple owners (rare but possible via certain transfer scenarios)?
  → The user is considered the owner if their email appears anywhere in the owners list.
- What happens when `--owned-only` is combined with `--dry-run`?
  → Both flags work together: dry-run reports what would happen, and owned-only filtering is reflected in the dry-run output (showing which files would be skipped).
- What happens when `--owned-only` is used with `assign-tasks --doc` for a file where ownership cannot be determined?
  → The command errors and aborts (fail-safe), with a message explaining that ownership could not be verified.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST provide a global `--owned-only` flag available on all commands (`run`, `organize`, `sync-calendar`, `assign-tasks`)
- **FR-002**: When `--owned-only` is active, system MUST NOT move files the user does not own
- **FR-003**: When `--owned-only` is active, system MUST NOT trash files or shortcuts targeting files the user does not own
- **FR-004**: When `--owned-only` is active, system MUST NOT share (modify permissions on) files the user does not own
- **FR-005**: When `--owned-only` is active, system MUST still create shortcuts to non-owned files in the user's own folder structure
- **FR-006**: When `--owned-only` is active during standalone `assign-tasks --doc`, system MUST exit with a non-zero status and an error message if the specified document is not owned by the user
- **FR-007**: When `--owned-only` is active during the `run` workflow, system MUST skip non-owned documents in the task assignment step without aborting the entire workflow
- **FR-008**: When `--owned-only` is active with `--verbose`, system MUST log each skipped file with its name and owner information
- **FR-009**: When `--owned-only` is active, system MUST display a summary count of skipped files at the end of execution
- **FR-010**: System MUST treat files with missing or unresolvable ownership data as non-owned (fail-safe) when `--owned-only` is active
- **FR-011**: The `--owned-only` setting MUST be configurable via the configuration file as a persistent default
- **FR-012**: Command-line `--owned-only` flag MUST override the configuration file setting
- **FR-013**: When `--owned-only` is not set (default), all existing behavior MUST remain unchanged — no regressions
- **FR-014**: When both `--owned-only` and `--dry-run` are active, dry-run output MUST reflect the ownership filtering (showing which files would be skipped)

### Configuration Requirements

- **CR-001**: `owned-only` - Boolean setting (default: `false`) configurable via configuration file and CLI flag

### Key Entities

- **File Ownership**: Determined by comparing the authenticated user's email against the file's owners list. A user is considered the owner if their email appears in the owners list.
- **Mutation**: Any operation that modifies a file's content, location, permissions, or lifecycle (move, share, trash, annotate, assign). Shortcut creation in the user's own folder is explicitly excluded from this definition.

## Assumptions

- Ownership is determined solely by the file's `owners` metadata as returned by the file storage service. No additional ownership resolution (e.g., delegation, team ownership) is in scope.
- The existing ownership detection mechanism (comparing authenticated user email against file owners) is reliable and does not need enhancement for this feature.
- Shortcut creation is a non-destructive operation that occurs in the user's own folder and does not constitute a mutation on the target file.

## Constraints & Tradeoffs

- **Ownership vs. Edit Access for Sharing**: The existing system permits sharing a file if the user has edit access (regardless of ownership). When `--owned-only` is active, sharing is restricted to files the user owns — a stricter gate. This means a user with editor permissions on a shared file will not share it with meeting attendees when `--owned-only` is active, even though they technically could. This is an intentional tradeoff favoring safety over convenience.
- **Shared Drive Exclusion**: Shared Drive files are treated as non-owned, which means `--owned-only` effectively excludes them from all mutations. This is a known limitation; organizations that rely heavily on Shared Drives should not enable this flag. A future enhancement could introduce a separate Shared Drive policy, but that is out of scope for this feature.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With `--owned-only` active, zero mutations occur on files not owned by the authenticated user across all commands
- **SC-002**: With `--owned-only` active, all non-owned files still receive shortcuts in the user's folder structure (100% shortcut creation rate maintained)
- **SC-003**: Without `--owned-only`, all existing command behaviors produce identical results to current behavior (zero regressions)
- **SC-004**: Users can set `--owned-only` as a persistent configuration default and override it per-invocation via CLI flag
- **SC-005**: Standalone `assign-tasks --doc` with a non-owned document produces a clear, actionable error message within 5 seconds (no wasted browser automation time)
- **SC-006**: Verbose output provides sufficient information for users to understand which files were skipped and why, without requiring manual investigation
