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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jflowers/gcal-organizer/internal/auth"
	"github.com/jflowers/gcal-organizer/internal/calendar"
	"github.com/jflowers/gcal-organizer/internal/config"
	"github.com/jflowers/gcal-organizer/internal/docs"
	"github.com/jflowers/gcal-organizer/internal/drive"
	"github.com/jflowers/gcal-organizer/internal/gemini"
	"github.com/jflowers/gcal-organizer/internal/logging"
	"github.com/jflowers/gcal-organizer/internal/organizer"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// Version is set at build time
	Version = "dev"

	// Global flags
	cfgFile string
	verbose bool
	dryRun  bool
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
	// Initialize OAuth client
	oauthClient, err := auth.NewOAuthClient(cfg.CredentialsFile, logging.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create OAuth client: %w\n\nTo set up OAuth:\n1. Download credentials from Google Cloud Console\n2. Save to: %s\n\nRun 'gcal-organizer doctor' for full diagnostics", err, cfg.CredentialsFile)
	}

	httpClient, err := oauthClient.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate: %w\n\nRun 'gcal-organizer auth login' to authenticate\nRun 'gcal-organizer doctor' for full diagnostics", err)
	}

	// Initialize Drive service
	driveSvc, err := drive.NewService(ctx, httpClient, cfg.FilenamePattern, cfg.DryRun, cfg.Verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Drive service: %w", err)
	}

	// Initialize Calendar service
	calSvc, err := calendar.NewService(ctx, httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Calendar service: %w", err)
	}

	// Create and return organizer
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

		days, _ := cmd.Flags().GetInt("days")
		if days > 0 {
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
		if len(docIDs) > 0 && !dryRun {
			fmt.Println("📝 STEP 3: Assigning Tasks")
			fmt.Println("───────────────────────────────────────────────────────────")
			fmt.Printf("   Found %d Notes documents to scan for tasks\n", len(docIDs))

			totalAssigned := 0
			totalFailed := 0

			for _, docID := range docIDs {
				assigned, failed, err := runAssignTasksForDoc(ctx, cfg, docID)
				if err != nil {
					fmt.Printf("   ⚠️  Error processing doc %s: %v\n", docID[:8], err)
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
			cfg.DaysToLookBack = days
		}
		cfg.DryRun = dryRun
		cfg.Verbose = verbose

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

// assignTasksCmd represents the assign-tasks command
var assignTasksCmd = &cobra.Command{
	Use:   "assign-tasks",
	Short: "Assign document tasks via browser automation",
	Long: `Use Playwright browser automation to assign checkbox items in Google Docs to the appropriate people.

This command:
1. Opens the document in a browser using your Chrome profile
2. Finds checkboxes in the "Suggested next steps" section
3. Uses Gemini AI to identify assignees
4. Clicks each checkbox and assigns via the native UI

Requires: Node.js and the browser/ directory to be set up.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		docID, _ := cmd.Flags().GetString("doc")
		if docID == "" {
			return fmt.Errorf("--doc flag is required")
		}

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		cfg.DryRun = dryRun
		cfg.Verbose = verbose

		if dryRun {
			fmt.Println("═══════════════════════════════════════════════════════════")
			fmt.Println("🔍 DRY RUN MODE - Previewing assignments without browser")
			fmt.Println("═══════════════════════════════════════════════════════════")
		}

		fmt.Printf("📄 Processing document: %s\n\n", docID)

		// In dry-run mode, we analyze the document and show what would be assigned
		// In normal mode, we invoke the Playwright script
		if dryRun {
			return runAssignTasksDryRun(ctx, cfg, docID)
		}
		return runAssignTasksBrowser(ctx, cfg, docID)
	},
}

func runAssignTasksDryRun(ctx context.Context, cfg *config.Config, docID string) error {
	// Initialize OAuth client to access the document
	oauthClient, err := auth.NewOAuthClient(cfg.CredentialsFile, logging.Logger)
	if err != nil {
		return fmt.Errorf("failed to create OAuth client: %w\n\nRun 'gcal-organizer doctor' for diagnostics", err)
	}

	httpClient, err := oauthClient.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w\n\nRun 'gcal-organizer auth login' to authenticate\nRun 'gcal-organizer doctor' for full diagnostics", err)
	}

	// Initialize Docs service
	docsSvc, err := docs.NewService(ctx, httpClient)
	if err != nil {
		return fmt.Errorf("failed to initialize Docs service: %w", err)
	}

	// Initialize Gemini client
	geminiClient, err := gemini.NewClient(ctx, cfg.GeminiAPIKey, cfg.GeminiModel)
	if err != nil {
		return fmt.Errorf("failed to initialize Gemini client: %w\n\nSet GEMINI_API_KEY environment variable\nRun 'gcal-organizer doctor' for full diagnostics", err)
	}

	// Get document content
	checkboxes, err := docsSvc.ExtractCheckboxItems(ctx, docID)
	if err != nil {
		return fmt.Errorf("failed to extract checkboxes: %w", err)
	}

	fmt.Printf("Found %d checkbox items\n\n", len(checkboxes))

	if len(checkboxes) == 0 {
		fmt.Println("No checkboxes found in this document.")
		return nil
	}

	// Convert to gemini format
	var items []gemini.CheckboxItem
	for i, cb := range checkboxes {
		if cb.IsProcessed {
			continue // Skip already assigned
		}
		items = append(items, gemini.CheckboxItem{
			Index: i,
			Text:  cb.Text,
		})
	}

	if len(items) == 0 {
		fmt.Println("All checkboxes are already assigned.")
		return nil
	}

	// Extract assignees using Gemini
	fmt.Println("🤖 Analyzing tasks with Gemini...")
	assignments, err := geminiClient.ExtractAssigneesFromCheckboxes(ctx, items)
	if err != nil {
		return fmt.Errorf("failed to extract assignees: %w", err)
	}

	fmt.Printf("\n📋 Planned Assignments (%d):\n", len(assignments))
	fmt.Println("───────────────────────────────────────────────────────────")
	for _, a := range assignments {
		fmt.Printf("   ✓ Would assign to %s: %s\n", a.Assignee, truncateText(a.Text, 50))
	}
	fmt.Println("───────────────────────────────────────────────────────────")
	fmt.Println("Run without --dry-run to execute assignments via browser.")

	return nil
}

func runAssignTasksBrowser(ctx context.Context, cfg *config.Config, docID string) error {
	// First, get assignments like dry-run mode
	oauthClient, err := auth.NewOAuthClient(cfg.CredentialsFile, logging.Logger)
	if err != nil {
		return fmt.Errorf("failed to create OAuth client: %w\n\nRun 'gcal-organizer doctor' for diagnostics", err)
	}

	httpClient, err := oauthClient.GetClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w\n\nRun 'gcal-organizer auth login' to authenticate\nRun 'gcal-organizer doctor' for full diagnostics", err)
	}

	docsSvc, err := docs.NewService(ctx, httpClient)
	if err != nil {
		return fmt.Errorf("failed to initialize Docs service: %w", err)
	}

	geminiClient, err := gemini.NewClient(ctx, cfg.GeminiAPIKey, cfg.GeminiModel)
	if err != nil {
		return fmt.Errorf("failed to initialize Gemini client: %w\n\nSet GEMINI_API_KEY environment variable\nRun 'gcal-organizer doctor' for full diagnostics", err)
	}

	// Get checkboxes
	checkboxes, err := docsSvc.ExtractCheckboxItems(ctx, docID)
	if err != nil {
		return fmt.Errorf("failed to extract checkboxes: %w", err)
	}

	fmt.Printf("Found %d checkbox items\n", len(checkboxes))

	if len(checkboxes) == 0 {
		fmt.Println("No checkboxes found.")
		return nil
	}

	// Convert to gemini format
	var items []gemini.CheckboxItem
	for i, cb := range checkboxes {
		if cb.IsProcessed {
			continue
		}
		items = append(items, gemini.CheckboxItem{
			Index: i,
			Text:  cb.Text,
		})
	}

	if len(items) == 0 {
		fmt.Println("All checkboxes are already assigned.")
		return nil
	}

	// Extract assignees using Gemini
	fmt.Println("🤖 Analyzing tasks with Gemini...")
	assignments, err := geminiClient.ExtractAssigneesFromCheckboxes(ctx, items)
	if err != nil {
		return fmt.Errorf("failed to extract assignees: %w", err)
	}

	if len(assignments) == 0 {
		fmt.Println("No assignable tasks found.")
		return nil
	}

	fmt.Printf("\n📋 Found %d assignments to make\n", len(assignments))

	// Build browser assignments JSON
	type BrowserAssignment struct {
		CheckboxIndex int    `json:"checkboxIndex"`
		Email         string `json:"email"`
		Text          string `json:"text"`
	}

	var browserAssignments []BrowserAssignment
	for _, a := range assignments {
		// TODO: Resolve name to email via document attendee list
		// For now, assume assignee is already an email or use as-is
		email := a.Assignee
		if a.Email != "" {
			email = a.Email
		}
		browserAssignments = append(browserAssignments, BrowserAssignment{
			CheckboxIndex: a.Index,
			Email:         email,
			Text:          a.Text,
		})
	}

	assignmentsJSON, err := json.Marshal(browserAssignments)
	if err != nil {
		return fmt.Errorf("failed to serialize assignments: %w", err)
	}

	// Find browser directory
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find executable path: %w", err)
	}
	browserDir := filepath.Join(filepath.Dir(execPath), "..", "browser")

	// Check if we're running from source
	if _, err := os.Stat(browserDir); os.IsNotExist(err) {
		// Try relative to current working directory
		cwd, _ := os.Getwd()
		browserDir = filepath.Join(cwd, "browser")
	}

	if _, err := os.Stat(browserDir); os.IsNotExist(err) {
		return fmt.Errorf("browser directory not found\n\nRun 'gcal-organizer setup-browser' to configure browser automation\nRun 'gcal-organizer doctor' for full diagnostics")
	}

	// Get Chrome profile path from config or use default
	chromeProfile := cfg.ChromeProfilePath
	if chromeProfile == "" {
		chromeProfile = "/Users/jflowers/Library/Application Support/Google/Chrome/Profile 1"
	}

	fmt.Println("🌐 Launching browser automation...")

	// Run the Playwright script
	cmd := exec.CommandContext(ctx, "npx", "tsx", "assign-tasks.ts",
		"--doc", docID,
		"--assignments", string(assignmentsJSON),
		"--profile", chromeProfile)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Dir = browserDir

	err = cmd.Run()

	// Print debug logs from stderr
	if stderrStr := stderr.String(); stderrStr != "" {
		fmt.Fprintf(os.Stderr, "%s", stderrStr)
	}

	if err != nil {
		return fmt.Errorf("browser automation failed: %s\n\nRun 'gcal-organizer setup-browser' to verify browser setup\nRun 'gcal-organizer doctor' for diagnostics", stderr.String())
	}

	output := stdout.Bytes()

	// Parse the output
	type AssignmentResult struct {
		CheckboxIndex int    `json:"checkboxIndex"`
		Email         string `json:"email"`
		Status        string `json:"status"`
		Reason        string `json:"reason,omitempty"`
	}

	type ScriptOutput struct {
		Success bool               `json:"success"`
		Results []AssignmentResult `json:"results"`
		Error   string             `json:"error,omitempty"`
	}

	var result ScriptOutput
	if err := json.Unmarshal(output, &result); err != nil {
		fmt.Printf("⚠️  Could not parse browser output: %s\n", string(output))
		return nil
	}

	if !result.Success {
		return fmt.Errorf("browser automation failed: %s", result.Error)
	}

	// Report results
	fmt.Println("\n───────────────────────────────────────────────────────────")
	assigned := 0
	skipped := 0
	failed := 0

	for _, r := range result.Results {
		switch r.Status {
		case "assigned":
			fmt.Printf("   ✓ Assigned to %s\n", r.Email)
			assigned++
		case "skipped":
			fmt.Printf("   ⊘ Skipped %s: %s\n", r.Email, r.Reason)
			skipped++
		case "failed":
			fmt.Printf("   ✗ Failed %s: %s\n", r.Email, r.Reason)
			failed++
		}
	}

	fmt.Println("───────────────────────────────────────────────────────────")
	fmt.Printf("✅ Done: %d assigned, %d skipped, %d failed\n", assigned, skipped, failed)

	return nil
}

// runAssignTasksForDoc scans a document for unassigned checkboxes and runs
// browser automation to assign them. Returns (assigned, failed, error).
func runAssignTasksForDoc(ctx context.Context, cfg *config.Config, docID string) (int, int, error) {
	oauthClient, err := auth.NewOAuthClient(cfg.CredentialsFile, logging.Logger)
	if err != nil {
		return 0, 0, fmt.Errorf("OAuth client: %w", err)
	}
	httpClient, err := oauthClient.GetClient(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("auth: %w", err)
	}

	docsSvc, err := docs.NewService(ctx, httpClient)
	if err != nil {
		return 0, 0, fmt.Errorf("docs service: %w", err)
	}

	checkboxes, err := docsSvc.ExtractCheckboxItems(ctx, docID)
	if err != nil {
		return 0, 0, fmt.Errorf("extract checkboxes: %w", err)
	}

	if len(checkboxes) == 0 {
		return 0, 0, nil
	}

	// Filter to unassigned only
	var items []gemini.CheckboxItem
	for i, cb := range checkboxes {
		if cb.IsProcessed {
			continue
		}
		items = append(items, gemini.CheckboxItem{
			Index: i,
			Text:  cb.Text,
		})
	}

	if len(items) == 0 {
		return 0, 0, nil
	}

	geminiClient, err := gemini.NewClient(ctx, cfg.GeminiAPIKey, cfg.GeminiModel)
	if err != nil {
		return 0, 0, fmt.Errorf("gemini client: %w", err)
	}

	fmt.Printf("   📄 Doc %s: %d checkboxes, %d unassigned\n", docID[:8], len(checkboxes), len(items))
	fmt.Println("   🤖 Analyzing tasks with Gemini...")

	assignments, err := geminiClient.ExtractAssigneesFromCheckboxes(ctx, items)
	if err != nil {
		return 0, 0, fmt.Errorf("extract assignees: %w", err)
	}

	if len(assignments) == 0 {
		fmt.Println("   No assignable tasks found")
		return 0, 0, nil
	}

	fmt.Printf("   📋 Found %d assignments to make\n", len(assignments))

	// Run browser automation
	if err := runAssignTasksBrowser(ctx, cfg, docID); err != nil {
		return 0, len(assignments), fmt.Errorf("browser automation: %w", err)
	}

	// Since runAssignTasksBrowser already prints its own results,
	// we return the count of assignments as a best estimate.
	// The actual results are printed inline by the browser function.
	return len(assignments), 0, nil
}

func truncateText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// configShowCmd represents the config show command
var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long:  `Display the merged configuration from environment variables and config file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		fmt.Println("📋 Current Configuration:")
		fmt.Println("───────────────────────────────────────────────────────────")
		fmt.Printf("   Master Folder:     %s\n", cfg.MasterFolderName)
		fmt.Printf("   Days to Look Back: %d\n", cfg.DaysToLookBack)
		fmt.Printf("   Filename Pattern:  %s\n", cfg.FilenamePattern)
		fmt.Printf("   Gemini Model:      %s\n", cfg.GeminiModel)
		fmt.Printf("   Gemini API Key:    %s\n", maskSecret(cfg.GeminiAPIKey))
		fmt.Printf("   Credentials File:  %s\n", cfg.CredentialsFile)

		// Show token storage location
		tokenStorage := auth.NewTokenStorage(logging.Logger)
		fmt.Printf("   Token Storage:     %s\n", tokenStorage.GetStorageLocation())
		fmt.Println("───────────────────────────────────────────────────────────")

		// Check if credentials exist
		if _, err := os.Stat(cfg.CredentialsFile); os.IsNotExist(err) {
			fmt.Println("⚠️  OAuth credentials not found!")
			fmt.Printf("   Download from Google Cloud Console and save to:\n   %s\n", cfg.CredentialsFile)
		} else {
			fmt.Println("✅ OAuth credentials file found")
		}

		// Check if token exists in secure storage
		if _, err := tokenStorage.LoadToken(); err != nil {
			fmt.Println("⚠️  Not authenticated - run 'gcal-organizer auth login'")
		} else {
			fmt.Println("✅ OAuth token found (authenticated)")
		}

		return nil
	},
}

// configCmd represents the config command group
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management",
	Long:  `View and manage gcal-organizer configuration.`,
}

// authCmd represents the auth command group
var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authentication management",
	Long:  `Manage OAuth authentication for Google Workspace APIs.`,
}

