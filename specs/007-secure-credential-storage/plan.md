# Implementation Plan: Secure Credential Storage

**Branch**: `007-secure-credential-storage` | **Date**: 2026-02-26 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/007-secure-credential-storage/spec.md`

## Summary

Move all sensitive credentials (OAuth tokens, Gemini API key, OAuth client credentials) from plaintext files in `~/.gcal-organizer/` into the OS credential store (macOS Keychain / Linux Secret Service) via a `SecretStore` abstraction. The implementation adds a new `internal/secrets/` package with two backends (keychain and file-based fallback), refactors the auth and config layers to use it, auto-migrates existing users transparently, and fixes the pre-existing bug where refreshed OAuth tokens are lost on process exit. A `--no-keyring` flag provides an opt-out path for headless environments.

## Technical Context

**Language/Version**: Go 1.24+ (module `github.com/jflowers/gcal-organizer`)
**Primary Dependencies**: `github.com/zalando/go-keyring` v0.2.6 (macOS Keychain via `/usr/bin/security`, Linux Secret Service via D-Bus — no CGo), `github.com/spf13/cobra` (CLI), `github.com/spf13/viper` (config), `golang.org/x/oauth2` (token handling), `github.com/charmbracelet/huh` (interactive prompts), `github.com/mattn/go-isatty` (terminal detection — already indirect dep)
**Storage**: OS credential store (primary), filesystem `~/.gcal-organizer/` (fallback). No database.
**Testing**: `go test ./...` — table-driven tests, `keyring.MockInit()` for keychain mocking
**Target Platform**: macOS (primary), Linux/Fedora (secondary). CLI tool.
**Project Type**: Single project — `cmd/`, `internal/`, `pkg/` layout
**Performance Goals**: No performance regression. Keychain calls are sub-millisecond. Token refresh persistence adds one `Set` call per workflow run (only when token actually refreshes).
**Constraints**: No CGo (Constitution: "No CGO required — pure Go for portability"). Must not break existing behavior when `--no-keyring` or no keyring is available. Must follow existing flag patterns (`--dry-run`, `--verbose`, `--owned-only`).
**Scale/Scope**: Medium feature — 1 new package (`internal/secrets/`, 4 files), 6 existing files modified, ~300 lines new logic, ~100 lines modified.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Gate | Principle | Status | Notes |
|------|-----------|--------|-------|
| CLI-First (I) | `--no-keyring` flag follows Unix conventions; config via env var `GCAL_NO_KEYRING` | **PASS** | Persistent flag on root, mirrors `--owned-only` pattern |
| API-Key Auth (II) | Credentials stored securely; never logged or exposed | **PASS** | Moves from plaintext files to OS credential store; `config show` already masks secrets |
| TDD (III) | Tests required before implementation | **PASS** | `keyring.MockInit()` enables unit tests without real keychain; table-driven tests for all paths |
| Idiomatic Go (IV) | Standard layout, explicit errors, no CGo | **PASS** | New package `internal/secrets/` follows layout; `go-keyring` is pure Go; errors wrapped with `%w` |
| Graceful Errors (V) | Actionable error messages | **PASS** | Keychain failures fall back gracefully with warning; migration errors reference `doctor` |
| Observability (VI) | Logging, dry-run support | **PASS** | Migration logs what was migrated; `--verbose` shows storage backend; fallback warnings logged |
| Self-Serve Diagnostics (VII) | `doctor` reports secret storage status | **PASS** | FR-009 requires `doctor` to report keychain vs. plaintext; errors reference `doctor` |
| No CGo (Tech) | Pure Go for portability | **PASS** | `zalando/go-keyring` uses subprocess (macOS) and D-Bus (Linux), no CGo |
| Documentation (Gov) | README, SETUP, man page updates | **PASS** | Plan includes doc updates for all three |
| Zero Regression (Gov) | Default behavior unchanged when flag not set | **PASS** | Keychain is the new default, but `--no-keyring` preserves old behavior; auto-detection falls back gracefully |

**Gate result: ALL PASS. Proceeding to Phase 0.**

## Project Structure

### Documentation (this feature)

```text
specs/007-secure-credential-storage/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (repository root)

