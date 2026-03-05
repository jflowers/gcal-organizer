package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jflowers/gcal-organizer/internal/logging"
)

// PromptFunc is a function that asks the user a yes/no question and returns
// their answer. This allows tests to inject a mock prompt.
type PromptFunc func(message string) (bool, error)

// Migrate auto-migrates existing plaintext secrets to the SecretStore.
// It is idempotent: running it multiple times produces the same result.
//
// For each secret type:
//  1. Check if the secret is already in the store
//  2. If in store AND on disk: re-attempt disk cleanup (crash recovery)
//  3. If not in store AND on disk: read from disk → write to store → cleanup
//  4. If in store and not on disk: no-op (already migrated)
//
// Parameters:
//   - store: target SecretStore (should be KeychainStore for migration to be useful)
//   - configDir: path to ~/.gcal-organizer
//   - interactive: if true, prompt before deleting credentials.json
//   - verbose: if true, log migration details
//   - promptFn: function to prompt user (nil uses default huh prompt)
func Migrate(store SecretStore, configDir string, interactive, verbose bool, promptFn PromptFunc) error {
	if err := migrateToken(store, configDir, verbose); err != nil {
		return fmt.Errorf("migrate token: %w", err)
	}
	if err := migrateAPIKey(store, configDir, verbose); err != nil {
		return fmt.Errorf("migrate API key: %w", err)
	}
	if err := migrateCredentials(store, configDir, interactive, verbose, promptFn); err != nil {
		return fmt.Errorf("migrate credentials: %w", err)
	}
	return nil
}

// migrateToken migrates token.json to the store and deletes the file.
func migrateToken(store SecretStore, configDir string, verbose bool) error {
	tokenPath := filepath.Join(configDir, "token.json")

	inStore := false
	if _, err := store.Get(KeyOAuthToken); err == nil {
		inStore = true
	}

	onDisk := false
	tokenData, err := os.ReadFile(tokenPath)
	if err == nil {
		onDisk = true
	}

	if !inStore && onDisk {
		// Migrate: disk → store
		if err := store.Set(KeyOAuthToken, string(tokenData)); err != nil {
			return fmt.Errorf("store token: %w", err)
		}
		if verbose {
			logging.Logger.Info("Migrated OAuth token to credential store")
		}
	}

	// Cleanup: delete file if token is in store (handles both fresh migration and crash recovery)
	if onDisk {
		if _, checkErr := store.Get(KeyOAuthToken); checkErr == nil {
			if err := os.Remove(tokenPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("delete token.json: %w", err)
			}
			if verbose {
				logging.Logger.Info("Deleted token.json")
			}
		}
	}

	return nil
}

// migrateAPIKey migrates the GEMINI_API_KEY from .env to the store and strips
// both GEMINI_API_KEY and GOOGLE_CREDENTIALS_FILE lines from .env.
func migrateAPIKey(store SecretStore, configDir string, verbose bool) error {
	envPath := filepath.Join(configDir, ".env")

	inStore := false
	if _, err := store.Get(KeyGeminiAPIKey); err == nil {
		inStore = true
	}

	// Read the API key from .env using FileStore's parsing
	fs := &FileStore{ConfigDir: configDir}
	apiKey, envErr := fs.readEnvValue("GEMINI_API_KEY")
	onDisk := envErr == nil && apiKey != ""

	if !inStore && onDisk {
		// Migrate: .env → store
		if err := store.Set(KeyGeminiAPIKey, apiKey); err != nil {
			return fmt.Errorf("store API key: %w", err)
		}
		if verbose {
			logging.Logger.Info("Migrated Gemini API key to credential store")
		}
	}

	// Cleanup: strip lines from .env if key is in store (handles fresh migration and crash recovery)
	if _, checkErr := store.Get(KeyGeminiAPIKey); checkErr == nil {
		if err := stripEnvLines(envPath, verbose); err != nil {
			return err
		}
	}

	return nil
}

// secretKeys lists the .env variable names that are considered secrets and
// should be stripped after migration to the credential store.
var secretKeys = map[string]bool{
	"GEMINI_API_KEY":          true,
	"GOOGLE_CREDENTIALS_FILE": true,
}

