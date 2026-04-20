## Why

When gcal-organizer creates shortcuts in meeting folders, it names them using the calendar attachment title (`att.Title`) rather than the actual Drive file name. For Google's auto-generated meeting notes, the attachment title is always "Notes by Gemini", but the actual file in Drive has a descriptive name like "Tanya / Joy 1-1 - 2026/02/12 13:55 EST - Notes by Gemini". This makes all Gemini notes shortcuts indistinguishable when browsing meeting folders. (GitHub issue #15)

## What Changes

- Always fetch the actual Drive file name via `GetFileName()` when creating shortcuts for calendar attachments, instead of using the generic attachment title
- Fall back to `att.Title` only when the Drive API call fails
- Cache `GetFileName()` results per file ID to avoid redundant API calls (the same file ID may appear across events)

## Capabilities

### New Capabilities

- `shortcut-file-naming`: Use the actual Google Drive file name for shortcut creation instead of the calendar attachment title

### Modified Capabilities

## Impact

- `internal/organizer/organizer.go`: Change the attachment title resolution logic in `SyncCalendarAttachments()` (lines 439-448) to always call `GetFileName()` first
- `internal/organizer/organizer_test.go`: Update tests to verify `GetFileName()` is called and its result is used for shortcut names
- One additional Drive API call per unique attachment file ID (mitigated by caching)
- No breaking changes -- shortcuts that already have correct names (e.g., transcripts where `att.Title` matches the file name) are unaffected
