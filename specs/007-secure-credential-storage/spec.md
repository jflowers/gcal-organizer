# Feature Specification: Secure Credential Storage

**Feature Branch**: `007-secure-credential-storage`  
**Created**: 2026-02-26  
**Status**: Draft  
**Input**: User concern about auth/token information stored in cleartext

---

## Problem Statement

All sensitive credentials are currently stored as **plaintext files** in `~/.gcal-organizer/`:

| File | Contents | Risk |
|------|----------|------|
| `token.json` | OAuth2 refresh + access tokens | Full Google Workspace access |
| `credentials.json` | OAuth2 client ID + client secret | Application impersonation |
| `.env` | Gemini API key | API billing abuse |

Any process or user with read access to `~/.gcal-organizer/` can exfiltrate these credentials. On a shared machine, or if the home directory is backed up unencrypted, this is a significant security gap.

Additionally, when the system refreshes an expired access token during a workflow run, the refreshed token is not persisted — it is lost when the process exits. This forces an unnecessary re-authentication round-trip on the next invocation.

## Clarifications

### Session 2026-02-26

- Q: What should happen when `credentials.json` deletion prompt cannot be displayed (non-interactive context such as cron, launchd, CI)? → A: Skip deletion in non-interactive mode; log that manual cleanup is needed. The credentials are still stored in the credential store for security benefit.
- Q: When migrating secrets from `.env`, which lines should be removed? → A: Remove both `GEMINI_API_KEY` and `GOOGLE_CREDENTIALS_FILE` lines, since both secrets are now in the credential store. Leave remaining non-secret configuration values intact.

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Secure Token Storage (Priority: P1)

As a user, I want my OAuth tokens stored in the operating system's credential store so that they are encrypted at rest and not readable by other processes.

**Why this priority**: The OAuth refresh token grants full Google Workspace access (Drive, Calendar, Docs, Tasks). This is the highest-value secret in the system.

**Independent Test**: Run the login command, then verify the token is retrievable from the OS credential store and that `token.json` no longer exists on disk.

**Acceptance Scenarios**:

1. **Given** a fresh install, **When** the user runs the login flow, **Then** the OAuth token is stored in the OS credential store (not as a plaintext file on disk).
2. **Given** a token already in the credential store, **When** the workflow runs, **Then** it retrieves the token from the credential store without user interaction.
3. **Given** a headless service (e.g., scheduled background job), **When** the workflow runs, **Then** it can access the credential store token without a GUI prompt.
4. **Given** the access token expires during a workflow run, **When** the system refreshes it, **Then** the refreshed token is persisted back to the credential store (not lost on exit).

---

### User Story 2 — Secure API Key Storage (Priority: P2)

As a user, I want my Gemini API key stored in the OS credential store instead of a plaintext `.env` file.

**Why this priority**: The API key allows billing-impacting usage of the Gemini API. Lower risk than OAuth tokens but still sensitive.

**Independent Test**: Run the setup/init flow, provide the API key, then verify it is retrievable from the OS credential store and no longer appears in `.env`.

**Acceptance Scenarios**:

1. **Given** the user runs the initial setup, **When** they enter a Gemini API key, **Then** it is saved to the credential store under a well-known service/key name.
2. **Given** an API key in the credential store, **When** configuration loads, **Then** it retrieves the key from the credential store, falling back to environment variables if not found.

---

### User Story 3 — Secure Client Credentials Storage (Priority: P2)

As a user, I want the OAuth client credentials (`credentials.json`) stored in the OS credential store so that the file does not need to remain on disk.

**Why this priority**: The client credentials file contains the OAuth client secret. While it alone cannot access user data (requires user consent), it enables application impersonation and should be protected.

**Independent Test**: After the login flow completes, verify the client credentials are retrievable from the credential store. The original file should only be removed after explicit user confirmation.

**Acceptance Scenarios**:

