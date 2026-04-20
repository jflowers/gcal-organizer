# Quickstart: Decision Markdown Export

**Feature**: `012-decision-markdown-export` | **Date**: 2026-04-19

## Prerequisites

- Go 1.24+ installed
- `gcal-organizer` built and configured (OAuth2 credentials, Gemini API key)
- Calendar events with transcript-bearing attachments (for end-to-end testing)

## Build & Test

```bash
# Build
go build ./...

# Run all tests
go test -race ./...

# Run only export package tests
go test -v ./internal/export/...

# Run full CI checks
make ci
```

## Verify the Feature

### 1. Check default configuration

After implementation, the default export directory should be `~/.gcal-organizer/decisions/`:

```bash
# Verify the config shows the new field
gcal-organizer config show
# Should include: decisions_export_dir: ~/.gcal-organizer/decisions/
```

### 2. Dry-run to see what would be exported

```bash
gcal-organizer run --dry-run --verbose
```

**Expected output** (among other dry-run messages):
```
Would export decisions to ~/.gcal-organizer/decisions/weekly-engineering-sync-2026-04-18.md
```

### 3. Full run to produce actual files

```bash
gcal-organizer run --verbose
```

**Expected output**:
```
Exported decisions to ~/.gcal-organizer/decisions/weekly-engineering-sync-2026-04-18.md
```

### 4. Verify the exported file

```bash
cat ~/.gcal-organizer/decisions/weekly-engineering-sync-2026-04-18.md
```

**Expected structure**:
```markdown
---
topic: Weekly Engineering Sync
date: "2026-04-18"
attendees:
  - alice@example.com
  - bob@example.com
---

## Decisions Made

- Team will adopt GitHub Actions for CI/CD

## Open Items

- Whether to migrate to new API version
```

### 5. Custom export directory

Set a custom directory via environment variable:

```bash
export GCAL_DECISIONS_EXPORT_DIR=~/Documents/meeting-decisions
gcal-organizer run --verbose
ls ~/Documents/meeting-decisions/
```

Or via config file (`~/.gcal-organizer/.env`):

```
GCAL_DECISIONS_EXPORT_DIR=~/Documents/meeting-decisions
```

### 6. Verify idempotent reprocessing

Run the command again for the same calendar period:

```bash
gcal-organizer run --verbose --days 1
```

**Expected**: The Google Docs step logs "Document already has Decisions tab, skipping" (existing behavior). The markdown file is **not** re-exported because the organizer skips the entire extraction when the Decisions tab already exists.

### 7. Verify error resilience

Make the export directory read-only to test FR-012:

```bash
mkdir -p /tmp/readonly-decisions
chmod 444 /tmp/readonly-decisions
GCAL_DECISIONS_EXPORT_DIR=/tmp/readonly-decisions gcal-organizer run --verbose
```

**Expected**: Warning logged about failed markdown export. Google Docs Decisions tab still created successfully. Pipeline continues.

```bash
# Cleanup
chmod 755 /tmp/readonly-decisions
rm -rf /tmp/readonly-decisions
```

## Verification Checklist

- [ ] `go build ./...` succeeds
- [ ] `go test -race ./...` passes
- [ ] `make ci` passes
- [ ] `gcal-organizer config show` displays `decisions_export_dir`
- [ ] `--dry-run` logs "Would export" without creating files
- [ ] Full run creates markdown files in the configured directory
- [ ] Exported files have valid YAML frontmatter
- [ ] Exported files have correct section headings and bullet items
- [ ] Empty categories are omitted from exported files
- [ ] Custom export directory works via env var and config file
- [ ] Export failure does not block Google Docs tab creation
- [ ] Filename uses kebab-case topic slug + ISO date