```text
internal/
├── secrets/                          # [NEW] Secret storage abstraction
│   ├── store.go                      #   SecretStore interface + NewStore factory
│   ├── keychain.go                   #   KeychainStore (zalando/go-keyring)
│   ├── file.go                       #   FileStore (current file-based behavior)
│   ├── migrate.go                    #   Auto-migration logic
│   └── store_test.go                 #   Tests (MockInit-based)
├── auth/
│   └── oauth.go                      # [MODIFY] Use SecretStore; fix token refresh persistence
├── config/
│   ├── config.go                     # [MODIFY] Add NoKeyring field; keychain-first API key loading
│   └── config_test.go                # [MODIFY] Add NoKeyring tests
├── logging/
│   └── logging.go                    # [UNCHANGED]
└── ...

cmd/gcal-organizer/
├── main.go                           # [MODIFY] Add --no-keyring flag; wire SecretStore into initServices
├── auth_config.go                    # [MODIFY] auth login/status uses SecretStore; config show reports backend
└── selfservice.go                    # [MODIFY] doctor reports keychain status; init stores API key in keychain

docs/SETUP.md                         # [MODIFY] Update credential setup instructions
README.md                             # [MODIFY] Document --no-keyring, migration, keychain behavior
man/gcal-organizer.1                  # [MODIFY] Add --no-keyring flag, update credential docs
AGENTS.md                             # [MODIFY] Add 007 to recent changes
```

**Structure Decision**: Single project layout. New `internal/secrets/` package encapsulates the `SecretStore` abstraction. No new top-level directories.

## Proposed Changes

### Phase 1: SecretStore Interface + Backends (FR-001, FR-006, FR-007, FR-010)

**`internal/secrets/store.go`** — Interface and factory

- `SecretStore` interface: `Get(key string) (string, error)`, `Set(key, value string) error`, `Delete(key string) error`
- `Backend` type: `BackendKeychain` | `BackendFile`
- `NewStore(noKeyring bool) (SecretStore, Backend)` factory: if `noKeyring`, return `FileStore`; otherwise attempt `KeychainStore`, fall back to `FileStore` with logged warning
- Well-known key constants: `KeyOAuthToken`, `KeyGeminiAPIKey`, `KeyClientCredentials`
- Service name constant: `ServiceName = "com.jflowers.gcal-organizer"`

**`internal/secrets/keychain.go`** — Keychain backend

- `KeychainStore` struct (no fields needed; uses package-level `keyring` functions)
- `Get`: calls `keyring.Get(ServiceName, key)`, maps `keyring.ErrNotFound` to sentinel
- `Set`: calls `keyring.Set(ServiceName, key, value)`, maps `keyring.ErrSetDataTooBig`
- `Delete`: calls `keyring.Delete(ServiceName, key)`, ignores `ErrNotFound`

**`internal/secrets/file.go`** — File-based fallback

- `FileStore` struct with `configDir string`
- `Get`: reads from appropriate file based on key (`token.json`, `.env` value, `credentials.json`)
- `Set`: writes to appropriate file based on key
- `Delete`: removes file or entry based on key

### Phase 2: Auth Layer Refactor (FR-001, FR-003, FR-008)

**`internal/auth/oauth.go`** modifications:

- `OAuthClient` gains `store secrets.SecretStore` field (replaces `tokenFile string`)
- `NewOAuthClient` signature changes to accept `SecretStore` + optional `credentialsFile` (fallback path)
- `loadToken()` → `store.Get(KeyOAuthToken)` + JSON unmarshal
- `saveToken()` → JSON marshal + `store.Set(KeyOAuthToken, ...)`
- Credentials loading: `store.Get(KeyClientCredentials)` first, file fallback
- Token refresh fix: wrap `oauth2.TokenSource` with `persistingTokenSource` that calls `saveToken` on refresh

