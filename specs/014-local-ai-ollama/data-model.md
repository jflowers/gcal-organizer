# Data Model: Local AI via Ollama with IBM Granite

**Feature**: 014-local-ai-ollama
**Date**: 2026-04-20
**Purpose**: Define types, interfaces, and configuration schema

---

## 1. New Types

### SensitivityResult (pkg/models/models.go)

```go
// SensitivityResult is the output of the sensitivity classifier for a single
// transcript. Contains a binary determination, a category, a confidence score,
// and a reasoning explanation.
type SensitivityResult struct {
    // Sensitive is true when the transcript contains sensitive content
    // that should not be processed by cloud AI or written to files.
    Sensitive bool `json:"sensitive"`

    // Category classifies the type of sensitive content detected.
    // One of: hr, legal, financial, health, termination, none.
    Category string `json:"category"`

    // Score is the classifier's confidence in the determination (0.0–1.0).
    // Transcripts with Score >= the configured threshold are treated as sensitive.
    Score float64 `json:"score"`

    // Reasoning explains why the content was classified as sensitive or not.
    // Logged at DEBUG level only (FR-004) to avoid leaking sensitive content
    // into standard log output.
    Reasoning string `json:"reasoning"`
}
```

### OllamaConfig (internal/config/config.go)

```go
// OllamaConfig holds configuration for the local AI integration via Ollama.
type OllamaConfig struct {
    // Enabled controls whether local AI features are active.
    // When false, all Ollama checks and features are skipped.
    Enabled bool `mapstructure:"enabled"`

    // Endpoint is the Ollama API base URL (e.g., "http://localhost:11434").
    Endpoint string `mapstructure:"endpoint"`

    // Timeout is the HTTP request timeout for generation requests, in seconds.
    Timeout int `mapstructure:"timeout"`

    // Sensitivity holds settings for the sensitivity classification gate.
    Sensitivity SensitivityConfig `mapstructure:"sensitivity"`

    // Assignments holds settings for local task assignment.
    Assignments AssignmentsConfig `mapstructure:"assignments"`

    // LocalOnly prevents all cloud AI API calls when true.
    // Decision extraction uses the local model instead of Gemini.
    LocalOnly bool `mapstructure:"local_only"`
}

// SensitivityConfig holds settings for the sensitivity classification gate.
type SensitivityConfig struct {
    // Enabled controls whether the sensitivity gate is active (FR-006).
    // Independent of the top-level OllamaConfig.Enabled toggle.
    // When false, transcripts are not screened for sensitivity.
    Enabled bool `mapstructure:"enabled"`

    // Model is the Ollama model name for sensitivity classification.
    Model string `mapstructure:"model"`

    // Threshold is the minimum score for a transcript to be classified
    // as sensitive. Comparison is >= (inclusive). Range: 0.0–1.0.
    Threshold float64 `mapstructure:"threshold"`
}

// AssignmentsConfig holds settings for local task assignment.
type AssignmentsConfig struct {
    // Model is the Ollama model name for assignee extraction.
    Model string `mapstructure:"model"`
}
```

**Default values** (FR-021):
```go
Ollama: OllamaConfig{
    Enabled:  true,
    Endpoint: "http://localhost:11434",
    Timeout:  120,
    Sensitivity: SensitivityConfig{
        Enabled:   true,
        Model:     "granite-guardian",
        Threshold: 0.7,
    },
    Assignments: AssignmentsConfig{
        Model: "granite3.2:8b",
    },
    LocalOnly: false,
}
```

---

## 2. Interfaces

### SensitivityClassifier (internal/ollama/guardian.go)

```go
// SensitivityClassifier classifies transcript content for sensitivity
// before any processing occurs. Implementations must be safe for
// concurrent use.
type SensitivityClassifier interface {
    // Classify analyzes a transcript and returns a sensitivity determination.
    // Returns an error if the classification fails (model unavailable,
    // malformed response after retry, timeout).
    Classify(ctx context.Context, transcript string) (*models.SensitivityResult, error)
}
```

### TaskAssigner (internal/ollama/assigner.go)

