# Feature Specification: Comprehensive Test Coverage & Contract Quality

**Feature Branch**: `009-test-coverage-quality`  
**Created**: 2026-03-02  
**Status**: Draft  
**Input**: User description: "Comprehensive test coverage and contract quality improvement to eliminate GazeCRAP Q3/Q4 functions, achieve high branch coverage, and add contract assertions across all internal packages"

## Clarifications

### Session 2026-03-02

- Q: Should the spec require a `.gaze.yaml` configuration file to tune the contractual classification threshold? → A: Add `.gaze.yaml` with `contractual-threshold: 65`.
- Q: How should the spec address the multi-target test detection problem? → A: Rename test helper constructors to avoid gaze target detection heuristics.
- Q: Should the spec require adding direct return-value/receiver-field assertions alongside existing mock-based assertions? → A: Require direct assertions only for functions with GazeCRAP > 15 or in Q4 (targeted, highest impact).

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Developer Validates Orchestration Logic (Priority: P1)

As a developer making changes to the organizer package (the core workflow orchestrator), I need every branch of `OrganizeDocuments`, `SyncCalendarAttachments`, `ExtractDecisionsForDoc`, `printSummary`, `logActionResult`, and `logCalendarAction` to be covered by tests with contract assertions so that I can refactor safely and catch regressions in stats tracking, error propagation, and downstream call arguments.

**Why this priority**: The organizer package is the highest-risk area — it orchestrates all business logic, and its two most complex functions (`SyncCalendarAttachments` at complexity 38, `OrganizeDocuments` at complexity 17) currently sit in GazeCRAP Q4 (dangerous: complex with no contract coverage). A regression here silently breaks the entire application.

**Independent Test**: Can be fully tested by running the organizer test suite and verifying all stat counters, error messages, and downstream mock call arguments are asserted in every test case. Delivers confidence that every orchestration branch behaves correctly.

**Acceptance Scenarios**:

1. **Given** the organizer package, **When** the full test suite runs, **Then** line coverage exceeds 85% and every test asserts at least one contract (stat counter, downstream argument, or error message).
2. **Given** `printSummary` with various stat combinations (dry-run, decisions present, errors present, skipped files), **When** tests run, **Then** every conditional branch is exercised without panics.
3. **Given** `logActionResult` with all possible `ActionResult` states (skipped/success, various reasons), **When** tests run, **Then** each test verifies the correct stat counter is incremented and the correct log level is used.
4. **Given** `logCalendarAction` with all possible states, **When** tests run, **Then** `ShortcutsCreated` and `Errors` stats are verified for each branch.
5. **Given** `RunFullWorkflow`, **When** tested, **Then** it calls `OrganizeDocuments` then `SyncCalendarAttachments` in sequence, and errors from either propagate correctly.

---

### User Story 2 — Developer Validates Document Service Logic (Priority: P1)

As a developer modifying the docs package, I need tests for `CreateDecisionsTab` (the most complex function in the codebase at complexity 25) and `ExtractCheckboxItems` that exercise all error-handling paths, so that API failures, duplicate tabs, and cleanup rollbacks are caught before deployment.

**Why this priority**: `CreateDecisionsTab` has the highest CRAP score in the codebase (650) with 0% coverage. It contains retry logic, duplicate detection, content insertion, and rollback cleanup — all untested. A bug here would corrupt user documents.

**Independent Test**: Can be fully tested by running the docs test suite with a mock HTTP server substituted for the Google Docs API. Delivers coverage of all 4 error-handling paths in `CreateDecisionsTab` and all branches of `ExtractCheckboxItems`.

**Acceptance Scenarios**:

1. **Given** `CreateDecisionsTab`, **When** the API returns success for both tab creation and content insertion, **Then** the function returns nil and exactly 2 API calls are made.
2. **Given** `CreateDecisionsTab`, **When** the API returns a 409 conflict, **Then** the function returns the duplicate-tab sentinel error.
3. **Given** `CreateDecisionsTab`, **When** content insertion fails but cleanup succeeds, **Then** the error message mentions "empty tab cleaned up".
4. **Given** `CreateDecisionsTab`, **When** both content insertion and cleanup fail, **Then** the error message mentions both failures.
5. **Given** `ExtractCheckboxItems`, **When** a document has a "Notes" tab with checkbox items, **Then** only items from the "Suggested next steps" section are returned.
6. **Given** the docs package, **When** the full test suite runs, **Then** line coverage exceeds 80%.

