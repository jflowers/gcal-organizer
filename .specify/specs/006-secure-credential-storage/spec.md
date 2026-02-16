# Feature Specification: Secure Credential Storage

**Feature Branch**: `006-secure-credential-storage`  
**Created**: 2026-02-12  
**Status**: Draft  
**Input**: User concern about auth/token information stored in cleartext

---

## Problem Statement

All sensitive credentials are currently stored as **plaintext files** in `~/.gcal-organizer/`:

| File | Contents | Risk |
|------|----------|------|
| `token.json` | OAuth2 refresh + access tokens | Full Google Workspace access |
| `credentials.json` | OAuth2 client ID + client secret | App impersonation |
| `.env` | Gemini API key | API billing abuse |

Any process or user with read access to `~/.gcal-organizer/` can exfiltrate these credentials. On a shared machine or if the home directory is backed up unencrypted, this is a significant security gap.

## Current Implementation

```
internal/auth/oauth.go     — reads/writes token.json via os.ReadFile / json.Encode
internal/config/config.go  — loads GEMINI_API_KEY from .env / environment variables
credentials.json           — read directly by google.ConfigFromJSON()
```

File permissions are set to `0600` on `token.json`, but `credentials.json` and `.env` have no enforced permissions.

---

## Platform Credential Storage Options

### macOS: Keychain

- Native credential store, accessible via `security` CLI or Security framework
- Items are encrypted at rest by the OS and protected by the user's login password
- Supports storing arbitrary secrets (passwords, keys, tokens)
- Headless/launchd services can access keychain items created by the same user without prompting

### Fedora (Linux): GNOME Keyring / Secret Service

- D-Bus Secret Service API (implemented by GNOME Keyring or KWallet)
- Encrypted at rest, unlocked automatically on login via PAM integration
- Headless access requires the session D-Bus bus and an unlocked keyring
- **Caveat**: headless services (systemd user units, cron) may not have a D-Bus session unless configured with `dbus-run-session` or `gnome-keyring-daemon`

### Go Libraries

