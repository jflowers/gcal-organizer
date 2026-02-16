// Package auth provides authentication for Google Workspace APIs and Gemini.
package auth

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/charmbracelet/log"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/tasks/v1"
)

// Scopes required for Google Workspace APIs
var Scopes = []string{
	drive.DriveScope,
	docs.DocumentsScope,
	calendar.CalendarReadonlyScope,
	tasks.TasksScope,
}

// OAuthClient handles OAuth2 authentication for Google Workspace APIs.
type OAuthClient struct {
	config       *oauth2.Config
	tokenStorage *TokenStorage
	httpClient   *http.Client
	logger       *log.Logger
}

// NewOAuthClient creates a new OAuth client from credentials file.
func NewOAuthClient(credentialsFile string, logger *log.Logger) (*OAuthClient, error) {
	b, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("unable to read credentials file: %w", err)
	}

	config, err := google.ConfigFromJSON(b, Scopes...)
	if err != nil {
		return nil, fmt.Errorf("unable to parse credentials: %w", err)
	}

	if logger == nil {
		logger = log.New(os.Stderr)
	}

	return &OAuthClient{
		config:       config,
		tokenStorage: NewTokenStorage(logger),
		logger:       logger,
	}, nil
}

// persistentTokenSource wraps an oauth2.TokenSource and persists refreshed tokens to storage.
// This ensures that when oauth2 automatically refreshes an expired token, the new token
// is saved to the keychain (FR-007).
type persistentTokenSource struct {
	src       oauth2.TokenSource
	storage   *TokenStorage
	logger    *log.Logger
	lastToken *oauth2.Token
}

// Token returns a valid token, refreshing if necessary, and persists any refreshed token.
func (pts *persistentTokenSource) Token() (*oauth2.Token, error) {
	token, err := pts.src.Token()
	if err != nil {
		return nil, err
	}

	// Check if token was refreshed (access token changed)
	// This happens when oauth2.TokenSource automatically refreshes an expired token
	if pts.lastToken == nil || token.AccessToken != pts.lastToken.AccessToken {
		pts.logger.Debug("Token refreshed, persisting to keychain")

		// Save refreshed token to keychain
		if err := pts.storage.SaveToken(token); err != nil {
			// Log error but don't fail the request - the token refresh succeeded
			// and the HTTP client can still use the in-memory token
			pts.logger.Warn("Failed to persist refreshed token to keychain", "error", err)
		} else {
			pts.logger.Debug("Refreshed token persisted to keychain")
		}

		pts.lastToken = token
	}

	return token, nil
}

// GetClient returns an authenticated HTTP client.
// If no cached token exists, it will prompt for authorization.
// The client automatically refreshes expired tokens and persists them to the keychain.
func (o *OAuthClient) GetClient(ctx context.Context) (*http.Client, error) {
	if o.httpClient != nil {
		return o.httpClient, nil
	}

	tok, err := o.tokenStorage.LoadToken()
	if err != nil {
		// No saved token, need to get one
		tok, err = o.getTokenFromWeb(ctx)
		if err != nil {
			return nil, fmt.Errorf("unable to get token: %w", err)
		}
		if err := o.tokenStorage.SaveToken(tok); err != nil {
			return nil, fmt.Errorf("unable to save token: %w", err)
		}
	}

	// Create base token source with automatic refresh capability
	baseSource := o.config.TokenSource(ctx, tok)

	// Wrap with persistent token source to save refreshed tokens to keychain
	persistentSource := &persistentTokenSource{
		src:       baseSource,
		storage:   o.tokenStorage,
		logger:    o.logger,
		lastToken: tok,
	}

	// Create HTTP client with the persistent token source
	o.httpClient = oauth2.NewClient(ctx, persistentSource)
	return o.httpClient, nil
}

// getTokenFromWeb starts an OAuth2 flow in the browser.
func (o *OAuthClient) getTokenFromWeb(ctx context.Context) (*oauth2.Token, error) {
	authURL := o.config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Print("🔗 Follow these steps to authorize gcal-organizer:\n\n")
	fmt.Print("  1. Open this URL in your browser:\n\n")
	fmt.Printf("     %v\n\n", authURL)
	fmt.Println("  2. Sign in with your Google account and click 'Allow'")
	fmt.Println("  3. You'll see a page saying \"This site can't be reached\"")
	fmt.Println("     — that's expected!")
	fmt.Println("  4. Look at the URL in your browser's address bar.")
	fmt.Println("     Find the part after 'code=' and before '&scope='")
	fmt.Print("     Copy that entire code.\n\n")
	fmt.Println("     Example URL: http://localhost/?code=4/0AXSc3g...abc&scope=...")
	fmt.Print("     The code is:                         4/0AXSc3g...abc\n\n")
	fmt.Print("📝 Paste the authorization code here: ")

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		return nil, fmt.Errorf("unable to read authorization code: %w", err)
	}

	tok, err := o.config.Exchange(ctx, authCode)
	if err != nil {
		return nil, fmt.Errorf("unable to exchange code for token: %w", err)
	}
	return tok, nil
}

// IsAuthenticated checks if a valid token exists.
func (o *OAuthClient) IsAuthenticated() bool {
	tok, err := o.tokenStorage.LoadToken()
	if err != nil {
		return false
	}
	return tok.Valid()
}

// GetTokenStorageLocation returns a human-readable description of where the token is stored.
func (o *OAuthClient) GetTokenStorageLocation() string {
	return o.tokenStorage.GetStorageLocation()
}
