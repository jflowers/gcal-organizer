# Tasks: Local AI via Ollama with IBM Granite

**Feature**: 014-local-ai-ollama
**Generated**: 2026-04-20
**Spec**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

---

## Phase 1: Foundation — Types, Config, and HTTP Client

> **Goal**: Establish the data types, configuration schema, and low-level Ollama HTTP client. No pipeline changes — all new code is leaf-level with no callers yet.
>
> **Checkpoint**: `go build ./...` compiles, `go test -race -count=1 ./internal/ollama/... ./internal/config/... ./pkg/models/...` passes.

- [x] [T01] [US1] Add `SensitivityResult` struct to `pkg/models/models.go`
  - Fields: `Sensitive bool`, `Category string`, `Score float64`, `Reasoning string` (all with JSON tags)
  - GoDoc comment referencing FR-002 (four outputs) and FR-004 (reasoning at DEBUG only)

- [x] [T02] [US1] Add `OllamaConfig`, `SensitivityConfig`, `AssignmentsConfig` structs to `internal/config/config.go`
  - `OllamaConfig`: Enabled, Endpoint, Timeout, Sensitivity, Assignments, LocalOnly (all with `mapstructure` tags)
  - `SensitivityConfig`: Model, Threshold
  - `AssignmentsConfig`: Model
  - Add `Ollama OllamaConfig` field to existing `Config` struct

- [x] [T03] [US1] Set `OllamaConfig` defaults in `DefaultConfig()` and add viper bindings in `Load()` — `internal/config/config.go`
  - Defaults per FR-021: enabled=true, endpoint=http://localhost:11434, timeout=120, sensitivity.model=granite-guardian, threshold=0.7, assignments.model=granite3.2:8b, local_only=false
  - Viper bindings: `ollama.enabled` → `GCAL_OLLAMA_ENABLED`, etc. (7 bindings per research.md §6)
  - Add viper value override logic in `Load()` matching existing pattern

- [x] [T04] [US1] Create `internal/ollama/client.go` — Ollama HTTP client
  - `Client` struct: baseURL, `*http.Client`, model cache (`sync.RWMutex` + `map[string]bool` + `time.Time` + `time.Duration`)
  - `NewClient(endpoint string, timeoutSeconds int) *Client`
  - `Generate(ctx context.Context, model, prompt string) (string, error)` — POST `/api/generate` with `stream:false`, `io.LimitReader` (50 MB cap), context propagation
  - `HealthCheck() bool` — GET `/api/tags` with 2-second timeout, returns true on HTTP 200
  - `ModelAvailable(model string) bool` — GET `/api/tags`, cache results for 30 seconds
  - `ListModels() ([]string, error)` — GET `/api/tags`, return model names
  - Sentinel errors: `ErrOllamaUnavailable`, `ErrModelNotAvailable`, `ErrMalformedResponse`
  - Internal request/response types: `generateRequest`, `generateResponse`, `tagsResponse`, `modelInfo`
  - FR-012: stdlib only (`net/http`, `encoding/json`, `io`, `sync`, `time`, `context`, `errors`, `fmt`)

- [x] [T05] [US1] Create `internal/ollama/client_test.go` — Client test suite
  - All tests use `httptest.NewServer` (no real Ollama dependency)
  - Test cases: Generate success, Generate HTTP 500, Generate malformed JSON response, Generate context cancellation, Generate unreachable server (invalid URL), HealthCheck success, HealthCheck unreachable, ModelAvailable present (with `:latest` suffix matching), ModelAvailable missing, ModelAvailable cache hit (verify single HTTP call for repeated checks), ListModels success, ListModels error
  - All tests run under `-race` flag

---

## Phase 2: Sensitivity Gate — Classifier and Pipeline Integration

> **Goal**: Implement the sensitivity classifier (granite-guardian) and wire it into the organizer pipeline as a pre-processing gate. This is the core safety feature (US1 P1).
>
> **Checkpoint**: `go test -race -count=1 ./internal/ollama/... ./internal/organizer/...` passes. Sensitivity gate correctly skips sensitive transcripts and respects dry-run.

