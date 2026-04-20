## ADDED Requirements

### Requirement: Shortcuts use Drive file name
When creating a shortcut for a calendar event attachment, the system SHALL use the actual Google Drive file name (retrieved via `GetFileName()`) as the shortcut name, rather than the calendar attachment title.

#### Scenario: Gemini notes shortcut uses Drive file name
- **WHEN** a calendar event has an attachment titled "Notes by Gemini" with a Drive file named "Sprint Planning - 2026/04/11 09:00 EST - Notes by Gemini"
- **THEN** the shortcut is created with the name "Sprint Planning - 2026/04/11 09:00 EST - Notes by Gemini"

#### Scenario: Transcript shortcut retains correct name
- **WHEN** a calendar event has an attachment titled "Sprint Planning - Transcript" and the Drive file has the same name
- **THEN** the shortcut is created with the name "Sprint Planning - Transcript" (no change in behavior)

#### Scenario: GetFileName API failure falls back to attachment title
- **WHEN** a calendar event has an attachment titled "Notes by Gemini" and the `GetFileName()` API call fails
- **THEN** the shortcut is created with the name "Notes by Gemini" (fallback to attachment title)

#### Scenario: Both title and GetFileName unavailable
- **WHEN** a calendar event has an attachment with an empty title and the `GetFileName()` API call fails
- **THEN** the shortcut is created with a truncated file ID as the name (e.g., "attachment (abc12345...)")

### Requirement: File name lookups are cached per run
The system SHALL cache the results of `GetFileName()` calls per file ID within a single `SyncCalendarAttachments` execution to avoid redundant Drive API requests.

#### Scenario: Same file ID across multiple events
- **WHEN** two calendar events reference the same attachment file ID
- **THEN** `GetFileName()` is called only once for that file ID and the cached result is reused for the second event

#### Scenario: Cache does not persist across runs
- **WHEN** the user runs `gcal-organizer run` twice
- **THEN** each run starts with a fresh cache (no stale file names from previous runs)
