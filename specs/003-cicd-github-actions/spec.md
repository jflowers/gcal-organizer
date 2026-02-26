# Feature Specification: CI/CD with GitHub Actions

**Feature Branch**: `003-cicd-github-actions`  
**Created**: 2026-02-07  
**Status**: Implemented  
**Input**: Set up CI/CD pipeline with GitHub Actions for automated quality checks and releases

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Automated Quality Checks on Push (Priority: P1)

As a developer, I want every push and pull request to automatically run build, vet, format checks, and tests so I can catch issues early.

**Why this priority**: Core CI functionality — prevents broken code from being merged

**Independent Test**: Push a commit and verify the workflow runs and reports results

**Acceptance Scenarios**:

1. **Given** a push to `main`, **When** GitHub Actions triggers, **Then** the workflow runs `go build`, `go vet`, `gofmt -l`, and `go test` and reports pass/fail
2. **Given** a pull request is opened, **When** GitHub Actions triggers, **Then** the same checks run and block merge if any fail
3. **Given** tests fail, **When** viewing the workflow run, **Then** failure details are clearly visible in the logs

---

### User Story 2 - Automated Release on Tag (Priority: P2)

As a developer, I want pushing a version tag (e.g., `v1.1.0`) to automatically create a GitHub Release with cross-compiled binaries.

**Why this priority**: Eliminates manual release process and ensures consistent builds

**Independent Test**: Push a tag and verify a GitHub Release is created with binaries

**Acceptance Scenarios**:

1. **Given** a tag `v*` is pushed, **When** GitHub Actions triggers, **Then** binaries are built for macOS (arm64, amd64) and Linux (amd64, arm64)
2. **Given** the release workflow completes, **When** viewing the GitHub Releases page, **Then** a release with the tag name, changelog, and downloadable binaries is available
3. **Given** the CI checks fail, **When** the release workflow runs, **Then** the release is NOT created

---

### Edge Cases

- What happens when `go.sum` is out of sync?
  → `go mod tidy` check catches this in CI
- What happens when `gofmt` has unformatted files?
  → `gofmt -l` lists them and the workflow fails
- What happens when a tag is pushed on a broken commit?
  → Release workflow depends on CI passing first
- What happens when system Go version doesn't match project Go version?
  → Pre-push hook auto-detects goenv Go 1.24.6; CI uses `actions/setup-go` with `1.24`

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: CI workflow MUST trigger on push to `main` and on pull requests
- **FR-002**: CI workflow MUST run `go build ./cmd/gcal-organizer`
- **FR-003**: CI workflow MUST run `go vet ./...`
- **FR-004**: CI workflow MUST check formatting with `gofmt -l`
- **FR-005**: CI workflow MUST run `go test ./...`
- **FR-006**: Release workflow MUST trigger on `v*` tags
- **FR-007**: Release workflow MUST build binaries for macOS (arm64, amd64) and Linux (amd64, arm64)
- **FR-008**: Release workflow MUST create a GitHub Release with the binaries attached
- **FR-009**: Release workflow MUST generate a versioned Homebrew formula with correct SHA256
- **FR-010**: Release workflow MUST package and attach the man page to the release
- **FR-011**: CI workflow MUST check that `go mod tidy` produces no diff to go.mod/go.sum
- **FR-012**: A `make ci` target MUST exist that mirrors GitHub Actions CI checks exactly (check-only gofmt, go mod tidy diff, vet, build, test -race)
- **FR-013**: Pre-push hooks SHOULD be available via `make install-hooks` to catch CI failures locally
- **FR-014**: Release workflow MUST auto-publish the Homebrew formula to the tap repo (`jflowers/homebrew-gcal-organizer`)
- **FR-015**: A bottle build workflow MUST build pre-compiled bottles for macOS (arm64) and Linux (x86_64) on release

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Every push to `main` triggers a CI run that completes within 3 minutes
- **SC-002**: Failed checks are clearly reported with actionable error messages
- **SC-003**: Pushing a version tag produces a GitHub Release with downloadable binaries within 5 minutes
- **SC-004**: Bottles are built and uploaded to the release within 15 minutes of release creation
- **SC-005**: `make ci` locally produces the same pass/fail result as GitHub Actions CI
