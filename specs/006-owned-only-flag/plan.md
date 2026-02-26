# Implementation Plan: Owned-Only Mode for File Mutation Protection

**Branch**: `006-owned-only-flag` | **Date**: 2026-02-26 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/006-owned-only-flag/spec.md`

## Summary

Add a global `--owned-only` persistent CLI flag (and config file option) that prevents all mutations on files the authenticated user does not own. When active, the system skips move, trash, share, and task assignment operations on non-owned files while still creating shortcuts for discoverability. The implementation touches the config layer, CLI flag registration, and three orchestration code paths (organize, sync-calendar, assign-tasks).

## Technical Context

**Language/Version**: Go 1.21+
**Primary Dependencies**: github.com/spf13/cobra (CLI), github.com/spf13/viper (config), Google Drive API v3
**Storage**: N/A (no new data persistence; flag stored in config file via existing viper mechanism)
**Testing**: `go test ./...` (table-driven tests, existing test framework)
**Target Platform**: CLI (macOS, Linux)
**Project Type**: Single project (cmd/, internal/, pkg/ layout)
**Performance Goals**: No performance regression; ownership check uses data already fetched by existing API calls (no additional API calls in organize path). Calendar sync path requires one additional `GetFileInfo` call per attachment for ownership resolution.
**Constraints**: Must not break existing behavior when flag is not set. Must follow existing flag registration patterns (`--dry-run`, `--verbose`).
**Scale/Scope**: Small feature — 6 files modified, 0 new source files, ~80 lines of new logic.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Gate | Principle | Status | Notes |
|------|-----------|--------|-------|
| CLI-First (I) | Flag follows Unix conventions, config file support | **PASS** | `--owned-only` is a standard boolean flag; persistent on root like `--dry-run` |
| Auth (II) | No credential changes | **PASS** | Uses existing OAuth2 identity (`currentUserEmail`) for ownership comparison |
| TDD (III) | Tests required before implementation | **PASS** | Plan includes unit tests for config, ownership filtering logic, and integration tests for organizer |
| Idiomatic Go (IV) | Standard layout, error handling | **PASS** | Modifies existing packages only; follows `fmt.Errorf` with `%w` wrapping |
| Graceful Errors (V) | Actionable error messages | **PASS** | `assign-tasks --doc` with non-owned doc produces clear error with guidance |
| Observability (VI) | Logging, dry-run support | **PASS** | Verbose logging of skipped files, dry-run interaction, summary counts |
| Self-Serve Diagnostics (VII) | Error messages reference `doctor` | **PASS** | API-failure errors (e.g., cannot verify ownership) include `doctor` reference per Constitution VII. Business-logic errors (e.g., "not owned by you") intentionally omit `doctor` — these are expected responses to user input, not diagnostic scenarios |
| Documentation (Governance) | README, man page updates required | **PASS** | Plan includes doc updates for README.md, man page, SETUP.md |
| Zero Regression (Governance) | Default behavior unchanged | **PASS** | Flag defaults to `false`; all existing code paths unchanged when not set |

**Gate result: ALL PASS. Proceeding to Phase 0.**

## Project Structure

### Documentation (this feature)

```text
specs/006-owned-only-flag/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (repository root)

```text
internal/
├── config/
│   ├── config.go          # [MODIFY] Add OwnedOnly field to Config struct
│   └── config_test.go     # [MODIFY] Add OwnedOnly default and env tests
├── drive/
│   ├── service.go         # [MODIFY] Add IsFileOwned() helper; fix GetFileInfo to populate IsOwned
│   └── service_test.go    # [MODIFY] Add IsOwned test coverage for parseDocument
├── organizer/
│   ├── organizer.go       # [MODIFY] Add ownership filtering in OrganizeDocuments and SyncCalendarAttachments
│   └── organizer_test.go  # [NEW] Unit tests for ownership filtering logic
cmd/gcal-organizer/
├── main.go                # [MODIFY] Register --owned-only flag, filter Step 3 notesDocIDs
└── assign_tasks.go        # [MODIFY] Add ownership check for standalone assign-tasks
docs/
├── README.md              # [MODIFY] Document --owned-only flag
├── SETUP.md               # [MODIFY] Add owned-only configuration example
└── man/gcal-organizer.1   # [MODIFY] Add --owned-only to man page
```

