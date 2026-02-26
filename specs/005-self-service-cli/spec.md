# Feature Specification: Self-Service CLI

**Feature Branch**: `005-self-service-cli`  
**Created**: 2026-02-08  
**Status**: Implemented  
**Input**: Make the CLI self-bootstrapping with actionable errors, eliminating documentation as a dependency for setup

## User Scenarios & Testing *(mandatory)*

### User Story 1 - First Run Setup (Priority: P0)

As a new user who just downloaded the binary, I want `gcal-organizer init` to guide me through setup so I don't need to read docs to get started.

**Why this priority**: Eliminates the #1 onboarding friction — reading SETUP.md before doing anything

**Independent Test**: Download a fresh binary, run `gcal-organizer init`, follow prompts, then `gcal-organizer run --dry-run` succeeds

**Acceptance Scenarios**:

1. **Given** `~/.gcal-organizer/` does not exist, **When** I run `gcal-organizer init`, **Then** it creates the directory, generates `.env` template, and prompts for `GEMINI_API_KEY`
2. **Given** I provide a valid API key, **When** init continues, **Then** it writes the key to `.env`
3. **Given** init completes, **When** I run `gcal-organizer run --dry-run`, **Then** config loads correctly from `~/.gcal-organizer/.env`
4. **Given** `~/.gcal-organizer/` already exists, **When** I run `gcal-organizer init`, **Then** it skips existing files and only creates missing ones
5. **Given** I pass `--non-interactive`, **When** I run `gcal-organizer init`, **Then** it uses defaults without prompting

---

### User Story 2 - Diagnostics (Priority: P0)

As a user encountering errors, I want `gcal-organizer doctor` to tell me exactly what's wrong and how to fix it.

**Why this priority**: Most support burden comes from misconfiguration — this eliminates it

**Independent Test**: Remove credentials.json and run `gcal-organizer doctor` — verify it reports the missing file with exact fix instructions

**Acceptance Scenarios**:

1. **Given** all prerequisites are met, **When** I run `gcal-organizer doctor`, **Then** I see all green checkmarks
2. **Given** credentials.json is missing, **When** I run `gcal-organizer doctor`, **Then** it reports the issue with a link to the setup guide
3. **Given** GEMINI_API_KEY is not set, **When** I run `gcal-organizer doctor`, **Then** it tells me to set it in `~/.gcal-organizer/.env`
4. **Given** Node.js is not installed, **When** I run `gcal-organizer doctor`, **Then** it warns that Step 3 (task assignment) will be unavailable
5. **Given** the OAuth token is expired, **When** I run `gcal-organizer doctor`, **Then** it tells me to run `gcal-organizer auth login`

---

### User Story 3 - Service Install via CLI (Priority: P1)

As a user, I want `gcal-organizer install` to set up the hourly service without needing Make.

**Why this priority**: Makefile is a developer tool; end users should use the binary

**Independent Test**: Run `gcal-organizer install`, verify the service is active with `gcal-organizer doctor`

**Acceptance Scenarios**:

1. **Given** I'm on macOS, **When** I run `gcal-organizer install`, **Then** it installs the LaunchAgent and starts the service
2. **Given** I'm on Linux, **When** I run `gcal-organizer install`, **Then** it installs the systemd user timer
3. **Given** the service is already installed, **When** I run `gcal-organizer install`, **Then** it is idempotent (reinstalls cleanly)
4. **Given** I want to remove it, **When** I run `gcal-organizer uninstall`, **Then** it stops and removes all service files

---

### User Story 4 - Actionable Errors (Priority: P0)

As a user hitting an error, I want the error message to tell me exactly what to do so I can fix it myself.

**Why this priority**: Every confusing error is a potential user abandonment

**Acceptance Scenarios**:

1. **Given** credentials.json is missing, **When** any command runs, **Then** the error includes the fix: `Run 'gcal-organizer init' or download from https://console.cloud.google.com/apis/credentials`
2. **Given** GEMINI_API_KEY is empty, **When** any command runs, **Then** the error includes: `Set GEMINI_API_KEY in ~/.gcal-organizer/.env or run 'gcal-organizer init'`
3. **Given** Node.js is not found, **When** assign-tasks runs, **Then** the error includes: `Install Node.js 18+ from https://nodejs.org`
4. **Given** OAuth token is expired, **When** any API call fails, **Then** the error includes: `Run 'gcal-organizer auth login' to re-authenticate`
5. **Given** Chrome data directory is not found, **When** assign-tasks runs, **Then** the error includes: `Run 'gcal-organizer setup-browser' to create it`
6. **Given** Chrome is installed via Flatpak without filesystem access to `~/.gcal-organizer/`, **When** `setup-browser` runs, **Then** it detects the sandbox restriction and provides the fix: `flatpak override --user --filesystem=~/.gcal-organizer com.google.Chrome`

---

### Edge Cases

- What if `init` is interrupted mid-way?
  → Partial files are left in place; re-running `init` fills in the gaps (idempotent)
- What if the user runs `install` before `init`?
  → `install` checks prerequisites and runs `init` first if needed
- What if Node.js is installed after initial setup?
  → `doctor` will detect it; no reconfig needed since Steps 1-2 work without it
- What if the config directory has wrong permissions?
  → `doctor` checks and reports with fix: `chmod 700 ~/.gcal-organizer`
- What if Chrome is installed via Flatpak and can't access `~/.gcal-organizer/chrome-data/`?
  → Flatpak sandboxes Chrome by default. `setup-browser` and `doctor` detect Flatpak installs and guide user to run `flatpak override --user --filesystem=~/.gcal-organizer com.google.Chrome`

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `gcal-organizer init` MUST create `~/.gcal-organizer/` and generate a `.env` file
- **FR-002**: `gcal-organizer init` MUST prompt for GEMINI_API_KEY interactively (or accept `--api-key` flag)
- **FR-003**: `gcal-organizer init` MUST create `~/.gcal-organizer/` directory structure for the current OS
- **FR-004**: `gcal-organizer init` MUST support `--non-interactive` flag for scripted use
- **FR-005**: `gcal-organizer doctor` MUST check: config dir, credentials, token validity, API key, Node.js, Chrome data directory, service status
- **FR-006**: `gcal-organizer doctor` MUST output actionable fix commands for each issue
- **FR-007**: `gcal-organizer install` MUST detect OS and install the appropriate service (launchd/systemd)
- **FR-008**: `gcal-organizer install` MUST be idempotent
- **FR-009**: `gcal-organizer uninstall` MUST stop and remove all service files
- **FR-010**: All error messages MUST include a `Fix:` line with exact resolution steps
- **FR-011**: `gcal-organizer init` MUST be idempotent — skip existing files, fill in missing ones
- **FR-012**: Self-service commands SHOULD use `charmbracelet/huh` for interactive prompts (API key input)
- **FR-013**: Self-service commands SHOULD use `charmbracelet/lipgloss` for styled terminal output
- **FR-014**: User-facing error messages MUST include a reference to `gcal-organizer doctor` for diagnostics
- **FR-015**: `gcal-organizer setup-browser` MUST create/use `~/.gcal-organizer/chrome-data/`, launch Chrome with `--user-data-dir` and `--remote-debugging-port=9222`, guide first-run Google sign-in, and verify CDP connectivity
- **FR-016**: `doctor` and `init` MUST use `charmbracelet/lipgloss` styled summary boxes for polished output
- **FR-017**: `setup-browser` MUST detect Flatpak Chrome installs and check/prompt for `flatpak override --user --filesystem=~/.gcal-organizer com.google.Chrome` before launching
- **FR-018**: `doctor` MUST detect Flatpak Chrome and warn if filesystem access has not been granted to `~/.gcal-organizer/`

### Non-Functional Requirements

- **NFR-001**: `init` completes in under 2 seconds (excluding OAuth flow)
- **NFR-002**: `doctor` completes in under 3 seconds
- **NFR-003**: Error messages use consistent formatting with emoji indicators (✅ ⚠️ ❌)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A user can go from binary download to first successful `run --dry-run` using only CLI commands (no docs)
- **SC-002**: `gcal-organizer doctor` identifies 100% of common misconfiguration issues
- **SC-003**: Every error reachable by end users includes an actionable fix
- **SC-004**: Service install/uninstall works without Makefile or build tools
