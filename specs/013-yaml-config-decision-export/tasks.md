# Tasks: YAML Config Migration & Decision Export Enhancements

**Feature**: 013-yaml-config-decision-export
**Generated**: 2026-04-20
**Spec**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Data Model**: [data-model.md](./data-model.md)

**User Stories**:
- US1 (P1): Auto-migrate .env to config.yaml on startup
- US2 (P2): Meeting allowlist filter (decisions.meetings, exact match case-insensitive)
- US3 (P3): Per-meeting folders + time in filename
- US4 (P3): Google Doc source link in frontmatter

---

## Phase 1: Config Struct & YAML Loading (Foundation)

_All subsequent phases depend on the Config struct changes and YAML loading. Must complete first._

- [x] T01 [US1] Add `DecisionsConfig` nested struct and update `Config` struct with `mapstructure` tags in `internal/config/config.go`
  - Add `DecisionsConfig` struct with `ExportDir` and `Meetings` fields (data-model.md §2)
  - Replace flat `DecisionsExportDir` field with `Decisions DecisionsConfig` field
  - Add `mapstructure` tags to all YAML-backed fields (FR-009)
  - Update `DefaultConfig()` to set `Decisions.ExportDir` default

- [x] T02 [US1] Update `Load()` to use viper YAML config file support in `internal/config/config.go`
  - Switch from `viper.GetString("decisions_export_dir")` to `viper.GetString("decisions.export_dir")` (nested key)
  - Add `viper.GetStringSlice("decisions.meetings")` for allowlist loading
  - Add `viper.GetStringSlice("filename_keywords")` to replace comma-split workaround
  - Bind `decisions.meetings` env var via `mustBindEnv` (FR-005)
  - Keep `AutomaticEnv()` + `SetEnvKeyReplacer` for env var overrides (FR-005)
  - Expand tilde in `Decisions.ExportDir` (consistent with existing pattern)

- [x] T03 [P] [US1] Update all callers of `cfg.DecisionsExportDir` to `cfg.Decisions.ExportDir`
  - `cmd/gcal-organizer/main.go`: Update `export.NewExporter(cfg.DecisionsExportDir, ...)` call
  - Grep for any other references to `DecisionsExportDir` across the codebase

- [x] T04 [US1] Update `MissingAPIKey()` error message in `internal/ux/ux.go`
  - Change `.env` reference to `config.yaml` in fix suggestion text

- [x] T05 [US1] Write tests for Config struct changes and YAML loading in `internal/config/config_test.go`
  - Test `DefaultConfig()` returns correct `Decisions.ExportDir` default
  - Test `Load()` reads `decisions.meetings` as `[]string` from viper
  - Test `Load()` reads `decisions.export_dir` as nested key
  - Test env var override for `GCAL_DECISIONS_MEETINGS` (comma-separated)

**Phase 1 Checkpoint**: `go build ./... && go test ./... && go vet ./...`

---

## Phase 2: Migration Logic (US1 Core)

_Depends on Phase 1 (Config struct must exist before migration can populate it)._

- [x] T06 [US1] Extract `ParseDotEnv()` function from `LoadDotEnv()` in `internal/config/dotenv.go`
  - Create `ParseDotEnv(path, home string) (map[string]string, error)` that returns key-value pairs
  - Refactor `LoadDotEnv()` to call `ParseDotEnv()` internally (reuse parsing logic, research.md D2)
  - Shared parsing: comment/blank skip, `SplitN`, quote stripping, tilde expansion, `ValidEnvKey` check

- [x] T07 [US1] Create migration logic in `internal/config/migrate.go`
  - Implement `MigrateEnvToYAML(envPath, yamlPath, home string) error` (data-model.md §5)
  - Define `secretEnvKeys` exclusion set: `GEMINI_API_KEY`, `GOOGLE_CREDENTIALS_FILE` (FR-003)
  - Define key mapping table: `.env` keys → YAML structure (data-model.md §1)
  - Handle `GCAL_FILENAME_KEYWORDS` comma-split → YAML list conversion
  - Handle `GCAL_DAYS_TO_LOOK_BACK` string → int conversion
  - Handle `GCAL_DECISIONS_EXPORT_DIR` → nested `decisions.export_dir`
  - Atomic write: temp file + `os.Rename` + verify readable + delete `.env` (research.md D2)
  - Log warnings for malformed/skipped lines (edge case from spec)

- [x] T08 [US1] Write tests for migration in `internal/config/migrate_test.go`
  - Test `ParseDotEnv()` returns correct key-value map
  - Test `ParseDotEnv()` handles quotes, tilde expansion, comments, blank lines
  - Test `ParseDotEnv()` skips malformed lines (no `=`, invalid key)
  - Test `MigrateEnvToYAML()` produces valid YAML with correct key mapping
  - Test `MigrateEnvToYAML()` excludes secret keys (FR-003)
  - Test `MigrateEnvToYAML()` splits comma-separated keywords into YAML list
  - Test `MigrateEnvToYAML()` deletes `.env` after successful write
  - Test `MigrateEnvToYAML()` preserves `.env` on write failure (error path)
  - Test `MigrateEnvToYAML()` is no-op when `.env` does not exist

