package secrets

import (
	"fmt"

	"github.com/zalando/go-keyring"
)

// KeychainStore implements SecretStore using the OS credential store
// (macOS Keychain, Linux Secret Service) via the go-keyring library.
type KeychainStore struct{}

// Get retrieves a secret from the OS keychain.
func (k *KeychainStore) Get(key string) (string, error) {
	val, err := keyring.Get(ServiceName, key)
	if err != nil {
		if err == keyring.ErrNotFound {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("keychain get %q: %w", key, err)
	}
	return val, nil
}

// Set stores a secret in the OS keychain.
func (k *KeychainStore) Set(key, value string) error {
	if err := keyring.Set(ServiceName, key, value); err != nil {
		return fmt.Errorf("keychain set %q: %w", key, err)
	}
	return nil
}

// Delete removes a secret from the OS keychain. No error if already absent.
func (k *KeychainStore) Delete(key string) error {
	err := keyring.Delete(ServiceName, key)
	if err != nil && err != keyring.ErrNotFound {
		return fmt.Errorf("keychain delete %q: %w", key, err)
	}
	return nil
}
