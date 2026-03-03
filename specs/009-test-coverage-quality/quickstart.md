# Quickstart: 009 Test Coverage & Contract Quality

**Branch**: `009-test-coverage-quality`
**Date**: 2026-03-02 (updated: 2026-03-02 post-clarification)

## Prerequisites

- Go 1.24+ installed (`go version`)
- gaze v1.2.3+ installed (`gaze --version`)
- Project builds cleanly (`go build ./...`)
- Feature branch `008-decision-extraction` merged to main (provides baseline test infrastructure)

## Implementation Order

Work in this order to ensure each phase builds on the previous and tests pass incrementally:

### Phase 0: Gaze Configuration (prerequisite — unlocks contract coverage)

```bash
# 0. Create .gaze.yaml at repository root (FR-017)
#    Without this, contract coverage is 0% across the entire module.
cat > .gaze.yaml << 'EOF'
classification:
  thresholds:
    contractual: 65
    incidental: 40
EOF

# Verify it takes effect
gaze quality ./internal/retry/ --format=text
# Should now show non-zero contract coverage for DefaultConfig
```

### Phase 1: Organizer Package (P1 — highest CRAP impact)

```bash
# 1. Add tests for printSummary, logActionResult, logCalendarAction, RunFullWorkflow
#    Target: organizer package coverage from 58% → >85%
#    Edit: internal/organizer/organizer_test.go

go test ./internal/organizer/ -v -cover
```

Key tests to add:
- `TestPrintSummary_DryRun` / `TestPrintSummary_RealRun` — exercise all conditional branches
- `TestLogActionResult` — table-driven with all ActionResult states
- `TestLogCalendarAction` — table-driven with all states
- `TestRunFullWorkflow` — verify sequential execution and error propagation
- Add contract assertions to existing SyncCalendarAttachments and OrganizeDocuments tests

### Phase 2: Docs Package (P1 — highest CRAP score)

```bash
# 2. Add httptest tests for CreateDecisionsTab and ExtractCheckboxItems
#    Target: docs package coverage from 41.5% → >80%
#    Edit: internal/docs/service_test.go

go test ./internal/docs/ -v -cover
```

Key tests to add:
- `TestCreateDecisionsTab_*` — 8 tests using httptest (happy, duplicate, cleanup, failure)
- `TestExtractCheckboxItems_*` — 4 tests using httptest (Notes tab, section boundary, processed items)

### Phase 3: Gemini Package (P1)

```bash
# 3. Add httptest tests for ExtractDecisions and ExtractAssigneesFromCheckboxes
#    Target: gemini package coverage from 49% → >85%
#    Edit: internal/gemini/client_test.go

go test ./internal/gemini/ -v -cover
```

Key tests to add:
- `TestExtractDecisions_*` — 4 tests (valid, empty input, no candidates, API error)
- `TestExtractAssigneesFromCheckboxes_*` — 4 tests (valid, null assignees, empty, error)

### Phase 4: Calendar Package (P2)

```bash
# 4. Add parseEvent tests and optionally ListRecentEvents
#    Target: calendar package coverage from 9.2% → >80%
#    Edit: internal/calendar/service_test.go

go test ./internal/calendar/ -v -cover
```

Key tests to add:
- `TestParseEvent` — table-driven with ~8 cases (DateTime, Date, attachments, attendees, Drive links)
- `TestListRecentEvents` — 3 httptest tests (single page, multi-page, error)

### Phase 5: Config, Secrets, UX, Drive (P2)

```bash
# 5. Close remaining gaps
go test ./internal/config/ ./internal/secrets/ ./internal/ux/ ./internal/drive/ -v -cover
```

- `internal/config/config_test.go` — add LoadSecrets test
- `internal/secrets/store_test.go` — add Backend.String, NewStore edge cases
- `internal/ux/ux_test.go` — NEW: table-driven test for all 8 error constructors
- `internal/drive/service_test.go` — add escapeQuery test

### Phase 6: Extract and Test cmd/ Functions (P3)

```bash
# 6. Extract functions, update callers, add tests
# Create: internal/config/dotenv.go, internal/ux/format.go
# Update: cmd/gcal-organizer/main.go, cmd/gcal-organizer/auth_config.go

go build ./...   # Verify extraction doesn't break compilation
go test ./...     # Verify all tests pass
```

- `internal/config/dotenv.go` — extracted `LoadDotEnv` + `validEnvKey`
- `internal/config/dotenv_test.go` — ~10 table-driven tests
- `internal/ux/format.go` — extracted `MaskSecret`, `TruncateText`
- `internal/ux/format_test.go` — table-driven tests
- `cmd/gcal-organizer/assign_tasks_test.go` — `extractUnassignedItems` tests

### Phase 1.5: Test Helper Renaming (P2 — reduces gaze warnings)

```bash
# Rename helpers to reduce multi-target warnings (FR-018)
# newTestOrganizer → setupOrganizer (organizer_test.go)
# mockService → setupDriveService (drive/service_test.go)
# buildTestDoc → makeTestDoc (docs/service_test.go)
# buildTab → makeTab (docs/service_test.go)
# buildParagraphElement → makeParagraphElement (docs/service_test.go)

go test ./... -count=1   # Verify all tests still pass after rename
```

## Verification

After all phases:

```bash
# Full test suite
go test ./... -count=1

# Race detector
go test -race ./...

# Coverage report
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out | tail -1  # Should show >55%

# Quality gates
go vet ./...
gofmt -l .
go mod tidy && git diff --exit-code go.mod go.sum

# Gaze quality verification (SC-015, SC-016)
gaze crap ./...                    # CRAPload < 15, no Q4 functions
gaze quality ./internal/organizer/ # Contract coverage > 50%
gaze quality ./internal/retry/     # Contract coverage > 50%

# CI
make ci
```

## Common Patterns

### Contract Assertion Pattern

Every test must assert at least one behavioral output:

```go
// BAD: line coverage only
func TestFoo(t *testing.T) {
    org.RunFullWorkflow(ctx)
    // no assertions — just exercises code
}

// GOOD: contract assertion
func TestFoo(t *testing.T) {
    err := org.RunFullWorkflow(ctx)
    if err != nil { t.Fatal(err) }
    if org.stats.DocumentsFound != 2 {
        t.Errorf("DocumentsFound: got %d, want 2", org.stats.DocumentsFound)
    }
}
```

### httptest Helper Pattern

```go
func newTestService(t *testing.T, handler http.HandlerFunc) (*Service, *httptest.Server) {
    t.Helper()
    ts := httptest.NewServer(handler)
    t.Cleanup(ts.Close)
    srv, err := docs.NewService(context.Background(),
        option.WithHTTPClient(ts.Client()),
        option.WithEndpoint(ts.URL),
        option.WithoutAuthentication(),
    )
    if err != nil { t.Fatalf("failed to create service: %v", err) }
    return &Service{client: srv}, ts
}
```
