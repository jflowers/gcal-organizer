# Data Model: Secure Credential Storage

**Feature**: `007-secure-credential-storage`
**Date**: 2026-02-26

## Entities

### New Entities

#### SecretStore (interface — `internal/secrets/store.go`)

| Method | Signature | Description |
|--------|-----------|-------------|
| `Get` | `Get(key string) (string, error)` | Retrieve a secret by key. Returns `ErrNotFound` if absent. |
| `Set` | `Set(key, value string) error` | Store or overwrite a secret by key. |
| `Delete` | `Delete(key string) error` | Remove a secret by key. No error if already absent. |

#### KeychainStore (struct — `internal/secrets/keychain.go`)

| Field | Type | Description |
|-------|------|-------------|
| (none) | — | Stateless; delegates to `keyring.Get/Set/Delete` with the service name constant. |

Implements `SecretStore`. All operations use `keyring.Get(ServiceName, key)` etc. where `ServiceName = "com.jflowers.gcal-organizer"`.

#### FileStore (struct — `internal/secrets/file.go`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `configDir` | `string` | `~/.gcal-organizer` | Base directory for credential files. |

Implements `SecretStore`. Maps keys to file operations:

| Key Constant | File | Get Behavior | Set Behavior | Delete Behavior |
|-------------|------|-------------|-------------|-----------------|
| `KeyOAuthToken` | `token.json` | Read + JSON decode | JSON encode + write (0600) | Remove file |
| `KeyGeminiAPIKey` | `.env` | Parse `GEMINI_API_KEY=` line | Write/update line in `.env` | Remove line from `.env` |
| `KeyClientCredentials` | `credentials.json` | Read file contents | Write file (0600) | Remove file |

#### Backend (type — `internal/secrets/store.go`)

| Value | Description |
|-------|-------------|
| `BackendKeychain` | OS credential store is in use |
| `BackendFile` | File-based fallback is in use |

Returned by `NewStore()` alongside the `SecretStore` instance. Used by `doctor` and `config show` for reporting.

### Modified Entities

#### Config (`internal/config/config.go`)

| Field | Type | Default | Source | Notes |
|-------|------|---------|--------|-------|
| `NoKeyring` | `bool` | `false` | CLI flag `--no-keyring`, env `GCAL_NO_KEYRING` | New field. Forces file-based secret storage. |

Existing fields `CredentialsFile` and `TokenFile` become fallback paths — only used when `FileStore` is active.

**New method**: `LoadSecrets(store SecretStore) error` — enriches the config with secrets from the credential store. Checks `store.Get(KeyGeminiAPIKey)` and overrides `GeminiAPIKey` if found; falls back to existing viper/env value if `ErrNotFound`. Called after `Load()` and `NewStore()` in the startup sequence.

**Removed method**: `ValidateForWorkflow()` — removed (zero callers in codebase). The `os.Stat` check on `CredentialsFile` would fail after credentials are migrated to keychain. Credential presence validation is now handled by `NewOAuthClient`, which checks the store first, then the file fallback.

#### OAuthClient (`internal/auth/oauth.go`)

| Field | Change | Notes |
|-------|--------|-------|
| `tokenFile string` | **Removed** | Replaced by `store` field. |
| `store SecretStore` | **Added** | Used for token load/save. |
| `credsFallbackPath string` | **Added** | Fallback path for `credentials.json` if not in store. |

#### Stats (`internal/organizer/organizer.go`)

No changes. Migration runs before orchestration; no new stats counters needed.

### Unchanged Entities

- `Document`, `MeetingFolder`, `CalendarEvent`, `Attachment`, `Attendee` (pkg/models/) — no credential handling
- `DriveService`, `CalendarService` interfaces (internal/organizer/) — unchanged
- `drive.Service`, `calendar.Service`, `docs.Service`, `gemini.Client` — consume `http.Client`, unaware of credential storage

## New Constants (`internal/secrets/store.go`)

