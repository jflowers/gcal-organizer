// Package config provides configuration management for gcal-organizer.
//
// migrate.go implements one-time migration from .env to config.yaml.
// Secret keys are excluded from the YAML output (FR-003).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jflowers/gcal-organizer/internal/logging"
	"gopkg.in/yaml.v3"
)

// secretEnvKeys are .env keys that contain secrets and must NOT be
// written to config.yaml. They are managed by the keychain.
var secretEnvKeys = map[string]bool{
	"GEMINI_API_KEY":          true,
	"GOOGLE_CREDENTIALS_FILE": true,
}

// yamlConfig is the serialization structure for config.yaml.
// It mirrors the YAML schema from data-model.md §1.
type yamlConfig struct {
	MasterFolderName  string         `yaml:"master_folder_name,omitempty"`
	DaysToLookBack    int            `yaml:"days_to_look_back,omitempty"`
	FilenameKeywords  []string       `yaml:"filename_keywords,omitempty"`
	FilenamePattern   string         `yaml:"filename_pattern,omitempty"`
	GeminiModel       string         `yaml:"gemini_model,omitempty"`
	ChromeProfilePath string         `yaml:"chrome_profile_path,omitempty"`
	Decisions         *yamlDecisions `yaml:"decisions,omitempty"`
}

// yamlDecisions is the nested decisions section in config.yaml.
type yamlDecisions struct {
	ExportDir string `yaml:"export_dir,omitempty"`
}

// envKeyMapping maps .env variable names to their YAML config equivalents.
// Keys not in this map (and not in secretEnvKeys) are skipped with a warning.
var envKeyMapping = map[string]string{
	"GCAL_MASTER_FOLDER_NAME":   "master_folder_name",
	"GCAL_DAYS_TO_LOOK_BACK":    "days_to_look_back",
	"GCAL_FILENAME_KEYWORDS":    "filename_keywords",
	"GCAL_FILENAME_PATTERN":     "filename_pattern",
	"GEMINI_MODEL":              "gemini_model",
	"CHROME_PROFILE_PATH":       "chrome_profile_path",
	"GCAL_DECISIONS_EXPORT_DIR": "decisions.export_dir",
}

// MigrateEnvToYAML converts a .env file to config.yaml.
// It parses the .env file, maps keys to the YAML structure,
// writes config.yaml atomically, and deletes the .env file.
// Secret keys (GEMINI_API_KEY, GOOGLE_CREDENTIALS_FILE) are excluded.
//
// Returns nil if migration succeeds or if no .env file exists.
// Returns an error if parsing or writing fails (the .env file is NOT
// deleted on error, preserving the user's configuration).
func MigrateEnvToYAML(envPath, yamlPath, home string) error {
	// Check if .env exists
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		return nil // no .env to migrate
	}

	// Parse .env file
	pairs, err := ParseDotEnv(envPath, home)
	if err != nil {
		return fmt.Errorf("parse .env for migration: %w", err)
	}

	// Build YAML config from .env key-value pairs
	cfg := &yamlConfig{}
	for key, val := range pairs {
		// Skip secrets — they belong in the keychain (FR-003)
		if secretEnvKeys[key] {
			continue
		}

		yamlKey, known := envKeyMapping[key]
		if !known {
			logging.Logger.Warn("Skipping unknown .env key during migration", "key", key)
			continue
		}

		switch yamlKey {
		case "master_folder_name":
			cfg.MasterFolderName = val
		case "days_to_look_back":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.DaysToLookBack = n
			} else {
				logging.Logger.Warn("Invalid days_to_look_back value, skipping", "value", val)
			}
		case "filename_keywords":
			// .env uses comma-separated values; YAML uses a list
			cfg.FilenameKeywords = strings.Split(val, ",")
			// Trim whitespace from each keyword
			for i, kw := range cfg.FilenameKeywords {
				cfg.FilenameKeywords[i] = strings.TrimSpace(kw)
			}
		case "filename_pattern":
			cfg.FilenamePattern = val
		case "gemini_model":
			cfg.GeminiModel = val
		case "chrome_profile_path":
			cfg.ChromeProfilePath = val
		case "decisions.export_dir":
			if cfg.Decisions == nil {
				cfg.Decisions = &yamlDecisions{}
			}
			cfg.Decisions.ExportDir = val
		}
	}

	// Marshal to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config to YAML: %w", err)
	}

	// Add header comment
	header := "# GCal Organizer Configuration\n# Migrated from .env by gcal-organizer\n\n"
	content := []byte(header + string(data))

	// Atomic write: temp file + rename (review finding X-1: use 0o600 permissions)
	tmpPath := yamlPath + ".tmp"
	tmpFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create temp config.yaml: %w", err)
	}
	if _, err := tmpFile.Write(content); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp config.yaml: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp config.yaml: %w", err)
	}

	if err := os.Rename(tmpPath, yamlPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp config.yaml: %w", err)
	}

	// Verify the written file is readable
	if _, err := os.ReadFile(yamlPath); err != nil {
		return fmt.Errorf("verify config.yaml readable: %w", err)
	}

	// Delete .env after successful migration
	if err := os.Remove(envPath); err != nil && !os.IsNotExist(err) {
		logging.Logger.Warn("Could not delete .env after migration", "error", err)
		// Non-fatal: config.yaml exists and is valid
	}

	logging.Logger.Info("Migrated .env to config.yaml", "path", yamlPath)
	return nil
}
