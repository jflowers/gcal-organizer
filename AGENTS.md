# GCal Organizer Development Guidelines

Auto-generated from all feature plans. Last updated: 2026-02-17

## Active Technologies
- Go 1.21+ + github.com/spf13/cobra (CLI), github.com/spf13/viper (config), Google Drive API v3 (006-owned-only-flag)
- N/A (no new data persistence; flag stored in config file via existing viper mechanism) (006-owned-only-flag)

- **Language**: Go 1.21+
- **CLI Framework**: github.com/spf13/cobra
- **Google APIs**: Drive v3, Docs v1, Calendar v3, Tasks v1
- **AI**: Gemini API via google.golang.org/genai
- **Browser Automation**: Playwright (TypeScript) via npx tsx
- **Authentication**: OAuth2 (Workspace), GCP API Key (Gemini)

## Project Structure

```text
gcal-organizer/
├── cmd/gcal-organizer/          # CLI entry point
├── internal/
│   ├── auth/                    # OAuth2 and API key handling
│   ├── config/                  # Configuration management
│   ├── drive/                   # Google Drive operations
│   ├── docs/                    # Google Docs parsing
│   ├── calendar/                # Calendar operations
│   ├── tasks/                   # Tasks API operations
│   ├── gemini/                  # Gemini AI client
│   └── organizer/               # Main orchestration
├── pkg/models/                  # Shared data models
├── scripts/                     # Browser automation (TypeScript)
├── .specify/                    # Spec-driven development artifacts
└── .opencode/                   # OpenCode agent commands
```

## Commands

```bash
# Build
go build ./...

# Test
go test ./...

# Lint
go vet ./...
gofmt -l .

# Run CI checks locally
make ci

# Install git hooks
make install-hooks
```

## Code Style

### Go Conventions
- Standard project layout (cmd/, internal/, pkg/)
- Error handling via explicit return values, not panic
- Use `context.Context` for cancellation and timeouts
- Table-driven tests preferred
- Wrap errors with context using `fmt.Errorf` with `%w`

### Documentation
- README.md, SETUP.md, man pages must be kept current
- New features require documentation before completion

## Recent Changes
- 006-owned-only-flag: Added Go 1.21+ + github.com/spf13/cobra (CLI), github.com/spf13/viper (config), Google Drive API v3

### 001-gcal-organizer-cli
Core CLI implementation with Google Workspace integration and Gemini AI for action item extraction.

### 002-browser-task-assignment
Browser automation via Playwright for task assignment through Google Docs native UI.

<!-- MANUAL ADDITIONS START -->

## Core Mission (Mission Command)
- **Strategic Architecture:** Engineers shift from manual coding to directing an "infinite supply of junior developers" (AI agents).
- **Outcome Orientation:** Focus on conveying business value and user intent rather than low-level technical sub-tasks.
- **Intent-to-Context:** Treat specs and rules as the medium through which human intent is manifested into code.

## Behavioral Constraints
- **Zero-Waste Mandate:** No orphaned code, unused dependencies, or "Feature Zombie" bloat.
- **Neighborhood Rule:** Changes must be audited for negative impacts on adjacent modules or the wider ecosystem.
- **Intent Drift Detection:** Evaluation must detect when the implementation drifts away from the original human-written "Statement of Intent."
- **Automated Governance:** Primary feedback is provided via automated constraints, reserving human energy for high-level security and logic.

## Technical Guardrails
- **WORM Persistence:** Use Write-Once-Read-Many patterns where data integrity is paramount.

## Council Governance Protocol
- **The Architect:** Must verify that "Intent Driving Implementation" is maintained.
- **The Adversary:** Acts as the primary "Automated Governance" gate for security.
- **The Guard:** Detects "Intent Drift" to ensure the business value remains intact.

**Rule:** A Pull Request is only "Ready for Human" once the `/review-council` command returns an **APPROVE** status.

<!-- MANUAL ADDITIONS END -->
