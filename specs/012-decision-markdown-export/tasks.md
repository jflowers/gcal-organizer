# Tasks: Decision Markdown Export

**Input**: Design documents from `/specs/012-decision-markdown-export/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, quickstart.md

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the new `internal/export` package and establish the project structure for this feature.

- [x] T001 Create `internal/export/` package directory with `export.go` stub (package declaration, GoDoc comment, imports placeholder)
- [x] T002 [P] Create `internal/export/slug.go` stub (package declaration, GoDoc comment)
- [x] T003 [P] Create `internal/export/export_test.go` stub (package declaration, test imports)
- [x] T004 [P] Create `internal/export/slug_test.go` stub (package declaration, test imports)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core data model changes and slug generation that ALL user stories depend on. These modify shared types and provide the filename generation logic used by every export operation.

**CRITICAL**: No user story work can begin until this phase is complete.

- [x] T005 Add `DecisionDocContext` struct to `internal/organizer/organizer.go` with fields: `DocID string`, `Source string`, `EventTitle string`, `EventDate time.Time`, `Attendees []string` (per data-model.md entity definition)
- [x] T006 Refactor `Organizer.decisionDocIDs` field from `map[string]string` to `map[string]DecisionDocContext` in `internal/organizer/organizer.go` and update `New()` initializer
- [x] T007 Refactor `GetDecisionDocIDs()` to `GetDecisionDocContexts()` returning `map[string]DecisionDocContext` in `internal/organizer/organizer.go`
- [x] T008 Update `SyncCalendarAttachments()` in `internal/organizer/organizer.go` to populate `DecisionDocContext` with `EventTitle`, `EventDate`, and `Attendees` (filtering out self and resource attendees) instead of plain string source
- [x] T009 Update Step 4 loop in `cmd/gcal-organizer/main.go` to call `GetDecisionDocContexts()` instead of `GetDecisionDocIDs()` and pass `DecisionDocContext` to `ExtractDecisionsForDoc()`
- [x] T010 Update `ExtractDecisionsForDoc()` signature in `internal/organizer/organizer.go` to accept `DecisionDocContext` instead of bare `docID string` (extract `docID` from context struct internally)
- [x] T011 Implement `TopicSlug(title string) string` function in `internal/export/slug.go` per slug generation rules in data-model.md: strip prefixes ("Notes by Gemini"), strip suffixes ("- Transcript"), lowercase, replace non-alphanumeric with hyphens, collapse consecutive hyphens, trim, fallback to "meeting" for empty result
- [x] T012 [P] Write table-driven tests for `TopicSlug()` in `internal/export/slug_test.go` covering all examples from data-model.md: "Weekly Engineering Sync", "Notes by Gemini", "Project Alpha - Transcript", "Q3 Budget / Planning", "Design Review: API v2", empty string

**Checkpoint**: Foundation ready — `DecisionDocContext` flows through the pipeline, slug generation works. User story implementation can now begin.

---

## Phase 3: User Story 1 — Export Decisions as Local Markdown (Priority: P1)

**Goal**: After writing decisions to Google Docs, also write a markdown copy to `~/.gcal-organizer/decisions/` with correct content structure (three category sections as bullet lists). This is the core value proposition — unlocking decisions from Google Docs into local files.

**Independent Test**: Run `gcal-organizer run` against a calendar with transcript-bearing events and verify markdown files appear in `~/.gcal-organizer/decisions/` with correct content matching the Google Docs Decisions tab.

### Implementation for User Story 1

- [x] T013 [US1] Define `Exporter` struct in `internal/export/export.go` with injectable dependencies: `writeFile func(string, []byte, os.FileMode) error`, `mkdirAll func(string, os.FileMode) error`, `logger *log.Logger`, `outputDir string` (per research.md D4)
- [x] T014 [US1] Implement `NewExporter(outputDir string, logger *log.Logger) *Exporter` constructor in `internal/export/export.go` that defaults `writeFile` to `os.WriteFile` and `mkdirAll` to `os.MkdirAll`
- [x] T015 [US1] Implement `renderMarkdown(decisions []models.Decision, topic string, date time.Time, attendees []string) []byte` internal function in `internal/export/export.go` that produces YAML frontmatter (`topic`, `date`) and three category sections (`## Decisions Made`, `## Decisions Deferred`, `## Open Items`) with bullet items, omitting empty categories (FR-015)
- [x] T016 [US1] Implement `Exporter.Export(ctx context.Context, decisions []models.Decision, meta DecisionDocContext, dryRun bool) error` method in `internal/export/export.go`: generate filename via `TopicSlug(meta.EventTitle)` + date, call `mkdirAll` for output dir (FR-007), call `writeFile` with rendered markdown (FR-001, FR-008), log warning on write failure without returning error (FR-012)
- [x] T017 [US1] Implement dry-run behavior in `Exporter.Export()`: when `dryRun` is true, log "Would export decisions to <path>" and return nil without writing (FR-011, research.md D7)
- [x] T018 [US1] Add export hook in `ExtractDecisionsForDoc()` in `internal/organizer/organizer.go`: after successful `CreateDecisionsTab()`, call `exporter.Export()` passing decisions and `DecisionDocContext`; log warning on failure but do not return error (FR-012, research.md D1)
- [x] T019 [US1] Wire `Exporter` creation in `cmd/gcal-organizer/main.go`: create `export.NewExporter()` with default output dir (`~/.gcal-organizer/decisions/`) before the Step 4 loop, pass to `ExtractDecisionsForDoc()` or set on Organizer
- [x] T020 [US1] Add `DecisionsExported int` and `DecisionsExportFailed int` fields to `Stats` struct in `internal/organizer/organizer.go` and increment them in the export hook
- [x] T021 [US1] Update `printSummary()` in `internal/organizer/organizer.go` to display export stats (`decisions_exported`, `decisions_export_failed`) when non-zero
- [x] T022 [US1] Write table-driven tests for `renderMarkdown()` in `internal/export/export_test.go`: all three categories populated, single category only, empty categories omitted, decisions with special characters
- [x] T023 [US1] Write tests for `Exporter.Export()` in `internal/export/export_test.go` using injected mock `writeFile`/`mkdirAll`: successful write, directory creation, overwrite existing file (FR-008), write failure logs warning (FR-012), dry-run suppresses write (FR-011)
- [x] T024 [US1] Write test for zero-decisions case in `internal/export/export_test.go`: verify no file is created when `decisions` slice is empty (edge case from spec)
- [x] T024b [US1] Write integration test for `ExtractDecisionsForDoc()` in `internal/organizer/` verifying that the exporter is called after `CreateDecisionsTab()` succeeds, and that export failure does not cause `ExtractDecisionsForDoc()` to return an error (FR-012)

