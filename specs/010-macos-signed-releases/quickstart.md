# Quickstart: macOS Signed Releases

**Feature**: 010-macos-signed-releases
**Date**: 2026-03-08

## Prerequisites

- GitHub repository `jflowers/gcal-organizer` with push access
- Apple Developer Program membership (for Developer ID Application certificate)
- App Store Connect API key (for notarization)
- GitHub personal access token with repo scope for `jflowers/homebrew-gcal-organizer` (stored as `HOMEBREW_TAP_GITHUB_TOKEN`)

## Step 1: Configure GitHub Secrets

Set the five signing secrets on the `jflowers/gcal-organizer` repository:

```bash
# Certificate (base64-encoded .p12)
gh secret set MACOS_SIGN_P12 --repo jflowers/gcal-organizer < <(base64 -i /path/to/DeveloperIDApplication.p12 | tr -d '\n')

# Certificate password
gh secret set MACOS_SIGN_PASSWORD --repo jflowers/gcal-organizer

# Notary API key (base64-encoded .p8)
gh secret set MACOS_NOTARY_KEY --repo jflowers/gcal-organizer < <(base64 -i /path/to/AuthKey_XXXXXXXXXX.p8 | tr -d '\n')

# Notary key ID
gh secret set MACOS_NOTARY_KEY_ID --repo jflowers/gcal-organizer

# Notary issuer ID
gh secret set MACOS_NOTARY_ISSUER_ID --repo jflowers/gcal-organizer
```

Also ensure `HOMEBREW_TAP_GITHUB_TOKEN` is set (may already exist from current release pipeline).

## Step 2: Verify GoReleaser Configuration Locally

```bash
# Validate GoReleaser config syntax
goreleaser check

# Dry-run build (builds binaries but doesn't publish)
goreleaser release --snapshot --clean

# Inspect generated archives
ls dist/*.tar.gz
tar tzf dist/gcal-organizer_*_darwin_arm64.tar.gz
# Should show: gcal-organizer, man/gcal-organizer.1, LICENSE
```

## Step 3: Test with a Pre-release Tag

```bash
# Create a test tag (won't trigger Homebrew Cask update if skip_upload: auto)
git tag v0.0.0-test.1
git push origin v0.0.0-test.1
```

Monitor the release workflow:
```bash
gh run watch --repo jflowers/gcal-organizer
```

Expected behavior:
1. `release` job builds archives, publishes GitHub Release, outputs `has_signing_secrets=true`
2. `sign-macos` job signs darwin binaries, notarizes, replaces assets, updates checksums
3. Homebrew Cask update is skipped for pre-release tags (`skip_upload: auto`)

## Step 4: Verify Signed Binaries

After the `sign-macos` job completes, download and verify:

```bash
# Download a signed darwin archive
gh release download v0.0.0-test.1 --repo jflowers/gcal-organizer --pattern "*darwin_arm64*" --dir /tmp/verify

# Extract and verify code signature
cd /tmp/verify
tar xzf gcal-organizer_*_darwin_arm64.tar.gz
codesign -dv --verbose=4 gcal-organizer
# Should show:
#   Authority=Developer ID Application: John Flowers (PGFWLVZX55)
#   Timestamp=...
#   Runtime Version=...

# Verify Gatekeeper assessment
spctl --assess --type execute -v gcal-organizer
# Should show: gcal-organizer: accepted
#   source=Notarized Developer ID
```

## Step 5: Verify Checksums

```bash
# Download checksums
gh release download v0.0.0-test.1 --repo jflowers/gcal-organizer --pattern "checksums.txt" --dir /tmp/verify

# Verify all checksums match
cd /tmp/verify
shasum -a 256 -c checksums.txt
# All files should show: OK
```

## Step 6: Verify Graceful Degradation

To test that the pipeline works without signing secrets:

1. Fork `jflowers/gcal-organizer` to a test account (no signing secrets configured)
2. Push a test tag on the fork
3. Verify the `release` job completes successfully
4. Verify the `sign-macos` job is skipped (not failed)

## Step 7: Production Release

```bash
# Create a real release tag
git tag v1.2.0
git push origin v1.2.0
```

After the full pipeline completes:

```bash
# Verify Homebrew Cask installation
brew tap jflowers/gcal-organizer
brew install --cask jflowers/gcal-organizer/gcal-organizer

# Verify installed binary
gcal-organizer --version
# Should show: v1.2.0

# Verify binary is signed (on macOS)
codesign -dv --verbose=4 $(which gcal-organizer)
```

## Troubleshooting

### Notarization fails with "Invalid" status

Check the notarization log:
```bash
xcrun notarytool log <submission-id> \
  --key /path/to/AuthKey.p8 \
  --key-id <key-id> \
  --issuer <issuer-id>
```

Common causes:
- Certificate expired → Export a new P12 and update `MACOS_SIGN_P12`
- Missing hardened runtime → Ensure `--options runtime` flag is set
- Missing secure timestamp → Ensure `--timestamp` flag is set

### Checksums don't match after signing

The sign-macos job should automatically recompute darwin checksums. If they still don't match:
1. Check that `gh release upload --clobber` succeeded (look for upload errors in the job log)
2. Verify the checksum update step ran after the asset replacement step
3. Re-run the sign-macos job: `gh run rerun <run-id> --job sign-macos`

### Homebrew Cask shows wrong checksums

The Cask checksums are updated in a separate step after signing. If they're wrong:
1. Check the "Update Homebrew cask checksums" step in the sign-macos job log
2. Manually verify: `shasum -a 256 gcal-organizer_*_darwin_*.tar.gz`
3. If needed, manually update `Casks/gcal-organizer.rb` in the tap repo

### sign-macos job is skipped unexpectedly

Verify that `MACOS_SIGN_P12` secret is set:
```bash
gh secret list --repo jflowers/gcal-organizer
```
The check-secrets step in the `release` job should output `has_secrets=true`. Check the job output for this step.
