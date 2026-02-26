# Tasks: Owned-Only Mode for File Mutation Protection

**Input**: Design documents from `/specs/006-owned-only-flag/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, quickstart.md

**Tests**: Included per Constitution Principle III (Test-Driven Development is mandatory).

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Phase 1: Setup (Config & Flag Registration)

**Purpose**: Add the `OwnedOnly` field to the config struct and register the `--owned-only` CLI flag, following the established `--dry-run` pattern.

- [x] T001 Add `OwnedOnly bool` field to Config struct in `internal/config/config.go` (after `DryRun` field, line ~43), and add `cfg.OwnedOnly = viper.GetBool("owned-only")` in `Load()` following the `DryRun` pattern at line ~111
- [x] T002 Add tests for `OwnedOnly` in `internal/config/config_test.go`: assert default is `false` in `TestDefaultConfig`, add env var override test setting `GCAL_OWNED_ONLY=true` and verifying `cfg.OwnedOnly == true`
- [x] T003 Register `--owned-only` persistent flag in `cmd/gcal-organizer/main.go`: add package-level `var ownedOnly bool` alongside `dryRun` (line ~38), add `rootCmd.PersistentFlags().BoolVar(&ownedOnly, "owned-only", false, "only mutate files you own; skip non-owned files")` and `viper.BindPFlag("owned-only", ...)` in `init()`, and add `cfg.OwnedOnly = ownedOnly` in each command's `RunE` that loads config (following `cfg.DryRun = dryRun` pattern)
- [x] T004 Run `go test ./internal/config/...` and `go vet ./...` to verify setup phase passes

**Checkpoint**: `--owned-only` flag is parseable and flows into `Config.OwnedOnly`. No behavioral changes yet.

---

## Phase 2: Foundational (Drive Service Enhancement)

**Purpose**: Add the `IsFileOwned()` helper method and fix `GetFileInfo()` to populate `IsOwned`. These are prerequisites for all user stories that need runtime ownership checks.

**CRITICAL**: No user story work can begin until this phase is complete.

- [x] T005 Add `IsFileOwned(ctx context.Context, fileID string) (bool, error)` method to `internal/drive/service.go`: call `Files.Get(fileID).Fields("owners").Do()`, iterate `file.Owners[].EmailAddress` comparing against `s.currentUserEmail`, return `(true, nil)` if match found, `(false, nil)` if no match, `(false, err)` on API failure
- [x] T006 Fix `GetFileInfo()` in `internal/drive/service.go` to populate `IsOwned` on the returned `Document` by comparing `file.Owners[].EmailAddress` against `s.currentUserEmail` (same logic as `parseDocument()` lines 235-242)
- [x] T007 Add `IsOwned` test coverage to `internal/drive/service_test.go`: add `Owners` field to existing test `drive.File` structs in `parseDocument` tests, add test cases for `IsOwned == true` when owner matches `currentUserEmail`, `IsOwned == false` when owner differs, and `IsOwned == false` when `Owners` is empty (fail-safe per FR-010)
- [x] T008 Run `go test ./internal/drive/...` and `go vet ./...` to verify foundational phase passes

**Checkpoint**: `IsFileOwned()` and `GetFileInfo()` ownership population are available for all downstream user stories.

---

## Phase 3: User Story 1 - Protect Non-Owned Files During Organization (Priority: P1) MVP

**Goal**: When `--owned-only` is active, skip move and trash operations on non-owned documents in `OrganizeDocuments()`, while still creating shortcuts for discoverability.

**Independent Test**: Run `gcal-organizer organize --owned-only` against a mix of owned and non-owned documents. Verify owned files are moved normally and non-owned files only get shortcuts.

**Maps to**: FR-002, FR-003, FR-005, FR-013

### Tests for User Story 1

- [x] T009 [US1] Create `internal/organizer/organizer_test.go` with table-driven tests for `OrganizeDocuments()` ownership filtering: test OwnedOnly=false processes all documents unchanged (no filtering, regression check), test OwnedOnly=true with non-owned doc creates shortcut but does NOT call MoveDocument, test OwnedOnly=true with owned doc calls MoveDocument normally, test Stats.Skipped count increments correctly for non-owned docs when OwnedOnly=true

### Implementation for User Story 1

- [x] T010 [US1] Add `Skipped int` field to `Stats` struct in `internal/organizer/organizer.go`
- [x] T011 [US1] Add ownership filtering in `OrganizeDocuments()` loop in `internal/organizer/organizer.go` (line ~149): when `o.config.OwnedOnly && !doc.IsOwned`, increment `o.stats.Skipped`, call `o.drive.CreateShortcut()` for discoverability, then `continue` to skip move/trash operations
- [x] T012 [US1] Run `go test ./internal/organizer/...` to verify US1 tests pass

**Checkpoint**: `organize --owned-only` protects non-owned files. Independently testable.

---

## Phase 4: User Story 2 - Skip Sharing Non-Owned Calendar Attachments (Priority: P1)

**Goal**: When `--owned-only` is active, skip `ShareFile()` for attachments the user doesn't own during `SyncCalendarAttachments()`, and exclude non-owned docs from the `notesDocIDs` collection for Step 3.

**Independent Test**: Run `gcal-organizer sync-calendar --owned-only` with calendar events containing non-owned attachments. Verify shortcuts are created but no permission changes are made.

**Maps to**: FR-004, FR-005, FR-007

### Tests for User Story 2

- [x] T013 [US2] Add table-driven tests for `SyncCalendarAttachments()` ownership filtering in `internal/organizer/organizer_test.go`: test OwnedOnly=true skips ShareFile for non-owned attachments (calls IsFileOwned, does NOT call ShareFile), test OwnedOnly=true still shares owned attachments normally, test OwnedOnly=true excludes non-owned docs from notesDocIDs collection, test OwnedOnly=false preserves existing CanEditFile-gated sharing behavior

### Implementation for User Story 2

- [x] T014 [US2] Add ownership check before `ShareFile()` in `SyncCalendarAttachments()` in `internal/organizer/organizer.go` (line ~244 sharing block): when `o.config.OwnedOnly`, call `o.drive.IsFileOwned(ctx, att.FileID)` and if not owned or error, increment `o.stats.Skipped` and `continue` to skip sharing
- [x] T015 [US2] Add ownership filter to `notesDocIDs` collection in `SyncCalendarAttachments()` in `internal/organizer/organizer.go` (line ~231): when `o.config.OwnedOnly`, call `o.drive.IsFileOwned(ctx, att.FileID)` and exclude non-owned docs from the map
- [x] T016 [US2] Run `go test ./internal/organizer/...` to verify US2 tests pass

**Checkpoint**: `sync-calendar --owned-only` skips sharing and Step 3 collection for non-owned files. Independently testable.

---

## Phase 5: User Story 3 - Block Task Assignment on Non-Owned Documents (Priority: P2)

**Goal**: When `--owned-only` is active, the standalone `assign-tasks --doc` command errors on non-owned docs. The `run` workflow skips non-owned docs in Step 3 (already filtered in US2 via notesDocIDs).

**Independent Test**: Run `assign-tasks --doc <non-owned-id> --owned-only` and verify error exit. Run `gcal-organizer run --owned-only` and verify Step 3 skips non-owned docs.

**Maps to**: FR-006, FR-007

### Implementation for User Story 3

- [x] T017 [US3] Add ownership check in standalone `assignTasksCmd.RunE` in `cmd/gcal-organizer/assign_tasks.go`: after loading doc ID and before any processing, when `ownedOnly` is true, call `driveSvc.IsFileOwned(ctx, docID)` and if not owned return `fmt.Errorf("document %s is not owned by you; --owned-only prevents processing non-owned documents", docID)` (no doctor reference — this is intentional business logic, not a diagnostic issue), if API error return `fmt.Errorf("cannot verify ownership of document %s: %w\n\nRun 'gcal-organizer doctor' for diagnostics", docID, err)` (doctor reference required per Constitution VII — API failure is a diagnostic scenario)
- [x] T018 [US3] Add verbose log in `cmd/gcal-organizer/main.go` Step 3 (`runCmd`): when `ownedOnly && len(docIDs) == 0`, log `"No owned Notes documents found for task assignment"` to inform the user

**Checkpoint**: Standalone `assign-tasks --doc` rejects non-owned docs with clear error. `run` Step 3 filtering works via US2. Independently testable.

---

## Phase 6: User Story 4 - Visibility Into Skipped Files (Priority: P3)

**Goal**: Provide verbose logging of skipped files with owner details, dry-run interaction showing ownership filtering, and summary count of skipped files at the end.

**Independent Test**: Run any command with `--owned-only --verbose` and verify skipped files are logged with owner email. Run with `--owned-only --dry-run` and verify dry-run output reflects ownership.

**Maps to**: FR-008, FR-009, FR-014

### Implementation for User Story 4

- [x] T019 [US4] Add verbose logging for skipped files in `OrganizeDocuments()` in `internal/organizer/organizer.go`: when `o.config.Verbose` and a file is skipped due to ownership, log `"Skipping non-owned document: %s (owner: %s)"` with the document name and owner email
- [x] T020 [US4] Add verbose logging for skipped shares in `SyncCalendarAttachments()` in `internal/organizer/organizer.go`: when `o.config.Verbose` and sharing is skipped, log `"Skipping share for non-owned attachment: %s"` with the attachment title
- [x] T021 [US4] Add skipped files summary to `PrintSummary()` in `internal/organizer/organizer.go`: when `o.stats.Skipped > 0`, print `"Skipped %d non-owned files (--owned-only active)"` in the summary output
- [x] T022 [US4] Add dry-run interaction in `internal/organizer/organizer.go`: when both `DryRun` and `OwnedOnly` are active, include ownership info in dry-run output messages: `"Would skip non-owned document: %s (owner: %s)"` and `"Would skip sharing non-owned attachment: %s"`

**Checkpoint**: Verbose output shows per-file skip details, dry-run reflects ownership, summary shows skip count. Independently testable.

---

## Phase 7: User Story 5 - Persist Owned-Only as Configuration Default (Priority: P3)

**Goal**: Users can set `owned-only: true` in their configuration file and have it apply to all commands without passing the flag. CLI flag overrides config.

**Independent Test**: Set `GCAL_OWNED_ONLY=true` in config, run any command without the flag, verify owned-only behavior is active. Then run with `--owned-only=false` and verify override.

**Maps to**: FR-011, FR-012

### Implementation for User Story 5

- [x] T023 [US5] Verify config persistence works end-to-end: the viper binding from T003 (`viper.BindPFlag("owned-only", ...)`) and the config load from T001 (`cfg.OwnedOnly = viper.GetBool("owned-only")`) should already support config file persistence. Verify by running `go test ./internal/config/...` with the env var test from T002. If additional binding is needed (e.g., `viper.BindEnv("owned_only", "GCAL_OWNED_ONLY")` in `Load()`), add it to `internal/config/config.go`
- [x] T024 [US5] Verify CLI override works: confirm that explicit `cfg.OwnedOnly = ownedOnly` in each command's `RunE` (from T003) ensures the CLI flag takes precedence over config file. This is the same dual-path pattern used by `--dry-run`

**Checkpoint**: Config persistence and CLI override both work. Independently testable.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, CI validation, and final verification across all stories.

- [x] T025 [P] Add `--owned-only` to the flags reference table in `README.md`, add usage example `gcal-organizer run --owned-only --verbose`, and add a section explaining owned-only behavior and its limitations (Shared Drive exclusion)
- [x] T026 [P] Add `--owned-only` flag description to `man/gcal-organizer.1` and add example in EXAMPLES section
- [x] T027 [P] Add `owned-only` configuration example to `docs/SETUP.md` with note about Shared Drive limitation and `GCAL_OWNED_ONLY` env var
- [x] T028 Run `go vet ./...` and `gofmt -l .` to verify no warnings or formatting issues
- [x] T029 Run `make ci` to verify full CI suite passes with all changes
- [x] T030 Run quickstart.md validation: manually verify examples from `specs/006-owned-only-flag/quickstart.md` work as documented

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 (Config.OwnedOnly must exist) - BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Phase 2 (`IsFileOwned` available, but US1 uses `doc.IsOwned` directly so technically only needs Phase 1)
- **User Story 2 (Phase 4)**: Depends on Phase 2 (`IsFileOwned()` needed for calendar attachment ownership checks)
- **User Story 3 (Phase 5)**: Depends on Phase 2 (`IsFileOwned()` needed for standalone assign-tasks check) and Phase 4 (notesDocIDs filtering)
- **User Story 4 (Phase 6)**: Depends on Phase 3 and Phase 4 (logging is added to the filtering code from US1/US2)
- **User Story 5 (Phase 7)**: Depends on Phase 1 (verifies config persistence already set up)
- **Polish (Phase 8)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Independent after Phase 2 - no dependencies on other stories
- **User Story 2 (P1)**: Independent after Phase 2 - no dependencies on other stories
- **User Story 3 (P2)**: Depends on US2 for notesDocIDs filtering; standalone assign-tasks is independent
- **User Story 4 (P3)**: Depends on US1 and US2 (adds logging to their filtering code)
- **User Story 5 (P3)**: Independent after Phase 1 - verifies existing setup

### Within Each User Story

- Tests MUST be written and FAIL before implementation (Constitution Principle III)
- Implementation follows test failures
- Run tests after implementation to verify they pass
- Story complete before moving to next priority

### Parallel Opportunities

- T001 and T003 modify different files (`config.go` vs `main.go`) - can run in parallel
- T002 and T003 modify different files (`config_test.go` vs `main.go`) - can run in parallel
- T005 and T006 modify the same file (`service.go`) - must be sequential
- T025, T026, T027 modify different documentation files - can all run in parallel
- US1 and US2 modify the same file (`organizer.go`) but different functions - could be parallel with care, but sequential is safer
- US5 is fully independent and can run in parallel with US3 or US4

---

## Parallel Example: Phase 1 Setup

```bash
# These can run in parallel (different files):
Task T001: "Add OwnedOnly to Config struct in internal/config/config.go"
Task T003: "Register --owned-only flag in cmd/gcal-organizer/main.go"

