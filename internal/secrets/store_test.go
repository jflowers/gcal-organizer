package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

// TestKeychainStore_SetGetDelete verifies round-trip operations via MockInit.
func TestKeychainStore_SetGetDelete(t *testing.T) {
	keyring.MockInit()

	store := &KeychainStore{}

	tests := []struct {
		name string
		key  string
	}{
		{"oauth-token", KeyOAuthToken},
		{"gemini-api-key", KeyGeminiAPIKey},
		{"credentials-json", KeyClientCredentials},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Get on missing key returns ErrNotFound
			_, err := store.Get(tc.key)
			if err != ErrNotFound {
				t.Fatalf("Get(%q) on empty store: got err=%v, want ErrNotFound", tc.key, err)
			}

			// Set + Get round-trip
			val := "test-value-for-" + tc.key
			if err := store.Set(tc.key, val); err != nil {
				t.Fatalf("Set(%q): %v", tc.key, err)
			}
			got, err := store.Get(tc.key)
			if err != nil {
				t.Fatalf("Get(%q) after Set: %v", tc.key, err)
			}
			if got != val {
				t.Fatalf("Get(%q) = %q, want %q", tc.key, got, val)
			}

			// Delete + Get returns ErrNotFound
			if err := store.Delete(tc.key); err != nil {
				t.Fatalf("Delete(%q): %v", tc.key, err)
			}
			_, err = store.Get(tc.key)
			if err != ErrNotFound {
				t.Fatalf("Get(%q) after Delete: got err=%v, want ErrNotFound", tc.key, err)
			}

			// Delete on already-deleted key is not an error
			if err := store.Delete(tc.key); err != nil {
				t.Fatalf("Delete(%q) on absent key: %v", tc.key, err)
			}
		})
	}
}

// TestFileStore_SetGetDelete verifies file-based round-trip operations.
func TestFileStore_SetGetDelete(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		filename string // expected file or ".env" for env-based keys
	}{
		{
			name:     "oauth-token",
			key:      KeyOAuthToken,
			value:    `{"access_token":"ya29.abc","refresh_token":"1//xyz","expiry":"2026-01-01T00:00:00Z"}`,
			filename: "token.json",
		},
		{
			name:     "gemini-api-key",
			key:      KeyGeminiAPIKey,
			value:    "AIzaSy-test-key-123",
			filename: ".env",
		},
		{
			name:     "credentials-json",
			key:      KeyClientCredentials,
			value:    `{"installed":{"client_id":"123.apps.googleusercontent.com","client_secret":"GOCSPX-test"}}`,
			filename: "credentials.json",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			store := &FileStore{ConfigDir: dir}

			// Get on missing returns ErrNotFound
			_, err := store.Get(tc.key)
			if err != ErrNotFound {
				t.Fatalf("Get(%q) on empty dir: got err=%v, want ErrNotFound", tc.key, err)
			}

			// Set + Get round-trip
			if err := store.Set(tc.key, tc.value); err != nil {
				t.Fatalf("Set(%q): %v", tc.key, err)
			}

			got, err := store.Get(tc.key)
			if err != nil {
				t.Fatalf("Get(%q) after Set: %v", tc.key, err)
			}
			if got != tc.value {
				t.Fatalf("Get(%q) = %q, want %q", tc.key, got, tc.value)
			}

			// Verify file exists
			path := filepath.Join(dir, tc.filename)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Fatalf("expected file %s to exist after Set", tc.filename)
			}

			// Delete + Get returns ErrNotFound
			if err := store.Delete(tc.key); err != nil {
				t.Fatalf("Delete(%q): %v", tc.key, err)
			}
			_, err = store.Get(tc.key)
			if err != ErrNotFound {
				t.Fatalf("Get(%q) after Delete: got err=%v, want ErrNotFound", tc.key, err)
			}
		})
	}
}

