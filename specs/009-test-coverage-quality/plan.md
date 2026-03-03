# Implementation Plan: Comprehensive Test Coverage & Contract Quality

**Branch**: `009-test-coverage-quality` | **Date**: 2026-03-02 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/009-test-coverage-quality/spec.md`

## Summary

Achieve comprehensive test coverage and contract quality across all internal packages by: (1) adding `.gaze.yaml` configuration to unlock contract coverage measurement, (2) renaming test helper constructors to resolve multi-target detection, (3) adding httptest-based tests for docs, gemini, and calendar API functions, (4) adding direct return-value/receiver-field assertions for high-GazeCRAP functions, and (5) extracting testable pure functions from the CLI package. The goal is to move from 29.4% line coverage / 0% contract coverage to >55% line / >50% contract, eliminating all GazeCRAP Q4 functions and reducing CRAPload below 15.

## Technical Context

**Language/Version**: Go 1.24.0 (toolchain go1.24.12), module `github.com/jflowers/gcal-organizer`
**Primary Dependencies**: `google.golang.org/api` (Drive v3, Docs v1, Calendar v3), `google.golang.org/genai` (Gemini), `github.com/spf13/cobra` (CLI), `github.com/spf13/viper` (config), `github.com/zalando/go-keyring` (secrets)
**Storage**: N/A (no new data persistence; this feature only adds tests and configuration)
**Testing**: `go test -race ./...` via `make ci`; `gaze crap/quality/analyze` for quality metrics
**Target Platform**: macOS / Linux (CLI tool)
**Project Type**: Single Go module with standard layout (cmd/, internal/, pkg/)
**Performance Goals**: N/A (test-only changes; no runtime performance impact)
**Constraints**: No new external dependencies (FR-013); all mocks use standard library `net/http/httptest`
**Scale/Scope**: 11 internal packages, 12 existing test files, 139 functions analyzed by gaze, ~55 exported functions

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. CLI-First Architecture | PASS | No CLI changes; tests only |
| II. API-Key Authentication Mode | PASS | No auth changes; mock credentials in tests only |
| III. Test-Driven Development | PASS | This feature IS test development |
| IV. Idiomatic Go | PASS | Table-driven tests, standard project layout, `context.Context` |
| V. Graceful Error Handling | PASS | Tests verify error wrapping with `%w` |
| VI. Observability | PASS | No changes to logging/observability |
| VII. Self-Serve Diagnostics | PASS | No changes to diagnostic commands |

**Quality Gates**:

| Gate | Status | Notes |
|------|--------|-------|
| `go build ./...` | PASS | No production code changes except pure function extractions (US6) |
| `go test ./...` | PASS | All new tests must pass |
| `go vet ./...` | PASS | No warnings expected from test code |
| `gofmt` | PASS | All code formatted |
| `go mod tidy` | PASS | No new dependencies |
| `make ci` | PASS | All checks pass |

**Constraint Violations**: None. This feature aligns with all constitution principles — it is primarily a testing/quality initiative with minimal production code changes (only US6 extractions modify source).

## Project Structure

### Documentation (this feature)

```text
specs/009-test-coverage-quality/
├── plan.md              # This file
├── research.md          # Phase 0: gaze behavior, helper naming, httptest patterns
├── data-model.md        # Phase 1: test helper taxonomy and renaming map
├── quickstart.md        # Phase 1: developer guide for running quality checks
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
.gaze.yaml                                    # NEW: gaze quality tool configuration (FR-017)

internal/
├── organizer/
│   └── organizer_test.go                     # MODIFIED: rename helpers, add contract assertions
├── docs/
│   └── service_test.go                       # MODIFIED: rename helpers, add httptest tests for CreateDecisionsTab
├── gemini/
│   └── client_test.go                        # MODIFIED: add httptest tests for ExtractDecisions, ExtractAssigneesFromCheckboxes
├── calendar/
│   └── service_test.go                       # MODIFIED: add httptest tests for ListRecentEvents
├── drive/
│   └── service_test.go                       # MODIFIED: rename mockService helper
├── config/
│   ├── config_test.go                        # EXISTING: minor assertion improvements
│   └── dotenv_test.go                        # EXISTING: already comprehensive
├── retry/
│   └── retry_test.go                         # EXISTING: already comprehensive
├── secrets/
│   └── store_test.go                         # EXISTING: already comprehensive
└── ux/
    ├── ux_test.go                            # EXISTING: already at 100%
    └── format_test.go                        # EXISTING: already at 100%
```

**Structure Decision**: Standard Go test layout — test files co-located with source in each package. No separate test directories. All new tests are additions to existing `*_test.go` files or new `*_test.go` files in existing packages. The only new non-test file is `.gaze.yaml` at repository root.

## Complexity Tracking

No constitution violations to justify.
