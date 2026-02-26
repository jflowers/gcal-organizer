# Implementation Plan: Decision Extraction from Meeting Transcripts

**Branch**: `008-decision-extraction` | **Date**: 2026-02-26 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/008-decision-extraction/spec.md`

## Summary

Extract decisions (made, deferred, open) from meeting transcript documents attached to calendar events and write them into a new "Decisions" tab in the same document with cross-tab links to transcript timestamp headings. Integrates into the existing `run` workflow as Step 4, using the Google Docs BatchUpdate API for tab creation/content insertion and the Gemini API for transcript analysis.

## Technical Context

**Language/Version**: Go 1.24+ (module `github.com/jflowers/gcal-organizer`)
**Primary Dependencies**: `google.golang.org/api/docs/v1` (Docs API — tab creation, content insertion, heading links), `google.golang.org/genai` (Gemini SDK — transcript analysis), `github.com/spf13/cobra` (CLI), `github.com/spf13/viper` (config)
**Storage**: N/A (no new data persistence; decisions written directly to Google Docs)
**Testing**: `go test` with table-driven tests; mock-based integration tests for Docs API interactions
**Target Platform**: macOS (primary), Linux (secondary) — CLI tool
**Project Type**: Single project — extends existing `cmd/`, `internal/`, `pkg/` structure
**Performance Goals**: <30 seconds per document (SC-001); dominated by Gemini API latency (~5-15s for transcript analysis)
**Constraints**: 3 Google API calls per document (1 GET + 2 BatchUpdate); Gemini 2.0 Flash 1M token context window for full transcripts
**Scale/Scope**: Typically 1-10 documents per hourly run; transcript length 5-120 minutes

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. CLI-First Architecture | PASS | Integrates into existing `run` command; no new commands required |
| II. API-Key Authentication | PASS | Reuses existing OAuth2 for Docs API + GCP API key for Gemini |
| III. Test-Driven Development | PASS | Tests before implementation; table-driven tests for Gemini response parsing, mock-based tests for Docs API |
| IV. Idiomatic Go | PASS | Standard project layout; new code in `internal/docs/`, `internal/gemini/`, `internal/organizer/` |
| V. Graceful Error Handling | PASS | FR-017 (skip on AI failure), FR-018 (optimistic concurrency), error wrapping with `%w` |
| VI. Observability | PASS | Structured logging for each document processed; dry-run mode support (FR-013) |
| VII. Self-Serve Diagnostics | PASS | Errors include `Run 'gcal-organizer doctor'` guidance per constitution |
| Quality Gates | PASS | `go build`, `go test`, `go vet`, `gofmt`, `go mod tidy` |
| Documentation | PASS | README.md + man page updates required before completion |
| YAGNI | PASS | No new CLI commands, no new config options, no new dependencies |

**Pre-design gate: PASS** — No violations. No complexity tracking needed.

## Project Structure

### Documentation (this feature)

```text
specs/008-decision-extraction/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (repository root)

```text
gcal-organizer/
├── cmd/gcal-organizer/
│   └── main.go                    # Wire Step 4 into run command
├── internal/
│   ├── docs/
│   │   └── service.go             # New methods: ExtractTranscriptContent, HasDecisionsTab, CreateDecisionsTab
│   ├── gemini/
│   │   ├── client.go              # New method: ExtractDecisions
│   │   └── client_test.go         # New tests: TestExtractDecisionsResponse
│   └── organizer/
│       ├── organizer.go           # New: decisionDocIDs collection, GetDecisionDocIDs(), DocsService interface
│       └── organizer_test.go      # New tests for decision doc collection
└── pkg/models/
    └── models.go                  # New: Decision, TranscriptHeading structs
```

**Structure Decision**: Extends existing packages only. No new packages created. The `internal/docs/` service gains write capability (previously read-only). The organizer gains a new `DocsService` interface for the decision extraction flow.

## API Call Pattern

Each document requires 3 Google API calls:

1. **GET `Documents.Get(docID)`** — Read full document with all tabs
   - Find Transcript tab → extract text + H3 heading IDs
   - Check for existing Decisions tab → skip if present (FR-005)

2. **BatchUpdate #1: `AddDocumentTab`** — Create "Decisions" tab
   - Response returns server-assigned `TabId`
   - If API rejects (duplicate tab), treat as already processed (FR-018)

3. **BatchUpdate #2: Content insertion** — Target new tab via `TabId`
   - `InsertText` — section headings + decision bullet text
   - `UpdateParagraphStyle` — style H2 headings
   - `CreateParagraphBullets` — bullet each decision
   - `UpdateTextStyle` — apply cross-tab `HeadingLink` to timestamp text

Plus 1 Gemini API call between steps 1 and 2:
- `GenerateContent` — full transcript text → structured JSON decisions

## Post-Design Constitution Re-Check

| Principle | Status | Notes |
|-----------|--------|-------|
| I. CLI-First Architecture | PASS | No new commands; Step 4 integrated into `run` |
| II. API-Key Authentication | PASS | No new auth flows |
| III. Test-Driven Development | PASS | Tests planned for all new functions |
| IV. Idiomatic Go | PASS | No new dependencies; extends existing interfaces |
| V. Graceful Error Handling | PASS | Skip-on-failure + optimistic concurrency |
| VI. Observability | PASS | Per-document logging + dry-run support |
| VII. Self-Serve Diagnostics | PASS | Error messages include doctor reference |
| Documentation | PASS | README, AGENTS.md updates tracked |

**Post-design gate: PASS**
