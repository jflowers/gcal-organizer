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