**Structure Decision**: Existing single-project Go layout. No new packages or directories needed. All changes are modifications to existing files except `internal/organizer/organizer_test.go` which is new (no organizer tests currently exist).

## Proposed Changes

### Phase 1: Config & Flag Registration

#### [MODIFY] `internal/config/config.go`
- Add `OwnedOnly bool` field to `Config` struct (after `DryRun` field, line ~43)
- In `Load()`: add `cfg.OwnedOnly = viper.GetBool("owned-only")` (following `DryRun` pattern at line ~111)
- No validation changes needed (`OwnedOnly` is a simple boolean with no dependencies)

#### [MODIFY] `internal/config/config_test.go`
- Add `OwnedOnly` assertion to `TestDefaultConfig` (expect `false`)
- Add env var test: set `GCAL_OWNED_ONLY=true`, verify `cfg.OwnedOnly == true`

#### [MODIFY] `cmd/gcal-organizer/main.go`
- Add package-level `var ownedOnly bool` (line ~38, alongside `dryRun`)
- In `init()`: register persistent flag `rootCmd.PersistentFlags().BoolVar(&ownedOnly, "owned-only", false, "only mutate files you own; skip non-owned files")`
- In `init()`: bind to viper `viper.BindPFlag("owned-only", rootCmd.PersistentFlags().Lookup("owned-only"))`
- In each command's `RunE` that loads config: add `cfg.OwnedOnly = ownedOnly` (following `cfg.DryRun = dryRun` pattern)

### Phase 2: Drive Service Enhancement

#### [MODIFY] `internal/drive/service.go`
- Add `IsFileOwned(ctx context.Context, fileID string) (bool, error)` method:
  - Calls `Files.Get(fileID).Fields("owners")` 
  - Compares `owners[].EmailAddress` against `s.currentUserEmail`
  - Returns `(true, nil)` if owned, `(false, nil)` if not, `(false, err)` on API failure
  - This is needed for calendar attachment ownership checks (Step 3 / sync-calendar) where `Document.IsOwned` is not available
- Fix `GetFileInfo()` to populate `IsOwned` on the returned `Document` using the same ownership comparison logic from `parseDocument()`

#### [MODIFY] `internal/drive/service_test.go`
- Add `Owners` field to test `drive.File` structs in `parseDocument` tests
- Add test cases verifying `IsOwned == true` when owner matches `currentUserEmail`
- Add test cases verifying `IsOwned == false` when owner differs
- Add test cases verifying `IsOwned == false` when `Owners` is empty (fail-safe)

### Phase 3: Organizer Filtering

#### [MODIFY] `internal/organizer/organizer.go`

**In `OrganizeDocuments()` (line ~149 loop):**
- Add ownership filtering with skip tracking:
  ```
  if o.config.OwnedOnly && !doc.IsOwned {
      o.stats.Skipped++
      if o.config.Verbose {
          log: "Skipping non-owned document: %s (owner: %s)"
      }
      // Still create shortcut for discoverability (FR-005)
      result = o.drive.CreateShortcut(...)
      continue  // Skip move/trash operations
  }
  ```

**In `SyncCalendarAttachments()` (line ~244 sharing block):**
- Add ownership check before `ShareFile()`:
  ```
  if o.config.OwnedOnly {
      owned, err := o.drive.IsFileOwned(ctx, att.FileID)
      if err != nil || !owned {
          o.stats.Skipped++
          if o.config.Verbose {
              log: "Skipping share for non-owned attachment: %s"
          }
          continue  // Skip sharing, shortcut already created above
      }
  }
  ```

