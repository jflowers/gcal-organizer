// Package drive provides Google Drive operations.
package drive

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jflowers/gcal-organizer/internal/logging"
	"github.com/jflowers/gcal-organizer/internal/retry"
	"github.com/jflowers/gcal-organizer/pkg/models"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// escapeQuery escapes special characters for Google Drive query strings.
func escapeQuery(s string) string {
	// Escape single quotes by replacing ' with \'
	return strings.ReplaceAll(s, "'", "\\'")
}

// ActionResult describes the result of an operation.
type ActionResult struct {
	Action  string // "move", "shortcut", "skip", "create_folder"
	Skipped bool   // True if operation was skipped (dry-run or already exists)
	Reason  string // Why it was skipped (e.g., "already exists", "dry-run")
	Details string // Human-readable details about what happened/would happen
}

// Service provides Google Drive operations.
type Service struct {
	client           *drive.Service
	filenamePattern  *regexp.Regexp
	fallbackPattern  *regexp.Regexp // For "Notes - [meeting name]" format
	masterFolderID   string
	masterFolderName string
	rootFolderID     string // Actual ID of "My Drive" root folder
	currentUserEmail string
	dryRun           bool
	verbose          bool
}

// NewService creates a new Drive service.
func NewService(ctx context.Context, httpClient *http.Client, filenamePattern string, dryRun, verbose bool) (*Service, error) {
	srv, err := drive.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create Drive service: %w", err)
	}

	pattern, err := regexp.Compile(filenamePattern)
	if err != nil {
		return nil, fmt.Errorf("invalid filename pattern: %w", err)
	}

	// Fallback pattern for "Notes - [meeting name]" format (no date)
	fallback := regexp.MustCompile(`^Notes\s*-\s*(.+)$`)

	// Get current user email
	about, err := srv.About.Get().Fields("user").Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	// Get the actual ID of the root folder ("My Drive")
	// The API accepts "root" as an alias, but returns the actual ID
	rootFile, err := srv.Files.Get("root").Fields("id").Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get root folder ID: %w", err)
	}

	return &Service{
		client:           srv,
		filenamePattern:  pattern,
		fallbackPattern:  fallback,
		rootFolderID:     rootFile.Id,
		currentUserEmail: about.User.EmailAddress,
		dryRun:           dryRun,
		verbose:          verbose,
	}, nil
}

// SetMasterFolder sets the master folder ID by name.
func (s *Service) SetMasterFolder(ctx context.Context, folderName string) error {
	// escapeQuery prevents query-injection when folderName contains single quotes.
	query := fmt.Sprintf("name = '%s' and mimeType = 'application/vnd.google-apps.folder' and trashed = false",
		escapeQuery(folderName))

	var result *drive.FileList
	err := retry.Do(ctx, retry.DefaultConfig(), func() error {
		var e error
		result, e = s.client.Files.List().Q(query).Fields("files(id, name)").Do()
		return e
	})
	if err != nil {
		return fmt.Errorf("failed to find master folder: %w", err)
	}

	if len(result.Files) == 0 {
		return fmt.Errorf("master folder '%s' not found", folderName)
	}

	s.masterFolderID = result.Files[0].Id
	s.masterFolderName = folderName
	return nil
}

