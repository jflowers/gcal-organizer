# Setup Guide

This guide walks you through setting up `gcal-organizer` with Google Cloud credentials.

> **Quick install**: If you use Homebrew, you can skip the build steps:
> ```bash
> brew tap jflowers/gcal-organizer && brew install gcal-organizer
> ```
> Then jump to [Step 1](#step-1-create-a-google-cloud-project) for API setup.

## Prerequisites

- **Go 1.24** or later
- **Node.js 18+** and npm (for browser-based task assignment)
- **Google Chrome** (for task assignment via Playwright)
- **Ollama** (for local AI features — sensitivity gate, local task assignment)
- A Google account with access to:
  - Google Drive
  - Google Calendar
  - Google Docs
  - Google Tasks
- A Google Cloud project with billing enabled (for Gemini API)

### Ollama (Local AI)

gcal-organizer uses [Ollama](https://ollama.com/) with IBM Granite models for local AI features:
- **Sensitivity gate**: Screens transcripts for sensitive content before cloud processing
- **Local task assignment**: Extracts assignees from action items locally
- **Local-only mode**: Runs all AI processing locally (no cloud AI calls)

**Install Ollama:**

```bash
# macOS
brew install ollama

# Linux
curl -fsSL https://ollama.com/install.sh | sh
```

**Pull required models:**

```bash
ollama pull granite-guardian    # Sensitivity classification
ollama pull granite3.2:8b       # Task assignment and decision extraction
```

**Start the Ollama service:**

```bash
ollama serve
```

> **Note**: If you don't want to use local AI features, set `ollama.enabled: false` in your `config.yaml`. All Ollama checks and features will be skipped.

---

## Step 1: Create a Google Cloud Project

1. Go to the [Google Cloud Console](https://console.cloud.google.com/)
2. Click "Select a project" → "New Project"
3. Name it (e.g., `gcal-organizer`) and click "Create"
4. Select the new project

---

## Step 2: Enable Required APIs

Navigate to **APIs & Services** → **Library** and enable:

| API | Purpose |
|-----|---------|
| Google Drive API | Organize documents, create folders/shortcuts |
| Google Calendar API | Read calendar events and attachments |
| Google Docs API | Read document content for action items |
| Google Tasks API | Create tasks from extracted action items |
| Generative Language API | Gemini AI for parsing action items |

**Quick links:**
- [Enable Drive API](https://console.cloud.google.com/apis/library/drive.googleapis.com)
- [Enable Calendar API](https://console.cloud.google.com/apis/library/calendar-json.googleapis.com)
- [Enable Docs API](https://console.cloud.google.com/apis/library/docs.googleapis.com)
- [Enable Tasks API](https://console.cloud.google.com/apis/library/tasks.googleapis.com)
- [Enable Generative Language API](https://console.cloud.google.com/apis/library/generativelanguage.googleapis.com)

---

## Step 3: Create OAuth 2.0 Credentials

1. Go to **APIs & Services** → **Credentials**
2. Click **Create Credentials** → **OAuth client ID**
3. If prompted, configure the OAuth consent screen:
   - User Type: **External** (or Internal if using Workspace)
   - App name: `gcal-organizer`
   - User support email: Your email
   - Developer contact: Your email
   - Scopes: Skip for now (we'll use the default scopes)
   - Test users: Add your email
4. Back in Credentials, create OAuth client ID:
   - Application type: **Desktop app**
   - Name: `gcal-organizer-cli`
5. Click **Create** and download the JSON file
6. Save it as `~/.gcal-organizer/credentials.json`:

```bash
# If you haven't run init yet, it will create the directory for you:
gcal-organizer init
mv ~/Downloads/client_secret_*.json ~/.gcal-organizer/credentials.json
```

---

## Step 4: Get a Gemini API Key

1. Go to [Google AI Studio](https://aistudio.google.com/app/apikey)
2. Click **Create API Key**
3. Select your project or create a new one
4. Copy the API key

> **Note**: You can find your API key in the [API keys](https://console.cloud.google.com/apis/credentials) page in the Google Cloud Console.

**Your Company may have a different process for getting a Gemini API key.**

---

## Step 5: Configure the Application

The easiest way to configure is with the setup wizard:

```bash
gcal-organizer init
```

This will:
- Create `~/.gcal-organizer/` if needed
- Generate a `.env` file with your settings
- Prompt for your `GEMINI_API_KEY`
- Auto-detect your Chrome profile path

Alternatively, use environment variables directly (see `.env.example`):

```bash
export GEMINI_API_KEY="your-key-here"
export MASTER_FOLDER_NAME="Meeting Notes"
```

### Owned-Only Mode

To always protect non-owned files from mutations, add to your `.env`:

```bash
GCAL_OWNED_ONLY=true
```

When active, gcal-organizer will only move, share, and assign tasks for files you own. Non-owned files still get shortcuts for discoverability. Override per-invocation with `--owned-only=false`.

**Note**: Shared Drive files are treated as non-owned (the organization owns them). Do not enable this setting if your workflow depends on Shared Drive mutations.

---

## Step 6: Build and Authenticate

```bash
# Clone and build
git clone https://github.com/jflowers/gcal-organizer.git
cd gcal-organizer
make install

# Install browser automation dependencies (for assign-tasks)
cd browser && npm install && cd ..

# Run init if you haven't yet
gcal-organizer init

# Authenticate with Google
gcal-organizer auth login

# Verify everything
gcal-organizer doctor
```

The OAuth flow will:
1. Open your browser to Google's consent page
2. Ask you to authorize the app
3. Store the token in the OS credential store (macOS Keychain / Linux Secret Service)

---

## Step 7: Verify Setup

```bash
# Check configuration
gcal-organizer config show

# Test with dry-run (no changes made)
gcal-organizer run --dry-run --verbose
```

---

## Browser Automation Setup

Step 3 (task assignment) uses [Playwright](https://playwright.dev/) to interact with the Google Docs UI. This is necessary because the Google Docs API does **not** provide access to the native "Assign as a task" widget — it's a canvas-rendered UI element with no API equivalent.

### Why Browser Automation?

The Google Docs API can read document text and checkbox state, but cannot:
- Interact with the "Assign to" tooltip on checkboxes
- Trigger the native Google Tasks integration built into Docs
- Click UI buttons rendered on the canvas

Playwright automates Chrome to hover over checkboxes, detect the "Assign" tooltip, and click it — the only way to use this feature programmatically.

### Chrome Data Directory

The tool creates a dedicated Chrome data directory at `~/.gcal-organizer/chrome-data/` to keep browser state isolated from your personal Chrome profile. Run `gcal-organizer setup-browser` to create it and sign in with your Google account.

#### Flatpak Chrome (Fedora)

If Chrome is installed via Flatpak (common on Fedora), you must grant it filesystem access to the config directory before `setup-browser` will work:

```bash
flatpak override --user --filesystem=~/.gcal-organizer com.google.Chrome
```

This is required because Flatpak sandboxes Chrome by default, blocking access to `~/.gcal-organizer/chrome-data/`. Without this override, Chrome will fail to create or read the data directory.

### Troubleshooting Browser Automation

- **"Browser closed unexpectedly"**: Make sure Chrome is not already running, or that remote debugging is not conflicting with another instance.
- **Tasks not assigned**: The script uses a hover-then-detect pattern. Ensure the Google Doc has the "Next steps" (or "Suggested next steps") checkboxes visible.
- **Flatpak Chrome can't access data directory**: Run `flatpak override --user --filesystem=~/.gcal-organizer com.google.Chrome` to grant access.

---

## Scopes Requested

The app requests these OAuth scopes:

| Scope | Purpose |
|-------|---------|
| `drive.file` | Create folders and shortcuts (only accesses files created by the app) |
| `drive.readonly` | Read file metadata for organizing |
| `calendar.readonly` | Read calendar events and attachments |
| `documents.readonly` | Read document content for action items |
| `tasks` | Create and manage tasks |

---

## Troubleshooting

### "Access blocked: This app's request is invalid"

Your OAuth consent screen may not be configured correctly:
1. Go to **APIs & Services** → **OAuth consent screen**
2. Make sure your email is added as a **Test user**
3. Or publish the app (requires verification for sensitive scopes)

### "File not found" errors for calendar attachments

This usually means:
- The file was deleted
- You don't have access to the file
- The file is in someone else's Drive and not shared with you

These are expected for external meeting recordings/transcripts.

### "Invalid credentials" error

Your `credentials.json` may be corrupted or wrong type:
1. Delete `~/.gcal-organizer/credentials.json`
2. Re-download from Google Cloud Console
3. Make sure it's a **Desktop app** credential, not Web or Service Account

### "Token expired" or authentication issues

Delete the token and re-authenticate:
```bash
rm ~/.gcal-organizer/token.json
./gcal-organizer auth login
```

---

## Data Privacy

- **Sensitivity gate**: When enabled (default), every transcript is screened locally by Granite Guardian before any cloud processing. Sensitive transcripts (HR, legal, financial, health, termination) are skipped entirely — no cloud AI calls, no document modifications.
- **What goes to Gemini AI**: Only decision extraction uses Gemini (unless local-only mode is enabled). Task assignment runs locally via Ollama.
- **Local-only mode**: Set `ollama.local_only: true` to prevent all cloud AI calls. All processing runs through local Granite models.
- **What stays local**: OAuth tokens, API keys, and client credentials are stored in the OS credential store (macOS Keychain / Linux Secret Service) by default. Configuration remains in `~/.gcal-organizer/config.yaml`.
- **Scopes are minimal**: The app requests only the scopes it needs (see table above).
- **Offline access**: The token includes refresh capability for long-running use.

---

## Credential Storage

By default, all sensitive credentials are stored in the OS credential store:

| Credential | Storage Location | Notes |
|-----------|-----------------|-------|
| OAuth token | OS keychain | Auto-migrated from `token.json` on upgrade |
| Gemini API key | OS keychain | Auto-migrated from `.env` on upgrade |
| Client credentials | OS keychain | Auto-migrated from `credentials.json` on upgrade |
| Non-secret config | `~/.gcal-organizer/.env` | Folder name, days, keywords, model |

### Headless / No-Keychain Environments

For CI, containers, or servers without a keyring provider, use file-based fallback:

```bash
# Via CLI flag
gcal-organizer run --no-keyring

# Via environment variable
export GCAL_NO_KEYRING=true
```

When using file-based storage, credentials remain in `~/.gcal-organizer/` as plaintext files (the pre-upgrade behavior).

### Existing User Migration

On first run after upgrading, credentials are automatically migrated to the OS credential store:

- `token.json` is moved and the file is deleted
- `GEMINI_API_KEY` is moved from `.env` (the line is removed; other config is preserved)
- `credentials.json` contents are stored; you are prompted before the file is deleted (it may be shared with other tools)

Migration is idempotent and transparent. Use `gcal-organizer doctor --verbose` to verify.

---

## Running as a Service

Once setup is complete, you can install gcal-organizer as an hourly background service:

```bash
# Install (auto-detects macOS launchd vs Fedora systemd)
make install-service

# Check status
make service-status

# View logs
make service-logs

# Uninstall
make uninstall-service
```

The service runs with `GCAL_DAYS_TO_LOOK_BACK=1` so it only processes the last day of events.

---

## Next Steps

Once setup is complete:

```bash
# Run the full workflow
gcal-organizer run --verbose

# Or run individual steps
gcal-organizer organize --dry-run
gcal-organizer sync-calendar --days 14
gcal-organizer assign-tasks --doc <DOC_ID>
```

For a complete command reference, run:

```bash
man gcal-organizer
```

See the [README](../README.md) for full usage documentation.
