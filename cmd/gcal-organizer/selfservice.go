package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/jflowers/gcal-organizer/internal/config"
	"github.com/jflowers/gcal-organizer/internal/ollama"
	"github.com/jflowers/gcal-organizer/internal/secrets"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
)

// --- Lip Gloss styles ---

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212")).
			PaddingLeft(1)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10"))

	warnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9"))

	subtleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 1)
)

func styledPass(msg string) string  { return successStyle.Render("  ✅ " + msg) }
func styledWarn(msg string) string  { return warnStyle.Render("  ⚠️  " + msg) }
func styledFail(msg string) string  { return errorStyle.Render("  ❌ " + msg) }
func styledFix(msg string) string   { return subtleStyle.Render("     Fix: " + msg) }
func styledTitle(msg string) string { return titleStyle.Render(msg) }

// doctorCmd checks system health and reports issues with fixes.
var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system health and report issues with fixes",
	Long:  `Diagnose common issues with gcal-organizer setup and report actionable fixes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}
		configDir := filepath.Join(home, ".gcal-organizer")
		passed := 0
		warned := 0
		failed := 0

		fmt.Println(styledTitle("🩺 gcal-organizer doctor"))
		fmt.Println()

		// 1. Config directory (existence + permissions)
		if info, err := os.Stat(configDir); err == nil && info.IsDir() {
			fmt.Println(styledPass("Config directory exists"))
			if verbose {
				fmt.Println(subtleStyle.Render("          " + configDir))
			}
			passed++
			// Check that the directory is not world- or group-readable (should be 0700).
			if info.Mode().Perm()&0077 != 0 {
				fmt.Println(styledWarn("Config directory has overly permissive permissions (should be 0700)"))
				fmt.Println(styledFix("Run: chmod 700 ~/.gcal-organizer"))
				warned++
			}
		} else {
			fmt.Println(styledFail("Config directory ~/.gcal-organizer/ not found"))
			fmt.Println(styledFix("Run 'gcal-organizer init'"))
			failed++
		}

		// 2. Config file (config.yaml)
		configFile := filepath.Join(configDir, "config.yaml")
		envFile := filepath.Join(configDir, ".env")
		if _, err := os.Stat(configFile); err == nil {
			fmt.Println(styledPass("Config file (config.yaml) exists"))
			if verbose {
				fmt.Println(subtleStyle.Render("          " + configFile))
			}
			passed++
		} else {
			fmt.Println(styledFail("Config file ~/.gcal-organizer/config.yaml not found"))
			fmt.Println(styledFix("Run 'gcal-organizer init'"))
			failed++
		}

		// Transitional check: warn if .env still exists alongside config.yaml
		if _, err := os.Stat(envFile); err == nil {
			if _, yamlErr := os.Stat(configFile); yamlErr == nil {
				fmt.Println(styledWarn("Legacy .env file still exists alongside config.yaml"))
				fmt.Println(styledFix("The .env file can be safely deleted — config.yaml is the active config"))
			} else {
				fmt.Println(styledWarn("Legacy .env file found — will be migrated to config.yaml on next run"))
				fmt.Println(styledFix("Run any gcal-organizer command to trigger automatic migration"))
			}
			warned++
		}

		// 3. Secret storage backend — must come before credential checks so
		// migration runs first and the store is available for store-first lookups.
		// Uses loadConfigAndStore() so --no-keyring is respected AND auto-migration
		// triggers (moves token.json, .env secrets, credentials.json to keychain).
		_, store, backend, storeErr := loadConfigAndStore()
		if storeErr != nil {
			// Fallback: create store directly if config load fails (e.g., during
			// initial setup when .env doesn't exist yet).
			noKeyring := viper.GetBool("no-keyring")
			store, backend = secrets.NewStore(noKeyring)
		}
		if backend == secrets.BackendKeychain {
			fmt.Println(styledPass("Secrets stored in OS keychain"))
			passed++
		} else {
			fmt.Println(styledWarn("Secrets stored in plaintext files"))
			if viper.GetBool("no-keyring") {
				fmt.Println(styledFix("Remove --no-keyring flag to use OS keychain"))
			} else {
				fmt.Println(styledFix("Install a keyring provider (macOS Keychain, GNOME Keyring)"))
			}
			warned++
		}

		// 4. credentials.json (check store first, then file fallback)
		credFile := filepath.Join(configDir, "credentials.json")
		if _, credErr := store.Get(secrets.KeyClientCredentials); credErr == nil {
			fmt.Println(styledPass("Google credentials found (in secret store)"))
			passed++
		} else if _, err := os.Stat(credFile); err == nil {
			fmt.Println(styledPass("Google credentials (credentials.json) found"))
			if verbose {
				fmt.Println(subtleStyle.Render("          " + credFile))
			}
			passed++
		} else {
			fmt.Println(styledFail("Google credentials not found"))
			fmt.Println(styledFix("Download from https://console.cloud.google.com/apis/credentials"))
			fmt.Println(subtleStyle.Render("          Save as ~/.gcal-organizer/credentials.json"))
			failed++
		}

		// Verbose: per-secret status
		if verbose {
			for _, secret := range []struct {
				name string
				key  string
			}{
				{"oauth-token", secrets.KeyOAuthToken},
				{"gemini-api-key", secrets.KeyGeminiAPIKey},
				{"credentials-json", secrets.KeyClientCredentials},
			} {
				if _, err := store.Get(secret.key); err == nil {
					fmt.Println(subtleStyle.Render(fmt.Sprintf("          %s: present", secret.name)))
				} else {
					fmt.Println(subtleStyle.Render(fmt.Sprintf("          %s: absent", secret.name)))
				}
			}
		}

		// 5. OAuth token (check store first, then file fallback)
		tokenFile := filepath.Join(configDir, "token.json")
		tokenFound := false
		if tokData, tokErr := store.Get(secrets.KeyOAuthToken); tokErr == nil && tokData != "" {
			var tok oauth2.Token
			if err := json.Unmarshal([]byte(tokData), &tok); err == nil {
				if tok.Expiry.After(time.Now()) || tok.RefreshToken != "" {
					fmt.Println(styledPass(fmt.Sprintf("OAuth token found (%s)", backend)))
					passed++
					tokenFound = true
				} else {
					fmt.Println(styledWarn("OAuth token exists but may be expired"))
					fmt.Println(styledFix("Run 'gcal-organizer auth login' to re-authenticate"))
					warned++
					tokenFound = true
				}
			}
		}
		// Fallback: check token.json on disk
		if !tokenFound {
			if _, err := os.Stat(tokenFile); err == nil {
				func() {
					f, err := os.Open(tokenFile)
					if err != nil {
						return
					}
					defer f.Close()
					var tok oauth2.Token
					if err := json.NewDecoder(f).Decode(&tok); err == nil {
						if tok.Expiry.After(time.Now()) || tok.RefreshToken != "" {
							fmt.Println(styledPass("OAuth token found (file)"))
							if verbose {
								fmt.Println(subtleStyle.Render("          " + tokenFile))
							}
							passed++
							tokenFound = true
						} else {
							fmt.Println(styledWarn("OAuth token exists but may be expired"))
							fmt.Println(styledFix("Run 'gcal-organizer auth login' to re-authenticate"))
							warned++
							tokenFound = true
						}
					} else {
						fmt.Println(styledWarn("OAuth token file is corrupted"))
						fmt.Println(styledFix("Run 'gcal-organizer auth login' to re-authenticate"))
						warned++
						tokenFound = true
					}
				}()
			}
		}
		if !tokenFound {
			fmt.Println(styledFail("Not authenticated — no OAuth token found"))
			fmt.Println(styledFix("Run 'gcal-organizer auth login'"))
			failed++
		}

		// 6. GEMINI_API_KEY (check store first, then env fallback)
		apiKey := ""
		if val, err := store.Get(secrets.KeyGeminiAPIKey); err == nil && val != "" {
			apiKey = val
		}
		if apiKey == "" {
			apiKey = os.Getenv("GEMINI_API_KEY")
		}
		if apiKey != "" && apiKey != "your-gcp-api-key-here" {
			prefix := apiKey[:min(4, len(apiKey))]
			fmt.Println(styledPass(fmt.Sprintf("GEMINI_API_KEY is set (%s****)", prefix)))
			passed++
		} else if apiKey == "your-gcp-api-key-here" {
			fmt.Println(styledFail("GEMINI_API_KEY is still set to placeholder value"))
			fmt.Println(styledFix("Get your API key from https://aistudio.google.com/app/apikey"))
			failed++
		} else {
			fmt.Println(styledFail("GEMINI_API_KEY is not set"))
			fmt.Println(styledFix("Run 'gcal-organizer init' or set the GEMINI_API_KEY environment variable"))
			failed++
		}

		// 6. Node.js
		if nodeOut, err := exec.Command("node", "--version").Output(); err == nil {
			version := strings.TrimSpace(string(nodeOut))
			fmt.Println(styledPass(fmt.Sprintf("Node.js found (%s)", version)))
			if verbose {
				if nodePath, err := exec.LookPath("node"); err == nil {
					fmt.Println(subtleStyle.Render("          " + nodePath))
				}
			}
			passed++
		} else {
			fmt.Println(styledWarn("Node.js not found — task assignment unavailable"))
			fmt.Println(styledFix("Install Node.js 18+ from https://nodejs.org"))
			warned++
		}

		// 7. Dedicated Chrome data directory
		chromePath, chromePathErr := chromeProfilePath()
		if chromePathErr != nil {
			fmt.Println(styledFail("Cannot determine Chrome profile path: " + chromePathErr.Error()))
			failed++
		} else if _, err := os.Stat(chromePath); err == nil {
			fmt.Println(styledPass(fmt.Sprintf("Dedicated Chrome data dir found (%s)", filepath.Base(chromePath))))
			if verbose {
				fmt.Println(subtleStyle.Render("          " + chromePath))
			}
			passed++
		} else {
			fmt.Println(styledWarn("Dedicated Chrome profile not yet created"))
			fmt.Println(styledFix("Run 'gcal-organizer setup-browser' to create it"))
			if verbose {
				fmt.Println(subtleStyle.Render("          Expected at: " + chromePath))
			}
			warned++
		}

		// 7b. Flatpak Chrome filesystem access
		if isFlatpakChrome() {
			if hasFlatpakFilesystemAccess() {
				fmt.Println(styledPass("Flatpak Chrome filesystem access granted"))
				passed++
			} else {
				fmt.Println(styledWarn("Flatpak Chrome lacks filesystem access to ~/.gcal-organizer/"))
				fmt.Println(styledFix("flatpak override --user --filesystem=~/.gcal-organizer com.google.Chrome"))
				warned++
			}
		}

		// 8. Service status
		if isServiceInstalled() {
			fmt.Println(styledPass("Hourly service is installed"))
			passed++
		} else {
			fmt.Println(styledWarn("Hourly service is not installed"))
			fmt.Println(styledFix("Run 'gcal-organizer install'"))
			warned++
		}

		// 9. Browser deps (npm install in browser/)
		browserDir, _ := findBrowserDir()
		if browserDir != "" {
			nodeModules := filepath.Join(browserDir, "node_modules")
			if _, err := os.Stat(nodeModules); err == nil {
				fmt.Println(styledPass("Browser automation deps installed"))
				if verbose {
					fmt.Println(subtleStyle.Render("          " + browserDir))
				}
				passed++
			} else {
				fmt.Println(styledWarn("Browser automation deps not installed"))
				fmt.Println(styledFix("Run 'gcal-organizer setup-browser'"))
				warned++
			}
		} else {
			fmt.Println(styledWarn("Browser directory not found"))
			fmt.Println(styledFix("Run from project root or install browser automation"))
			warned++
		}

		// 10. Chrome debugging port
		if isPortOpen(9222) {
			fmt.Println(styledPass("Chrome debugging port (9222) is active"))
			passed++
		} else {
			fmt.Println(styledWarn("Chrome debugging port (9222) not active"))
			fmt.Println(styledFix("Run 'gcal-organizer setup-browser' to launch Chrome"))
			warned++
		}

		// 11-14. Ollama checks (FR-027: skip when disabled)
		cfg, _, _, cfgErr := loadConfigAndStore()
		if cfgErr != nil {
			// If config load fails, try defaults
			cfg = config.DefaultConfig()
		}

		if cfg.Ollama.Enabled {
			// Check 11: Ollama binary installed
			if _, lookErr := exec.LookPath("ollama"); lookErr == nil {
				fmt.Println(styledPass("Ollama binary installed"))
				passed++
			} else {
				fmt.Println(styledFail("Ollama binary not found"))
				fmt.Println(styledFix("Install: brew install ollama"))
				failed++
			}

			// Check 12: Ollama service running
			ollamaClient := ollama.NewClient(cfg.Ollama.Endpoint, 5)
			ollamaRunning := ollamaClient.HealthCheck()
			if ollamaRunning {
				fmt.Println(styledPass("Ollama service running"))
				passed++
			} else {
				fmt.Println(styledFail("Ollama service not running"))
				fmt.Println(styledFix("Run: ollama serve"))
				failed++
			}

			// Checks 13-14: Model availability (only if service is running)
			if ollamaRunning {
				// Check 13: Sensitivity model
				if ollamaClient.ModelAvailable(cfg.Ollama.Sensitivity.Model) {
					fmt.Println(styledPass(fmt.Sprintf("Sensitivity model (%s) available", cfg.Ollama.Sensitivity.Model)))
					passed++
				} else {
					fmt.Println(styledFail(fmt.Sprintf("Sensitivity model (%s) not found", cfg.Ollama.Sensitivity.Model)))
					fmt.Println(styledFix(fmt.Sprintf("Run: ollama pull %s", cfg.Ollama.Sensitivity.Model)))
					failed++
				}

				// Check 14: Assignment model
				if ollamaClient.ModelAvailable(cfg.Ollama.Assignments.Model) {
					fmt.Println(styledPass(fmt.Sprintf("Assignment model (%s) available", cfg.Ollama.Assignments.Model)))
					passed++
				} else {
					fmt.Println(styledFail(fmt.Sprintf("Assignment model (%s) not found", cfg.Ollama.Assignments.Model)))
					fmt.Println(styledFix(fmt.Sprintf("Run: ollama pull %s", cfg.Ollama.Assignments.Model)))
					failed++
				}
			} else {
				fmt.Println(styledWarn("Model checks skipped — Ollama not running"))
				warned++
			}
		}

		// Summary
		fmt.Println()
		summaryLine := fmt.Sprintf("  ✅ %d passed  ⚠️  %d warnings  ❌ %d failed", passed, warned, failed)
		fmt.Println(boxStyle.Render(summaryLine))
		if failed > 0 {
			fmt.Println(subtleStyle.Render("  Run 'gcal-organizer init' to fix most issues."))
		} else if warned > 0 {
			fmt.Println(subtleStyle.Render("  All critical checks passed. Warnings are informational."))
		} else {
			fmt.Println(successStyle.Render("  🎉 Everything looks good!"))
		}
		return nil
	},
}

// initCmd sets up the gcal-organizer configuration.
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Set up gcal-organizer configuration",
	Long:  `Create the config directory and generate a YAML configuration file with your API keys.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}
		configDir := filepath.Join(home, ".gcal-organizer")
		configFile := filepath.Join(configDir, "config.yaml")
		nonInteractive, _ := cmd.Flags().GetBool("non-interactive")
		apiKey, _ := cmd.Flags().GetString("api-key")

		fmt.Println(styledTitle("🚀 gcal-organizer init"))
		fmt.Println()

		// 1. Create config directory
		if _, err := os.Stat(configDir); os.IsNotExist(err) {
			if err := os.MkdirAll(configDir, 0700); err != nil {
				return fmt.Errorf("failed to create config directory: %w", err)
			}
			fmt.Println(styledPass("Created ~/.gcal-organizer/"))
		} else {
			fmt.Println(styledPass("Config directory already exists"))
		}

		// Create store early so it's available for all checks below.
		noKeyring := viper.GetBool("no-keyring")
		store, backend := secrets.NewStore(noKeyring)

		// 2. Generate config.yaml (FR-006)
		if _, err := os.Stat(configFile); os.IsNotExist(err) {
			// Get API key
			if apiKey == "" && !nonInteractive {
				form := huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Gemini API Key").
							Description("From https://aistudio.google.com/app/apikey").
							Placeholder("AIza...").
							Value(&apiKey),
					),
				)
				if err := form.Run(); err != nil {
					apiKey = ""
				}
			}
			if apiKey == "" {
				apiKey = "your-gcp-api-key-here"
			}

			// Store API key in SecretStore when keychain is available
			if apiKey != "your-gcp-api-key-here" {
				if err := store.Set(secrets.KeyGeminiAPIKey, apiKey); err == nil {
					fmt.Println(styledPass(fmt.Sprintf("Gemini API key stored in %s", backend)))
				}
			}

			// Write config.yaml with defaults and comments
			yamlContent := generateConfigYAML()
			if err := os.WriteFile(configFile, []byte(yamlContent), 0600); err != nil {
				return fmt.Errorf("failed to write config.yaml: %w", err)
			}
			fmt.Println(styledPass("Created ~/.gcal-organizer/config.yaml"))
		} else {
			fmt.Println(styledPass("Config file already exists (skipped)"))
		}

		// 3. Check for credentials.json (store first, then file fallback)
		credFile := filepath.Join(configDir, "credentials.json")
		hasCreds := false
		if _, credErr := store.Get(secrets.KeyClientCredentials); credErr == nil {
			fmt.Println(styledPass("Google credentials found (in secret store)"))
			hasCreds = true
		} else if _, err := os.Stat(credFile); err == nil {
			fmt.Println(styledPass("Google credentials found"))
			hasCreds = true
		} else {
			fmt.Println()
			fmt.Println(styledWarn("credentials.json not found"))
			fmt.Println(styledFix("Download OAuth credentials from Google Cloud Console:"))
			fmt.Println(subtleStyle.Render("     https://console.cloud.google.com/apis/credentials"))
			fmt.Println(subtleStyle.Render("     Save as: ~/.gcal-organizer/credentials.json"))
		}

		// 4. Ollama model pull prompt (FR-028)
		cfg := config.DefaultConfig()
		// Try to load actual config if available.
		if loadedCfg, loadErr := config.Load(); loadErr == nil {
			cfg = loadedCfg
		}
		if cfg.Ollama.Enabled {
			ollamaClient := ollama.NewClient(cfg.Ollama.Endpoint, 5)
			if ollamaClient.HealthCheck() {
				var missingModels []string
				if !ollamaClient.ModelAvailable(cfg.Ollama.Sensitivity.Model) {
					missingModels = append(missingModels, cfg.Ollama.Sensitivity.Model)
				}
				if !ollamaClient.ModelAvailable(cfg.Ollama.Assignments.Model) {
					missingModels = append(missingModels, cfg.Ollama.Assignments.Model)
				}

				if len(missingModels) > 0 && !nonInteractive {
					var pullConfirm bool
					form := huh.NewForm(
						huh.NewGroup(
							huh.NewConfirm().
								Title("Local AI models are needed for transcript screening. Pull them now?").
								Affirmative("Yes").
								Negative("No").
								Value(&pullConfirm),
						),
					)
					if err := form.Run(); err == nil && pullConfirm {
						for _, model := range missingModels {
							fmt.Printf("   Pulling %s...\n", model)
							pullCmd := exec.Command("ollama", "pull", model)
							pullCmd.Stdout = os.Stdout
							pullCmd.Stderr = os.Stderr
							if err := pullCmd.Run(); err != nil {
								fmt.Println(styledWarn(fmt.Sprintf("Failed to pull %s: %v", model, err)))
							} else {
								fmt.Println(styledPass(fmt.Sprintf("Pulled %s", model)))
							}
						}
					}
				} else if len(missingModels) > 0 && nonInteractive {
					fmt.Println(styledWarn(fmt.Sprintf("Missing Ollama models: %v", missingModels)))
					fmt.Println(styledFix("Run: ollama pull <model> for each missing model"))
				} else {
					fmt.Println(styledPass("All Ollama models available"))
				}
			}
		}

		fmt.Println()
		var nextSteps string
		if !hasCreds {
			nextSteps = "  Next steps:\n  1. Download credentials.json (see above)\n  2. Run 'gcal-organizer auth login'"
		} else {
			// Check store first for token, then file fallback
			hasToken := false
			if _, tokErr := store.Get(secrets.KeyOAuthToken); tokErr == nil {
				hasToken = true
			} else {
				tokenFile := filepath.Join(configDir, "token.json")
				if _, err := os.Stat(tokenFile); err == nil {
					hasToken = true
				}
			}
			if !hasToken {
				nextSteps = "  Next steps:\n  1. Run 'gcal-organizer auth login'"
			} else {
				nextSteps = "  Next steps:\n  1. Run 'gcal-organizer run --dry-run' to test"
			}
		}
		nextSteps += "\n  2. Run 'gcal-organizer doctor' to verify setup"
		fmt.Println(boxStyle.Render(nextSteps))
		return nil
	},
}

