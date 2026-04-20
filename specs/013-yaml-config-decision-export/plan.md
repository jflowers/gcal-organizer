# Implementation Plan: YAML Config Migration & Decision Export Enhancements

**Branch**: `013-yaml-config-decision-export` | **Date**: 2026-04-20 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/013-yaml-config-decision-export/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

Migrate gcal-organizer from flat `.env` configuration to structured `config.yaml` using viper's native YAML support. This enables nested configuration (meeting allowlist) and list values that `.env` cannot express. The migration is automatic and one-time: on startup, if `.env` exists and `config.yaml` does not, the app parses `.env`, writes `config.yaml`, and deletes `.env`. Additionally, the decision export pipeline (feature 012) is enhanced with per-meeting folder organization, time-based filenames, and Google Doc source links in frontmatter.

## Technical Context

**Language/Version**: Go 1.24.0 (toolchain go1.24.12)
**Primary Dependencies**: `github.com/spf13/viper` (config), `github.com/spf13/cobra` (CLI), `github.com/charmbracelet/log` (logging), `github.com/zalando/go-keyring` (secrets) — all existing; no new dependencies
**Storage**: Local filesystem (`~/.gcal-organizer/config.yaml` replaces `~/.gcal-organizer/.env`). Secrets remain in OS keychain.
**Testing**: `go test ./...` (table-driven tests, injected I/O for testability)
**Target Platform**: macOS (primary), Linux (systemd service mode)
**Project Type**: Single CLI application (cmd/, internal/, pkg/)
**Performance Goals**: N/A — config loading is startup-only, not a hot path
**Constraints**: Zero-downtime migration (existing users must not lose configuration). Backward compatibility with env var overrides (FR-005).
**Scale/Scope**: ~8 files modified, ~2 new files, 4 coordinated changes

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. CLI-First Architecture | PASS | Config file change is transparent to CLI interface. `--config` flag updated to accept YAML path. |
| II. API-Key Authentication Mode | PASS | Secrets remain in keychain. Migration explicitly excludes secrets from `config.yaml` (FR-003). |
| III. Test-Driven Development | PASS | All new functions (migration, allowlist filtering, path generation) will have table-driven tests. Existing Exporter tests use injected I/O. |
| IV. Idiomatic Go | PASS | Uses viper's native YAML support (standard Go config pattern). No new external dependencies. |
| V. Graceful Error Handling | PASS | Migration logs warnings for malformed lines. Config loading wraps errors with context. |
| VI. Observability | PASS | Migration logs each step. Dry-run mode updated for new folder structure. |
| VII. Self-Serve Diagnostics | PASS | `doctor` command updated to check for `config.yaml` instead of `.env` (FR-007). |
| Documentation Requirements | PASS | README.md, SETUP.md must be updated with new config format. |
| Quality Gates | PASS | `go build`, `go test`, `go vet`, `gofmt`, `go mod tidy` — no new deps, no gate changes. |
| YAGNI | PASS | All four changes are spec'd and motivated by concrete user needs. |

**Gate Result**: PASS — No violations. Proceed to Phase 0.

## Project Structure

### Documentation (this feature)

```text
specs/013-yaml-config-decision-export/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
internal/
├── config/
│   ├── config.go          # MODIFY: Add Decisions nested struct, YAML loading, migration logic
│   ├── dotenv.go          # MODIFY: Reuse parsing for migration, deprecate LoadDotEnv for normal startup
│   ├── migrate.go         # NEW: .env → config.yaml migration logic (parse, map, write, delete)
│   └── config_test.go     # NEW/MODIFY: Tests for YAML loading, migration, allowlist
├── export/
│   ├── export.go          # MODIFY: Per-meeting folders, time filenames, source frontmatter
│   └── export_test.go     # MODIFY: Update tests for new path format and source field
├── secrets/
│   ├── file.go            # MODIFY: readEnvValue still needed for FileStore fallback
│   └── migrate.go         # REVIEW: Secret migration interacts with .env deletion timing
└── organizer/
    └── organizer.go       # MODIFY: Add meeting allowlist filtering before decision export

cmd/gcal-organizer/
├── main.go                # MODIFY: initConfig() switches from LoadDotEnv to YAML, --config flag
└── selfservice.go         # MODIFY: init generates config.yaml, doctor checks config.yaml, wrapper script
```

**Structure Decision**: Existing Go project layout (cmd/, internal/, pkg/) is preserved. New migration logic goes in `internal/config/migrate.go` to keep config.go focused on the Config struct and loading. No new packages needed.

## Complexity Tracking

> No violations to justify — Constitution Check passed cleanly.
