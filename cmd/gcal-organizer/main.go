/*
Package main provides the entry point for the gcal-organizer CLI.

gcal-organizer is a tool that automates the lifecycle of meeting notes by:
  - Organizing Google Drive documents into topic-based folders
  - Syncing calendar event attachments to meeting folders
  - Sharing meeting folders with calendar attendees
*/
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jflowers/gcal-organizer/internal/auth"
	"github.com/jflowers/gcal-organizer/internal/calendar"
	"github.com/jflowers/gcal-organizer/internal/config"
	"github.com/jflowers/gcal-organizer/internal/drive"
	"github.com/jflowers/gcal-organizer/internal/logging"
	"github.com/jflowers/gcal-organizer/internal/organizer"
	"github.com/jflowers/gcal-organizer/internal/ux"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// Version is set at build time
	Version = "dev"

	// Global flags
	cfgFile   string
	verbose   bool
	dryRun    bool
	ownedOnly bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gcal-organizer",
	Short: "Organize meeting notes and extract action items",
	Long: `gcal-organizer automates the lifecycle of meeting notes by:

  • Organizing Google Drive documents into topic-based folders
  • Syncing calendar event attachments to meeting folders
  • Using Gemini AI to extract action items from checkboxes
  • Creating Google Tasks from extracted action items

Use the subcommands to run specific operations or 'run' for the full workflow.`,
	Version: Version,
}

// initServices initializes all Google API services and returns an Organizer.
func initServices(ctx context.Context, cfg *config.Config) (*organizer.Organizer, error) {
	oauthClient, err := auth.NewOAuthClient(cfg.CredentialsFile, cfg.TokenFile)
	if err != nil {
		return nil, ux.OAuthSetupFailed(cfg.CredentialsFile)
	}

	httpClient, err := oauthClient.GetClient(ctx)
	if err != nil {
		return nil, ux.AuthFailed()
	}

	driveSvc, err := drive.NewService(ctx, httpClient, cfg.FilenamePattern, cfg.DryRun, cfg.Verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Drive service: %w\n\nRun 'gcal-organizer doctor' for diagnostics", err)
	}

	calSvc, err := calendar.NewService(ctx, httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Calendar service: %w\n\nRun 'gcal-organizer doctor' for diagnostics", err)
	}

	return organizer.New(cfg, driveSvc, calSvc), nil
}

// runCmd represents the run command (full workflow)
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the complete workflow",
	Long:  `Execute all operations: organize documents, sync calendar attachments, and assign tasks.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w\n\nRun 'gcal-organizer doctor' for diagnostics", err)
		}
		cfg.DryRun = dryRun
		cfg.Verbose = verbose
		cfg.OwnedOnly = ownedOnly

		days, _ := cmd.Flags().GetInt("days")
		if days > 0 {
			if days > 365 {
				return fmt.Errorf("--days must be 365 or fewer (got %d)", days)
			}
			cfg.DaysToLookBack = days
		}

		org, err := initServices(ctx, cfg)
		if err != nil {
			return err
		}

		// Steps 1 & 2: Organize documents + Sync calendar
		if err := org.RunFullWorkflow(ctx); err != nil {
			return err
		}

		// Step 3: Assign tasks from collected Notes documents
		docIDs := org.GetNotesDocIDs()
		if ownedOnly && len(docIDs) == 0 {
			fmt.Println("📝 STEP 3: No owned Notes documents found for task assignment")
		}
		if len(docIDs) > 0 && !dryRun {
			fmt.Println("📝 STEP 3: Assigning Tasks")
			fmt.Println("───────────────────────────────────────────────────────────")
			fmt.Printf("   Found %d Notes documents to scan for tasks\n", len(docIDs))

			totalAssigned := 0
			totalFailed := 0

			for _, docID := range docIDs {
				assigned, failed, err := runAssignTasksForDoc(ctx, cfg, docID)
				if err != nil {
					fmt.Printf("   ⚠️  Error processing doc %s: %v\n", docID[:min(8, len(docID))], err)
					continue
				}
				totalAssigned += assigned
				totalFailed += failed
			}

			org.AddTaskStats(totalAssigned, totalFailed)
			fmt.Println()
		} else if len(docIDs) > 0 && dryRun {
			fmt.Printf("📝 STEP 3: Would scan %d Notes documents for task assignments\n", len(docIDs))
			fmt.Println()
		}

		// Print final summary
		org.PrintSummary()

		return nil
	},
}

// organizeCmd represents the organize command
var organizeCmd = &cobra.Command{
	Use:   "organize",
	Short: "Organize meeting documents into folders",
	Long:  `Scan Google Drive for meeting notes and organize them into topic-based subfolders.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		cfg.DryRun = dryRun
		cfg.Verbose = verbose
		cfg.OwnedOnly = ownedOnly

		org, err := initServices(ctx, cfg)
		if err != nil {
			return err
		}

		if dryRun {
			fmt.Println("═══════════════════════════════════════════════════════════")
			fmt.Println("🔍 DRY RUN MODE - No changes will be made")
			fmt.Println("═══════════════════════════════════════════════════════════")
		}

		return org.OrganizeDocuments(ctx)
	},
}