// installCmd installs gcal-organizer as an hourly service.
var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install as an hourly background service",
	Long:  `Install gcal-organizer as an hourly service. Uses launchd on macOS and systemd on Linux.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}
		configDir := filepath.Join(home, ".gcal-organizer")

		// Check prerequisites
		if _, err := os.Stat(configDir); os.IsNotExist(err) {
			fmt.Println("⚠️  Config not found. Running 'gcal-organizer init' first...")
			fmt.Println()
			if err := initCmd.RunE(cmd, args); err != nil {
				return fmt.Errorf("init failed: %w", err)
			}
			fmt.Println()
		}

		// Find binary path
		binaryPath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("could not determine binary path: %w", err)
		}

		fmt.Println("📦 gcal-organizer install")
		fmt.Println("═══════════════════════════════════════════════════════════")
		fmt.Println()

		switch runtime.GOOS {
		case "darwin":
			return installMacOS(home, binaryPath)
		case "linux":
			return installLinux(home, binaryPath)
		default:
			return fmt.Errorf("unsupported OS: %s (supported: darwin, linux)", runtime.GOOS)
		}
	},
}

// uninstallCmd removes the gcal-organizer service.
var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the hourly background service",
	Long:  `Stop and remove the gcal-organizer service files.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}

		fmt.Println("🗑️  gcal-organizer uninstall")
		fmt.Println("═══════════════════════════════════════════════════════════")
		fmt.Println()

		switch runtime.GOOS {
		case "darwin":
			return uninstallMacOS(home)
		case "linux":
			return uninstallLinux(home)
		default:
			return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
		}
	},
}

