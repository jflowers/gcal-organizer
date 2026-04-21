# Implementation Plan: Local AI via Ollama with IBM Granite

**Branch**: `014-local-ai-ollama` | **Date**: 2026-04-20 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/014-local-ai-ollama/spec.md`

## Summary

Add a local AI layer using Ollama with IBM Granite models to gcal-organizer. The feature introduces three capabilities: (1) a sensitivity gate that classifies every meeting transcript before processing and skips sensitive content, (2) local task assignment that replaces Gemini for assignee extraction, and (3) an optional local-only mode that eliminates all cloud AI dependencies. The implementation uses raw HTTP to Ollama's REST API (following the proven Dewey pattern), with hard-stop behavior when Ollama is configured but unavailable. Doctor and init commands are extended to validate Ollama setup.

## Technical Context

**Language/Version**: Go 1.24.0 (toolchain go1.24.12)
**Primary Dependencies**: `net/http` (Ollama REST API — no SDK), `github.com/spf13/cobra` (CLI), `github.com/spf13/viper` (config), `github.com/charmbracelet/log` (logging), `github.com/charmbracelet/huh` (interactive prompts)
**Storage**: N/A (no new data persistence; configuration via existing YAML config file)
**Testing**: `go test -race -count=1 ./...` with `net/http/httptest` for Ollama API mocking
**Target Platform**: macOS (primary), Linux (secondary)
**Project Type**: Single Go CLI application
**Performance Goals**: Sensitivity classification completes within Ollama's configured timeout (default 120s); doctor checks complete in <3 seconds (SC-005)
**Constraints**: No new Go dependencies (raw HTTP only — FR-012); hard-stop on Ollama unavailability (FR-009); all tests must pass under race detector (TC-005)
**Scale/Scope**: 4 new source files in `internal/ollama/`, modifications to config, organizer, and selfservice

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. CLI-First Architecture | ✅ PASS | All Ollama features exposed via existing CLI commands (`run`, `doctor`, `init`). New config via YAML. No new commands needed. |
| II. API-Key Authentication Mode | ✅ PASS | Ollama uses no authentication (local HTTP). Gemini API key handling unchanged. No new credentials to manage. |
| III. Test-Driven Development | ✅ PASS | All Ollama HTTP interactions testable via `httptest.NewServer`. No external dependencies needed for testing. Follows Dewey's proven test pattern. |
| IV. Idiomatic Go | ✅ PASS | Standard `net/http` client, `context.Context` for timeouts, `fmt.Errorf` with `%w` for error wrapping. No new external dependencies (FR-012). |
| V. Graceful Error Handling | ✅ PASS | Hard-stop with actionable error messages when Ollama unavailable (FR-008/FR-009). Error messages include specific fix steps (install, start, pull commands). |
| VI. Observability | ✅ PASS | Sensitivity classification logged at INFO (category + score + doc URL), reasoning at DEBUG only (FR-004). Dry-run mode supported (FR-007). |
| VII. Self-Serve Diagnostics | ✅ PASS | Doctor command extended with 4 new checks (FR-023–FR-027). Init command extended with model pull prompt (FR-028). |

**Quality Gates**:
| Gate | Status | Notes |
|------|--------|-------|
| `go build ./...` | Will verify | New package must compile |
| `go test ./...` | Will verify | All new tests must pass |
| `go vet ./...` | Will verify | No warnings |
| `gofmt` | Will verify | All new files formatted |
| `go mod tidy` | Will verify | No new dependencies expected |
| `make ci` | Will verify | Must mirror CI exactly |

**No constitution violations. No complexity tracking needed.**

## Project Structure

### Documentation (this feature)

```text
specs/014-local-ai-ollama/
├── plan.md              # This file
├── research.md          # Phase 0: Ollama API patterns, Granite models, prompt engineering
├── data-model.md        # Phase 1: Types, interfaces, config schema
├── quickstart.md        # Phase 1: Integration guide
├── checklists/
│   └── requirements.md  # Pre-existing spec quality checklist
└── tasks.md             # Phase 2 output (/speckit.tasks — NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
internal/
├── ollama/
│   ├── client.go         # OllamaClient: HTTP client, health check, model availability
│   ├── guardian.go        # SensitivityClassifier: granite-guardian via /api/generate
│   ├── assigner.go        # TaskAssigner: granite3.2:8b for assignee extraction
│   ├── decisions.go       # DecisionExtractor: granite3.2:8b for local-only decisions
│   ├── client_test.go     # Client tests with httptest
│   ├── guardian_test.go   # Sensitivity classifier tests
│   ├── assigner_test.go   # Task assigner tests
│   └── decisions_test.go  # Decision extractor tests
├── config/
│   └── config.go          # MODIFIED: Add OllamaConfig nested struct
└── organizer/
    └── organizer.go       # MODIFIED: Add sensitivity gate to pipeline, swap AI services

cmd/gcal-organizer/
├── selfservice.go         # MODIFIED: Add 4 doctor checks, init model pull prompt
└── main.go                # MODIFIED: Wire OllamaClient into pipeline

pkg/models/
└── models.go              # MODIFIED: Add SensitivityResult struct
```

**Structure Decision**: Follows existing project layout. New `internal/ollama/` package is a leaf dependency (imports only stdlib + `pkg/models` + `internal/config`). Mirrors the Dewey `llm/` package pattern: separate files for client, classifier, and assigner per Single Responsibility Principle.

## Complexity Tracking

> No constitution violations. Table intentionally empty.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
