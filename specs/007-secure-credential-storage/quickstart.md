# Quickstart: Secure Credential Storage

**Feature**: `007-secure-credential-storage`
**Date**: 2026-02-26

## New User Experience

After this feature, new users follow the same setup flow — credentials are automatically stored in the OS keychain:

```bash
# 1. Initialize (API key goes to keychain, not .env)
gcal-organizer init

# 2. Download credentials.json and place in ~/.gcal-organizer/
#    (will be moved to keychain during auth login)

# 3. Authenticate (token goes to keychain, not token.json)
gcal-organizer auth login

# 4. Verify
gcal-organizer doctor
# Output includes: ✅ Secrets stored in OS keychain
```

## Existing User Migration

Existing users are migrated automatically on first run after upgrade:

```bash
# Just run any command — migration happens transparently
gcal-organizer run --dry-run

# Output:
# Migrated OAuth token to OS keychain (deleted token.json)
# Migrated Gemini API key to OS keychain (removed from .env)
# Migrated client credentials to OS keychain
# Would you like to delete credentials.json? It may be shared with other tools. [y/N]
```

After migration, `~/.gcal-organizer/` retains only non-secret config:

```bash
ls ~/.gcal-organizer/
# .env              (non-secret config only: folder name, days, keywords, model)
# chrome-data/      (browser automation profile)
```

## Behavior Comparison

| Scenario | Before (plaintext) | After (keychain) |
|----------|-------------------|-------------------|
| Fresh `auth login` | Token saved to `~/.gcal-organizer/token.json` | Token saved to OS keychain |
| Fresh `init` | API key written to `.env` | API key saved to OS keychain |
| Token refresh during run | New token lost on exit | New token persisted to keychain |
| `config show` | Shows `Token File: ~/.gcal-organizer/token.json` | Shows `Secret storage: OS keychain` |
| `doctor` | Checks `token.json` exists | Reports "Secrets stored in OS keychain" |
| Headless/no keyring | N/A | Falls back to file-based storage with warning |

## Opt-Out (Headless / CI)

```bash
# Via CLI flag
gcal-organizer run --no-keyring

# Via environment variable
export GCAL_NO_KEYRING=true
gcal-organizer run

# In .env (for service wrapper)
GCAL_NO_KEYRING=true
```

When opted out, behavior is identical to pre-feature (file-based storage). A warning is logged if keyring was available but explicitly disabled.

## Verifying Secret Storage

```bash
# Check via doctor
gcal-organizer doctor --verbose
# ✅ Secrets stored in OS keychain
#    oauth-token: present
#    gemini-api-key: present
#    credentials-json: present

# Check via auth status
gcal-organizer auth status
# ✅ Authenticated and token valid
# ✅ Secrets stored in OS keychain

# Verify on macOS (Keychain Access or CLI)
security find-generic-password -s "com.jflowers.gcal-organizer" -a "oauth-token"
```

## Known Limitations

- **Shared Drive machines**: On Linux servers without GNOME Keyring or Secret Service, the tool falls back to file-based storage automatically. Use `--no-keyring` to suppress the warning.
- **`credentials.json` not auto-deleted**: Since this file may be shared with other Google Cloud tools, the system always prompts before deleting it. In non-interactive contexts (cron, launchd), deletion is skipped and a log message suggests manual cleanup.
- **Keychain Access.app visibility**: On macOS, stored secrets are visible in Keychain Access.app under the "login" keychain. This is expected — the secrets are encrypted at rest by the OS.