// --- Helper functions ---

// mustUserHomeDir returns the user home directory or panics.
// Used by functions where the home directory is required and failure
// is not recoverable (e.g. service install paths).
func mustUserHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// This should never happen in normal use. All callers that need
		// graceful handling use userHomeDir() instead.
		panic(fmt.Sprintf("os.UserHomeDir() failed: %v", err))
	}
	return home
}

// chromeProfilePath returns the path for the dedicated gcal-organizer Chrome
// data directory at ~/.gcal-organizer/chrome-data.
// Returns ("", error) if the home directory cannot be determined.
func chromeProfilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".gcal-organizer", "chrome-data"), nil
}

// copyBrowserDir copies the browser/ automation directory (TypeScript scripts,
// package.json, etc.) from the source tree to ~/.gcal-organizer/browser/.
// node_modules/ is excluded — npm install is run in the destination afterwards.
// The source directory is located via findBrowserDir() which checks adjacent to
// the executable and the current working directory.
func copyBrowserDir(home string) error {
	destDir := filepath.Join(home, ".gcal-organizer", "browser")

	// Locate the source browser/ directory. Since we haven't installed it
	// to ~/.gcal-organizer/browser/ yet, findBrowserDir will fall through
	// to the executable-adjacent or CWD-relative paths.
	srcDir, err := findBrowserDir()
	if err != nil {
		return fmt.Errorf("cannot locate browser/ directory in source tree: %w", err)
	}

	// Remove old install so we get a clean copy.
	if err := os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("failed to remove old browser directory: %w", err)
	}

	// Walk and copy, skipping node_modules (will be reinstalled).
	err = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(srcDir, path)
		// Skip node_modules entirely — it will be npm-installed at dest.
		if d.IsDir() && d.Name() == "node_modules" {
			return filepath.SkipDir
		}
		dest := filepath.Join(destDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		return copyFile(path, dest)
	})
	if err != nil {
		return fmt.Errorf("failed to copy browser directory: %w", err)
	}

	// Run npm install in the destination.
	npmCmd := exec.Command("npm", "install", "--production")
	npmCmd.Dir = destDir
	if out, err := npmCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("npm install failed in %s: %w\n%s", destDir, err, string(out))
	}

	return nil
}

