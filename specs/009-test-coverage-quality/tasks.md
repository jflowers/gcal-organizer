# Tasks: Comprehensive Test Coverage & Contract Quality

**Input**: Design documents from `/specs/009-test-coverage-quality/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, quickstart.md

**Tests**: This feature IS test development — every task produces tests. All tests use contract-style assertions (FR-011).

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Verify baseline and ensure 008-decision-extraction changes are merged

- [X] T001 Verify 008-decision-extraction is merged to main and baseline tests pass: `go test ./... -count=1`
- [X] T002 Record baseline coverage numbers per package: `go test ./... -coverprofile=baseline.out && go tool cover -func=baseline.out`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Create `.gaze.yaml` configuration file (FR-017) which unlocks contract coverage measurement across the entire module. Without this, all side effects are classified as "ambiguous" (confidence 64-79, below the default threshold of 80), yielding 0% contract coverage regardless of assertion quality.

**CRITICAL**: This task MUST be completed before any contract coverage verification (SC-015) can succeed.

- [X] T073 Create `.gaze.yaml` at repository root with `contractual-threshold: 65` and `incidental-threshold: 40` per FR-017 and research.md Section 1. Verify it takes effect by running `gaze quality ./internal/retry/ --format=text` and confirming non-zero contract coverage for `DefaultConfig` in `.gaze.yaml`

**Checkpoint**: Foundation ready — contract coverage measurement now functional. Verify: `gaze quality ./internal/retry/ --format=text` shows contract coverage > 0%.

---

## Phase 3: User Story 1 — Orchestration Logic (Priority: P1) MVP

**Goal**: Bring organizer package from 58% to >85% coverage by adding contract-assertion tests for `printSummary`, `logActionResult`, `logCalendarAction`, `RunFullWorkflow`, and strengthening existing `OrganizeDocuments`/`SyncCalendarAttachments` tests. Eliminates both Q4 functions.

**Independent Test**: `go test ./internal/organizer/ -v -cover` — all tests pass with >85% coverage, every test has at least one contract assertion.

### Implementation for User Story 1

- [X] T003 [US1] Add `TestLogActionResult` table-driven test covering all 6 ActionResult branches (success move, success shortcut, dry-run move, dry-run shortcut, "already exists"/"already in folder" skip, error reason) with stat counter assertions (`DocumentsMoved`, `ShortcutsCreated`, `Errors`) in `internal/organizer/organizer_test.go`
- [X] T004 [US1] Add `TestLogCalendarAction` table-driven test covering all 4 branches (success, dry-run, "already exists" skip, error) with stat counter assertions (`ShortcutsCreated`, `Errors`) in `internal/organizer/organizer_test.go`
- [X] T005 [US1] Add `TestPrintSummary_DryRun` test with mock DriveService (IsDryRun=true) setting various stat combinations (decisions present, skipped files) verifying no panics in `internal/organizer/organizer_test.go`
- [X] T006 [US1] Add `TestPrintSummary_RealRun` test with mock DriveService (IsDryRun=false) exercising all conditional branches (DecisionsProcessed>0, ShortcutsTrashed>0, AttachmentsShared>0, TasksFailed>0, Skipped>0, Errors>0) in `internal/organizer/organizer_test.go`
- [X] T007 [US1] Add `TestRunFullWorkflow_HappyPath` and `TestRunFullWorkflow_OrganizeError` and `TestRunFullWorkflow_SyncError` tests verifying sequential execution order and error propagation in `internal/organizer/organizer_test.go`
- [X] T008 [US1] Strengthen existing `TestOrganizeDocuments_*` tests with contract assertions on `DocumentsFound`, `SetMasterFolder` call args, `GetOrCreateMeetingFolder` call args, `MoveDocument`/`CreateShortcut` argument verification in `internal/organizer/organizer_test.go`
- [X] T009 [US1] Strengthen existing `TestSyncCalendarAttachments_*` tests with contract assertions on `EventsProcessed`, `EventsWithAttach`, `AttachmentsShared`, `ShareFile` argument verification, `CanEditFile` call counts in `internal/organizer/organizer_test.go`
- [X] T010 [US1] Add `TestOrganizeDocuments_ListMeetingDocsError` and `TestOrganizeDocuments_EmptyDocumentList` tests with error propagation and stats contract assertions in `internal/organizer/organizer_test.go`
- [X] T011 [US1] Add `TestSyncCalendarAttachments_ListEventsError`, `TestSyncCalendarAttachments_EmptyEventsList`, `TestSyncCalendarAttachments_SkipsCalendarResources`, `TestSyncCalendarAttachments_OwnershipCacheAvoidsRedundantCalls` tests in `internal/organizer/organizer_test.go`
- [X] T012 [US1] Add trivial tests for `New`, `AddTaskStats`, `AddDecisionStats`, `PrintSummary` (exported wrapper) verifying they don't panic and set expected state in `internal/organizer/organizer_test.go`
- [X] T013 [US1] Verify organizer package coverage >85%: `go test ./internal/organizer/ -cover -count=1`

**Checkpoint**: Organizer package at >85% coverage, Q4 functions eliminated. Run: `go test ./internal/organizer/ -v -cover`

---

## Phase 3.1: Contract Assertion Strengthening for Q4 Functions (FR-011)

**Purpose**: Add direct return-value and receiver-field assertions to existing organizer tests so gaze can map them to classified side effects. This is required by FR-011 (amended): functions with GazeCRAP > 15 or in Q4 MUST have assertions on error returns and receiver mutations, not only on mock call recordings. See data-model.md Contract Assertion Map for the specific side effects to assert.

- [X] T074 [US1] Add direct error-return assertions (`assert.NoError(t, err)` or `assert.Error(t, err)`) to every `TestOrganizeDocuments_*` test that calls `OrganizeDocuments` but currently only asserts on mock call recordings, in `internal/organizer/organizer_test.go`
- [X] T075 [US1] Add direct error-return and receiver-mutation assertions to every `TestSyncCalendarAttachments_*` test: assert `err` return value, assert `org.stats.ShortcutsCreated`/`org.stats.AttachmentsShared`/`org.stats.Errors` counts, and assert `org.GetNotesDocIDs()` and `org.GetDecisionDocIDs()` map contents where applicable, in `internal/organizer/organizer_test.go`
- [X] T076 [US1] Add direct error-return assertions to `TestRunFullWorkflow_*` tests: verify `err` is nil on happy path and non-nil with expected message on error paths, in `internal/organizer/organizer_test.go`

**Checkpoint**: Q4 functions now have gaze-mappable contract assertions. Verify: `gaze quality --format=text ./internal/organizer/` shows contract coverage > 0% for OrganizeDocuments, SyncCalendarAttachments, RunFullWorkflow.

---

## Phase 3.2: Test Helper Renaming (FR-018)

**Purpose**: Rename test helper constructors to reduce gaze multi-target detection warnings toward SC-016. Per research.md Section 2, these warnings are informational noise (gaze still correctly processes tests), but renaming reduces the warning count. All tasks can run in parallel — different files.

- [X] T077 [P] [US1] Rename `newTestOrganizer` to `setupOrganizer` in `internal/organizer/organizer_test.go` — update function definition (line 165) and all call sites; verify tests pass: `go test ./internal/organizer/ -count=1`
- [X] T078 [P] [US5] Rename `mockService` to `setupDriveService` in `internal/drive/service_test.go` — update function definition (line 11) and all call sites; verify tests pass: `go test ./internal/drive/ -count=1`
- [X] T079 [P] [US2] Rename `buildTestDoc` to `makeTestDoc`, `buildTab` to `makeTab`, `buildParagraphElement` to `makeParagraphElement` in `internal/docs/service_test.go` — update definitions (lines 250, 258, 273) and all call sites; verify tests pass: `go test ./internal/docs/ -count=1`

**Checkpoint**: Test helper renaming complete. Verify all tests pass: `go test ./... -count=1`

---

## Phase 4: User Story 2 — Document Service Logic (Priority: P1)

**Goal**: Bring docs package from 41.5% to >80% coverage by adding httptest-based tests for `CreateDecisionsTab` (all 4 error paths) and `ExtractCheckboxItems`. Eliminates CRAP 650 function.

**Independent Test**: `go test ./internal/docs/ -v -cover` — all tests pass with >80% coverage.

### Implementation for User Story 2

- [ ] T014 [US2] Add `newTestService(t, handler)` httptest helper function in `internal/docs/service_test.go` using `option.WithHTTPClient`, `option.WithEndpoint`, `option.WithoutAuthentication` per research.md R1 pattern
- [ ] T015 [US2] Add `TestCreateDecisionsTab_HappyPath` test verifying nil return and exactly 2 BatchUpdate calls in `internal/docs/service_test.go`
- [ ] T016 [US2] Add `TestCreateDecisionsTab_DuplicateTab_GoogleAPIError` test verifying 409 returns `ErrDecisionsTabExists` sentinel in `internal/docs/service_test.go`
- [ ] T017 [US2] Add `TestCreateDecisionsTab_DuplicateTab_StringMatch` tests verifying "already exists" and "duplicate" string matching returns sentinel in `internal/docs/service_test.go`
- [ ] T018 [US2] Add `TestCreateDecisionsTab_ContentInsertionFails_CleanupSucceeds` test verifying error mentions "empty tab cleaned up" and exactly 3 API calls in `internal/docs/service_test.go`
- [ ] T019 [US2] Add `TestCreateDecisionsTab_ContentInsertionFails_CleanupFails` test verifying error mentions both "failed to insert content" and "cleanup of empty tab also failed" in `internal/docs/service_test.go`
- [ ] T020 [US2] Add `TestCreateDecisionsTab_NoTabIDReturned` and `TestCreateDecisionsTab_EmptyDecisions` edge case tests in `internal/docs/service_test.go`
- [ ] T021 [US2] Add `TestCreateDecisionsTab_RequestBodyValidation` test verifying first call is AddDocumentTab and second contains InsertText in `internal/docs/service_test.go`
- [ ] T022 [US2] Add `TestExtractCheckboxItems_NotesTab` httptest test returning a mock Document with Notes tab, verifying extraction from "Suggested next steps" section in `internal/docs/service_test.go`
- [ ] T023 [US2] Add `TestExtractCheckboxItems_SectionBoundary` test verifying extraction stops at next heading after "Suggested next steps" in `internal/docs/service_test.go`
- [ ] T024 [US2] Add `TestExtractCheckboxItems_ProcessedFilter` test verifying items with ProcessedEmoji are marked IsProcessed=true in `internal/docs/service_test.go`
- [ ] T025 [US2] Add `TestExtractCheckboxItems_NoNotesTab` test verifying behavior when document has no Notes tab in `internal/docs/service_test.go`
- [ ] T026 [US2] Verify docs package coverage >80%: `go test ./internal/docs/ -cover -count=1`

**Checkpoint**: Docs package at >80% coverage, CRAP 650 eliminated. Run: `go test ./internal/docs/ -v -cover`

---

## Phase 5: User Story 3 — AI Integration (Priority: P1)

**Goal**: Bring gemini package from 49% to >85% coverage by adding httptest-based tests for `ExtractDecisions` and `ExtractAssigneesFromCheckboxes`.

**Independent Test**: `go test ./internal/gemini/ -v -cover` — all tests pass with >85% coverage.

### Implementation for User Story 3

- [ ] T027 [US3] Add `newTestClient(t, handler)` httptest helper function in `internal/gemini/client_test.go` using `genai.ClientConfig{HTTPClient, HTTPOptions.BaseURL}` per research.md R2 pattern
- [ ] T028 [US3] Add `TestExtractDecisions_ValidResponse` test with mock returning well-formed JSON decisions, verifying correct categorization and field mapping in `internal/gemini/client_test.go`
- [ ] T029 [US3] Add `TestExtractDecisions_EmptyTranscript` test verifying nil return without API call for empty input in `internal/gemini/client_test.go`
- [ ] T030 [US3] Add `TestExtractDecisions_NoCandidates` test with mock returning empty candidates array, verifying descriptive error in `internal/gemini/client_test.go`
- [ ] T031 [US3] Add `TestExtractDecisions_APIError` test with mock returning HTTP error, verifying error propagation in `internal/gemini/client_test.go`
- [ ] T032 [US3] Add `TestExtractAssigneesFromCheckboxes_ValidResponse` test with mock returning assignments, verifying index/text/assignee/email mapping in `internal/gemini/client_test.go`
- [ ] T033 [US3] Add `TestExtractAssigneesFromCheckboxes_NullAssignees` test verifying null-assignee entries are filtered from results in `internal/gemini/client_test.go`
- [ ] T034 [US3] Add `TestExtractAssigneesFromCheckboxes_EmptyItems` test verifying behavior with empty input slice in `internal/gemini/client_test.go`
- [ ] T035 [US3] Add `TestExtractAssigneesFromCheckboxes_APIError` test verifying error propagation from mock server failure in `internal/gemini/client_test.go`
- [ ] T036 [US3] Verify gemini package coverage >85%: `go test ./internal/gemini/ -cover -count=1`

**Checkpoint**: Gemini package at >85% coverage. Run: `go test ./internal/gemini/ -v -cover`

---

## Phase 6: User Story 4 — Calendar Event Parsing (Priority: P2)

**Goal**: Bring calendar package from 9.2% to >80% coverage by testing `parseEvent` with all date/time formats and `ListRecentEvents` via httptest.

**Independent Test**: `go test ./internal/calendar/ -v -cover` — all tests pass with >80% coverage.

### Implementation for User Story 4

- [X] T037 [US4] Add `TestParseEvent_DateTimeFormat` test with RFC3339 Start.DateTime and End.DateTime verifying correct time parsing in `internal/calendar/service_test.go`
- [X] T038 [US4] Add `TestParseEvent_DateOnlyFormat` test with all-day event Start.Date and End.Date verifying correct date parsing in `internal/calendar/service_test.go`
- [X] T039 [US4] Add `TestParseEvent_InvalidDateTime` test with malformed date strings verifying error return in `internal/calendar/service_test.go`
- [X] T040 [US4] Add `TestParseEvent_Attachments` test with explicit event attachments (FileID, Title, MimeType, FileURL) verifying all fields mapped in `internal/calendar/service_test.go`
- [X] T041 [US4] Add `TestParseEvent_DescriptionDriveLinks` test with Drive URLs in event description verifying extracted attachments combined with explicit attachments in `internal/calendar/service_test.go`
- [X] T042 [US4] Add `TestParseEvent_Attendees` test verifying Email, DisplayName, IsSelf, IsOrganizer mapping for multiple attendee types in `internal/calendar/service_test.go`
- [X] T043 [US4] Add `TestParseEvent_EmptyEvent` test with minimal event (no attachments, no attendees, no description) verifying no panics in `internal/calendar/service_test.go`
- [X] T044 [US4] Add `TestParseEvent_MixedDateFormats` test with DateTime start and Date-only end verifying both parse correctly in `internal/calendar/service_test.go`
- [ ] T045 [US4] Add `newTestCalendarService(t, handler)` httptest helper and `TestListRecentEvents_SinglePage` test verifying event parsing from mock Calendar API response in `internal/calendar/service_test.go`
- [ ] T046 [US4] Add `TestListRecentEvents_Pagination` test with multi-page response (nextPageToken) verifying all events collected in `internal/calendar/service_test.go`
- [ ] T047 [US4] Add `TestListRecentEvents_Error` test with mock returning error verifying error propagation in `internal/calendar/service_test.go`
- [ ] T048 [US4] Verify calendar package coverage >80%: `go test ./internal/calendar/ -cover -count=1`

**Checkpoint**: Calendar package at >80% coverage. Run: `go test ./internal/calendar/ -v -cover`

---

## Phase 7: User Story 5 — Config, Secrets, UX, Drive (Priority: P2)

**Goal**: Close remaining coverage gaps in config (>90%), secrets (>85%), ux (100%), and drive (>15%) packages.

**Independent Test**: `go test ./internal/config/ ./internal/secrets/ ./internal/ux/ ./internal/drive/ -v -cover` — all targets met.

### Implementation for User Story 5

- [X] T049 [P] [US5] Add `TestLoadSecrets` test with mock SecretStore verifying config fields populated from store override env defaults in `internal/config/config_test.go`
- [X] T050 [P] [US5] Add `TestMustBindEnv_Valid` and `TestMustBindEnv_Edge` tests closing the 50% gap in `internal/config/config_test.go`
- [X] T051 [P] [US5] Add `TestBackendString` table-driven test for `BackendKeychain` and `BackendFile` string representations in `internal/secrets/store_test.go`
- [X] T052 [P] [US5] Add `TestNewStore_KeychainSuccess` test verifying successful keychain backend selection in `internal/secrets/store_test.go`
- [X] T053 [P] [US5] Add `TestWriteEnvValue_NewFile` and `TestWriteLines_Atomic` tests closing remaining secrets gaps in `internal/secrets/store_test.go`
- [X] T054 [P] [US5] Create `internal/ux/ux_test.go` with table-driven `TestErrorConstructors` covering all 8 constructors (NewError, MissingCredentials, MissingAPIKey, TokenExpired, MissingToken, OAuthSetupFailed, AuthFailed, ActionError.Error) verifying Message and Fix fields
- [X] T055 [P] [US5] Add `TestEscapeQuery` table-driven test for single-quote escaping (empty, no quotes, single quote, multiple quotes, quote at edges) in `internal/drive/service_test.go`
- [X] T056 [P] [US5] Add `TestParseDocument_NilOwners` and `TestParseDocument_EmptyParents` edge case tests in `internal/drive/service_test.go`
- [ ] T057 [US5] Verify per-package coverage targets: config >90%, secrets >85%, ux 100%, drive >15%: `go test ./internal/config/ ./internal/secrets/ ./internal/ux/ ./internal/drive/ -cover`

**Checkpoint**: All P2 utility packages meet coverage targets. Run: `go test ./internal/config/ ./internal/secrets/ ./internal/ux/ ./internal/drive/ -v -cover`

---

## Phase 8: User Story 6 — Extract and Test CLI Pure Functions (Priority: P3)

**Goal**: Extract `loadDotEnv`, `maskSecret`, `truncateText` from `cmd/` to `internal/` packages and test them. Test `extractUnassignedItems` in place.

**Independent Test**: `go build ./... && go test ./internal/config/ ./internal/ux/ ./cmd/gcal-organizer/ -v -cover` — extraction compiles, all tests pass.

### Implementation for User Story 6

- [X] T058 [US6] Extract `loadDotEnv` function and `validEnvKey` regexp from `cmd/gcal-organizer/main.go:382` to `internal/config/dotenv.go` as exported `LoadDotEnv(path, home string)` and update caller in `initConfig()` to `config.LoadDotEnv(envFile, home)`
- [X] T059 [US6] Extract `maskSecret` from `cmd/gcal-organizer/auth_config.go:171` and `truncateText` from `cmd/gcal-organizer/main.go:297` to `internal/ux/format.go` as exported `MaskSecret(s string) string` and `TruncateText(s string, maxLen int) string`, update callers
- [X] T060 [US6] Verify extraction compiles cleanly: `go build ./...`
- [X] T061 [US6] Add `TestLoadDotEnv` table-driven test with 10+ cases (comments, blank lines, KEY=VALUE, double-quoted, single-quoted, POSIX escape, tilde expansion, tilde-only, env precedence, invalid keys, missing file) in `internal/config/dotenv_test.go`
- [X] T062 [P] [US6] Add `TestMaskSecret` table-driven test (empty, short 1-3 chars, medium 4-8 chars, long >8 chars) verifying correct reveal/mask ratio in `internal/ux/format_test.go`
- [X] T063 [P] [US6] Add `TestTruncateText` table-driven test (shorter than max, exactly max, longer than max, empty, Unicode multi-byte, max=0) verifying truncation with ellipsis in `internal/ux/format_test.go`
- [X] T064 [US6] Add `TestExtractUnassignedItems` table-driven test (all processed, none processed, mixed, empty input) verifying correct indices and filtering in `cmd/gcal-organizer/assign_tasks_test.go`
- [X] T065 [US6] Verify all extractions and tests pass: `go build ./... && go test ./...`

**Checkpoint**: All pure functions extracted and tested. Run: `go build ./... && go test ./... -v`

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: Final verification, quality gates, and documentation updates

- [X] T066 Run full test suite: `go test ./... -count=1`
- [X] T067 Run race detector: `go test -race ./...`
- [X] T068 Generate coverage report and verify >55% overall: `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out`
- [X] T069 Run quality gates: `go vet ./... && gofmt -l . && go mod tidy`
- [X] T070 Run `make ci` and verify all checks pass
- [ ] T071 Update AGENTS.md if project structure changed (new files in internal/config/, internal/ux/)
- [ ] T072 Run gaze full report and verify CRAPload <15, zero Q4, at most 2 Q3 (SC-002, SC-003, SC-004)
- [ ] T080 Verify SC-015: run `gaze quality --format=json ./...` and confirm average contract coverage across analyzed functions exceeds 50%
- [ ] T081 Verify SC-016: run `gaze quality ./... 2>&1` and count multi-target skip warnings; confirm fewer than 20 test-helper-sourced warnings (note: warnings from unexported production functions like `logActionResult`, `isCalendarResource` are expected noise per research.md Section 2)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Setup. Creates `.gaze.yaml` (T073) — BLOCKS contract coverage verification in all later phases (SC-015, T080)
- **User Stories (Phases 3-8)**: All depend on Setup completion. User story tests can proceed without `.gaze.yaml`, but contract coverage verification requires Phase 2.
  - US1, US2, US3 are all P1 and can proceed in parallel (different files)
  - US4, US5 are P2 and can proceed in parallel with each other and with P1 stories
  - US6 is P3 and is the only story requiring production code changes
- **Phase 3.1 (Contract Assertions)**: Depends on US1 completion (Phase 3) — strengthens existing tests with gaze-mappable assertions
- **Phase 3.2 (Helper Renaming)**: Can run at any time — operates on test files independently. Can be parallelized with US2-US6. Should complete before SC-016 verification (T081).
- **Polish (Phase 9)**: Depends on all user stories, Phase 2, Phase 3.1, and Phase 3.2 being complete

### User Story Dependencies

- **US1 (Organizer)**: No dependencies on other stories — operates on `internal/organizer/organizer_test.go` only
- **US2 (Docs)**: No dependencies on other stories — operates on `internal/docs/service_test.go` only
- **US3 (Gemini)**: No dependencies on other stories — operates on `internal/gemini/client_test.go` only
- **US4 (Calendar)**: No dependencies on other stories — operates on `internal/calendar/service_test.go` only
- **US5 (Config/Secrets/UX/Drive)**: No dependencies — operates on 4 independent test files
- **US6 (Extract cmd/)**: No dependencies on US1-US5 — operates on `cmd/` and new files in `internal/config/` and `internal/ux/`

### New Task Dependencies (from clarification session)

- **T073 (.gaze.yaml)**: No dependencies — can be done immediately. BLOCKS T080 (SC-015 verification).
- **T074-T076 (Contract assertions)**: Depend on US1 completion (T003-T013 all `[X]`). Operate on same file as US1.
- **T077-T079 (Helper renaming)**: No dependencies on each other — all [P]. Each operates on a different test file. T077 should not run concurrently with T074-T076 (same file). T079 should not run concurrently with T014-T026 (same file).
- **T080-T081 (SC-015/SC-016)**: Depend on T073, T074-T076, T077-T079 being complete.

### Within Each User Story

- httptest helper MUST be created before tests that use it (within same file)
- All tests within a story operate on a single file — sequential within story
- Coverage verification is the last task in each story

### Parallel Opportunities

- **US1, US2, US3 can all run in parallel** — different test files, no shared state
- **US4, US5 can run in parallel with each other and with P1 stories** — different packages
- **T073 (.gaze.yaml) can run in parallel with any user story** — different file
- **T077, T078, T079 (helper renaming) can run in parallel with each other** — different files
- **Within US5**: All tasks marked [P] can run in parallel (different files)
- **Within US6**: T062, T063 can run in parallel (different files)
- **US6 is independent** of all other stories — can run at any time after Setup

---

## Parallel Example: Foundational + User Stories 1, 2, 3

```text
# T073 (.gaze.yaml) can run in parallel with all user stories:

Agent A: .gaze.yaml                             → T073 (foundational, quick)
Agent B (US1): internal/organizer/organizer_test.go
  → T003-T013: logActionResult, logCalendarAction, printSummary, RunFullWorkflow [all done]
  → T074-T076: contract assertion strengthening for Q4 functions
  → T077: rename newTestOrganizer → setupOrganizer

Agent C (US2): internal/docs/service_test.go
  → T014-T026: CreateDecisionsTab httptest, ExtractCheckboxItems httptest
  → T079: rename buildTestDoc/buildTab/buildParagraphElement

Agent D (US3): internal/gemini/client_test.go
  → T027-T036: ExtractDecisions httptest, ExtractAssigneesFromCheckboxes httptest
```

## Parallel Example: User Story 5 + Helper Renaming (All P2, all [P])

```text
# All US5 tasks and T078 (drive helper rename) operate on different files:

Agent A: internal/config/config_test.go         → T049-T050
Agent B: internal/secrets/store_test.go         → T051-T053
Agent C: internal/ux/ux_test.go                 → T054
Agent D: internal/drive/service_test.go         → T055-T056, T078 (rename mockService)
```

---

## Implementation Strategy

### MVP First (User Story 1 + Gaze Config)

1. Complete Phase 1: Setup (T001-T002) [done]
2. Complete Phase 2: Foundational — `.gaze.yaml` (T073)
3. Complete Phase 3: User Story 1 — Organizer (T003-T013) [done]
4. Complete Phase 3.1: Contract assertion strengthening (T074-T076)
5. **STOP and VALIDATE**: `go test ./internal/organizer/ -v -cover` — >85% coverage; `gaze quality ./internal/organizer/` — contract coverage > 0%
6. This alone eliminates both Q4 functions and unlocks contract coverage measurement (highest impact)

### Incremental Delivery

1. Setup → ready [done]
2. Foundational → `.gaze.yaml` created, contract coverage unlocked
3. US1 (Organizer) → Q4 eliminated, organizer >85% → validate [done]
4. Phase 3.1 → contract assertions added, gaze can map them → validate
5. Phase 3.2 → helper renaming, warning count reduced → validate
6. US2 (Docs) → CRAP 650 eliminated, docs >80% → validate
7. US3 (Gemini) → gemini >85% → validate
8. US4 (Calendar) → calendar >80% → validate
9. US5 (Config/Secrets/UX/Drive) → utility packages meet targets → validate
10. US6 (Extract cmd/) → pure functions testable → validate [done]
11. Polish → overall >55%, CRAPload <15, contract coverage >50%, `make ci` passes

### Parallel Team Strategy

With multiple agents:

1. All agents: Complete Setup (T001-T002) [done]
2. Any agent: Complete Foundational — `.gaze.yaml` (T073) — quick, 1 file
3. Once Setup done:
   - Agent A: US1 contract assertions (T074-T076) then US1 helper rename (T077)
   - Agent B: US2 (Docs, T014-T026) then US2 helper rename (T079)
   - Agent C: US3 (Gemini, T027-T036)
4. After P1 stories complete:
   - Agent A: US4 (Calendar)
   - Agent B: US5 (Config/Secrets/UX/Drive) + US5 helper rename (T078)
   - Agent C: US6 (Extract cmd/) [done]
5. All agents: Polish phase (T066-T072, T080-T081)

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Every test MUST include at least one contract assertion (FR-011). For Q4/GazeCRAP>15 functions, assertions MUST be on return values or receiver fields, not only mock call recordings.
- `.gaze.yaml` MUST exist at repo root before contract coverage verification (FR-017, T073)
- Test helper renaming follows `setup*` or `make*` convention (FR-018, T077-T079)
- No new external dependencies — all mocking uses `net/http/httptest` (FR-013)
- All tests must be race-detector safe (FR-012)
- Commit after each completed user story phase
- Stop at any checkpoint to validate story independently
- SC-016 note: Most multi-target warnings come from unexported production functions (not test helpers). Helper renaming addresses the controllable subset. See research.md Section 2.