- [x] [T06] [US1] Create `internal/ollama/guardian.go` — SensitivityClassifier implementation
  - `SensitivityClassifier` interface: `Classify(ctx context.Context, transcript string) (*models.SensitivityResult, error)`
  - `Guardian` struct implementing `SensitivityClassifier`, holds `*Client` + model name
  - `NewGuardian(client *Client, model string) *Guardian`
  - Prompt template from research.md §3 (workplace sensitivity classifier with 6 categories)
  - JSON response parsing: strip markdown fences, unmarshal, validate category (default "none"), clamp score to [0.0, 1.0]
  - Retry strategy: retry once on malformed response (total 2 attempts), hard-stop on network errors (FR-009)
  - Transcript truncation: check byte length against configurable max (default ~24000 chars), keep first 60% + separator + last 40%, log warning

- [x] [T07] [US1] Create `internal/ollama/guardian_test.go` — Sensitivity classifier tests
  - Test cases: Classify sensitive (category=hr, score=0.92), Classify not sensitive (category=none, score=0.1), Threshold boundary (score=0.70 at threshold=0.70 → sensitive per FR-003), Threshold boundary (score=0.69 at threshold=0.70 → not sensitive), Malformed response + successful retry, Malformed response + failed retry → ErrMalformedResponse, Network error → immediate hard-stop (no retry), Transcript truncation (verify truncated text preserves beginning + end), All 6 categories (hr, legal, financial, health, termination, none)
  - All tests use `httptest.NewServer`

- [x] [T08] [US1] Add sensitivity gate to organizer pipeline — `internal/organizer/organizer.go`
  - Add `sensitivityClassifier` field to `Organizer` struct (type: interface from ollama package)
  - Add `SensitivitySkipped int` and `SensitivityProcessed int` to `Stats` struct
  - Add `SetSensitivityClassifier(classifier)` method or constructor option
  - Add `ClassifyTranscript(ctx, docCtx, docsSvc) (*SensitivityResult, error)` method to Organizer that extracts the transcript and runs the classifier
  - Insert sensitivity gate in the main.go orchestration loop (Step 4 decision extraction loop), BEFORE calling `ExtractDecisionsForDoc()` — same level as the allowlist filter. This keeps ExtractDecisionsForDoc focused on extraction, not gating.
  - Gate logic: if classifier is non-nil and sensitivity.enabled, call ClassifyTranscript; if score >= threshold, skip (continue loop) unless dry-run
  - Logging per FR-004: category + score + doc URL at INFO; reasoning at DEBUG only
  - Dry-run per FR-007: log classification result but proceed with processing
  - Add integration test: inject mock classifier into organizer, verify ExtractDecisionsForDoc is NOT called when classifier returns sensitive result
  - Update `printSummary()` to include sensitivity stats when non-zero

- [x] [T09] [US1] Wire OllamaClient and sensitivity gate in `cmd/gcal-organizer/main.go`
  - In `runCmd.RunE`: after `loadConfigAndStore()`, if `cfg.Ollama.Enabled`:
    - Create `ollama.NewClient(cfg.Ollama.Endpoint, cfg.Ollama.Timeout)`
    - Validate availability: `client.HealthCheck()` → hard-stop with actionable error if false (FR-008)
    - Validate sensitivity model: `client.ModelAvailable(cfg.Ollama.Sensitivity.Model)` → hard-stop if false (FR-010)
    - Create `ollama.NewGuardian(client, cfg.Ollama.Sensitivity.Model)`
    - Set classifier on organizer via `SetSensitivityClassifier()`
  - Hard-stop error messages must include fix steps: "Install: brew install ollama", "Start: ollama serve", "Pull: ollama pull <model>"

---

## Phase 3: Local Task Assignment

> **Goal**: Replace Gemini for task assignment with local granite3.2:8b model. Same structured output format as existing cloud extraction (FR-014).
>
> **Checkpoint**: `go test -race -count=1 ./internal/ollama/...` passes. Assignment output format matches `gemini.CheckboxAssignment`.