**In `SyncCalendarAttachments()` (line ~231 notesDocIDs collection):**
- Add ownership filter when `OwnedOnly` is active:
  ```
  if o.config.OwnedOnly {
      owned, err := o.drive.IsFileOwned(ctx, att.FileID)
      if err != nil || !owned {
          continue  // Don't collect non-owned docs for Step 3
      }
  }
  ```

**In `Stats` struct and `PrintSummary()`:**
- Add `Skipped int` field to `Stats`
- In `PrintSummary()`: when `Skipped > 0`, print `"Skipped %d non-owned files (--owned-only active)"`

#### [NEW] `internal/organizer/organizer_test.go`
- Table-driven tests for `OrganizeDocuments()` with mock Drive service:
  - Test: OwnedOnly=false processes all documents (no filtering)
  - Test: OwnedOnly=true skips non-owned documents but creates shortcuts
  - Test: OwnedOnly=true processes owned documents normally
  - Test: Stats.Skipped count is correct
- Table-driven tests for `SyncCalendarAttachments()` ownership filtering:
  - Test: OwnedOnly=true skips ShareFile for non-owned attachments
  - Test: OwnedOnly=true excludes non-owned docs from notesDocIDs

### Phase 4: Assign-Tasks Integration

#### [MODIFY] `cmd/gcal-organizer/assign_tasks.go`
- In `assignTasksCmd.RunE` (standalone): after loading doc ID, check ownership:
  ```
  if ownedOnly {
      owned, err := driveSvc.IsFileOwned(ctx, docID)
      if err != nil {
          return fmt.Errorf("cannot verify ownership of document %s: %w\n\nRun 'gcal-organizer doctor' for diagnostics", docID, err)
      }
      if !owned {
          return fmt.Errorf("document %s is not owned by you; --owned-only prevents processing non-owned documents", docID)
      }
  }
  ```
- Exit with non-zero status (cobra handles this via returned error)

#### [MODIFY] `cmd/gcal-organizer/main.go` (Step 3 in `runCmd`)
- The notesDocIDs are already filtered at collection time in organizer.go (Phase 3 above), so no additional filtering needed here
- Add verbose log when `ownedOnly && len(docIDs) == 0`: `"No owned Notes documents found for task assignment"`

### Phase 5: Documentation

#### [MODIFY] `README.md`
- Add `--owned-only` to the flags reference table
- Add usage example: `gcal-organizer run --owned-only --verbose`
- Add a section explaining owned-only behavior

#### [MODIFY] `man/gcal-organizer.1`
- Add `--owned-only` flag description
- Add example in EXAMPLES section

#### [MODIFY] `docs/SETUP.md`
- Add `owned-only` to config file example
- Add note about Shared Drive limitation

### Phase 6: Dry-Run Interaction

#### [MODIFY] `internal/organizer/organizer.go`
- In dry-run output paths: when both `DryRun` and `OwnedOnly` are active, include ownership info in dry-run messages:
  - `"Would skip non-owned document: %s (owner: %s)"`
  - `"Would skip sharing non-owned attachment: %s"`

## Verification Plan

### Automated Tests
1. `go test ./internal/config/...` — OwnedOnly default, env override
2. `go test ./internal/drive/...` — IsOwned population, IsFileOwned helper
3. `go test ./internal/organizer/...` — Ownership filtering in OrganizeDocuments and SyncCalendarAttachments
4. `go vet ./...` — No warnings
5. `gofmt -l .` — No formatting issues
6. `make ci` — Full CI suite passes

### Manual Tests
1. `gcal-organizer organize --owned-only --verbose` — Verify non-owned files skipped, shortcuts created
2. `gcal-organizer organize` (without flag) — Verify no regression
3. `gcal-organizer sync-calendar --owned-only --verbose` — Verify non-owned attachments not shared
4. `gcal-organizer assign-tasks --doc <non-owned> --owned-only` — Verify error exit
5. `gcal-organizer run --owned-only --dry-run` — Verify dry-run output reflects ownership filtering
6. Set `GCAL_OWNED_ONLY=true` in config, run without flag — Verify behavior active

## Complexity Tracking

No constitution violations. All changes follow existing patterns. No complexity justification needed.
