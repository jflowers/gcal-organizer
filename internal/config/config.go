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

// Config holds all configuration values for the application.
type Config struct {
	// MasterFolderName is the name of the master folder in Google Drive
	MasterFolderName string

	// DaysToLookBack is the number of days to scan for calendar events
	DaysToLookBack int

	// FilenamePattern is the regex pattern for parsing document names
	FilenamePattern string

	// FilenameKeywords are keywords used to filter relevant documents
	FilenameKeywords []string

	// GeminiAPIKey is the GCP API key for Gemini
	GeminiAPIKey string

	// GeminiModel is the Gemini model to use
	GeminiModel string

	// CredentialsFile is the path to the Google OAuth credentials file
	CredentialsFile string

	// TokenFile is the path to store OAuth tokens (legacy, used by FileStore)
	TokenFile string

	// Verbose enables verbose output
	Verbose bool

	// DryRun prevents making changes
	DryRun bool

	// OwnedOnly restricts mutations to files owned by the authenticated user
	OwnedOnly bool

	// NoKeyring disables OS credential store; use file-based storage
	NoKeyring bool

	// ChromeProfilePath is the path to Chrome profile for browser automation
	ChromeProfilePath string
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
	if v := viper.GetString("filename_keywords"); v != "" {
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

	cfg.Verbose = viper.GetBool("verbose")
	cfg.DryRun = viper.GetBool("dry-run")
	cfg.OwnedOnly = viper.GetBool("owned-only")
	cfg.NoKeyring = viper.GetBool("no-keyring")

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
func (c *Config) Validate() error {
	if c.GeminiAPIKey == "" {
		return ux.MissingAPIKey()
	}
	return nil
}