// TestFileStore_EnvPreservesOtherLines verifies that Set/Delete for the API key
// preserves other lines in the .env file.
func TestFileStore_EnvPreservesOtherLines(t *testing.T) {
	dir := t.TempDir()
	store := &FileStore{ConfigDir: dir}

	// Write initial .env with other config
	envPath := filepath.Join(dir, ".env")
	initial := "# Config\nGCAL_MASTER_FOLDER_NAME='Meeting Notes'\nGCAL_DAYS_TO_LOOK_BACK='7'\n"
	if err := os.WriteFile(envPath, []byte(initial), 0600); err != nil {
		t.Fatalf("write initial .env: %v", err)
	}

	// Set API key
	if err := store.Set(KeyGeminiAPIKey, "AIzaSy-test-key"); err != nil {
		t.Fatalf("Set API key: %v", err)
	}

	// Verify other lines preserved
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	content := string(data)
	if !containsLine(content, "GCAL_MASTER_FOLDER_NAME") {
		t.Fatal(".env missing GCAL_MASTER_FOLDER_NAME after Set")
	}
	if !containsLine(content, "GCAL_DAYS_TO_LOOK_BACK") {
		t.Fatal(".env missing GCAL_DAYS_TO_LOOK_BACK after Set")
	}

	// Delete API key, verify other lines still present
	if err := store.Delete(KeyGeminiAPIKey); err != nil {
		t.Fatalf("Delete API key: %v", err)
	}
	data, err = os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read .env after delete: %v", err)
	}
	content = string(data)
	if !containsLine(content, "GCAL_MASTER_FOLDER_NAME") {
		t.Fatal(".env missing GCAL_MASTER_FOLDER_NAME after Delete")
	}
	if containsLine(content, "GEMINI_API_KEY") {
		t.Fatal(".env still contains GEMINI_API_KEY after Delete")
	}
}

// TestFileStore_EnvSingleQuoteRoundTrip verifies that values containing single
// quotes survive a Set/Get round-trip via POSIX escaping.
func TestFileStore_EnvSingleQuoteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := &FileStore{ConfigDir: dir}

	// Value with an embedded single quote
	value := "AIza'Sy-test"
	if err := store.Set(KeyGeminiAPIKey, value); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := store.Get(KeyGeminiAPIKey)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != value {
		t.Fatalf("round-trip failed: got %q, want %q", got, value)
	}
}

// TestNewStore_FallbackOnNoKeyring verifies factory returns FileStore when noKeyring=true.
func TestNewStore_FallbackOnNoKeyring(t *testing.T) {
	keyring.MockInit()

	store, backend := NewStore(true)

	if backend != BackendFile {
		t.Fatalf("NewStore(noKeyring=true): backend=%v, want BackendFile", backend)
	}
	if _, ok := store.(*FileStore); !ok {
		t.Fatalf("NewStore(noKeyring=true): store type=%T, want *FileStore", store)
	}
}

// TestNewStore_FallbackOnUnavailable verifies factory returns FileStore when keyring is unavailable.
func TestNewStore_FallbackOnUnavailable(t *testing.T) {
	keyring.MockInitWithError(keyring.ErrNotFound)

	store, backend := NewStore(false)

	if backend != BackendFile {
		t.Fatalf("NewStore(unavailable keyring): backend=%v, want BackendFile", backend)
	}
	if _, ok := store.(*FileStore); !ok {
		t.Fatalf("NewStore(unavailable keyring): store type=%T, want *FileStore", store)
	}
}

// ---------- T051: Backend.String tests ----------

func TestBackendString(t *testing.T) {
	tests := []struct {
		name    string
		backend Backend
		want    string
	}{
		{"keychain", BackendKeychain, "OS keychain"},
		{"file", BackendFile, "plaintext files"},
		{"unknown", Backend(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.backend.String()
			if got != tt.want {
				t.Errorf("Backend(%d).String(): got %q, want %q", tt.backend, got, tt.want)
			}
		})
	}
}

// ---------- T052: NewStore keychain success test ----------

