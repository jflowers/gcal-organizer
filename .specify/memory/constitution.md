# GCal Organizer Constitution

## Core Principles

### I. CLI-First Architecture
All functionality is exposed through a command-line interface.
- Commands follow standard Unix conventions (stdin/stdout/stderr)
- Support both human-readable and JSON output formats
- Exit codes follow POSIX conventions (0 = success, non-zero = error)
- Configuration via environment variables and config files

### II. API-Key Authentication Mode
Uses GCP API key for Gemini access (not OAuth for Gemini).
- Google Workspace APIs (Drive, Docs, Calendar, Tasks) use OAuth2
- Gemini API uses GCP API key via Vertex AI compatible endpoint
- Credentials stored securely via environment variables
- Never log or expose credentials in output

### III. Test-Driven Development
Testing is mandatory before implementation.
- Unit tests for all business logic
- Integration tests for Google API interactions (use mocks)
- Table-driven tests preferred for Go
- `go test` must pass before any commit

### IV. Idiomatic Go
Follow Go best practices and conventions.
- Standard project layout (cmd/, internal/, pkg/)
- Error handling via explicit return values, not panic
- Use `context.Context` for cancellation and timeouts
- Minimize external dependencies; prefer standard library

### V. Graceful Error Handling
User-friendly error messages with actionable guidance.
- Wrap errors with context using `fmt.Errorf` with `%w`
- Log sufficient detail for debugging
- Display human-readable messages to users
- Support verbose/debug mode for troubleshooting

### VI. Observability
Built-in logging and debugging capabilities.
- Structured logging with configurable levels
- Dry-run mode for testing without side effects
- Progress indicators for long-running operations
- Clear output of what actions were taken

### VII. Self-Serve Diagnostics
Error messages guide users to self-service tools.
- User-facing error messages MUST include `Run 'gcal-organizer doctor' for diagnostics`
- Setup-related errors SHOULD also reference the relevant self-service command (e.g., `init`, `setup-browser`, `auth login`)
- The `doctor` command MUST be kept comprehensive enough to diagnose all common setup issues

## Technical Constraints

### API Limitations
- GCP Gemini API key (project: gcp-jflowers-pro-gemini)
- Use `google.golang.org/genai` SDK
- Model: gemini-2.0-flash (or configurable)
- Handle rate limiting and quotas gracefully

### Google Workspace Integration
Required scopes:
- `https://www.googleapis.com/auth/documents` (Read/Write)
- `https://www.googleapis.com/auth/drive` (File operations)
- `https://www.googleapis.com/auth/calendar.readonly` (Event scanning)
- `https://www.googleapis.com/auth/tasks` (Task creation)

### Technology Stack
- Language: Go 1.21+
- Build: Standard Go toolchain
- Dependencies: Go modules
- No CGO required (pure Go for portability)

## Development Workflow

### Incremental Implementation
1. **Test-Driven**: Write tests before implementation
2. **API-First**: Build API integrations first, fallback strategies second
3. **Checkpoint-First**: Implement resume capability early for long-running operations
4. **Security Review**: All credential handling reviewed before merge

### Code Review Requirements
- All changes via feature branches
- Clear commit messages following conventional commits
- Tests must pass before merge
- Self-documented code preferred over comments

### Quality Gates
- `go build ./...` - must compile
- `go test ./...` - must pass
- `go vet ./...` - must have no warnings
- `gofmt` - code must be formatted (check-only, not auto-fix)
- `go mod tidy` - go.mod/go.sum must be clean (no diff after tidy)
- `make ci` MUST exist and mirror GitHub Actions CI checks exactly
- Pre-push hooks SHOULD be installed via `make install-hooks` to catch failures before push
- Local `make ci` and GitHub Actions CI MUST run identical checks — any divergence is a bug

## Documentation Requirements

### Documentation Maintenance
- **README.md**, **SETUP.md**, **man/gcal-organizer.1** MUST be kept up to date with all user-facing changes
- Any change to CLI flags, commands, or configuration options MUST be reflected in README.md
- New features MUST include documentation before being considered complete
- Breaking changes MUST be clearly documented with migration instructions

### Documentation Review Checklist
When making changes, review and update as needed:
1. **README.md** - Usage examples, flags, configuration options
2. **AGENTS.md** - Build commands, project structure if changed
3. **Constitution** - If architectural decisions or core principles change
4. **Config examples** - If configuration schema changes

## Governance

This constitution guides all development decisions for the GCal Organizer CLI.
- **Constitution supersedes all other implementation decisions**
- Changes to core strategies require documentation and testing
- Security-related changes require explicit review
- **All feature changes require documentation review before commit**
- Amendments require documentation and justification
- Trade-offs against principles must be explicitly noted
- Simplicity preferred over feature creep (YAGNI)

**Version**: 1.2.0 | **Ratified**: 2026-02-01 | **Last Amended**: 2026-02-08