```go
// TaskAssigner extracts assignees from checkbox action items using
// a local AI model. Produces results in the same format as the
// existing cloud-based extraction (FR-014).
//
// Note: CheckboxItem and CheckboxAssignment types are defined in
// pkg/models/models.go (moved from internal/gemini/client.go) so
// that both internal/ollama and internal/gemini can use them without
// creating a cross-package dependency (review finding A-1).
type TaskAssigner interface {
    // ExtractAssignees identifies the responsible individual for each
    // checkbox item. Returns assignments only for items with a clear
    // single assignee; items with group/ambiguous assignees are omitted.
    ExtractAssignees(ctx context.Context, items []models.CheckboxItem) ([]models.CheckboxAssignment, error)
}
```

### DecisionExtractor (internal/ollama/decisions.go)

```go
// DecisionExtractor extracts structured decisions from meeting transcripts
// using a local AI model. Used in local-only mode as a replacement for
// the cloud-based Gemini extraction.
type DecisionExtractor interface {
    // ExtractDecisions analyzes a transcript and returns categorized decisions
    // (made, deferred, open) with text, timestamp, and context fields.
    ExtractDecisions(ctx context.Context, transcriptText string) ([]models.Decision, error)
}
```

### OllamaClient (internal/ollama/client.go)

```go
// Client provides low-level HTTP communication with the Ollama API.
// All higher-level components (SensitivityClassifier, TaskAssigner,
// DecisionExtractor) use Client for HTTP operations.
//
// Design decision: Single client shared across components rather than
// each component managing its own HTTP client. This centralizes timeout
// configuration, health checking, and model availability caching.
type Client struct {
    baseURL string
    client  *http.Client

    // Model availability cache (same pattern as Dewey's OllamaSynthesizer)
    mu            sync.RWMutex
    modelCache    map[string]bool // model name → available
    lastCheck     time.Time
    checkInterval time.Duration
}

// NewClient creates an OllamaClient that connects to the Ollama API
// at the given endpoint with the specified timeout.
func NewClient(endpoint string, timeoutSeconds int) *Client

// Generate sends a prompt to the specified model via POST /api/generate
// and returns the generated text. Uses stream=false for single-response mode.
func (c *Client) Generate(ctx context.Context, model, prompt string) (string, error)

// HealthCheck reports whether Ollama is reachable by sending GET /api/tags
// with a 2-second timeout. Returns true if the response is HTTP 200.
func (c *Client) HealthCheck() bool

// ModelAvailable reports whether the specified model is available in the
// Ollama instance by querying GET /api/tags. Caches results for 30 seconds.
func (c *Client) ModelAvailable(model string) bool

// ListModels returns the names of all models available in the Ollama instance.
func (c *Client) ListModels() ([]string, error)
```

---

### Moved Types: CheckboxItem and CheckboxAssignment (pkg/models/models.go)

Move from `internal/gemini/client.go` to `pkg/models/models.go` to allow both
`internal/ollama` and `internal/gemini` to use them without cross-package dependency.

```go
// CheckboxItem represents a single checkbox action item from a Google Doc.
type CheckboxItem struct {
    Index int
    Text  string
}

// CheckboxAssignment is the result of assignee extraction for a checkbox item.
type CheckboxAssignment struct {
    Index    int
    Text     string
    Assignee string
    Email    string
}
```

Update `internal/gemini/client.go` to import these from `pkg/models` instead of defining them locally. Type aliases may be used for backward compatibility if needed.

---

## 3. Modified Types

### Config (internal/config/config.go)

Add `Ollama` field to existing `Config` struct:

```go
type Config struct {
    // ... existing fields ...

    // Ollama holds configuration for the local AI integration.
    Ollama OllamaConfig `mapstructure:"ollama"`
}
```

### Stats (internal/organizer/organizer.go)

Add sensitivity tracking fields:

```go
type Stats struct {
    // ... existing fields ...

    // SensitivitySkipped counts transcripts skipped due to sensitivity classification.
    SensitivitySkipped int

    // SensitivityProcessed counts transcripts that passed the sensitivity gate.
    SensitivityProcessed int
}
```

### Organizer (internal/organizer/organizer.go)

Add optional sensitivity classifier field:

```go
type Organizer struct {
    // ... existing fields ...

    // sensitivityClassifier is the optional sensitivity gate.
    // When non-nil, every transcript is classified before processing.
    sensitivityClassifier ollama.SensitivityClassifier
}
```

---

## 4. Ollama API Request/Response Schemas

### POST /api/generate

**Request**:
```json
{
    "model": "granite-guardian",
    "prompt": "You are a workplace content sensitivity classifier...",
    "stream": false
}
```

**Response** (stream=false):
```json
{
    "model": "granite-guardian",
    "created_at": "2026-04-20T12:00:00Z",
    "response": "{\"sensitive\":true,\"category\":\"hr\",\"score\":0.92,\"reasoning\":\"...\"}",
    "done": true,
    "total_duration": 1234567890,
    "load_duration": 123456789,
    "prompt_eval_count": 100,
    "eval_count": 50
}
```

### GET /api/tags

**Response**:
```json
{
    "models": [
        {
            "name": "granite-guardian:latest",
            "model": "granite-guardian:latest",
            "modified_at": "2026-04-20T12:00:00Z",
            "size": 4700000000
        },
        {
            "name": "granite3.2:8b",
            "model": "granite3.2:8b",
            "modified_at": "2026-04-20T12:00:00Z",
            "size": 4900000000
        }
    ]
}
```

---

## 5. Configuration YAML Schema

```yaml
# Local AI configuration (Ollama with IBM Granite models)
ollama:
  # Enable local AI features (sensitivity gate, local task assignment)
  # When disabled, all Ollama checks and features are skipped.
  # Default: true
  enabled: true

  # Ollama API endpoint
  # Default: http://localhost:11434
  endpoint: "http://localhost:11434"

  # Request timeout for generation requests (seconds)
  # Default: 120
  timeout: 120

  # Sensitivity classification settings
  sensitivity:
    # Model for sensitivity classification
    # Default: granite-guardian
    model: "granite-guardian"

    # Sensitivity threshold (0.0–1.0)
    # Transcripts scoring >= this value are skipped.
    # Default: 0.7
    threshold: 0.7

  # Task assignment settings
  assignments:
    # Model for assignee extraction
    # Default: granite3.2:8b
    model: "granite3.2:8b"

  # Local-only mode
  # When true, all AI processing runs locally (no cloud AI calls).
  # Decision extraction uses the local model instead of Gemini.
  # Default: false
  local_only: false
```

---

## 6. Error Types

No custom error types are introduced. All errors use `fmt.Errorf` with `%w` wrapping per constitution principle V and convention CS-006.

Sentinel-style errors for specific conditions:

```go
// ErrOllamaUnavailable indicates Ollama is configured but not reachable.
// The pipeline must hard-stop (FR-009).
var ErrOllamaUnavailable = errors.New("ollama is configured but not available")

// ErrModelNotAvailable indicates a required model is not pulled.
var ErrModelNotAvailable = errors.New("required ollama model is not available")

// ErrMalformedResponse indicates the model returned unparseable output
// after the retry attempt.
var ErrMalformedResponse = errors.New("ollama model returned malformed response")
```

---

## 7. Package Dependency Graph

```
cmd/gcal-organizer/
    └── imports: internal/config, internal/ollama, internal/organizer

internal/organizer/
    └── imports: internal/config, internal/ollama (interfaces only), pkg/models

internal/ollama/
    ├── client.go    → imports: net/http, encoding/json (stdlib only)
    ├── guardian.go   → imports: internal/ollama (Client), pkg/models
    ├── assigner.go   → imports: internal/ollama (Client), internal/gemini (types only)
    └── decisions.go  → imports: internal/ollama (Client), pkg/models

internal/config/
    └── imports: (no new imports — OllamaConfig is a plain struct)

pkg/models/
    └── imports: (no new imports — SensitivityResult is a plain struct)
```

**No circular dependencies.** The `internal/ollama` package is a leaf that imports only stdlib, `pkg/models`, and `internal/gemini` (for shared types like `CheckboxItem` and `CheckboxAssignment`). The organizer imports ollama interfaces, not concrete types.