- [x] [T10] [US2] Create `internal/ollama/assigner.go` — TaskAssigner implementation
  - `TaskAssigner` interface: `ExtractAssignees(ctx context.Context, items []models.CheckboxItem) ([]models.CheckboxAssignment, error)` (types moved to pkg/models per review finding A-1)
  - `Assigner` struct implementing `TaskAssigner`, holds `*Client` + model name
  - `NewAssigner(client *Client, model string) *Assigner`
  - Reuse the existing Gemini prompt from `internal/gemini/client.go:ExtractAssigneesFromCheckboxes()` (model-agnostic prompt)
  - JSON response parsing: same schema as Gemini (`[{"index":0,"assignee":"Jay"},{"index":1,"assignee":null}]`)
  - Retry once on malformed response, hard-stop on network errors
  - FR-015: same assignment rules — single named individual only, null for groups/shared/vague

- [x] [T11] [US2] Create `internal/ollama/assigner_test.go` — Task assigner tests
  - Test cases: Single assignee extracted ("Jay will schedule the follow-up" → assignee="Jay"), Group assignee returns null ("The team will discuss" → assignee=null), Ambiguous reference returns null ("Someone should check" → assignee=null), Multiple items batch (mixed assignees and nulls), Empty items list → nil result, Malformed response + retry, Network error → hard-stop
  - All tests use `httptest.NewServer`

- [x] [T12] [US2] Wire local task assigner in `cmd/gcal-organizer/main.go`
  - In Step 3 (Assign Tasks): when `cfg.Ollama.Enabled`, validate assignment model availability (`client.ModelAvailable(cfg.Ollama.Assignments.Model)`)
  - Create `ollama.NewAssigner(client, cfg.Ollama.Assignments.Model)`
  - Pass local assigner to `runAssignTasksForDoc()` instead of Gemini client for assignee extraction (FR-013)
  - Requires understanding how `assign_tasks.go` currently uses Gemini — may need interface extraction

---

## Phase 4: Local-Only Mode — Decision Extraction

> **Goal**: Add local decision extraction using granite3.2:8b for users who want zero cloud AI calls. Completes the local-only mode (US3).
>
> **Checkpoint**: `go test -race -count=1 ./internal/ollama/...` passes. Decision output matches `[]models.Decision` format.

- [x] [T13] [US3] Create `internal/ollama/decisions.go` — DecisionExtractor implementation
  - `DecisionExtractor` interface: `ExtractDecisions(ctx context.Context, transcriptText string) ([]models.Decision, error)`
  - `DecisionExtractor` struct (concrete type, different name to avoid collision — e.g., `LocalDecisionExtractor`), holds `*Client` + model name
  - `NewDecisionExtractor(client *Client, model string) *LocalDecisionExtractor`
  - Reuse the existing Gemini prompt from `internal/gemini/client.go:ExtractDecisions()` (model-agnostic prompt)
  - JSON response parsing: same schema as Gemini (`[]models.Decision` with category, text, timestamp, context)
  - FR-017: produce same three categories (made, deferred, open)
  - Retry once on malformed response, hard-stop on network errors

- [x] [T14] [US3] Create `internal/ollama/decisions_test.go` — Decision extractor tests
  - Test cases: Extract made/deferred/open decisions, Empty transcript → empty result, Malformed response + successful retry, Malformed response + failed retry → error, Network error → hard-stop, Verify all three categories present in output
  - All tests use `httptest.NewServer`

- [x] [T15] [US3] Wire local-only mode in `cmd/gcal-organizer/main.go`
  - In Step 4 (Extract Decisions): when `cfg.Ollama.Enabled && cfg.Ollama.LocalOnly`:
    - Create `ollama.NewDecisionExtractor(client, cfg.Ollama.Assignments.Model)` (reuses assignment model for decisions)
    - Pass local extractor to `org.ExtractDecisionsForDoc()` instead of Gemini client (FR-016, FR-017)
  - When `cfg.Ollama.LocalOnly && !cfg.Ollama.Enabled`: error — local-only requires Ollama enabled
  - FR-018: if local-only and Ollama unavailable → hard-stop (already handled by Phase 2 availability check)
  - FR-019: when local-only is false (default), decision extraction continues to use Gemini

- [x] [T16] [US3] Validate no cloud AI calls in local-only mode — `internal/config/config.go`
  - Add `Validate()` enhancement: when `Ollama.LocalOnly` is true, `GeminiAPIKey` is NOT required (skip Gemini validation)
  - FR-016: local-only mode prevents all cloud AI API calls — Gemini key becomes optional

---

