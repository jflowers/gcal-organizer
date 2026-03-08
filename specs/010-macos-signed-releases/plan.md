# Implementation Plan: macOS Signed Releases

**Branch**: `010-macos-signed-releases` | **Date**: 2026-03-08 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/010-macos-signed-releases/spec.md`

## Summary

Replace the current manual build-loop release pipeline with a GoReleaser-based two-job workflow that builds cross-platform tar.gz archives on ubuntu-latest, then signs and notarizes macOS binaries on a separate macos-latest runner. Migrate Homebrew distribution from a source-based Formula to a binary-download Cask. The design is modeled on the proven pipeline in the Gaze project (`unbound-force/gaze`).

## Technical Context

**Language/Version**: Go 1.24.0 (toolchain go1.24.12)
**Primary Dependencies**: GoReleaser v2 (CI only — `goreleaser/goreleaser-action`), `gh` CLI (release asset management), native macOS `codesign` + `xcrun notarytool` (signing/notarization)
**Storage**: N/A (no data persistence; pipeline configuration files only)
**Testing**: Manual verification via test tag releases (`v0.0.0-test.*`), `codesign --verify`, `spctl --assess`, `shasum -a 256`, `brew install --cask`
**Target Platform**: GitHub Actions (ubuntu-latest for builds, macos-latest for signing)
**Project Type**: Single project — CI/CD pipeline configuration changes only
**Performance Goals**: Signing + notarization completes within 30 minutes of release job finishing
**Constraints**: No Go code changes required; no CGO; signing job conditional on secret availability
**Scale/Scope**: 4 target platforms (darwin/arm64, darwin/amd64, linux/arm64, linux/amd64), 5 signing secrets, 2 CI workflow jobs

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. CLI-First Architecture | **PASS** | No CLI changes. Binary output unchanged. Version injection preserved via ldflags. |
| II. API-Key Authentication Mode | **PASS** | No authentication changes. Signing credentials are CI-only secrets, never in application code. Aligns with "Never log or expose credentials in output." |
| III. Test-Driven Development | **PASS** | No Go code changes, so no new unit tests needed. Verification is via manual release testing (codesign --verify, brew install --cask). |
| IV. Idiomatic Go | **PASS** | No Go code changes. GoReleaser uses standard `go build` with CGO_ENABLED=0. |
| V. Graceful Error Handling | **PASS** | Pipeline gracefully degrades when signing secrets absent (FR-010). Signing failures don't block unsigned release. |
| VI. Observability | **PASS** | Pipeline steps produce verbose output (codesign --verify --verbose=2, notarytool submission logs). |
| VII. Self-Serve Diagnostics | **N/A** | No user-facing diagnostic changes. |
| Quality Gates (go build, go test, go vet, gofmt, go mod tidy) | **PASS** | Release job runs `go test -race ./...` before GoReleaser. GoReleaser runs `go build`. Existing CI workflow unchanged. |
| Documentation Requirements | **PASS** | README.md will be updated with new installation instructions (Cask-based). Man page included in archives. |
| No CGO | **PASS** | `CGO_ENABLED=0` set explicitly in GoReleaser config. |
| Security Review | **PASS** | All credentials stored as GitHub Secrets. Temporary keychain created and destroyed per job. No secrets logged. P12 and P8 decoded to ephemeral runner temp directory. |

**Gate result: PASS — no violations. Proceeding to Phase 0.**

## Project Structure

### Documentation (this feature)

```text
specs/010-macos-signed-releases/
├── plan.md              # This file
├── research.md          # Phase 0 output — architectural decisions
├── data-model.md        # Phase 1 output — entity/lifecycle model
├── quickstart.md        # Phase 1 output — verification guide
└── tasks.md             # Phase 2 output (created by /speckit.tasks)
```

### Source Code (repository root)

```text
.goreleaser.yaml                          # NEW — GoReleaser v2 configuration
.github/workflows/release.yml             # MODIFIED — two-job pipeline (release + sign-macos)
.github/workflows/bottles.yml             # REMOVED — superseded by Cask distribution
deploy/homebrew/gcal-organizer.rb          # REMOVED — superseded by GoReleaser Cask generation
```

**Structure Decision**: This feature modifies only CI/CD pipeline configuration files at the repository root. No changes to `cmd/`, `internal/`, `pkg/`, or any Go source code. The GoReleaser config (`.goreleaser.yaml`) is a new file at the repo root (standard location). The existing `release.yml` is rewritten. The `bottles.yml` and `deploy/homebrew/gcal-organizer.rb` are removed as part of the Formula-to-Cask migration.

## Constitution Check — Post-Design Re-evaluation

*Re-checked after Phase 1 design completion.*

| Principle | Pre-design | Post-design | Notes |
|-----------|-----------|-------------|-------|
| I. CLI-First Architecture | PASS | **PASS** | No CLI changes in design. |
| II. API-Key Authentication Mode | PASS | **PASS** | Signing secrets are CI-only, separate from application auth. |
| III. Test-Driven Development | PASS | **PASS** | No Go code changes. Verification via manual test tags documented in quickstart.md. |
| IV. Idiomatic Go | PASS | **PASS** | GoReleaser uses standard `go build`. CGO_ENABLED=0. |
| V. Graceful Error Handling | PASS | **PASS** | Graceful degradation designed (Decision 10 in research.md). |
| VI. Observability | PASS | **PASS** | Verbose codesign verification and notarytool output in pipeline logs. |
| VII. Self-Serve Diagnostics | N/A | **N/A** | No user-facing diagnostic changes. |
| Quality Gates | PASS | **PASS** | Release job runs `go test -race ./...` before GoReleaser. |
| Documentation Requirements | PASS | **PASS** | README.md update needed (Cask install instructions). |
| No CGO | PASS | **PASS** | Explicit `CGO_ENABLED=0` in GoReleaser config. |
| Security Review | PASS | **PASS** | Temporary keychain pattern documented (Decision 4 in research.md). All secrets ephemeral. |

**Post-design gate result: PASS — no violations introduced. No complexity tracking needed.**
