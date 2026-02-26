# Research: Secure Credential Storage

**Feature**: `007-secure-credential-storage`
**Date**: 2026-02-26

## R1: Go Keyring Library Selection

**Decision**: `github.com/zalando/go-keyring` v0.2.6

**Rationale**: Pure Go (no CGo), 3-function API (`Set`, `Get`, `Delete`), built-in `MockInit()` for testing, MIT licensed, actively maintained. Supports macOS Keychain (via `/usr/bin/security` subprocess), Linux Secret Service (via D-Bus), and Windows Credential Manager. Satisfies the Constitution's "No CGO required" constraint.

**Alternatives Considered**:
- `99designs/keyring`: Multi-backend (Keychain, Secret Service, KWallet, pass, file). More configuration surface area. Requires choosing and configuring backends explicitly. Overkill for a CLI tool that only needs macOS + Linux.
- Direct `/usr/bin/security` calls: macOS-only. Would require a separate Linux implementation. Reinventing what `go-keyring` already provides.
- `keyctl` syscalls (Linux): Low-level, kernel keyring only. Not persistent across reboots. Not equivalent to macOS Keychain.

## R2: Keyring Availability Detection

**Decision**: Attempt a probe `Set`/`Get`/`Delete` cycle with a sentinel key during `NewStore()`. If any step fails, fall back to `FileStore`.

**Rationale**: `go-keyring` does not expose a "is available?" function. The most reliable detection is to try a small operation. Using a sentinel key (`__gcal_organizer_probe__`) ensures we don't interfere with real data. The probe is cleaned up immediately.

**Alternatives Considered**:
- Check `DBUS_SESSION_BUS_ADDRESS` env var (Linux): Unreliable — some systems have D-Bus but no keyring, or have keyring via other mechanisms.
- Check for `/usr/bin/security` existence (macOS): Would detect the binary but not whether Keychain access is permitted (could be locked or denied).
- Catch errors lazily on first real operation: Complicates every call site with fallback logic. Better to decide once at startup.

## R3: Token Refresh Persistence Pattern

**Decision**: Custom `persistingTokenSource` wrapping `oauth2.Config.TokenSource()`. Compares returned tokens and saves on change.

**Rationale**: The `golang.org/x/oauth2` library's `TokenSource` interface returns a new `*Token` when the access token is refreshed. By wrapping this with a comparator that detects when the token has changed (different `AccessToken` or `Expiry`), we can persist only when a refresh actually occurred. This avoids unnecessary keychain writes on every API call.

**Alternatives Considered**:
- `oauth2.ReuseTokenSourceWithExpiry()`: Provides token caching but no persistence hook. Would need patching.
- Custom `http.RoundTripper`: Too invasive. Intercepts at the wrong layer (HTTP vs. token).
- Periodic background save: Adds complexity (goroutine, timer) for a problem that only occurs at most once per access token lifetime (typically 1 hour).

## R4: Non-Interactive Terminal Detection

**Decision**: Use `isatty.IsTerminal(os.Stdin.Fd())` from `github.com/mattn/go-isatty` (already an indirect dependency).

**Rationale**: Reliable cross-platform detection of whether stdin is a terminal. When false (pipe, cron, launchd, systemd), skip interactive prompts (e.g., `credentials.json` deletion). Already available in the dependency tree.

**Alternatives Considered**:
- Check `os.Getenv("TERM")`: Unreliable — some headless environments set `TERM`.
- Check `os.Getenv("CI")`: Only covers CI runners, not launchd/systemd.
- Always skip prompt and log: Too aggressive — interactive users should get the option.

## R5: Credential Store Naming Convention

**Decision**: Service name `com.jflowers.gcal-organizer`, keys: `oauth-token`, `gemini-api-key`, `credentials-json`.

**Rationale**: Reverse-DNS matches the existing launchd label (`com.jflowers.gcal-organizer`). Keys use lowercase-hyphenated format consistent with CLI flag naming. The `(service, key)` tuple uniquely identifies each secret in `go-keyring`.

**Alternatives Considered**:
- Flat service name `gcal-organizer`: Less specific, could collide with other tools.
- Hierarchical keys (`com.jflowers.gcal-organizer/oauth-token`): `go-keyring` uses the service name as the namespace; embedding hierarchy in the key is redundant.

## R6: `.env` Line Removal Strategy

**Decision**: Parse `.env` line by line, remove lines matching `GEMINI_API_KEY=` and `GOOGLE_CREDENTIALS_FILE=`, preserve all other lines (including comments). Write back atomically.

**Rationale**: The `.env` file contains both secrets and non-secret configuration. Removing only the secret entries preserves the user's other settings. Atomic write (write to temp file, then rename) prevents data loss on crash.

**Alternatives Considered**:
- Delete entire `.env` file: Destroys non-secret config (`GCAL_MASTER_FOLDER_NAME`, `GCAL_DAYS_TO_LOOK_BACK`, etc.).
- Use viper to rewrite: Viper doesn't preserve comments or formatting in dotenv files.
- Leave `.env` unchanged: Leaves secrets on disk, defeating the purpose of migration.

## R7: `credentials.json` Size and Keychain Limits

**Decision**: Store the full JSON blob as a single keychain entry. No chunking needed.

**Rationale**: A typical Google OAuth `credentials.json` is 400-800 bytes. `go-keyring`'s macOS limit is ~3000 bytes for the combined service+key+password. Even with the service name (`com.jflowers.gcal-organizer`, 28 chars) and key (`credentials-json`, 16 chars), there is ample room. Linux Secret Service has no practical limit for payloads under 100 KB.

**Alternatives Considered**:
- Chunking across multiple keychain entries: Unnecessary complexity for payloads well under limits.
- Compressing the JSON: Savings negligible on sub-1KB payloads; adds decode complexity.
- Storing only the client secret (not the full file): `google.ConfigFromJSON()` needs the full JSON structure including `redirect_uris`, `auth_uri`, etc. Storing just the secret would require reconstructing the JSON.
