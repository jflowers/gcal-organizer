// Package ollama provides a raw HTTP client for the Ollama REST API.
// All communication uses net/http (FR-012: no SDK dependency).
// Design follows the proven Dewey llm.go pattern: stream=false, response
// body cap, context propagation, and 30-second model availability cache.
package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Sentinel errors for specific Ollama failure conditions.
var (
	// ErrOllamaUnavailable indicates Ollama is configured but not reachable.
	// The pipeline must hard-stop (FR-009).
	ErrOllamaUnavailable = errors.New("ollama is configured but not available")

	// ErrModelNotAvailable indicates a required model is not pulled.
	ErrModelNotAvailable = errors.New("required ollama model is not available")

	// ErrMalformedResponse indicates the model returned unparseable output
	// after the retry attempt.
	ErrMalformedResponse = errors.New("ollama model returned malformed response")
)

// maxResponseBytes caps the response body size to prevent unbounded memory
// allocation from unexpectedly large responses (same as Dewey: 50 MB).
const maxResponseBytes = 50 * 1024 * 1024

// generateRequest is the JSON body for POST /api/generate.
type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// generateResponse is the JSON response from POST /api/generate (stream=false).
type generateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// tagsResponse is the JSON response from GET /api/tags.
type tagsResponse struct {
	Models []modelInfo `json:"models"`
}

// modelInfo represents a single model entry from /api/tags.
type modelInfo struct {
	Name string `json:"name"`
}

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

	// Model availability cache (same pattern as Dewey's OllamaSynthesizer).
	mu            sync.RWMutex
	modelCache    map[string]bool
	lastCheck     time.Time
	checkInterval time.Duration
}

// NewClient creates a Client that connects to the Ollama API at the given
// endpoint with the specified timeout (in seconds) for generation requests.
func NewClient(endpoint string, timeoutSeconds int) *Client {
	return &Client{
		baseURL: strings.TrimRight(endpoint, "/"),
		client: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
		modelCache:    make(map[string]bool),
		checkInterval: 30 * time.Second,
	}
}

// Generate sends a prompt to the specified model via POST /api/generate
// and returns the generated text. Uses stream=false for single-response mode.
// The response body is capped at 50 MB to prevent unbounded memory allocation.
func (c *Client) Generate(ctx context.Context, model, prompt string) (string, error) {
	reqBody := generateRequest{
		Model:  model,
		Prompt: prompt,
		Stream: false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal generate request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", fmt.Errorf("create generate request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama generate request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama generate returned HTTP %d", resp.StatusCode)
	}

	// Cap response body to prevent unbounded memory allocation.
	limited := io.LimitReader(resp.Body, maxResponseBytes)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("read generate response: %w", err)
	}

	var genResp generateResponse
	if err := json.Unmarshal(data, &genResp); err != nil {
		return "", fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	}

	return genResp.Response, nil
}

// HealthCheck reports whether Ollama is reachable by sending GET /api/tags
// with a 2-second timeout. Returns true if the response is HTTP 200.
func (c *Client) HealthCheck() bool {
	healthClient := &http.Client{Timeout: 2 * time.Second}
	resp, err := healthClient.Get(c.baseURL + "/api/tags")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// ModelAvailable reports whether the specified model is available in the
// Ollama instance by querying GET /api/tags. Caches results for 30 seconds
// to avoid redundant HTTP calls during pipeline runs.
func (c *Client) ModelAvailable(model string) bool {
	c.mu.RLock()
	if time.Since(c.lastCheck) < c.checkInterval {
		// Check exact match and :latest suffix match in cache.
		if c.modelCache[model] || c.modelCache[model+":latest"] {
			c.mu.RUnlock()
			return true
		}
		// Model not found in a fresh cache — it's genuinely missing.
		if len(c.modelCache) > 0 {
			c.mu.RUnlock()
			return false
		}
	}
	c.mu.RUnlock()

	// Cache miss or expired — refresh.
	models, err := c.ListModels()
	if err != nil {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Rebuild cache.
	c.modelCache = make(map[string]bool)
	for _, m := range models {
		c.modelCache[m] = true
	}
	c.lastCheck = time.Now()

	// Check with :latest suffix matching: "granite-guardian" matches
	// "granite-guardian:latest".
	return c.modelCache[model] || c.modelCache[model+":latest"]
}

// ListModels returns the names of all models available in the Ollama instance.
func (c *Client) ListModels() ([]string, error) {
	resp, err := c.client.Get(c.baseURL + "/api/tags")
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list models returned HTTP %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, maxResponseBytes)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read models response: %w", err)
	}

	var tags tagsResponse
	if err := json.Unmarshal(data, &tags); err != nil {
		return nil, fmt.Errorf("parse models response: %w", err)
	}

	names := make([]string, len(tags.Models))
	for i, m := range tags.Models {
		names[i] = m.Name
	}
	return names, nil
}