// stripEnvLines removes GEMINI_API_KEY and GOOGLE_CREDENTIALS_FILE lines from
// .env, along with any immediately preceding comment/blank lines that describe
// the removed secret (orphaned headers). Preserves all other content. Uses
// atomic write (temp file + rename).
func stripEnvLines(envPath string, verbose bool) error {
	data, err := os.ReadFile(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no .env to clean up
		}
		return fmt.Errorf("read .env: %w", err)
	}

	lines := strings.Split(string(data), "\n")

	// First pass: identify which lines are secret value lines.
	isSecretLine := make([]bool, len(lines))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 && secretKeys[strings.TrimSpace(parts[0])] {
			isSecretLine[i] = true
		}
	}

	// Second pass: mark comment/blank lines immediately above secret lines
	// as orphaned (they describe the removed secret).
	remove := make([]bool, len(lines))
	for i, isSec := range isSecretLine {
		if !isSec {
			continue
		}
		remove[i] = true
		// Walk backwards from the secret line, removing comment and blank lines
		// that form the header block for this secret.
		for j := i - 1; j >= 0; j-- {
			trimmed := strings.TrimSpace(lines[j])
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				remove[j] = true
			} else {
				break
			}
		}
	}

	var kept []string
	stripped := 0
	for i, line := range lines {
		if remove[i] {
			stripped++
			continue
		}
		kept = append(kept, line)
	}

	if stripped == 0 {
		return nil // nothing to strip
	}

	// Collapse runs of more than two consecutive blank lines (cosmetic cleanup).
	var collapsed []string
	blanks := 0
	for _, line := range kept {
		if strings.TrimSpace(line) == "" {
			blanks++
			if blanks > 2 {
				continue
			}
		} else {
			blanks = 0
		}
		collapsed = append(collapsed, line)
	}

	// Write back atomically
	content := strings.Join(collapsed, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	tmp := envPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0600); err != nil {
		return fmt.Errorf("write temp .env: %w", err)
	}
	if err := os.Rename(tmp, envPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename temp .env: %w", err)
	}

	if verbose {
		logging.Logger.Info("Removed secret entries from .env", "count", stripped)
	}
	return nil
}

// migrateCredentials migrates credentials.json to the store. The file is only
// deleted if the user confirms via an interactive prompt.
func migrateCredentials(store SecretStore, configDir string, interactive, verbose bool, promptFn PromptFunc) error {
	credsPath := filepath.Join(configDir, "credentials.json")

	inStore := false
	if _, err := store.Get(KeyClientCredentials); err == nil {
		inStore = true
	}

	onDisk := false
	credsData, err := os.ReadFile(credsPath)
	if err == nil {
		onDisk = true
	}

	if !inStore && onDisk {
		// Migrate: disk → store
		if err := store.Set(KeyClientCredentials, string(credsData)); err != nil {
			return fmt.Errorf("store credentials: %w", err)
		}
		if verbose {
			logging.Logger.Info("Migrated client credentials to credential store")
		}
	}

	// For crash recovery: if in store AND on disk, attempt cleanup
	if onDisk {
		if _, checkErr := store.Get(KeyClientCredentials); checkErr == nil {
			if interactive && promptFn != nil {
				ok, err := promptFn("credentials.json is now stored in the credential store.\nWould you like to delete the file? It may be shared with other tools.")
				if err != nil {
					// Prompt failed (e.g., stdin closed) — treat as non-interactive
					logging.Logger.Warn("Could not prompt for credentials.json deletion; skipping", "error", err)
				} else if ok {
					if err := os.Remove(credsPath); err != nil && !os.IsNotExist(err) {
						return fmt.Errorf("delete credentials.json: %w", err)
					}
					if verbose {
						logging.Logger.Info("Deleted credentials.json")
					}
				}
			} else if !interactive {
				logging.Logger.Info("credentials.json still on disk — manual cleanup suggested (non-interactive mode)")
			}
		}
	}

	return nil
}
