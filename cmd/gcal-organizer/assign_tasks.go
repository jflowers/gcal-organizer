package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jflowers/gcal-organizer/internal/auth"
	"github.com/jflowers/gcal-organizer/internal/config"
	"github.com/jflowers/gcal-organizer/internal/docs"
	"github.com/jflowers/gcal-organizer/internal/drive"
	"github.com/jflowers/gcal-organizer/internal/gemini"
	"github.com/jflowers/gcal-organizer/internal/secrets"
	"github.com/jflowers/gcal-organizer/internal/ux"
	"github.com/spf13/cobra"
)

// assignTasksCmd represents the assign-tasks command.
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
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		docID, _ := cmd.Flags().GetString("doc")
		if docID == "" {
			return fmt.Errorf("--doc flag is required")
		}

		cfg, store, _, err := loadConfigAndStore()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		cfg.DryRun = dryRun
		cfg.Verbose = verbose
		cfg.OwnedOnly = ownedOnly

		// Initialize OAuth client once and reuse for both ownership check
		// and Docs/Gemini services, avoiding redundant token refresh.
		oauthClient, err := auth.NewOAuthClient(store, cfg.CredentialsFile)
		if err != nil {
			return ux.OAuthSetupFailed(cfg.CredentialsFile)
		}
		httpClient, err := oauthClient.GetClient(ctx)
		if err != nil {
			return ux.AuthFailed()
		}

		// When --owned-only is active, verify ownership before processing
		if ownedOnly {
			driveSvc, driveErr := drive.NewService(ctx, httpClient, cfg.FilenamePattern, cfg.DryRun, cfg.Verbose)
			if driveErr != nil {
				return fmt.Errorf("failed to initialize Drive service: %w\n\nRun 'gcal-organizer doctor' for diagnostics", driveErr)
			}
			owned, checkErr := driveSvc.IsFileOwned(ctx, docID)
			if checkErr != nil {
				return fmt.Errorf("cannot verify ownership of document %s: %w\n\nRun 'gcal-organizer doctor' for diagnostics", docID, checkErr)
			}
			if !owned {
				return fmt.Errorf("document %s is not owned by you; --owned-only prevents processing non-owned documents", docID)
			}
		}

		if dryRun {
			fmt.Println("═══════════════════════════════════════════════════════════")
			fmt.Println("🔍 DRY RUN MODE - Previewing assignments without browser")
			fmt.Println("═══════════════════════════════════════════════════════════")
		}

		fmt.Printf("📄 Processing document: %s\n\n", docID)

		// Initialize Docs+Gemini services using the shared httpClient
		docsSvc, docsErr := docs.NewService(ctx, httpClient)
		if docsErr != nil {
			return fmt.Errorf("failed to initialize Docs service: %w\n\nRun 'gcal-organizer doctor' for diagnostics", docsErr)
		}
		geminiClient, geminiErr := gemini.NewClient(ctx, cfg.GeminiAPIKey, cfg.GeminiModel)
		if geminiErr != nil {
			return ux.MissingAPIKey()
		}

		if dryRun {
			return runAssignTasksDryRunWithServices(ctx, cfg, docsSvc, geminiClient, docID)
		}
		return runAssignTasksBrowserWithServices(ctx, cfg, docsSvc, geminiClient, docID)
	},
}

// initDocsAndGemini is a shared helper that initialises the Docs service and
// Gemini client, both of which are required by every assign-tasks flow.
func initDocsAndGemini(ctx context.Context, cfg *config.Config, store secrets.SecretStore) (*docs.Service, *gemini.Client, error) {
	oauthClient, err := auth.NewOAuthClient(store, cfg.CredentialsFile)
	if err != nil {
		return nil, nil, ux.OAuthSetupFailed(cfg.CredentialsFile)
	}
	httpClient, err := oauthClient.GetClient(ctx)
	if err != nil {
		return nil, nil, ux.AuthFailed()
	}
	docsSvc, err := docs.NewService(ctx, httpClient)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize Docs service: %w\n\nRun 'gcal-organizer doctor' for diagnostics", err)
	}
	geminiClient, err := gemini.NewClient(ctx, cfg.GeminiAPIKey, cfg.GeminiModel)
	if err != nil {
		return nil, nil, ux.MissingAPIKey()
	}
	return docsSvc, geminiClient, nil
}

