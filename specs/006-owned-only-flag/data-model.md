# Data Model: Owned-Only Mode

**Feature**: `006-owned-only-flag`
**Date**: 2026-02-26

## Entities

### Modified Entities

#### Config (internal/config/config.go)

| Field | Type | Default | Source | Notes |
|-------|------|---------|--------|-------|
| `OwnedOnly` | `bool` | `false` | CLI flag `--owned-only`, config file `owned-only`, env `GCAL_OWNED_ONLY` | New field. Controls mutation filtering by ownership. |

No other Config fields are changed.

#### Stats (internal/organizer/organizer.go)

| Field | Type | Default | Notes |
|-------|------|---------|-------|
| `Skipped` | `int` | `0` | New field. Count of non-owned files skipped due to `--owned-only`. |

Existing Stats fields (`DocumentsFound`, `DocumentsOrganized`, `ShortcutsCreated`, `EventsProcessed`, `AttachmentsLinked`, `FilesShared`, `TasksAssigned`, `TasksFailed`) remain unchanged.

### Unchanged Entities

#### Document (pkg/models/models.go)

Already has `IsOwned bool` field (line 24). No changes needed. Ownership is set during `parseDocument()` in the Drive service.

#### Attachment (pkg/models/models.go)

No ownership field. Ownership for attachments must be resolved at runtime via `drive.Service.IsFileOwned()`.

## New Methods

### drive.Service.IsFileOwned(ctx, fileID) (bool, error)

| Parameter | Type | Description |
|-----------|------|-------------|
| `ctx` | `context.Context` | Request context |
| `fileID` | `string` | Google Drive file ID to check |
| **Returns** | `(bool, error)` | `true` if authenticated user is an owner; `false` if not or on error (fail-safe) |

**Behavior**:
- Calls `Files.Get(fileID).Fields("owners").Do()`
- Iterates `file.Owners[].EmailAddress`, compares against `s.currentUserEmail`
- On API error: returns `(false, err)` — caller decides whether to skip or abort

**Rationale**: Calendar attachments and standalone `assign-tasks --doc` both need ownership checks but don't have `Document.IsOwned` available. This method provides a lightweight, single-purpose ownership check.

## State Transitions

No new state transitions. The `--owned-only` flag is a static boolean for the duration of a command execution. It does not change state during a run.

## Data Flow

```
CLI Flag / Config File / Env Var
        │
        ▼
    Config.OwnedOnly
        │
        ├──► OrganizeDocuments()
        │       │
        │       ├── doc.IsOwned == true  → MoveDocument (normal)
        │       ├── doc.IsOwned == false && !OwnedOnly → CreateShortcut (normal)
        │       └── doc.IsOwned == false && OwnedOnly  → CreateShortcut + Skip mutations + Log
        │
        ├──► SyncCalendarAttachments()
        │       │
        │       ├── OwnedOnly → IsFileOwned(att.FileID)
        │       │     ├── owned → ShareFile (normal)
        │       │     └── not owned → Skip ShareFile + Log
        │       │
        │       └── OwnedOnly → IsFileOwned(att.FileID) for notesDocIDs
        │             ├── owned → Collect for Step 3
        │             └── not owned → Exclude from Step 3
        │
        └──► assign-tasks --doc (standalone)
                │
                ├── OwnedOnly → IsFileOwned(docID)
                │     ├── owned → Proceed normally
                │     └── not owned → Error + Exit non-zero
                │
                └── !OwnedOnly → Proceed normally (no check)
```