---

### User Story 3 — Developer Validates AI Integration Parsing (Priority: P1)

As a developer modifying the AI integration, I need tests for `ExtractDecisions` and `ExtractAssigneesFromCheckboxes` that exercise the full API call path including response parsing, so that malformed AI responses, empty results, and API errors are handled correctly.

**Why this priority**: These functions are the bridge between the AI service and the application logic. They have CRAP scores of 90 and 110 with 0% coverage on the API-calling methods. The response parsing is already well-tested, but the orchestration around it (prompt construction, response extraction, error handling) is not.

**Independent Test**: Can be fully tested by running the gemini test suite with a mock HTTP server for the AI API endpoint. Delivers confidence that prompt construction, response parsing, and error handling work end-to-end.

**Acceptance Scenarios**:

1. **Given** `ExtractDecisions` with valid transcript text, **When** the AI API returns a well-formed JSON response, **Then** the function returns correctly categorized decisions.
2. **Given** `ExtractDecisions` with empty transcript text, **When** called, **Then** it returns nil without making an API call.
3. **Given** `ExtractDecisions`, **When** the AI API returns no candidates, **Then** the function returns a descriptive error.
4. **Given** `ExtractAssigneesFromCheckboxes` with valid items, **When** the API returns assignments with null assignees, **Then** null entries are filtered from the result.
5. **Given** the gemini package, **When** the full test suite runs, **Then** line coverage exceeds 85%.

---

### User Story 4 — Developer Validates Calendar Event Parsing (Priority: P2)

As a developer, I need the calendar package's `parseEvent` function (CRAP 132, complexity 11, 0% coverage) tested with all date/time format combinations and attachment/attendee extraction, so that edge cases in event parsing don't cause silent data loss.

**Why this priority**: Calendar event parsing is the entry point for all attachment and decision workflows. Incorrect parsing silently drops events, attachments, or attendees. The function handles 4 date formats and multiple optional fields.

**Independent Test**: Can be fully tested by running the calendar test suite with constructed calendar event structs. Delivers coverage of all date parsing branches and attachment extraction.

**Acceptance Scenarios**:

1. **Given** a calendar event with `Start.DateTime` in RFC3339 format, **When** parsed, **Then** the resulting event start time matches the expected time.
2. **Given** a calendar event with `Start.Date` (all-day event), **When** parsed, **Then** the start time is parsed correctly from the date-only format.
3. **Given** a calendar event with attachments and Drive links in the description, **When** parsed, **Then** both explicit attachments and description-extracted links are included.
4. **Given** a calendar event with attendees, **When** parsed, **Then** each attendee's email, display name, self flag, and organizer flag are correctly mapped.
5. **Given** the calendar package, **When** the full test suite runs, **Then** line coverage exceeds 80%.

---

### User Story 5 — Developer Validates Configuration and Utility Packages (Priority: P2)

As a developer, I need remaining coverage gaps in config, secrets, ux, and drive (pure logic only) closed, so that the overall project health grade improves and every package meets a minimum quality bar.

**Why this priority**: These packages have smaller individual impact but collectively represent significant uncovered surface area. Closing these gaps lifts the overall project coverage above 55% and eliminates remaining Q3 functions.

**Independent Test**: Can be tested per-package independently. Each package delivers independent value.

**Acceptance Scenarios**:

1. **Given** `config.LoadSecrets` with a mock secret store containing values, **When** called, **Then** config fields are populated from the store.
2. **Given** `secrets.Backend.String()`, **When** called for each backend type, **Then** it returns the correct human-readable name.
3. **Given** all 8 ux error constructors, **When** called, **Then** each returns an error with the expected message and fix suggestion fields.
4. **Given** `drive.escapeQuery` with strings containing single quotes, **When** called, **Then** quotes are properly escaped.
5. **Given** each package, **When** the test suite runs, **Then** config exceeds 90%, secrets exceeds 85%, ux reaches 100%, and drive exceeds 15%.