1. **Given** a `credentials.json` file on disk, **When** the login flow runs, **Then** the system stores the file contents in the credential store.
2. **Given** the client credentials are in the credential store, **When** the system needs them for authentication, **Then** it reads from the credential store first, falling back to the file on disk.
3. **Given** the client credentials have been stored in the credential store, **When** migration completes, **Then** the system prompts the user before deleting `credentials.json` (since it may be shared with other tools).

---

### User Story 4 — Migration from Plaintext (Priority: P2)

As an existing user upgrading to this version, I want my plaintext secrets automatically migrated to the credential store without manual intervention.

**Why this priority**: Existing installs need an upgrade path. Without it, users remain on plaintext storage forever.

**Independent Test**: With existing `token.json`, `.env`, and `credentials.json` on disk, run any command and verify secrets are moved to the credential store, plaintext files cleaned up (with appropriate prompting for shared files), and subsequent runs work without errors.

**Acceptance Scenarios**:

1. **Given** an existing `token.json` on disk, **When** the system starts, **Then** it auto-migrates the token to the credential store and deletes `token.json`.
2. **Given** a Gemini API key and credentials file path in `.env`, **When** the system starts, **Then** it migrates the API key to the credential store and removes both the `GEMINI_API_KEY` and `GOOGLE_CREDENTIALS_FILE` entries from `.env` (preserving all other configuration values).
3. **Given** an existing `credentials.json` on disk, **When** migration runs, **Then** it stores the contents in the credential store and **prompts the user** before deleting the file.
4. **Given** migration has already completed, **When** the system starts again, **Then** no migration occurs and no errors are logged (idempotent).
5. **Given** migration runs in a non-interactive context (cron, launchd, CI), **When** `credentials.json` exists on disk, **Then** the system stores the contents in the credential store but skips the deletion prompt, logging that manual cleanup is needed.

---

### User Story 5 — Fallback for Headless/No-Credential-Store Environments (Priority: P3)

As a user running on a headless server without an OS credential store, I want the tool to gracefully fall back to file-based storage so it continues to work.

**Why this priority**: Not all environments have a credential store available (CI runners, minimal server installs, containers). The tool must not break in these scenarios.

**Independent Test**: Disable or remove the OS credential store, run the tool, verify it logs a warning and falls back to file-based storage with current behavior preserved.

**Acceptance Scenarios**:

1. **Given** no OS credential store is available, **When** the tool starts, **Then** it warns the user and falls back to file-based storage (current behavior).
2. **Given** a flag or environment variable to opt out of credential store usage, **When** secrets are loaded, **Then** file-based storage is used regardless of credential store availability.

---

### Edge Cases

- What happens if the credential store is **locked** (e.g., macOS Keychain locked, Linux keyring locked)?
  - The system should detect the locked state and either prompt the user (interactive) or fall back to file-based storage (non-interactive/headless) with a warning.
- What happens if the user **denies** credential store access on macOS?
  - Treat as "no credential store available" — fall back to file-based storage with a warning.
- How does the system handle token **refresh** persistence?
  - When the underlying OAuth2 library refreshes an expired access token, the new token must be saved back to the credential store immediately (not just held in memory).
- What if `credentials.json` is **shared** across multiple projects?
  - Never auto-delete `credentials.json`. Always prompt the user before removing it, explaining that other tools may depend on it. In non-interactive contexts, skip the deletion entirely and log that manual cleanup is needed.
- What if migration is **interrupted** (e.g., crash after storing in credential store but before deleting file)?
  - The system must be idempotent: if a secret exists in both the credential store and on disk, it should treat the credential store version as authoritative and re-attempt cleanup on the next run.
- What if the credential store has a **size limit** on stored values?
  - The `credentials.json` blob is typically under 1 KB. OAuth tokens are under 2 KB. These are well within typical credential store limits. If a store rejects the value, fall back to file-based storage for that specific secret with a warning.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST store OAuth refresh and access tokens in the OS credential store by default.