| Library | macOS | Linux | Notes |
|---------|-------|-------|-------|
| [`zalando/go-keyring`](https://github.com/zalando/go-keyring) | Keychain via `/usr/bin/security` | Secret Service D-Bus | No CGo, pure Go. Simple `Set/Get/Delete` API. |
| [`99designs/keyring`](https://github.com/99designs/keyring) | Keychain via native API | Secret Service, KWallet, pass, file | Multi-backend, more config surface. |

> [!TIP]
> `zalando/go-keyring` is the simpler choice — no CGo, 3-function API (`Set`, `Get`, `Delete`), actively maintained. Good fit for a CLI tool.

---

## How Secrets Enter the Keychain

No new commands are needed — existing commands are modified to write to the keychain:

| Secret | Command | Mechanism |
|--------|---------|----------|
| OAuth token | `auth login` (existing) | `saveToken()` writes to keychain instead of `token.json`; `loadToken()` reads from keychain |
| Gemini API key | `init` (existing) | Prompt saves to keychain instead of writing `GEMINI_API_KEY` to `.env`; `config.Load()` checks keychain first, then falls back to env vars |
| `credentials.json` | `init` / `auth login` (existing) | Reads file contents, stores JSON blob in keychain. **Prompts user** before deleting the file (it may be shared with other tools). |

For **existing users**, auto-migration happens transparently inside `loadToken()` / `config.Load()` on first run after upgrade: detect plaintext on disk → store in keychain → remove plaintext (with prompt for `credentials.json`).

---

## User Scenarios & Testing

### User Story 1 — Secure Token Storage (Priority: P1)

As a user, I want my OAuth tokens stored in the OS keychain so that they are encrypted at rest and not readable by other processes.

**Why this priority**: The OAuth refresh token grants full Google Workspace access (Drive, Calendar, Docs, Tasks). This is the highest-value secret.

**Independent Test**: Run `gcal-organizer auth login`, then verify the token is in the keychain and `token.json` no longer exists on disk.

**Acceptance Scenarios**:

1. **Given** a fresh install, **When** user runs `gcal-organizer auth login`, **Then** the OAuth token is stored in the OS keychain (macOS Keychain or GNOME Keyring), not as a file.
2. **Given** a token already in the keychain, **When** the workflow runs, **Then** it retrieves the token from the keychain without user interaction.
3. **Given** a headless launchd/systemd service, **When** the workflow runs, **Then** it can access the keychain token without a GUI prompt.

---

### User Story 2 — Secure API Key Storage (Priority: P2)

As a user, I want my Gemini API key stored in the OS keychain instead of a plaintext `.env` file.

**Why this priority**: The API key allows billing-impacting usage of the Gemini API. Lower risk than OAuth tokens but still sensitive.

**Independent Test**: Run `gcal-organizer init`, provide the API key, then verify it's in the keychain and no longer in `.env`.

**Acceptance Scenarios**:

1. **Given** a user runs `gcal-organizer init`, **When** they enter a Gemini API key, **Then** it is saved to the keychain under a well-known service/key name.
2. **Given** an API key in the keychain, **When** `config.Load()` runs, **Then** it retrieves the key from the keychain (falling back to env var if not found).

---

### User Story 3 — Migration from Plaintext (Priority: P2)

As an existing user, I want a smooth migration path that moves my existing plaintext secrets into the keychain.

**Why this priority**: Existing installs need an upgrade path. Without it users are stuck with cleartext forever.

**Independent Test**: With existing `token.json` and `.env`, run `gcal-organizer auth migrate` (or have it auto-migrate on first run), verify secrets moved to keychain and plaintext files removed.

**Acceptance Scenarios**:

1. **Given** an existing `token.json` on disk, **When** `gcal-organizer` starts, **Then** it auto-migrates the token to the keychain and deletes `token.json`.
2. **Given** a `GEMINI_API_KEY` in `.env`, **When** `gcal-organizer` starts, **Then** it migrates the key to the keychain and removes the line from `.env`.
3. **Given** an existing `credentials.json` on disk, **When** migration runs, **Then** it stores the contents in the keychain and **prompts the user** before deleting the file (since it may be used by other tools).
4. **Given** migration has already completed, **When** `gcal-organizer` starts again, **Then** no migration occurs and no errors are logged.

---

### User Story 4 — Fallback for Headless/No-Keyring Environments (Priority: P3)

As a user running on a headless Linux server without GNOME Keyring, I want the tool to gracefully fall back to encrypted file storage.

**Why this priority**: Not all Linux environments have Secret Service. The tool must not break in CI or minimal server installs.

**Independent Test**: Unset `DBUS_SESSION_BUS_ADDRESS`, run the tool, verify it logs a warning and falls back to file-based storage.

**Acceptance Scenarios**:

1. **Given** no Secret Service is available, **When** the tool starts, **Then** it warns and falls back to the current file-based storage (or an encrypted file with a passphrase).
2. **Given** a `--no-keyring` flag is passed, **When** secrets are loaded, **Then** file-based storage is used regardless of keyring availability.

---

### Edge Cases

- What happens if the keychain is **locked** (macOS) or the keyring is **locked** (Linux)?
- What happens if the user **denies** keychain access on macOS?
- What happens if `credentials.json` is referenced by path in config — should it stay on disk or move into the keychain too?
- How do we handle token **refresh**? (The `oauth2` library returns new tokens that must be re-saved.)
- `credentials.json` may be **shared** across multiple projects — never auto-delete; always prompt.

---

## Requirements

### Functional Requirements

- **FR-001**: System MUST store OAuth refresh/access tokens in the OS credential store (macOS Keychain or Linux Secret Service) by default.
- **FR-002**: System MUST store the Gemini API key in the OS credential store by default.
- **FR-003**: System MUST auto-migrate existing plaintext secrets to the keychain on first run after upgrade.
- **FR-004**: System MUST delete `token.json` and remove `GEMINI_API_KEY` from `.env` after successful migration. System MUST **prompt before deleting** `credentials.json` since it may be shared with other tools.
- **FR-005**: System MUST gracefully fall back to file-based storage when no credential store is available, with a logged warning.
- **FR-006**: System MUST support a `--no-keyring` flag or `GCAL_NO_KEYRING=true` env var to opt out of keychain storage.
- **FR-007**: System MUST re-save refreshed OAuth tokens back to the credential store (not to disk).
- **FR-008**: `gcal-organizer doctor` MUST report whether secrets are stored securely or in plaintext.

### Key Entities

- **SecretStore** (interface): Abstraction over keychain vs. file storage — `Get(key)`, `Set(key, value)`, `Delete(key)`.
- **KeychainStore**: Implementation using `zalando/go-keyring` for macOS Keychain / Linux Secret Service.
- **FileStore**: Fallback implementation using current file-based approach (existing behavior).

### Design Decision: What about `credentials.json`?

> [!IMPORTANT]
> `credentials.json` contains the OAuth **client secret**, but the Google `oauth2` library expects it as a file path passed to `google.ConfigFromJSON()`. Options:
>
> 1. **Store the entire JSON blob** in the keychain and pass it as bytes (no file needed)
> 2. **Leave it on disk** but enforce `0600` permissions — it's less sensitive than the tokens since the client secret alone can't access data without user consent
>
> **Recommendation**: Option 1 (store in keychain) for full coverage, since `google.ConfigFromJSON()` accepts `[]byte` not a file path.

---

## Success Criteria

### Measurable Outcomes

- **SC-001**: After migration, `ls ~/.gcal-organizer/` shows no `token.json` and no `GEMINI_API_KEY` in `.env`
- **SC-002**: `gcal-organizer doctor` reports "✅ Secrets stored in OS keychain"
- **SC-003**: Scheduled launchd/systemd runs complete without keychain prompts
- **SC-004**: Tool runs without error on a headless Linux machine (falls back gracefully)
