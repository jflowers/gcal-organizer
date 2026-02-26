# Feature Specification: Hourly Service Deployment

**Feature Branch**: `004-service-deployment`  
**Created**: 2026-02-07  
**Status**: Implemented  
**Input**: Run gcal-organizer hourly as a service on Fedora and macOS with 1-day lookback

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Hourly Scheduling on macOS (Priority: P1)

As a macOS user, I want gcal-organizer to run automatically every hour so my meeting documents, calendar attachments, and task assignments stay organized without manual intervention.

**Why this priority**: Primary development platform

**Independent Test**: Install the service, wait 1 hour, check logs for a successful run

**Acceptance Scenarios**:

1. **Given** the service is installed via `gcal-organizer install` (or `make install-service`), **When** 1 hour elapses, **Then** `gcal-organizer run` executes with 1-day lookback and logs output
2. **Given** the service is running, **When** I run `gcal-organizer doctor` (or `make service-status`), **Then** I see the current state and last run time
3. **Given** I want to stop it, **When** I run `gcal-organizer uninstall` (or `make uninstall-service`), **Then** the service is removed cleanly

---

### User Story 2 - Hourly Scheduling on Fedora (Priority: P1)

As a Fedora user, I want the same hourly scheduling via systemd user services.

**Why this priority**: Secondary platform, equal importance

**Independent Test**: Install the systemd timer, wait 1 hour, check `journalctl --user` for output

**Acceptance Scenarios**:

1. **Given** the service is installed via `gcal-organizer install` (or `make install-service`), **When** 1 hour elapses, **Then** `gcal-organizer run` executes with 1-day lookback and logs output
2. **Given** the service is running, **When** I run `systemctl --user status gcal-organizer.timer`, **Then** I see the timer state and next trigger time
3. **Given** I want to stop it, **When** I run `gcal-organizer uninstall` (or `make uninstall-service`), **Then** the systemd units are disabled and removed

---

### User Story 3 - Log Output (Priority: P2)

As a user, I want to review logs from previous runs to troubleshoot issues.

**Why this priority**: Debugging aid

**Acceptance Scenarios**:

1. **Given** the service has run, **When** I check logs, **Then** output includes timestamps and full workflow summary
2. **On macOS**: Logs available via `log show --predicate 'subsystem=="com.jflowers.gcal-organizer"'` or log file
3. **On Fedora**: Logs available via `journalctl --user -u gcal-organizer.service`

---

### Edge Cases

- What happens when the browser automation (task assignment) runs without a display?
  → On macOS, launchd runs in the user session and has access to the GUI. On Fedora, the systemd user service runs in the user session; if no display is available, Step 3 (assign-tasks) is skipped gracefully
- What happens when the machine is asleep during a scheduled run?
  → macOS launchd will run the job when the machine wakes. systemd timers with `Persistent=true` catch up on missed runs
- What happens when credentials expire?
  → The service logs the auth error and exits; user re-authenticates manually
- What happens on network failure?
  → API errors are logged, service exits non-zero, runs again next hour
- What happens when the log file grows too large?
  → The run wrapper rotates the log file when it exceeds the size limit, keeping one backup (`.1`) so at most 2× the limit is used

---

### User Story 4 - Homebrew Install (Priority: P1)

As a macOS or Linux user, I want to install gcal-organizer via Homebrew so I get the binary, dependencies, service, and man page in one command.

**Why this priority**: Homebrew is the standard package manager for macOS and widely used on Linux

**Independent Test**: Run `brew install` from the formula, verify binary, man page, and `brew services start` all work

**Acceptance Scenarios**:

1. **Given** the tap is configured, **When** I run `brew install gcal-organizer`, **Then** the binary, man page, and browser dependencies are installed
2. **Given** gcal-organizer is installed via brew, **When** I run `brew services start gcal-organizer`, **Then** the hourly service starts (launchd on macOS, systemd on Linux)
3. **Given** I want to remove it, **When** I run `brew uninstall gcal-organizer`, **Then** the binary, man page, and dependencies are removed cleanly

---

### User Story 5 - Man Page (Priority: P2)

As a user, I want to read `man gcal-organizer` for offline reference of all commands, flags, and configuration.

**Why this priority**: Standard CLI convention for discoverability

**Acceptance Scenarios**:

1. **Given** gcal-organizer is installed, **When** I run `man gcal-organizer`, **Then** I see a formatted manual covering commands, flags, env vars, files, and examples


## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Service MUST run `gcal-organizer run` with `GCAL_DAYS_TO_LOOK_BACK=1` every hour
- **FR-002**: Service MUST use the user's existing OAuth token and credentials
- **FR-003**: macOS service MUST be a launchd user agent (LaunchAgent)
- **FR-004**: Fedora service MUST be a systemd user service with timer
- **FR-005**: Service MUST log output with timestamps for troubleshooting
- **FR-006**: Service MUST provide install/uninstall via `gcal-organizer install`/`uninstall` commands (Makefile targets retained for developers)
- **FR-007**: Service MUST catch up on missed runs (macOS wake, systemd Persistent=true)
- **FR-008**: Homebrew formula MUST build from source and declare `node` as a runtime dependency
- **FR-009**: Homebrew formula caveats MUST reference all self-service commands (`init`, `auth login`, `setup-browser`, `doctor`, `install`)
- **FR-010**: Man page MUST be installed to `man1` and cover all commands, flags, env vars, and files
- **FR-011**: Homebrew formula MUST install the browser automation scripts and their npm dependencies
- **FR-012**: Release workflow MUST attach the Homebrew formula to the GitHub Release
- **FR-013**: A Homebrew tap (`jflowers/homebrew-gcal-organizer`) MUST exist for `brew tap` + `brew install`
- **FR-014**: Release workflow MUST auto-publish the updated formula to the tap on every tagged release
- **FR-015**: Pre-compiled bottles MUST be built for macOS (arm64) and Linux (x86_64) on release
- **FR-016**: On macOS, the run wrapper MUST rotate the log file when it exceeds 5 MB, keeping at most one rotated backup (`.1`)
- **FR-017**: On Linux, log rotation is handled by journalctl and requires no additional configuration

### Configuration Requirements

- **CR-001**: Environment variables loaded from `~/.gcal-organizer/.env` or system environment
- **CR-002**: Binary path auto-detected or configurable

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Service runs successfully every hour without manual intervention
- **SC-002**: `make install-service` completes in under 5 seconds
- **SC-003**: Logs from the last 24 hours are easily retrievable
- **SC-004**: Service survives system reboot (auto-starts on login)
- **SC-005**: `brew install gcal-organizer` installs binary, man page, and browser deps
- **SC-006**: `gcal-organizer install` starts the hourly service (replaces `brew services`)
- **SC-007**: `man gcal-organizer` renders the full manual
- **SC-008**: `brew tap jflowers/gcal-organizer` taps the formula successfully
- **SC-009**: Bottles are available for macOS and Linux after a release
- **SC-010**: Log disk usage MUST NOT exceed 10 MB (5 MB active + 5 MB rotated backup) on macOS

