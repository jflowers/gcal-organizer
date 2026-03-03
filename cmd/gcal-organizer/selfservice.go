package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/jflowers/gcal-organizer/internal/secrets"
	"github.com/spf13/cobra"
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

		// 2. .env file
		envFile := filepath.Join(configDir, ".env")
		if _, err := os.Stat(envFile); err == nil {
			fmt.Println(styledPass("Environment file (.env) exists"))
			if verbose {
				fmt.Println(subtleStyle.Render("          " + envFile))
			}
			passed++
		} else {
			fmt.Println(styledFail("Environment file ~/.gcal-organizer/.env not found"))
			fmt.Println(styledFix("Run 'gcal-organizer init'"))
			failed++
		}

		// 3. credentials.json
		credFile := filepath.Join(configDir, "credentials.json")
		if _, err := os.Stat(credFile); err == nil {
			fmt.Println(styledPass("Google credentials (credentials.json) found"))
			if verbose {
				fmt.Println(subtleStyle.Render("          " + credFile))
			}
			passed++
		} else {
			fmt.Println(styledFail("Google credentials not found at " + credFile))
			fmt.Println(styledFix("Download from https://console.cloud.google.com/apis/credentials"))
			fmt.Println(subtleStyle.Render("          Save as ~/.gcal-organizer/credentials.json"))
			failed++
		}

		// 4. OAuth token (check store first, then file fallback)
		tokenFile := filepath.Join(configDir, "token.json")
		tokenFound := false
		// Try the secret store first (keychain or file-based)
		store, backend := secrets.NewStore(false)
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
				// Use a closure so f.Close() runs immediately after use,
				// not deferred to the end of the enclosing RunE function.
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

		// 5. GEMINI_API_KEY
		apiKey := os.Getenv("GEMINI_API_KEY")
		if apiKey == "" {
			// Try loading from .env
			apiKey = loadEnvValue(envFile, "GEMINI_API_KEY")
		}
		if apiKey != "" && apiKey != "your-gcp-api-key-here" {
			// Use min() to avoid a panic if the key is shorter than 4 characters.
			prefix := apiKey[:min(4, len(apiKey))]
			fmt.Println(styledPass(fmt.Sprintf("GEMINI_API_KEY is set (%s****)", prefix)))
			passed++
		} else if apiKey == "your-gcp-api-key-here" {
			fmt.Println(styledFail("GEMINI_API_KEY is still set to placeholder value"))
			fmt.Println(styledFix("Get your API key from https://aistudio.google.com/app/apikey"))
			failed++
		} else {
			fmt.Println(styledFail("GEMINI_API_KEY is not set"))
			fmt.Println(styledFix("Run 'gcal-organizer init' or set in ~/.gcal-organizer/.env"))
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
	Long:  `Create the config directory and generate an environment file with your API keys.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}
		configDir := filepath.Join(home, ".gcal-organizer")
		envFile := filepath.Join(configDir, ".env")
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

		// 2. Generate .env file
		if _, err := os.Stat(envFile); os.IsNotExist(err) {
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

			// Write .env
			envContent := generateEnvFile(apiKey)
			if err := os.WriteFile(envFile, []byte(envContent), 0600); err != nil {
				return fmt.Errorf("failed to write .env file: %w", err)
			}
			fmt.Println(styledPass("Created ~/.gcal-organizer/.env"))
		} else {
			fmt.Println(styledPass("Environment file already exists (skipped)"))
			existing := loadEnvValue(envFile, "GEMINI_API_KEY")
			if existing == "your-gcp-api-key-here" {
				fmt.Println(styledWarn("GEMINI_API_KEY is still set to placeholder — edit ~/.gcal-organizer/.env"))
			}
		}

		// 3. Check for credentials.json
		credFile := filepath.Join(configDir, "credentials.json")
		if _, err := os.Stat(credFile); os.IsNotExist(err) {
			fmt.Println()
			fmt.Println(styledWarn("credentials.json not found"))
			fmt.Println(styledFix("Download OAuth credentials from Google Cloud Console:"))
			fmt.Println(subtleStyle.Render("     https://console.cloud.google.com/apis/credentials"))
			fmt.Println(subtleStyle.Render("     Save as: ~/.gcal-organizer/credentials.json"))
		} else {
			fmt.Println(styledPass("Google credentials found"))
		}

		fmt.Println()
		var nextSteps string
		if _, err := os.Stat(credFile); os.IsNotExist(err) {
			nextSteps = "  Next steps:\n  1. Download credentials.json (see above)\n  2. Run 'gcal-organizer auth login'"
		} else {
			tokenFile := filepath.Join(configDir, "token.json")
			if _, err := os.Stat(tokenFile); os.IsNotExist(err) {
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

// generateEnvFile creates the .env file content.
// All values are double-quoted for bash source compatibility.
func generateEnvFile(apiKey string) string {
	home := mustUserHomeDir()
	var b strings.Builder
	b.WriteString("# GCal Organizer Configuration\n")
	b.WriteString("# Generated by 'gcal-organizer init'\n\n")
	b.WriteString("# Required: GCP API Key for Gemini AI\n")
	// Single-quote the value so shell treats it as a literal string, preventing
	// injection via $, `, or " characters in the API key. A single-quote inside
	// the value is escaped with the POSIX '\'' sequence, which closes the
	// current single-quoted string, inserts a literal ', then re-opens it.
	escapedKey := strings.ReplaceAll(apiKey, "'", "'\\''")
	b.WriteString(fmt.Sprintf("GEMINI_API_KEY='%s'\n\n", escapedKey))
	b.WriteString("# Required: Path to Google OAuth2 credentials\n")
	b.WriteString(fmt.Sprintf("GOOGLE_CREDENTIALS_FILE=\"%s/.gcal-organizer/credentials.json\"\n\n", home))
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
	return fmt.Sprintf(`#!/bin/bash
# gcal-organizer service wrapper
# Generated by 'gcal-organizer install'

set -euo pipefail

# Source env file if it exists
ENV_FILE="${HOME}/.gcal-organizer/.env"
if [ -f "$ENV_FILE" ]; then
    set -a
    source "$ENV_FILE"
    set +a
fi

# Override days to look back for service mode (1 day)
export GCAL_DAYS_TO_LOOK_BACK=1

# --- Log rotation (FR-016) ---
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
%s run
echo "$(date '+%%Y-%%m-%%d %%H:%%M:%%S') — Completed gcal-organizer run"
`, quotedBin)
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
	// systemd honours double-quotes around ExecStart arguments to handle paths
	// containing spaces. EnvironmentFile does not support quoting, but the
	// leading '-' (suppress-missing) is outside the path, so we place the path
	// inside double-quotes immediately after the dash prefix isn't supported —
	// instead we document that home directories with spaces require a symlink.
	// ExecStart with double-quotes is correct per systemd.exec(5).
	return fmt.Sprintf(`[Unit]
Description=GCal Organizer - Meeting note organization
Documentation=https://github.com/jflowers/gcal-organizer

[Service]
Type=oneshot
ExecStart=/bin/bash "%s"
EnvironmentFile=-%s/.gcal-organizer/.env
Environment=HOME=%s

[Install]
WantedBy=default.target
`, wrapperPath, home, home)
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