// extractUnassignedItems returns the subset of checkbox items that have not
// yet been assigned (IsProcessed == false).
func extractUnassignedItems(checkboxes []*docs.CheckboxItem) []gemini.CheckboxItem {
	var items []gemini.CheckboxItem
	for i, cb := range checkboxes {
		if cb.IsProcessed {
			continue
		}
		items = append(items, gemini.CheckboxItem{Index: i, Text: cb.Text})
	}
	return items
}

// runAssignTasksDryRunWithServices analyses a document and prints what would be assigned.
// Uses pre-initialized services to avoid redundant OAuth/Gemini client creation.
func runAssignTasksDryRunWithServices(ctx context.Context, cfg *config.Config, docsSvc *docs.Service, geminiClient *gemini.Client, docID string) error {
	checkboxes, err := docsSvc.ExtractCheckboxItems(ctx, docID)
	if err != nil {
		return fmt.Errorf("failed to extract checkboxes: %w", err)
	}

	fmt.Printf("Found %d checkbox items\n\n", len(checkboxes))
	if len(checkboxes) == 0 {
		fmt.Println("No checkboxes found in this document.")
		return nil
	}

	items := extractUnassignedItems(checkboxes)
	if len(items) == 0 {
		fmt.Println("All checkboxes are already assigned.")
		return nil
	}

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

// browserAssignment is the JSON contract sent to the Playwright script.
type browserAssignment struct {
	CheckboxIndex int    `json:"checkboxIndex"`
	Email         string `json:"email"`
	Text          string `json:"text"`
}

// assignmentResult is one entry in the Playwright script's JSON output.
type assignmentResult struct {
	CheckboxIndex int    `json:"checkboxIndex"`
	Email         string `json:"email"`
	Status        string `json:"status"`
	Reason        string `json:"reason,omitempty"`
}

// scriptOutput is the top-level JSON envelope returned by the Playwright script.
type scriptOutput struct {
	Success bool               `json:"success"`
	Results []assignmentResult `json:"results"`
	Error   string             `json:"error,omitempty"`
}

// runAssignTasksBrowserWithServices extracts assignees then invokes the Playwright script.
// Uses pre-initialized services to avoid redundant OAuth/Gemini client creation.
func runAssignTasksBrowserWithServices(ctx context.Context, cfg *config.Config, docsSvc *docs.Service, geminiClient *gemini.Client, docID string) error {
	checkboxes, err := docsSvc.ExtractCheckboxItems(ctx, docID)
	if err != nil {
		return fmt.Errorf("failed to extract checkboxes: %w", err)
	}

	fmt.Printf("Found %d checkbox items\n", len(checkboxes))
	if len(checkboxes) == 0 {
		fmt.Println("No checkboxes found.")
		return nil
	}

	items := extractUnassignedItems(checkboxes)
	if len(items) == 0 {
		fmt.Println("All checkboxes are already assigned.")
		return nil
	}

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
	return runBrowserScript(ctx, cfg, docID, assignments)
}

// runBrowserScript serialises assignments and invokes the Playwright script.
func runBrowserScript(ctx context.Context, cfg *config.Config, docID string, assignments []gemini.CheckboxAssignment) error {
	var payload []browserAssignment
	for _, a := range assignments {
		email := a.Assignee
		if a.Email != "" {
			email = a.Email
		}
		payload = append(payload, browserAssignment{
			CheckboxIndex: a.Index,
			Email:         email,
			Text:          a.Text,
		})
	}

	assignmentsJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to serialize assignments: %w", err)
	}

	// Guard against exceeding OS argument length limits (ARG_MAX).
	// 100 KB is a conservative limit well below typical ARG_MAX (~2 MB on Linux).
	const maxAssignmentsJSONBytes = 100 * 1024
	if len(assignmentsJSON) > maxAssignmentsJSONBytes {
		return fmt.Errorf("assignments JSON too large (%d bytes, max %d); reduce the number of assignments per document", len(assignmentsJSON), maxAssignmentsJSONBytes)
	}

	browserDir, err := findBrowserDir()
	if err != nil {
		return err
	}

	chromeProfile := cfg.ChromeProfilePath
	if chromeProfile == "" {
		var profileErr error
		chromeProfile, profileErr = chromeProfilePath()
		if profileErr != nil {
			return fmt.Errorf("cannot determine Chrome profile path: %w", profileErr)
		}
	}

	fmt.Println("🌐 Launching browser automation...")

	// Cap browser automation at 10 minutes to avoid hung processes.
	const browserTimeout = 10 * time.Minute
	browserCtx, browserCancel := context.WithTimeout(ctx, browserTimeout)
	defer browserCancel()

	cmd := exec.CommandContext(browserCtx, "npx", "tsx", "assign-tasks.ts",
		"--doc", docID,
		"--assignments", string(assignmentsJSON),
		"--profile", chromeProfile)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Dir = browserDir

	// exec.CommandContext sends SIGKILL to the subprocess when browserCtx is
	// cancelled. browserCtx inherits from the parent ctx which is already
	// wired to SIGINT/SIGTERM via signal.NotifyContext in the calling command.
	// No additional signal handling is needed here.

	err = cmd.Run()

	if err != nil {
		// Include stderr in the error so context is preserved without double-printing.
		return fmt.Errorf("browser automation failed: %s\n\nRun 'gcal-organizer setup-browser' to verify browser setup\nRun 'gcal-organizer doctor' for diagnostics", stderr.String())
	}

	// On success, forward any [assign] debug logs to stderr (verbose mode output).
	if stderrStr := stderr.String(); stderrStr != "" {
		fmt.Fprintf(os.Stderr, "%s", stderrStr)
	}

	var result scriptOutput
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return fmt.Errorf("could not parse browser output: %w\nRaw output: %s", err, stdout.String())
	}

	if !result.Success {
		return fmt.Errorf("browser automation failed: %s", result.Error)
	}

	// Report results
	fmt.Println("\n───────────────────────────────────────────────────────────")
	assigned, skipped, failed := 0, 0, 0
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

