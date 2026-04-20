package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ---------- T08: ParseDotEnv tests ----------

func TestParseDotEnv(t *testing.T) {
	tests := []struct {
		name    string
		content string
		home    string
		want    map[string]string
	}{
		{
			name:    "basic key-value pairs",
			content: "MY_KEY=my_value\nOTHER_KEY=other_value\n",
			home:    "/fake-home",
			want:    map[string]string{"MY_KEY": "my_value", "OTHER_KEY": "other_value"},
		},
		{
			name:    "double-quoted value",
			content: `MY_KEY="hello world"` + "\n",
			home:    "/fake-home",
			want:    map[string]string{"MY_KEY": "hello world"},
		},
		{
			name:    "single-quoted value",
			content: "MY_KEY='hello world'\n",
			home:    "/fake-home",
			want:    map[string]string{"MY_KEY": "hello world"},
		},
		{
			name:    "comments and blank lines skipped",
			content: "# comment\n\n  \nACTUAL_KEY=value\n",
			home:    "/fake-home",
			want:    map[string]string{"ACTUAL_KEY": "value"},
		},
		{
			name:    "tilde expansion",
			content: "PATH_KEY=~/config\n",
			home:    "/fake-home",
			want:    map[string]string{"PATH_KEY": "/fake-home/config"},
		},
		{
			name:    "malformed lines skipped",
			content: "NO_EQUALS\n123INVALID=value\nVALID_KEY=ok\n",
			home:    "/fake-home",
			want:    map[string]string{"VALID_KEY": "ok"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			envPath := filepath.Join(dir, ".env")
			if err := os.WriteFile(envPath, []byte(tt.content), 0600); err != nil {
				t.Fatalf("write .env: %v", err)
			}

			got, err := ParseDotEnv(envPath, tt.home)
			if err != nil {
				t.Fatalf("ParseDotEnv() error: %v", err)
			}

			for k, wantV := range tt.want {
				if gotV, ok := got[k]; !ok {
					t.Errorf("missing key %q", k)
				} else if gotV != wantV {
					t.Errorf("key %q: got %q, want %q", k, gotV, wantV)
				}
			}

			// Ensure no extra keys
			for k := range got {
				if _, ok := tt.want[k]; !ok {
					t.Errorf("unexpected key %q with value %q", k, got[k])
				}
			}
		})
	}
}

func TestParseDotEnv_MissingFile(t *testing.T) {
	_, err := ParseDotEnv("/nonexistent/path/.env", "/home")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

// ---------- T08: MigrateEnvToYAML tests ----------

func TestMigrateEnvToYAML_BasicMigration(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	yamlPath := filepath.Join(dir, "config.yaml")

	envContent := `GCAL_MASTER_FOLDER_NAME="My Notes"
GCAL_DAYS_TO_LOOK_BACK=7
GCAL_FILENAME_KEYWORDS="Notes,Meeting,Sync"
GEMINI_MODEL="gemini-2.0-flash"
`
	if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	if err := MigrateEnvToYAML(envPath, yamlPath, "/fake-home"); err != nil {
		t.Fatalf("MigrateEnvToYAML() error: %v", err)
	}

	// Verify config.yaml was created
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "master_folder_name: My Notes") {
		t.Errorf("config.yaml missing master_folder_name\n%s", content)
	}
	if !strings.Contains(content, "days_to_look_back: 7") {
		t.Errorf("config.yaml missing days_to_look_back\n%s", content)
	}
	if !strings.Contains(content, "gemini_model: gemini-2.0-flash") {
		t.Errorf("config.yaml missing gemini_model\n%s", content)
	}

	// Verify keywords are a YAML list
	var cfg yamlConfig
	// Strip header comment lines for parsing
	yamlData := data
	if err := yaml.Unmarshal(yamlData, &cfg); err != nil {
		t.Fatalf("unmarshal config.yaml: %v", err)
	}
	if len(cfg.FilenameKeywords) != 3 {
		t.Errorf("expected 3 filename_keywords, got %d: %v", len(cfg.FilenameKeywords), cfg.FilenameKeywords)
	}

	// Verify .env was deleted
	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Error(".env should be deleted after migration")
	}
}

