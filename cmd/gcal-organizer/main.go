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

	"net/url"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/jflowers/gcal-organizer/internal/auth"
	"github.com/jflowers/gcal-organizer/internal/calendar"
	"github.com/jflowers/gcal-organizer/internal/config"
	"github.com/jflowers/gcal-organizer/internal/drive"
	"github.com/jflowers/gcal-organizer/internal/export"
	"github.com/jflowers/gcal-organizer/internal/logging"
	"github.com/jflowers/gcal-organizer/internal/ollama"
	"github.com/jflowers/gcal-organizer/internal/organizer"
	"github.com/jflowers/gcal-organizer/internal/secrets"
	"github.com/jflowers/gcal-organizer/internal/ux"
	"github.com/mattn/go-isatty"
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

	// migrationPending is set by initConfig when a .env file is found
	// and needs to be migrated to config.yaml after secret migration.
	migrationPending bool
	// pendingEnvPath and pendingYAMLPath store paths for deferred migration.
	pendingEnvPath  string
	pendingYAMLPath string
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
//
// When the keychain backend is active, it runs auto-migration to transparently
// move any plaintext secrets (token.json, .env API key, credentials.json) into
// the OS credential store.
func loadConfigAndStore() (*config.Config, secrets.SecretStore, secrets.Backend, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, 0, err
	}
	store, backend := secrets.NewStore(cfg.NoKeyring)
	cfg.LoadSecrets(store)

	// Auto-migrate plaintext secrets to the credential store (T034).
	// Only runs when the keychain backend is active — migrating to file-based
	// storage is pointless.
	if backend == secrets.BackendKeychain {
		home, _ := os.UserHomeDir()
		configDir := filepath.Join(home, ".gcal-organizer")
		interactive := isatty.IsTerminal(os.Stdin.Fd())

		// Default prompt using huh for interactive credentials.json deletion
		promptFn := func(message string) (bool, error) {
			var confirm bool
			form := huh.NewForm(
				huh.NewGroup(
					huh.NewConfirm().
						Title(message).
						Affirmative("Yes, delete it").
						Negative("No, keep it").
						Value(&confirm),
				),
			)
			if err := form.Run(); err != nil {
				return false, err
			}
			return confirm, nil
		}

		if err := secrets.Migrate(store, configDir, interactive, cfg.Verbose, promptFn); err != nil {
			logging.Logger.Warn("Secret migration failed", "error", err)
		}
	}

	// D5: Run .env → config.yaml migration after secret migration completes.
	// This ensures secrets are safely in the keychain before .env is deleted.
	if migrationPending {
		home, _ := os.UserHomeDir()
		if err := config.MigrateEnvToYAML(pendingEnvPath, pendingYAMLPath, home); err != nil {
			logging.Logger.Warn("Config migration from .env to config.yaml failed", "error", err)
		} else {
			migrationPending = false
		}
	}

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

		// Initialize Ollama client and sensitivity gate when enabled.
		var ollamaClient *ollama.Client
		if cfg.Ollama.Enabled {
			ollamaClient = ollama.NewClient(cfg.Ollama.Endpoint, cfg.Ollama.Timeout)

			// S-1: Warn when Ollama endpoint is not localhost.
			if !isLocalEndpoint(cfg.Ollama.Endpoint) {
				logging.Logger.Warn("Ollama endpoint is not localhost — sensitive transcripts will be sent over the network",
					"endpoint", cfg.Ollama.Endpoint)
			}

			// FR-008: Hard-stop when Ollama configured but unavailable.
			if !ollamaClient.HealthCheck() {
				return fmt.Errorf("Ollama is configured but not available at %s\n\n"+
					"Fix steps:\n"+
					"  1. Install: brew install ollama\n"+
					"  2. Start:   ollama serve\n"+
					"  3. Verify:  ollama list\n\n"+
					"Or disable: set ollama.enabled=false in config.yaml", cfg.Ollama.Endpoint)
			}

			// FR-010: Validate sensitivity model availability.
			if cfg.Ollama.Sensitivity.Enabled {
				if !ollamaClient.ModelAvailable(cfg.Ollama.Sensitivity.Model) {
					return fmt.Errorf("Ollama sensitivity model %q is not available\n\n"+
						"Fix: ollama pull %s", cfg.Ollama.Sensitivity.Model, cfg.Ollama.Sensitivity.Model)
				}
				guardian := ollama.NewGuardian(ollamaClient, cfg.Ollama.Sensitivity.Model)
				org.SetSensitivityClassifier(guardian)
			}

			// Validate assignment model availability.
			if !ollamaClient.ModelAvailable(cfg.Ollama.Assignments.Model) {
				return fmt.Errorf("Ollama assignment model %q is not available\n\n"+
					"Fix: ollama pull %s", cfg.Ollama.Assignments.Model, cfg.Ollama.Assignments.Model)
			}
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

			// Initialize Docs service and assignee extractor.
			// When Ollama is enabled, use local assigner (FR-013).
			// Otherwise, use Gemini.
			taskDocsSvc, taskGeminiClient, taskInitErr := initDocsAndGemini(ctx, cfg, store)
			if taskInitErr != nil {
				fmt.Printf("   ⚠️  Error initializing services for Step 3: %v\n", taskInitErr)
			} else {
				var extractor AssigneeExtractor
				if cfg.Ollama.Enabled && ollamaClient != nil {
					extractor = ollama.NewAssigner(ollamaClient, cfg.Ollama.Assignments.Model)
					logging.Logger.Info("Using local AI for task assignment", "model", cfg.Ollama.Assignments.Model)
				} else {
					extractor = taskGeminiClient
				}

				totalAssigned := 0
				totalFailed := 0

				for _, docID := range docIDs {
					assigned, failed, err := runAssignTasksForDoc(ctx, cfg, taskDocsSvc, extractor, docID)
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
		decisionDocContexts := org.GetDecisionDocContexts()
		if ownedOnly && len(decisionDocContexts) == 0 {
			fmt.Println("📋 STEP 4: No owned transcript documents found for decision extraction")
		}
		if len(decisionDocContexts) > 0 {
			if dryRun {
				fmt.Printf("📋 STEP 4: Would extract decisions from %d transcript documents\n", len(decisionDocContexts))
			} else {
				fmt.Println("📋 STEP 4: Extracting Decisions")
				fmt.Println("───────────────────────────────────────────────────────────")
				fmt.Printf("   Found %d transcript documents to process\n", len(decisionDocContexts))
			}

			// FR-016: local-only mode requires Ollama enabled.
			if cfg.Ollama.LocalOnly && !cfg.Ollama.Enabled {
				return fmt.Errorf("local-only mode requires Ollama to be enabled\n\n" +
					"Fix: set ollama.enabled=true in config.yaml, or disable local_only")
			}

			// Create decision exporter with configured output directory
			decisionExporter := export.NewExporter(cfg.Decisions.ExportDir, logging.Logger)

			// Initialize services once for all documents.
			// When local-only mode is active, use local decision extractor
			// instead of Gemini (FR-016, FR-017).
			var geminiSvc organizer.GeminiService
			docsSvc, geminiClient, initErr := initDocsAndGemini(ctx, cfg, store)
			if initErr != nil {
				// In local-only mode, Gemini init failure is not fatal for decisions.
				// We still need docsSvc for transcript extraction and tab creation.
				if cfg.Ollama.Enabled && cfg.Ollama.LocalOnly && ollamaClient != nil {
					if docsSvc == nil {
						// initDocsAndGemini may have returned docsSvc even on Gemini failure,
						// but if OAuth/Docs also failed, try docs-only initialization.
						var docsErr error
						docsSvc, docsErr = initDocsOnly(ctx, cfg, store)
						if docsErr != nil {
							fmt.Printf("   ⚠️  Error initializing Docs service for Step 4: %v\n", docsErr)
						}
					}
					logging.Logger.Debug("Gemini init skipped, using local-only mode for decisions", "error", initErr)
				} else {
					fmt.Printf("   ⚠️  Error initializing services for Step 4: %v\n", initErr)
				}
			}

			if cfg.Ollama.Enabled && cfg.Ollama.LocalOnly && ollamaClient != nil {
				geminiSvc = ollama.NewDecisionExtractor(ollamaClient, cfg.Ollama.Assignments.Model)
				logging.Logger.Info("Using local AI for decision extraction", "model", cfg.Ollama.Assignments.Model)
			} else if geminiClient != nil {
				geminiSvc = geminiClient
			}

			if docsSvc != nil && geminiSvc != nil {
				totalFailed := 0

				for _, docCtx := range decisionDocContexts {
					// US2: Apply meeting allowlist filter before processing
					if !export.ShouldExportDecisions(docCtx.EventTitle, cfg.Decisions.Meetings) {
						logging.Logger.Debug("Skipping meeting not in allowlist", "title", docCtx.EventTitle)
						continue
					}

					// Sensitivity gate: classify transcript before processing (FR-001).
					// Inserted here (same level as allowlist filter) to keep
					// ExtractDecisionsForDoc focused on extraction, not gating.
					if cfg.Ollama.Enabled && cfg.Ollama.Sensitivity.Enabled {
						sensitivityResult, classifyErr := org.ClassifyTranscript(ctx, docCtx, docsSvc)
						if classifyErr != nil {
							// Hard-stop on classification failure (FR-009).
							return fmt.Errorf("sensitivity classification failed for doc %s: %w",
								docCtx.DocID[:min(8, len(docCtx.DocID))], classifyErr)
						}
						if sensitivityResult != nil {
							docURL := fmt.Sprintf("https://docs.google.com/document/d/%s/edit", docCtx.DocID)
							if sensitivityResult.Score >= cfg.Ollama.Sensitivity.Threshold {
								// FR-004: Log category+score+doc URL at INFO.
								logging.Logger.Info("Skipped sensitive transcript",
									"category", sensitivityResult.Category,
									"score", sensitivityResult.Score,
									"doc", docURL,
								)
								// FR-004: Reasoning at DEBUG only.
								logging.Logger.Debug("Sensitivity reasoning",
									"reasoning", sensitivityResult.Reasoning,
								)
								org.AddSensitivitySkipped()
								if !dryRun {
									// FR-007: In dry-run, log but proceed.
									continue
								}
								// FR-007: Dry-run proceeds with processing.
								logging.Logger.Info("Dry-run: would skip sensitive transcript, but proceeding")
							} else {
								org.AddSensitivityProcessed()
								logging.Logger.Debug("Transcript passed sensitivity gate",
									"category", sensitivityResult.Category,
									"score", sensitivityResult.Score,
									"doc", docURL,
								)
							}
						}
					}

					if !dryRun {
						fmt.Printf("   📄 Processing doc %s (source: %s)\n", docCtx.DocID[:min(8, len(docCtx.DocID))], docCtx.Source)
					}
					err := org.ExtractDecisionsForDoc(ctx, docCtx, docsSvc, geminiSvc, decisionExporter, dryRun)
					if err != nil {
						fmt.Printf("   ⚠️  Error processing doc %s: %v\n", docCtx.DocID[:min(8, len(docCtx.DocID))], err)
						totalFailed++
					}
				}

				// Only add externally-tracked failures; processed/skipped counts are
				// managed internally by ExtractDecisionsForDoc via organizer stats.
				org.AddDecisionStats(0, 0, totalFailed)
			} // end if docsSvc != nil && geminiSvc != nil
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
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.gcal-organizer/config.yaml)")
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
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	configDir := filepath.Join(home, ".gcal-organizer")

	// Determine config file paths based on --config flag or defaults.
	// D6: detect format by extension. .env or files named ".env" → legacy path.
	// .yaml/.yml → use directly. Default is config.yaml.
	var yamlPath, envPath string

	if cfgFile != "" {
		// User specified a config file via --config flag
		base := filepath.Base(cfgFile)
		ext := filepath.Ext(cfgFile)
		if base == ".env" || ext == ".env" {
			// D6: .env file specified — use legacy path, schedule migration
			envPath = cfgFile
			yamlPath = filepath.Join(filepath.Dir(cfgFile), "config.yaml")
		} else if ext == ".yaml" || ext == ".yml" {
			yamlPath = cfgFile
			envPath = filepath.Join(filepath.Dir(cfgFile), ".env")
		} else {
			// Unknown extension — treat as YAML
			yamlPath = cfgFile
			envPath = filepath.Join(filepath.Dir(cfgFile), ".env")
		}
	} else {
		yamlPath = filepath.Join(configDir, "config.yaml")
		envPath = filepath.Join(configDir, ".env")
	}

	// D5: Config loading flow
	if _, err := os.Stat(yamlPath); err == nil {
		// config.yaml exists — use viper's native YAML loading (FR-004)
		viper.SetConfigFile(yamlPath)
		viper.SetConfigType("yaml")
		if err := viper.ReadInConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: error reading config file %s: %v\n", yamlPath, err)
		}
	} else if _, err := os.Stat(envPath); err == nil {
		// .env exists but config.yaml does not — legacy path for first run.
		// Load .env into process environment so viper picks up values.
		// Migration will run after secret migration in loadConfigAndStore().
		config.LoadDotEnv(envPath, home)
		migrationPending = true
		pendingEnvPath = envPath
		pendingYAMLPath = yamlPath
	}
	// else: neither exists — defaults + env vars only

	viper.AutomaticEnv()

	// Wire --verbose to charm log level
	logging.SetVerbose(verbose)
}

// loadDotEnv, validEnvKey, maskSecret, and truncateText have been extracted
// to internal/config/dotenv.go and internal/ux/format.go respectively.

// isLocalEndpoint returns true if the given URL points to localhost.
// Used for S-1: warn when Ollama endpoint is not localhost.
func isLocalEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1" ||
		strings.HasPrefix(host, "127.")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
