# Quickstart: Decision Extraction from Meeting Transcripts

**Feature**: `008-decision-extraction` | **Date**: 2026-02-26

## Prerequisites

- Go 1.24+ installed
- Existing `gcal-organizer` setup complete (`gcal-organizer doctor` passes)
- OAuth2 credentials with `documents` scope (already authorized)
- `GEMINI_API_KEY` configured
- At least one calendar event with a "Notes by Gemini" or "- Transcript" attachment

## How It Works

Decision extraction runs as Step 4 of the `gcal-organizer run` workflow:

```
Step 1: Organize Documents (existing)
Step 2: Sync Calendar Attachments (existing) ← collects transcript doc IDs
Step 3: Assign Tasks (existing)
Step 4: Extract Decisions (new) ← processes collected transcript docs
```

## Usage

The feature integrates into the existing `run` command with no new flags or commands:

```bash
# Full workflow including decision extraction
gcal-organizer run

# Preview mode — see which docs would be processed
gcal-organizer run --dry-run

# Only process documents you own
gcal-organizer run --owned-only

# Process events from the last 7 days
gcal-organizer run --days 7
```

## What Gets Created

For each eligible document, a new "Decisions" tab is added with:

1. **Decisions Made** — decisions the team committed to
2. **Decisions Deferred** — items explicitly tabled for later
3. **Open Items** — unresolved topics still under discussion

Each decision is a bullet point with a clickable timestamp link back to the transcript.

## Document Detection

The system looks for two types of calendar event attachments:

| Pattern | Match Type | Example |
|---------|-----------|---------|
| `Notes by Gemini` | Exact title match | "Notes by Gemini" |
| `- Transcript` | Title suffix match | "ComplyTime Standup - 2026/02/25 14:00 WET - Transcript" |

If both types are attached to the same event, only the "Notes by Gemini" document is processed.

## Idempotency

Documents that already have a "Decisions" tab are skipped automatically. The workflow is safe to run repeatedly — it will never create duplicate tabs.

## Error Handling

- **AI failure**: Document is skipped with a warning. The next run will retry automatically (no Decisions tab was created).
- **Concurrent processing**: If another instance creates the tab first, the current instance treats it as already processed.
- **No decisions found**: A Decisions tab is still created with a "No decisions identified" note, preventing re-processing.

## Development

```bash
# Build
go build ./...

# Test
go test ./...

# Lint
go vet ./...

# Run CI checks
make ci
```

### Key Source Files

| File | Purpose |
|------|---------|
| `internal/docs/service.go` | Transcript reading + Decisions tab creation |
| `internal/gemini/client.go` | Decision extraction prompt + response parsing |
| `internal/organizer/organizer.go` | Step 4 orchestration + doc collection |
| `cmd/gcal-organizer/main.go` | Step 4 wiring in `run` command |
| `pkg/models/models.go` | Decision and TranscriptHeading structs |
