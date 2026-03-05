package auth

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jflowers/gcal-organizer/internal/secrets"
	"golang.org/x/oauth2"
)

// mockStore is a simple in-memory SecretStore for testing.
type mockStore struct {
	data     map[string]string
	setCalls int
	mu       sync.Mutex
}

func newMockStore() *mockStore {
	return &mockStore{data: make(map[string]string)}
}

func (m *mockStore) Get(key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[key]
	if !ok {
		return "", secrets.ErrNotFound
	}
	return v, nil
}

func (m *mockStore) Set(key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	m.setCalls++
	return nil
}

func (m *mockStore) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

// TestOAuthClient_LoadFromStore verifies that loadToken retrieves and
// deserializes a JSON-serialized oauth2.Token from the SecretStore.
func TestOAuthClient_LoadFromStore(t *testing.T) {
	store := newMockStore()

	// Pre-populate a valid token in the store
	tok := &oauth2.Token{
		AccessToken:  "ya29.test-access-token",
		RefreshToken: "1//test-refresh-token",
		TokenType:    "Bearer",
		Expiry:       time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(tok)
	if err != nil {
		t.Fatalf("marshal token: %v", err)
	}
	store.data[secrets.KeyOAuthToken] = string(data)

	client := &OAuthClient{store: store}
	loaded, err := client.loadToken()
	if err != nil {
		t.Fatalf("loadToken: %v", err)
	}

	if loaded.AccessToken != tok.AccessToken {
		t.Errorf("AccessToken: got %q, want %q", loaded.AccessToken, tok.AccessToken)
	}
	if loaded.RefreshToken != tok.RefreshToken {
		t.Errorf("RefreshToken: got %q, want %q", loaded.RefreshToken, tok.RefreshToken)
	}
	if !loaded.Expiry.Equal(tok.Expiry) {
		t.Errorf("Expiry: got %v, want %v", loaded.Expiry, tok.Expiry)
	}
}

// TestOAuthClient_LoadFromStore_NotFound verifies that loadToken returns an
// error when no token exists in the store.
func TestOAuthClient_LoadFromStore_NotFound(t *testing.T) {
	store := newMockStore()
	client := &OAuthClient{store: store}

	_, err := client.loadToken()
	if err == nil {
		t.Fatal("loadToken: expected error for missing token, got nil")
	}
}

// TestOAuthClient_SaveToStore verifies that saveToken JSON-serializes and
// stores the token via store.Set(KeyOAuthToken, ...).
func TestOAuthClient_SaveToStore(t *testing.T) {
	store := newMockStore()
	client := &OAuthClient{store: store}

	tok := &oauth2.Token{
		AccessToken:  "ya29.saved-access",
		RefreshToken: "1//saved-refresh",
		TokenType:    "Bearer",
		Expiry:       time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC),
	}

	if err := client.saveToken(tok); err != nil {
		t.Fatalf("saveToken: %v", err)
	}

	// Verify it was stored
	raw, ok := store.data[secrets.KeyOAuthToken]
	if !ok {
		t.Fatal("token was not stored in the store")
	}

	// Verify round-trip: deserialize and compare
	var got oauth2.Token
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("unmarshal stored token: %v", err)
	}
	if got.AccessToken != tok.AccessToken {
		t.Errorf("stored AccessToken: got %q, want %q", got.AccessToken, tok.AccessToken)
	}
	if got.RefreshToken != tok.RefreshToken {
		t.Errorf("stored RefreshToken: got %q, want %q", got.RefreshToken, tok.RefreshToken)
	}
}

// mockTokenSource is a test double that returns different tokens on successive
// calls to simulate OAuth2 token refresh.
type mockTokenSource struct {
	tokens []*oauth2.Token
	idx    int
	mu     sync.Mutex
}

func (m *mockTokenSource) Token() (*oauth2.Token, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.idx >= len(m.tokens) {
		return nil, fmt.Errorf("no more tokens")
	}
	tok := m.tokens[m.idx]
	m.idx++
	return tok, nil
}

