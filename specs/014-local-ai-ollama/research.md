# Research: Local AI via Ollama with IBM Granite

**Feature**: 014-local-ai-ollama
**Date**: 2026-04-20
**Purpose**: Document technical research informing implementation decisions

---

## 1. Ollama HTTP API Patterns (from Dewey Codebase)

### Reference Implementation

The Dewey project (`unbound-force/dewey`) provides a production-proven pattern for Ollama integration via raw HTTP. Key files:

- **`llm/llm.go`** — `OllamaSynthesizer` struct: raw HTTP client to Ollama's REST API
- **`llm/llm_test.go`** — Full test suite using `net/http/httptest`
- **`embed/embed.go`** — `OllamaEmbedder`: same pattern for embeddings (different endpoint)
- **`main.go`** — `ensureOllama()`: health check, auto-start, binary detection

### API Endpoints Used

| Endpoint | Method | Purpose | Request Body | Response |
|----------|--------|---------|-------------|----------|
| `/api/generate` | POST | Text generation | `{"model":"...", "prompt":"...", "stream":false}` | `{"response":"...", "done":true}` |
| `/api/tags` | GET | List available models | None | `{"models":[{"name":"model:tag"},...]}` |

### Key Design Patterns from Dewey

1. **Raw HTTP, no SDK** (FR-012): Uses `net/http` directly. No `github.com/ollama/ollama` dependency. This keeps the dependency tree clean and avoids version coupling.

2. **`stream: false`**: All requests use non-streaming mode. The response is a single JSON object with the complete generated text. This simplifies parsing and error handling.

3. **Response body cap**: `io.LimitReader(resp.Body, 50*1024*1024)` prevents unbounded memory allocation from unexpectedly large responses.

4. **Context propagation**: `http.NewRequestWithContext(ctx, ...)` ensures caller-imposed deadlines and cancellation are respected.

5. **Model availability caching**: `Available()` caches the result of `GET /api/tags` for 30 seconds using `sync.RWMutex` with double-checked locking. Avoids redundant HTTP calls during pipeline runs.

6. **Configurable HTTP timeout**: Client created with `&http.Client{Timeout: 120 * time.Second}`. Generation is slower than embedding, so the timeout is generous.

7. **Test pattern**: `httptest.NewServer` with handler that validates request path, method, and body. Tests cover: success, error status, malformed response, context cancellation, unreachable server, caching behavior.

8. **NoopSynthesizer**: Test double that implements the interface with configurable response/error/availability. Exported for cross-package testing.

### Health Check Pattern from Dewey

```go
// From main.go — ollamaHealthCheck
func ollamaHealthCheck(endpoint string) bool {
    client := &http.Client{Timeout: 2 * time.Second}
    resp, err := client.Get(endpoint + "/api/tags")
    if err != nil { return false }
    defer resp.Body.Close()
    return resp.StatusCode == http.StatusOK
}
```

### Hard Error Pattern from Dewey

Dewey's `openspec/changes/archive/2026-03-29-ollama-hard-error/` documents the deliberate shift from graceful degradation to hard error when the embedding model is unavailable. The error message includes:
- The model name
- The endpoint
- The fix command (`ollama pull <model>`)
- How to skip (`--no-embeddings`)

This directly informs our FR-008/FR-009 design: when Ollama is configured but unavailable, hard-stop with actionable error. No silent fallback to cloud AI.

---

## 2. IBM Granite Model Capabilities

### Granite Guardian (Sensitivity Classification)

- **Model**: `granite-guardian` (available via `ollama pull granite-guardian`)
- **Purpose**: Safety and risk classification. Designed to identify harmful, sensitive, or risky content.
- **Architecture**: Built on IBM Granite foundation, fine-tuned for safety classification tasks.
- **Capabilities**: Can classify content across safety dimensions including: harmful content, sensitive personal information, workplace-inappropriate content.
- **Prompt format**: Accepts natural language prompts describing the classification task. Returns structured assessment.
- **Context window**: Sufficient for typical meeting transcripts (8K+ tokens). Transcripts exceeding the window should be truncated with beginning and end preserved (per spec edge case).

**Sensitivity categories for our use case**:
- `hr` — Performance reviews, disciplinary actions, complaints, accommodation requests
- `legal` — Legal proceedings, compliance issues, regulatory matters, contracts under negotiation
- `financial` — Compensation discussions, budget allocations involving individual salaries, M&A discussions
- `health` — Medical information, health accommodations, wellness concerns
- `termination` — Layoff discussions, termination decisions, severance negotiations
- `none` — No sensitive content detected

### Granite 3.2:8b (Task Assignment & Decision Extraction)

- **Model**: `granite3.2:8b` (available via `ollama pull granite3.2:8b`)
- **Purpose**: General-purpose instruction-following model with strong structured output capabilities.
- **Parameters**: 8 billion — sufficient for structured extraction tasks (assignee identification, decision categorization).
- **Capabilities**: JSON output generation, instruction following, text analysis, entity extraction.
- **Context window**: 128K tokens — more than sufficient for meeting transcripts.
- **Quality tradeoff**: For structured extraction tasks (identify assignee from "Jay will schedule the follow-up"), 8B models perform comparably to cloud models. For nuanced reasoning (decision extraction with context), quality may be lower — this is an accepted tradeoff for local-only mode (per spec assumptions).

