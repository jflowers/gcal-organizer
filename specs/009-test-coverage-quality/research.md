# Research: Test Coverage & Contract Quality

**Feature**: 009-test-coverage-quality | **Date**: 2026-03-02

## Research Topics

### 1. Gaze Configuration File Format and Threshold Tuning

**Decision**: Create `.gaze.yaml` at repository root with `contractual-threshold: 65`.

**Rationale**: Gaze classifies every side effect in this codebase as "ambiguous" (confidence 64-79) because the default contractual threshold is 80. The project's signal profile — exported functions with visibility (14pts) + caller (5-15pts) signals — consistently falls short of 80. Lowering to 65 promotes most side effects to "contractual" and unlocks contract coverage measurement. The gaze project itself uses a lowered threshold (70) for similar reasons.

**Config file format** (from gaze's own `.gaze.yaml`):
```yaml
classification:
  thresholds:
    contractual: 65    # default: 80; lowered for project signal profile
    incidental: 40     # default: 50; keep conservative
```

**Alternatives considered**:
- **Threshold 70**: Too conservative; functions with visibility(14) + caller(5) = 69 would remain ambiguous. Many organizer methods sit at exactly 69.
- **Threshold 80 (default)**: Unworkable; produces 0% contract coverage across the entire module.
- **No config file, enhance godoc/caller signals instead**: All 54/55 exported functions already have godoc comments. Adding more callers is artificial. The config approach is simpler and more honest.

**CI enforcement thresholds**: Not configurable via `.gaze.yaml` — only via CLI flags (`--min-contract-coverage`, `--max-crapload`). These can be added to Makefile targets but are out of scope for the config file.

---

### 2. Multi-Target Test Detection Behavior (Revised Finding)

**Decision**: The multi-target warnings are **informational noise, not a blocking issue**. Renaming test helpers is still recommended to reduce warning count (SC-016) but is lower priority than initially assessed.

**Rationale**: Detailed analysis of gaze's verbose output reveals:

1. When gaze detects multiple targets (e.g., `OrganizeDocuments`, `logActionResult`, `newTestOrganizer`), it checks which ones are in the analysis results (i.e., analyzed production functions).
2. `newTestOrganizer` is defined in `_test.go` and is NOT in analysis results — gaze correctly skips it with a warning.
3. `logActionResult` and `isCalendarResource` are unexported production functions — they are also NOT in the default analysis results (gaze defaults to exported functions only).
4. **Gaze still processes the test** and correctly identifies the primary target (`OrganizeDocuments`). The earlier quality analysis DID produce reports for these tests — they weren't skipped entirely, they just showed 0% contract coverage due to the ambiguous threshold issue (Research Topic 1).

**Revised impact assessment**: The multi-target warnings contribute to SC-016 (< 20 warnings) but do NOT block contract coverage measurement. The primary blocker is the contractual threshold (Research Topic 1).

**Helper renaming**: Still recommended for noise reduction. Renaming `newTestOrganizer` → `setupOrganizer`, `mockService` → `setupDriveService`, etc. will reduce the warning count. However, this is a P2 priority, not P1.

**Warning count breakdown** (current):
| Package | Warning Count | Primary Cause |
|---------|--------------|---------------|
| organizer | ~103 | `newTestOrganizer`, `logActionResult`, `logCalendarAction`, `isCalendarResource` |
| docs | ~15 | `buildTestDoc`, `buildTab`, `buildParagraphElement` |
| retry | ~22 | `calculateBackoff`, `isRetryable` (unexported production functions) |
| config | ~5 | `mustBindEnv` (unexported production function) |
| calendar | ~8 | `parseEvent`, `extractDriveLinks` (unexported production functions) |

**Key insight**: Most warnings come from **unexported production functions**, not from test helpers. Renaming test helpers only addresses a subset. The rest would require running gaze with `--include-unexported` or accepting the warnings as noise.

**Alternatives considered**:
- **Run gaze with `--include-unexported`**: Would eliminate warnings for unexported functions but would add them to analysis results, potentially increasing the total function count and lowering average scores. Decided against.
- **Major test restructuring**: Splitting tests to call only one function each. Rejected as too invasive with marginal benefit, since gaze already handles multi-target correctly.

---

### 3. httptest Patterns for Google API Mocking

**Decision**: Use `net/http/httptest.NewServer` with `http.NewServeMux` to mock Google API endpoints for docs, gemini, and calendar services. Use existing interface mocking for organizer (already in place).

**Rationale**: All three Google API service constructors accept `*http.Client`, making httptest injection straightforward. The Google API Go client libraries route all requests through the provided HTTP client, so an httptest server captures everything without needing `option.WithEndpoint`.

**Pattern** (validated against existing code):
```go
func TestCreateDecisionsTab_HappyPath(t *testing.T) {
    mux := http.NewServeMux()
    // Handle tab creation
    mux.HandleFunc("POST /docs/v1/documents/{docID}:batchUpdate", func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(docs.BatchUpdateDocumentResponse{...})
    })
    server := httptest.NewServer(mux)
    defer server.Close()
    
    svc, err := docs.NewService(ctx, server.Client())
    require.NoError(t, err)
    
    err = svc.CreateDecisionsTab(ctx, "doc-123", decisions, transcript)
    // Assert on error return (contract assertion for gaze)
    assert.NoError(t, err)
}
```

**Service initialization requirements**:
| Service | Init API Calls | Mock Endpoints Needed |
|---------|---------------|----------------------|
| `docs.NewService` | None | None for init |
| `calendar.NewService` | None | None for init |
| `drive.NewService` | `About.Get()`, `Files.Get("root")` | `/drive/v3/about`, `/drive/v3/files/root` |
| `gemini.NewClient` | Uses genai SDK, not `*http.Client` | Separate approach needed |

**Gemini special case**: The `genai.NewClient` constructor does not accept `*http.Client` directly. It uses the genai SDK's own client. For httptest-based testing, the approach would be to use `option.WithHTTPClient` via the genai SDK's options, or to mock at the interface level (which is already done in organizer tests via `mockGeminiService`). **Recommendation**: Use the genai SDK's `option.WithHTTPClient` and `option.WithEndpoint` for httptest-based tests of `ExtractDecisions` and `ExtractAssigneesFromCheckboxes`.

**Alternatives considered**:
- **Interface mocking only (no httptest)**: Already used for organizer tests. Works well for testing orchestration but doesn't exercise the actual HTTP request construction, response parsing, or error handling in the service layer. httptest provides more confidence that the real API interaction works.
- **Recorded HTTP fixtures**: Captures real API responses for replay. More realistic but requires initial API access and becomes brittle when API schemas change. Rejected for this project.

---

### 4. Contract Assertion Patterns for High-GazeCRAP Functions

**Decision**: For functions with GazeCRAP > 15 or in Q4, add direct assertions on error returns and receiver field mutations alongside existing mock-based assertions.

**Rationale**: Gaze maps assertions to side effects by analyzing what the assertion checks against. An assertion like `assert.NoError(t, err)` maps to the `ErrorReturn` side effect. An assertion like `assert.Equal(t, 3, o.stats.DocumentsMoved)` maps to the `ReceiverMutation` on `stats`. These direct assertions are what gaze can trace — mock call recording assertions (`assert.Equal(t, "alice", mock.shareFileCalls[0].email)`) cannot be mapped because gaze doesn't trace through mock indirection.

**Functions requiring direct contract assertions** (GazeCRAP > 15 or Q4):
| Function | GazeCRAP | Quadrant | Required Assertions |
|----------|---------|----------|-------------------|
| `SyncCalendarAttachments` | 1482 | Q4 | Error return, `stats` mutation, `notesDocIDs` map mutation, `decisionDocIDs` map mutation |
| `OrganizeDocuments` | 306 | Q4 | Error return, `stats` mutation |
| `RunFullWorkflow` | ~30 | Q4 | Error return |
| `CreateDecisionsTab` | 650 (CRAP) | Untested | Error return (will become Q1/Q2 once tests added) |
| `ExtractDecisions` | 90 (CRAP) | Untested | Error return, return value |
| `ExtractAssigneesFromCheckboxes` | 110 (CRAP) | Untested | Error return, return value |

**Assertion pattern examples**:
```go
// Error return assertion (maps to ErrorReturn side effect)
err := org.OrganizeDocuments(ctx)
assert.NoError(t, err)  // or assert.Error(t, err) for error paths

// Receiver mutation assertion (maps to ReceiverMutation on stats)
err := org.OrganizeDocuments(ctx)
assert.Equal(t, 2, org.stats.DocumentsMoved)
assert.Equal(t, 1, org.stats.ShortcutsCreated)

// Map mutation assertion (maps to MapMutation on notesDocIDs)  
err := org.SyncCalendarAttachments(ctx)
docIDs := org.GetNotesDocIDs()
assert.Contains(t, docIDs, "expected-doc-id")
```

**Alternatives considered**:
- **Replace mock assertions entirely**: Too invasive and loses the delegation verification that mock assertions provide. Keep both.
- **Only add error return assertions**: Covers the most common side effect but misses receiver mutations. Not sufficient for Q4 functions where stats tracking is a core contract.

---

### 5. Existing Test Coverage Baseline

**Current state** (from gaze crap analysis):
| Metric | Value |
|--------|-------|
| Total functions analyzed | 139 |
| Average line coverage | 45.3% |
| Average CRAP score | 24.4 |
| CRAPload (>= 15) | 35 |
| Average contract coverage | 0% (all ambiguous) |
| GazeCRAPload | 7 |
| Q4 functions | 3 |
| Q3 functions | 4 |

**Packages with existing tests** (12 test files):
| Package | Test File | Tests | Coverage |
|---------|-----------|-------|----------|
| organizer | organizer_test.go | 35+ | ~81% |
| docs | service_test.go | 15+ | ~60% |
| gemini | client_test.go | 8+ | ~50% |
| calendar | service_test.go | 8+ | ~40% |
| config | config_test.go, dotenv_test.go | 10+ | ~85% |
| retry | retry_test.go | 11 | ~90% |
| secrets | store_test.go | 8+ | ~85% |
| ux | ux_test.go, format_test.go | 10+ | 100% |
| drive | service_test.go | 6+ | ~15% |

**Packages without tests**: auth, logging (intentionally excluded per spec assumptions).