// ListMeetingDocuments finds documents matching the filename pattern.
func (s *Service) ListMeetingDocuments(ctx context.Context, keywords []string) ([]*models.Document, error) {
	var keywordQuery string
	if len(keywords) > 0 {
		parts := make([]string, len(keywords))
		for i, kw := range keywords {
			parts[i] = fmt.Sprintf("name contains '%s'", escapeQuery(kw))
		}
		keywordQuery = "(" + strings.Join(parts, " or ") + ") and "
	}

	query := keywordQuery + "mimeType = 'application/vnd.google-apps.document' and trashed = false"

	if s.verbose {
		logging.Logger.Debug("Document query", "root_folder", s.rootFolderID, "query", query)
	}

	var docs []*models.Document
	var scannedCount, fallbackCandidates int
	pageToken := ""
	for {
		req := s.client.Files.List().
			Q(query).
			Fields("nextPageToken, files(id, name, mimeType, owners, parents, webViewLink)").
			PageSize(100)

		if pageToken != "" {
			req.PageToken(pageToken)
		}

		var result *drive.FileList
		err := retry.Do(ctx, retry.DefaultConfig(), func() error {
			var e error
			result, e = req.Do()
			return e
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list documents: %w", err)
		}

		for _, file := range result.Files {
			scannedCount++
			// Check if this looks like a fallback candidate for verbose logging
			if s.verbose && s.fallbackPattern.MatchString(file.Name) {
				fallbackCandidates++
				parentInfo := "no parent"
				if len(file.Parents) > 0 {
					parentInfo = file.Parents[0]
				}
				logging.Logger.Debug("Fallback candidate", "name", file.Name, "parent", parentInfo)
			}

			doc, err := s.parseDocument(file)
			if err != nil {
				continue // Skip documents that don't match the pattern
			}
			docs = append(docs, doc)
		}

		pageToken = result.NextPageToken
		if pageToken == "" {
			break
		}
	}

	if s.verbose {
		logging.Logger.Debug("Document scan complete",
			"scanned", scannedCount, "fallback_candidates", fallbackCandidates, "matched", len(docs))
	}

	return docs, nil
}

// parseDocument parses a Drive file into a Document model.
// Tries the primary pattern first, then falls back to "Notes - [meeting name]" format.
func (s *Service) parseDocument(file *drive.File) (*models.Document, error) {
	var meetingName string
	var date time.Time
	var isFallback bool

	// Try primary pattern: "[meeting name] - [date]"
	matches := s.filenamePattern.FindStringSubmatch(file.Name)
	if matches != nil && len(matches) >= 3 {
		meetingName = strings.TrimSpace(matches[1])
		dateStr := matches[2]
		parsedDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return nil, fmt.Errorf("invalid date in filename: %s", dateStr)
		}
		date = parsedDate
	} else {
		// Try fallback pattern: "Notes - [meeting name]" (only for files in Drive root)
		isInRoot := len(file.Parents) > 0 && file.Parents[0] == s.rootFolderID
		fallbackMatches := s.fallbackPattern.FindStringSubmatch(file.Name)
		if !isInRoot || fallbackMatches == nil || len(fallbackMatches) < 2 {
			return nil, fmt.Errorf("filename does not match any pattern: %s", file.Name)
		}
		meetingName = strings.TrimSpace(fallbackMatches[1])
		isFallback = true
		// date remains zero value - no date in this format
	}

	// Check ownership
	isOwned := false
	for _, owner := range file.Owners {
		if owner.EmailAddress == s.currentUserEmail {
			isOwned = true
			break
		}
	}

	parentID := ""
	if len(file.Parents) > 0 {
		parentID = file.Parents[0]
	}

	return &models.Document{
		ID:             file.Id,
		Name:           file.Name,
		MeetingName:    meetingName,
		Date:           date,
		MimeType:       file.MimeType,
		IsOwned:        isOwned,
		IsFallback:     isFallback,
		ParentFolderID: parentID,
		WebViewLink:    file.WebViewLink,
	}, nil
}

