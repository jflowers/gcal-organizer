# Implementation Plan: Decision Markdown Export

**Branch**: `012-decision-markdown-export` | **Date**: 2026-04-19 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/012-decision-markdown-export/spec.md`

## Summary

Add a local markdown export step to the existing decision extraction pipeline (spec 008). After writing decisions to a Google Docs "Decisions" tab, the system also writes a markdown copy to a configurable local directory (`~/.gcal-organizer/decisions/` by default). Each file uses YAML frontmatter (`topic`, `date`, `attendees`) and standard markdown headings (`## Decisions Made`, `## Decisions Deferred`, `## Open Items`), making files natively indexable by Dewey and other semantic search tools.

The implementation hooks into `organizer.ExtractDecisionsForDoc()` — the existing orchestration point — and adds a new `internal/export` package responsible for rendering and writing markdown files. No new CLI commands or flags are introduced beyond a new config key (`decisions_export_dir`). The `--dry-run` flag already threaded through the pipeline suppresses file writes.

## Technical Context

**Language/Version**: Go 1.24.0 (toolchain go1.24.12)
**Primary Dependencies**: `github.com/spf13/cobra` (CLI), `github.com/spf13/viper` (config), `github.com/charmbracelet/log` (logging) — all existing; no new dependencies
**Storage**: Local filesystem (`~/.gcal-organizer/decisions/` default). No database.
**Testing**: `go test -race ./...` — table-driven tests, mock interfaces for filesystem operations
**Target Platform**: macOS (primary), Linux (secondary) — same as existing CLI
**Project Type**: Single CLI application
**Performance Goals**: N/A — file writes are negligible compared to Google API calls
**Constraints**: No new external dependencies. Must not break existing decision extraction pipeline. Local export failure must not block Google Docs tab creation (FR-012).
**Scale/Scope**: Typically 1–10 decision files per run (one per meeting with decisions)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. CLI-First Architecture | ✅ PASS | No new commands; hooks into existing `run` workflow. Config via viper. |
| II. API-Key Authentication | ✅ PASS | No new auth. Reuses existing pipeline context. |
| III. Test-Driven Development | ✅ PASS | All new code (export package, slug generation, markdown rendering) will have unit tests. |
| IV. Idiomatic Go | ✅ PASS | New `internal/export` package follows standard layout. Error returns, `context.Context`, no panics. |
| V. Graceful Error Handling | ✅ PASS | FR-012: export failure logs warning, does not block pipeline. Errors wrapped with `fmt.Errorf` + `%w`. |
| VI. Observability | ✅ PASS | FR-011: `--dry-run` suppresses writes and logs what would happen. Export actions logged. |
| VII. Self-Serve Diagnostics | ✅ PASS | No new diagnostic commands needed. |
| Quality Gates | ✅ PASS | `make ci` mirrors GitHub Actions. No gate modifications. |
| Documentation | ✅ PASS | README.md, config examples updated. |
| YAGNI | ✅ PASS | Writes files only. No Dewey API integration. Zero coupling. |

**Post-Phase 1 Re-check**: All gates still pass. The `internal/export` package is a pure function layer (decisions in → file out) with injected filesystem, fully testable without external resources.

## Project Structure

### Documentation (this feature)

```text
specs/012-decision-markdown-export/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
└── tasks.md             # Phase 2 output (created by /speckit.tasks)
```

### Source Code (repository root)

```text
internal/
├── export/              # NEW — markdown export package
│   ├── export.go        # DecisionExporter: render + write markdown files
│   ├── export_test.go   # Table-driven tests for rendering and file writing
│   ├── slug.go          # Topic slug generation (kebab-case, filename-safe)
│   └── slug_test.go     # Slug generation tests
├── config/
│   └── config.go        # MODIFIED — add DecisionsExportDir field + viper binding
├── organizer/
│   └── organizer.go     # MODIFIED — call export after CreateDecisionsTab
└── docs/
    └── service.go       # READ ONLY — reuse buildDecisionsContent() pattern

cmd/gcal-organizer/
└── main.go              # MODIFIED — pass config to organizer for export dir

pkg/models/
└── models.go            # READ ONLY — reuse Decision, CalendarEvent, Attendee types
```

**Structure Decision**: New `internal/export` package follows the existing pattern of domain-specific packages (`internal/docs`, `internal/drive`, `internal/gemini`). The export logic is isolated from Google API concerns, making it independently testable. No new top-level directories needed.

## Complexity Tracking

> No violations to justify. All gates pass cleanly.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| (none) | — | — |
