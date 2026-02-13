package auth

import (
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