func TestMigrateEnvToYAML_ExcludesSecrets(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	yamlPath := filepath.Join(dir, "config.yaml")

	envContent := `GEMINI_API_KEY="secret-key-123"
GOOGLE_CREDENTIALS_FILE="/path/to/creds.json"
GCAL_MASTER_FOLDER_NAME="Notes"
`
	if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	if err := MigrateEnvToYAML(envPath, yamlPath, "/fake-home"); err != nil {
		t.Fatalf("MigrateEnvToYAML() error: %v", err)
	}

	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}

	content := string(data)
	// FR-003: secrets must NOT appear in config.yaml
	if strings.Contains(content, "secret-key-123") {
		t.Error("config.yaml must not contain GEMINI_API_KEY value")
	}
	if strings.Contains(content, "creds.json") {
		t.Error("config.yaml must not contain GOOGLE_CREDENTIALS_FILE value")
	}
	if strings.Contains(content, "gemini_api_key") {
		t.Error("config.yaml must not contain gemini_api_key key")
	}
	// Non-secret values should be present
	if !strings.Contains(content, "master_folder_name: Notes") {
		t.Errorf("config.yaml missing non-secret value\n%s", content)
	}
}

func TestMigrateEnvToYAML_SplitsCommaKeywords(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	yamlPath := filepath.Join(dir, "config.yaml")

	envContent := `GCAL_FILENAME_KEYWORDS="Notes,Meeting"
`
	if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	if err := MigrateEnvToYAML(envPath, yamlPath, "/fake-home"); err != nil {
		t.Fatalf("MigrateEnvToYAML() error: %v", err)
	}

	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}

	var cfg yamlConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config.yaml: %v", err)
	}

	if len(cfg.FilenameKeywords) != 2 {
		t.Fatalf("expected 2 keywords, got %d: %v", len(cfg.FilenameKeywords), cfg.FilenameKeywords)
	}
	if cfg.FilenameKeywords[0] != "Notes" || cfg.FilenameKeywords[1] != "Meeting" {
		t.Errorf("keywords: got %v, want [Notes Meeting]", cfg.FilenameKeywords)
	}
}

func TestMigrateEnvToYAML_DeletesEnvAfterSuccess(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	yamlPath := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(envPath, []byte("GCAL_MASTER_FOLDER_NAME=Test\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	if err := MigrateEnvToYAML(envPath, yamlPath, "/fake-home"); err != nil {
		t.Fatalf("MigrateEnvToYAML() error: %v", err)
	}

	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Error(".env should be deleted after successful migration")
	}
}

func TestMigrateEnvToYAML_NoOpWhenEnvMissing(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	yamlPath := filepath.Join(dir, "config.yaml")

	// .env does not exist — should be a no-op
	err := MigrateEnvToYAML(envPath, yamlPath, "/fake-home")
	if err != nil {
		t.Fatalf("expected nil error for missing .env, got: %v", err)
	}

	// config.yaml should not be created
	if _, err := os.Stat(yamlPath); !os.IsNotExist(err) {
		t.Error("config.yaml should not be created when .env is missing")
	}
}

func TestMigrateEnvToYAML_DecisionsExportDir(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	yamlPath := filepath.Join(dir, "config.yaml")

	envContent := `GCAL_DECISIONS_EXPORT_DIR="/custom/decisions"
`
	if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	if err := MigrateEnvToYAML(envPath, yamlPath, "/fake-home"); err != nil {
		t.Fatalf("MigrateEnvToYAML() error: %v", err)
	}

	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}

	var cfg yamlConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal config.yaml: %v", err)
	}

	if cfg.Decisions == nil {
		t.Fatal("decisions section should be present")
	}
	if cfg.Decisions.ExportDir != "/custom/decisions" {
		t.Errorf("decisions.export_dir: got %q, want %q", cfg.Decisions.ExportDir, "/custom/decisions")
	}
}

func TestMigrateEnvToYAML_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	yamlPath := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(envPath, []byte("GCAL_MASTER_FOLDER_NAME=Test\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	if err := MigrateEnvToYAML(envPath, yamlPath, "/fake-home"); err != nil {
		t.Fatalf("MigrateEnvToYAML() error: %v", err)
	}

	// Review finding X-1: verify 0o600 permissions
	info, err := os.Stat(yamlPath)
	if err != nil {
		t.Fatalf("stat config.yaml: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("config.yaml permissions: got %o, want 600", perm)
	}
}
