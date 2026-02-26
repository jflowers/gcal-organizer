# Feature Specification: Browser Automation for Task Assignment

**Feature Branch**: `002-browser-task-assignment`  
**Created**: 2026-02-06  
**Status**: Implemented  
**Input**: User wants to automate native Google Docs checkbox task assignment using browser automation

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Assign Tasks to Anyone (Priority: P1)

As a user running `gcal-organizer assign-tasks --doc <id>`, I want the system to automatically assign checkbox items in the "Suggested next steps" section to the appropriate people using Google Docs' native UI.

**Why this priority**: Core functionality - the main reason for this feature

**Independent Test**: Can be tested by running against a specific document and visually confirming assignments appear with user avatars in the doc

**Acceptance Scenarios**:

1. **Given** a Google Doc with checkbox items in "Suggested next steps" mentioning "Jay Flowers", **When** I run `assign-tasks --doc <id>`, **Then** the checkbox is assigned to Jay Flowers via the native UI and shows their avatar
2. **Given** a checkbox mentioning multiple people, **When** I run `assign-tasks`, **Then** the system uses Gemini to determine the primary assignee
3. **Given** a checkbox already assigned, **When** I run `assign-tasks`, **Then** the system skips that checkbox
4. **Given** the search text matches multiple places in the doc, **When** running `assign-tasks`, **Then** the system dynamically increases search length until it finds a unique match (1 of 1)

---

### User Story 2 - Preview Mode (Priority: P2)

As a user, I want to preview what assignments would be made before the browser automation runs.

**Why this priority**: Safety feature - allows verification before making changes

**Independent Test**: Run with `--dry-run` flag and verify output shows planned assignments without opening browser

**Acceptance Scenarios**:

1. **Given** a document with assignable tasks, **When** I run `assign-tasks --doc <id> --dry-run`, **Then** I see a list of planned assignments without any browser interaction
2. **Given** dry-run output, **When** user reviews, **Then** they see assignee names and task descriptions

---

### User Story 3 - Progress Feedback (Priority: P3)

As a user, I want to see progress as tasks are being assigned.

**Why this priority**: UX improvement - helps user understand what's happening

**Independent Test**: Run command with `--verbose` and observe console output during execution

**Acceptance Scenarios**:

1. **Given** browser automation is running with `--verbose`, **When** each action occurs, **Then** output shows prefixed debug lines: KEY, FILL, HOVER, CLICK, TOOLTIP for every action
2. **Given** an assignment fails, **When** error occurs, **Then** output shows "✗ Failed: [task] - [reason]"

---

### User Story 4 - Integrated in Full Workflow (Priority: P2)

As a user running `gcal-organizer run`, I want task assignment to happen automatically as Step 3 after calendar sync.

**Why this priority**: Enables full end-to-end automation in a single command

**Acceptance Scenarios**:

1. **Given** calendar sync discovers Notes documents, **When** `gcal-organizer run` completes Steps 1 & 2, **Then** Step 3 scans each Notes doc for unassigned checkboxes and runs browser automation
2. **Given** dry-run mode, **When** `gcal-organizer run --dry-run`, **Then** Step 3 reports how many docs would be scanned without launching a browser

---

### Edge Cases

- What happens when the assignee email is not recognized by Google Docs?
  → Assignment still created, Google Docs shows unresolved email
- How does system handle checkboxes that are already assigned?
  → Skipped via `IsProcessed` flag from Docs API
- What if the document is open in another browser/user?
  → Canvas widget still appears, automation proceeds normally
- What if "Suggested next steps" section doesn't exist?
  → No checkboxes found, command exits cleanly
- What happens when Gemini returns malformed JSON?
  → Parse gracefully, log the error, skip the item
- What happens when text contains hidden control characters?
  → Dual-layer sanitization: Go-side `unicode.IsControl`/`IsPrint` + TS-side regex strip
