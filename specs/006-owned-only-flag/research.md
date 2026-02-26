# Research: Owned-Only Mode for File Mutation Protection

**Feature**: `006-owned-only-flag`
**Date**: 2026-02-26

## Research Questions

### R1: How does the existing codebase determine file ownership?

**Decision**: Use the existing `Document.IsOwned` field, which is set in `drive.Service.parseDocument()` by comparing `file.Owners[].EmailAddress` against `s.currentUserEmail`.

**Rationale**: The ownership detection mechanism is already implemented and reliable. The Google Drive API v3 `owners` field provides definitive ownership information. The `currentUserEmail` is obtained during `drive.NewService()` from the Drive About API, which returns the authenticated user's email.

**Alternatives considered**:
- `CanEditFile()` — checks edit capability, not ownership. Too broad: editors who are not owners would pass.
- Querying `permissions` list — more API calls, more complex, returns roles not ownership.
- Comparing against a configured email — fragile, requires manual configuration, may mismatch OAuth identity.

### R2: Where is ownership data available vs. missing?

**Decision**: Ownership is available in the `OrganizeDocuments()` path (via `Document.IsOwned`) but missing in the `SyncCalendarAttachments()` path (calendar `Attachment` model has no ownership field). A new `IsFileOwned()` Drive service method is needed for the calendar/assign-tasks paths.

**Rationale**: The `ListMeetingDocuments()` API call already requests the `owners` field and populates `IsOwned` during `parseDocument()`. However, calendar event attachments are resolved via attachment URLs, and the `Attachment` struct (`pkg/models/models.go:107-119`) contains only `FileID`, `Title`, `MimeType`, and `IconLink` — no ownership data. An additional Drive API call (`Files.Get(fileID).Fields("owners")`) is required per attachment.

**Alternatives considered**:
- Batch API calls to reduce overhead — Google Drive API supports batching, but the complexity is not justified for the typical number of attachments per sync (usually 1-5 per event).
- Cache ownership lookups — premature optimization; the number of calls is small.
- Skip ownership filtering for calendar attachments entirely — violates FR-004 (must not share non-owned files).

### R3: What is the best pattern for adding a new global boolean flag?

**Decision**: Follow the exact pattern used by `--dry-run` and `--verbose`: package-level variable, `PersistentFlags().BoolVar()`, `viper.BindPFlag()`, and explicit assignment in each command's `RunE`.

**Rationale**: Consistency with existing codebase. The `--dry-run` pattern is proven and well-understood:
1. `var dryRun bool` at package level (`main.go:38`)
2. `rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, ...)` in `init()` (`main.go:242`)
3. `viper.BindPFlag("dry-run", ...)` in `init()` (`main.go:246`)
4. `cfg.DryRun = dryRun` in each command's `RunE` (`main.go:92`, etc.)

This dual-path approach (viper for config file, explicit assignment for CLI override) ensures CLI flags always take precedence.

**Alternatives considered**:
- Viper-only (no explicit assignment) — would work but breaks the established pattern and makes precedence less predictable.
- Separate `--owned-only` flags per command — unnecessary complexity; the flag is global by design.

### R4: How should ownership checks interact with the existing `CanEditFile()` gate on sharing?

**Decision**: When `--owned-only` is active, the ownership check replaces `CanEditFile()` as the gate for sharing. When `--owned-only` is not active, `CanEditFile()` remains the gate (preserving existing behavior).

**Rationale**: `CanEditFile()` checks `capabilities.canEdit`, which returns `true` for owners AND editors. The `--owned-only` flag specifically restricts mutations to files the user owns. Using `CanEditFile()` as the gate would still allow editors (non-owners) to share, which violates FR-004.

**Alternatives considered**:
- Stack both checks (`IsOwned AND CanEditFile`) — redundant; owners always have edit capability.
- Replace `CanEditFile()` globally with ownership — too aggressive; would change behavior even without `--owned-only`.

### R5: How should `assign-tasks --doc` handle ownership with explicit doc ID?

**Decision**: When `--owned-only` is active and the user provides a non-owned doc ID via `--doc`, exit with a non-zero status and a clear error message. This is a hard error, not a skip.

**Rationale**: The standalone `assign-tasks --doc <id>` command is an explicit user action targeting a specific document. Silently skipping would be confusing — the user explicitly asked to process this document. A clear error with guidance is the appropriate response. This aligns with FR-006.

**Alternatives considered**:
- Warn and proceed anyway — undermines the purpose of `--owned-only`.
- Warn and skip (exit 0) — ambiguous success status; user may not notice the skip.
- Prompt for confirmation — not appropriate for a CLI tool that may be automated.

### R6: Performance impact of additional API calls for calendar attachment ownership

**Decision**: Accept the overhead of one `Files.Get(fileID).Fields("owners")` call per calendar attachment when `--owned-only` is active. No performance mitigation needed.

**Rationale**: Calendar sync typically processes 1-10 events per run (configurable via `--days`, default 1 day). Each event has 0-3 attachments. The additional ownership check is a lightweight Drive API read (~50-100ms per call). Total overhead: 0-3 seconds per sync, which is negligible compared to the existing sharing operation it may skip (which itself makes API calls).

**Alternatives considered**:
- Parallel ownership checks — unnecessary complexity for 1-10 calls.
- Batch API request — Google Drive batch API adds implementation complexity for minimal gain.
- Skip ownership check for calendar path — violates spec requirements.
