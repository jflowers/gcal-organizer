## Context

gcal-organizer's Step 2 (`SyncCalendarAttachments`) creates shortcuts in per-meeting folders for each calendar event attachment. The shortcut name is derived from `att.Title` -- the title field from the Calendar API's attachment object. For most attachments this is fine, but for Google's auto-generated "Notes by Gemini" documents, the Calendar API always returns the generic title "Notes by Gemini" even though the actual Drive file has a descriptive name like "Tanya / Joy 1-1 - 2026/02/12 13:55 EST - Notes by Gemini".

The `GetFileName()` Drive API call exists and is already used as a fallback when `att.Title` is empty, but it is never reached for Gemini notes because the Calendar API always populates the title field.

## Goals / Non-Goals

**Goals:**
- Shortcuts are named after the actual Drive file, not the calendar attachment title
- Drive API calls for file names are cached per file ID within a single run to avoid redundant requests
- Existing shortcut behavior is preserved for attachments where the title already matches the file name

**Non-Goals:**
- Renaming existing shortcuts that were already created with "Notes by Gemini" -- users can re-run the organizer and the shortcut-exists check will skip duplicates
- Changing how transcripts or other attachment types are named -- only the title resolution order changes, which is transparent

## Decisions

### D1: Always prefer Drive file name over attachment title

**Decision**: Call `GetFileName()` first for every attachment. Use its result as the shortcut name. Fall back to `att.Title` only if the API call fails. Fall back to a truncated file ID if both are unavailable.

**Rationale**: The Drive file name is the authoritative name set by the document creator (Google or human). The calendar attachment title is a label that may be generic or stale. Preferring the Drive name produces more descriptive, distinguishable shortcuts.

**Alternatives considered**:
- Only call `GetFileName()` when `att.Title` matches a known generic pattern (e.g., "Notes by Gemini"): rejected because this is fragile -- new generic patterns would require code changes. The unconditional approach is simpler and handles all cases.
- Construct a name from the event title + date: rejected because this invents a name rather than using the actual file name, which may confuse users who expect the shortcut to match the file.

### D2: Cache GetFileName results per file ID

**Decision**: Use the existing `ownershipCache` pattern -- maintain a `map[string]string` for file name lookups within `SyncCalendarAttachments`. If a file ID has been resolved, reuse the cached name.

**Rationale**: The same file ID can appear in multiple calendar events (e.g., a recurring meeting's notes). Caching avoids redundant Drive API calls and keeps the performance impact proportional to unique files, not total attachments.

## Risks / Trade-offs

- **[Additional API calls]** → Each unique attachment file ID triggers one `GetFileName()` call that wasn't made before. For a typical run with 5-20 unique attachments, this adds 5-20 API calls (well within quota). Mitigated by per-file-ID caching.
- **[File name may differ from expected]** → If a user has manually renamed a Drive file, the shortcut will use the current name, not the original. This is correct behavior (the shortcut should match the file) but may surprise users who expected the old name.
- **[GetFileName failure]** → If the Drive API call fails (permissions, network), the system falls back to `att.Title`, preserving the current behavior. No degradation beyond the existing behavior.
