// Package config provides configuration management for gcal-organizer.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jflowers/gcal-organizer/internal/secrets"
	"github.com/jflowers/gcal-organizer/internal/ux"
	"github.com/spf13/viper"
)

// mustBindEnv wraps viper.BindEnv and panics on error. Configuration binding
// failures indicate a programming error (typo in key name) that should be
// caught immediately at startup.
func mustBindEnv(args ...string) {
	if err := viper.BindEnv(args...); err != nil {
		panic(fmt.Sprintf("viper.BindEnv(%v) failed: %v", args, err))
	}
}

// OllamaConfig holds configuration for the local AI integration via Ollama.
type OllamaConfig struct {
	// Enabled controls whether local AI features are active.
	// When false, all Ollama checks and features are skipped.
	Enabled bool `mapstructure:"enabled"`

	// Endpoint is the Ollama API base URL (e.g., "http://localhost:11434").
	Endpoint string `mapstructure:"endpoint"`

	// Timeout is the HTTP request timeout for generation requests, in seconds.
	Timeout int `mapstructure:"timeout"`

	// Sensitivity holds settings for the sensitivity classification gate.
	Sensitivity SensitivityConfig `mapstructure:"sensitivity"`

	// Assignments holds settings for local task assignment.
	Assignments AssignmentsConfig `mapstructure:"assignments"`

	// LocalOnly prevents all cloud AI API calls when true.
	// Decision extraction uses the local model instead of Gemini.
	LocalOnly bool `mapstructure:"local_only"`
}

// SensitivityConfig holds settings for the sensitivity classification gate.
type SensitivityConfig struct {
	// Enabled controls whether the sensitivity gate is active (FR-006).
	// Independent of the top-level OllamaConfig.Enabled toggle.
	// When false, transcripts are not screened for sensitivity.
	Enabled bool `mapstructure:"enabled"`

	// Model is the Ollama model name for sensitivity classification.
	Model string `mapstructure:"model"`

	// Threshold is the minimum score for a transcript to be classified
	// as sensitive. Comparison is >= (inclusive). Range: 0.0–1.0.
	Threshold float64 `mapstructure:"threshold"`
}

// AssignmentsConfig holds settings for local task assignment.
type AssignmentsConfig struct {
	// Model is the Ollama model name for assignee extraction.
	Model string `mapstructure:"model"`
}

// DecisionsConfig holds configuration for the decision export feature.
type DecisionsConfig struct {
	// ExportDir is the output directory for decision markdown files.
	ExportDir string `mapstructure:"export_dir"`

	// Meetings is the allowlist of meeting titles for decision export.
	// When non-empty, only meetings whose titles exactly match (case-insensitive)
	// an entry in this list will have decisions exported.
	// When empty or nil, decisions are exported for all meetings.
	Meetings []string `mapstructure:"meetings"`
}