**Checkpoint**: At this point, User Story 1 should be fully functional — `gcal-organizer run` produces markdown files in the default directory with correct content structure.

---

## Phase 4: User Story 2 — Configure Export Directory (Priority: P2)

**Goal**: Allow users to configure where decision markdown files are saved via the config file or `GCAL_DECISIONS_EXPORT_DIR` environment variable, defaulting to `~/.gcal-organizer/decisions/`.

**Independent Test**: Set `GCAL_DECISIONS_EXPORT_DIR=~/Documents/meeting-decisions`, run `gcal-organizer run`, and verify files appear at the custom location.

### Implementation for User Story 2

- [x] T025 [US2] Add `DecisionsExportDir string` field to `Config` struct in `internal/config/config.go` with default `~/.gcal-organizer/decisions/` set in `DefaultConfig()`
- [x] T026 [US2] Add `mustBindEnv("decisions_export_dir", "GCAL_DECISIONS_EXPORT_DIR")` and viper override in `Load()` in `internal/config/config.go`, following the existing pattern for other config keys
- [x] T027 [US2] Add tilde (`~`) expansion for `DecisionsExportDir` in `Load()` in `internal/config/config.go`, consistent with how `CredentialsFile` and `TokenFile` paths are handled
- [x] T028 [US2] Update `Exporter` creation in `cmd/gcal-organizer/main.go` to use `cfg.DecisionsExportDir` instead of hardcoded default path
- [x] T029 [US2] Write test in `internal/config/config_test.go` (or appropriate test file) verifying `DefaultConfig()` sets `DecisionsExportDir` to `~/.gcal-organizer/decisions/`
- [x] T030 [US2] Write test for tilde expansion of `DecisionsExportDir` verifying `~/foo` resolves to an absolute path

**Checkpoint**: At this point, User Stories 1 AND 2 should both work — files are written to the configured directory (or default if unconfigured).

---

## Phase 5: User Story 3 — Include Meeting Metadata in Frontmatter (Priority: P3)

**Goal**: Enrich exported markdown files with YAML frontmatter containing `attendees` list from the calendar event, enabling search tools to filter by meeting participants.

**Independent Test**: Export a decision file and verify the YAML frontmatter contains the correct `attendees` list parsed from the calendar event. Verify `attendees` is omitted when no attendees are available.

### Implementation for User Story 3

