# Research: YAML Config Migration & Decision Export Enhancements

**Feature**: 013-yaml-config-decision-export
**Date**: 2026-04-20
**Status**: Complete

## R1: Switching Viper from AutomaticEnv-Only to YAML File + AutomaticEnv

### Current State

The current config loading flow is:

1. `initConfig()` (cobra.OnInitialize) calls `config.LoadDotEnv(envFile, home)` which parses `.env` and sets values as `os.Setenv()` environment variables.
2. `config.Load()` calls `viper.AutomaticEnv()` and `viper.BindEnv()` for each key, then reads values via `viper.GetString()`/`viper.GetInt()`.
3. The net effect: `.env` values are laundered through the process environment, and viper reads them as if they were real env vars.

This roundabout approach exists because viper's `AutomaticEnv()` only reads from `os.Environ()`, not from files. The `.env` file is not a viper-native config format.

### Target State

Switch to viper's native YAML config file support:

```go
// In initConfig() or config.Load():
viper.SetConfigFile(configPath)  // e.g., ~/.gcal-organizer/config.yaml
viper.SetConfigType("yaml")
if err := viper.ReadInConfig(); err != nil {
    if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
        return err  // real error
    }
    // No config file — use defaults + env vars
}
viper.AutomaticEnv()  // env vars still override YAML values
```

### Key Findings

1. **Viper precedence order** (highest to lowest): explicit `Set()` > flags > env vars > config file > defaults. This means `AutomaticEnv()` env var overrides work automatically with YAML — no code change needed for FR-005.

2. **Key mapping**: Viper uses dots for nested keys in YAML. A YAML structure like:
   ```yaml
   decisions:
     meetings:
       - "Sprint Planning"
   ```
   is accessed as `viper.GetStringSlice("decisions.meetings")`. The existing `SetEnvKeyReplacer(strings.NewReplacer(".", "_"))` maps this to env var `GCAL_DECISIONS_MEETINGS` automatically.

3. **List values**: Viper natively handles YAML lists. `viper.GetStringSlice("filename_keywords")` returns `[]string` directly from YAML, eliminating the current `strings.Split(v, ",")` workaround.

4. **No new dependencies**: Viper already includes YAML support via its transitive dependency on `gopkg.in/yaml.v3`. No `go.mod` changes needed.

5. **SetConfigFile vs SetConfigName**: Use `SetConfigFile()` (exact path) rather than `SetConfigName()` + `AddConfigPath()` because we know the exact location. This avoids viper's search behavior which could find config files in unexpected locations.

### Decision

**D1**: Use `viper.SetConfigFile()` + `viper.ReadInConfig()` for YAML loading. Keep `viper.AutomaticEnv()` for env var overrides. Remove `LoadDotEnv()` call from `initConfig()` when `config.yaml` exists.

## R2: .env Migration Strategy

### Parsing

The existing `LoadDotEnv()` function in `internal/config/dotenv.go` already handles:
- Comment lines (`#`)
- Blank lines
- `KEY=VALUE` parsing with `SplitN(line, "=", 2)`
- Quote stripping (double and single quotes)
- POSIX `'\''` unescaping for single-quoted values
- Tilde expansion (`~/` → home directory)
- Validation via `ValidEnvKey` regex

This parser can be reused for migration. Rather than duplicating parsing logic, the migration function should call a shared parsing function that returns a `map[string]string` of key-value pairs.

### Key Mapping (.env → YAML)

| .env Key | YAML Key | Type | Notes |
|----------|----------|------|-------|
| `GCAL_MASTER_FOLDER_NAME` | `master_folder_name` | string | Direct mapping |
| `GCAL_DAYS_TO_LOOK_BACK` | `days_to_look_back` | int | Parse string to int |
| `GCAL_FILENAME_KEYWORDS` | `filename_keywords` | []string | Split on comma |
| `GCAL_FILENAME_PATTERN` | `filename_pattern` | string | Direct mapping |
| `GEMINI_MODEL` | `gemini_model` | string | Direct mapping |
| `CHROME_PROFILE_PATH` | `chrome_profile_path` | string | Direct mapping, preserve path |
| `GCAL_DECISIONS_EXPORT_DIR` | `decisions.export_dir` | string | Nested under decisions |
| `GEMINI_API_KEY` | *(skip)* | — | Secret: stays in keychain (FR-003) |
| `GOOGLE_CREDENTIALS_FILE` | *(skip)* | — | Secret: stays in keychain (FR-003) |

### Atomic Write Strategy