// findBrowserDir locates the browser/ automation directory relative to the
// executable or the current working directory (for `go run`).
func findBrowserDir() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to find executable path: %w", err)
	}
	browserDir := filepath.Join(filepath.Dir(execPath), "..", "browser")
	if _, err := os.Stat(browserDir); os.IsNotExist(err) {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return "", fmt.Errorf("failed to determine working directory: %w", cwdErr)
		}
		browserDir = filepath.Join(cwd, "browser")
	}
	if _, err := os.Stat(browserDir); os.IsNotExist(err) {
		return "", fmt.Errorf("browser directory not found\n\nRun 'gcal-organizer setup-browser' to configure browser automation\nRun 'gcal-organizer doctor' for full diagnostics")
	}
	return browserDir, nil
}

// runAssignTasksForDoc scans a document for unassigned checkboxes and runs
// browser automation to assign them. Returns (assigned, failed, error).
// docsSvc and geminiClient should be pre-initialized by the caller to avoid
// redundant OAuth/Gemini client creation per document.
func runAssignTasksForDoc(ctx context.Context, cfg *config.Config, docsSvc *docs.Service, geminiClient *gemini.Client, docID string) (int, int, error) {
	checkboxes, err := docsSvc.ExtractCheckboxItems(ctx, docID)
	if err != nil {
		return 0, 0, fmt.Errorf("extract checkboxes: %w", err)
	}
	if len(checkboxes) == 0 {
		return 0, 0, nil
	}

	items := extractUnassignedItems(checkboxes)
	if len(items) == 0 {
		return 0, 0, nil
	}

	fmt.Printf("   📄 Doc %s: %d checkboxes, %d unassigned\n", docID[:min(8, len(docID))], len(checkboxes), len(items))
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
	if err := runBrowserScript(ctx, cfg, docID, assignments); err != nil {
		return 0, len(assignments), fmt.Errorf("browser automation: %w", err)
	}
	return len(assignments), 0, nil
}
