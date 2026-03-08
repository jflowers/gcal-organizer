# Research: macOS Signed Releases

**Feature**: 010-macos-signed-releases
**Date**: 2026-03-08
**Reference implementation**: `unbound-force/gaze` (specs/015-native-macos-signing)

## Decision 1: Build Tool — GoReleaser v2

**Decision**: Use GoReleaser v2 via `goreleaser/goreleaser-action` in GitHub Actions to build, archive, checksum, and publish releases.

**Rationale**: GoReleaser handles cross-compilation, archive packaging, checksum generation, changelog creation, and Homebrew Cask publishing in a single declarative config file. It eliminates the current manual build loop in `release.yml` (which uses `GOOS`/`GOARCH` env vars and `softprops/action-gh-release`). The Gaze project uses GoReleaser v2 successfully with the same signing pipeline.

**Alternatives considered**:
- **Manual build loop (current approach)**: Requires custom scripting for archives, checksums, changelogs. More code to maintain, no Cask generation. Rejected because GoReleaser provides all of this out of the box.
- **goreleaser-cross**: Adds CGO cross-compilation support. Unnecessary since gcal-organizer uses `CGO_ENABLED=0`.

## Decision 2: Signing Approach — Native `codesign` + `notarytool` on macOS Runner

**Decision**: Use a separate `sign-macos` job on `macos-latest` that downloads darwin archives from the release, signs each binary with `codesign`, and submits to Apple's notary service via `xcrun notarytool`.

**Rationale**: This is Apple's supported code signing toolchain. The Gaze project previously attempted quill-based cross-platform signing (spec 014) but abandoned it due to two bugs: (1) quill never sets `TeamIdentifier` (upstream bug #147, open since Sep 2023), causing notarization to stall; (2) quill's polling exhausts Apple's API rate limit. Native `codesign`/`notarytool` on a macOS runner avoids both issues.

