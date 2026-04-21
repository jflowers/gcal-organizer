package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGenerate_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/generate" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		resp := generateResponse{Response: "Hello, world!", Done: true}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	result, err := client.Generate(context.Background(), "test-model", "say hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello, world!" {
		t.Errorf("expected 'Hello, world!', got %q", result)
	}
}

func TestGenerate_HTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	_, err := client.Generate(context.Background(), "test-model", "say hello")
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("expected HTTP 500 in error, got: %v", err)
	}
}

func TestGenerate_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	_, err := client.Generate(context.Background(), "test-model", "say hello")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "malformed response") {
		t.Errorf("expected 'malformed response' in error, got: %v", err)
	}
}

func TestGenerate_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		resp := generateResponse{Response: "too late", Done: true}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 30)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.Generate(ctx, "test-model", "say hello")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestGenerate_UnreachableServer(t *testing.T) {
	client := NewClient("http://127.0.0.1:1", 2)
	_, err := client.Generate(context.Background(), "test-model", "say hello")
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestHealthCheck_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(tagsResponse{Models: []modelInfo{}})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	if !client.HealthCheck() {
		t.Error("expected HealthCheck to return true")
	}
}

func TestHealthCheck_Unreachable(t *testing.T) {
	client := NewClient("http://127.0.0.1:1", 2)
	if client.HealthCheck() {
		t.Error("expected HealthCheck to return false for unreachable server")
	}
}

func TestModelAvailable_Present(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(tagsResponse{
			Models: []modelInfo{
				{Name: "granite-guardian:latest"},
				{Name: "granite3.2:8b"},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)

	// Exact match
	if !client.ModelAvailable("granite3.2:8b") {
		t.Error("expected granite3.2:8b to be available")
	}

	// :latest suffix matching
	if !client.ModelAvailable("granite-guardian") {
		t.Error("expected granite-guardian to match granite-guardian:latest")
	}
}

func TestModelAvailable_Missing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(tagsResponse{
			Models: []modelInfo{
				{Name: "other-model:latest"},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	if client.ModelAvailable("granite-guardian") {
		t.Error("expected granite-guardian to be unavailable")
	}
}

func TestModelAvailable_CacheHit(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		json.NewEncoder(w).Encode(tagsResponse{
			Models: []modelInfo{
				{Name: "granite-guardian:latest"},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)

	// First call populates cache
	client.ModelAvailable("granite-guardian")
	firstCount := callCount

	// Second call should use cache (no additional HTTP call)
	client.ModelAvailable("granite-guardian")
	if callCount != firstCount {
		t.Errorf("expected cache hit (no additional HTTP call), but got %d total calls", callCount)
	}
}

func TestListModels_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(tagsResponse{
			Models: []modelInfo{
				{Name: "model-a"},
				{Name: "model-b:latest"},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	models, err := client.ListModels()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0] != "model-a" || models[1] != "model-b:latest" {
		t.Errorf("unexpected models: %v", models)
	}
}

func TestListModels_Error(t *testing.T) {
	client := NewClient("http://127.0.0.1:1", 2)
	_, err := client.ListModels()
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}