// TestPersistingTokenSource verifies that persistingTokenSource:
// 1. Calls store.Set when the token changes (simulating refresh)
// 2. Does NOT call store.Set when the token is unchanged
func TestPersistingTokenSource(t *testing.T) {
	initialToken := &oauth2.Token{
		AccessToken:  "initial-access",
		RefreshToken: "1//refresh",
		Expiry:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	refreshedToken := &oauth2.Token{
		AccessToken:  "refreshed-access",
		RefreshToken: "1//refresh",
		Expiry:       time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC),
	}

	store := newMockStore()
	baseTS := &mockTokenSource{
		tokens: []*oauth2.Token{initialToken, initialToken, refreshedToken},
	}

	pts := &persistingTokenSource{
		base:    baseTS,
		store:   store,
		current: initialToken,
	}

	// First call: token unchanged from current → should NOT persist
	tok1, err := pts.Token()
	if err != nil {
		t.Fatalf("Token() call 1: %v", err)
	}
	if tok1.AccessToken != "initial-access" {
		t.Errorf("call 1: got %q, want %q", tok1.AccessToken, "initial-access")
	}
	if store.setCalls != 0 {
		t.Errorf("call 1: expected 0 Set calls (unchanged token), got %d", store.setCalls)
	}

	// Second call: still the same token → should NOT persist
	tok2, err := pts.Token()
	if err != nil {
		t.Fatalf("Token() call 2: %v", err)
	}
	if tok2.AccessToken != "initial-access" {
		t.Errorf("call 2: got %q, want %q", tok2.AccessToken, "initial-access")
	}
	if store.setCalls != 0 {
		t.Errorf("call 2: expected 0 Set calls (unchanged token), got %d", store.setCalls)
	}

	// Third call: refreshed token → SHOULD persist
	tok3, err := pts.Token()
	if err != nil {
		t.Fatalf("Token() call 3: %v", err)
	}
	if tok3.AccessToken != "refreshed-access" {
		t.Errorf("call 3: got %q, want %q", tok3.AccessToken, "refreshed-access")
	}
	if store.setCalls != 1 {
		t.Errorf("call 3: expected 1 Set call (refreshed token), got %d", store.setCalls)
	}

	// Verify the persisted value is correct
	raw, ok := store.data[secrets.KeyOAuthToken]
	if !ok {
		t.Fatal("refreshed token was not persisted to store")
	}
	var persisted oauth2.Token
	if err := json.Unmarshal([]byte(raw), &persisted); err != nil {
		t.Fatalf("unmarshal persisted token: %v", err)
	}
	if persisted.AccessToken != "refreshed-access" {
		t.Errorf("persisted AccessToken: got %q, want %q", persisted.AccessToken, "refreshed-access")
	}
}

// TestPersistingTokenSource_ErrorPropagated verifies that errors from the
// underlying TokenSource are propagated correctly.
func TestPersistingTokenSource_ErrorPropagated(t *testing.T) {
	store := newMockStore()
	baseTS := &mockTokenSource{tokens: nil} // will return error immediately

	pts := &persistingTokenSource{
		base:  baseTS,
		store: store,
	}

	_, err := pts.Token()
	if err == nil {
		t.Fatal("expected error from empty token source, got nil")
	}
}

// TestOAuthClient_LoadCredentialsFromStore verifies that NewOAuthClient reads
// client credentials from the SecretStore first, without needing a file on disk.
func TestOAuthClient_LoadCredentialsFromStore(t *testing.T) {
	store := newMockStore()

	// A minimal but valid OAuth2 credentials JSON for a "web" application.
	// google.ConfigFromJSON requires at least client_id, client_secret,
	// and at least one of auth_uri/token_uri.
	credsJSON := `{
		"installed": {
			"client_id": "123-test.apps.googleusercontent.com",
			"client_secret": "GOCSPX-test-secret",
			"auth_uri": "https://accounts.google.com/o/oauth2/auth",
			"token_uri": "https://oauth2.googleapis.com/token",
			"redirect_uris": ["http://localhost"]
		}
	}`

	// Store credentials in the mock store (no file on disk)
	store.data[secrets.KeyClientCredentials] = credsJSON

	// NewOAuthClient should succeed using the store, even though the
	// fallback file path doesn't exist.
	client, err := NewOAuthClient(store, "/nonexistent/credentials.json")
	if err != nil {
		t.Fatalf("NewOAuthClient with store credentials: %v", err)
	}

	if client.config == nil {
		t.Fatal("expected non-nil oauth2.Config")
	}
	if client.config.ClientID != "123-test.apps.googleusercontent.com" {
		t.Errorf("ClientID: got %q, want %q", client.config.ClientID, "123-test.apps.googleusercontent.com")
	}
}

// TestOAuthClient_LoadCredentials_NeitherSource verifies that NewOAuthClient
// returns an actionable error when credentials are in neither the store nor the file.
func TestOAuthClient_LoadCredentials_NeitherSource(t *testing.T) {
	store := newMockStore()

	_, err := NewOAuthClient(store, "/nonexistent/credentials.json")
	if err == nil {
		t.Fatal("expected error when no credentials available, got nil")
	}
}