**Alternatives considered**:
- **GoReleaser built-in `notarize.macos` (quill)**: Cross-platform signing from ubuntu-latest using GoReleaser's embedded quill library. Rejected due to the TeamIdentifier bug and rate-limit exhaustion — both documented in Gaze spec 014.
- **gon (Mitchell Hashimoto's notarization tool)**: Archived/deprecated. No longer maintained. Rejected.
- **rcodesign (cross-platform Rust-based signing)**: Mature but adds a Rust dependency. Less proven in CI than native tools. Rejected for simplicity.

## Decision 3: Two-Job Split-Runner Workflow

**Decision**: Use two sequential jobs: `release` on ubuntu-latest (build + publish) → `sign-macos` on macos-latest (sign + replace). The sign-macos job is conditional on `has_signing_secrets`.

**Rationale**: macOS runners are slower and more expensive. Building on ubuntu-latest keeps the build fast and cheap. Only the signing step requires macOS APIs (`codesign`, `Security.framework` for keychain). This split also enables graceful degradation — if signing secrets aren't configured, the sign-macos job simply doesn't run.

**Alternatives considered**:
- **Single macOS job**: Build and sign in one job. Rejected because macOS runners are 3x slower for Go builds and more expensive. Also prevents graceful degradation.
- **Matrix build with per-platform jobs**: Run one job per OS/arch. Rejected as overly complex for this use case and incompatible with GoReleaser's built-in cross-compilation.

## Decision 4: Certificate Import — Temporary Keychain Pattern

**Decision**: Create a temporary macOS keychain in `$RUNNER_TEMP`, import the P12 certificate, set partition lists for `codesign` access, and use it for signing. The keychain is ephemeral (destroyed when the runner is recycled).

**Rationale**: The GitHub Actions macOS runner's default keychain is shared and may have access restrictions. A dedicated temporary keychain avoids conflicts and is the pattern recommended by Apple and used by all major signing CI integrations (Xcode Cloud, fastlane, the Gaze project).

**Alternatives considered**:
- **Import into default keychain**: Risk of access conflicts and password prompts in CI. Rejected.
- **Keychain-free signing (rcodesign)**: Would avoid keychain entirely but requires a non-Apple tool. Rejected for simplicity.

## Decision 5: Notarization via App Store Connect API Key

**Decision**: Authenticate with Apple's notary service using an App Store Connect API key (.p8 file) with `--key`, `--key-id`, and `--issuer` flags. Wait for notarization approval with `--wait --timeout 20m`.

**Rationale**: API key authentication is the modern Apple-recommended approach. It avoids storing Apple ID credentials and 2FA tokens in CI. The 20-minute timeout is generous (typical notarization takes 2-5 minutes) while preventing indefinite hangs.

**Alternatives considered**:
- **Apple ID + app-specific password**: Legacy approach. Requires 2FA management in CI. Rejected.
- **Fire-and-forget notarization (`--wait false`)**: Don't wait for approval. Rejected because the Cask checksums must be updated after signing, and we need to confirm notarization succeeded before publishing.

## Decision 6: Homebrew Distribution — Cask (Binary Download)

**Decision**: Switch from the current source-based Homebrew Formula to a Homebrew Cask that downloads pre-built binary archives. GoReleaser generates the initial Cask via `homebrew_casks`. The sign-macos job updates macOS checksums post-signing.

**Rationale**: A Cask installs the pre-built signed binary directly. Users don't need Go or Node installed. This matches the Gaze project pattern exactly. The current Formula requires `go` and `node` build dependencies and runs `npm install` during install — this is fragile and slow for end users. The signed binary in a Cask provides a better user experience.

**Alternatives considered**:
- **Keep source-based Formula**: Binary signing doesn't affect source-based builds. Users still need Go + Node. The signing effort only benefits direct GitHub Release downloads. Rejected because it doesn't deliver the signed binary to Homebrew users.
- **Hybrid (Formula + Cask)**: Maintain both. Rejected as confusing and doubles maintenance.

**Migration impact**:
- Remove `deploy/homebrew/gcal-organizer.rb` (Formula template)
- Remove `.github/workflows/bottles.yml` (builds from source — irrelevant with Cask)
- The tap repo (`jflowers/homebrew-gcal-organizer`) switches from `Formula/` to `Casks/` directory
- GoReleaser auto-generates and pushes `Casks/gcal-organizer.rb` on each release

## Decision 7: Release Asset Replacement Strategy

**Decision**: After signing, upload signed darwin archives to the existing GitHub Release using `gh release upload --clobber` (overwrites unsigned archives in-place). Then recompute checksums: strip darwin lines from `checksums.txt`, append new SHA256 hashes for signed archives, and re-upload.

**Rationale**: This preserves a single clean release without duplicate assets. The `--clobber` flag is an atomic replacement. The checksum update preserves linux lines (which didn't change) and only recomputes darwin lines. This is the exact pattern used by the Gaze project.

**Alternatives considered**:
- **Separate signed assets**: Upload signed archives alongside unsigned ones with different names (e.g., `-signed` suffix). Rejected because it confuses users about which to download and doubles the asset count.
- **Build signed from the start**: Would require building on macOS. Rejected per Decision 3.

## Decision 8: Archive Contents — Binary + Man Page

**Decision**: Include the compiled binary and the man page (`man/gcal-organizer.1`) in each release archive. Exclude browser automation scripts, deploy templates, and other non-binary files.

**Rationale**: The man page is useful for direct-download users who `tar xzf` the archive. Browser automation requires separate setup (`gcal-organizer setup-browser`) and is not a binary dependency. GoReleaser's `archives.files` field provides explicit inclusion control.

**Alternatives considered**:
- **Binary only**: Simpler but loses the man page for non-Homebrew users. Rejected.
- **Include browser/ directory**: Would require Node.js and npm install at user's end. Not appropriate for a binary archive. Rejected.

## Decision 9: Codesign Flags

**Decision**: Use `codesign --force --timestamp --options runtime --sign "Developer ID Application: John Flowers (PGFWLVZX55)"`.

**Rationale**:
- `--force`: Replaces any existing signature (safe for re-runs).
- `--timestamp`: Embeds a secure timestamp from Apple's timestamp server, proving the binary was signed while the certificate was valid.
- `--options runtime`: Enables hardened runtime, required for notarization since macOS 10.14.5.
- `--sign "Developer ID Application: ..."`: The full identity string matches the certificate in the keychain. Same identity used across all projects by this developer.

**Alternatives considered**:
- **Without `--options runtime`**: Notarization would be rejected. Not viable.
- **Without `--timestamp`**: Signature would expire when the certificate expires, even for binaries signed while it was valid. Rejected.
- **Entitlements file**: Not needed for a CLI tool that doesn't use restricted APIs (JIT, camera, microphone, etc.).

## Decision 10: Graceful Degradation — Secret Detection

**Decision**: The `release` job checks if `MACOS_SIGN_P12` secret is set and outputs `has_signing_secrets` (true/false). The `sign-macos` job uses `if: ${{ needs.release.outputs.has_signing_secrets == 'true' }}`.

**Rationale**: Checking a single representative secret is sufficient — if the P12 is configured, the other 4 secrets are expected to be configured too. This is simpler than checking all 5 secrets. The conditional `if` on the job level means the entire signing job is cleanly skipped (not failed) when secrets are absent.

**Alternatives considered**:
- **Check all 5 secrets**: More thorough but verbose. A partial configuration (e.g., P12 set but password missing) would cause a mid-job failure, which is an acceptable signal that configuration is incomplete. Rejected as over-engineering.
- **Repository variable instead of secret check**: Would require manual configuration of a `ENABLE_SIGNING` variable. Rejected because the secret existence check is automatic.
