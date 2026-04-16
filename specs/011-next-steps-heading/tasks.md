# Tasks: Support Updated Notes by Gemini Heading and Section Position

**Input**: Design documents from `/specs/011-next-steps-heading/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, quickstart.md
**Tests**: Yes — this is a bug fix; all behavioral changes require test coverage.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3, US4)
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: No setup needed — existing project, no new dependencies or files.

*(Skipped — all changes modify existing files in an established project structure.)*

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core helpers and constants that MUST be complete before any user story implementation.

**⚠️ CRITICAL**: US1–US4 all depend on these foundational changes to `internal/docs/service.go`.

- [x] T001 [US1,US2] Replace the `SuggestedNextStepsHeading` constant (line 70 of `internal/docs/service.go`) with a package-level variable `NextStepsHeadingCandidates = []string{"Next steps", "Suggested next steps"}`. Keep the old constant temporarily if `grep -r SuggestedNextStepsHeading` shows other references; otherwise remove it. Update the doc comment to reference FR-004 (first match in document order wins) and note extensibility for future heading names.

- [x] T002 Add an `isHeadingParagraph(para *docs.Paragraph) bool` helper function to `internal/docs/service.go` near the existing `extractParagraphText` function (line 158). The function returns `true` if `para.ParagraphStyle != nil && strings.HasPrefix(para.ParagraphStyle.NamedStyleType, "HEADING_")`. Add a GoDoc comment explaining it checks for H1–H6 heading styles per the Google Docs API `NamedStyleType` field.

- [x] T003 Add a `matchesNextStepsHeading(para *docs.Paragraph) bool` helper function to `internal/docs/service.go`. The function returns `true` only if `isHeadingParagraph(para)` is true AND `strings.EqualFold(strings.TrimSpace(extractParagraphText(para)), candidate)` matches any entry in `NextStepsHeadingCandidates`. Add a GoDoc comment referencing FR-001, FR-002, and the spec's requirement for full heading match (not substring) to avoid matching body text.

- [x] T004 [P] Add table-driven unit tests for `isHeadingParagraph` in `internal/docs/service_test.go`. Test cases: `HEADING_1` (true), `HEADING_6` (true), `NORMAL_TEXT` (false), `TITLE` (false), nil `ParagraphStyle` (false), empty `Paragraph{}` (false). Use the table-driven pattern consistent with existing tests like `TestUtf16Len`.

- [x] T005 [P] Add unit tests for `matchesNextStepsHeading` in `internal/docs/service_test.go`. Test cases: "Next steps" with HEADING_2 style (true), "Suggested next steps" with HEADING_2 style (true), "NEXT STEPS" with HEADING_1 style (true — case-insensitive), "Next steps" with NORMAL_TEXT style (false — body text), "We discussed the next steps" with HEADING_2 style (false — not full match), " Next steps " with HEADING_2 style (true — whitespace trimmed), empty text with HEADING_2 style (false).

**Checkpoint**: Helpers and constants ready — `go test ./internal/docs/...` passes with new helper tests.

---

## Phase 3: US1 + US2 — New Heading Support + Backward Compatibility (Priority: P1)

**Goal**: The system recognizes both "Next steps" and "Suggested next steps" headings for checkbox extraction, using full heading match with heading style verification.

**Independent Test**: Run `go test ./internal/docs/...` — new tests for "Next steps" heading pass; existing tests for "Suggested next steps" continue to pass.

### Tests for US1 + US2 (Write FIRST, ensure they FAIL)

- [x] T006 [P] [US1] Add `TestExtractItemsFromSection_NextStepsHeading` to `internal/docs/service_test.go`. Create a document structure with a "Next steps" paragraph (ParagraphStyle: HEADING_2) followed by one bullet item. Assert that `extractItemsFromSection` returns exactly 1 item with the correct text. See quickstart.md §4a for the exact test structure.

- [x] T007 [P] [US2] Update the existing `TestExtractItemsFromSection_SuggestedNextSteps` test in `internal/docs/service_test.go` (line 13). Add `ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_2"}` to the "Suggested next steps" paragraph element (currently has empty `ParagraphStyle{}`). This is required because the new implementation checks for heading style. Verify the test still passes after the implementation change — this confirms backward compatibility.

- [x] T008 [P] [US2] Update `TestExtractItemsFromSection_ProcessedEmoji` (line 100), `TestExtractItemsFromSection_EmptyBullet` (line 151), and `TestExtractItemsFromSection_NilParagraph` (line 186) in `internal/docs/service_test.go`. Add `NamedStyleType: "HEADING_2"` to each test's "Suggested next steps" paragraph's `ParagraphStyle`. These tests must continue to pass after the implementation change to confirm FR-008 (idempotency preservation) and existing edge case handling.

