# Feature Specification: macOS Signed Releases

**Feature Branch**: `010-macos-signed-releases`  
**Created**: 2026-03-08  
**Status**: Draft  
**Input**: User description: "Publish signed macOS binaries in the release pipeline"

## Clarifications

### Session 2026-03-08

- Q: Should Homebrew distribution use the current source-based Formula or switch to a binary-download Cask (matching the Gaze project pattern)? → A: Switch to a Homebrew Cask (binary-download) matching the Gaze project. The build tool generates the initial Cask with unsigned binary checksums, the signing pipeline updates the Cask's macOS checksums to match signed archives, and the existing source-based Formula and `bottles.yml` workflow are replaced.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Trusted macOS Binary Installation (Priority: P1)

As a macOS user downloading gcal-organizer from GitHub Releases, I want the binary to be code-signed and notarized by Apple so that macOS Gatekeeper does not block execution or display an "unidentified developer" warning.

**Why this priority**: This is the core value proposition. Without signing, macOS users must manually bypass Gatekeeper security warnings, which erodes trust and creates a poor first-run experience. Many enterprise-managed Macs outright block unsigned binaries.

**Independent Test**: Can be fully tested by downloading a darwin release archive, extracting the binary, and running it on a macOS machine with default Gatekeeper settings. The binary should execute without any security prompt or quarantine warning.

**Acceptance Scenarios**:

1. **Given** a new release tag is pushed, **When** the release pipeline completes, **Then** the macOS binaries in the GitHub Release are code-signed with a valid Developer ID Application certificate.
2. **Given** a signed macOS binary is published, **When** a user downloads and attempts to execute it on macOS with default security settings, **Then** Gatekeeper does not display an "unidentified developer" or quarantine warning.
3. **Given** a signed macOS binary is published, **When** the binary's code signature is inspected, **Then** it shows a valid Developer ID Application identity with a secure timestamp.
4. **Given** a signed macOS binary is published, **When** it is submitted for notarization, **Then** Apple's notary service accepts and approves the submission.

---

### User Story 2 - Accurate Checksums After Signing (Priority: P1)

As a security-conscious user, I want the published checksums file to reflect the actual signed binaries so that I can verify download integrity against the correct hashes.

**Why this priority**: Equal to P1 because incorrect checksums undermine the entire trust chain. If checksums correspond to unsigned binaries while signed binaries are served, verification fails and users lose confidence in the release.

**Independent Test**: Can be tested by downloading the checksums file and each release archive, computing SHA256 locally, and confirming every hash matches.

**Acceptance Scenarios**:

1. **Given** macOS binaries have been signed and replaced in the release, **When** a user downloads `checksums.txt`, **Then** the SHA256 hashes for darwin archives match the actual signed archives.
2. **Given** macOS binaries have been signed and replaced, **When** a user downloads a non-macOS (Linux) archive, **Then** its checksum in `checksums.txt` still matches the original unsigned archive (Linux binaries are unaffected by signing).

---

### User Story 3 - Graceful Degradation Without Signing Credentials (Priority: P2)

As a fork maintainer or contributor, I want the release pipeline to publish unsigned binaries successfully when signing credentials are not configured, so that forked projects can still create releases without macOS signing infrastructure.

**Why this priority**: Important for open-source health. Forks and contributors should not be blocked from releasing because they lack the maintainer's Apple Developer credentials.

**Independent Test**: Can be tested by triggering the release pipeline in a repository where the signing secrets are not configured and verifying that a complete release is published with unsigned binaries.

**Acceptance Scenarios**:

1. **Given** signing credentials are not configured in the repository, **When** a release tag is pushed, **Then** the release pipeline completes successfully and publishes all binaries (unsigned).
2. **Given** signing credentials are not configured, **When** the release pipeline runs, **Then** the signing step is skipped entirely (not failed) and no error is reported.

---

### User Story 4 - Homebrew Cask Integrity After Signing (Priority: P2)

As a Homebrew user, I want the Homebrew Cask in the tap repository to reflect correct checksums for signed macOS binary archives so that `brew install --cask` succeeds without hash mismatches.

**Why this priority**: Homebrew is a primary distribution channel. If the Cask checksums don't match the actual signed binary archives on the GitHub Release, installation fails for all Homebrew users.

**Independent Test**: Can be tested by running `brew install --cask jflowers/gcal-organizer/gcal-organizer` after a signed release and verifying it completes without checksum errors.

**Acceptance Scenarios**:

1. **Given** macOS binaries have been signed and published, **When** the pipeline updates the Homebrew tap, **Then** the Cask's SHA256 values for macOS archives match the signed binary archives.
2. **Given** a signed release has been published and the Cask updated, **When** a user runs `brew install --cask`, **Then** installation succeeds without checksum verification failures.
3. **Given** a signed release has been published, **When** the Cask is inspected in the tap repository, **Then** it downloads pre-built binary archives (not source) for each supported platform/architecture.

---

### User Story 5 - Standardized Release Archive Format (Priority: P3)

As a user downloading gcal-organizer, I want release binaries to be packaged as consistently-named tar.gz archives with a predictable naming convention so that I can script downloads and automate installations.

**Why this priority**: Lower priority because the tool is functional without standardized archives. However, consistent archive naming enables automation (e.g., `curl | tar` install scripts) and is a prerequisite for the signing pipeline to work correctly across architectures.

**Independent Test**: Can be tested by inspecting the GitHub Release assets and verifying that archives follow the naming convention and contain the expected binary plus man page.

**Acceptance Scenarios**:

1. **Given** a release tag is pushed, **When** the release pipeline completes, **Then** each platform/architecture combination is published as a tar.gz archive with a consistent naming pattern (e.g., `gcal-organizer_<version>_<os>_<arch>.tar.gz`).
2. **Given** a release archive is downloaded, **When** it is extracted, **Then** it contains the gcal-organizer binary and the man page.

---

### Edge Cases

- What happens when the Apple notarization service is temporarily unavailable or slow? The signing step should time out gracefully after a reasonable period (e.g., 20 minutes) and fail the signing job without affecting the already-published unsigned release.
- What happens when the signing certificate expires? The signing step should fail with a clear error message indicating the certificate issue, while the unsigned release remains available.
- What happens when only some signing secrets are configured (partial configuration)? The pipeline should treat this the same as no secrets configured and skip signing entirely rather than failing mid-process.
- What happens when a release is re-tagged or re-run? The pipeline should overwrite existing release assets with freshly signed binaries and update checksums accordingly.
- What happens when the Homebrew tap repository is temporarily unreachable? The signing and asset replacement should still succeed; the Homebrew Cask update step should fail independently without rolling back signed assets.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The release pipeline MUST build binaries for macOS (arm64, amd64) and Linux (arm64, amd64) from a single release trigger.
- **FR-002**: The release pipeline MUST package each platform/architecture binary as a tar.gz archive with a consistent naming convention: `gcal-organizer_<version>_<os>_<arch>.tar.gz`.
- **FR-003**: The release pipeline MUST generate a `checksums.txt` file containing SHA256 hashes for all release archives.
- **FR-004**: The release pipeline MUST publish a GitHub Release with all archives, the checksums file, and auto-generated release notes.
- **FR-005**: The release pipeline MUST code-sign macOS binaries using a Developer ID Application certificate when signing credentials are available.
- **FR-006**: The release pipeline MUST submit signed macOS binaries to Apple's notarization service and wait for approval before publishing.
- **FR-007**: The release pipeline MUST verify the code signature on each macOS binary after signing and before publishing.
- **FR-008**: The release pipeline MUST replace the initially-published unsigned macOS archives with signed versions in the GitHub Release.
- **FR-009**: The release pipeline MUST recompute and update the checksums file after replacing unsigned macOS archives with signed ones, preserving non-macOS checksums.
- **FR-010**: The release pipeline MUST skip the signing step entirely (without error) when signing credentials are not configured.
- **FR-011**: The release pipeline MUST detect credential availability before attempting to sign, using a check that outputs the result for conditional job execution.
- **FR-012**: The release pipeline MUST publish a Homebrew Cask to the tap repository that downloads pre-built binary archives for each supported platform/architecture.
- **FR-013**: The signing pipeline MUST update the Homebrew Cask's macOS SHA256 checksums in the tap repository to match the signed binary archives after signing is complete.
- **FR-014**: The release pipeline MUST include the man page (`gcal-organizer.1`) in each release archive alongside the binary.
- **FR-015**: The release pipeline MUST inject the release version into the binary at build time so that `gcal-organizer --version` reports the correct tag.
- **FR-016**: Signing credentials MUST be stored as repository secrets, never committed to the codebase or logged in pipeline output.
- **FR-017**: The existing source-based Homebrew Formula and `bottles.yml` workflow MUST be removed as part of the migration to the Cask-based distribution model.

### Key Entities

- **Release Archive**: A tar.gz file containing the built binary and man page for a specific platform/architecture combination. Named `gcal-organizer_<version>_<os>_<arch>.tar.gz`. Exists in two lifecycle states: unsigned (initial publish) and signed (post-signing replacement, macOS only).
- **Checksums File**: A text file (`checksums.txt`) containing SHA256 hashes for all release archives. Updated after signing to reflect signed macOS archives while preserving Linux hashes.
- **Signing Credential Set**: The collection of five secrets required for macOS code signing and notarization: the Developer ID certificate (P12), its password, the App Store Connect API key (P8), the API key ID, and the issuer ID.
- **Homebrew Cask**: The Ruby file in the tap repository (`Casks/gcal-organizer.rb`) that defines how Homebrew installs gcal-organizer by downloading pre-built binary archives. Contains per-platform SHA256 checksums that are updated after signing to match the signed macOS archives.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of macOS release binaries pass Apple Gatekeeper verification on a fresh macOS machine with default security settings (no manual bypass needed).
- **SC-002**: 100% of checksums in the published `checksums.txt` match the actual release archive contents (both macOS and Linux).
- **SC-003**: The signing and notarization process completes within 30 minutes of the initial release build finishing.
- **SC-004**: The release pipeline succeeds without errors in repositories where signing credentials are not configured (graceful degradation).
- **SC-005**: `brew install --cask` from the tap succeeds without checksum mismatches after a signed release.
- **SC-006**: The release pipeline produces archives for all 4 target platforms (darwin/arm64, darwin/amd64, linux/arm64, linux/amd64) on every release.

## Assumptions

- The Apple Developer ID Application certificate is valid and not expired at the time of release.
- The App Store Connect API key has sufficient permissions for notarization submissions.
- The signing identity (`Developer ID Application: John Flowers (PGFWLVZX55)`) is the same identity used across all projects by this developer.
- The Homebrew tap repository (`jflowers/homebrew-gcal-organizer`) will migrate from a source-based Formula (under `Formula/`) to a binary-download Cask (under `Casks/`), matching the pattern used by the Gaze project's `unbound-force/homebrew-tap`.
- macOS CLI binaries (bare Mach-O executables) cannot be stapled; notarization approval is verified online by Gatekeeper via Apple's servers.
- The existing `bottles.yml` workflow and source-based Formula (`deploy/homebrew/gcal-organizer.rb`) are superseded by the Cask-based distribution and will be removed.
