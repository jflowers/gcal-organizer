package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDecisionExtractor_MadeDeferredOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := generateResponse{
			Response: `[
				{"category": "made", "text": "Adopt GitHub Actions", "timestamp": "12:34", "context": "Team voted"},
				{"category": "deferred", "text": "Budget allocation", "timestamp": "13:15", "context": "Waiting for finance"},
				{"category": "open", "text": "API migration", "timestamp": "13:45", "context": "Need benchmarks"}
			]`,
			Done: true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	extractor := NewDecisionExtractor(client, "granite3.2:8b")

	decisions, err := extractor.ExtractDecisions(context.Background(), "Meeting transcript here...")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(decisions) != 3 {
		t.Fatalf("expected 3 decisions, got %d", len(decisions))
	}

	// Verify all three categories present
	categories := map[string]bool{}
	for _, d := range decisions {
		categories[d.Category] = true
	}
	for _, cat := range []string{"made", "deferred", "open"} {
		if !categories[cat] {
			t.Errorf("expected category %q in results", cat)
		}
	}

	// Verify first decision
	if decisions[0].Text != "Adopt GitHub Actions" {
		t.Errorf("expected text 'Adopt GitHub Actions', got %q", decisions[0].Text)
	}
	if decisions[0].Timestamp != "12:34" {
		t.Errorf("expected timestamp '12:34', got %q", decisions[0].Timestamp)
	}
}

func TestDecisionExtractor_EmptyTranscript(t *testing.T) {
	client := NewClient("http://unused", 10)
	extractor := NewDecisionExtractor(client, "granite3.2:8b")

	decisions, err := extractor.ExtractDecisions(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decisions != nil {
		t.Errorf("expected nil for empty transcript, got %v", decisions)
	}
}

func TestDecisionExtractor_MalformedResponseRetrySuccess(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var respText string
		if callCount == 1 {
			respText = "not json at all"
		} else {
			respText = `[{"category": "made", "text": "Decision after retry", "timestamp": "", "context": ""}]`
		}
		resp := generateResponse{Response: respText, Done: true}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	extractor := NewDecisionExtractor(client, "granite3.2:8b")

	decisions, err := extractor.ExtractDecisions(context.Background(), "Some transcript")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision after retry, got %d", len(decisions))
	}
	if callCount != 2 {
		t.Errorf("expected 2 attempts, got %d", callCount)
	}
}

func TestDecisionExtractor_MalformedResponseRetryFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := generateResponse{Response: "not json", Done: true}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	extractor := NewDecisionExtractor(client, "granite3.2:8b")

	_, err := extractor.ExtractDecisions(context.Background(), "Some transcript")
	if err == nil {
		t.Fatal("expected error after 2 failed attempts")
	}
	if !errors.Is(err, ErrMalformedResponse) {
		t.Errorf("expected ErrMalformedResponse, got: %v", err)
	}
}

func TestDecisionExtractor_NetworkError_HardStop(t *testing.T) {
	client := NewClient("http://127.0.0.1:1", 2)
	extractor := NewDecisionExtractor(client, "granite3.2:8b")

	_, err := extractor.ExtractDecisions(context.Background(), "Some transcript")
	if err == nil {
		t.Fatal("expected error for network failure")
	}
	if errors.Is(err, ErrMalformedResponse) {
		t.Error("network error should not be wrapped as ErrMalformedResponse")
	}
}

func TestDecisionExtractor_AllThreeCategories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := generateResponse{
			Response: `[
				{"category": "made", "text": "Decision 1", "timestamp": "", "context": ""},
				{"category": "deferred", "text": "Decision 2", "timestamp": "", "context": ""},
				{"category": "open", "text": "Decision 3", "timestamp": "", "context": ""}
			]`,
			Done: true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	extractor := NewDecisionExtractor(client, "granite3.2:8b")

	decisions, err := extractor.ExtractDecisions(context.Background(), "Some transcript")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(decisions) != 3 {
		t.Fatalf("expected 3 decisions, got %d", len(decisions))
	}

	wantCategories := []string{"made", "deferred", "open"}
	for i, want := range wantCategories {
		if decisions[i].Category != want {
			t.Errorf("decision %d: expected category %q, got %q", i, want, decisions[i].Category)
		}
	}
}

func TestParseLocalDecisionsResponse_InvalidCategory(t *testing.T) {
	decisions, err := parseLocalDecisionsResponse(`[{"category": "unknown", "text": "Test", "timestamp": "", "context": ""}]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if decisions[0].Category != "open" {
		t.Errorf("expected unknown category to default to 'open', got %q", decisions[0].Category)
	}
}

func TestParseLocalDecisionsResponse_EmptyText(t *testing.T) {
	decisions, err := parseLocalDecisionsResponse(`[{"category": "made", "text": "", "timestamp": "", "context": ""}]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(decisions) != 0 {
		t.Errorf("expected empty text to be filtered out, got %d decisions", len(decisions))
	}
}
