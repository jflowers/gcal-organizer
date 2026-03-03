/*
Package main provides the entry point for the gcal-organizer CLI.

gcal-organizer is a tool that automates the lifecycle of meeting notes by:
  - Organizing Google Drive documents into topic-based folders
  - Syncing calendar event attachments to meeting folders
  - Sharing meeting folders with calendar attendees
*/
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/jflowers/gcal-organizer/internal/auth"
	"github.com/jflowers/gcal-organizer/internal/calendar"
	"github.com/jflowers/gcal-organizer/internal/config"
	"github.com/jflowers/gcal-organizer/internal/drive"
	"github.com/jflowers/gcal-organizer/internal/logging"
	"github.com/jflowers/gcal-organizer/internal/organizer"
	"github.com/jflowers/gcal-organizer/internal/secrets"
	"github.com/jflowers/gcal-organizer/internal/ux"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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

// mustBindPFlag wraps viper.BindPFlag and panics on error. Errors here indicate
// a programming mistake (typo in flag name) and should surface at startup.
func mustBindPFlag(key string, flag *pflag.Flag) {
	if err := viper.BindPFlag(key, flag); err != nil {
		panic(fmt.Sprintf("viper.BindPFlag(%q) failed: %v", key, err))
	}
}

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

// loadConfigAndStore loads configuration and creates a SecretStore.
// This is the standard startup sequence for all commands that need secrets.
// Returns the backend so callers can display it without re-probing the keychain.
func loadConfigAndStore() (*config.Config, secrets.SecretStore, secrets.Backend, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, 0, err
	}
	store, backend := secrets.NewStore(cfg.NoKeyring)
	cfg.LoadSecrets(store)
	return cfg, store, backend, nil
}

// initServices initializes all Google API services and returns an Organizer.
func initServices(ctx context.Context, cfg *config.Config, store secrets.SecretStore) (*organizer.Organizer, error) {
	oauthClient, err := auth.NewOAuthClient(store, cfg.CredentialsFile)
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
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		cfg, store, _, err := loadConfigAndStore()
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

		org, err := initServices(ctx, cfg, store)
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

			// Initialize Docs+Gemini once for all documents to avoid
			// redundant OAuth/Gemini client creation per document.
			taskDocsSvc, taskGeminiClient, taskInitErr := initDocsAndGemini(ctx, cfg, store)
			if taskInitErr != nil {
				fmt.Printf("   ⚠️  Error initializing services for Step 3: %v\n", taskInitErr)
			} else {
				totalAssigned := 0
				totalFailed := 0

				for _, docID := range docIDs {
					assigned, failed, err := runAssignTasksForDoc(ctx, cfg, taskDocsSvc, taskGeminiClient, docID)
					if err != nil {
						fmt.Printf("   ⚠️  Error processing doc %s: %v\n", docID[:min(8, len(docID))], err)
						continue
					}
					totalAssigned += assigned
					totalFailed += failed
				}

				org.AddTaskStats(totalAssigned, totalFailed)
			}
			fmt.Println()
		} else if len(docIDs) > 0 && dryRun {
			fmt.Printf("📝 STEP 3: Would scan %d Notes documents for task assignments\n", len(docIDs))
			fmt.Println()
		}

		// Step 4: Extract Decisions from transcript documents
		decisionDocIDs := org.GetDecisionDocIDs()
		if ownedOnly && len(decisionDocIDs) == 0 {
			fmt.Println("📋 STEP 4: No owned transcript documents found for decision extraction")
		}
		if len(decisionDocIDs) > 0 {
			if dryRun {
				fmt.Printf("📋 STEP 4: Would extract decisions from %d transcript documents\n", len(decisionDocIDs))
			} else {
				fmt.Println("📋 STEP 4: Extracting Decisions")
				fmt.Println("───────────────────────────────────────────────────────────")
				fmt.Printf("   Found %d transcript documents to process\n", len(decisionDocIDs))
			}

			// Initialize services once for all documents
			docsSvc, geminiClient, initErr := initDocsAndGemini(ctx, cfg, store)
			if initErr != nil {
				fmt.Printf("   ⚠️  Error initializing services for Step 4: %v\n", initErr)
			} else {
				totalFailed := 0

				for docID, source := range decisionDocIDs {
					if !dryRun {
						fmt.Printf("   📄 Processing doc %s (source: %s)\n", docID[:min(8, len(docID))], source)
					}
					err := org.ExtractDecisionsForDoc(ctx, docID, docsSvc, geminiClient, dryRun)
					if err != nil {
						fmt.Printf("   ⚠️  Error processing doc %s: %v\n", docID[:min(8, len(docID))], err)
						totalFailed++
					}
				}

				// Only add externally-tracked failures; processed/skipped counts are
				// managed internally by ExtractDecisionsForDoc via organizer stats.
				org.AddDecisionStats(0, 0, totalFailed)
			}
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
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		cfg, store, _, err := loadConfigAndStore()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		cfg.DryRun = dryRun
		cfg.Verbose = verbose
		cfg.OwnedOnly = ownedOnly

		org, err := initServices(ctx, cfg, store)
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
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		cfg, store, _, err := loadConfigAndStore()
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

		org, err := initServices(ctx, cfg, store)
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

// truncateText is now ux.TruncateText in internal/ux/format.go.
// This wrapper preserves the local name for callers in this package.
func truncateText(s string, maxLen int) string {
	return ux.TruncateText(s, maxLen)
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
	rootCmd.PersistentFlags().Bool("no-keyring", false, "disable OS credential store; use file-based storage")

	// Bind flags to viper. Errors here indicate a programming mistake (typo in
	// flag name) and should surface immediately at startup.
	mustBindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	mustBindPFlag("dry-run", rootCmd.PersistentFlags().Lookup("dry-run"))
	mustBindPFlag("owned-only", rootCmd.PersistentFlags().Lookup("owned-only"))
	mustBindPFlag("no-keyring", rootCmd.PersistentFlags().Lookup("no-keyring"))

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

	config.LoadDotEnv(envFile, home)

	viper.AutomaticEnv()

	// Wire --verbose to charm log level
	logging.SetVerbose(verbose)
}

// loadDotEnv, validEnvKey, maskSecret, and truncateText have been extracted
// to internal/config/dotenv.go and internal/ux/format.go respectively.

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