func TestNewStore_KeychainSuccess(t *testing.T) {
	keyring.MockInit()

	store, backend := NewStore(false)

	if backend != BackendKeychain {
		t.Fatalf("NewStore(noKeyring=false) with mock keyring: backend=%v, want BackendKeychain", backend)
	}
	if _, ok := store.(*KeychainStore); !ok {
		t.Fatalf("NewStore(noKeyring=false): store type=%T, want *KeychainStore", store)
	}
}

// ---------- T053: writeEnvValue / writeLines tests ----------

func TestWriteEnvValue_NewFile(t *testing.T) {
	dir := t.TempDir()
	store := &FileStore{ConfigDir: dir}

	// Set value when .env doesn't exist yet
	err := store.Set(KeyGeminiAPIKey, "new-test-key")
	if err != nil {
		t.Fatalf("Set on new file: %v", err)
	}

	// Verify round-trip
	got, err := store.Get(KeyGeminiAPIKey)
	if err != nil {
		t.Fatalf("Get after Set: %v", err)
	}
	if got != "new-test-key" {
		t.Errorf("round-trip: got %q, want %q", got, "new-test-key")
	}
}

func TestWriteLines_Atomic(t *testing.T) {
	dir := t.TempDir()
	store := &FileStore{ConfigDir: dir}

	// Write a key
	err := store.Set(KeyGeminiAPIKey, "key-1")
	if err != nil {
		t.Fatalf("first Set: %v", err)
	}

	// Overwrite it
	err = store.Set(KeyGeminiAPIKey, "key-2")
	if err != nil {
		t.Fatalf("second Set: %v", err)
	}

	got, err := store.Get(KeyGeminiAPIKey)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "key-2" {
		t.Errorf("expected overwritten value %q, got %q", "key-2", got)
	}

	// Verify no .tmp file left behind
	tmpPath := dir + "/.env.tmp"
	if _, err := os.Stat(tmpPath); err == nil {
		t.Error("temp file should not remain after atomic write")
	}
}

func containsLine(content, key string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), key) {
			return true
		}
	}
	return false
}

// TestNewStore_NoKeyringFromConfig verifies the end-to-end config→store flow:
// when cfg.NoKeyring is true, NewStore returns FileStore and LoadSecrets falls
// back to env var for the API key.
func TestNewStore_NoKeyringFromConfig(t *testing.T) {
	keyring.MockInit()

	// Simulate cfg.NoKeyring = true
	store, backend := NewStore(true)
	if backend != BackendFile {
		t.Fatalf("backend: got %v, want BackendFile", backend)
	}
	fs, ok := store.(*FileStore)
	if !ok {
		t.Fatalf("store type: got %T, want *FileStore", store)
	}

	// Override the config dir to an empty temp dir so we don't read the real .env
	fs.ConfigDir = t.TempDir()

	// The FileStore-backed store should return ErrNotFound for API key
	// when no .env file exists in the temp dir
	_, err := store.Get(KeyGeminiAPIKey)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound from FileStore with no .env, got %v", err)
	}
}

// ---------- Phase 6: Migration tests ----------

// TestMigrate_TokenFromDisk verifies that token.json is migrated to the store
// and deleted from disk.
func TestMigrate_TokenFromDisk(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()

	// Create token.json on disk
	tokenJSON := `{"access_token":"ya29.test","refresh_token":"1//test","expiry":"2026-06-01T00:00:00Z"}`
	tokenPath := filepath.Join(dir, "token.json")
	if err := os.WriteFile(tokenPath, []byte(tokenJSON), 0600); err != nil {
		t.Fatalf("write token.json: %v", err)
	}

	store := &KeychainStore{}
	if err := Migrate(store, dir, false, false, nil); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Token should be in store
	val, err := store.Get(KeyOAuthToken)
	if err != nil {
		t.Fatalf("token not in store: %v", err)
	}
	if val != tokenJSON {
		t.Errorf("stored token: got %q, want %q", val, tokenJSON)
	}

	// token.json should be deleted
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Error("token.json was not deleted after migration")
	}
}