# Then sequentially:
Task T002: "Add OwnedOnly tests in internal/config/config_test.go" (depends on T001)
Task T004: "Run go test and go vet" (depends on T001-T003)
```

## Parallel Example: Phase 8 Documentation

```bash
# These can all run in parallel (different files):
Task T025: "Update README.md with --owned-only documentation"
Task T026: "Update man/gcal-organizer.1 with --owned-only"
Task T027: "Update docs/SETUP.md with owned-only config example"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001-T004)
2. Complete Phase 2: Foundational (T005-T008)
3. Complete Phase 3: User Story 1 (T009-T012)
4. **STOP and VALIDATE**: Run `gcal-organizer organize --owned-only` against real documents
5. Deploy/demo if ready — core ownership protection is functional

### Incremental Delivery

1. Setup + Foundational -> Config and Drive service ready
2. Add US1 (organize protection) -> Test independently -> Core MVP
3. Add US2 (sync-calendar sharing) -> Test independently -> Full P1 coverage
4. Add US3 (assign-tasks blocking) -> Test independently -> P2 complete
5. Add US4 (observability) + US5 (config persistence) -> Test independently -> Full feature
6. Polish -> Documentation, CI, quickstart validation

### Sequential Strategy (Single Developer)

Phase 1 -> Phase 2 -> Phase 3 (US1) -> Phase 4 (US2) -> Phase 5 (US3) -> Phase 6 (US4) -> Phase 7 (US5) -> Phase 8 (Polish)

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Constitution Principle III requires TDD - write tests first, verify they fail, then implement
- US1 and US2 are both P1 priority but US1 is recommended as MVP since it requires no additional API calls
- The `organizer.go` file is touched by US1, US2, and US4 — execute these sequentially to avoid merge conflicts
- All 30 tasks are specific enough for an LLM to execute without additional context