- [x] T009 [P] [US1] Add `TestExtractItemsFromSection_CaseInsensitive` to `internal/docs/service_test.go`. Test that "NEXT STEPS" (all caps) with HEADING_2 style is recognized. Create a document with heading "NEXT STEPS" and one bullet; assert 1 item returned.

### Implementation for US1 + US2

- [x] T010 [US1,US2] Modify the `extractItemsFromSection` method in `internal/docs/service.go` (lines 110–156). Replace the current heading detection logic (`strings.Contains` substring match on line 123) with a call to `matchesNextStepsHeading(para)`. Rename `inSuggestedSection` to `inSection`. Move `paraText` extraction to after the section-entry check (it's only needed for bullet text, not heading detection). Update the function's doc comment to reference both heading names. The `continue` after setting `inSection = true` must remain to skip the heading paragraph itself. See quickstart.md §1d for the complete replacement code.

- [x] T011 [US1] Update the `ExtractCheckboxItems` doc comment (line 75 of `internal/docs/service.go`) from `"Suggested next steps"` to `"Next steps" or "Suggested next steps"`. Reference FR-006 (empty slice when no matching heading found).

**Checkpoint**: `go test ./internal/docs/...` passes. Both "Next steps" and "Suggested next steps" headings are recognized. Existing tests confirm backward compatibility.

---

## Phase 4: US3 — Section Boundary Enforcement (Priority: P2)

**Goal**: Extraction stops at the next heading after the target section, preventing false positives from subsequent sections.

**Independent Test**: Run `go test ./internal/docs/...` — boundary tests pass; items from sections after the target heading are excluded.

### Tests for US3 (Write FIRST, ensure they FAIL)

- [x] T012 [P] [US3] Add `TestExtractItemsFromSection_SectionBoundary` to `internal/docs/service_test.go`. Create a document with: "Next steps" (HEADING_2) → 1 bullet → "Meeting summary" (HEADING_2) → 1 bullet. Assert that only 1 item is returned (the one under "Next steps"), not the bullet under "Meeting summary". See quickstart.md §4b.

- [x] T013 [P] [US3] Add `TestExtractItemsFromSection_FirstMatchWins` to `internal/docs/service_test.go`. Create a document with: "Next steps" (HEADING_2) → 1 bullet → "Suggested next steps" (HEADING_2) → 1 bullet. Assert that only 1 item is returned (from the first "Next steps" section). The second heading acts as a section boundary. See quickstart.md §4c.

- [x] T014 [P] [US3] Add `TestExtractItemsFromSection_NoBoundary_EndOfDoc` to `internal/docs/service_test.go`. Create a document with: "Next steps" (HEADING_2) → 3 bullets → (end of document, no following heading). Assert that all 3 items are returned. This confirms legacy behavior is preserved when the section is at the end of the document (US3-AS3).

- [x] T015 [P] [US3] Add `TestExtractItemsFromSection_BoundaryAtDifferentHeadingLevel` to `internal/docs/service_test.go`. Create a document with: "Next steps" (HEADING_2) → 1 bullet → "Details" (HEADING_3) → 1 bullet. Assert only 1 item returned. This confirms FR-003: boundary enforcement works at any heading level (H1–H6), not just the same level.

### Implementation for US3

- [x] T016 [US3] Add section boundary enforcement to `extractItemsFromSection` in `internal/docs/service.go`. After the `inSection` flag is set to `true`, add a check: `if isHeadingParagraph(para) { break }`. This must appear BEFORE the bullet check (`para.Bullet == nil`). The break exits the loop when any heading (H1–H6) is encountered after entering the target section. See quickstart.md §1d for placement.

**Checkpoint**: `go test ./internal/docs/...` passes. Section boundary enforcement works at all heading levels. Legacy end-of-document behavior preserved.

---

## Phase 5: US4 — Graceful Missing Section Handling (Priority: P3)

**Goal**: When no matching heading exists, the system returns an empty result without errors.

**Independent Test**: Run `go test ./internal/docs/...` — missing section test returns empty slice, no error.

### Tests for US4

- [x] T017 [P] [US4] Add `TestExtractItemsFromSection_BodyTextNotMatched` to `internal/docs/service_test.go`. Create a document with a NORMAL_TEXT paragraph containing "We discussed the next steps for the project" followed by a bullet item. Assert 0 items returned — body text containing "next steps" must NOT trigger section matching. See quickstart.md §4d.

- [x] T018 [P] [US4] Verify the existing `TestExtractItemsFromSection_NoSuggestedSection` test (line 67 of `internal/docs/service_test.go`) still passes. This test already covers FR-006 (empty result when no matching heading found). No changes needed to this test — it uses "Some other heading" which has no heading style, so it correctly tests the missing-section path.

### Implementation for US4

*(No additional implementation needed — the changes in T010 and T016 already satisfy US4. The `matchesNextStepsHeading` function requires heading style, so body text is never matched. When no heading matches, `inSection` stays false and the function returns an empty slice. T017 and T018 verify this.)*

**Checkpoint**: `go test ./internal/docs/...` passes. Missing sections return empty results. Body text is not falsely matched.

---

## Phase 6: Browser Automation Updates

**Purpose**: Update the Playwright browser script to try the new heading first, with fallback to the old heading.

- [x] T019 [US1,US2] Add a `NEXT_STEPS_HEADINGS` constant array to `browser/assign-tasks.ts` after the interface declarations (around line 48). Define it as `const NEXT_STEPS_HEADINGS = ['Next steps', 'Suggested next steps'] as const;`. Add a JSDoc comment referencing FR-005 (try "Next steps" first, fall back to "Suggested next steps").

- [x] T020 [US1,US2,US4] Replace the hardcoded "Suggested next steps" find-bar navigation in `browser/assign-tasks.ts` (lines 184–210) with a fallback loop over `NEXT_STEPS_HEADINGS`. For each candidate: fill the find input, wait 500ms, read the match count from `.docs-findinput-count` via `page.evaluate()`, and check if `total > 0`. If found, log the match and break. If no candidate matches, log a warning per FR-007. After the loop, press Enter to jump to the match and Escape to close the find bar. See quickstart.md §2b for the complete replacement code.

- [x] T021 [P] [US1] Update the file header comment in `browser/assign-tasks.ts` (line 7). Change `"Suggested next steps"` to `"Next steps" (or "Suggested next steps")` to reflect the new behavior.

**Checkpoint**: Browser script compiles (`npx tsx --check browser/assign-tasks.ts` or equivalent). Fallback logic is in place.

---

## Phase 7: Polish & Documentation

**Purpose**: Update help text and documentation to reflect the new behavior.

- [x] T022 [P] Update the `Long` description of `assignTasksCmd` in `cmd/gcal-organizer/assign_tasks.go` (line 32). Change `"Suggested next steps"` to `"Next steps" (or "Suggested next steps")` to match the new behavior.

- [x] T023 [P] Update `docs/SETUP.md` line 198. Change `"Suggested next steps"` to `"Next steps" (or "Suggested next steps")` in the troubleshooting section about tasks not being assigned.

- [x] T024 Run full verification: `go build ./... && go test ./... && go vet ./... && gofmt -l . && go mod tidy`. All must pass with zero warnings and no diff.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 2 (Foundational)**: No dependencies — start immediately. BLOCKS all user story phases.
- **Phase 3 (US1+US2)**: Depends on Phase 2 (T001–T003 for helpers/constants).
- **Phase 4 (US3)**: Depends on Phase 3 (T010 for the refactored `extractItemsFromSection`).
- **Phase 5 (US4)**: Depends on Phase 3 (T010). Can run in parallel with Phase 4.
- **Phase 6 (Browser)**: Independent of Go-side phases. Can run in parallel with Phases 3–5.
- **Phase 7 (Polish)**: Depends on all previous phases being complete.

### Parallel Opportunities

- T004 + T005: Helper tests can run in parallel (same file, different test functions — no conflicts)
- T006 + T007 + T008 + T009: US1/US2 tests can run in parallel (adding independent test functions)
- T012 + T013 + T014 + T015: US3 tests can run in parallel (adding independent test functions)
- T017 + T018: US4 tests can run in parallel
- T019 → T020 → T021: Browser tasks are sequential (T020 depends on T019's constant)
- T022 + T023: Documentation updates can run in parallel (different files)
- Phase 6 (browser) can run in parallel with Phases 3–5 (Go-side changes) — different files entirely

### Within Each Phase

- Tests MUST be written and FAIL before implementation
- Implementation tasks are sequential within a phase
- Run `go test ./internal/docs/...` at each checkpoint

---

## Summary

| Phase | Tasks | Files Modified |
|-------|-------|----------------|
| Phase 2: Foundational | T001–T005 | `internal/docs/service.go`, `internal/docs/service_test.go` |
| Phase 3: US1+US2 | T006–T011 | `internal/docs/service.go`, `internal/docs/service_test.go` |
| Phase 4: US3 | T012–T016 | `internal/docs/service.go`, `internal/docs/service_test.go` |
| Phase 5: US4 | T017–T018 | `internal/docs/service_test.go` |
| Phase 6: Browser | T019–T021 | `browser/assign-tasks.ts` |
| Phase 7: Polish | T022–T024 | `cmd/gcal-organizer/assign_tasks.go`, `docs/SETUP.md` |

**Total**: 24 tasks across 7 phases, modifying 5 existing files (no new files created).
<!-- spec-review: passed -->
<!-- code-review: passed -->