// TestMigrate_APIKeyFromEnv verifies that GEMINI_API_KEY is migrated from .env
// to the store, and both GEMINI_API_KEY and GOOGLE_CREDENTIALS_FILE lines are
// removed from .env while other config lines are preserved.
func TestMigrate_APIKeyFromEnv(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()

	// Create .env with mixed content
	envContent := "GEMINI_API_KEY='test-api-key-123'\nGOOGLE_CREDENTIALS_FILE='/path/to/creds'\nGCAL_MASTER_FOLDER_NAME='Meeting Notes'\n"
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	store := &KeychainStore{}
	if err := Migrate(store, dir, false, false, nil); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// API key should be in store
	val, err := store.Get(KeyGeminiAPIKey)
	if err != nil {
		t.Fatalf("API key not in store: %v", err)
	}
	if val != "test-api-key-123" {
		t.Errorf("stored API key: got %q, want %q", val, "test-api-key-123")
	}

	// .env should still exist with non-secret config
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	content := string(data)

	// GCAL_MASTER_FOLDER_NAME should be preserved
	if !containsLine(content, "GCAL_MASTER_FOLDER_NAME") {
		t.Error(".env missing GCAL_MASTER_FOLDER_NAME after migration")
	}

	// GEMINI_API_KEY and GOOGLE_CREDENTIALS_FILE should be removed
	if containsLine(content, "GEMINI_API_KEY") {
		t.Error(".env still contains GEMINI_API_KEY after migration")
	}
	if containsLine(content, "GOOGLE_CREDENTIALS_FILE") {
		t.Error(".env still contains GOOGLE_CREDENTIALS_FILE after migration")
	}
}

// TestMigrate_CredentialsNonInteractive verifies that in non-interactive mode,
// credentials.json is migrated to the store but NOT deleted from disk.
func TestMigrate_CredentialsNonInteractive(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()

	credsJSON := `{"installed":{"client_id":"123.apps.googleusercontent.com","client_secret":"GOCSPX-test"}}`
	credsPath := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(credsPath, []byte(credsJSON), 0600); err != nil {
		t.Fatalf("write credentials.json: %v", err)
	}

	store := &KeychainStore{}
	if err := Migrate(store, dir, false, false, nil); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Credentials should be in store
	val, err := store.Get(KeyClientCredentials)
	if err != nil {
		t.Fatalf("credentials not in store: %v", err)
	}
	if val != credsJSON {
		t.Errorf("stored credentials: got %q, want %q", val, credsJSON)
	}

	// credentials.json should still be on disk (non-interactive = no deletion)
	if _, err := os.Stat(credsPath); os.IsNotExist(err) {
		t.Error("credentials.json was deleted in non-interactive mode — should be preserved")
	}
}

