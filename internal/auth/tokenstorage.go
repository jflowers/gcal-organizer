package auth

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/charmbracelet/log"
	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

const (
	// KeyringService is the service name used in the system keychain
	KeyringService = "gcal-organizer"
	// KeyringUser is the username/key for the OAuth token
	KeyringUser = "oauth-token"
)

// TokenStorage handles secure storage and retrieval of OAuth tokens
type TokenStorage struct {
	logger *log.Logger
}

// NewTokenStorage creates a new secure token storage handler
func NewTokenStorage(logger *log.Logger) *TokenStorage {
	return &TokenStorage{
		logger: logger,
	}
}

// SaveToken saves an OAuth token securely to the system keychain
func (ts *TokenStorage) SaveToken(token *oauth2.Token) error {
	// Serialize token to JSON
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	// Save to system keychain
	err = keyring.Set(KeyringService, KeyringUser, string(data))
	if err != nil {
		return fmt.Errorf("failed to save token to keychain: %w\n\n%s", err, getKeychainSetupInstructions())
	}

	ts.logger.Debug("Token saved to system keychain")
	return nil
}

// LoadToken loads an OAuth token from the system keychain
func (ts *TokenStorage) LoadToken() (*oauth2.Token, error) {
	// Load from keyring
	data, err := keyring.Get(KeyringService, KeyringUser)
	if err != nil {
		return nil, fmt.Errorf("no token found in keychain: %w\n\nRun 'gcal-organizer auth login' to authenticate", err)
	}

	token := &oauth2.Token{}
	if err := json.Unmarshal([]byte(data), token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token from keyring: %w", err)
	}

	ts.logger.Debug("Token loaded from system keychain")
	return token, nil
}

// DeleteToken removes the stored token from the keychain
func (ts *TokenStorage) DeleteToken() error {
	err := keyring.Delete(KeyringService, KeyringUser)
	if err != nil {
		ts.logger.Debug("Failed to delete from keyring", "error", err)
	}
	return err
}

// GetStorageLocation returns a human-readable description of where the token is stored
func (ts *TokenStorage) GetStorageLocation() string {
	_, err := keyring.Get(KeyringService, KeyringUser)
	if err == nil {
		return getKeychainName()
	}
	return "Not stored"
}

// getKeychainName returns the platform-specific keychain name
func getKeychainName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS Keychain"
	case "windows":
		return "Windows Credential Manager"
	case "linux":
		return "Linux Secret Service"
	default:
		return "System Keychain"
	}
}

// getKeychainSetupInstructions returns platform-specific setup instructions
func getKeychainSetupInstructions() string {
	switch runtime.GOOS {
	case "darwin":
		return `Keychain Setup Instructions (macOS):
  The macOS Keychain should work by default. If you're seeing this error:
  1. Open "Keychain Access" application
  2. Ensure you're logged in to your login keychain
  3. Try running the command again

  If the issue persists, you may need to unlock your keychain:
    security unlock-keychain ~/Library/Keychains/login.keychain-db`

	case "linux":
		return `Keychain Setup Instructions (Linux):
  Install a keychain/secret service daemon:

  For GNOME/Ubuntu/Fedora:
    sudo dnf install gnome-keyring          # Fedora
    sudo apt install gnome-keyring          # Ubuntu/Debian

  For KDE:
    sudo dnf install kwalletmanager         # Fedora
    sudo apt install kwalletmanager         # Ubuntu/Debian

  Then ensure the service is running:
    gnome-keyring-daemon --start           # For gnome-keyring

  Note: On headless systems, you may need to configure the keyring
  with a password or use SSH agent forwarding.`

	case "windows":
		return `Keychain Setup Instructions (Windows):
  Windows Credential Manager should work by default. If you're seeing this error:
  1. Open "Credential Manager" from Control Panel
  2. Ensure you have access to Windows Credential Manager
  3. Try running the command again as Administrator

  The credential will be stored under:
    Generic Credentials > gcal-organizer`

	default:
		return `Keychain Setup Instructions:
  Your operating system requires a keychain/credential manager.
  Please install the appropriate package for your system.`
	}
}