- [x] T031 [US3] Update `renderMarkdown()` in `internal/export/export.go` to include `attendees` field in YAML frontmatter when the attendees slice is non-empty (FR-006); omit the field entirely when empty
- [x] T032 [US3] Implement topic cleaning in `renderMarkdown()` or a helper in `internal/export/slug.go`: strip "- Transcript" suffix and "Notes by Gemini" prefix from the topic used in frontmatter (FR-013), distinct from the slug used in filenames
- [x] T033 [US3] Write tests in `internal/export/export_test.go` for frontmatter with attendees: verify YAML `attendees` list renders correctly with multiple attendees
- [x] T034 [US3] Write test in `internal/export/export_test.go` for frontmatter without attendees: verify `attendees` field is omitted (not an empty list) when attendees slice is empty
- [x] T035 [US3] Write test in `internal/export/export_test.go` for topic cleaning: verify "Weekly Sync - Transcript" produces frontmatter `topic: Weekly Sync` and "Notes by Gemini - Project Alpha" produces `topic: Project Alpha`

**Checkpoint**: All user stories should now be independently functional — exported files have full metadata, configurable output, and correct content.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, summary reporting, and validation across all stories.

- [x] T036 [P] Update dry-run summary in `printSummary()` in `internal/organizer/organizer.go` to include "Would export decisions" count alongside existing dry-run stats
- [x] T037 [P] Add `decisions_export_dir` to `config show` output if a config show command exists (check `cmd/gcal-organizer/auth_config.go` or equivalent)
- [x] T038 Run `go build ./...` to verify compilation succeeds with all changes
- [x] T039 Run `go test -race ./...` to verify all tests pass with race detector
- [x] T040 Run `go vet ./...` to verify no vet warnings
- [x] T041 Run `make ci` to verify full CI parity (per AGENTS.md CI Parity Gate)
- [ ] T042 Run quickstart.md verification checklist: dry-run output, full run file creation, YAML frontmatter validity, empty category omission, custom export directory, error resilience
- [x] T043 Update README.md to document the `decisions_export_dir` configuration option, the decision markdown export behavior, and the `GCAL_DECISIONS_EXPORT_DIR` environment variable

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 completion — BLOCKS all user stories
- **User Stories (Phases 3–5)**: All depend on Phase 2 completion
  - US1 (Phase 3) must complete before US2 (Phase 4) — US2 modifies how the exporter is created
  - US3 (Phase 5) can start after Phase 2 but integrates with US1's `renderMarkdown()`
  - Recommended order: US1 → US2 → US3 (sequential by priority)
- **Polish (Phase 6)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Phase 2. Creates the `Exporter`, `renderMarkdown()`, and the pipeline hook. All other stories build on this.
- **User Story 2 (P2)**: Depends on US1 (T019 creates the exporter with a hardcoded path; T028 replaces it with config). Can be implemented immediately after US1.
- **User Story 3 (P3)**: Depends on US1 (T015 creates `renderMarkdown()`; T031 extends it with attendees). Can be implemented after US1.

### Within Each User Story

- Struct/type definitions before method implementations
- Core logic before integration hooks
- Implementation before tests (tests validate the implementation)
- Pipeline wiring (main.go, organizer.go hooks) after core logic

### Parallel Opportunities

- Phase 1: T002, T003, T004 can run in parallel with each other (after T001)
- Phase 2: T011 and T012 (slug) can run in parallel with T005–T010 (data model refactor) since they touch different files
- Phase 6: T036 and T037 can run in parallel (different files)

---

## Parallel Example: Phase 2 (Foundational)

```bash
# Stream 1: Data model refactor (sequential — same file)
Task T005: "Add DecisionDocContext struct to internal/organizer/organizer.go"
Task T006: "Refactor decisionDocIDs to map[string]DecisionDocContext"
Task T007: "Refactor GetDecisionDocIDs to GetDecisionDocContexts"
Task T008: "Update SyncCalendarAttachments to populate DecisionDocContext"
Task T009: "Update Step 4 loop in cmd/gcal-organizer/main.go"
Task T010: "Update ExtractDecisionsForDoc signature"

# Stream 2: Slug generation (parallel — different files)
Task T011: "Implement TopicSlug in internal/export/slug.go"
Task T012: "Write slug tests in internal/export/slug_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (4 tasks)
2. Complete Phase 2: Foundational (8 tasks)
3. Complete Phase 3: User Story 1 (12 tasks)
4. **STOP and VALIDATE**: Run `go test -race ./...` and verify markdown files are produced
5. Deploy/demo if ready — decisions are now exported locally

### Incremental Delivery

1. Setup + Foundational -> Foundation ready
2. Add User Story 1 -> Test independently -> Decisions exported to default dir (MVP!)
3. Add User Story 2 -> Test independently -> Configurable export directory
4. Add User Story 3 -> Test independently -> Rich metadata in frontmatter
5. Polish -> CI validation -> Feature complete

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- No new external dependencies — all libraries are already in go.mod
- FR-012 is the critical resilience requirement: export failure MUST NOT block the pipeline
<!-- spec-review: passed -->
<!-- code-review: passed -->
