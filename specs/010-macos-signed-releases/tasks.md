# Tasks: macOS Signed Releases

**Input**: Design documents from `/specs/010-macos-signed-releases/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, quickstart.md

**Tests**: No automated tests requested. This feature modifies only CI/CD pipeline configuration files. Verification is manual via test tag releases (see quickstart.md).

**Organization**: Tasks are grouped by user story. Note that US5 (archives) and US3 (graceful degradation) are foundational — they are structural properties of the GoReleaser config and release job that all other stories depend on.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Secrets & Credentials)

**Purpose**: Configure GitHub repository secrets required for macOS code signing and notarization

- [x] T001 Set `MACOS_SIGN_P12` secret on `jflowers/gcal-organizer` via `gh secret set` using base64-encoded P12 certificate value from `/Users/jflowers/Projects/github/unbound-force/temp/env.md`
- [x] T002 [P] Set `MACOS_SIGN_PASSWORD` secret on `jflowers/gcal-organizer` via `gh secret set` using password value from `/Users/jflowers/Projects/github/unbound-force/temp/env.md`
- [x] T003 [P] Set `MACOS_NOTARY_KEY` secret on `jflowers/gcal-organizer` via `gh secret set` using base64-encoded P8 key value from `/Users/jflowers/Projects/github/unbound-force/temp/env.md`
- [x] T004 [P] Set `MACOS_NOTARY_KEY_ID` secret on `jflowers/gcal-organizer` via `gh secret set` with value `4K669B7BD9`
- [x] T005 [P] Set `MACOS_NOTARY_ISSUER_ID` secret on `jflowers/gcal-organizer` via `gh secret set` with value `f3feda93-660b-47a6-a402-7f95d678ca7c`
- [x] T006 Verify `HOMEBREW_TAP_GITHUB_TOKEN` secret exists on `jflowers/gcal-organizer` (rename from existing `HOMEBREW_TAP_TOKEN` if needed) via `gh secret list`

---

## Phase 2: Foundational (GoReleaser Config + Release Job)

**Purpose**: Create GoReleaser v2 configuration and rewrite the release job. This phase covers **US5** (standardized archives) and **US3** (graceful degradation) as structural properties of the pipeline.

**CRITICAL**: No signing work (Phase 3) can begin until this phase is complete.

### US5 — Standardized Release Archive Format (P3)

- [x] T007 [US5] Create `.goreleaser.yaml` at repository root with GoReleaser v2 schema: builds section targeting `./cmd/gcal-organizer` with `CGO_ENABLED=0`, `goos: [darwin, linux]`, `goarch: [amd64, arm64]`, ldflags `-X main.Version={{.Tag}}` (per data-model.md Entity: GoReleaser Configuration)
- [x] T008 [US5] Add archives section to `.goreleaser.yaml`: format `tar.gz`, name template `gcal-organizer_{{ .Version }}_{{ .Os }}_{{ .Arch }}`, files list including `LICENSE*` and `man/gcal-organizer.1` (FR-002, FR-014)
- [x] T009 [US5] Add checksum section to `.goreleaser.yaml`: name template `checksums.txt` (FR-003)
- [x] T010 [US5] Add changelog section to `.goreleaser.yaml`: grouped by features/fixes/docs, exclude `chore:` commits (matching Gaze reference at `/Users/jflowers/Projects/github/unbound-force/gaze/.goreleaser.yaml`)
- [x] T011 [US5] Add `homebrew_casks` section to `.goreleaser.yaml`: name `gcal-organizer`, directory `Casks`, repository `{owner: jflowers, name: homebrew-gcal-organizer}`, token `{{ .Env.HOMEBREW_TAP_GITHUB_TOKEN }}`, binaries `[gcal-organizer]`, manpages `[man/gcal-organizer.1]`, `skip_upload: auto`, description and homepage fields (FR-012, data-model.md Entity: Homebrew Cask)
- [x] T012 [US5] Validate GoReleaser config locally by running `goreleaser check` and `goreleaser release --snapshot --clean`, then inspect `dist/*.tar.gz` to confirm each archive contains `gcal-organizer` binary, `man/gcal-organizer.1`, and `LICENSE`

### US3 — Graceful Degradation (P2) + Release Job Core

- [x] T013 [US3] Rewrite `.github/workflows/release.yml` — replace the entire current content with a two-job pipeline. Job 1 (`release`): runs on `ubuntu-latest` with 45-minute timeout. Add `check-secrets` step that checks if `MACOS_SIGN_P12` is set and outputs `has_signing_secrets` (true/false) via `$GITHUB_OUTPUT` (FR-010, FR-011, research.md Decision 10)
- [x] T014 [US3] In `.github/workflows/release.yml` release job: add checkout step (actions/checkout with `fetch-depth: 0`), setup Go step (actions/setup-go with `go-version-file: go.mod`), and GoReleaser step (`goreleaser/goreleaser-action` v2 with `args: release --clean`) passing `GITHUB_TOKEN` and `HOMEBREW_TAP_GITHUB_TOKEN` env vars (FR-001, FR-004, FR-015)
- [x] T015 [US3] In `.github/workflows/release.yml`: add `outputs` section to `release` job declaring `has_signing_secrets: ${{ steps.check-secrets.outputs.has_secrets }}` and pin all action versions to commit SHAs matching the Gaze reference at `/Users/jflowers/Projects/github/unbound-force/gaze/.github/workflows/release.yml`

**Checkpoint**: At this point, pushing a tag should produce a working release with unsigned tar.gz archives, checksums.txt, changelog, and Homebrew Cask. The sign-macos job doesn't exist yet so it will be skipped. US5 and US3 are independently testable.

---

## Phase 3: User Story 1 + User Story 2 — Signing, Notarization & Checksums (Priority: P1) MVP

**Goal**: Add the `sign-macos` job that code-signs darwin binaries, notarizes them with Apple, replaces unsigned archives, and updates checksums. US1 and US2 are implemented together because the checksum update (US2) is a direct consequence of the asset replacement step in signing (US1).

**Independent Test**: Push a test tag (`v0.0.0-test.1`), wait for both jobs to complete, then download a darwin archive and verify with `codesign -dv --verbose=4` and `spctl --assess --type execute -v`. Download `checksums.txt` and verify with `shasum -a 256 -c checksums.txt`.

### Implementation

- [x] T016 [US1] In `.github/workflows/release.yml`: add `sign-macos` job skeleton — runs on `macos-latest`, `needs: release`, conditional `if: ${{ needs.release.outputs.has_signing_secrets == 'true' }}`, 30-minute timeout (research.md Decision 3)
- [x] T017 [US1] In `.github/workflows/release.yml` sign-macos job: add "Import certificate into Keychain" step — decode `MACOS_SIGN_P12` base64 to `$RUNNER_TEMP/cert.p12`, create temporary keychain with random password, import cert, set partition list for `codesign` access, set as user keychain (research.md Decision 4, data-model.md Temporary Artifacts). Reference implementation at `/Users/jflowers/Projects/github/unbound-force/gaze/.github/workflows/release.yml` lines 55-69
- [x] T018 [US1] In `.github/workflows/release.yml` sign-macos job: add "Prepare notary key" step — decode `MACOS_NOTARY_KEY` base64 to `$RUNNER_TEMP/notary_key.p8` (research.md Decision 5, data-model.md Temporary Artifacts). Reference: Gaze release.yml lines 71-75
- [x] T019 [US1] In `.github/workflows/release.yml` sign-macos job: add "Download darwin archives" step — use `gh release download "${GITHUB_REF_NAME}"` with pattern `gcal-organizer_*_darwin_*.tar.gz` to `./artifacts` directory. Reference: Gaze release.yml lines 77-81
- [x] T020 [US1] In `.github/workflows/release.yml` sign-macos job: add "Sign and notarize" step — loop over each `./artifacts/gcal-organizer_*_darwin_*.tar.gz`: extract to temp dir, run `codesign --force --timestamp --options runtime --sign "Developer ID Application: John Flowers (PGFWLVZX55)"`, verify with `codesign --verify --verbose=2`, create zip via `ditto -c -k`, submit to `xcrun notarytool submit --wait --timeout 20m` with key/key-id/issuer args, re-archive signed binary to `./signed/` (FR-005, FR-006, FR-007, research.md Decision 9). Reference: Gaze release.yml lines 83-116
- [x] T021 [US2] In `.github/workflows/release.yml` sign-macos job: add "Replace release assets and update checksums" step — upload signed archives with `gh release upload --clobber`, download existing `checksums.txt`, strip darwin lines with `grep -v darwin`, recompute SHA256 for each signed archive with `shasum -a 256`, re-upload checksums (FR-008, FR-009, research.md Decision 7). Reference: Gaze release.yml lines 121-141

**Checkpoint**: At this point, pushing a tag should produce a release with signed macOS binaries and accurate checksums. US1 and US2 are independently verifiable per quickstart.md Steps 4-5.

---

## Phase 4: User Story 4 — Homebrew Cask Integrity After Signing (Priority: P2)

**Goal**: After signing darwin archives, update the Homebrew Cask file in the tap repository so its macOS SHA256 checksums match the signed binary archives.

**Independent Test**: After a signed release, run `brew tap jflowers/gcal-organizer && brew install --cask jflowers/gcal-organizer/gcal-organizer` and verify it installs without checksum errors.

### Implementation

- [x] T022 [US4] In `.github/workflows/release.yml` sign-macos job: add "Update Homebrew cask checksums" step — compute signed checksums for `darwin_arm64` and `darwin_amd64` archives, clone `jflowers/homebrew-gcal-organizer` tap repo using `HOMEBREW_TAP_GITHUB_TOKEN`, locate `Casks/gcal-organizer.rb`, use `awk` to replace darwin `sha256` values based on URL context (`darwin_amd64` / `darwin_arm64`), commit and push with message `"Update macOS checksums for gcal-organizer v${VERSION} (post-signing)"` (FR-013, data-model.md Entity: Homebrew Cask lifecycle). Reference: Gaze release.yml lines 143-190

**Checkpoint**: The full two-job pipeline is now complete. All signing, checksum, and Cask update steps are in place.

---

## Phase 5: Cleanup & Migration

**Purpose**: Remove superseded files from the Formula-based distribution model and update documentation.

- [x] T023 Remove `.github/workflows/bottles.yml` — superseded by Cask-based distribution (FR-017)
- [x] T024 [P] Remove `deploy/homebrew/gcal-organizer.rb` — superseded by GoReleaser Cask generation (FR-017)
- [x] T025 [P] Update `README.md` — replace Homebrew installation instructions: change from `brew install jflowers/gcal-organizer/gcal-organizer` (Formula) to `brew install --cask jflowers/gcal-organizer/gcal-organizer` (Cask). Add note about signed macOS binaries and Gatekeeper compatibility.
- [x] T026 Run `goreleaser check` to validate final `.goreleaser.yaml` config
- [x] T027 Run `make ci` to confirm no Go build/test/vet/fmt regressions from pipeline changes
- [ ] T028 Run quickstart.md validation: push a test tag, verify both pipeline jobs complete, verify codesign/notarization/checksums/Cask per quickstart.md Steps 3-7 *(deferred to post-merge — requires tag push on main)*

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately. Secrets must be configured before any signing test.
- **Foundational (Phase 2)**: No dependency on Phase 1 for code changes, but secrets (Phase 1) needed for end-to-end testing. BLOCKS all signing work (Phases 3-4).
- **US1+US2 Signing (Phase 3)**: Depends on Phase 2 completion (GoReleaser config + release job must exist).
- **US4 Cask Checksums (Phase 4)**: Depends on Phase 3 (signing steps must exist for Cask update to have signed archives to checksum).
- **Cleanup (Phase 5)**: Depends on Phases 2-4 being complete. Can start removals (T023, T024) in parallel with documentation (T025).

### User Story Dependencies

```
US5 (Archives) ──┐
                  ├── US1+US2 (Signing + Checksums) ──── US4 (Cask Checksums)
US3 (Graceful) ──┘
```

- **US5** (P3) and **US3** (P2): Foundational — implemented in Phase 2 as part of the GoReleaser config and release job structure. No dependency on other stories.
- **US1+US2** (P1): Depends on US5 (archives must exist to sign them) and US3 (release job must output `has_signing_secrets`).
- **US4** (P2): Depends on US1 (darwin archives must be signed before Cask checksums can be updated).

### Within Each Phase

- Phase 1: T001 first (largest secret), T002-T005 in parallel, T006 last (verification).
- Phase 2: T007-T011 sequential (building up `.goreleaser.yaml` incrementally), T012 validation. Then T013-T015 sequential (building up `release.yml`).
- Phase 3: T016-T020 sequential (building up `sign-macos` job step by step), T021 after T020.
- Phase 4: T022 single task.
- Phase 5: T023-T025 in parallel (different files), T026-T028 sequential (validation).

### Parallel Opportunities

- **Phase 1**: T002-T005 can all run in parallel (independent `gh secret set` commands)
- **Phase 2**: T007-T011 (GoReleaser config) and T013-T015 (release.yml) touch different files and could run in parallel, but T013-T015 depend on the GoReleaser config existing for the workflow to function
- **Phase 5**: T023, T024, T025 can all run in parallel (different files)

---

## Parallel Example: Phase 1 (Setup)

```bash
# Set T001 first (it's the largest secret and validates gh access):
Task: "Set MACOS_SIGN_P12 secret via gh secret set"

# Then launch T002-T005 in parallel:
Task: "Set MACOS_SIGN_PASSWORD secret"
Task: "Set MACOS_NOTARY_KEY secret"
Task: "Set MACOS_NOTARY_KEY_ID secret"
Task: "Set MACOS_NOTARY_ISSUER_ID secret"
```

## Parallel Example: Phase 5 (Cleanup)

```bash
# Launch T023-T025 in parallel (different files):
Task: "Remove .github/workflows/bottles.yml"
Task: "Remove deploy/homebrew/gcal-organizer.rb"
Task: "Update README.md with Cask installation instructions"
```

---

## Implementation Strategy

### MVP First (Phase 1 + Phase 2 Only)

1. Complete Phase 1: Set GitHub secrets
2. Complete Phase 2: GoReleaser config + release job rewrite
3. **STOP and VALIDATE**: Push a test tag, verify unsigned archives are published correctly
4. This delivers US5 (standardized archives) and US3 (graceful degradation) without any signing

### Full Delivery (All Phases)

1. Complete Phase 1: Secrets → Phase 2: GoReleaser + release job → **Validate**
2. Complete Phase 3: sign-macos job (signing + checksums) → **Validate** (US1 + US2)
3. Complete Phase 4: Cask checksum update → **Validate** (US4)
4. Complete Phase 5: Cleanup + documentation → **Final validation**
5. Each phase adds value without breaking previous phases

### Single-Developer Sequential Strategy

1. T001-T006 (secrets) — 10 minutes
2. T007-T015 (GoReleaser + release job) — 30 minutes
3. Push test tag, validate unsigned release — 15 minutes
4. T016-T021 (sign-macos job) — 30 minutes
5. T022 (Cask update) — 15 minutes
6. Push test tag, validate full signed release — 20 minutes
7. T023-T028 (cleanup + docs + final validation) — 20 minutes

**Estimated total**: ~2.5 hours

---

## Notes

- All file changes are in CI/CD configuration (YAML, Ruby, Markdown) — no Go code changes
- The Gaze reference implementation at `/Users/jflowers/Projects/github/unbound-force/gaze/.github/workflows/release.yml` should be used as the primary reference for all pipeline steps
- Secret values are in `/Users/jflowers/Projects/github/unbound-force/temp/env.md`
- The signing identity is `Developer ID Application: John Flowers (PGFWLVZX55)` — same across all projects
- Commit after each completed phase, not after individual tasks (pipeline files build incrementally)
- The `HOMEBREW_TAP_GITHUB_TOKEN` secret name must match what GoReleaser expects in `.goreleaser.yaml`
