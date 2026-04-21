# Quickstart: Local AI via Ollama with IBM Granite

**Feature**: 014-local-ai-ollama
**Date**: 2026-04-20
**Purpose**: Integration guide for implementers

---

## 1. Prerequisites

Before implementing, ensure you understand:

1. **Dewey's Ollama pattern** — Read `llm/llm.go` and `llm/llm_test.go` in the Dewey repo (`unbound-force/dewey`). This is the reference implementation for raw HTTP to Ollama's REST API. Our `internal/ollama/client.go` follows this pattern exactly.

2. **Existing Gemini integration** — Read `internal/gemini/client.go`. The prompts for assignee extraction (`ExtractAssigneesFromCheckboxes`) and decision extraction (`ExtractDecisions`) will be reused with minimal modification for the local models.

3. **Pipeline orchestration** — Read `internal/organizer/organizer.go`, specifically `ExtractDecisionsForDoc()` and `RunFullWorkflow()`. The sensitivity gate inserts before decision extraction. The AI service swap (Gemini → Ollama) happens at the caller level.

4. **Doctor/init pattern** — Read `cmd/gcal-organizer/selfservice.go`. The 10 existing doctor checks and the init command's interactive flow are the templates for the 4 new Ollama checks and the model pull prompt.

5. **Config pattern** — Read `internal/config/config.go`. The `DecisionsConfig` nested struct is the template for `OllamaConfig`.

---

## 2. Implementation Order

### Phase 1: Foundation (no pipeline changes)

1. **`pkg/models/models.go`** — Add `SensitivityResult` struct
2. **`internal/config/config.go`** — Add `OllamaConfig` struct and defaults
3. **`internal/ollama/client.go`** — HTTP client (Generate, HealthCheck, ModelAvailable, ListModels)
4. **`internal/ollama/client_test.go`** — Full test suite with httptest

### Phase 2: Sensitivity Gate

5. **`internal/ollama/guardian.go`** — SensitivityClassifier implementation
6. **`internal/ollama/guardian_test.go`** — Classification tests (sensitive, not sensitive, malformed response, threshold boundary)
7. **`internal/organizer/organizer.go`** — Wire sensitivity gate into pipeline

### Phase 3: Local Task Assignment

8. **`internal/ollama/assigner.go`** — TaskAssigner implementation (reuse Gemini prompt)
9. **`internal/ollama/assigner_test.go`** — Assignment tests (single assignee, group, ambiguous)
10. Wire into caller (replace Gemini for task assignment when Ollama enabled)

### Phase 4: Local-Only Mode

11. **`internal/ollama/decisions.go`** — DecisionExtractor implementation (reuse Gemini prompt)
12. **`internal/ollama/decisions_test.go`** — Decision extraction tests
13. Wire local-only mode: swap Gemini for Ollama when `local_only=true`

### Phase 5: Doctor/Init + Documentation

14. **`cmd/gcal-organizer/selfservice.go`** — Add 4 doctor checks + init model pull
15. **Documentation** — Update README.md, SETUP.md, config.yaml template

---

## 3. Key Integration Points

### 3.1 OllamaClient (client.go)

Follow Dewey's `OllamaSynthesizer` pattern exactly:

```go
// NewClient creates an Ollama HTTP client.
func NewClient(endpoint string, timeoutSeconds int) *Client {
    return &Client{
        baseURL: endpoint,
        client: &http.Client{
            Timeout: time.Duration(timeoutSeconds) * time.Second,
        },
        modelCache:    make(map[string]bool),
        checkInterval: 30 * time.Second,
    }
}

// Generate sends POST /api/generate with stream=false.
func (c *Client) Generate(ctx context.Context, model, prompt string) (string, error) {
    reqBody := generateRequest{Model: model, Prompt: prompt, Stream: false}
    body, err := json.Marshal(reqBody)
    // ... http.NewRequestWithContext, c.client.Do, io.LimitReader ...
}
```

### 3.2 Sensitivity Gate Integration

The sensitivity gate inserts at the beginning of `ExtractDecisionsForDoc()`:

```go
func (o *Organizer) ExtractDecisionsForDoc(ctx context.Context, docCtx models.DecisionDocContext, ...) error {
    // NEW: Sensitivity gate (before any other processing)
    if o.sensitivityClassifier != nil {
        result, err := o.sensitivityClassifier.Classify(ctx, transcript.FullText)
        if err != nil {
            return fmt.Errorf("sensitivity classification failed: %w", err)
        }

        // Log classification result (FR-004)
        o.logger.Info("Sensitivity classification",
            "category", result.Category,
            "score", result.Score,
            "doc", docURL,
        )
        if o.config.Verbose {
            o.logger.Debug("Sensitivity reasoning", "reasoning", result.Reasoning)
        }

        if result.Score >= o.config.Ollama.Sensitivity.Threshold {
            if !o.config.DryRun {
                o.logger.Info("Skipping sensitive transcript",
                    "category", result.Category,
                    "score", result.Score,
                    "doc", docURL,
                )
                o.stats.SensitivitySkipped++
                return nil
            }
            // Dry-run: log but proceed (FR-007)
            o.logger.Warn("Would skip sensitive transcript (dry-run)",
                "category", result.Category,
                "score", result.Score,
                "doc", docURL,
            )
        }
        o.stats.SensitivityProcessed++
    }

    // ... existing decision extraction logic ...
}
```

### 3.3 Config Loading

Add to `DefaultConfig()`:

```go
func DefaultConfig() *Config {
    // ... existing defaults ...
    return &Config{
        // ... existing fields ...
        Ollama: OllamaConfig{
            Enabled:  true,
            Endpoint: "http://localhost:11434",
            Timeout:  120,
            Sensitivity: SensitivityConfig{
                Model:     "granite-guardian",
                Threshold: 0.7,
            },
            Assignments: AssignmentsConfig{
                Model: "granite3.2:8b",
            },
            LocalOnly: false,
        },
    }
}
```

Add viper bindings in `Load()`:

```go
mustBindEnv("ollama.enabled", "GCAL_OLLAMA_ENABLED")
mustBindEnv("ollama.endpoint", "GCAL_OLLAMA_ENDPOINT")
mustBindEnv("ollama.timeout", "GCAL_OLLAMA_TIMEOUT")
mustBindEnv("ollama.sensitivity.model", "GCAL_OLLAMA_SENSITIVITY_MODEL")
mustBindEnv("ollama.sensitivity.threshold", "GCAL_OLLAMA_SENSITIVITY_THRESHOLD")
mustBindEnv("ollama.assignments.model", "GCAL_OLLAMA_ASSIGNMENTS_MODEL")
mustBindEnv("ollama.local_only", "GCAL_OLLAMA_LOCAL_ONLY")
```

### 3.4 Doctor Checks

Add after check #10 (Chrome debugging port), conditional on `ollama.enabled`:

```go
// 11-14: Ollama checks (only when enabled)
if cfg.Ollama.Enabled {
    // 11. Ollama binary
    if _, err := exec.LookPath("ollama"); err == nil {
        fmt.Println(styledPass("Ollama binary installed"))
        passed++
    } else {
        fmt.Println(styledFail("Ollama binary not found"))
        fmt.Println(styledFix("Install: brew install ollama"))
        failed++
    }

    // 12. Ollama service running
    ollamaClient := ollama.NewClient(cfg.Ollama.Endpoint, 5)
    if ollamaClient.HealthCheck() {
        fmt.Println(styledPass("Ollama service running"))
        passed++

        // 13. Sensitivity model
        if ollamaClient.ModelAvailable(cfg.Ollama.Sensitivity.Model) {
            fmt.Println(styledPass(fmt.Sprintf("Model %s available", cfg.Ollama.Sensitivity.Model)))
            passed++
        } else {
            fmt.Println(styledFail(fmt.Sprintf("Model %s not found", cfg.Ollama.Sensitivity.Model)))
            fmt.Println(styledFix(fmt.Sprintf("Run: ollama pull %s", cfg.Ollama.Sensitivity.Model)))
            failed++
        }

        // 14. Assignment model
        if ollamaClient.ModelAvailable(cfg.Ollama.Assignments.Model) {
            fmt.Println(styledPass(fmt.Sprintf("Model %s available", cfg.Ollama.Assignments.Model)))
            passed++
        } else {
            fmt.Println(styledFail(fmt.Sprintf("Model %s not found", cfg.Ollama.Assignments.Model)))
            fmt.Println(styledFix(fmt.Sprintf("Run: ollama pull %s", cfg.Ollama.Assignments.Model)))
            failed++
        }
    } else {
        fmt.Println(styledFail("Ollama service not running"))
        fmt.Println(styledFix("Run: ollama serve"))
        failed++
        // Skip model checks when service is down
        fmt.Println(styledWarn("Model checks skipped (Ollama not running)"))
        warned += 2
    }
}
```

### 3.5 Init Model Pull

Add after credentials check in `initCmd`:

```go
// Ollama model pull (when service is running)
if cfg.Ollama.Enabled {
    ollamaClient := ollama.NewClient(cfg.Ollama.Endpoint, 5)
    if ollamaClient.HealthCheck() {
        needsPull := false
        var modelsToPull []string
        if !ollamaClient.ModelAvailable(cfg.Ollama.Sensitivity.Model) {
            modelsToPull = append(modelsToPull, cfg.Ollama.Sensitivity.Model)
            needsPull = true
        }
        if !ollamaClient.ModelAvailable(cfg.Ollama.Assignments.Model) {
            modelsToPull = append(modelsToPull, cfg.Ollama.Assignments.Model)
            needsPull = true
        }

        if needsPull && !nonInteractive {
            var pullModels bool
            form := huh.NewForm(
                huh.NewGroup(
                    huh.NewConfirm().
                        Title("Local AI models are needed for transcript screening. Pull them now?").
                        Value(&pullModels),
                ),
            )
            if err := form.Run(); err == nil && pullModels {
                for _, model := range modelsToPull {
                    fmt.Printf("  Pulling %s...\n", model)
                    cmd := exec.Command("ollama", "pull", model)
                    cmd.Stdout = os.Stdout
                    cmd.Stderr = os.Stderr
                    if err := cmd.Run(); err != nil {
                        fmt.Println(styledWarn(fmt.Sprintf("Failed to pull %s: %v", model, err)))
                    } else {
                        fmt.Println(styledPass(fmt.Sprintf("Pulled %s", model)))
                    }
                }
            }
        }
    }
}
```

---

## 4. Testing Strategy

### 4.1 httptest Pattern (from Dewey)

All Ollama HTTP interactions are tested with `httptest.NewServer`:

```go
func TestClient_Generate_Success(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/api/generate" {
            http.NotFound(w, r)
            return
        }
        var req generateRequest
        json.NewDecoder(r.Body).Decode(&req)
        resp := generateResponse{Response: `{"sensitive":false,"category":"none","score":0.1,"reasoning":"routine meeting"}`, Done: true}
        json.NewEncoder(w).Encode(resp)
    }))
    defer srv.Close()

    client := NewClient(srv.URL, 5)
    result, err := client.Generate(context.Background(), "granite-guardian", "classify this")
    // assert...
}
```

### 4.2 Test Coverage Requirements

| Component | Key Test Cases |
|-----------|---------------|
| `client.go` | Generate success, Generate error (500), Generate malformed JSON, Generate context cancellation, Generate unreachable server, HealthCheck success, HealthCheck unreachable, ModelAvailable present, ModelAvailable missing, ModelAvailable caching |
| `guardian.go` | Classify sensitive (hr, legal, financial, health, termination), Classify not sensitive, Threshold boundary (0.70 = sensitive, 0.69 = not), Malformed response + retry, Truncation warning |
| `assigner.go` | Single assignee extracted, Group assignee returns null, Ambiguous returns null, Multiple items batch, Empty items list |
| `decisions.go` | Extract made/deferred/open decisions, Empty transcript, Malformed response + retry |
| Config | OllamaConfig defaults, Viper binding, Env var override |
| Doctor | All 4 checks pass, Binary missing, Service down, Model missing, Ollama disabled (checks skipped) |

---

## 5. Common Pitfalls

1. **Don't import `github.com/ollama/ollama`** — FR-012 requires raw HTTP only. No SDK dependency.

2. **Don't retry network errors** — Unlike Gemini (which retries transient errors), Ollama network errors indicate a real problem. Hard-stop immediately per FR-009.

3. **Don't log reasoning at INFO** — FR-004 requires reasoning at DEBUG only. The reasoning may contain excerpts from the sensitive transcript.

4. **Don't fall back to Gemini** — FR-009 is explicit: when Ollama is configured but unavailable, hard-stop. Never silently fall back to cloud AI.

5. **Don't forget `stream: false`** — Without this, Ollama streams newline-delimited JSON objects instead of a single response. All our requests must set `stream: false`.

6. **Don't skip the response body cap** — Always use `io.LimitReader(resp.Body, 50*1024*1024)` to prevent unbounded memory allocation.

7. **Threshold comparison is `>=`, not `>`** — Per spec acceptance scenario 6: a score of exactly 0.70 with threshold 0.70 IS sensitive.

8. **Don't modify quality gates** — Per AGENTS.md gatekeeping rules, never modify coverage thresholds, CRAP scores, or CI flags to make the implementation pass.