// syncCalendarCmd represents the sync-calendar command
var syncCalendarCmd = &cobra.Command{
	Use:   "sync-calendar",
	Short: "Sync calendar attachments to meeting folders",
	Long:  `Scan recent calendar events and sync any attached documents to corresponding meeting folders.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		days, _ := cmd.Flags().GetInt("days")
		if days > 0 {
			if days > 365 {
				return fmt.Errorf("--days must be 365 or fewer (got %d)", days)
			}
			cfg.DaysToLookBack = days
		}
		cfg.DryRun = dryRun
		cfg.Verbose = verbose
		cfg.OwnedOnly = ownedOnly

		org, err := initServices(ctx, cfg)
		if err != nil {
			return err
		}

		if dryRun {
			fmt.Println("═══════════════════════════════════════════════════════════")
			fmt.Println("🔍 DRY RUN MODE - No changes will be made")
			fmt.Println("═══════════════════════════════════════════════════════════")
		}

		return org.SyncCalendarAttachments(ctx)
	},
}

// assignTasksCmd is defined in assign_tasks.go.

// truncateText shortens s to at most maxLen runes, appending "..." if truncated.
// Operates on runes (not bytes) to correctly handle multi-byte UTF-8 characters
// that are common in task text (names, emoji, non-ASCII punctuation).
// maxLen < 3 is clamped to 3 to avoid a negative-index panic in the slice expression.
// Defined here because it is used by both main.go (run command) and assign_tasks.go.
func truncateText(s string, maxLen int) string {
	if maxLen < 3 {
		maxLen = 3
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen-3]) + "..."
}

// configCmd, authCmd, and related sub-commands are defined in auth_config.go.
// assignTasksCmd and its helper functions are defined in assign_tasks.go.

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.gcal-organizer/.env)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "show what would be done without making changes")
	rootCmd.PersistentFlags().BoolVar(&ownedOnly, "owned-only", false, "only mutate files you own; skip non-owned files")

	// Bind flags to viper
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("dry-run", rootCmd.PersistentFlags().Lookup("dry-run"))
	viper.BindPFlag("owned-only", rootCmd.PersistentFlags().Lookup("owned-only"))

	// Add flags to specific commands
	syncCalendarCmd.Flags().Int("days", 8, "number of days to look back for calendar events")
	runCmd.Flags().Int("days", 0, "number of days to look back for calendar events (overrides GCAL_DAYS_TO_LOOK_BACK)")
	assignTasksCmd.Flags().String("doc", "", "Google Doc ID to process (required)")

	// Add subcommands
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(organizeCmd)
	rootCmd.AddCommand(syncCalendarCmd)
	rootCmd.AddCommand(assignTasksCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(setupBrowserCmd)

	configCmd.AddCommand(configShowCmd)
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)

	// Init command flags
	initCmd.Flags().Bool("non-interactive", false, "skip interactive prompts")
	initCmd.Flags().String("api-key", "", "Gemini API key (skips prompt)")
}

func initConfig() {
	// Load .env file into process environment so viper picks up the values
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	envFile := cfgFile
	if envFile == "" {
		envFile = filepath.Join(home, ".gcal-organizer", ".env")
	}

	loadDotEnv(envFile, home)

	viper.AutomaticEnv()

	// Wire --verbose to charm log level
	logging.SetVerbose(verbose)
}

// loadDotEnv reads a .env file and sets any KEY=VALUE pairs as environment
// variables, but only if they are not already set (env vars take precedence).
// Tilde (~) in values is expanded to the user's home directory.
func loadDotEnv(path, home string) {
	f, err := os.Open(path)
	if err != nil {
		return // .env is optional
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// Strip surrounding quotes (double or single) for bash compatibility.
		// For single-quoted values also unescape the POSIX '\'' sequence that
		// generateEnvFile uses to embed a literal single-quote in the value.
		if len(val) >= 2 {
			switch {
			case val[0] == '"' && val[len(val)-1] == '"':
				val = val[1 : len(val)-1]
			case val[0] == '\'' && val[len(val)-1] == '\'':
				val = val[1 : len(val)-1]
				// Unescape '\'' → ' (POSIX single-quote escape sequence)
				val = strings.ReplaceAll(val, `'\''`, `'`)
			}
		}

		// Expand ~ to home directory
		if strings.HasPrefix(val, "~/") {
			val = home + val[1:]
		} else if val == "~" {
			val = home
		}

		// Only set if not already in environment (explicit env vars win)
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
		}
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