// GetOrCreateMeetingFolder gets or creates a subfolder for a meeting topic.
func (s *Service) GetOrCreateMeetingFolder(ctx context.Context, meetingName string) (*models.MeetingFolder, error) {
	if s.masterFolderID == "" {
		return nil, fmt.Errorf("master folder not set")
	}

	// Search for existing folder (escape special characters in name)
	query := fmt.Sprintf("name = '%s' and '%s' in parents and mimeType = 'application/vnd.google-apps.folder' and trashed = false",
		escapeQuery(meetingName), s.masterFolderID)

	var result *drive.FileList
	err := retry.Do(ctx, retry.DefaultConfig(), func() error {
		var e error
		result, e = s.client.Files.List().Q(query).Fields("files(id, name)").Do()
		return e
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search for folder: %w", err)
	}

	if len(result.Files) > 0 {
		return &models.MeetingFolder{
			ID:       result.Files[0].Id,
			Name:     result.Files[0].Name,
			ParentID: s.masterFolderID,
			IsNew:    false, // Existing folder
		}, nil
	}

	// Don't create in dry-run mode, but mark as new
	if s.dryRun {
		return &models.MeetingFolder{
			ID:       "", // Empty ID signals this folder doesn't exist yet
			Name:     meetingName,
			ParentID: s.masterFolderID,
			IsNew:    true, // Would be created
		}, nil
	}

	// Create new folder
	folder := &drive.File{
		Name:     meetingName,
		MimeType: "application/vnd.google-apps.folder",
		Parents:  []string{s.masterFolderID},
	}
	var created *drive.File
	err = retry.Do(ctx, retry.DefaultConfig(), func() error {
		var e error
		created, e = s.client.Files.Create(folder).Fields("id, name").Do()
		return e
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}

	return &models.MeetingFolder{
		ID:       created.Id,
		Name:     created.Name,
		ParentID: s.masterFolderID,
		IsNew:    true, // Just created
	}, nil
}

// MoveDocument moves a document to a target folder.
// Returns ActionResult describing what happened.
func (s *Service) MoveDocument(ctx context.Context, docID, docName, currentParentID, targetFolderID, targetFolderName string) ActionResult {
	// Check if already in target folder (idempotent)
	if currentParentID == targetFolderID {
		return ActionResult{
			Action:  "move",
			Skipped: true,
			Reason:  "already in folder",
			Details: fmt.Sprintf("%s is already in %s", docName, targetFolderName),
		}
	}

	details := fmt.Sprintf("Move %s to %s/%s", docName, s.masterFolderName, targetFolderName)

	if s.dryRun {
		return ActionResult{
			Action:  "move",
			Skipped: false,
			Reason:  "dry-run",
			Details: details,
		}
	}

	err := retry.Do(ctx, retry.DefaultConfig(), func() error {
		_, e := s.client.Files.Update(docID, nil).
			AddParents(targetFolderID).
			RemoveParents(currentParentID).
			Do()
		return e
	})
	if err != nil {
		return ActionResult{
			Action:  "move",
			Skipped: true,
			Reason:  fmt.Sprintf("error: %v", err),
			Details: details,
		}
	}

	return ActionResult{
		Action:  "move",
		Skipped: false,
		Details: details,
	}
}

// ShortcutExists checks if a shortcut to the target file already exists in the folder.
// Paginates through all pages so it does not miss shortcuts in large folders.
// Returns: exists, existingShortcutName, debugInfo, error
func (s *Service) ShortcutExists(ctx context.Context, targetFileID, folderID string) (bool, string, string, error) {
	query := fmt.Sprintf("'%s' in parents and mimeType = 'application/vnd.google-apps.shortcut' and trashed = false",
		folderID)

	var allFiles []*drive.File
	pageToken := ""
	for {
		var result *drive.FileList
		err := retry.Do(ctx, retry.DefaultConfig(), func() error {
			req := s.client.Files.List().
				Q(query).
				Fields("nextPageToken, files(id, name, shortcutDetails)").
				PageSize(100)
			if pageToken != "" {
				req.PageToken(pageToken)
			}
			var e error
			result, e = req.Do()
			return e
		})
		if err != nil {
			return false, "", "", fmt.Errorf("failed to check for existing shortcuts: %w", err)
		}
		allFiles = append(allFiles, result.Files...)
		pageToken = result.NextPageToken
		if pageToken == "" {
			break
		}
	}

	// Build debug info showing all shortcuts found
	var debugInfo string
	if len(allFiles) > 0 {
		debugInfo = fmt.Sprintf("Found %d shortcuts in folder. Looking for target: %s. ", len(allFiles), targetFileID)
		for _, file := range allFiles {
			if file.ShortcutDetails != nil {
				debugInfo += fmt.Sprintf("[%s → %s] ", file.Name, file.ShortcutDetails.TargetId)
			}
		}
	} else {
		debugInfo = "No shortcuts found in folder"
	}

	for _, file := range allFiles {
		if file.ShortcutDetails != nil && file.ShortcutDetails.TargetId == targetFileID {
			return true, file.Name, debugInfo, nil
		}
	}

	return false, "", debugInfo, nil
}

// CreateShortcut creates a shortcut to a file in the target folder.
// If folderIsNew is true, skips the existence check (new folders can't have existing shortcuts).
// Returns ActionResult describing what happened.
func (s *Service) CreateShortcut(ctx context.Context, fileID, fileName, targetFolderID, targetFolderName string, folderIsNew bool) ActionResult {
	// Skip existence check for new folders (they can't have existing shortcuts)
	if !folderIsNew && targetFolderID != "" {
		exists, existingName, debugInfo, err := s.ShortcutExists(ctx, fileID, targetFolderID)
		if err != nil {
			return ActionResult{
				Action:  "shortcut",
				Skipped: true,
				Reason:  fmt.Sprintf("error: %v", err),
				Details: fmt.Sprintf("Check shortcut for %s in %s", fileName, targetFolderName),
			}
		}

		if exists {
			return ActionResult{
				Action:  "shortcut",
				Skipped: true,
				Reason:  "already exists",
				Details: fmt.Sprintf("Shortcut '%s' to %s already exists in %s", existingName, fileName, targetFolderName),
			}
		}

		// Not found - include debug info in dry-run output
		if s.dryRun {
			details := fmt.Sprintf("Create shortcut to %s in %s/%s\n      Debug: %s", fileName, s.masterFolderName, targetFolderName, debugInfo)
			return ActionResult{
				Action:  "shortcut",
				Skipped: false,
				Reason:  "dry-run",
				Details: details,
			}
		}
	}

	details := fmt.Sprintf("Create shortcut to %s in %s/%s", fileName, s.masterFolderName, targetFolderName)

	if s.dryRun {
		return ActionResult{
			Action:  "shortcut",
			Skipped: false,
			Reason:  "dry-run",
			Details: details,
		}
	}

	shortcut := &drive.File{
		Name:     fileName,
		MimeType: "application/vnd.google-apps.shortcut",
		Parents:  []string{targetFolderID},
		ShortcutDetails: &drive.FileShortcutDetails{
			TargetId: fileID,
		},
	}
	err := retry.Do(ctx, retry.DefaultConfig(), func() error {
		_, e := s.client.Files.Create(shortcut).Do()
		return e
	})
	if err != nil {
		// Include file URL for requesting access if permission denied
		fileURL := fmt.Sprintf("https://drive.google.com/file/d/%s/view", fileID)
		reason := fmt.Sprintf("error: %v", err)
		if strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "forbidden") || strings.Contains(err.Error(), "permission") {
			reason = fmt.Sprintf("Permission denied. Request access: %s", fileURL)
		} else if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "notFound") {
			reason = fmt.Sprintf("File not found or deleted: %s", fileURL)
		}
		return ActionResult{
			Action:  "shortcut",
			Skipped: true,
			Reason:  reason,
			Details: details,
		}
	}

	return ActionResult{
		Action:  "shortcut",
		Skipped: false,
		Details: details,
	}
}

