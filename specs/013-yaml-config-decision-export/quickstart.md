# Quickstart: YAML Config Migration & Decision Export Enhancements

**Feature**: 013-yaml-config-decision-export
**Date**: 2026-04-20

## Verification Steps

### V1: Automatic .env → config.yaml Migration

**Setup**:
1. Ensure `~/.gcal-organizer/.env` exists with standard configuration
2. Ensure `~/.gcal-organizer/config.yaml` does NOT exist
3. Ensure secrets are in the OS keychain (run `gcal-organizer doctor --verbose` to verify)

**Test**:
```bash
# Run any command to trigger migration
gcal-organizer doctor

# Verify config.yaml was created
cat ~/.gcal-organizer/config.yaml

# Verify .env was deleted
ls ~/.gcal-organizer/.env  # Should fail: No such file or directory
```

**Expected**:
- `config.yaml` exists with equivalent settings in YAML format
- `.env` is deleted
- No secret values (API key, credentials path) appear in `config.yaml`
- `gcal-organizer doctor` reports all checks passing

### V2: Fresh Install Generates config.yaml

**Setup**:
1. Remove `~/.gcal-organizer/` entirely (or use a fresh user)

**Test**:
```bash
gcal-organizer init --api-key "test-key-123"
cat ~/.gcal-organizer/config.yaml
ls ~/.gcal-organizer/.env  # Should fail: No such file or directory
```

**Expected**:
- `config.yaml` is created with default values
- No `.env` file is created
- API key is stored in keychain, not in `config.yaml`

### V3: Meeting Allowlist Filtering

**Setup**:
1. Edit `~/.gcal-organizer/config.yaml`:
   ```yaml
   decisions:
     meetings:
       - "Sprint Planning"
       - "Design Review"
   ```
2. Have calendar events for "Sprint Planning", "Design Review", and "Weekly Standup"

**Test**:
```bash
gcal-organizer run --dry-run
```

**Expected**:
- Decision export messages appear for "Sprint Planning" and "Design Review"
- No decision export message for "Weekly Standup"
- Case-insensitive: "sprint planning" matches "Sprint Planning"

### V4: Per-Meeting Folders with Time-Based Filenames

**Setup**:
1. Have a meeting "Sprint Planning" at 9:00 AM on 2026-04-21

**Test**:
```bash
gcal-organizer run
ls ~/.gcal-organizer/decisions/sprint-planning/
```

**Expected**:
- Directory `sprint-planning/` was created automatically
- File `2026-04-21T09-00.md` exists in that directory
- File contains YAML frontmatter with `topic`, `date`, `time`, `attendees`, and `source` fields

### V5: Google Doc Source Link in Frontmatter

**Test**:
```bash
head -10 ~/.gcal-organizer/decisions/sprint-planning/2026-04-21T09-00.md
```

**Expected**:
```yaml
---
topic: Sprint Planning
date: "2026-04-21"
time: "09:00"
attendees:
  - alice@example.com
source: https://docs.google.com/document/d/<docID>/edit
---
```

### V6: Dry-Run Shows New Path Format

**Test**:
```bash
gcal-organizer run --dry-run
```

**Expected**:
- Log output shows paths like `sprint-planning/2026-04-21T09-00.md` (not the old flat format)

### V7: Environment Variable Override Still Works

**Setup**:
1. Have `config.yaml` with `days_to_look_back: 1`

**Test**:
```bash
GCAL_DAYS_TO_LOOK_BACK=7 gcal-organizer run --dry-run
```

**Expected**:
- The application looks back 7 days (env var overrides config file)

### V8: --config Flag with YAML Path

**Test**:
```bash
gcal-organizer doctor --config /path/to/custom/config.yaml
```

**Expected**:
- Configuration loaded from the specified YAML file

### V9: --config Flag with .env Path (Migration)

**Setup**:
1. Create a `.env` file at `/tmp/test-config/.env` with valid settings

**Test**:
```bash
gcal-organizer doctor --config /tmp/test-config/.env
```

**Expected**:
- `.env` is migrated to `/tmp/test-config/config.yaml`
- `.env` is deleted
- Configuration loaded from the new YAML file

### V10: Doctor Reports config.yaml Status

**Test**:
```bash
gcal-organizer doctor
```

**Expected**:
- Check output shows "Config file (config.yaml) exists" (not ".env")
- If `.env` exists alongside `config.yaml`, no warning (`.env` is ignored)

### V11: Service Mode Works Without .env

**Setup**:
1. Run `gcal-organizer install` after migration

**Test**:
```bash
cat ~/.local/bin/gcal-organizer-wrapper.sh
```

**Expected**:
- Wrapper script does NOT contain `.env` sourcing block
- Binary runs directly with config loaded from `config.yaml`

## Build Verification

```bash
# All must pass before feature is complete
go build ./...
go test ./...
go vet ./...
gofmt -l .        # Should produce no output
go mod tidy       # go.mod/go.sum should have no diff
make ci           # Full CI check
```