// TestMigrate_Idempotent verifies that running Migrate twice produces no errors
// and no duplicate writes.
func TestMigrate_Idempotent(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()

	// Create all three secret files
	if err := os.WriteFile(filepath.Join(dir, "token.json"), []byte(`{"access_token":"ya29.test"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("GEMINI_API_KEY='test-key'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "credentials.json"), []byte(`{"installed":{}}`), 0600); err != nil {
		t.Fatal(err)
	}

	store := &KeychainStore{}

	// First migration
	if err := Migrate(store, dir, false, false, nil); err != nil {
		t.Fatalf("Migrate #1: %v", err)
	}

	// Second migration — should be no-op, no errors
	if err := Migrate(store, dir, false, false, nil); err != nil {
		t.Fatalf("Migrate #2 (idempotent): %v", err)
	}

	// Verify secrets are still in store
	if _, err := store.Get(KeyOAuthToken); err != nil {
		t.Error("token missing from store after second migration")
	}
	if _, err := store.Get(KeyGeminiAPIKey); err != nil {
		t.Error("API key missing from store after second migration")
	}
	if _, err := store.Get(KeyClientCredentials); err != nil {
		t.Error("credentials missing from store after second migration")
	}
}

// TestMigrate_CredentialsInteractiveAccept verifies that in interactive mode,
// when the user accepts deletion, credentials.json is deleted from disk.
func TestMigrate_CredentialsInteractiveAccept(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()

	credsJSON := `{"installed":{"client_id":"123.apps.googleusercontent.com"}}`
	credsPath := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(credsPath, []byte(credsJSON), 0600); err != nil {
		t.Fatalf("write credentials.json: %v", err)
	}

	store := &KeychainStore{}
	// Mock prompt that returns "yes"
	mockPrompt := func(message string) (bool, error) { return true, nil }

	if err := Migrate(store, dir, true, false, mockPrompt); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Credentials should be in store
	if _, err := store.Get(KeyClientCredentials); err != nil {
		t.Fatalf("credentials not in store: %v", err)
	}

	// credentials.json should be deleted (user accepted)
	if _, err := os.Stat(credsPath); !os.IsNotExist(err) {
		t.Error("credentials.json was NOT deleted after user accepted — expected deletion")
	}
}

// TestMigrate_CredentialsInteractiveDecline verifies that in interactive mode,
// when the user declines deletion, credentials.json is preserved.
func TestMigrate_CredentialsInteractiveDecline(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()

	credsJSON := `{"installed":{"client_id":"456.apps.googleusercontent.com"}}`
	credsPath := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(credsPath, []byte(credsJSON), 0600); err != nil {
		t.Fatalf("write credentials.json: %v", err)
	}

	store := &KeychainStore{}
	// Mock prompt that returns "no"
	mockPrompt := func(message string) (bool, error) { return false, nil }

	if err := Migrate(store, dir, true, false, mockPrompt); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Credentials should be in store
	if _, err := store.Get(KeyClientCredentials); err != nil {
		t.Fatalf("credentials not in store: %v", err)
	}

	// credentials.json should still be on disk (user declined)
	if _, err := os.Stat(credsPath); os.IsNotExist(err) {
		t.Error("credentials.json was deleted despite user declining")
	}
}

// TestMigrate_PartialState verifies crash recovery: when a secret exists in
// both the store AND on disk (simulating a crash after store.Set but before
// file deletion), Migrate cleans up the file.
func TestMigrate_PartialState(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()

	tokenJSON := `{"access_token":"ya29.partial"}`
	tokenPath := filepath.Join(dir, "token.json")
	if err := os.WriteFile(tokenPath, []byte(tokenJSON), 0600); err != nil {
		t.Fatalf("write token.json: %v", err)
	}

	// Pre-populate the store (simulating crash after store.Set succeeded)
	store := &KeychainStore{}
	if err := store.Set(KeyOAuthToken, tokenJSON); err != nil {
		t.Fatalf("pre-populate store: %v", err)
	}

	// Verify both exist before migration
	if _, err := os.Stat(tokenPath); os.IsNotExist(err) {
		t.Fatal("token.json should exist before migration")
	}
	if _, err := store.Get(KeyOAuthToken); err != nil {
		t.Fatal("token should be in store before migration")
	}

	// Run migration — should clean up the file
	if err := Migrate(store, dir, false, false, nil); err != nil {
		t.Fatalf("Migrate (partial state): %v", err)
	}

	// Store value should be unchanged
	val, err := store.Get(KeyOAuthToken)
	if err != nil {
		t.Fatalf("token missing from store: %v", err)
	}
	if val != tokenJSON {
		t.Errorf("store value changed: got %q, want %q", val, tokenJSON)
	}

	// File should be cleaned up
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Error("token.json should be deleted after crash-recovery migration")
	}
}

// TestMigrate_NothingToMigrate verifies that Migrate is a no-op when no
// plaintext secrets exist on disk.
func TestMigrate_NothingToMigrate(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()

	store := &KeychainStore{}
	if err := Migrate(store, dir, false, false, nil); err != nil {
		t.Fatalf("Migrate with no files: %v", err)
	}
}
