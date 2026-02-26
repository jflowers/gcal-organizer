// Package config provides configuration management for gcal-organizer.
package config

import (
	"os"

	"strings"

	"github.com/jflowers/gcal-organizer/internal/ux"
	"github.com/spf13/viper"
)

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

	// TokenFile is the path to store OAuth tokens
	TokenFile string

	// Verbose enables verbose output
	Verbose bool

	// DryRun prevents making changes
	DryRun bool

	// OwnedOnly restricts mutations to files owned by the authenticated user
	OwnedOnly bool

	// ChromeProfilePath is the path to Chrome profile for browser automation
	ChromeProfilePath string
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()

	// Dedicated Chrome profile for gcal-organizer automation.
	// Stored alongside other config — same path on all OSes.
	chromePath := home + "/.gcal-organizer/chrome-data"

	return &Config{
		MasterFolderName:  "Meeting Notes",
		DaysToLookBack:    1,
		FilenamePattern:   `(.+)\s*-\s*(\d{4}-\d{2}-\d{2})`,
		FilenameKeywords:  []string{"Notes", "Meeting"},
		GeminiModel:       "gemini-2.0-flash",
		CredentialsFile:   home + "/.gcal-organizer/credentials.json",
		TokenFile:         home + "/.gcal-organizer/token.json",
		ChromeProfilePath: chromePath,
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
	viper.BindEnv("master_folder_name", "GCAL_MASTER_FOLDER_NAME")
	viper.BindEnv("days_to_look_back", "GCAL_DAYS_TO_LOOK_BACK")
	viper.BindEnv("filename_pattern", "GCAL_FILENAME_PATTERN")
	viper.BindEnv("filename_keywords", "GCAL_FILENAME_KEYWORDS")
	viper.BindEnv("gemini_api_key", "GEMINI_API_KEY")
	viper.BindEnv("gemini_model", "GEMINI_MODEL")
	viper.BindEnv("credentials_file", "GOOGLE_CREDENTIALS_FILE")

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

	return cfg, nil
}

// Validate checks that required configuration values are set.
func (c *Config) Validate() error {
	if c.GeminiAPIKey == "" {
		return ux.MissingAPIKey()
	}
	return nil
}

// ValidateForWorkflow checks that all configuration needed for the full workflow is set.
func (c *Config) ValidateForWorkflow() error {
	if err := c.Validate(); err != nil {
		return err
	}

	if _, err := os.Stat(c.CredentialsFile); os.IsNotExist(err) {
		return ux.MissingCredentials(c.CredentialsFile)
	}

	return nil
}
