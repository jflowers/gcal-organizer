# GCal Organizer

[![CI](https://github.com/jflowers/gcal-organizer/actions/workflows/ci.yml/badge.svg)](https://github.com/jflowers/gcal-organizer/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/jflowers/gcal-organizer)](https://goreportcard.com/report/github.com/jflowers/gcal-organizer)
[![GitHub release](https://img.shields.io/github/v/release/jflowers/gcal-organizer)](https://github.com/jflowers/gcal-organizer/releases/latest)
[![License](https://img.shields.io/github/license/jflowers/gcal-organizer)](LICENSE)

A Go CLI tool that automates meeting note organization, calendar attachment syncing, AI-powered task assignment, and decision extraction using Google Workspace APIs and Gemini AI.

## ✨ What It Does

GCal Organizer runs a 4-step workflow that keeps your Google Drive and Calendar in sync:

| Step | Command | Description |
|------|---------|-------------|
| 1 | `organize` | Finds meeting docs in Drive and organizes them into topic-based folders |
| 2 | `sync-calendar` | Links calendar attachments to meeting folders, shares with attendees |
| 3 | `assign-tasks` | Locates checkbox action items in Google Docs `Notes by Gemini` and assigns them if Gemini can determine the assignee  |
| 4 | *(automatic)* | Extracts decisions from meeting transcripts and creates a "Decisions" tab with categorized sections |

Run all four with `gcal-organizer run`, or steps 1-3 individually.

### How the Folder Routing Works

Documents are matched by the naming pattern `Topic Name - YYYY-MM-DD` (configurable via regex). The topic name becomes the folder:

```
Google Drive
├── Meeting Notes/                        ← Master folder (configurable)
│   ├── Weekly Standup/
│   │   ├── Notes - Weekly Standup - 2026-02-03   ← moved here
│   │   └── Notes - Weekly Standup - 2026-02-10   ← moved here
│   ├── Project Alpha Review/
│   │   ├── Notes - Project Alpha Review - 2026-02-05
│   │   └── 🔗 Design Spec (shortcut)            ← calendar attachment
│   └── 1-1 with Jordan/
│       └── Notes - 1-1 with Jordan - 2026-02-07
```

**Before**: Docs scattered across "My Drive" and "Shared with me"
**After**: Auto-organized into topic folders with calendar attachments linked as shortcuts

### Workflow Example

1. Take meeting notes in a Google Doc named `Notes - Sprint Planning - 2026-02-08`
2. Have Gemini take notes. It will add checkboxes like `☐ @jordan review the API spec by Friday`
3. Attach the doc to the calendar event
4. Run `gcal-organizer run`
5. Result:
   - Doc moves to `Meeting Notes / Sprint Planning /` folder
   - Calendar attachment linked as a shortcut in the meeting folder
   - Folder and attachment shared with all attendees (edit access)
   - Jordan gets the task assigned in Google Docs

## 🚀 Quick Start

### Prerequisites

| Requirement | Purpose | Install |
|-------------|---------|---------|
| **Go 1.24+** | Build the CLI | [go.dev/dl](https://go.dev/dl/) |
| **Node.js 18+** | Browser automation for task assignment | [nodejs.org](https://nodejs.org/) |
| **Google Chrome** | Task assignment via Playwright | [google.com/chrome](https://www.google.com/chrome/) |
| **GCP Project** | OAuth2 credentials + Gemini API key | [Setup Guide](docs/SETUP.md) |

### Install via Homebrew (macOS & Linux)

```bash
# Tap and install (includes Node.js dependency + man page)
brew tap jflowers/gcal-organizer
brew install gcal-organizer

# Authenticate with Google
gcal-organizer auth login

# Set up browser automation and install hourly service
gcal-organizer setup-browser
gcal-organizer install

# Test with dry-run first
gcal-organizer run --dry-run --verbose
```

### Install from Source

```bash
# Clone and build
git clone https://github.com/jflowers/gcal-organizer.git
cd gcal-organizer
make install

# Configure and authenticate
gcal-organizer init
gcal-organizer auth login

# Set up browser automation
gcal-organizer setup-browser

# Test
gcal-organizer run --dry-run --verbose

# Install hourly service
gcal-organizer install
```

### Run as an Hourly Service

```bash
# Install the hourly service (works for both Homebrew and source installs):
gcal-organizer install

# Check status
gcal-organizer doctor

# Remove the service
gcal-organizer uninstall

# Or via Makefile (source installs only):
make install-service    # install
make service-status     # check status
make service-logs       # view logs
make service-trigger    # trigger immediate run
make uninstall-service  # remove
```

The service runs with `GCAL_DAYS_TO_LOOK_BACK=1` to process only the last day of events.

### Man Page

```bash
man gcal-organizer
```

## 🔧 Commands

| Command | Description |
|---------|-------------|
| `gcal-organizer run` | Run the full 4-step workflow |
| `gcal-organizer organize` | Step 1 only: organize documents into folders |
| `gcal-organizer sync-calendar` | Step 2 only: sync calendar attachments |
| `gcal-organizer assign-tasks --doc <ID>` | Step 3 only: assign tasks from a specific doc |
| `gcal-organizer auth login` | Authenticate with Google |
| `gcal-organizer auth status` | Check authentication status |
| `gcal-organizer config show` | Show current configuration |
| `gcal-organizer init` | Set up configuration (interactive wizard) |
| `gcal-organizer doctor` | Check system health and report fixes |
| `gcal-organizer install` | Install as hourly background service |
| `gcal-organizer uninstall` | Remove the background service |

### Flags

| Flag | Description |
|------|-------------|
| `--dry-run` | Show what would be done without making changes |
| `--verbose` / `-v` | Detailed output |
| `--owned-only` | Only mutate files you own; skip non-owned files |
| `--days N` | Days to look back for calendar events (default: 8) |

### Owned-Only Mode

The `--owned-only` flag prevents gcal-organizer from modifying files you don't own. When active:

- **Owned files**: Moved, shared, and processed normally
- **Non-owned files**: Only shortcuts are created (no moves, shares, or task assignments)
- Use `--verbose` to see which files are skipped and why
- Combine with `--dry-run` to preview ownership filtering

```bash
# Run with ownership protection
gcal-organizer run --owned-only --verbose

# Preview what would be skipped
gcal-organizer run --owned-only --dry-run --verbose
```

**Limitation**: Shared Drive files are treated as non-owned (the organization owns them). Do not use `--owned-only` if your workflow depends on Shared Drive mutations.

## 🔑 Configuration

Environment variables (or `~/.gcal-organizer/.env`):

```bash
# Required
GEMINI_API_KEY=your-gcp-api-key

# Optional
GCAL_MASTER_FOLDER_NAME="Meeting Notes"   # Default: "Meeting Notes"
GCAL_DAYS_TO_LOOK_BACK=1                  # Default: 1
GCAL_OWNED_ONLY=true                      # Default: false (only mutate owned files)
GCAL_FILENAME_KEYWORDS="Notes,Meeting"    # Comma-separated
GCAL_FILENAME_PATTERN="(.+)\s*-\s*(\d{4}-\d{2}-\d{2})"
GEMINI_MODEL="gemini-2.0-flash"           # Default: gemini-2.0-flash
```

## 🔒 Data Privacy

**What goes to Gemini AI**: Individual checkbox items for task assignment (e.g., `"@jordan review the API spec by Friday"`) and full transcript text for decision extraction. The tool does *not* upload entire documents for task assignment — it extracts checkbox text via the Google Docs API. For decision extraction (Step 4), the full transcript tab content is sent to Gemini for analysis.

**What stays local**: OAuth tokens, credentials, and configuration are stored securely and never transmitted.

### Secure Credential Storage

By default, OAuth tokens, the Gemini API key, and client credentials are stored in your OS credential store (macOS Keychain or Linux Secret Service). This prevents sensitive data from being stored as plaintext files on disk.

| Data | Default Storage | Fallback |
|------|----------------|----------|
| OAuth token | OS keychain | `~/.gcal-organizer/token.json` |
| Gemini API key | OS keychain | `~/.gcal-organizer/.env` |
| Client credentials | OS keychain | `~/.gcal-organizer/credentials.json` |

To disable keychain storage (e.g., on headless servers without a credential store):

```bash
gcal-organizer run --no-keyring
# or
export GCAL_NO_KEYRING=true
```

## 🤖 Why Browser Automation for Task Assignment?

The Google Docs API provides read access to document content (including checkboxes), but **cannot interact with the native "Assign as a task" canvas widget** in Google Docs. This is a UI-only feature with no API equivalent. The tool uses a Playwright/Node.js sidecar to:

1. Read checkbox text and extract assignees via Gemini
2. Open each Google Doc in a headed Chrome browser
3. Hover over checkboxes to reveal the "Assign" tooltip
4. Click to assign the task to the identified person

This requires Chrome with your Google profile for authentication. See [Browser Automation Setup](docs/SETUP.md#browser-automation-setup) for details.

## 📋 Step 4: Decision Extraction

Step 4 automatically processes meeting transcript documents to extract and categorize decisions:

1. During calendar sync (Step 2), the tool identifies documents titled "Notes by Gemini" (exact match) or ending with "- Transcript" (suffix match)
2. For each eligible document, it reads the transcript content and sends it to Gemini AI
3. Gemini extracts decisions into three categories:
   - **Decisions Made** -- commitments the team agreed on
   - **Decisions Deferred** -- items explicitly tabled for later
   - **Open Items** -- unresolved topics needing further discussion
4. A new "Decisions" tab is created in the document with clickable timestamp links back to the transcript

**Idempotent**: Documents with an existing "Decisions" tab are automatically skipped. Safe to run repeatedly.

**Error handling**: If Gemini fails on a document, it's skipped with a warning. The next run will retry since no tab was created.

## 📁 Project Structure

```
gcal-organizer/
├── cmd/gcal-organizer/        # CLI entry point (Cobra)
├── internal/
│   ├── auth/                  # OAuth2 + Gemini API auth
│   ├── calendar/              # Calendar event operations
│   ├── config/                # Configuration management
│   ├── docs/                  # Docs checkbox extraction + decision tab creation
│   ├── drive/                 # Drive folder/file operations
│   ├── gemini/                # Gemini AI assignee + decision extraction
│   └── organizer/             # Workflow orchestration
├── browser/                   # Playwright task assignment script (TypeScript)
├── deploy/                    # Service files (launchd, systemd)
├── .specify/                  # Spec-kit artifacts
├── .github/workflows/         # CI/CD (build/test + release)
├── man/                       # Man page (roff)
├── Makefile                   # Build, test, service management
└── docs/SETUP.md              # Full setup guide
```

## 🛠 Development

```bash
make build          # Build the binary
make test           # Run tests
make check          # Format, vet, and test
make lint           # Run golangci-lint
make help           # Show all targets
```

### Spec-Driven Development

This project uses [Spec-Kit](https://github.com/github/spec-kit) for specification-driven development. Specs are in `specs/`:

| Spec | Description |
|------|-------------|
| `001-gcal-organizer-cli` | Core CLI — organize, sync, share |
| `002-browser-task-assignment` | Playwright-based task assignment |
| `003-cicd-github-actions` | CI/CD workflows |
| `004-service-deployment` | Hourly service on macOS/Fedora |

## 📄 License

See [LICENSE](LICENSE) for details.
