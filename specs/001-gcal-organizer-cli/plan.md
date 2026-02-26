# Implementation Plan: GCal Organizer CLI

**Branch**: `001-gcal-organizer-cli` | **Date**: 2026-02-01 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification for Go CLI rewrite with GCP Gemini API key support

## Summary

Build a Go CLI tool that organizes Google Drive meeting documents, syncs calendar attachments, and uses Gemini AI to extract action items from Google Docs checkboxes to create Google Tasks. The tool uses OAuth2 for Google Workspace APIs and a GCP API key for Gemini.

## Technical Context

**Language/Version**: Go 1.21+  
**Primary Dependencies**:
- `google.golang.org/genai` - Gemini AI SDK
- `google.golang.org/api/drive/v3` - Drive API
- `google.golang.org/api/docs/v1` - Docs API
- `google.golang.org/api/calendar/v3` - Calendar API
- `google.golang.org/api/tasks/v1` - Tasks API
- `github.com/spf13/cobra` - CLI framework
- `golang.org/x/oauth2/google` - OAuth2 support

**Storage**: Local config file (`~/.gcal-organizer/config.yaml`) + OAuth token cache  
**Testing**: `go test ./...` with table-driven tests  
**Target Platform**: macOS, Linux (cross-platform CLI)  
**Project Type**: Single CLI application  
**Performance Goals**: Process 100+ documents in under 5 minutes  
**Constraints**: Must work with GCP API key (not Gemini Developer API key)  
**Scale/Scope**: Single-user local CLI tool

## Constitution Check

| Principle | Status | Notes |
|-----------|--------|-------|
| CLI-First Architecture | ✅ Pass | All features exposed via CLI commands |
| API-Key Authentication | ✅ Pass | Gemini uses GCP API key, Workspace uses OAuth2 |
| Test-Driven Development | ✅ Pass | Unit tests for all business logic planned |
| Idiomatic Go | ✅ Pass | Standard project layout, context.Context support |
| Graceful Error Handling | ✅ Pass | Wrapped errors, actionable messages |
| Observability | ✅ Pass | --verbose and --dry-run flags |

## Project Structure

### Documentation (this feature)

```text
specs/001-gcal-organizer-cli/
├── spec.md              # Feature specification
├── plan.md              # This file
├── contracts/           # API contracts and interfaces
│   ├── gemini.go        # Gemini client interface
│   ├── drive.go         # Drive service interface
│   ├── docs.go          # Docs service interface
│   ├── calendar.go      # Calendar service interface
│   └── tasks.go         # Tasks service interface
└── tasks.md             # Task breakdown (created by /speckit.tasks)
```

### Source Code (repository root)

```text
gcal-organizer/
├── cmd/
│   └── gcal-organizer/
│       └── main.go              # Entry point
├── internal/
│   ├── auth/
│   │   ├── oauth.go             # OAuth2 flow for Workspace APIs
│   │   └── gemini.go            # API key auth for Gemini
│   ├── config/
│   │   ├── config.go            # Configuration loading
│   │   └── env.go               # Environment variable handling
│   ├── drive/
│   │   ├── service.go           # Drive operations
│   │   ├── folder.go            # Folder management
│   │   └── shortcuts.go         # Shortcut creation
│   ├── docs/
│   │   ├── service.go           # Docs operations
│   │   └── parser.go            # Checkbox extraction
│   ├── calendar/
│   │   ├── service.go           # Calendar operations
│   │   └── attachments.go       # Attachment handling
│   ├── tasks/
│   │   ├── service.go           # Tasks operations
│   │   └── creator.go           # Task creation
│   ├── gemini/
│   │   ├── client.go            # Gemini client wrapper
│   │   ├── prompts.go           # Prompt templates
│   │   └── parser.go            # JSON response parsing
│   └── organizer/
│       ├── organizer.go         # Main orchestration logic
│       └── workflow.go          # Workflow coordination
├── pkg/
│   └── models/
│       ├── document.go          # Document model
│       ├── meeting.go           # Meeting entities
│       └── actionitem.go        # Action item model
├── go.mod
├── go.sum
├── .env.example                 # Example environment config
├── README.md
└── Makefile                     # Build and test commands
```

**Structure Decision**: Single CLI application following standard Go project layout. Internal packages for implementation details, `pkg/models` for shared data structures that could potentially be exported.

## Proposed Changes

### Phase 1: Project Setup & Configuration

#### [NEW] `go.mod`
Initialize Go module with dependencies.

#### [NEW] `cmd/gcal-organizer/main.go`
CLI entry point using Cobra:
- Root command with global flags (--verbose, --dry-run, --config)
- Subcommands: `organize`, `sync-calendar`, `assign-tasks`, `run`