- What happens when Chrome is installed via Flatpak and `--user-data-dir` points outside the sandbox?
  → Flatpak's sandbox blocks access to `~/.gcal-organizer/chrome-data/` by default. User must run `flatpak override --user --filesystem=~/.gcal-organizer com.google.Chrome` to grant access. `setup-browser` and `doctor` should detect this and provide the fix command.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST open a browser and navigate to the target Google Doc
- **FR-002**: System MUST use a dedicated `~/.gcal-organizer/chrome-data/` directory for browser automation, isolated from the user's default browsing profile to avoid profile lock conflicts
- **FR-003**: System MUST use Ctrl+F to navigate to checkbox text in the document
- **FR-004**: System MUST identify unchecked checkbox items in the "Suggested next steps" section via the Docs API
- **FR-005**: System MUST use Gemini to extract assignee names from each checkbox text
- **FR-006**: System MUST dynamically increase search text length until `.docs-findinput-count` shows exactly "1 of 1" match
- **FR-007**: System MUST use the hover-then-detect pattern to find the canvas-rendered "Assign as a task" widget at offset -35 from line start
- **FR-008**: System MUST enter the assignee name in the assignee input field (`input.kix-task-bubble-assignee-input-field`)
- **FR-009**: System MUST confirm assignment via Tab × 3 + Enter key sequence
- **FR-010**: System MUST skip checkboxes that already have an assignee (IsProcessed flag)
- **FR-011**: System MUST provide `--dry-run` mode to preview without browser
- **FR-012**: System MUST provide verbose debug logging showing KEY, FILL, HOVER, CLICK, TOOLTIP actions
- **FR-013**: System MUST sanitize control characters from Google Docs API text (Go-side unicode + TS-side regex)
- **FR-014**: System MUST be integrated as Step 3 in the `run` command's full workflow
- **FR-015**: CLI MUST provide `setup-browser` command to launch Chrome with debugging port and guide Google account authentication
- **FR-016**: `doctor` MUST check browser automation readiness (npm deps in `browser/`, Chrome debugging port 9222)
- **FR-017**: When Chrome is detected as a Flatpak install, `setup-browser` MUST check/guide the user to grant filesystem access via `flatpak override --user --filesystem=~/.gcal-organizer com.google.Chrome`

### Configuration Requirements

- **CR-001**: `GEMINI_API_KEY` - GCP API key for Gemini (string, required)
- **CR-002**: Chrome data directory is fixed at `~/.gcal-organizer/chrome-data/` — created by `gcal-organizer setup-browser`
- **CR-003**: On Flatpak Chrome installs, filesystem access must be granted: `flatpak override --user --filesystem=~/.gcal-organizer com.google.Chrome`

### Key Entities

- **TaskAssignment**: Checkbox text, assignee name (from Gemini), assignee email, status
- **DocumentContext**: Doc ID, title

## Implementation Details

### Hover-Then-Detect Pattern
The "Assign as a task" widget is a **canvas overlay** with zero DOM presence. The automation:
1. Presses `Home` → reads `.kix-cursor-caret` position → presses `End`
2. Hovers at `lineStart.x - 35` → detects tooltip via `[data-tooltip]` containing "assign"
3. Clicks at the tooltip position → assignee popover opens

### Dynamic Search Length
Short search text (20 chars) can match multiple places in the doc. The system reads `.docs-findinput-count` ("X of Y") and increases by 10 chars per iteration until `total === 1`.

### Dual-Layer Sanitization
- **Go side**: `unicode.IsControl` / `unicode.IsPrint` filtering in `extractParagraphText`
- **TypeScript side**: Regex `[\x00-\x1f\x7f\u200b\u200c\u200d\ufeff]` strip before Ctrl+F search

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Running `assign-tasks --doc <id>` successfully assigns all identifiable tasks ✅ (4/4 verified)
- **SC-002**: Assignee avatars appear next to checkboxes in the document ✅
- **SC-003**: Assigned users receive task notifications (via Google's native system) ✅
- **SC-004**: Dry-run mode shows accurate preview of planned assignments ✅
- **SC-005**: Dynamic search ensures unique match for all checkbox items ✅
- **SC-006**: Gemini extraction accuracy > 85% for well-formatted checkbox items ✅
