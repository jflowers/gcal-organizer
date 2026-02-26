// Package auth provides authentication for Google Workspace APIs and Gemini.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/jflowers/gcal-organizer/internal/secrets"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
)

// Scopes required for Google Workspace APIs
var Scopes = []string{
	drive.DriveScope,
	docs.DocumentsScope,
	calendar.CalendarReadonlyScope,
}

// OAuthClient handles OAuth2 authentication for Google Workspace APIs.
type OAuthClient struct {
	config            *oauth2.Config
	store             secrets.SecretStore
	credsFallbackPath string
	httpClient        *http.Client
}

// NewOAuthClient creates a new OAuth client. It loads client credentials from
// the SecretStore first, falling back to reading the file at credsFallbackPath.
func NewOAuthClient(store secrets.SecretStore, credsFallbackPath string) (*OAuthClient, error) {
	// Try loading credentials from the store first
	var b []byte
	credsJSON, err := store.Get(secrets.KeyClientCredentials)
	if err == nil && credsJSON != "" {
		b = []byte(credsJSON)
	} else {
		// Fall back to reading from the file
		b, err = os.ReadFile(credsFallbackPath)
		if err != nil {
			return nil, fmt.Errorf("unable to read credentials: %w\n\nTo set up OAuth:\n1. Go to https://console.cloud.google.com\n2. Create OAuth 2.0 credentials (Desktop app)\n3. Download and save to: %s\n\nRun 'gcal-organizer doctor' for diagnostics", err, credsFallbackPath)
		}
	}

	config, err := google.ConfigFromJSON(b, Scopes...)
	if err != nil {
		return nil, fmt.Errorf("unable to parse credentials: %w", err)
	}

	return &OAuthClient{
		config:            config,
		store:             store,
		credsFallbackPath: credsFallbackPath,
	}, nil
}

// GetClient returns an authenticated HTTP client.
// If no cached token exists, it will prompt for authorization.
// Token refresh is automatically persisted via persistingTokenSource.
func (o *OAuthClient) GetClient(ctx context.Context) (*http.Client, error) {
	if o.httpClient != nil {
		return o.httpClient, nil
	}

	tok, err := o.loadToken()
	if err != nil {
		// No saved token, need to get one
		tok, err = o.getTokenFromWeb(ctx)
		if err != nil {
			return nil, fmt.Errorf("unable to get token: %w", err)
		}
		if err := o.saveToken(tok); err != nil {
			return nil, fmt.Errorf("unable to save token: %w", err)
		}
	}

	// Wrap the token source with persistingTokenSource so refreshed tokens
	// are saved back to the store automatically.
	baseTS := o.config.TokenSource(ctx, tok)
	persistTS := &persistingTokenSource{
		base:    baseTS,
		store:   o.store,
		current: tok,
	}

	o.httpClient = oauth2.NewClient(ctx, persistTS)
	return o.httpClient, nil
}

// loadToken reads the cached OAuth token from the SecretStore.
func (o *OAuthClient) loadToken() (*oauth2.Token, error) {
	data, err := o.store.Get(secrets.KeyOAuthToken)
	if err != nil {
		return nil, err
	}

	tok := &oauth2.Token{}
	if err := json.Unmarshal([]byte(data), tok); err != nil {
		return nil, fmt.Errorf("failed to decode stored token: %w", err)
	}
	return tok, nil
}

// saveToken saves the OAuth token to the SecretStore.
func (o *OAuthClient) saveToken(token *oauth2.Token) error {
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to encode token: %w", err)
	}
	return o.store.Set(secrets.KeyOAuthToken, string(data))
}

// persistingTokenSource wraps an oauth2.TokenSource and persists refreshed
// tokens to the SecretStore. This ensures that when the oauth2 library
// automatically refreshes an expired access token, the new token is saved
// so it survives process restarts.
type persistingTokenSource struct {
	base    oauth2.TokenSource
	store   secrets.SecretStore
	current *oauth2.Token
	mu      sync.Mutex
}

// Token returns the current token or refreshes it. If the token was refreshed,
// it is persisted to the store.
func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	tok, err := p.base.Token()
	if err != nil {
		return nil, err
	}

	// Only persist if the token actually changed (new access token or new expiry)
	if p.current == nil || tok.AccessToken != p.current.AccessToken {
		data, err := json.Marshal(tok)
		if err == nil {
			_ = p.store.Set(secrets.KeyOAuthToken, string(data))
		}
		p.current = tok
	}

	return tok, nil
}

// randomState generates a cryptographically random OAuth2 state parameter.
//
// In a loopback-redirect flow the state would be compared against the value
// echoed back by Google's authorization server, providing CSRF protection.
// This CLI uses a manual copy/paste flow instead: the user copies only the
// authorization code from the redirect URL, so the state parameter is never
// automatically verified. Generating a random (non-guessable) value is still
// better than the previous static "state-token" string because it prevents
// an observer who can read the terminal from replaying a known state value.
// Full CSRF enforcement would require a loopback HTTP listener; that is a
// future enhancement tracked as a potential improvement.
func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random state: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// getTokenFromWeb starts an OAuth2 flow in the browser.
func (o *OAuthClient) getTokenFromWeb(ctx context.Context) (*oauth2.Token, error) {
	state, err := randomState()
	if err != nil {
		return nil, err
	}
	authURL := o.config.AuthCodeURL(state, oauth2.AccessTypeOffline)
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

// IsAuthenticated checks if a valid token exists in the store.
func (o *OAuthClient) IsAuthenticated() bool {
	tok, err := o.loadToken()
	if err != nil {
		return false
	}
	return tok.Valid()
}