1. Parse `.env` into `map[string]string`
2. Map keys to YAML structure (see data-model.md)
3. Marshal to YAML via `gopkg.in/yaml.v3`
4. Write to `config.yaml.tmp` (temp file in same directory)
5. `os.Rename(config.yaml.tmp, config.yaml)` — atomic on same filesystem
6. Verify `config.yaml` is readable (re-read and validate)
7. Delete `.env`

The temp-file-then-rename pattern is already used in `internal/secrets/file.go` (`writeLines()`), so this is a proven pattern in the codebase.

### Three .env Parsers — Impact Analysis

The codebase has three independent `.env` parsers:

1. **`config.LoadDotEnv()`** — Full parser in `internal/config/dotenv.go`. Used by `initConfig()` to load `.env` into process environment. **Impact**: This call is removed from `initConfig()` when `config.yaml` exists. The function remains available for migration parsing.

2. **`loadEnvValue()`** — Simplified parser in `cmd/gcal-organizer/selfservice.go`. Used by `doctor` and `init` commands to read individual values from `.env`. **Impact**: After migration, `.env` no longer exists. These call sites must be updated to read from `config.yaml` or removed. The `doctor` command's GEMINI_API_KEY check should use the config/store, not `.env`.

3. **`FileStore.readEnvValue()`** — Parser in `internal/secrets/file.go`. Used by `FileStore.Get()` for the `KeyGeminiAPIKey` case. **Impact**: The FileStore is the fallback when keychain is unavailable. After migration, if `.env` is deleted, `FileStore.Get(KeyGeminiAPIKey)` returns `ErrNotFound`. This is acceptable because:
   - If keychain is available: secrets are in keychain, FileStore is not used.
   - If keychain is unavailable: the API key must be provided via env var or the user must re-run `init` (which now generates `config.yaml`).

### Decision

**D2**: Reuse `LoadDotEnv`'s parsing logic (extract to a shared `ParseDotEnv()` function that returns `map[string]string`). Use atomic write (temp + rename). Delete `.env` only after `config.yaml` is verified readable.

**D3**: The `FileStore.readEnvValue()` path is acceptable to break after migration because the FileStore is only used when keychain is unavailable, and in that scenario the user can provide the API key via env var. No code change needed in `file.go`.

## R3: Service Mode Template Updates

### Current State

Two service mode artifacts reference `.env`:

1. **Wrapper script** (`generateWrapper()` in `selfservice.go`):
   ```bash
   ENV_FILE="${HOME}/.gcal-organizer/.env"
   if [ -f "$ENV_FILE" ]; then
       set -a
       source "$ENV_FILE"
       set +a
   fi
   ```
   This sources `.env` to set environment variables before running the binary.

2. **Systemd service unit** (`generateSystemdService()` in `selfservice.go`):
   ```ini
   EnvironmentFile=-$HOME/.gcal-organizer/.env
   ```
   The `-` prefix means "don't fail if file is missing."

### Target State

After migration, the binary loads all configuration from `config.yaml` internally. The wrapper script and systemd unit no longer need to source `.env`.

**Wrapper script changes**:
- Remove the `.env` sourcing block entirely
- The binary handles all config loading via `config.yaml`
- Environment variables still work as overrides (viper precedence)

**Systemd service unit changes**:
- Remove the `EnvironmentFile` directive
- The binary handles all config loading via `config.yaml`

### Backward Compatibility

Users who have already installed the service will have the old wrapper script and systemd unit on disk. The old wrapper script sources `.env` — but after migration, `.env` is deleted, so the `if [ -f "$ENV_FILE" ]` guard means the sourcing is silently skipped. No breakage.

Users who run `gcal-organizer install` after the update will get the new templates without `.env` references.

### Decision

**D4**: Remove `.env` sourcing from `generateWrapper()` and `EnvironmentFile` from `generateSystemdService()`. The old templates are backward-compatible (guard clause handles missing `.env`), so no forced re-install is needed.

## R4: Migration Timing and initConfig() Flow

### Current Flow

```
cobra.OnInitialize(initConfig)
  → initConfig()
    → LoadDotEnv(envFile, home)  // sets env vars from .env
    → viper.AutomaticEnv()       // viper reads env vars
  → command.RunE()
    → loadConfigAndStore()
      → config.Load()            // reads viper values into Config struct
      → secrets.NewStore()
      → secrets.Migrate()        // migrates secrets to keychain
```

### New Flow

