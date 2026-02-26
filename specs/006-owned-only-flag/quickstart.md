# Quickstart: Owned-Only Mode

**Feature**: `006-owned-only-flag`
**Date**: 2026-02-26

## Overview

The `--owned-only` flag prevents gcal-organizer from modifying files you don't own. When active, only files where you are listed as an owner will be moved, shared, trashed, or have tasks assigned. Non-owned files still get shortcuts in your folder structure for discoverability.

## Usage

### One-time use via CLI flag

```bash
# Run full workflow, only mutating owned files
gcal-organizer run --owned-only

# Organize documents, only mutating owned files
gcal-organizer organize --owned-only

# Sync calendar with ownership protection
gcal-organizer sync-calendar --owned-only

# See what would be skipped
gcal-organizer run --owned-only --dry-run --verbose
```

### Persist as default in configuration

Add to your `.env` or config file:

```bash
GCAL_OWNED_ONLY=true
```

Override per-invocation:

```bash
# Disable owned-only for a single run
gcal-organizer run --owned-only=false
```

## What changes with --owned-only?

| Operation | Owned Files | Non-Owned Files |
|-----------|-------------|-----------------|
| Move to meeting folder | Normal | Shortcut created instead |
| Share with attendees | Normal | Skipped |
| Trash redundant shortcuts | Normal | Skipped |
| Assign tasks (browser) | Normal | Skipped (error in standalone mode) |
| Create shortcuts | Normal | Normal (always allowed) |

## Observability

- Use `--verbose` to see which files are skipped and why
- A summary count of skipped files is printed at the end of each run
- Combine with `--dry-run` to preview ownership filtering without any changes

## Limitations

- **Shared Drive files** are treated as non-owned (organization owns them). Do not use `--owned-only` if your workflow depends on Shared Drive mutations.
- When `--owned-only` is active, you cannot share files you have edit access to but don't own. This is intentional — use without the flag if you need to share as an editor.