| Constant | Value | Description |
|----------|-------|-------------|
| `ServiceName` | `"com.jflowers.gcal-organizer"` | Keychain service name (reverse-DNS) |
| `KeyOAuthToken` | `"oauth-token"` | Key for OAuth2 refresh + access token |
| `KeyGeminiAPIKey` | `"gemini-api-key"` | Key for Gemini API key |
| `KeyClientCredentials` | `"credentials-json"` | Key for OAuth client credentials blob |

## New Functions

### `NewStore(noKeyring bool) (SecretStore, Backend)`

| Parameter | Type | Description |
|-----------|------|-------------|
| `noKeyring` | `bool` | If true, skip keychain detection and return FileStore |
| **Returns** | `(SecretStore, Backend)` | The active store and its backend type |

**Behavior**: If `noKeyring` is false, attempt a probe write/read/delete to the keychain. If the probe succeeds, return `KeychainStore`. If it fails (keyring unavailable, locked, denied), log a warning and return `FileStore`. Logging uses the shared `internal/logging` logger — backend selection is logged at Info level, fallback warnings at Warn level. The logger respects `--verbose` via its configured level.

### `Migrate(store SecretStore, configDir string, interactive bool, verbose bool) error`

| Parameter | Type | Description |
|-----------|------|-------------|
| `store` | `SecretStore` | Target store (must be `KeychainStore` for migration to proceed) |
| `configDir` | `string` | Path to `~/.gcal-organizer` |
| `interactive` | `bool` | Whether to prompt for `credentials.json` deletion |
| `verbose` | `bool` | Whether to log migration details |
| **Returns** | `error` | nil on success or if nothing to migrate |

**Behavior**: For each secret: if not in store and file exists on disk, read from disk → write to store → clean up disk (with interactive gate for `credentials.json`). Idempotent.

## Data Flow

```
CLI startup
    │
    ├── config.Load()
    │     └── Read NoKeyring from flag/env (no secrets yet)
    │
    ├── secrets.NewStore(cfg.NoKeyring)
    │     ├── noKeyring=true → FileStore
    │     └── noKeyring=false
    │           ├── probe keychain → success → KeychainStore
    │           └── probe keychain → fail → FileStore + warning
    │
    ├── cfg.LoadSecrets(store)
    │     └── store.Get(KeyGeminiAPIKey) → override cfg.GeminiAPIKey (or keep env fallback)
    │
    ├── secrets.Migrate(store, configDir, isInteractive, verbose)
    │     ├── token.json on disk? → store.Set(KeyOAuthToken) → delete file
    │     ├── GEMINI_API_KEY in .env? → store.Set(KeyGeminiAPIKey) → strip lines
    │     └── credentials.json on disk? → store.Set(KeyClientCredentials)
    │           ├── interactive → prompt → delete or keep
    │           └── non-interactive → log → keep
    │
    ├── auth.NewOAuthClient(store, credsFallbackPath)
    │     └── store.Get(KeyClientCredentials) || readFile(credsFallbackPath)
    │           └── google.ConfigFromJSON(bytes, scopes...)
    │
    ├── oauthClient.GetClient(ctx)
    │     ├── store.Get(KeyOAuthToken) → unmarshal
    │     │     └── not found → getTokenFromWeb() → store.Set(KeyOAuthToken)
    │     └── config.Client(ctx, tok) with persistingTokenSource
    │           └── on refresh → store.Set(KeyOAuthToken)
    │
    └── gemini.NewClient(cfg.GeminiAPIKey)
          └── (API key already loaded via cfg.LoadSecrets)
```

## State Transitions

No explicit state machine. Migration state is implicit:

| Condition | State | Action |
|-----------|-------|--------|
| Secret in store, not on disk | Migrated | No-op |
| Secret in store AND on disk | Partially migrated | Re-attempt disk cleanup |
| Secret on disk, not in store | Not migrated | Migrate: disk → store → cleanup |
| Secret in neither | Not configured | Normal setup flow |