---

## 3. Prompt Engineering for Sensitivity Classification

### Classification Prompt Design

The sensitivity classifier prompt must:
1. Define the classification task clearly
2. Enumerate the sensitivity categories with examples
3. Request structured JSON output with all four fields (sensitive, category, score, reasoning)
4. Include few-shot examples for calibration

### Proposed Prompt Template

```
You are a workplace content sensitivity classifier. Analyze the following meeting transcript and determine if it contains sensitive content that should not be processed by cloud AI services or written to external files.

Classify the transcript into one of these categories:
- "hr": Performance reviews, disciplinary actions, employee complaints, accommodation requests, hiring/firing discussions about specific individuals
- "legal": Legal proceedings, compliance investigations, regulatory matters, contracts under negotiation, litigation strategy
- "financial": Individual compensation discussions, salary negotiations, M&A discussions, confidential budget allocations
- "health": Medical information, health accommodations, wellness concerns about specific individuals
- "termination": Layoff planning, termination decisions, severance negotiations, reduction-in-force discussions
- "none": No sensitive content detected

Return your analysis as a JSON object with these fields:
- "sensitive": boolean (true if the transcript contains sensitive content)
- "category": string (one of: hr, legal, financial, health, termination, none)
- "score": number between 0.0 and 1.0 (confidence in the classification)
- "reasoning": string (brief explanation of why the content is or is not sensitive)

Important rules:
1. Err on the side of caution — if uncertain, classify as sensitive with a moderate score
2. Focus on content about SPECIFIC INDIVIDUALS, not general policy discussions
3. A meeting about "updating the PTO policy" is NOT sensitive; a meeting about "Sarah's excessive absences" IS sensitive
4. Return ONLY the JSON object, no other text

Transcript:
%s
```

### JSON Parsing Strategy

The response parsing follows the same pattern as `gemini/client.go`:
1. Strip markdown code fences (`\`\`\`json` / `\`\`\``)
2. Trim whitespace
3. Attempt JSON unmarshal into `SensitivityResult` struct
4. Validate category is one of the known values (default to "none" if unknown)
5. Validate score is in [0.0, 1.0] range (clamp if out of range)

### Retry Strategy

Per spec edge case: "The system retries once. If the retry also fails, it stops the pipeline with an error describing the malformed response."

This differs from the Gemini retry strategy (which uses exponential backoff with 5 retries for transient errors). For Ollama:
- **Malformed output**: Retry once (total 2 attempts). If both fail, hard-stop.
- **Network/timeout errors**: Hard-stop immediately (FR-009 — no graceful degradation).
- **Rationale**: Ollama is local, so network errors indicate a real problem (service crashed, not running). Retrying network errors wastes time.

---

## 4. Task Assignment Prompt Reuse

The local task assignment prompt can reuse the existing Gemini prompt from `internal/gemini/client.go:ExtractAssigneesFromCheckboxes()` with minimal modification. The prompt is model-agnostic — it describes the task, provides examples, and requests JSON output.

**Key compatibility requirement**: The local model must produce the same JSON schema:
```json
[
  {"index": 0, "assignee": "Jay"},
  {"index": 1, "assignee": null}
]
```

The `parseAssignmentsResponse()` function in `gemini/client.go` can be extracted to a shared utility or duplicated in the ollama package. Given that the parsing logic is simple (JSON unmarshal + index mapping), duplication is acceptable to keep packages independent.

---

## 5. Decision Extraction Prompt Reuse

Similarly, the decision extraction prompt from `internal/gemini/client.go:ExtractDecisions()` can be reused for local-only mode. The prompt requests the same three categories (made, deferred, open) with the same fields (text, timestamp, context).

The `parseDecisionsResponse()` function follows the same pattern and can be reused or duplicated.

**Quality note**: Decision extraction requires more nuanced reasoning than assignee extraction. The 8B model may produce lower-quality results (less context, fewer implicit decisions detected). This is an accepted tradeoff per spec assumptions.

---

## 6. Configuration Schema Design

### YAML Structure

```yaml
# Local AI configuration (Ollama)
ollama:
  # Enable local AI features (default: true)
  enabled: true

  # Ollama API endpoint (default: http://localhost:11434)
  endpoint: "http://localhost:11434"

  # Request timeout in seconds (default: 120)
  timeout: 120

  # Sensitivity classification settings
  sensitivity:
    # Model for sensitivity classification (default: granite-guardian)
    model: "granite-guardian"
    # Sensitivity threshold — transcripts scoring >= this are skipped (default: 0.7)
    threshold: 0.7

  # Task assignment settings
  assignments:
    # Model for assignee extraction (default: granite3.2:8b)
    model: "granite3.2:8b"

  # Local-only mode — all AI processing runs locally, no cloud AI calls
  local_only: false
```

