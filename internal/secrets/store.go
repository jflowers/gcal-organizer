// Package secrets provides a SecretStore abstraction for credential storage.
// It supports two backends: OS credential store (KeychainStore) and
// file-based storage (FileStore) for headless/fallback environments.
package secrets

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/jflowers/gcal-organizer/internal/logging"
	"github.com/zalando/go-keyring"
)

// ServiceName is the reverse-DNS identifier used as the keychain service name.
const ServiceName = "com.jflowers.gcal-organizer"

// Well-known keys for stored secrets.
const (
	KeyOAuthToken        = "oauth-token"
	KeyGeminiAPIKey      = "gemini-api-key"
	KeyClientCredentials = "credentials-json"
)

// probeKey is the sentinel key used to detect keychain availability.
const probeKey = "__gcal_organizer_probe__"

// ErrNotFound is returned when a requested secret does not exist in the store.
var ErrNotFound = errors.New("secret not found")

// Backend indicates which storage backend is active.
type Backend int

const (
	// BackendKeychain indicates the OS credential store is in use.
	BackendKeychain Backend = iota
	// BackendFile indicates file-based storage is in use.
	BackendFile
)

// String returns a human-readable name for the backend.
func (b Backend) String() string {
	switch b {
	case BackendKeychain:
		return "OS keychain"
	case BackendFile:
		return "plaintext files"
	default:
		return "unknown"
	}
}

// SecretStore is the interface for credential storage operations.
type SecretStore interface {
	// Get retrieves a secret by key. Returns ErrNotFound if absent.
	Get(key string) (string, error)
	// Set stores or overwrites a secret by key.
	Set(key, value string) error
	// Delete removes a secret by key. No error if already absent.
	Delete(key string) error
}

// defaultConfigDir returns the default config directory (~/.gcal-organizer).
func defaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".gcal-organizer"
	}
	return filepath.Join(home, ".gcal-organizer")
}

// NewStore creates a SecretStore, preferring the OS keychain unless noKeyring
// is true or the keychain is unavailable. It returns the store and which
// backend was selected.
func NewStore(noKeyring bool) (SecretStore, Backend) {
	configDir := defaultConfigDir()

	if noKeyring {
		logging.Logger.Info("Secret storage: plaintext files (--no-keyring)")
		return &FileStore{ConfigDir: configDir}, BackendFile
	}

	// Probe keychain availability with a write/read/delete cycle.
	if err := keyring.Set(ServiceName, probeKey, "probe"); err != nil {
		logging.Logger.Warn("OS keychain unavailable, falling back to file-based storage", "error", err)
		return &FileStore{ConfigDir: configDir}, BackendFile
	}
	if _, err := keyring.Get(ServiceName, probeKey); err != nil {
		_ = keyring.Delete(ServiceName, probeKey) // best-effort cleanup
		logging.Logger.Warn("OS keychain unavailable, falling back to file-based storage", "error", err)
		return &FileStore{ConfigDir: configDir}, BackendFile
	}
	_ = keyring.Delete(ServiceName, probeKey) // best-effort cleanup

	logging.Logger.Info("Secret storage: OS keychain")
	return &KeychainStore{}, BackendKeychain
}
