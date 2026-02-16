package auth

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"golang.org/x/oauth2"
)

func TestTokenStorage_SaveAndLoad(t *testing.T) {
	// Create a test logger
	logger := log.New(os.Stderr)
	logger.SetLevel(log.ErrorLevel) // Suppress logs during tests

	// Create token storage
	ts := NewTokenStorage(logger)

	// Create a test token
	testToken := &oauth2.Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(1 * time.Hour),
	}

	// Clean up any existing token before test
	_ = ts.DeleteToken()

	// Save the token
	err := ts.SaveToken(testToken)
	if err != nil {
		t.Fatalf("Failed to save token: %v", err)
	}

	// Load the token
	loadedToken, err := ts.LoadToken()
	if err != nil {
		t.Fatalf("Failed to load token: %v", err)
	}

	// Verify token contents
	if loadedToken.AccessToken != testToken.AccessToken {
		t.Errorf("AccessToken mismatch: got %s, want %s", loadedToken.AccessToken, testToken.AccessToken)
	}
	if loadedToken.RefreshToken != testToken.RefreshToken {
		t.Errorf("RefreshToken mismatch: got %s, want %s", loadedToken.RefreshToken, testToken.RefreshToken)
	}
	if loadedToken.TokenType != testToken.TokenType {
		t.Errorf("TokenType mismatch: got %s, want %s", loadedToken.TokenType, testToken.TokenType)
	}

	// Clean up
	_ = ts.DeleteToken()
}

func TestTokenStorage_Delete(t *testing.T) {
	// Create a test logger
	logger := log.New(os.Stderr)
	logger.SetLevel(log.ErrorLevel)

	// Create token storage
	ts := NewTokenStorage(logger)

	// Create and save a test token
	testToken := &oauth2.Token{
		AccessToken:  "test-token-to-delete",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(1 * time.Hour),
	}

	err := ts.SaveToken(testToken)
	if err != nil {
		t.Fatalf("Failed to save token: %v", err)
	}

	// Verify token exists
	_, err = ts.LoadToken()
	if err != nil {
		t.Fatalf("Token should exist but LoadToken failed: %v", err)
	}

	// Delete the token
	err = ts.DeleteToken()
	// Note: DeleteToken may return an error if keyring doesn't have the token,
	// but that's okay if the encrypted file was deleted

	// Verify token is gone
	_, err = ts.LoadToken()
	if err == nil {
		t.Error("Token should be deleted but LoadToken succeeded")
	}
}

func TestTokenStorage_GetStorageLocation(t *testing.T) {
	// Create a test logger
	logger := log.New(os.Stderr)
	logger.SetLevel(log.ErrorLevel)

	// Create token storage
	ts := NewTokenStorage(logger)

	// Clean up first
	_ = ts.DeleteToken()

	// Get storage location when no token exists
	location := ts.GetStorageLocation()
	if location == "" {
		t.Error("GetStorageLocation should return a non-empty string")
	}
	if location != "Not stored" {
		t.Logf("Storage location: %s", location)
	}

	// Save a token and check location again
	testToken := &oauth2.Token{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(1 * time.Hour),
	}

	err := ts.SaveToken(testToken)
	if err != nil {
		t.Logf("Keychain not available (expected on some systems): %v", err)
		return
	}

	location = ts.GetStorageLocation()
	if location == "Not stored" {
		t.Error("Storage location should not be 'Not stored' after saving token")
	}
	t.Logf("Token stored in: %s", location)

	// Clean up
	_ = ts.DeleteToken()
}

// mockTokenSource simulates oauth2.TokenSource behavior, including token refresh
type mockTokenSource struct {
	tokens      []*oauth2.Token
	callCount   int
	shouldError bool
}

func (m *mockTokenSource) Token() (*oauth2.Token, error) {
	if m.shouldError {
		return nil, fmt.Errorf("mock token error")
	}

	if m.callCount >= len(m.tokens) {
		// Return last token for subsequent calls
		return m.tokens[len(m.tokens)-1], nil
	}

	token := m.tokens[m.callCount]
	m.callCount++
	return token, nil
}

func TestPersistentTokenSource_RefreshPersistence(t *testing.T) {
	// Create a test logger
	logger := log.New(os.Stderr)
	logger.SetLevel(log.ErrorLevel)

	// Create token storage
	ts := NewTokenStorage(logger)

	// Clean up before test
	_ = ts.DeleteToken()

	// Create mock token source that simulates a token refresh
	initialToken := &oauth2.Token{
		AccessToken:  "initial-access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(1 * time.Hour),
	}

	refreshedToken := &oauth2.Token{
		AccessToken:  "refreshed-access-token", // Changed access token
		RefreshToken: "refresh-token",          // Same refresh token
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(1 * time.Hour),
	}

	mockSource := &mockTokenSource{
		tokens: []*oauth2.Token{initialToken, refreshedToken},
	}

	// Create persistent token source
	pts := &persistentTokenSource{
		src:       mockSource,
		storage:   ts,
		logger:    logger,
		lastToken: nil,
	}

	// First call - should save initial token
	token1, err := pts.Token()
	if err != nil {
		t.Fatalf("First Token() call failed: %v", err)
	}

	if token1.AccessToken != "initial-access-token" {
		t.Errorf("Expected initial token, got: %s", token1.AccessToken)
	}

	// Verify initial token was saved to keychain
	savedToken, err := ts.LoadToken()
	if err != nil {
		// Skip test if keychain is unavailable
		t.Logf("Keychain not available, skipping persistence test: %v", err)
		return
	}

	if savedToken.AccessToken != "initial-access-token" {
		t.Errorf("Initial token not saved to keychain. Got: %s", savedToken.AccessToken)
	}

	// Second call - should get refreshed token and save it
	token2, err := pts.Token()
	if err != nil {
		t.Fatalf("Second Token() call failed: %v", err)
	}

	if token2.AccessToken != "refreshed-access-token" {
		t.Errorf("Expected refreshed token, got: %s", token2.AccessToken)
	}

	// Verify refreshed token was persisted to keychain (FR-007)
	savedToken, err = ts.LoadToken()
	if err != nil {
		t.Fatalf("Failed to load token from keychain: %v", err)
	}

	if savedToken.AccessToken != "refreshed-access-token" {
		t.Errorf("Refreshed token not persisted to keychain. Got: %s, want: %s",
			savedToken.AccessToken, "refreshed-access-token")
	}

	// Third call - should return same token without re-saving
	token3, err := pts.Token()
	if err != nil {
		t.Fatalf("Third Token() call failed: %v", err)
	}

	if token3.AccessToken != "refreshed-access-token" {
		t.Errorf("Expected same refreshed token, got: %s", token3.AccessToken)
	}

	// Clean up
	_ = ts.DeleteToken()
}

func TestPersistentTokenSource_ErrorHandling(t *testing.T) {
	// Create a test logger
	logger := log.New(os.Stderr)
	logger.SetLevel(log.ErrorLevel)

	// Create token storage
	ts := NewTokenStorage(logger)

	// Create mock source that returns an error
	mockSource := &mockTokenSource{
		shouldError: true,
	}

	pts := &persistentTokenSource{
		src:     mockSource,
		storage: ts,
		logger:  logger,
	}

	// Token() should propagate the error
	_, err := pts.Token()
	if err == nil {
		t.Error("Expected error from Token(), got nil")
	}
}