---

### User Story 6 — Developer Extracts and Tests Pure Functions from CLI (Priority: P3)

As a developer, I need pure logic functions currently in the CLI entry-point package (`loadDotEnv`, `extractUnassignedItems`, `maskSecret`, `truncateText`) extracted into testable internal packages and covered by tests, so that the highest-CRAP CLI functions become maintainable.

**Why this priority**: The CLI package has 39 functions at 0% coverage. Most are CLI wiring that isn't worth unit testing, but 4 pure functions contain real logic (especially `loadDotEnv` at CRAP 240, complexity 15) that can be extracted and tested. This is lower priority because it requires refactoring production code, not just adding tests.

**Independent Test**: Can be tested by running tests for the target internal packages after extraction. Each extracted function is independently valuable.

**Acceptance Scenarios**:

1. **Given** `loadDotEnv` (extracted to internal config package), **When** tested with a .env file containing comments, blank lines, KEY=VALUE pairs, quoted values, tilde expansion, and POSIX escaping, **Then** all parsing rules produce correct results.
2. **Given** `loadDotEnv`, **When** an environment variable is already set, **Then** the .env value does not override it (env vars take precedence).
3. **Given** `extractUnassignedItems` (extracted to internal docs package), **When** given a mix of processed and unprocessed items, **Then** only unprocessed items are returned with correct indices.
4. **Given** `maskSecret`, **When** given strings of various lengths, **Then** the correct number of characters are revealed and the rest are masked.
5. **Given** `truncateText`, **When** given strings shorter than, equal to, and longer than the max length, **Then** truncation with ellipsis works correctly for ASCII and Unicode.

---

### Edge Cases