// copyFile copies a single file preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// loadEnvValue reads a single value from a .env file, applying the same
// quote-stripping and POSIX single-quote unescaping as loadDotEnv, so callers
// always receive the actual string value rather than the shell-quoted form.
func loadEnvValue(envFile, key string) string {
	data, err := os.ReadFile(envFile)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if strings.TrimSpace(parts[0]) != key {
			continue
		}
		val := strings.TrimSpace(parts[1])
		// Strip surrounding quotes and unescape '\'' → ' for single-quoted values.
		if len(val) >= 2 {
			switch {
			case val[0] == '"' && val[len(val)-1] == '"':
				val = val[1 : len(val)-1]
			case val[0] == '\'' && val[len(val)-1] == '\'':
				val = val[1 : len(val)-1]
				val = strings.ReplaceAll(val, `'\''`, `'`)
			}
		}
		return val
	}
	return ""
}

// generateConfigYAML creates the config.yaml file content with defaults and comments.
// Secrets are not included — they are stored in the OS keychain.
func generateConfigYAML() string {
	var b strings.Builder
	b.WriteString("# GCal Organizer Configuration\n")
	b.WriteString("# Generated by 'gcal-organizer init'\n")
	b.WriteString("# Secrets (API keys, credentials) are stored in the OS keychain.\n")
	b.WriteString("# Run 'gcal-organizer doctor --verbose' to verify.\n\n")

	b.WriteString("# Master folder name in Google Drive\n")
	b.WriteString("master_folder_name: \"Meeting Notes\"\n\n")

	b.WriteString("# Days to look back for calendar events\n")
	b.WriteString("days_to_look_back: 1\n\n")

	b.WriteString("# Keywords to filter relevant documents\n")
	b.WriteString("filename_keywords:\n")
	b.WriteString("  - \"Notes\"\n")
	b.WriteString("  - \"Meeting\"\n\n")

	b.WriteString("# Regex pattern for parsing document names\n")
	b.WriteString("# filename_pattern: '(.+)\\s*-\\s*(\\d{4}-\\d{2}-\\d{2})'\n\n")

	b.WriteString("# Gemini model to use\n")
	b.WriteString("gemini_model: \"gemini-2.0-flash\"\n\n")

	b.WriteString("# Path to Chrome profile for browser automation\n")
	b.WriteString("# chrome_profile_path: \"~/.gcal-organizer/chrome-data\"\n\n")

	b.WriteString("# Decision export configuration\n")
	b.WriteString("decisions:\n")
	b.WriteString("  # Output directory for decision markdown files\n")
	b.WriteString("  export_dir: \"~/.gcal-organizer/decisions\"\n\n")
	b.WriteString("  # Meeting allowlist: only export decisions for these meetings.\n")
	b.WriteString("  # Matching is exact, case-insensitive.\n")
	b.WriteString("  # When empty or absent, decisions are exported for all meetings.\n")
	b.WriteString("  # meetings:\n")
	b.WriteString("  #   - \"Sprint Planning\"\n")
	b.WriteString("  #   - \"Design Review\"\n")
	b.WriteString("  #   - \"Weekly Sync\"\n\n")

	b.WriteString("# Local AI configuration (Ollama with IBM Granite models)\n")
	b.WriteString("ollama:\n")
	b.WriteString("  # Enable local AI features (sensitivity gate, local task assignment)\n")
	b.WriteString("  # When disabled, all Ollama checks and features are skipped.\n")
	b.WriteString("  # Default: true\n")
	b.WriteString("  enabled: true\n\n")
	b.WriteString("  # Ollama API endpoint\n")
	b.WriteString("  # Default: http://localhost:11434\n")
	b.WriteString("  endpoint: \"http://localhost:11434\"\n\n")
	b.WriteString("  # Request timeout for generation requests (seconds)\n")
	b.WriteString("  # Default: 120\n")
	b.WriteString("  timeout: 120\n\n")
	b.WriteString("  # Sensitivity classification settings\n")
	b.WriteString("  sensitivity:\n")
	b.WriteString("    # Enable sensitivity gate\n")
	b.WriteString("    # Default: true\n")
	b.WriteString("    enabled: true\n\n")
	b.WriteString("    # Model for sensitivity classification\n")
	b.WriteString("    # Default: granite-guardian\n")
	b.WriteString("    model: \"granite-guardian\"\n\n")
	b.WriteString("    # Sensitivity threshold (0.0-1.0)\n")
	b.WriteString("    # Transcripts scoring >= this value are skipped.\n")
	b.WriteString("    # Default: 0.7\n")
	b.WriteString("    threshold: 0.7\n\n")
	b.WriteString("  # Task assignment settings\n")
	b.WriteString("  assignments:\n")
	b.WriteString("    # Model for assignee extraction\n")
	b.WriteString("    # Default: granite3.2:8b\n")
	b.WriteString("    model: \"granite3.2:8b\"\n\n")
	b.WriteString("  # Local-only mode\n")
	b.WriteString("  # When true, all AI processing runs locally (no cloud AI calls).\n")
	b.WriteString("  # Decision extraction uses the local model instead of Gemini.\n")
	b.WriteString("  # Default: false\n")
	b.WriteString("  local_only: false\n")

	return b.String()
}