- **FR-002**: System MUST store the Gemini API key in the OS credential store by default.
- **FR-003**: System MUST store the OAuth client credentials (`credentials.json` contents) in the OS credential store by default.
- **FR-004**: System MUST auto-migrate existing plaintext secrets to the credential store on first run after upgrade (transparent to the user).
- **FR-005**: System MUST delete `token.json` after successful migration. System MUST remove both the `GEMINI_API_KEY` and `GOOGLE_CREDENTIALS_FILE` entries from `.env` after successful migration (preserving all other configuration values in the file). System MUST **prompt the user before deleting** `credentials.json` since it may be shared with other tools.
- **FR-006**: System MUST gracefully fall back to file-based storage when no credential store is available, logging a warning to the user.
- **FR-007**: System MUST support a CLI flag and environment variable to opt out of credential store usage, forcing file-based storage.
- **FR-008**: System MUST persist refreshed OAuth tokens back to the credential store (not only to memory) whenever the access token is renewed during a workflow run.
- **FR-009**: The system health check command MUST report whether secrets are stored in the OS credential store or in plaintext files.
- **FR-010**: System MUST read credentials from the credential store first, then fall back to file-based sources (environment variables, `.env` file, file on disk) if not found.
- **FR-011**: Migration MUST be idempotent — running it multiple times produces the same result with no errors or data loss.
- **FR-012**: System MUST NOT auto-delete `credentials.json` under any circumstances. Deletion requires explicit user confirmation via an interactive prompt. In non-interactive contexts (e.g., cron, launchd, CI), the system MUST skip the deletion prompt and log that manual cleanup is needed.

### Configuration Requirements

- **CR-001**: The opt-out flag MUST be available as both a persistent CLI flag and an environment variable.
- **CR-002**: The credential store service name MUST use a reverse-DNS identifier consistent with the application's existing service identifiers.

### Key Entities

- **SecretStore**: An abstraction over the credential storage backend. Supports three operations: retrieve a secret by key, store a secret by key, and delete a secret by key. Has two concrete variants: one backed by the OS credential store and one backed by the filesystem (current behavior).
- **Credential Keys**: Well-known identifiers for each stored secret (OAuth token, Gemini API key, client credentials). Used to retrieve and store values consistently.
- **Migration State**: Implicit state derived from the presence or absence of secrets in the credential store vs. on disk. No explicit state file is needed — the system infers migration status at startup.

## Assumptions

- The OS credential store on macOS (Keychain) and Linux (Secret Service / GNOME Keyring) is the user's preferred secure storage mechanism.
- Headless macOS services (launchd agents) can access the login keychain without GUI prompts when the user is logged in.
- Headless Linux services may not have access to a Secret Service provider — the fallback path is essential for these environments.
- The `credentials.json` file is a shared resource that may be used by other tools — the system must never delete it without asking.
- OAuth token payloads (JSON-serialized) are small enough (under 2 KB) to fit within credential store value limits on all supported platforms.
- The existing OAuth2 library handles token refresh transparently; the system only needs to intercept the refreshed token for persistence.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After setup or migration, `~/.gcal-organizer/` contains no `token.json` file and no `GEMINI_API_KEY` entry in `.env` — all secrets reside in the OS credential store.
- **SC-002**: The system health check command reports "Secrets stored in OS credential store" (or equivalent) when the credential store is in use.
- **SC-003**: Scheduled background runs (launchd on macOS, systemd on Linux) complete without credential store prompts or authentication failures.
- **SC-004**: The tool runs without error on a headless machine lacking an OS credential store, falling back gracefully with a user-visible warning.
- **SC-005**: After a workflow run where the access token was refreshed, the next invocation uses the refreshed token without requiring a network round-trip for re-authentication.
- **SC-006**: Existing users upgrading from plaintext storage experience zero-downtime migration — no manual steps required, no data loss, no broken workflows.