// GetFileName fetches the name of a file by its ID.
func (s *Service) GetFileName(ctx context.Context, fileID string) (string, error) {
	var file *drive.File
	err := retry.Do(ctx, retry.DefaultConfig(), func() error {
		var e error
		file, e = s.client.Files.Get(fileID).Fields("name").Do()
		return e
	})
	if err != nil {
		return "", fmt.Errorf("failed to get file: %w", err)
	}
	return file.Name, nil
}

// IsDryRun returns whether the service is in dry-run mode.
func (s *Service) IsDryRun() bool {
	return s.dryRun
}

// FindShortcutToFile finds a shortcut in a folder that points to a specific file.
// Paginates through all pages so it does not miss shortcuts in large folders.
// Returns the shortcut ID if found, empty string otherwise.
func (s *Service) FindShortcutToFile(ctx context.Context, targetFileID, folderID string) (string, error) {
	query := fmt.Sprintf("'%s' in parents and mimeType = 'application/vnd.google-apps.shortcut' and trashed = false",
		folderID)

	pageToken := ""
	for {
		var result *drive.FileList
		err := retry.Do(ctx, retry.DefaultConfig(), func() error {
			req := s.client.Files.List().
				Q(query).
				Fields("nextPageToken, files(id, shortcutDetails)").
				PageSize(100)
			if pageToken != "" {
				req.PageToken(pageToken)
			}
			var e error
			result, e = req.Do()
			return e
		})
		if err != nil {
			return "", fmt.Errorf("failed to search for shortcuts: %w", err)
		}

		for _, file := range result.Files {
			if file.ShortcutDetails != nil && file.ShortcutDetails.TargetId == targetFileID {
				return file.Id, nil
			}
		}

		pageToken = result.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return "", nil
}

// TrashFile moves a file to trash.
// Returns ActionResult describing what happened.
func (s *Service) TrashFile(ctx context.Context, fileID, description string) ActionResult {
	if s.dryRun {
		return ActionResult{
			Action:  "trash",
			Skipped: false,
			Reason:  "dry-run",
			Details: description,
		}
	}

	err := retry.Do(ctx, retry.DefaultConfig(), func() error {
		_, e := s.client.Files.Update(fileID, &drive.File{Trashed: true}).Do()
		return e
	})
	if err != nil {
		return ActionResult{
			Action:  "trash",
			Skipped: true,
			Reason:  fmt.Sprintf("error: %v", err),
			Details: description,
		}
	}

	return ActionResult{
		Action:  "trash",
		Skipped: false,
		Details: description,
	}
}

// ShareFile shares a file or folder with the specified email address and role.
// Role should be "reader", "writer", or "commenter".
// Returns ActionResult describing what happened.
func (s *Service) ShareFile(ctx context.Context, fileID, fileName, email, role string) ActionResult {
	if s.dryRun {
		return ActionResult{
			Action:  "share",
			Skipped: true,
			Reason:  "dry-run",
			Details: fmt.Sprintf("Would share '%s' with %s (%s)", fileName, email, role),
		}
	}

	// Check if user already has access. A failure here (e.g., caller lacks
	// permission to list permissions) is non-fatal: we log a warning and
	// proceed to the Create call, which is idempotent in practice because
	// Drive returns 409/already-exists if the permission exists already.
	var permissions *drive.PermissionList
	if err := retry.Do(ctx, retry.DefaultConfig(), func() error {
		var e error
		permissions, e = s.client.Permissions.List(fileID).Fields("permissions(emailAddress, role)").Do()
		return e
	}); err != nil {
		logging.Logger.Warn("could not list permissions, proceeding with share attempt",
			"file", fileName, "email", email, "err", err)
	}
	if permissions != nil {
		for _, perm := range permissions.Permissions {
			if strings.EqualFold(perm.EmailAddress, email) {
				return ActionResult{
					Action:  "share",
					Skipped: true,
					Reason:  "already shared",
					Details: fmt.Sprintf("'%s' already shared with %s", fileName, email),
				}
			}
		}
	}

	// Create permission for the email
	permission := &drive.Permission{
		Type:         "user",
		Role:         role,
		EmailAddress: email,
	}

	err := retry.Do(ctx, retry.DefaultConfig(), func() error {
		_, e := s.client.Permissions.Create(fileID, permission).
			SendNotificationEmail(false).
			Do()
		return e
	})
	if err != nil {
		return ActionResult{
			Action:  "share",
			Skipped: true,
			Reason:  fmt.Sprintf("error: %v", err),
			Details: fmt.Sprintf("Failed to share '%s' with %s", fileName, email),
		}
	}

	return ActionResult{
		Action:  "share",
		Skipped: false,
		Details: fmt.Sprintf("Shared '%s' with %s", fileName, email),
	}
}

// IsFileOwned checks if the current user is an owner of the given file.
// Returns (true, nil) if owned, (false, nil) if not, (false, err) on API failure.
func (s *Service) IsFileOwned(ctx context.Context, fileID string) (bool, error) {
	var file *drive.File
	err := retry.Do(ctx, retry.DefaultConfig(), func() error {
		var e error
		file, e = s.client.Files.Get(fileID).
			Fields("owners").
			Do()
		return e
	})
	if err != nil {
		return false, fmt.Errorf("failed to check file ownership: %w", err)
	}
	for _, owner := range file.Owners {
		if owner.EmailAddress == s.currentUserEmail {
			return true, nil
		}
	}
	return false, nil
}

// CanEditFile checks if the current user owns or has editor access to the file.
func (s *Service) CanEditFile(ctx context.Context, fileID string) bool {
	var file *drive.File
	err := retry.Do(ctx, retry.DefaultConfig(), func() error {
		var e error
		file, e = s.client.Files.Get(fileID).
			Fields("capabilities(canEdit)").
			Do()
		return e
	})
	if err != nil {
		return false
	}
	return file.Capabilities != nil && file.Capabilities.CanEdit
}