**Token refresh persistence** (`persistingTokenSource`):

```
type persistingTokenSource struct {
    base    oauth2.TokenSource
    store   SecretStore
    current *oauth2.Token
    mu      sync.Mutex
}
```

`Token()` calls `base.Token()`, compares with `current`, and if different (refreshed), persists via `store.Set()`.

### Phase 3: Config Layer (FR-002, FR-007, CR-001)

**`internal/config/config.go`**:

- Add `NoKeyring bool` field
- `Load()` checks keychain for `gemini-api-key` before env vars (when keychain available)
- Bind `GCAL_NO_KEYRING` env var

**`cmd/gcal-organizer/main.go`**:

- Add `--no-keyring` persistent flag on root
- Wire `SecretStore` creation in `initServices()` / command `RunE` functions

### Phase 4: Auto-Migration (FR-004, FR-005, FR-011, FR-012)

**`internal/secrets/migrate.go`**:

- `Migrate(store SecretStore, configDir string, interactive bool, verbose bool) error`
- For each secret type: check if exists in store → if not, check disk → if on disk, store in keychain → cleanup disk
- `token.json`: auto-delete after migration
- `.env` entries: strip `GEMINI_API_KEY` and `GOOGLE_CREDENTIALS_FILE` lines, preserve rest
- `credentials.json`: if interactive (`isatty`), prompt via `huh`; if non-interactive, log and skip deletion
- Idempotent: no-op if already migrated

### Phase 5: Doctor + Documentation (FR-009)

**`cmd/gcal-organizer/selfservice.go`**:

- `doctor` adds check: "Secrets stored in OS keychain" vs "Secrets stored in plaintext files"
- Reports per-secret status when `--verbose`
- `init` stores API key in keychain when available

**Documentation updates**: README.md, SETUP.md, man page, AGENTS.md

## Verification Plan

### Automated Tests

| Test | Package | What it verifies |
|------|---------|-----------------|
| `TestKeychainStore_SetGetDelete` | `secrets` | Round-trip via MockInit |
| `TestFileStore_SetGetDelete` | `secrets` | File-based read/write/delete |
| `TestNewStore_FallbackOnNoKeyring` | `secrets` | Factory returns FileStore when `--no-keyring` |
| `TestNewStore_FallbackOnUnavailable` | `secrets` | Factory returns FileStore when keyring unavailable |
| `TestMigrate_TokenFromDisk` | `secrets` | token.json → keychain + file deleted |
| `TestMigrate_APIKeyFromEnv` | `secrets` | .env entry → keychain + lines removed |
| `TestMigrate_CredentialsPrompt` | `secrets` | credentials.json → keychain + prompt behavior |
| `TestMigrate_NonInteractive` | `secrets` | Skips credentials.json deletion in non-interactive mode |
| `TestMigrate_Idempotent` | `secrets` | Second run is no-op |
| `TestOAuthClient_LoadFromStore` | `auth` | Token loaded from SecretStore |
| `TestOAuthClient_SaveToStore` | `auth` | Token saved to SecretStore |
| `TestPersistingTokenSource` | `auth` | Refreshed token persisted via store.Set |
| `TestConfigLoad_KeychainAPIKey` | `config` | API key from keychain preferred over env |
| `TestConfigLoad_NoKeyring` | `config` | NoKeyring flag falls back to env |

### Manual Tests

1. Fresh install: `gcal-organizer auth login` → verify token in Keychain Access.app
2. Migration: place `token.json` + `.env` + `credentials.json`, run any command → verify migration
3. Headless: `gcal-organizer run` via launchd → verify no prompts
4. No keyring: `gcal-organizer run --no-keyring` → verify file-based storage used
5. Doctor: `gcal-organizer doctor` → verify "Secrets stored in OS keychain" line

## Complexity Tracking

No constitution violations to justify. All gates pass.
