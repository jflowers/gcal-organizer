// Package ux provides user-facing error types with actionable fix suggestions.
package ux

import "fmt"

// ActionError is an error that includes a fix suggestion for the user.
type ActionError struct {
	Message string
	Fix     string
}

func (e *ActionError) Error() string {
	if e.Fix == "" {
		return fmt.Sprintf("❌ Error: %s", e.Message)
	}
	return fmt.Sprintf("❌ Error: %s\n   Fix:   %s", e.Message, e.Fix)
}

// NewError creates an ActionError with a message and fix.
func NewError(message, fix string) *ActionError {
	return &ActionError{Message: message, Fix: fix}
}

// --- Common error constructors ---

// MissingCredentials returns an error for missing OAuth credentials file.
func MissingCredentials(path string) *ActionError {
	return &ActionError{
		Message: fmt.Sprintf("credentials.json not found at %s", path),
		Fix:     "Run 'gcal-organizer init' or download from https://console.cloud.google.com/apis/credentials",
	}
}

// MissingAPIKey returns an error for missing Gemini API key.
func MissingAPIKey() *ActionError {
	return &ActionError{
		Message: "GEMINI_API_KEY is not set",
		Fix:     "Run 'gcal-organizer init' to configure your API key, or set the GEMINI_API_KEY environment variable",
	}
}

// TokenExpired returns an error for expired OAuth token.
func TokenExpired() *ActionError {
	return &ActionError{
		Message: "OAuth token is expired or invalid",
		Fix:     "Run 'gcal-organizer auth login' to re-authenticate",
	}
}

// MissingToken returns an error for missing OAuth token.
func MissingToken() *ActionError {
	return &ActionError{
		Message: "not authenticated — no OAuth token found",
		Fix:     "Run 'gcal-organizer auth login' to authenticate with Google",
	}
}

// OAuthSetupFailed returns an error for OAuth client creation failures.
func OAuthSetupFailed(credPath string) *ActionError {
	return &ActionError{
		Message: fmt.Sprintf("failed to read OAuth credentials from %s", credPath),
		Fix:     "Run 'gcal-organizer init' or download credentials from https://console.cloud.google.com/apis/credentials",
	}
}

// AuthFailed returns an error when authentication fails.
func AuthFailed() *ActionError {
	return &ActionError{
		Message: "authentication with Google failed",
		Fix:     "Run 'gcal-organizer auth login' to re-authenticate, then 'gcal-organizer doctor' for diagnostics",
	}
}