// generateEnvFile creates the .env file content.
// When storedInKeychain is true, secrets (GEMINI_API_KEY and
// GOOGLE_CREDENTIALS_FILE) are omitted since they live in the OS credential
// store. This avoids the write-then-migrate cycle where init writes secrets
// to .env and migration immediately strips them.
// Deprecated: kept for backward compatibility. New installations use generateConfigYAML.
func generateEnvFile(apiKey string, storedInKeychain bool) string {
	var b strings.Builder
	b.WriteString("# GCal Organizer Configuration\n")
	b.WriteString("# Generated by 'gcal-organizer init'\n\n")

	if storedInKeychain {
		b.WriteString("# Gemini API key and OAuth credentials are stored in the OS keychain.\n")
		b.WriteString("# Run 'gcal-organizer doctor --verbose' to verify.\n\n")
	} else {
		home := mustUserHomeDir()
		b.WriteString("# Required: GCP API Key for Gemini AI\n")
		// Single-quote the value so shell treats it as a literal string, preventing
		// injection via $, `, or " characters in the API key. A single-quote inside
		// the value is escaped with the POSIX '\'' sequence, which closes the
		// current single-quoted string, inserts a literal ', then re-opens it.
		escapedKey := strings.ReplaceAll(apiKey, "'", "'\\''")
		b.WriteString(fmt.Sprintf("GEMINI_API_KEY='%s'\n\n", escapedKey))
		b.WriteString("# Required: Path to Google OAuth2 credentials\n")
		b.WriteString(fmt.Sprintf("GOOGLE_CREDENTIALS_FILE=\"%s/.gcal-organizer/credentials.json\"\n\n", home))
	}

	b.WriteString("# Optional: Master folder name in Google Drive\n")
	b.WriteString("GCAL_MASTER_FOLDER_NAME=\"Meeting Notes\"\n\n")
	b.WriteString("# Optional: Days to look back for calendar events\n")
	b.WriteString("GCAL_DAYS_TO_LOOK_BACK=\"1\"\n\n")
	b.WriteString("# Optional: Keywords to filter documents (comma-separated)\n")
	b.WriteString("GCAL_FILENAME_KEYWORDS=\"Notes,Meeting\"\n\n")
	b.WriteString("# Optional: Gemini model\n")
	b.WriteString("GEMINI_MODEL=\"gemini-2.0-flash\"\n")
	return b.String()
}

// isServiceInstalled checks if the hourly service is installed.
func isServiceInstalled() bool {
	home := mustUserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		plist := filepath.Join(home, "Library", "LaunchAgents", "com.jflowers.gcal-organizer.plist")
		_, err := os.Stat(plist)
		return err == nil
	case "linux":
		timer := filepath.Join(home, ".config", "systemd", "user", "gcal-organizer.timer")
		_, err := os.Stat(timer)
		return err == nil
	}
	return false
}

// --- macOS install/uninstall ---

