package ux

import (
	"strings"
	"testing"
)

// ---------- T054: Error constructor tests ----------

func TestErrorConstructors(t *testing.T) {
	tests := []struct {
		name        string
		err         *ActionError
		wantMessage string
		wantFix     string
	}{
		{
			name:        "NewError",
			err:         NewError("test message", "test fix"),
			wantMessage: "test message",
			wantFix:     "test fix",
		},
		{
			name:        "MissingCredentials",
			err:         MissingCredentials("/path/to/creds"),
			wantMessage: "credentials.json not found",
			wantFix:     "gcal-organizer init",
		},
		{
			name:        "MissingAPIKey",
			err:         MissingAPIKey(),
			wantMessage: "GEMINI_API_KEY is not set",
			wantFix:     "GEMINI_API_KEY",
		},
		{
			name:        "TokenExpired",
			err:         TokenExpired(),
			wantMessage: "expired or invalid",
			wantFix:     "auth login",
		},
		{
			name:        "MissingToken",
			err:         MissingToken(),
			wantMessage: "not authenticated",
			wantFix:     "auth login",
		},
		{
			name:        "OAuthSetupFailed",
			err:         OAuthSetupFailed("/path/to/creds"),
			wantMessage: "failed to read OAuth credentials",
			wantFix:     "gcal-organizer init",
		},
		{
			name:        "AuthFailed",
			err:         AuthFailed(),
			wantMessage: "authentication with Google failed",
			wantFix:     "auth login",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatal("constructor returned nil")
			}

			// Contract: Message field contains expected substring
			if !strings.Contains(tt.err.Message, tt.wantMessage) {
				t.Errorf("Message: got %q, want substring %q", tt.err.Message, tt.wantMessage)
			}

			// Contract: Fix field contains expected substring
			if !strings.Contains(tt.err.Fix, tt.wantFix) {
				t.Errorf("Fix: got %q, want substring %q", tt.err.Fix, tt.wantFix)
			}

			// Contract: Error() method includes both message and fix
			errStr := tt.err.Error()
			if !strings.Contains(errStr, tt.err.Message) {
				t.Errorf("Error() should contain Message, got: %q", errStr)
			}
			if tt.err.Fix != "" && !strings.Contains(errStr, tt.err.Fix) {
				t.Errorf("Error() should contain Fix, got: %q", errStr)
			}
		})
	}
}

func TestActionError_ErrorWithoutFix(t *testing.T) {
	err := &ActionError{Message: "something broke", Fix: ""}
	errStr := err.Error()

	// Contract: when Fix is empty, Error() should not include "Fix:" label
	if strings.Contains(errStr, "Fix:") {
		t.Errorf("Error() with empty Fix should not contain 'Fix:' label, got: %q", errStr)
	}
	if !strings.Contains(errStr, "something broke") {
		t.Errorf("Error() should contain message, got: %q", errStr)
	}
}
