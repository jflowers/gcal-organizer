package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jflowers/gcal-organizer/pkg/models"
)

func TestAssigner_SingleAssignee(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := generateResponse{
			Response: `[{"index": 0, "assignee": "Jay"}]`,
			Done:     true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	assigner := NewAssigner(client, "granite3.2:8b")

	items := []models.CheckboxItem{
		{Index: 0, Text: "Jay will schedule the follow-up meeting"},
	}

	assignments, err := assigner.ExtractAssignees(context.Background(), items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assignments))
	}
	if assignments[0].Assignee != "Jay" {
		t.Errorf("expected assignee 'Jay', got %q", assignments[0].Assignee)
	}
	if assignments[0].Text != "Jay will schedule the follow-up meeting" {
		t.Errorf("expected text preserved, got %q", assignments[0].Text)
	}
}

func TestAssigner_GroupAssigneeReturnsNull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := generateResponse{
			Response: `[{"index": 0, "assignee": null}]`,
			Done:     true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	assigner := NewAssigner(client, "granite3.2:8b")

	items := []models.CheckboxItem{
		{Index: 0, Text: "The team will discuss the proposal"},
	}

	assignments, err := assigner.ExtractAssignees(context.Background(), items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Null assignees are omitted from results
	if len(assignments) != 0 {
		t.Errorf("expected 0 assignments for group task, got %d", len(assignments))
	}
}

func TestAssigner_AmbiguousReturnsNull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := generateResponse{
			Response: `[{"index": 0, "assignee": null}]`,
			Done:     true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	assigner := NewAssigner(client, "granite3.2:8b")

	items := []models.CheckboxItem{
		{Index: 0, Text: "Someone should check the report"},
	}

	assignments, err := assigner.ExtractAssignees(context.Background(), items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assignments) != 0 {
		t.Errorf("expected 0 assignments for ambiguous task, got %d", len(assignments))
	}
}

func TestAssigner_MultipleMixed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := generateResponse{
			Response: `[
				{"index": 0, "assignee": "Jay"},
				{"index": 1, "assignee": null},
				{"index": 2, "assignee": "Sarah"}
			]`,
			Done: true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	assigner := NewAssigner(client, "granite3.2:8b")

	items := []models.CheckboxItem{
		{Index: 0, Text: "Jay will schedule the meeting"},
		{Index: 1, Text: "The team will review"},
		{Index: 2, Text: "Sarah will send the email"},
	}

	assignments, err := assigner.ExtractAssignees(context.Background(), items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assignments) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(assignments))
	}
	if assignments[0].Assignee != "Jay" {
		t.Errorf("expected first assignee 'Jay', got %q", assignments[0].Assignee)
	}
	if assignments[1].Assignee != "Sarah" {
		t.Errorf("expected second assignee 'Sarah', got %q", assignments[1].Assignee)
	}
}

func TestAssigner_EmptyItems(t *testing.T) {
	client := NewClient("http://unused", 10)
	assigner := NewAssigner(client, "granite3.2:8b")

	assignments, err := assigner.ExtractAssignees(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if assignments != nil {
		t.Errorf("expected nil for empty items, got %v", assignments)
	}
}

func TestAssigner_MalformedResponseRetry(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var respText string
		if callCount == 1 {
			respText = "not json"
		} else {
			respText = `[{"index": 0, "assignee": "Jay"}]`
		}
		resp := generateResponse{Response: respText, Done: true}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, 10)
	assigner := NewAssigner(client, "granite3.2:8b")

	items := []models.CheckboxItem{
		{Index: 0, Text: "Jay will do it"},
	}

	assignments, err := assigner.ExtractAssignees(context.Background(), items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assignments) != 1 {
		t.Fatalf("expected 1 assignment after retry, got %d", len(assignments))
	}
	if callCount != 2 {
		t.Errorf("expected 2 attempts, got %d", callCount)
	}
}

func TestAssigner_NetworkError_HardStop(t *testing.T) {
	client := NewClient("http://127.0.0.1:1", 2)
	assigner := NewAssigner(client, "granite3.2:8b")

	items := []models.CheckboxItem{
		{Index: 0, Text: "Jay will do it"},
	}

	_, err := assigner.ExtractAssignees(context.Background(), items)
	if err == nil {
		t.Fatal("expected error for network failure")
	}
	if errors.Is(err, ErrMalformedResponse) {
		t.Error("network error should not be wrapped as ErrMalformedResponse")
	}
}