## Phase 5: Doctor Checks and Init Integration

> **Goal**: Extend the doctor command with 4 Ollama health checks and the init command with model pull prompts. Makes the other stories usable in practice (US4).
>
> **Checkpoint**: `go build ./...` compiles. Doctor checks display correctly for all states (installed/not, running/not, models present/missing, disabled).

- [x] [T17] [US4] Add 4 Ollama doctor checks to `cmd/gcal-organizer/selfservice.go`
  - Check 11: Ollama binary installed — `exec.LookPath("ollama")`; fail fix: "Install: brew install ollama"
  - Check 12: Ollama service running — `ollama.NewClient(cfg.Ollama.Endpoint, 5).HealthCheck()`; fail fix: "Run: ollama serve"
  - Check 13: Sensitivity model pulled — `client.ModelAvailable(cfg.Ollama.Sensitivity.Model)`; fail fix: "Run: ollama pull <model>"
  - Check 14: Assignment model pulled — `client.ModelAvailable(cfg.Ollama.Assignments.Model)`; fail fix: "Run: ollama pull <model>"
  - FR-027: when `cfg.Ollama.Enabled` is false, skip all 4 checks entirely (not reported as failures)
  - Checks 13-14 only run if check 12 passes (service must be running to query models)
  - When service is down, model checks show as warned ("Model checks skipped — Ollama not running")
  - Requires loading config in doctorCmd — use `loadConfigAndStore()` or read viper directly

- [x] [T18] [US4] Add model pull prompt to init command — `cmd/gcal-organizer/selfservice.go`
  - After credentials check in `initCmd.RunE`: when `cfg.Ollama.Enabled`:
    - Create `ollama.NewClient(cfg.Ollama.Endpoint, 5)`
    - If service is running (`HealthCheck()`) but models are missing (`!ModelAvailable()`):
      - Prompt: "Local AI models are needed for transcript screening. Pull them now? (Y/n)" using `huh.NewConfirm()`
      - If confirmed: `exec.Command("ollama", "pull", model)` for each missing model, with stdout/stderr passthrough
      - Skip prompt in `--non-interactive` mode
  - FR-028: interactive prompt for model pull

- [x] [T19] [US4] Add Ollama section to `generateConfigYAML()` — `cmd/gcal-organizer/selfservice.go`
  - Append Ollama YAML block with all settings and comments (matching research.md §6 schema)
  - Include: enabled, endpoint, timeout, sensitivity.model, sensitivity.threshold, assignments.model, local_only
  - All values commented with defaults

---

## Phase 6: Documentation

> **Goal**: Update user-facing documentation to include Ollama as a prerequisite and document the new configuration options.
>
> **Checkpoint**: Documentation accurately reflects the new features. No broken links.

- [x] [T20] [US4] Update `docs/SETUP.md` — add Ollama as prerequisite
  - Add Ollama installation section: `brew install ollama` (macOS), package manager (Linux)
  - Add model pull instructions: `ollama pull granite-guardian`, `ollama pull granite3.2:8b`
  - Add configuration section: YAML config for `ollama:` block
  - FR-029: setup documentation lists local AI service as prerequisite

- [x] [T21] [US4] Update `README.md` — document local AI features
  - Add "Local AI" section describing sensitivity gate, local task assignment, local-only mode
  - Add configuration reference for `ollama:` YAML block
  - Add troubleshooting: "Ollama not running" → `ollama serve`, "Model not found" → `ollama pull <model>`

---

## Summary

| Metric | Count |
|--------|-------|
| **Total tasks** | 21 |
| **US1 (Sensitivity Gate)** | 9 (T01–T09) |
| **US2 (Local Task Assignment)** | 3 (T10–T12) |
| **US3 (Local-Only Mode)** | 4 (T13–T16) |
| **US4 (Doctor/Init/Docs)** | 5 (T17–T21) |
| **Phases** | 6 |
| **New files** | 8 (client.go, client_test.go, guardian.go, guardian_test.go, assigner.go, assigner_test.go, decisions.go, decisions_test.go) |
| **Modified files** | 5 (models.go, config.go, organizer.go, main.go, selfservice.go) + 2 docs (SETUP.md, README.md) |

<!-- spec-review: passed -->
<!-- code-review: passed -->