// Config holds all configuration values for the application.
type Config struct {
	// MasterFolderName is the name of the master folder in Google Drive
	MasterFolderName string `mapstructure:"master_folder_name"`

	// DaysToLookBack is the number of days to scan for calendar events
	DaysToLookBack int `mapstructure:"days_to_look_back"`

	// FilenamePattern is the regex pattern for parsing document names
	FilenamePattern string `mapstructure:"filename_pattern"`

	// FilenameKeywords are keywords used to filter relevant documents
	FilenameKeywords []string `mapstructure:"filename_keywords"`

	// GeminiAPIKey is the GCP API key for Gemini
	GeminiAPIKey string // Not in YAML — loaded from keychain/env

	// GeminiModel is the Gemini model to use
	GeminiModel string `mapstructure:"gemini_model"`

	// CredentialsFile is the path to the Google OAuth credentials file
	CredentialsFile string // Not in YAML — loaded from keychain/env

	// TokenFile is the path to store OAuth tokens (legacy, used by FileStore)
	TokenFile string // Legacy, used by FileStore

	// Verbose enables verbose output
	Verbose bool // CLI flag only

	// DryRun prevents making changes
	DryRun bool // CLI flag only

	// OwnedOnly restricts mutations to files owned by the authenticated user
	OwnedOnly bool // CLI flag only

	// NoKeyring disables OS credential store; use file-based storage
	NoKeyring bool // CLI flag only

	// ChromeProfilePath is the path to Chrome profile for browser automation
	ChromeProfilePath string `mapstructure:"chrome_profile_path"`

	// Decisions holds configuration for the decision export feature.
	Decisions DecisionsConfig `mapstructure:"decisions"`

	// Ollama holds configuration for the local AI integration.
	Ollama OllamaConfig `mapstructure:"ollama"`
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()

	configDir := filepath.Join(home, ".gcal-organizer")

	return &Config{
		MasterFolderName:  "Meeting Notes",
		DaysToLookBack:    1,
		FilenamePattern:   `(.+)\s*-\s*(\d{4}-\d{2}-\d{2})`,
		FilenameKeywords:  []string{"Notes", "Meeting"},
		GeminiModel:       "gemini-2.0-flash",
		CredentialsFile:   filepath.Join(configDir, "credentials.json"),
		TokenFile:         filepath.Join(configDir, "token.json"),
		ChromeProfilePath: filepath.Join(configDir, "chrome-data"),
		Decisions: DecisionsConfig{
			ExportDir: filepath.Join(configDir, "decisions"),
		},
		Ollama: OllamaConfig{
			Enabled:  true,
			Endpoint: "http://localhost:11434",
			Timeout:  120,
			Sensitivity: SensitivityConfig{
				Enabled:   true,
				Model:     "granite-guardian",
				Threshold: 0.7,
			},
			Assignments: AssignmentsConfig{
				Model: "granite3.2:8b",
			},
			LocalOnly: false,
		},
	}
}