```
cobra.OnInitialize(initConfig)
  → initConfig()
    → if config.yaml exists:
        → viper.SetConfigFile(configYamlPath)
        → viper.ReadInConfig()
      else if .env exists:
        → config.MigrateEnvToYAML(envPath, configYamlPath)  // parse, write yaml, delete .env
        → viper.SetConfigFile(configYamlPath)
        → viper.ReadInConfig()
      else:
        → (no config file — defaults + env vars only)
    → viper.AutomaticEnv()
  → command.RunE()
    → loadConfigAndStore()
      → config.Load()            // reads viper values into Config struct
      → secrets.NewStore()
      → secrets.Migrate()        // migrates secrets to keychain (unchanged)
```

### Key Insight: Migration Before Viper

The migration must happen in `initConfig()` (before any command runs) because:
1. All commands call `loadConfigAndStore()` → `config.Load()` which reads from viper
2. Viper must have the YAML file loaded before `config.Load()` is called
3. The migration is a one-time operation that converts `.env` → `config.yaml`

### Secret Migration Ordering

The secret migration (`secrets.Migrate()`) runs in `loadConfigAndStore()`, which is called after `initConfig()`. This means:
1. `initConfig()` migrates `.env` → `config.yaml` (non-secret values only)
2. `loadConfigAndStore()` → `secrets.Migrate()` migrates secrets from `.env` to keychain

But wait — if `initConfig()` deletes `.env`, then `secrets.Migrate()` can't read secrets from it!

**Resolution**: The `.env` → `config.yaml` migration must happen AFTER secret migration, OR the migration must extract secrets from `.env` before deleting it and pass them to the secret store.

**Revised flow**:
1. `initConfig()`: If `.env` exists and `config.yaml` does not, load `.env` via `LoadDotEnv()` (legacy path) so viper can read values. Set a flag indicating migration is pending.
2. `loadConfigAndStore()`: After `secrets.Migrate()` completes, run the `.env` → `config.yaml` migration (parse `.env`, write `config.yaml`, delete `.env`).
3. On subsequent runs, `initConfig()` finds `config.yaml` and uses viper's native YAML loading.

### Decision

**D5**: Migration runs in `loadConfigAndStore()` after `secrets.Migrate()` to ensure secrets are safely in the keychain before `.env` is deleted. On the first run after upgrade, `initConfig()` still uses `LoadDotEnv()` to populate viper. On subsequent runs, `initConfig()` uses `viper.ReadInConfig()` with the YAML file.

## R5: --config Flag Behavior

### Current Behavior

The `--config` flag sets `cfgFile` which is used as the `.env` file path in `initConfig()`. If not set, defaults to `~/.gcal-organizer/.env`.

### New Behavior

The `--config` flag accepts a path to a YAML config file:
- If the path ends in `.yaml` or `.yml`: use it directly as the config file
- If the path points to a `.env` file (or any non-YAML file): migrate it to `config.yaml` in the same directory, then use the YAML file
- If not set: default to `~/.gcal-organizer/config.yaml`

### Detection Heuristic

Rather than relying on file extension (which could be wrong), detect the file format by content:
1. Try to parse as YAML first
2. If YAML parsing fails, try `.env` parsing
3. If `.env` parsing succeeds, migrate

Simpler approach: check if the file contains `=` on non-comment lines (`.env` format) vs. `:` with indentation (YAML format). But this is fragile.

**Simplest approach**: Check file extension. `.env` or no extension → treat as `.env`. `.yaml`/`.yml` → treat as YAML. This matches user expectations and the spec says "if pointed at a `.env` file, the system MUST migrate it."

### Decision

**D6**: Use file extension to determine format. `.env` or files named `.env` → migrate. `.yaml`/`.yml` → use directly. Default path changes from `~/.gcal-organizer/.env` to `~/.gcal-organizer/config.yaml`.

## R6: init Command — Generating config.yaml

### Current Behavior

`initCmd` generates a `.env` file via `generateEnvFile()` with comments and default values.

### New Behavior

`initCmd` generates a `config.yaml` file with default values and comments. YAML supports comments natively (`#`), so the user experience is similar.

The generated file should include:
- All non-secret config keys with defaults
- Comments explaining each key
- The `decisions` section with empty `meetings` list (commented out as example)
- No secret values (API key stored in keychain, not in config file)

### Decision

**D7**: Replace `generateEnvFile()` with `generateConfigYAML()`. The new function produces a commented YAML file with defaults. Secrets are stored in keychain via the existing `store.Set()` path (unchanged).

## R7: doctor Command — Checking config.yaml

### Current Behavior

`doctorCmd` checks for `.env` file existence (check #2 in the doctor output).

### New Behavior

Check for `config.yaml` instead. If `.env` exists but `config.yaml` does not, suggest running any command to trigger migration (or running `init`).

### Decision

**D8**: Update doctor check #2 to look for `config.yaml`. Add a transitional check: if `.env` exists, warn that it will be migrated on next run.