**Phase 2 Checkpoint**: `go build ./... && go test ./... && go vet ./...`

---

## Phase 3: initConfig & Startup Integration (US1 Wiring)

_Depends on Phase 2 (migration functions must exist before wiring into startup)._

- [x] T09 [US1] Update `initConfig()` in `cmd/gcal-organizer/main.go` to support YAML config
  - Implement new flow (research.md D5, R4):
    - If `config.yaml` exists: `viper.SetConfigFile()` + `viper.ReadInConfig()`
    - Else if `.env` exists: call `LoadDotEnv()` (legacy path for first run), set migration-pending flag
    - Else: defaults + env vars only
  - Keep `viper.AutomaticEnv()` after config file loading (FR-005)
  - Update default `cfgFile` path from `.env` to `config.yaml` (research.md D6)
  - Update `--config` flag help text from `.env` to `config.yaml`

- [x] T10 [US1] Update `loadConfigAndStore()` in `cmd/gcal-organizer/main.go` to trigger migration
  - After `secrets.Migrate()` completes, call `config.MigrateEnvToYAML()` if migration-pending (research.md D5)
  - This ensures secrets are in keychain before `.env` is deleted

- [x] T11 [US1] Handle `--config` flag pointing to `.env` file in `cmd/gcal-organizer/main.go`
  - Detect file format by extension (research.md D6): `.env` or files named `.env` → migrate
  - `.yaml`/`.yml` → use directly
  - After migration, switch viper to the new YAML file

**Phase 3 Checkpoint**: `go build ./... && go test ./... && go vet ./...`

---

## Phase 4: Self-Service Commands (US1 Completion)

_Depends on Phase 3 (startup flow must work before updating init/doctor/install)._

- [x] T12 [US1] Update `initCmd` in `cmd/gcal-organizer/selfservice.go` to generate `config.yaml`
  - Replace `generateEnvFile()` call with new `generateConfigYAML()` function (research.md D7, FR-006)
  - Generate commented YAML with defaults (data-model.md §1 schema)
  - Include `decisions` section with commented-out `meetings` example
  - Omit secrets from generated file (API key stored in keychain)
  - Update file path from `.env` to `config.yaml`
  - Update user-facing messages to reference `config.yaml`

- [x] T13 [US1] Update `doctorCmd` in `cmd/gcal-organizer/selfservice.go` to check `config.yaml`
  - Change check #2 from `.env` to `config.yaml` (research.md D8, FR-007)
  - Add transitional check: if `.env` exists, warn it will be migrated on next run
  - Update fix suggestion messages from `.env` to `config.yaml`
  - Update `loadEnvValue()` fallback for GEMINI_API_KEY check to handle YAML config

- [x] T14 [P] [US1] Update service mode templates in `cmd/gcal-organizer/selfservice.go`
  - Remove `.env` sourcing block from `generateWrapper()` (FR-017, data-model.md §6)
  - Remove `EnvironmentFile` directive from `generateSystemdService()` (FR-018, data-model.md §6)

- [x] T15 [P] [US1] Update `.env` references in `internal/ux/ux.go`
  - Update `MissingAPIKey()` fix text: reference `config.yaml` instead of `.env`
  - Scan for any other `.env` references in ux messages

**Phase 4 Checkpoint**: `go build ./... && go test ./... && go vet ./...`

---

## Phase 5: Meeting Allowlist Filter (US2)

_Depends on Phase 1 (Config.Decisions.Meetings field must exist). Independent of Phases 2–4._

- [x] T16 [US2] Implement `ShouldExportDecisions()` filter function in `internal/export/export.go`
  - Signature: `ShouldExportDecisions(title string, allowlist []string) bool` (data-model.md §4)
  - Empty allowlist → return true (backward compatible, FR-011)
  - Non-empty allowlist → exact match, case-insensitive (FR-010)
  - No substring matching (acceptance scenario US2.3)

- [x] T17 [US2] Integrate allowlist filter in `cmd/gcal-organizer/main.go` decision extraction loop
  - In Step 4 loop, call `export.ShouldExportDecisions(docCtx.EventTitle, cfg.Decisions.Meetings)` before processing
  - Skip non-matching meetings with `continue` (data-model.md §4 integration point)
  - Log skipped meetings at debug level

- [x] T18 [US2] Write tests for `ShouldExportDecisions()` in `internal/export/export_test.go`
  - Test empty allowlist returns true for any title (FR-011)
  - Test nil allowlist returns true for any title
  - Test exact match returns true (FR-010)
  - Test case-insensitive match: "sprint planning" matches "Sprint Planning" (US2.4)
  - Test no substring match: "Sprint Planning - Q3 Kickoff" does NOT match "Sprint Planning" (US2.3)
  - Test multiple allowlist entries
  - Test special characters in meeting titles (edge case from spec)

**Phase 5 Checkpoint**: `go build ./... && go test ./... && go vet ./...`

---

## Phase 6: Per-Meeting Folders & Time Filenames (US3)

