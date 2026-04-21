package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jflowers/gcal-organizer/pkg/models"
)

func TestGuardian_ClassifySensitive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := generateResponse{
			Response: `{"sensitive":true,"category":"hr","score":0.92,"reasoning":"Discussion about employee performance"}`,
			Done:     true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	guardian := NewGuardian(client, "granite-guardian")

	result, err := guardian.Classify(context.Background(), "We need to discuss Sarah's performance issues")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Sensitive {
		t.Error("expected Sensitive=true")
	}
	if result.Category != "hr" {
		t.Errorf("expected category 'hr', got %q", result.Category)
	}
	if result.Score != 0.92 {
		t.Errorf("expected score 0.92, got %f", result.Score)
	}
}

func TestGuardian_ClassifyNotSensitive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := generateResponse{
			Response: `{"sensitive":false,"category":"none","score":0.1,"reasoning":"Routine sprint planning"}`,
			Done:     true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	guardian := NewGuardian(client, "granite-guardian")

	result, err := guardian.Classify(context.Background(), "Sprint planning for next week")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Sensitive {
		t.Error("expected Sensitive=false")
	}
	if result.Category != "none" {
		t.Errorf("expected category 'none', got %q", result.Category)
	}
}

func TestGuardian_ThresholdBoundaryInclusive(t *testing.T) {
	// Score exactly at threshold (0.70) should be treated as sensitive (FR-003: >=)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := generateResponse{
			Response: `{"sensitive":true,"category":"legal","score":0.70,"reasoning":"Borderline case"}`,
			Done:     true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	guardian := NewGuardian(client, "granite-guardian")

	result, err := guardian.Classify(context.Background(), "Some transcript")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Score != 0.70 {
		t.Errorf("expected score 0.70, got %f", result.Score)
	}
	// The threshold comparison (>= 0.7) is done by the caller, not the classifier.
	// The classifier just returns the result.
}

func TestGuardian_ThresholdBoundaryBelow(t *testing.T) {
	// Score below threshold (0.69) should NOT be treated as sensitive
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := generateResponse{
			Response: `{"sensitive":false,"category":"none","score":0.69,"reasoning":"Below threshold"}`,
			Done:     true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	guardian := NewGuardian(client, "granite-guardian")

	result, err := guardian.Classify(context.Background(), "Some transcript")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Score != 0.69 {
		t.Errorf("expected score 0.69, got %f", result.Score)
	}
}

func TestGuardian_MalformedResponseRetrySuccess(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var respText string
		if callCount == 1 {
			respText = "not valid json at all"
		} else {
			respText = `{"sensitive":false,"category":"none","score":0.1,"reasoning":"ok"}`
		}
		resp := generateResponse{Response: respText, Done: true}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	guardian := NewGuardian(client, "granite-guardian")

	result, err := guardian.Classify(context.Background(), "Some transcript")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Category != "none" {
		t.Errorf("expected category 'none', got %q", result.Category)
	}
	if callCount != 2 {
		t.Errorf("expected 2 attempts, got %d", callCount)
	}
}

func TestGuardian_MalformedResponseRetryFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := generateResponse{Response: "not json", Done: true}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	guardian := NewGuardian(client, "granite-guardian")

	_, err := guardian.Classify(context.Background(), "Some transcript")
	if err == nil {
		t.Fatal("expected error after 2 failed attempts")
	}
	if !errors.Is(err, ErrMalformedResponse) {
		t.Errorf("expected ErrMalformedResponse, got: %v", err)
	}
}

func TestGuardian_NetworkError_HardStop(t *testing.T) {
	client := NewClient("http://127.0.0.1:1", 2)
	guardian := NewGuardian(client, "granite-guardian")

	_, err := guardian.Classify(context.Background(), "Some transcript")
	if err == nil {
		t.Fatal("expected error for network failure")
	}
	// Network errors should NOT retry — immediate hard-stop
	if errors.Is(err, ErrMalformedResponse) {
		t.Error("network error should not be wrapped as ErrMalformedResponse")
	}
}

func TestGuardian_TranscriptTruncation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse the request to check the prompt
		var req generateRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Verify truncation happened
		if !strings.Contains(req.Prompt, "[... transcript truncated ...]") {
			t.Error("expected truncation marker in prompt")
		}

		resp := generateResponse{
			Response: `{"sensitive":false,"category":"none","score":0.1,"reasoning":"truncated"}`,
			Done:     true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	guardian := NewGuardian(client, "granite-guardian")
	guardian.maxTranscriptLen = 100 // Set very low for testing

	longTranscript := strings.Repeat("x", 200)
	result, err := guardian.Classify(context.Background(), longTranscript)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestGuardian_AllCategories(t *testing.T) {
	categories := []string{"hr", "legal", "financial", "health", "termination", "none"}

	for _, cat := range categories {
		t.Run(cat, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				result := models.SensitivityResult{
					Sensitive: cat != "none",
					Category:  cat,
					Score:     0.85,
					Reasoning: "test",
				}
				respJSON, _ := json.Marshal(result)
				resp := generateResponse{Response: string(respJSON), Done: true}
				json.NewEncoder(w).Encode(resp)
			}))
			defer srv.Close()

			client := NewClient(srv.URL, 10)
			guardian := NewGuardian(client, "granite-guardian")

			result, err := guardian.Classify(context.Background(), "test transcript")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Category != cat {
				t.Errorf("expected category %q, got %q", cat, result.Category)
			}
		})
	}
}

func TestParseSensitivityResponse_ScoreClamping(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantScore float64
	}{
		{"negative score", `{"sensitive":false,"category":"none","score":-0.5,"reasoning":"test"}`, 0.0},
		{"score above 1", `{"sensitive":true,"category":"hr","score":1.5,"reasoning":"test"}`, 1.0},
		{"normal score", `{"sensitive":true,"category":"hr","score":0.85,"reasoning":"test"}`, 0.85},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseSensitivityResponse(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Score != tt.wantScore {
				t.Errorf("expected score %f, got %f", tt.wantScore, result.Score)
			}
		})
	}
}

func TestParseSensitivityResponse_UnknownCategory(t *testing.T) {
	result, err := parseSensitivityResponse(`{"sensitive":false,"category":"unknown_cat","score":0.5,"reasoning":"test"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Category != "none" {
		t.Errorf("expected unknown category to default to 'none', got %q", result.Category)
	}
}

func TestParseSensitivityResponse_MarkdownFences(t *testing.T) {
	input := "```json\n{\"sensitive\":false,\"category\":\"none\",\"score\":0.1,\"reasoning\":\"test\"}\n```"
	result, err := parseSensitivityResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Category != "none" {
		t.Errorf("expected category 'none', got %q", result.Category)
	}
}