- What happens when a mock HTTP server is slow or unresponsive? Tests must use short timeouts and context cancellation to avoid hanging.
- How does the test suite handle concurrent test execution? All tests must be safe to run with the race detector enabled.
- What happens when a mock returns unexpected types or nil pointers? Tests must exercise nil-safety in all code paths.
- How are tests structured when a function has both pure logic and API calls? Pure logic is tested directly; API orchestration uses mock HTTP servers or interface mocks.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: All functions in the organizer package with complexity >= 3 MUST have tests that exercise every conditional branch and assert on contract outputs (stat counters, error messages, downstream call arguments).
- **FR-002**: `CreateDecisionsTab` MUST have tests covering all 4 error-handling paths: happy path, duplicate tab detection, content insertion failure with successful cleanup, and content insertion failure with failed cleanup.
- **FR-003**: `ExtractCheckboxItems` MUST have tests verifying Notes tab selection, section boundary detection, and checkbox extraction with processed-item filtering.
- **FR-004**: `ExtractDecisions` and `ExtractAssigneesFromCheckboxes` MUST have tests using a mock HTTP server that exercise prompt construction, response parsing, empty input handling, and API error handling.
- **FR-005**: `parseEvent` MUST have tests covering all 4 date/time parsing branches (DateTime, Date-only for both start and end), attachment extraction, description-based Drive link extraction, and attendee mapping.
- **FR-006**: `config.LoadSecrets` MUST have a test verifying that store values override default config values.
- **FR-007**: All 8 ux error constructors MUST have tests verifying message and fix-suggestion field correctness.
- **FR-008**: `drive.escapeQuery` MUST have tests for single-quote escaping.
- **FR-009**: `loadDotEnv` MUST be relocated from the CLI package to an internal package and tested with at least 10 cases covering comments, blank lines, quoting, tilde expansion, env precedence, and invalid keys.
- **FR-010**: `extractUnassignedItems`, `maskSecret`, and `truncateText` MUST be relocated from the CLI package to internal packages and tested.
- **FR-011**: All tests MUST use contract-style assertions — meaning assertions that gaze can map to a target function's classified side effects (return values, error returns, receiver mutations, map mutations). For functions with GazeCRAP > 15 or in GazeCRAP Q4, tests MUST include direct assertions on the function's return values (including error) and/or receiver field mutations, not only on mock call recordings.
- **FR-012**: All tests MUST be race-condition safe (the race detector must not report issues).
- **FR-013**: No new external dependencies MUST be introduced (mock HTTP servers use only the standard library).
- **FR-014**: `printSummary` MUST have tests covering both dry-run and non-dry-run branches, with every conditional stat-check branch exercised.
- **FR-015**: `logActionResult` and `logCalendarAction` MUST have tests covering all action-result states (skipped with various reasons, success, dry-run) with stat counter assertions.
- **FR-016**: `RunFullWorkflow` MUST have a test verifying sequential execution of document organization followed by calendar sync, with error propagation from each step.
- **FR-017**: A `.gaze.yaml` configuration file MUST be created at the repository root with `contractual-threshold: 65` to match the project's signal profile, where most exported functions score 64-79 on gaze's confidence scale due to the combination of visibility and caller signals available.
- **FR-018**: Test helper constructors (e.g., `newTestOrganizer`, `newMockService`, `buildTestDoc`, `buildTab`, `buildParagraphElement`) MUST be renamed to avoid gaze multi-target detection heuristics. Helper names MUST NOT match the pattern of production function names that gaze would consider target functions. Recommended naming convention: prefix with `setup` or `make` (e.g., `setupOrganizer`, `makeTestDoc`).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Overall project line coverage exceeds 55% (up from 29.4%).
- **SC-002**: CRAPload (functions with CRAP score >= 15) drops below 15 (down from 40).
- **SC-003**: Zero functions remain in GazeCRAP Q4 (dangerous: complex with no contract coverage).
- **SC-004**: At most 2 functions remain in GazeCRAP Q3 (simple but underspecified).
- **SC-005**: The organizer package line coverage exceeds 85%.
- **SC-006**: The docs package line coverage exceeds 80%.
- **SC-007**: The gemini package line coverage exceeds 85%.
- **SC-008**: The calendar package line coverage exceeds 80%.
- **SC-009**: The config package line coverage exceeds 90%.
- **SC-010**: The secrets package line coverage exceeds 85%.
- **SC-011**: The ux package reaches 100% line coverage.
- **SC-012**: All tests pass with status 0.
- **SC-013**: The race detector finds no issues across all tests.
- **SC-014**: Every test function includes at least one contract assertion (verifying a behavioral output, not just exercising a code path).
- **SC-015**: Average contract coverage across gaze-analyzed functions exceeds 50%.
- **SC-016**: Gaze quality analysis reports fewer than 20 multi-target skip warnings across the entire module.

### Assumptions

- The quality measurement tool is available for measuring CRAP scores and contract coverage.
- The decision-extraction feature branch changes (which already improved docs and organizer coverage) will be merged before this work begins, providing a higher baseline.
- Drive API wrapper methods (CRAP 20-210) are intentionally left untested because they are thin wrappers around the Google Drive SDK; testing them via mock HTTP servers would be high-effort with low value since the logic is in the SDK, not our code.
- The auth package is intentionally excluded because it delegates to the standard OAuth2 library and testing it provides minimal value relative to effort.
- CLI functions that are pure CLI wiring, browser automation, or OS-specific install logic are intentionally excluded from testing scope.
- Functions at complexity 1-2 with 0% coverage in the CLI package (style helpers, template generators) are not worth testing and are excluded.
- Gaze classifies side effects as "contractual" when their confidence score meets or exceeds the `contractual-threshold` configured in `.gaze.yaml`. With the project's signal profile (visibility + caller signals typically yielding 64-79), a threshold of 65 is appropriate to promote most exported function effects from "ambiguous" to "contractual."

### Dependencies

- Depends on the decision-extraction feature being merged to main (provides the docs/gemini service interfaces, decision extraction method, and existing test infrastructure).
- No new external dependencies required — all test infrastructure uses the standard library.