### Viper Binding

Following the existing pattern in `config.go`:
```go
mustBindEnv("ollama.enabled", "GCAL_OLLAMA_ENABLED")
mustBindEnv("ollama.endpoint", "GCAL_OLLAMA_ENDPOINT")
mustBindEnv("ollama.timeout", "GCAL_OLLAMA_TIMEOUT")
mustBindEnv("ollama.sensitivity.model", "GCAL_OLLAMA_SENSITIVITY_MODEL")
mustBindEnv("ollama.sensitivity.threshold", "GCAL_OLLAMA_SENSITIVITY_THRESHOLD")
mustBindEnv("ollama.assignments.model", "GCAL_OLLAMA_ASSIGNMENTS_MODEL")
mustBindEnv("ollama.local_only", "GCAL_OLLAMA_LOCAL_ONLY")
```

---

## 7. Pipeline Integration Points

### Current Pipeline Flow (organizer.go)

```
RunFullWorkflow:
  1. OrganizeDocuments (Drive operations)
  2. SyncCalendarAttachments (Calendar → Drive shortcuts)
  [caller handles:]
  3. Assign Tasks (browser automation)
  4. Extract Decisions (Gemini → Docs tab + markdown export)
```

### Modified Pipeline Flow

```
RunFullWorkflow:
  0. Validate Ollama availability (if configured) — HARD STOP on failure
  1. OrganizeDocuments (Drive operations) — unchanged
  2. SyncCalendarAttachments (Calendar → Drive shortcuts) — unchanged
  [caller handles:]
  3. For each transcript:
     a. Sensitivity Gate (granite-guardian) — skip if sensitive
     b. If not sensitive (or dry-run): proceed
  4. Assign Tasks (local granite3.2:8b instead of browser/Gemini)
  5. Extract Decisions:
     - local_only=true: granite3.2:8b
     - local_only=false: Gemini (existing)
  6. Export decisions (markdown — existing)
```

### Interface Design

The sensitivity gate introduces a new interface consumed by the organizer:

```go
// SensitivityClassifier classifies transcript content for sensitivity.
type SensitivityClassifier interface {
    Classify(ctx context.Context, transcript string) (*models.SensitivityResult, error)
}
```

The task assigner reuses the existing `GeminiService` interface pattern but for local execution:

```go
// TaskAssigner extracts assignees from checkbox items using local AI.
type TaskAssigner interface {
    ExtractAssignees(ctx context.Context, items []CheckboxItem) ([]CheckboxAssignment, error)
}
```

---

## 8. Doctor Check Design

### New Checks (4 total, all conditional on `ollama.enabled`)

| # | Check | Pass | Fail | Fix |
|---|-------|------|------|-----|
| 11 | Ollama binary installed | `ollama` in PATH | Binary not found | `brew install ollama` |
| 12 | Ollama service running | `GET /api/tags` returns 200 | Connection refused / timeout | `ollama serve` |
| 13 | Sensitivity model pulled | `granite-guardian` in `/api/tags` response | Model not in list | `ollama pull granite-guardian` |
| 14 | Assignment model pulled | `granite3.2:8b` in `/api/tags` response | Model not in list | `ollama pull granite3.2:8b` |

When `ollama.enabled` is false, all 4 checks are skipped entirely (FR-027).

### Init Model Pull Prompt

```
Local AI models are needed for transcript screening.
Pull them now? (Y/n)
```

If confirmed, runs:
```bash
ollama pull granite-guardian
ollama pull granite3.2:8b
```

---

## 9. Transcript Truncation Strategy

Per spec edge case: "The system truncates the transcript with a warning log and classifies/processes the truncated version. The truncation point preserves the beginning and end of the transcript."

**Strategy**: Keep first 60% and last 40% of the content when truncation is needed. Sensitive topics are most likely introduced at the beginning (agenda items) or summarized at the end (action items, next steps).

**Implementation**: Check byte length against model context window (configurable, default ~6000 tokens ≈ ~24000 characters for granite-guardian). If exceeded, take first 60% of characters + `\n\n[... transcript truncated ...]\n\n` + last 40% of characters.

---

## 10. Key Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Granite Guardian misclassifies non-sensitive content as sensitive | False positives skip legitimate transcripts | Configurable threshold (default 0.7), dry-run mode to preview classifications, verbose logging of reasoning |
| Granite 3.2:8b produces lower quality assignments than Gemini | Incorrect or missing assignees | Same prompt as Gemini (proven), SC-002 requires 90% agreement on test set |
| Ollama service crashes mid-pipeline | Pipeline hangs until timeout | Configurable timeout (default 120s), hard-stop with actionable error |
| Model context window exceeded | Truncated transcripts miss sensitive content | Preserve beginning + end (where sensitive topics most likely appear), log warning |
| User forgets to start Ollama before running pipeline | Confusing error | Doctor check + clear error message with `ollama serve` fix instruction |
