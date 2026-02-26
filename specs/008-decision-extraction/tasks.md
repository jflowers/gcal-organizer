# Tasks: Decision Extraction from Meeting Transcripts

**Input**: Design documents from `/specs/008-decision-extraction/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md

**Tests**: Included per Constitution Principle III (Test-Driven Development).

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3, US4)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Add shared data models and types needed by all user stories

- [x] T001 [P] Add `Decision` struct (Category, Text, Timestamp, Context fields) and `TranscriptHeading` struct (HeadingID, Text, Index fields) to `pkg/models/models.go` — per data-model.md entity definitions
- [x] T002 [P] Add `TranscriptContent` struct (TabID, FullText, Headings fields) to `pkg/models/models.go` — aggregates transcript tab metadata
- [x] T003 Add `decisionDocIDs` field (`map[string]string` mapping docID→source) to `Organizer` struct and `GetDecisionDocIDs()` method to `internal/organizer/organizer.go` — uses `map[string]string` (unlike `notesDocIDs`'s `map[string]bool`) because the source value (`"notes-by-gemini"` or `"transcript"`) is needed for per-event deduplication and logging

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Document collection during calendar sync and Gemini decision extraction — MUST complete before any user story

**CRITICAL**: No user story work can begin until this phase is complete

### Tests

- [x] T004 [P] Write `TestDecisionDocCollection` table-driven test in `internal/organizer/organizer_test.go` — verify attachment title matching: exact match "Notes by Gemini", suffix match "- Transcript", non-matching titles rejected, per-event deduplication preferring "Notes by Gemini" (FR-001, FR-002)
- [x] T005 [P] Write `TestParseDecisionsResponse` table-driven test in `internal/gemini/client_test.go` — verify JSON parsing of decision extraction response: valid array, markdown-wrapped response, empty decisions filtered, invalid category defaults to "open", empty text filtered, empty array (data-model.md parsing rules)

### Implementation

- [x] T006 Add decision document collection logic to `SyncCalendarAttachments` in `internal/organizer/organizer.go` — after existing `notesDocIDs` collection (line 288): for each event, check attachment titles for exact "Notes by Gemini" or `strings.HasSuffix(title, "- Transcript")`, apply per-event deduplication, check `--owned-only` via `IsFileOwned`, store in `decisionDocIDs` map (FR-001, FR-002, FR-014, FR-015, R6, R7)
- [x] T007 Implement `ExtractDecisions(ctx, transcriptText string) ([]models.Decision, error)` method on `gemini.Client` in `internal/gemini/client.go` — build prompt requesting JSON array with category/text/timestamp/context fields, call `c.client.Models.GenerateContent` with retry, parse response via new `parseDecisionsResponse` helper (FR-006, FR-007, R4)
- [x] T008 Implement `parseDecisionsResponse(responseText string) ([]models.Decision, error)` helper in `internal/gemini/client.go` — strip markdown code fences, extract JSON array via regex, unmarshal into `[]Decision`, filter empty text, validate/default category (data-model.md parsing rules)
- [x] T009 Run `go test ./internal/organizer/... ./internal/gemini/...` and verify T004, T005 pass. Run `go vet ./...` and `gofmt -l .`

**Checkpoint**: Document collection and Gemini decision extraction work independently. Foundation ready for user story implementation.

---

## Phase 3: User Story 1 — Extract Decisions from Meeting Transcript (Priority: P1) MVP

**Goal**: For each eligible transcript document, read the transcript, extract decisions via Gemini, create a "Decisions" tab with three categorized sections.

**Independent Test**: Run workflow against a calendar event with a "Notes by Gemini" attachment containing a Transcript tab. Verify a "Decisions" tab is created with "Decisions Made", "Decisions Deferred", and "Open Items" sections.

### Tests for User Story 1

- [x] T010 [P] [US1] Write `TestExtractTranscriptContent` table-driven test in `internal/docs/service_test.go` — verify: finds "Transcript" tab in multi-tab doc, uses first tab's content for single-tab doc (via `includeTabsContent=true`), returns empty TranscriptContent for doc with no transcript, extracts full text and H3 heading metadata (FR-003, FR-004)
- [x] T011 [P] [US1] Write `TestCreateDecisionsTab` table-driven test in `internal/docs/service_test.go` — verify: creates tab with correct title "Decisions", inserts three H2 section headings, inserts decision bullet text under correct section, handles empty decisions list with "No decisions identified" note (FR-009, FR-010, FR-011, FR-016)

### Implementation for User Story 1

- [x] T012 [US1] Implement `ExtractTranscriptContent(ctx, docID string) (*models.TranscriptContent, error)` in `internal/docs/service.go` — call `Documents.Get(docID)` with `includeTabsContent=true`, always iterate `doc.Tabs` (single-tab docs have one tab entry): find tab with title "Transcript" or use the sole tab for single-tab docs, extract `tab.DocumentTab.Body.Content` text and H3 headings (ParagraphStyle.NamedStyleType == "HEADING_3") with HeadingID/Text/Index (FR-003, FR-004, R2)
- [x] T013 [US1] Implement `CreateDecisionsTab(ctx, docID string, decisions []models.Decision, transcript *models.TranscriptContent) error` in `internal/docs/service.go` — BatchUpdate #1: `AddDocumentTab` with title "Decisions", extract TabId from response; BatchUpdate #2: `InsertText` for section headings ("Decisions Made\n", "Decisions Deferred\n", "Open Items\n") and decision bullets, `UpdateParagraphStyle` to style headings as HEADING_2, `CreateParagraphBullets` for decision items; handle empty decisions with "No decisions identified" note (FR-009, FR-010, FR-011, FR-016, R1, R3)
- [x] T014 [US1] Add `DocsService` interface to `internal/organizer/organizer.go` with methods: `ExtractTranscriptContent(ctx, docID) (*models.TranscriptContent, error)`, `HasDecisionsTab(ctx, docID) (bool, error)`, `CreateDecisionsTab(ctx, docID, decisions, transcript) error`
- [x] T015 [US1] Implement `ExtractDecisionsForDoc(ctx, docID string, docsSvc DocsService, geminiClient *gemini.Client, dryRun bool) error` orchestration function in `internal/organizer/organizer.go` — coordinates: extract transcript → call Gemini → create tab; skip on AI failure with warning (FR-017); log actions for dry-run (FR-013)
- [x] T016 [US1] Wire Step 4 into `runCmd.RunE` in `cmd/gcal-organizer/main.go` — after Step 3 block (line ~143) and before `PrintSummary`: get `org.GetDecisionDocIDs()`, iterate, call `initDocsAndGemini` + `ExtractDecisionsForDoc` per doc, aggregate stats, handle dry-run logging
- [x] T017 [US1] Add decision stats fields (`DecisionsProcessed`, `DecisionsSkipped`, `DecisionsFailed`) to `Stats` struct and `AddDecisionStats` method in `internal/organizer/organizer.go`; update `PrintSummary` to include decision extraction results
- [x] T018 [US1] Run `go test ./...` and verify all tests pass. Run `go build ./...` and `go vet ./...`

**Checkpoint**: Decision extraction is fully functional. Documents get a "Decisions" tab with categorized decisions (without cross-tab links yet). MVP complete.

---

## Phase 4: User Story 2 — Cross-Tab Timestamp Links (Priority: P2)

**Goal**: Each decision's timestamp text becomes a clickable link navigating to the corresponding H3 heading in the Transcript tab.

**Independent Test**: Click a decision's timestamp link in the Decisions tab and verify it navigates to the correct heading in the Transcript tab.

### Tests for User Story 2

- [x] T019 [P] [US2] Write `TestTimestampToHeadingMatch` table-driven test in `internal/docs/service_test.go` — verify: exact timestamp match returns correct HeadingID, nearest preceding heading found when no exact match, no headings returns nil, multiple headings with same prefix handled correctly (FR-008, data-model.md timestamp matching rules)
- [x] T020 [P] [US2] Write `TestCreateDecisionsTabWithLinks` test in `internal/docs/service_test.go` — verify: `UpdateTextStyle` requests include `Link.Heading` with correct `HeadingLink{Id, TabId}` for each decision with a matched timestamp; decisions without matched timestamps have no link applied (FR-012)

### Implementation for User Story 2

- [x] T021 [US2] Implement `matchTimestampToHeading(timestamp string, headings []models.TranscriptHeading) *models.TranscriptHeading` helper in `internal/docs/service.go` — find heading whose text contains the decision's timestamp; if no exact match, find nearest preceding by document position; return nil if no headings (FR-008, data-model.md timestamp matching)
- [x] T022 [US2] Extend `CreateDecisionsTab` in `internal/docs/service.go` to add cross-tab heading links — in BatchUpdate #2: for each decision with a matched timestamp heading, add `UpdateTextStyleRequest` with `TextStyle.Link.Heading = &HeadingLink{Id: heading.HeadingID, TabId: transcript.TabID}` targeting the timestamp text range within the decision bullet (FR-012, R2)
- [x] T023 [US2] Run `go test ./internal/docs/...` and verify T019, T020 pass alongside existing tests

**Checkpoint**: Decisions now have clickable cross-tab timestamp links. US1 + US2 both work independently.

---

## Phase 5: User Story 3 — Idempotent Processing (Priority: P3)

**Goal**: Documents with an existing "Decisions" tab are skipped. Concurrent tab creation is handled gracefully.

**Independent Test**: Run the workflow twice against the same document and verify the second run skips without errors.

### Tests for User Story 3

- [x] T024 [P] [US3] Write `TestHasDecisionsTab` table-driven test in `internal/docs/service_test.go` — verify: returns true when "Decisions" tab exists, returns false when no "Decisions" tab, returns true for manually-created "Decisions" tab (FR-005)
- [x] T025 [P] [US3] Write `TestOptimisticConcurrency` test in `internal/docs/service_test.go` — verify: when `AddDocumentTab` returns an error indicating duplicate tab, `CreateDecisionsTab` returns a sentinel error that the caller treats as "already processed" (FR-018, R5)

### Implementation for User Story 3

- [x] T026 [US3] Implement `HasDecisionsTab(ctx, docID string) (bool, error)` in `internal/docs/service.go` — call `GetDocument`, iterate `doc.Tabs` checking for `tab.TabProperties.Title == "Decisions"`, return true/false (FR-005, R5)
- [x] T027 [US3] Add idempotency check to `ExtractDecisionsForDoc` in `internal/organizer/organizer.go` — call `docsSvc.HasDecisionsTab` before processing; if true, log "already processed" and increment `DecisionsSkipped` stat; return early (FR-005)
- [x] T028 [US3] Add optimistic concurrency handling to `CreateDecisionsTab` in `internal/docs/service.go` — if `AddDocumentTab` BatchUpdate returns an error, check if error indicates duplicate tab name; if so, return `ErrDecisionsTabExists` sentinel; caller in `ExtractDecisionsForDoc` catches this and treats as "already processed" (FR-018, R5)
- [x] T029 [US3] Run `go test ./...` and verify all tests pass including idempotency scenarios

**Checkpoint**: Idempotent and concurrency-safe. Multiple runs produce zero duplicates (SC-004).

---

## Phase 6: User Story 4 — Dry-Run and Ownership Controls (Priority: P3)

**Goal**: `--dry-run` previews without mutating; `--owned-only` skips unowned documents.

**Independent Test**: Run with `--dry-run` and verify no documents modified. Run with `--owned-only` and verify unowned docs skipped.

### Implementation for User Story 4

- [x] T030 [US4] Add dry-run guard to `ExtractDecisionsForDoc` in `internal/organizer/organizer.go` — when `dryRun` is true: skip Gemini and all BatchUpdate calls; only log eligible document title and transcript size (e.g., "Would extract decisions from [doc title] (N characters)"); consistent with Step 3's dry-run pattern which skips expensive operations (FR-013)
- [x] T031 [US4] Verify `--owned-only` filtering in `SyncCalendarAttachments` decision collection (from T006) — ensure `IsFileOwned` check gates entry into `decisionDocIDs` map when `o.config.OwnedOnly` is true (FR-014); already implemented in T006 but verify with manual test
- [x] T032 [US4] Add dry-run and owned-only output formatting in Step 4 block of `cmd/gcal-organizer/main.go` — match existing Step 3 patterns: "Would extract decisions from N documents" for dry-run, "No owned transcript documents found" when `--owned-only` filters all docs

**Checkpoint**: All four user stories complete. Full feature ready for polish.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, cleanup, and validation across all stories

- [x] T033 [P] Update README.md with Step 4 documentation — describe decision extraction behavior, supported document patterns, example output
- [x] T034 [P] Update AGENTS.md project structure if needed — verify `internal/docs/` description reflects write capability
- [x] T035 [P] Update `man/gcal-organizer.1` man page with Step 4 behavior — add decision extraction description to the workflow section, per constitution documentation requirement
- [x] T036 Run `make ci` — verify no regressions and all quality gates pass (SC-006, constitution Quality Gates)
- [ ] T037 Run `gcal-organizer run --dry-run` against a real calendar with transcript attachments — manual validation of end-to-end flow
- [ ] T038 Verify quickstart.md scenarios work as documented in `specs/008-decision-extraction/quickstart.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion — BLOCKS all user stories
- **User Stories (Phases 3-6)**: All depend on Foundational phase completion
  - US1 (Phase 3): Independent — no story dependencies
  - US2 (Phase 4): Extends US1's `CreateDecisionsTab` with link insertion
  - US3 (Phase 5): Adds guard to US1's `ExtractDecisionsForDoc`
  - US4 (Phase 6): Adds guard to US1's `ExtractDecisionsForDoc`