func installMacOS(home, binaryPath string) error {
	logDir := filepath.Join(home, "Library", "Logs")
	logFile := filepath.Join(logDir, "gcal-organizer.log")
	plistDest := filepath.Join(home, "Library", "LaunchAgents", "com.jflowers.gcal-organizer.plist")
	wrapperDest := filepath.Join(home, ".local", "bin", "gcal-organizer-wrapper.sh")

	// Create wrapper script
	if err := os.MkdirAll(filepath.Dir(wrapperDest), 0755); err != nil {
		return fmt.Errorf("failed to create wrapper directory: %w", err)
	}

	wrapper := generateWrapper(binaryPath)
	if err := os.WriteFile(wrapperDest, []byte(wrapper), 0755); err != nil {
		return fmt.Errorf("failed to write wrapper script: %w", err)
	}
	fmt.Println("  ✅ Created wrapper script")

	// Copy browser automation scripts
	if err := copyBrowserDir(home); err != nil {
		// Non-fatal: browser automation is optional (only needed for task assignment).
		fmt.Printf("  ⚠️  Browser automation not installed: %v\n", err)
		fmt.Println("     Task assignment via browser will not work in service mode.")
		fmt.Println("     Run 'gcal-organizer install' from the project directory to fix.")
	} else {
		fmt.Println("  ✅ Installed browser automation scripts")
	}

	// Create plist
	if err := os.MkdirAll(filepath.Dir(plistDest), 0755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory: %w", err)
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	plist := generatePlist(wrapperDest, logFile, home, binaryPath)
	if err := os.WriteFile(plistDest, []byte(plist), 0644); err != nil {
		return fmt.Errorf("failed to write plist: %w", err)
	}
	fmt.Println("  ✅ Created LaunchAgent")

	// Load the service
	uid := fmt.Sprintf("%d", os.Getuid())
	// Intentionally ignore bootout error: the service may not be loaded yet.
	_ = exec.Command("launchctl", "bootout", "gui/"+uid, plistDest).Run()
	if err := exec.Command("launchctl", "bootstrap", "gui/"+uid, plistDest).Run(); err != nil {
		return fmt.Errorf("failed to load LaunchAgent: %w\n   Fix: Try 'launchctl load %s'", err, plistDest)
	}
	fmt.Println("  ✅ Service loaded and running")

	fmt.Println()
	fmt.Println("───────────────────────────────────────────────────────────")
	fmt.Printf("  Logs:    %s\n", logFile)
	fmt.Println("  Status:  gcal-organizer doctor")
	fmt.Println("  Remove:  gcal-organizer uninstall")
	fmt.Println("═══════════════════════════════════════════════════════════")
	return nil
}

func uninstallMacOS(home string) error {
	plistDest := filepath.Join(home, "Library", "LaunchAgents", "com.jflowers.gcal-organizer.plist")
	wrapperDest := filepath.Join(home, ".local", "bin", "gcal-organizer-wrapper.sh")

	uid := fmt.Sprintf("%d", os.Getuid())
	// Intentionally ignore bootout error: the service may not be loaded.
	_ = exec.Command("launchctl", "bootout", "gui/"+uid, plistDest).Run()
	fmt.Println("  ✅ Service stopped")

	if err := os.Remove(plistDest); err != nil && !os.IsNotExist(err) {
		fmt.Printf("  ⚠️  Could not remove LaunchAgent: %v\n", err)
	} else {
		fmt.Println("  ✅ Removed LaunchAgent")
	}

	if err := os.Remove(wrapperDest); err != nil && !os.IsNotExist(err) {
		fmt.Printf("  ⚠️  Could not remove wrapper script: %v\n", err)
	} else {
		fmt.Println("  ✅ Removed wrapper script")
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  Service fully removed.")
	fmt.Println("═══════════════════════════════════════════════════════════")
	return nil
}

// --- Linux install/uninstall ---

func installLinux(home, binaryPath string) error {
	systemdDir := filepath.Join(home, ".config", "systemd", "user")
	wrapperDest := filepath.Join(home, ".local", "bin", "gcal-organizer-wrapper.sh")

	// Create wrapper
	if err := os.MkdirAll(filepath.Dir(wrapperDest), 0755); err != nil {
		return fmt.Errorf("failed to create wrapper directory: %w", err)
	}
	wrapper := generateWrapper(binaryPath)
	if err := os.WriteFile(wrapperDest, []byte(wrapper), 0755); err != nil {
		return fmt.Errorf("failed to write wrapper script: %w", err)
	}
	fmt.Println("  ✅ Created wrapper script")

	// Copy browser automation scripts
	if err := copyBrowserDir(home); err != nil {
		fmt.Printf("  ⚠️  Browser automation not installed: %v\n", err)
		fmt.Println("     Task assignment via browser will not work in service mode.")
		fmt.Println("     Run 'gcal-organizer install' from the project directory to fix.")
	} else {
		fmt.Println("  ✅ Installed browser automation scripts")
	}

	// Create systemd directory
	if err := os.MkdirAll(systemdDir, 0755); err != nil {
		return fmt.Errorf("failed to create systemd directory: %w", err)
	}

	// Write service unit
	service := generateSystemdService(wrapperDest, home)
	if err := os.WriteFile(filepath.Join(systemdDir, "gcal-organizer.service"), []byte(service), 0644); err != nil {
		return fmt.Errorf("failed to write service unit: %w", err)
	}
	fmt.Println("  ✅ Created systemd service")

	// Write timer unit
	timer := generateSystemdTimer()
	if err := os.WriteFile(filepath.Join(systemdDir, "gcal-organizer.timer"), []byte(timer), 0644); err != nil {
		return fmt.Errorf("failed to write timer unit: %w", err)
	}
	fmt.Println("  ✅ Created systemd timer")

	// Enable and start
	// Intentionally ignore daemon-reload error: best-effort before enable.
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	if err := exec.Command("systemctl", "--user", "enable", "--now", "gcal-organizer.timer").Run(); err != nil {
		return fmt.Errorf("failed to enable timer: %w\n   Fix: Check 'systemctl --user status gcal-organizer.timer'", err)
	}
	fmt.Println("  ✅ Timer enabled and started")

	fmt.Println()
	fmt.Println("───────────────────────────────────────────────────────────")
	fmt.Println("  Logs:    journalctl --user -u gcal-organizer.service")
	fmt.Println("  Status:  gcal-organizer doctor")
	fmt.Println("  Remove:  gcal-organizer uninstall")
	fmt.Println("═══════════════════════════════════════════════════════════")
	return nil
}

func uninstallLinux(home string) error {
	systemdDir := filepath.Join(home, ".config", "systemd", "user")
	wrapperDest := filepath.Join(home, ".local", "bin", "gcal-organizer-wrapper.sh")

	// Intentionally ignore disable error: the timer may not be enabled.
	_ = exec.Command("systemctl", "--user", "disable", "--now", "gcal-organizer.timer").Run()
	fmt.Println("  ✅ Timer stopped and disabled")

	if err := os.Remove(filepath.Join(systemdDir, "gcal-organizer.service")); err != nil && !os.IsNotExist(err) {
		fmt.Printf("  ⚠️  Could not remove service unit: %v\n", err)
	} else {
		fmt.Println("  ✅ Removed service unit")
	}
	if err := os.Remove(filepath.Join(systemdDir, "gcal-organizer.timer")); err != nil && !os.IsNotExist(err) {
		fmt.Printf("  ⚠️  Could not remove timer unit: %v\n", err)
	} else {
		fmt.Println("  ✅ Removed timer unit")
	}

	if err := os.Remove(wrapperDest); err != nil && !os.IsNotExist(err) {
		fmt.Printf("  ⚠️  Could not remove wrapper script: %v\n", err)
	} else {
		fmt.Println("  ✅ Removed wrapper script")
	}

	// Intentionally ignore daemon-reload error: best-effort cleanup.
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  Service fully removed.")
	fmt.Println("═══════════════════════════════════════════════════════════")
	return nil
}

// --- Embedded service templates ---

func generateWrapper(binaryPath string) string {
	// Validate that binaryPath does not contain control characters that could
	// break out of shell quoting or cause unexpected behaviour.
	if strings.ContainsAny(binaryPath, "\n\r\x00") {
		// This should never happen with os.Executable(), but defend in depth.
		panic(fmt.Sprintf("binary path contains control characters: %q", binaryPath))
	}
	// Single-quote binaryPath so spaces and shell metacharacters in the install
	// path are handled safely. Embed any literal single-quotes via '\''.
	quotedBin := "'" + strings.ReplaceAll(binaryPath, "'", "'\\''") + "'"
	// FR-017: wrapper script does not source .env — the binary loads config
	// from config.yaml internally.
	return fmt.Sprintf(`#!/bin/bash
# gcal-organizer service wrapper
# Generated by 'gcal-organizer install'

set -euo pipefail

# Override days to look back for service mode (1 day)
export GCAL_DAYS_TO_LOOK_BACK=1

# --- Log rotation ---
# Rotate log when it exceeds 5 MB, keeping one backup (.1).
# Max disk usage: ~10 MB (5 MB active + 5 MB rotated).
LOG_FILE="${HOME}/Library/Logs/gcal-organizer.log"
MAX_LOG_BYTES=$((5 * 1024 * 1024))  # 5 MB
if [ -f "$LOG_FILE" ]; then
    LOG_SIZE=$(stat -f%%z "$LOG_FILE" 2>/dev/null || stat --format=%%s "$LOG_FILE" 2>/dev/null || echo 0)
    if [ "$LOG_SIZE" -gt "$MAX_LOG_BYTES" ]; then
        mv "$LOG_FILE" "${LOG_FILE}.1"
        echo "$(date '+%%Y-%%m-%%d %%H:%%M:%%S') — Log rotated (was ${LOG_SIZE} bytes)"
    fi
fi

echo "$(date '+%%Y-%%m-%%d %%H:%%M:%%S') — Starting gcal-organizer run"
echo "%s run"
%s run && RC=$? || RC=$?
echo "Exit code: $RC"
echo "$(date '+%%Y-%%m-%%d %%H:%%M:%%S') — Completed gcal-organizer run"
exit $RC
`, quotedBin, quotedBin)
}

func generatePlist(wrapperPath, logPath, home, binaryPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.jflowers.gcal-organizer</string>

    <key>ProgramArguments</key>
    <array>
        <string>/bin/bash</string>
        <string>%s</string>
    </array>

    <key>StartInterval</key>
    <integer>3600</integer>

    <key>RunAtLoad</key>
    <true/>

    <key>StandardOutPath</key>
    <string>%s</string>

    <key>StandardErrorPath</key>
    <string>%s</string>

    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin</string>
        <key>HOME</key>
        <string>%s</string>
        <key>GCAL_ORGANIZER_BIN</key>
        <string>%s</string>
    </dict>
</dict>
</plist>
`, wrapperPath, logPath, logPath, home, binaryPath)
}

func generateSystemdService(wrapperPath, home string) string {
	// FR-018: EnvironmentFile directive removed — the binary loads config
	// from config.yaml internally. ExecStart with double-quotes is correct
	// per systemd.exec(5) for paths containing spaces.
	return fmt.Sprintf(`[Unit]
Description=GCal Organizer - Meeting note organization
Documentation=https://github.com/jflowers/gcal-organizer

[Service]
Type=oneshot
ExecStart=/bin/bash "%s"
Environment=HOME=%s

[Install]
WantedBy=default.target
`, wrapperPath, home)
}

func generateSystemdTimer() string {
	return `[Unit]
Description=Run gcal-organizer hourly

[Timer]
OnCalendar=hourly
Persistent=true
RandomizedDelaySec=120

[Install]
WantedBy=timers.target
`
}

// setupBrowserCmd guides the user through browser setup for task assignment.
var setupBrowserCmd = &cobra.Command{
	Use:   "setup-browser",
	Short: "Set up Chrome for browser-based task assignment",
	Long: `Launch Chrome with remote debugging using a dedicated profile.

This command:
  1. Checks Node.js is installed
  2. Creates/uses a dedicated gcal-organizer Chrome profile
  3. Checks browser automation dependencies (npm install)
  4. Launches Chrome with --remote-debugging-port=9222
  5. Guides you to sign in to Google (first run only)
  6. Verifies Chrome is accessible via CDP`,
	RunE: func(cmd *cobra.Command, args []string) error {

		fmt.Println(styledTitle("🌐 gcal-organizer setup-browser"))
		fmt.Println()

		// Step 1: Check Node.js
		fmt.Println(subtleStyle.Render("  Step 1/5: Checking Node.js..."))
		nodeOut, err := exec.Command("node", "--version").Output()
		if err != nil {
			fmt.Println(styledFail("Node.js is required but not found"))
			fmt.Println(styledFix("Install Node.js 18+ from https://nodejs.org"))
			return fmt.Errorf("Node.js is required for browser automation")
		}
		fmt.Println(styledPass(fmt.Sprintf("Node.js %s", strings.TrimSpace(string(nodeOut)))))

		// Step 2: Dedicated Chrome profile
		fmt.Println()
		fmt.Println(subtleStyle.Render("  Step 2/5: Setting up dedicated Chrome profile..."))

		chromePath, chromeErr := chromeProfilePath()
		if chromeErr != nil {
			fmt.Println(styledFail("Could not determine Chrome profile path: " + chromeErr.Error()))
			return fmt.Errorf("Chrome profile detection failed: %w", chromeErr)
		}

		firstRun := false
		if _, err := os.Stat(chromePath); os.IsNotExist(err) {
			fmt.Println(styledPass("Will create dedicated profile: gcal-organizer"))
			firstRun = true
		} else {
			fmt.Println(styledPass("Dedicated profile found: gcal-organizer"))
		}

		// Step 3: Check browser deps (non-blocking — profile setup is more important)
		fmt.Println()
		fmt.Println(subtleStyle.Render("  Step 3/5: Checking browser automation dependencies..."))
		browserDir, _ := findBrowserDir()
		if browserDir == "" {
			fmt.Println(styledWarn("Browser directory not found"))
			fmt.Println(styledFix("Install browser automation or run from project root for Step 3 (task assignment)"))
		} else {
			nodeModules := filepath.Join(browserDir, "node_modules")
			if _, err := os.Stat(nodeModules); os.IsNotExist(err) {
				fmt.Println(subtleStyle.Render("     Installing npm dependencies..."))
				npmCmd := exec.Command("npm", "install")
				npmCmd.Dir = browserDir
				npmCmd.Stdout = os.Stdout
				npmCmd.Stderr = os.Stderr
				if err := npmCmd.Run(); err != nil {
					fmt.Println(styledWarn("npm install failed — task assignment may not work"))
				} else {
					fmt.Println(styledPass("Browser dependencies installed"))
				}
			} else {
				fmt.Println(styledPass("Browser dependencies already installed"))
			}
		}

		// Step 4: Launch Chrome with debugging
		fmt.Println()

		// Step 3b: Flatpak filesystem access check
		if isFlatpakChrome() && !hasFlatpakFilesystemAccess() {
			fmt.Println(styledWarn("Flatpak Chrome detected — filesystem access required"))
			fmt.Println()
			fmt.Println(boxStyle.Render(
				"  Chrome is sandboxed by Flatpak and cannot access\n" +
					"  ~/.gcal-organizer/chrome-data/ by default.\n\n" +
					"  Run this command to grant access:\n\n" +
					"  flatpak override --user --filesystem=~/.gcal-organizer com.google.Chrome\n\n" +
					"  Then re-run 'gcal-organizer setup-browser'."))
			return fmt.Errorf("Flatpak Chrome needs filesystem access to ~/.gcal-organizer/")
		}

		fmt.Println(subtleStyle.Render("  Step 4/5: Launching Chrome with remote debugging..."))

		if isPortOpen(9222) {
			fmt.Println(styledPass("Chrome is already running on port 9222"))
		} else {
			chromeCmd, err := launchChrome(chromePath)
			if err != nil {
				fmt.Println(styledFail("Failed to launch Chrome"))
				return err
			}
			_ = chromeCmd // Process continues in background

			// Wait for port to be ready
			ready := false
			for i := 0; i < 20; i++ {
				time.Sleep(500 * time.Millisecond)
				if isPortOpen(9222) {
					ready = true
					break
				}
			}
			if !ready {
				fmt.Println(styledWarn("Chrome started but port 9222 not yet ready"))
				fmt.Println(subtleStyle.Render("     It may take a moment. Check chrome://version in the browser."))
			} else {
				fmt.Println(styledPass("Chrome is running with remote debugging on port 9222"))
			}
		}

		// Step 5: Prompt user to authenticate (first run only)
		fmt.Println()
		fmt.Println(subtleStyle.Render("  Step 5/5: Google authentication"))
		fmt.Println()
		if firstRun {
			fmt.Println(boxStyle.Render(
				"  A new Chrome window opened with a fresh profile.\n" +
					"  This profile is dedicated to gcal-organizer.\n\n" +
					"  1. Sign in with your Google account\n" +
					"  2. Go to docs.google.com\n" +
					"  3. Verify you can see your documents\n\n" +
					"  Press Enter when done..."))
		} else {
			fmt.Println(boxStyle.Render(
				"  Chrome opened with your gcal-organizer profile.\n" +
					"  You should already be signed in.\n\n" +
					"  Press Enter to verify..."))
		}
		fmt.Println()

		reader := bufio.NewReader(os.Stdin)
		if _, err := reader.ReadString('\n'); err != nil {
			// Non-interactive (stdin closed) — continue without waiting
			fmt.Println(styledWarn("stdin not available — skipping interactive pause"))
		}

		// Verify CDP is accessible
		if isPortOpen(9222) {
			fmt.Println(styledPass("Chrome debugging port is active"))
		} else {
			fmt.Println(styledWarn("Chrome debugging port not responding"))
			fmt.Println(styledFix("Make sure Chrome is still running"))
		}

		fmt.Println()
		fmt.Println(boxStyle.Render(
			successStyle.Render("  ✅ Browser setup complete!") + "\n\n" +
				"  Chrome is running with debugging enabled.\n" +
				"  Keep this Chrome window open for task assignment.\n\n" +
				subtleStyle.Render("  Test with: gcal-organizer run --dry-run")))
		return nil
	},
}

// --- Additional helper functions ---
// findBrowserDir is defined in assign_tasks.go.

// isPortOpen checks if a TCP port is listening on localhost.
func isPortOpen(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// findChromeBinary locates the Chrome executable for the current OS.
func findChromeBinary() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	case "linux":
		// Try standard Chrome binaries
		for _, bin := range []string{"google-chrome", "google-chrome-stable", "chromium-browser"} {
			if p, err := exec.LookPath(bin); err == nil {
				return p
			}
		}
		// Try Flatpak Chrome
		if _, err := exec.LookPath("flatpak"); err == nil {
			out, err := exec.Command("flatpak", "info", "com.google.Chrome").Output()
			if err == nil && len(out) > 0 {
				return "flatpak-chrome" // sentinel handled in launchChrome
			}
		}
	}
	return ""
}

// launchChrome starts Chrome with remote debugging on port 9222.
func launchChrome(profilePath string) (*exec.Cmd, error) {
	chromeBin := findChromeBinary()
	if chromeBin == "" {
		return nil, fmt.Errorf("Chrome not found. Install Google Chrome and try again")
	}

	var cmd *exec.Cmd
	if chromeBin == "flatpak-chrome" {
		// Flatpak Chrome needs special invocation
		cmd = exec.Command("flatpak", "run", "com.google.Chrome",
			"--remote-debugging-port=9222",
			"--user-data-dir="+profilePath,
			"https://docs.google.com",
		)
	} else {
		cmd = exec.Command(chromeBin,
			"--remote-debugging-port=9222",
			"--user-data-dir="+profilePath,
			"https://docs.google.com",
		)
	}
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to launch Chrome: %w", err)
	}

	return cmd, nil
}

// isFlatpakChrome returns true if Chrome is installed via Flatpak.
func isFlatpakChrome() bool {
	return findChromeBinary() == "flatpak-chrome"
}

// hasFlatpakFilesystemAccess checks if the Flatpak Chrome override grants
// access to ~/.gcal-organizer/. It checks the user-level overrides file.
func hasFlatpakFilesystemAccess() bool {
	home := mustUserHomeDir()
	overridesFile := filepath.Join(home, ".local", "share", "flatpak", "overrides", "com.google.Chrome")

	data, err := os.ReadFile(overridesFile)
	if err != nil {
		return false
	}

	// Check if the overrides file contains a filesystem grant for ~/.gcal-organizer
	content := string(data)
	return strings.Contains(content, "~/.gcal-organizer") ||
		strings.Contains(content, home+"/.gcal-organizer")
}
