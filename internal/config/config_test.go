package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MasterFolderName != "Meeting Notes" {
		t.Errorf("expected MasterFolderName 'Meeting Notes', got %s", cfg.MasterFolderName)
	}

	if cfg.DaysToLookBack != 1 {
		t.Errorf("expected DaysToLookBack 1, got %d", cfg.DaysToLookBack)
	}

	if cfg.GeminiModel != "gemini-2.0-flash" {
		t.Errorf("expected GeminiModel 'gemini-2.0-flash', got %s", cfg.GeminiModel)
	}

	if cfg.OwnedOnly != false {
		t.Errorf("expected OwnedOnly false, got %v", cfg.OwnedOnly)
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Set up test environment
	os.Setenv("GCAL_MASTER_FOLDER_NAME", "Test Folder")
	os.Setenv("GCAL_DAYS_TO_LOOK_BACK", "14")
	os.Setenv("GEMINI_API_KEY", "test-api-key")
	defer func() {
		os.Unsetenv("GCAL_MASTER_FOLDER_NAME")
		os.Unsetenv("GCAL_DAYS_TO_LOOK_BACK")
		os.Unsetenv("GEMINI_API_KEY")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.MasterFolderName != "Test Folder" {
		t.Errorf("expected MasterFolderName 'Test Folder', got %s", cfg.MasterFolderName)
	}

	if cfg.GeminiAPIKey != "test-api-key" {
		t.Errorf("expected GeminiAPIKey 'test-api-key', got %s", cfg.GeminiAPIKey)
	}
}

func TestLoadOwnedOnlyFromEnv(t *testing.T) {
	os.Setenv("GCAL_OWNED_ONLY", "true")
	defer os.Unsetenv("GCAL_OWNED_ONLY")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if !cfg.OwnedOnly {
		t.Errorf("expected OwnedOnly true when GCAL_OWNED_ONLY=true, got %v", cfg.OwnedOnly)
	}
}

// ---------- T049: LoadSecrets tests ----------

type mockSecretStore struct {
	secrets map[string]string
}

func (m *mockSecretStore) Get(key string) (string, error) {
	if v, ok := m.secrets[key]; ok {
		return v, nil
	}
	return "", fmt.Errorf("not found")
}

func (m *mockSecretStore) Set(key, value string) error { return nil }
func (m *mockSecretStore) Delete(key string) error     { return nil }

func TestLoadSecrets(t *testing.T) {
	tests := []struct {
		name       string
		envKey     string
		storeKey   string
		wantResult string
	}{
		{
			name:       "store overrides env",
			envKey:     "env-key-123",
			storeKey:   "store-key-456",
			wantResult: "store-key-456",
		},
		{
			name:       "env used when store empty",
			envKey:     "env-key-789",
			storeKey:   "",
			wantResult: "env-key-789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.GeminiAPIKey = tt.envKey

			store := &mockSecretStore{secrets: map[string]string{}}
			if tt.storeKey != "" {
				store.secrets["gemini-api-key"] = tt.storeKey
			}

			cfg.LoadSecrets(store)

			if cfg.GeminiAPIKey != tt.wantResult {
				t.Errorf("GeminiAPIKey: got %q, want %q", cfg.GeminiAPIKey, tt.wantResult)
			}
		})
	}
}

// ---------- T050: mustBindEnv tests ----------

func TestMustBindEnv_Valid(t *testing.T) {
	// Should not panic for valid keys
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("mustBindEnv panicked unexpectedly: %v", r)
		}
	}()
	mustBindEnv("test_key_valid_12345", "TEST_KEY_VALID_12345")
}

// TestConfigLoad_NoKeyring verifies that GCAL_NO_KEYRING=true causes
// cfg.NoKeyring to be true after Load().
func TestConfigLoad_NoKeyring(t *testing.T) {
	os.Setenv("GCAL_NO_KEYRING", "true")
	defer os.Unsetenv("GCAL_NO_KEYRING")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if !cfg.NoKeyring {
		t.Error("expected NoKeyring=true when GCAL_NO_KEYRING=true, got false")
	}
}

// ---------- T029: DefaultConfig sets Decisions.ExportDir ----------

func TestDefaultConfig_DecisionsExportDir(t *testing.T) {
	cfg := DefaultConfig()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("could not get home dir: %v", err)
	}

	want := filepath.Join(home, ".gcal-organizer", "decisions")
	if cfg.Decisions.ExportDir != want {
		t.Errorf("Decisions.ExportDir: got %q, want %q", cfg.Decisions.ExportDir, want)
	}
}

// ---------- T030: Tilde expansion for Decisions.ExportDir ----------

func TestLoad_DecisionsExportDir_TildeExpansion(t *testing.T) {
	os.Setenv("GCAL_DECISIONS_EXPORT_DIR", "~/custom-decisions")
	defer os.Unsetenv("GCAL_DECISIONS_EXPORT_DIR")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Should be expanded to an absolute path
	if strings.HasPrefix(cfg.Decisions.ExportDir, "~/") {
		t.Errorf("Decisions.ExportDir should be expanded, got %q", cfg.Decisions.ExportDir)
	}

	home, _ := os.UserHomeDir()
	want := filepath.Join(home, "custom-decisions")
	if cfg.Decisions.ExportDir != want {
		t.Errorf("Decisions.ExportDir: got %q, want %q", cfg.Decisions.ExportDir, want)
	}
}

// ---------- T05: DefaultConfig sets Decisions.Meetings to nil ----------

func TestDefaultConfig_DecisionsMeetingsNil(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Decisions.Meetings != nil {
		t.Errorf("Decisions.Meetings should be nil by default, got %v", cfg.Decisions.Meetings)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		apiKey    string
		wantError bool
	}{
		{"empty key", "", true},
		{"valid key", "test-key", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.GeminiAPIKey = tt.apiKey
			err := cfg.Validate()

			if tt.wantError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}