// authLoginCmd represents the auth login command
var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to Google Workspace",
	Long:  `Start the OAuth2 flow to authenticate with Google Workspace APIs.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		fmt.Println("🔐 Starting OAuth2 login flow...")
		fmt.Println("")

		oauthClient, err := auth.NewOAuthClient(cfg.CredentialsFile, logging.Logger)
		if err != nil {
			return fmt.Errorf("failed to create OAuth client: %w\n\nTo set up OAuth:\n1. Go to https://console.cloud.google.com\n2. Create OAuth 2.0 credentials (Desktop app)\n3. Download and save to: %s\n\nRun 'gcal-organizer doctor' for full diagnostics", err, cfg.CredentialsFile)
		}

		_, err = oauthClient.GetClient(ctx)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}

		fmt.Println("")
		fmt.Println("✅ Authentication successful!")
		fmt.Printf("   Token storage: %s\n", oauthClient.GetTokenStorageLocation())
		fmt.Println("")
		fmt.Println("You can now run: gcal-organizer run --dry-run")
		return nil
	},
}

// authStatusCmd represents the auth status command
var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check authentication status",
	Long:  `Check if OAuth authentication is configured and valid.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		fmt.Println("🔐 Authentication Status:")
		fmt.Println("───────────────────────────────────────────────────────────")

		// Check credentials file
		if _, err := os.Stat(cfg.CredentialsFile); os.IsNotExist(err) {
			fmt.Println("❌ OAuth credentials file NOT found")
			fmt.Printf("   Expected: %s\n", cfg.CredentialsFile)
			fmt.Println("")
			fmt.Println("To fix: Download OAuth credentials from Google Cloud Console")
			return nil
		}
		fmt.Println("✅ OAuth credentials file found")

		// Try to load and validate token from secure storage
		oauthClient, err := auth.NewOAuthClient(cfg.CredentialsFile, logging.Logger)
		if err != nil {
			fmt.Printf("❌ Error loading credentials: %v\n", err)
			return nil
		}

		storageLocation := oauthClient.GetTokenStorageLocation()
		if oauthClient.IsAuthenticated() {
			fmt.Printf("✅ Authenticated and token valid (%s)\n", storageLocation)
		} else {
			if storageLocation == "Not stored" {
				fmt.Println("❌ Not authenticated")
				fmt.Println("")
				fmt.Println("Run: gcal-organizer auth login")
			} else {
				fmt.Printf("⚠️  Token exists (%s) but expired\n", storageLocation)
				fmt.Println("")
				fmt.Println("Run: gcal-organizer auth login")
			}
		}

		// Check Gemini API key
		fmt.Println("")
		if cfg.GeminiAPIKey != "" {
			fmt.Println("✅ Gemini API key configured")
		} else {
			fmt.Println("❌ Gemini API key NOT set")
			fmt.Println("   Set GEMINI_API_KEY environment variable")
		}

		return nil
	},
}

func maskSecret(s string) string {
	if len(s) == 0 {
		return "(not set)"
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.gcal-organizer/.env)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "show what would be done without making changes")

	// Bind flags to viper
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("dry-run", rootCmd.PersistentFlags().Lookup("dry-run"))

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

		// Strip surrounding quotes (double or single) for bash compatibility
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
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