_Depends on Phase 1 (Config struct). Independent of Phases 2–5._

- [x] T19 [US3] Update `Export()` in `internal/export/export.go` for per-meeting subdirectories and time-based filenames
  - Change path generation: `<outputDir>/<slug>/<YYYY-MM-DDTHH-MM>.md` (FR-012, FR-013)
  - Use `meta.EventDate.Format("2006-01-02T15-04")` for filename (data-model.md §3)
  - Create per-meeting subdirectory with `mkdirAll` (FR-014)
  - Overwrite existing files (FR-015, idempotent)
  - Update dry-run log message to show new path format

- [x] T20 [US3] Update `renderMarkdown()` in `internal/export/export.go` to include `time` in frontmatter
  - Add `time: "HH:MM"` field to YAML frontmatter (data-model.md §3)
  - Use `date.Format("15:04")` for time value

- [x] T21 [US3] Update tests in `internal/export/export_test.go` for new path format and time frontmatter
  - Update `TestExporterExport` expected path from `weekly-sync-2026-04-18.md` to `weekly-sync/2026-04-18T14-00.md`
  - Update `TestExporterExport` to verify `mkdirAll` is called with subdirectory path
  - Add test for two meetings on same day at different times produce different filenames
  - Update `TestRenderMarkdown` to verify `time:` field in frontmatter
  - Add test for dry-run log shows new folder/time path format

**Phase 6 Checkpoint**: `go build ./... && go test ./... && go vet ./...`

---

## Phase 7: Google Doc Source Link (US4)

_Depends on Phase 6 (renderMarkdown changes). Can be done in parallel with Phase 5._

- [x] T22 [US4] Add `source` field to frontmatter in `renderMarkdown()` in `internal/export/export.go`
  - Accept `docID` parameter (from `meta.DocID`)
  - Generate URL: `https://docs.google.com/document/d/<docID>/edit` (FR-016, data-model.md §3)
  - Add `source:` field to YAML frontmatter after `attendees:`

- [x] T23 [US4] Update `Export()` to pass `docID` to `renderMarkdown()` in `internal/export/export.go`
  - Pass `meta.DocID` to `renderMarkdown()` for source URL generation

- [x] T24 [US4] Write tests for source link in `internal/export/export_test.go`
  - Test `renderMarkdown()` includes `source: https://docs.google.com/document/d/<docID>/edit`
  - Test source URL uses correct doc ID from metadata
  - Update existing `TestRenderMarkdown` cases to include docID parameter

**Phase 7 Checkpoint**: `go build ./... && go test ./... && go vet ./...`

---

## Phase 8: Documentation & Final Verification

_Depends on all previous phases._

- [x] T25 Update AGENTS.md with new config format and any new commands
  - Update "Active Technologies" section if needed
  - Update any `.env` references to `config.yaml`

- [x] T26 Final CI parity check
  - Run: `go mod tidy && git diff --exit-code go.mod go.sum`
  - Run: `gofmt -l .` (must produce no output)
  - Run: `go vet ./...`
  - Run: `go build ./cmd/gcal-organizer`
  - Run: `go test -v -race ./...`

**Phase 8 Checkpoint**: Full CI parity — all commands from `.github/workflows/ci.yml` pass.

---

## Summary

| Metric | Count |
|--------|-------|
| **Total Tasks** | 26 |
| **US1 (Config Migration)** | 15 |
| **US2 (Meeting Allowlist)** | 3 |
| **US3 (Folders + Time)** | 3 |
| **US4 (Source Link)** | 3 |
| **Cross-cutting** | 2 |
| **Phases** | 8 |
| **Parallelizable Tasks** | 3 (T03, T14, T15) |

### Phase Dependencies

```
Phase 1 (Config Struct) ──┬──→ Phase 2 (Migration) ──→ Phase 3 (Startup) ──→ Phase 4 (Self-Service)
                          ├──→ Phase 5 (Allowlist) [independent]
                          ├──→ Phase 6 (Folders/Time) ──→ Phase 7 (Source Link)
                          └──→ Phase 8 (Docs) [after all]
```

### Files Modified

| File | Tasks | Stories |
|------|-------|---------|
| `internal/config/config.go` | T01, T02 | US1 |
| `internal/config/config_test.go` | T05 | US1 |
| `internal/config/dotenv.go` | T06 | US1 |
| `internal/config/migrate.go` (NEW) | T07 | US1 |
| `internal/config/migrate_test.go` (NEW) | T08 | US1 |
| `cmd/gcal-organizer/main.go` | T03, T09, T10, T11, T17 | US1, US2 |
| `cmd/gcal-organizer/selfservice.go` | T12, T13, T14 | US1 |
| `internal/export/export.go` | T16, T19, T20, T22, T23 | US2, US3, US4 |
| `internal/export/export_test.go` | T18, T21, T24 | US2, US3, US4 |
| `internal/ux/ux.go` | T04, T15 | US1 |
| `cmd/gcal-organizer/auth_config.go` | T03 | US1 |
| `AGENTS.md` | T25 | — |

<!-- spec-review: passed -->
<!-- code-review: passed -->