// Load reads configuration from environment variables and config file.
func Load() (*Config, error) {
	cfg := DefaultConfig()

	// Set up environment variable binding
	viper.SetEnvPrefix("GCAL")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()

	// Bind specific environment variables
	mustBindEnv("master_folder_name", "GCAL_MASTER_FOLDER_NAME")
	mustBindEnv("days_to_look_back", "GCAL_DAYS_TO_LOOK_BACK")
	mustBindEnv("filename_pattern", "GCAL_FILENAME_PATTERN")
	mustBindEnv("filename_keywords", "GCAL_FILENAME_KEYWORDS")
	mustBindEnv("gemini_api_key", "GEMINI_API_KEY")
	mustBindEnv("gemini_model", "GEMINI_MODEL")
	mustBindEnv("credentials_file", "GOOGLE_CREDENTIALS_FILE")
	mustBindEnv("owned-only", "GCAL_OWNED_ONLY")
	mustBindEnv("no-keyring", "GCAL_NO_KEYRING")
	mustBindEnv("decisions.export_dir", "GCAL_DECISIONS_EXPORT_DIR")
	mustBindEnv("decisions.meetings", "GCAL_DECISIONS_MEETINGS")
	mustBindEnv("ollama.enabled", "GCAL_OLLAMA_ENABLED")
	mustBindEnv("ollama.endpoint", "GCAL_OLLAMA_ENDPOINT")
	mustBindEnv("ollama.timeout", "GCAL_OLLAMA_TIMEOUT")
	mustBindEnv("ollama.sensitivity.enabled", "GCAL_OLLAMA_SENSITIVITY_ENABLED")
	mustBindEnv("ollama.sensitivity.model", "GCAL_OLLAMA_SENSITIVITY_MODEL")
	mustBindEnv("ollama.sensitivity.threshold", "GCAL_OLLAMA_SENSITIVITY_THRESHOLD")
	mustBindEnv("ollama.assignments.model", "GCAL_OLLAMA_ASSIGNMENTS_MODEL")
	mustBindEnv("ollama.local_only", "GCAL_OLLAMA_LOCAL_ONLY")

	// Override defaults with viper values
	if v := viper.GetString("master_folder_name"); v != "" {
		cfg.MasterFolderName = v
	}
	if v := viper.GetInt("days_to_look_back"); v > 0 {
		cfg.DaysToLookBack = v
	}
	if v := viper.GetString("filename_pattern"); v != "" {
		cfg.FilenamePattern = v
	}

	// filename_keywords: YAML provides a native list; env var uses comma-separated string.
	// viper.GetStringSlice handles both cases when the value comes from YAML.
	// For env var overrides, check the raw string and split on comma.
	if keywords := viper.GetStringSlice("filename_keywords"); len(keywords) > 0 {
		cfg.FilenameKeywords = keywords
	} else if v := viper.GetString("filename_keywords"); v != "" {
		cfg.FilenameKeywords = strings.Split(v, ",")
	}

	if v := viper.GetString("gemini_api_key"); v != "" {
		cfg.GeminiAPIKey = v
	}
	if v := viper.GetString("gemini_model"); v != "" {
		cfg.GeminiModel = v
	}
	if v := viper.GetString("credentials_file"); v != "" {
		cfg.CredentialsFile = v
	}

	if v := viper.GetString("decisions.export_dir"); v != "" {
		cfg.Decisions.ExportDir = v
	}

	// decisions.meetings: YAML provides a native list; env var uses comma-separated string.
	if meetings := viper.GetStringSlice("decisions.meetings"); len(meetings) > 0 {
		cfg.Decisions.Meetings = meetings
	}

	// Ollama configuration overrides from viper (YAML or env vars).
	// viper.IsSet checks whether a key was explicitly provided (YAML or env),
	// avoiding overwriting defaults with zero values.
	if viper.IsSet("ollama.enabled") {
		cfg.Ollama.Enabled = viper.GetBool("ollama.enabled")
	}
	if v := viper.GetString("ollama.endpoint"); v != "" {
		cfg.Ollama.Endpoint = v
	}
	if viper.IsSet("ollama.timeout") {
		cfg.Ollama.Timeout = viper.GetInt("ollama.timeout")
	}
	if viper.IsSet("ollama.sensitivity.enabled") {
		cfg.Ollama.Sensitivity.Enabled = viper.GetBool("ollama.sensitivity.enabled")
	}
	if v := viper.GetString("ollama.sensitivity.model"); v != "" {
		cfg.Ollama.Sensitivity.Model = v
	}
	if viper.IsSet("ollama.sensitivity.threshold") {
		cfg.Ollama.Sensitivity.Threshold = viper.GetFloat64("ollama.sensitivity.threshold")
	}
	if v := viper.GetString("ollama.assignments.model"); v != "" {
		cfg.Ollama.Assignments.Model = v
	}
	if viper.IsSet("ollama.local_only") {
		cfg.Ollama.LocalOnly = viper.GetBool("ollama.local_only")
	}

	cfg.Verbose = viper.GetBool("verbose")
	cfg.DryRun = viper.GetBool("dry-run")
	cfg.OwnedOnly = viper.GetBool("owned-only")
	cfg.NoKeyring = viper.GetBool("no-keyring")

	// Expand tilde in Decisions.ExportDir, consistent with CredentialsFile/TokenFile handling
	if strings.HasPrefix(cfg.Decisions.ExportDir, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			cfg.Decisions.ExportDir = filepath.Join(home, cfg.Decisions.ExportDir[2:])
		}
	}

	return cfg, nil
}

// LoadSecrets loads secrets from the SecretStore, overriding values from
// environment variables when a keychain value is present. This allows
// keychain-stored secrets to take precedence over env vars while preserving
// env var fallback behavior.
func (c *Config) LoadSecrets(store secrets.SecretStore) {
	// Try to load the Gemini API key from the store. If found, it takes
	// precedence over the env var value already loaded by Load().
	if val, err := store.Get(secrets.KeyGeminiAPIKey); err == nil && val != "" {
		c.GeminiAPIKey = val
	}
}

// Validate checks that required configuration values are set.
// FR-016: When Ollama.LocalOnly is true, GeminiAPIKey is NOT required
// because all AI processing runs locally.
func (c *Config) Validate() error {
	if c.Ollama.Enabled && c.Ollama.LocalOnly {
		// Local-only mode: Gemini key is optional (FR-016).
		return nil
	}
	if c.GeminiAPIKey == "" {
		return ux.MissingAPIKey()
	}
	return nil
}