#### [NEW] `internal/config/config.go`
Configuration management:
- Load from environment variables
- Load from config file (~/.gcal-organizer/config.yaml)
- Provide defaults

---

### Phase 2: Authentication

#### [NEW] `internal/auth/oauth.go`
OAuth2 flow for Google Workspace APIs:
- Interactive browser-based auth for first run
- Token storage and refresh
- Scopes: drive, docs, calendar.readonly, tasks

#### [NEW] `internal/auth/gemini.go`
Gemini authentication:
- Load API key from environment (`GEMINI_API_KEY`)
- Configure genai client with API key

---

### Phase 3: Core Services

#### [NEW] `internal/drive/service.go`
Drive service wrapper:
- List files with query filters
- Move files between folders
- Create folders
- Create shortcuts
- Check file ownership

#### [NEW] `internal/docs/service.go`
Docs service wrapper:
- Get document content
- Parse document structure
- Update document content (add emoji tags)

#### [NEW] `internal/calendar/service.go`
Calendar service wrapper:
- List events for date range
- Extract attachments from events
- Parse description for Drive links

#### [NEW] `internal/tasks/service.go`
Tasks service wrapper:
- Create tasks with title and due date
- List task lists

#### [NEW] `internal/gemini/client.go`
Gemini client wrapper:
- Initialize with API key
- Send structured prompts
- Parse JSON responses
- Handle errors and retries

---

### Phase 4: Business Logic

#### [NEW] `internal/organizer/organizer.go`
Core orchestration:
- Parse document names
- Match documents to folders
- Coordinate service calls

#### [NEW] `internal/docs/parser.go`
Checkbox extraction:
- Identify checkbox list items
- Skip processed items (🆔 emoji)
- Extract text for Gemini

#### [NEW] `pkg/models/*.go`
Data models for Document, Meeting, ActionItem

---

### Phase 5: CLI Commands

#### [MODIFY] `cmd/gcal-organizer/main.go`
Add command implementations:
- `organize`: Run document organization workflow
- `sync-calendar`: Sync calendar attachments
- `assign-tasks`: Assign tasks via browser automation (see spec 002)
- `run`: Execute full workflow (organize → sync → assign tasks)

## Verification Plan

### Automated Tests

Since this is a new Go project, we'll create tests as we build:

1. **Unit Tests** - Create alongside each internal package:
   ```bash
   go test ./internal/... -v
   ```

2. **Config Tests** - Test configuration loading:
   ```bash
   go test ./internal/config/... -v
   ```

3. **Model Tests** - Test data structures and parsing:
   ```bash
   go test ./pkg/models/... -v
   ```

4. **Build Verification**:
   ```bash
   go build ./cmd/gcal-organizer
   go vet ./...
   gofmt -d .
   ```

### Integration Tests (with mocks)

We'll use interfaces to allow mocking external services:
```bash
go test ./internal/... -tags=integration -v
```

### Manual Verification

1. **Setup Test**:
   - Run `gcal-organizer --help` to verify CLI structure
   - Expected: Shows available commands and flags

2. **Config Test**:
   - Set `GEMINI_API_KEY` environment variable
   - Run `gcal-organizer config show`
   - Expected: Displays merged configuration

3. **Dry Run Test**:
   - Run `gcal-organizer run --dry-run --verbose`
   - Expected: Shows what would be done without making changes

4. **OAuth Test**:
   - Run `gcal-organizer auth login`
   - Expected: Opens browser for OAuth flow, stores token

> [!IMPORTANT]
> **User Verification Required**: After Phase 2 (Authentication), please verify:
> 1. GCP API key works with Gemini by running a test prompt
> 2. OAuth flow completes successfully
> 3. All required scopes are granted

## Complexity Tracking

No constitution violations anticipated. All principles will be followed.

## Dependencies

| Dependency | Version | Purpose |
|------------|---------|---------|
| `github.com/spf13/cobra` | latest | CLI framework |
| `github.com/spf13/viper` | latest | Configuration management |
| `google.golang.org/genai` | latest | Gemini AI SDK |
| `google.golang.org/api` | latest | Google Workspace APIs |
| `golang.org/x/oauth2` | latest | OAuth2 support |
| `gopkg.in/yaml.v3` | latest | YAML config files |

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| GCP API key vs Gemini Developer API | Research confirms `google.golang.org/genai` supports both; will test early |
| OAuth token expiry during long runs | Implement token refresh in auth package |
| Rate limiting on Google APIs | Implement exponential backoff and progress indicators |
| Gemini JSON parsing failures | Robust parsing with fallbacks, skip items on failure |