- **Polish (Phase 7)**: Depends on all user stories being complete

### User Story Dependencies

- **US1 (P1)**: Can start after Foundational (Phase 2) — no dependencies on other stories
- **US2 (P2)**: Extends US1's `CreateDecisionsTab` — requires US1 complete
- **US3 (P3)**: Adds idempotency guard to US1's flow — requires US1 complete; independent of US2
- **US4 (P3)**: Adds dry-run/ownership guards to US1's flow — requires US1 complete; independent of US2, US3

### Within Each User Story

- Tests MUST be written and FAIL before implementation
- Models/types before service methods
- Service methods before orchestration
- Orchestration before CLI wiring
- Run tests after each implementation task

### Parallel Opportunities

- T001, T002 can run in parallel (different structs in same file, no conflicts)
- T004, T005 can run in parallel (different test files)
- T010, T011 can run in parallel (different test functions)
- T019, T020 can run in parallel (different test functions)
- T024, T025 can run in parallel (different test functions)
- T033, T034, T035 can run in parallel (different documentation files)
- US3 and US4 can run in parallel after US1 completes (independent guards on the same function)

---

## Parallel Example: User Story 1

```bash
# Launch tests for US1 together:
Task: "T010 — TestExtractTranscriptContent in internal/docs/service_test.go"
Task: "T011 — TestCreateDecisionsTab in internal/docs/service_test.go"

# Then implement service methods in parallel:
Task: "T012 — ExtractTranscriptContent in internal/docs/service.go"
Task: "T013 — CreateDecisionsTab in internal/docs/service.go"

# Then orchestration (depends on T012, T013):
Task: "T014 — DocsService interface in internal/organizer/organizer.go"
Task: "T015 — ExtractDecisionsForDoc in internal/organizer/organizer.go"
Task: "T016 — Wire Step 4 into runCmd in cmd/gcal-organizer/main.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001-T003)
2. Complete Phase 2: Foundational (T004-T009) — CRITICAL
3. Complete Phase 3: User Story 1 (T010-T018)
4. **STOP and VALIDATE**: Decisions tab created with categorized bullets — no links yet
5. Deploy/demo if ready — core value delivered

### Incremental Delivery

1. Setup + Foundational → Foundation ready
2. Add US1 → Decisions tab with categories → Deploy (MVP!)
3. Add US2 → Cross-tab timestamp links → Deploy (full experience)
4. Add US3 → Idempotent processing → Deploy (production-safe)
5. Add US4 → Dry-run and ownership controls → Deploy (complete)
6. Polish → Documentation and validation → Final release

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together
2. Once Foundational is done:
   - Developer A: User Story 1
3. Once US1 is done:
   - Developer A: User Story 2
   - Developer B: User Story 3 (parallel — independent guard)
   - Developer C: User Story 4 (parallel — independent guard)
4. Stories complete and integrate independently

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Verify tests fail before implementing
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Constitution Principle III mandates TDD — tests included in all phases
