# Tasks: Secure Credential Storage

**Input**: Design documents from `/specs/007-secure-credential-storage/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, quickstart.md

**Tests**: Included — Constitution requires TDD (Principle III).

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup

**Purpose**: Add the go-keyring dependency and create the `internal/secrets/` package skeleton

- [x] T001 Add `github.com/zalando/go-keyring` v0.2.6 dependency via `go get github.com/zalando/go-keyring@v0.2.6` and promote `github.com/mattn/go-isatty` from indirect to direct dependency via `go get github.com/mattn/go-isatty`
- [x] T002 Create `internal/secrets/` package directory with empty files: `store.go`, `keychain.go`, `file.go`, `migrate.go`, `store_test.go`

---

## Phase 2: Foundational (SecretStore Interface + Backends)

**Purpose**: Core `SecretStore` abstraction that ALL user stories depend on. Maps to plan.md Phase 1 (FR-001, FR-006, FR-007, FR-010).

**CRITICAL**: No user story work can begin until this phase is complete.

### Tests

- [x] T003 [P] Write `TestKeychainStore_SetGetDelete` in `internal/secrets/store_test.go` — table-driven test using `keyring.MockInit()` to verify round-trip Set/Get/Delete for all three key constants (`KeyOAuthToken`, `KeyGeminiAPIKey`, `KeyClientCredentials`). Verify `ErrNotFound` on missing keys.
- [x] T004 [P] Write `TestFileStore_SetGetDelete` in `internal/secrets/store_test.go` — table-driven test using `t.TempDir()` as configDir. For `KeyOAuthToken`: verify JSON token file read/write/delete. For `KeyGeminiAPIKey`: verify `.env` line read/write/delete (preserving other lines). For `KeyClientCredentials`: verify `credentials.json` file read/write/delete. Verify `ErrNotFound` on missing keys.
- [x] T005 [P] Write `TestNewStore_FallbackOnNoKeyring` and `TestNewStore_FallbackOnUnavailable` in `internal/secrets/store_test.go` — verify factory returns `(FileStore, BackendFile)` when `noKeyring=true`, and returns `(FileStore, BackendFile)` with `MockInitWithError` simulating unavailable keyring.

### Implementation

- [x] T006 Implement `SecretStore` interface, `Backend` type, key constants (`KeyOAuthToken`, `KeyGeminiAPIKey`, `KeyClientCredentials`), `ServiceName` constant, `ErrNotFound` sentinel, and `NewStore(noKeyring bool) (SecretStore, Backend)` factory function in `internal/secrets/store.go` — per data-model.md. Factory probes keychain availability via sentinel key `__gcal_organizer_probe__` write/read/delete cycle (research.md R2). Uses `internal/logging.Logger` for backend selection (Info) and fallback warnings (Warn) — no verbose parameter needed.
- [x] T007 Implement `KeychainStore` struct with `Get`, `Set`, `Delete` methods in `internal/secrets/keychain.go` — delegates to `keyring.Get/Set/Delete(ServiceName, key)`. Map `keyring.ErrNotFound` to `secrets.ErrNotFound`. Map `keyring.ErrSetDataTooBig` to a wrapped error. `Delete` ignores `ErrNotFound`.
- [x] T008 Implement `FileStore` struct with `configDir` field and `Get`, `Set`, `Delete` methods in `internal/secrets/file.go` — maps keys to file operations per data-model.md table: `KeyOAuthToken` → `token.json` (JSON read/write, 0600 perms), `KeyGeminiAPIKey` → `.env` line parsing (read `GEMINI_API_KEY=` value / write-update line / delete line preserving other content), `KeyClientCredentials` → `credentials.json` (raw file read/write, 0600 perms). `Get` returns `ErrNotFound` when file or entry is absent.
- [x] T009 Run `go test ./internal/secrets/...` and verify T003, T004, T005 pass. Run `go vet ./internal/secrets/...` and `gofmt -l internal/secrets/`.

**Checkpoint**: SecretStore interface and both backends are functional. All foundational tests pass.

---

## Phase 3: User Story 1 — Secure Token Storage (Priority: P1) MVP

**Goal**: OAuth tokens stored in OS credential store; refreshed tokens persisted (not lost on exit).

**Independent Test**: Run `auth login`, verify token is in keychain (via `secrets.Get(KeyOAuthToken)`), verify `token.json` does not exist on disk. Run a workflow, force token refresh, verify refreshed token is persisted.

### Tests

- [ ] T010 [P] [US1] Write `TestOAuthClient_LoadFromStore` in `internal/auth/oauth_test.go` — create a mock `SecretStore` (or use `keyring.MockInit()` + `KeychainStore`), pre-populate a JSON-serialized `oauth2.Token`, verify `loadToken()` retrieves and deserializes it correctly.
- [ ] T011 [P] [US1] Write `TestOAuthClient_SaveToStore` in `internal/auth/oauth_test.go` — verify `saveToken()` JSON-serializes and stores via `store.Set(KeyOAuthToken, ...)`.
- [ ] T012 [P] [US1] Write `TestPersistingTokenSource` in `internal/auth/oauth_test.go` — create a mock `TokenSource` that returns a different token on second call (simulating refresh). Verify `persistingTokenSource.Token()` calls `store.Set` only when the token changes, and does not call `store.Set` when the token is unchanged.

### Implementation

- [ ] T013 [US1] Refactor `OAuthClient` in `internal/auth/oauth.go`: replace `tokenFile string` field with `store secrets.SecretStore` field. Add `credsFallbackPath string` field. Update `NewOAuthClient` signature to `NewOAuthClient(store secrets.SecretStore, credsFallbackPath string) (*OAuthClient, error)` — load client credentials from `store.Get(KeyClientCredentials)` first, fall back to reading `credsFallbackPath` file. Parse credentials via `google.ConfigFromJSON(bytes, Scopes...)`. If neither source has credentials, return an actionable error referencing `auth login` and `doctor` (Constitution Principle VII). Also remove `ValidateForWorkflow()` from `internal/config/config.go` (dead code — zero callers; its `os.Stat` check would fail after keychain migration).
- [ ] T014 [US1] Rewrite `loadToken()` in `internal/auth/oauth.go` to call `o.store.Get(secrets.KeyOAuthToken)` and JSON-unmarshal the result. Rewrite `saveToken()` to JSON-marshal and call `o.store.Set(secrets.KeyOAuthToken, ...)`.
- [ ] T015 [US1] Implement `persistingTokenSource` struct in `internal/auth/oauth.go` — fields: `base oauth2.TokenSource`, `store secrets.SecretStore`, `current *oauth2.Token`, `mu sync.Mutex`. `Token()` method: call `base.Token()`, compare `AccessToken` and `Expiry` with `current`, if different call `saveToken` equivalent via `store.Set`, update `current`, return token. Per research.md R3.
- [ ] T016 [US1] Update `GetClient()` in `internal/auth/oauth.go` to wrap the `oauth2.Config.TokenSource(ctx, tok)` with `persistingTokenSource` before creating `oauth2.NewClient(ctx, persistingTS)`.
- [ ] T017 [US1] Update all callers of `NewOAuthClient` in `cmd/gcal-organizer/` — pass the `SecretStore` instance and `cfg.CredentialsFile` as fallback path. Update `auth_config.go` (`auth login`, `auth status`) and any command `RunE` functions that create an `OAuthClient`.
- [ ] T018 [US1] Run `go test ./internal/auth/... ./internal/secrets/...` and verify T010-T012 pass. Run `go build ./...` to verify no compilation errors across the project.

**Checkpoint**: OAuth tokens are stored in and loaded from the SecretStore. Token refresh is persisted. `auth login` and `auth status` work with the new backend.

---

## Phase 4: User Story 2 — Secure API Key Storage (Priority: P2)

**Goal**: Gemini API key loaded from credential store first, falling back to environment variables.

**Independent Test**: Store an API key via `store.Set(KeyGeminiAPIKey, ...)`, run `config.Load()`, verify the key is retrieved from the store. Remove from store, set env var, verify fallback works.

### Tests

- [ ] T019 [P] [US2] Write `TestLoadSecrets_KeychainAPIKey` in `internal/config/config_test.go` — use `keyring.MockInit()`, pre-populate `KeyGeminiAPIKey` in the mock keychain, call `cfg.LoadSecrets(store)`, verify `cfg.GeminiAPIKey` is the keychain value (not the env var value when both are set).
- [ ] T020 [P] [US2] Write `TestLoadSecrets_APIKeyFallback` in `internal/config/config_test.go` — use `keyring.MockInit()` with no API key stored, set `GEMINI_API_KEY` env var, call `cfg.LoadSecrets(store)`, verify `cfg.GeminiAPIKey` falls back to the env var value from `Load()`.

### Implementation

- [ ] T021 [US2] Add `LoadSecrets(store secrets.SecretStore) error` method to `*Config` in `internal/config/config.go`. Check `store.Get(secrets.KeyGeminiAPIKey)` — if found, override `c.GeminiAPIKey` with the keychain value. If `ErrNotFound`, keep the existing value from `Load()` (env var / viper). `Load()` signature remains unchanged.
- [ ] T022 [US2] Update `init` command in `cmd/gcal-organizer/selfservice.go` to store the user-provided API key in the `SecretStore` via `store.Set(KeyGeminiAPIKey, apiKey)` when keychain is available, instead of (or in addition to) writing it to `.env`.
- [ ] T023 [US2] Update all callers of `config.Load()` in `cmd/gcal-organizer/` to add `cfg.LoadSecrets(store)` after `config.Load()` and `secrets.NewStore()`. The `Load()` call itself is unchanged. Startup flow: `cfg, _ := config.Load()` → `store, backend := secrets.NewStore(cfg.NoKeyring)` → `cfg.LoadSecrets(store)`. Ensure `config show` still displays the masked API key correctly regardless of source.
- [ ] T024 [US2] Run `go test ./internal/config/... ./internal/secrets/...` and verify T019-T020 pass. Run `go build ./...`.

**Checkpoint**: API key is loaded from keychain first, env var second. `init` stores keys in keychain. Config commands work correctly.

---

## Phase 5: User Story 3 — Secure Client Credentials Storage (Priority: P2)

**Goal**: `credentials.json` contents stored in credential store; file on disk becomes optional fallback.

**Independent Test**: Store credentials blob via `store.Set(KeyClientCredentials, ...)`, remove `credentials.json` from disk, verify `auth login` and `auth status` still work by reading from the store.

### Implementation

- [ ] T025 [US3] Verify `NewOAuthClient` in `internal/auth/oauth.go` (already implemented in T013) correctly reads `KeyClientCredentials` from store first, falls back to file. Add a test `TestOAuthClient_LoadCredentialsFromStore` in `internal/auth/oauth_test.go` — pre-populate `KeyClientCredentials` in mock store with a valid `credentials.json` blob, verify `NewOAuthClient` succeeds without any file on disk.
- [ ] T026 [US3] Update `auth login` in `cmd/gcal-organizer/auth_config.go` — after successfully reading `credentials.json` for the login flow, call `store.Set(KeyClientCredentials, fileContents)` to persist the blob in the credential store.
- [ ] T027 [US3] Update `auth status` in `cmd/gcal-organizer/auth_config.go` — check for client credentials in store first (`store.Get(KeyClientCredentials)`), then file. Report presence/absence accordingly.
- [ ] T028 [US3] Run `go test ./internal/auth/...` and verify T025 passes. Run `go build ./...`.

**Checkpoint**: Client credentials are stored in keychain during login. Auth commands work with credentials from either store or file.

---

## Phase 6: User Story 4 — Migration from Plaintext (Priority: P2)

**Goal**: Existing plaintext secrets auto-migrate to credential store on first run. Idempotent. Prompts before deleting `credentials.json` (interactive only).

**Independent Test**: Place `token.json`, `.env` (with `GEMINI_API_KEY` and `GOOGLE_CREDENTIALS_FILE`), and `credentials.json` on disk. Run any command. Verify: token in keychain + `token.json` deleted; API key in keychain + `.env` lines stripped; credentials in keychain + `credentials.json` still on disk (non-interactive) or prompted (interactive). Run again — verify no-op.

### Tests

- [ ] T029 [P] [US4] Write `TestMigrate_TokenFromDisk` in `internal/secrets/store_test.go` — create `token.json` in temp dir, call `Migrate()`, verify token is in store and file is deleted.
- [ ] T030 [P] [US4] Write `TestMigrate_APIKeyFromEnv` in `internal/secrets/store_test.go` — create `.env` with `GEMINI_API_KEY=test123`, `GOOGLE_CREDENTIALS_FILE=/path`, and `GCAL_MASTER_FOLDER_NAME=Notes`. Call `Migrate()`. Verify API key in store. Verify `.env` retains `GCAL_MASTER_FOLDER_NAME` line but `GEMINI_API_KEY` and `GOOGLE_CREDENTIALS_FILE` lines are removed.
- [ ] T031 [P] [US4] Write `TestMigrate_CredentialsNonInteractive` in `internal/secrets/store_test.go` — create `credentials.json` in temp dir, call `Migrate(interactive=false)`, verify credentials in store AND file still on disk (not deleted).
- [ ] T032 [P] [US4] Write `TestMigrate_Idempotent` in `internal/secrets/store_test.go` — run `Migrate()` twice, verify second run is no-op (no errors, no duplicate writes).

### Implementation

- [ ] T033 [US4] Implement `Migrate` function in `internal/secrets/migrate.go` per data-model.md. Parameters: `store SecretStore, configDir string, interactive bool, verbose bool`. Logic per secret: check `store.Get()` → if `ErrNotFound`, check disk → if on disk, `store.Set()` → cleanup. For `token.json`: delete file. For `.env`: parse line-by-line, remove `GEMINI_API_KEY=` and `GOOGLE_CREDENTIALS_FILE=` lines, write back atomically (temp file + rename, per research.md R6). For `credentials.json`: if `interactive`, prompt via `huh.NewConfirm()`; if not interactive, log warning and skip deletion.
- [ ] T034 [US4] Wire `Migrate()` into CLI startup in `cmd/gcal-organizer/main.go` — call after `NewStore()` and before command execution. Pass `isatty.IsTerminal(os.Stdin.Fd())` for the `interactive` parameter and `cfg.Verbose` for `verbose`. Only call `Migrate` when backend is `BackendKeychain` (skip for `BackendFile` since migration to file-based storage is pointless).
- [ ] T035 [US4] Run `go test ./internal/secrets/...` and verify T029-T032 pass. Run `go build ./...`.

**Checkpoint**: Migration works for all three secret types. Idempotent. Non-interactive mode skips `credentials.json` deletion.

---

## Phase 7: User Story 5 — Fallback for Headless/No-Credential-Store Environments (Priority: P3)

**Goal**: `--no-keyring` flag and `GCAL_NO_KEYRING` env var force file-based storage. Graceful fallback when keyring unavailable.

**Independent Test**: Run `gcal-organizer run --no-keyring` — verify file-based storage used, no keychain interaction. Simulate unavailable keyring (via `MockInitWithError`) — verify fallback with warning.

### Tests

- [ ] T036 [P] [US5] Write `TestConfigLoad_NoKeyring` in `internal/config/config_test.go` — set `GCAL_NO_KEYRING=true`, verify `cfg.NoKeyring` is `true` after `config.Load()`.
- [ ] T037 [P] [US5] Write `TestNewStore_NoKeyringFromConfig` in `internal/secrets/store_test.go` — verify that when `cfg.NoKeyring` is `true` (set via env var or flag), `NewStore(cfg.NoKeyring)` returns `(FileStore, BackendFile)` and that `cfg.LoadSecrets(store)` correctly falls back to env var for the API key (end-to-end config→store flow, distinct from T005 which tests the factory in isolation).

### Implementation

- [ ] T038 [US5] Add `NoKeyring bool` field to `Config` struct in `internal/config/config.go`. Add viper binding: `viper.BindEnv("no_keyring", "GCAL_NO_KEYRING")`. Load via `cfg.NoKeyring = viper.GetBool("no-keyring")`.
- [ ] T039 [US5] Add `--no-keyring` persistent flag on `rootCmd` in `cmd/gcal-organizer/main.go` — `rootCmd.PersistentFlags().Bool("no-keyring", false, "Disable OS credential store; use file-based storage")`. Bind to viper: `viper.BindPFlag("no-keyring", rootCmd.PersistentFlags().Lookup("no-keyring"))`.
- [ ] T040 [US5] Run `go test ./internal/config/... ./internal/secrets/...` and verify T036-T037 pass. Run `go build ./...`.

**Checkpoint**: `--no-keyring` and `GCAL_NO_KEYRING` force file-based storage. Unavailable keyring falls back gracefully.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Doctor reporting, documentation, and final validation.

- [ ] T041 [P] Update `doctor` command in `cmd/gcal-organizer/selfservice.go` — add a new check reporting secret storage backend: "Secrets stored in OS keychain" (pass) or "Secrets stored in plaintext files" (warn with fix suggestion). When `--verbose`, report per-secret status (`oauth-token: present/absent`, `gemini-api-key: present/absent`, `credentials-json: present/absent`). Update the `token.json` check to handle keychain-based storage (token may not be a file anymore).
- [ ] T042 [P] Update `config show` in `cmd/gcal-organizer/auth_config.go` — replace "Token File: ..." line with "Secret storage: OS keychain" or "Secret storage: plaintext files" depending on active backend. Keep masked API key display.
- [ ] T043 [P] Update `auth status` in `cmd/gcal-organizer/auth_config.go` — report storage backend alongside authentication status.
- [ ] T044 [P] Update `README.md` — add "Secure Credential Storage" section documenting keychain behavior, `--no-keyring` flag, `GCAL_NO_KEYRING` env var, migration behavior, and behavior comparison table (from quickstart.md).
- [ ] T045 [P] Update `docs/SETUP.md` — update credential setup instructions to mention keychain storage as default, add guidance for headless environments and `--no-keyring` opt-out.
- [ ] T046 [P] Update `man/gcal-organizer.1` — add `--no-keyring` flag description, update credential storage documentation.
- [ ] T047 Run `go test ./...` to verify all tests pass across the entire project. Run `go vet ./...` and `gofmt -l .` to verify no lint issues. Run `go mod tidy` to ensure clean go.mod/go.sum.
- [ ] T048 Run `make ci` to verify all CI checks pass locally.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 — BLOCKS all user stories
- **US1 (Phase 3)**: Depends on Phase 2 — MVP delivery
- **US2 (Phase 4)**: Depends on Phase 2. Can run in parallel with US1 (different files), but logically follows US1 since config.Load changes need the store wired in.
- **US3 (Phase 5)**: Depends on Phase 2 and T013 (OAuthClient refactor from US1). Mostly verification of work already done in US1.
- **US4 (Phase 6)**: Depends on Phase 2. Independent of US1-US3 (migrate.go is a new file). Can run in parallel.
- **US5 (Phase 7)**: Depends on Phase 2 (needs `NewStore` factory). Independent of US1-US4. Can run in parallel.
- **Polish (Phase 8)**: Depends on all user stories being complete

### User Story Dependencies

- **US1 (P1)**: After Phase 2. No dependencies on other stories. **MVP target.**
- **US2 (P2)**: After Phase 2. Needs store wired into CLI (done in US1 T017), so best after US1.
- **US3 (P2)**: After Phase 2. Leverages OAuthClient refactor from US1 T013. Best after US1.
- **US4 (P2)**: After Phase 2. Independent new file (migrate.go). Can parallelize with US1.
- **US5 (P3)**: After Phase 2. Independent (config + flag). Can parallelize with US1.

### Within Each User Story

- Tests MUST be written and FAIL before implementation
- Interface/model before service logic
- Core implementation before CLI wiring
- Run tests after implementation to verify pass

### Parallel Opportunities

- T003, T004, T005 can run in parallel (different test functions, same file but no conflicts)
- T010, T011, T012 can run in parallel (different test functions)
- T019, T020 can run in parallel
- T029, T030, T031, T032 can run in parallel
- T036, T037 can run in parallel
- T041, T042, T043, T044, T045, T046 can all run in parallel (different files)
- US4 and US5 can run in parallel with each other (and with US1 if team capacity allows)

---

## Parallel Example: User Story 1

```bash
# Launch all tests for US1 together:
Task: "T010 [US1] TestOAuthClient_LoadFromStore in internal/auth/oauth_test.go"
Task: "T011 [US1] TestOAuthClient_SaveToStore in internal/auth/oauth_test.go"
Task: "T012 [US1] TestPersistingTokenSource in internal/auth/oauth_test.go"

# Then implement sequentially (same file, dependent changes):
Task: "T013 [US1] Refactor OAuthClient in internal/auth/oauth.go"
Task: "T014 [US1] Rewrite loadToken/saveToken in internal/auth/oauth.go"
Task: "T015 [US1] Implement persistingTokenSource in internal/auth/oauth.go"
Task: "T016 [US1] Update GetClient() in internal/auth/oauth.go"
Task: "T017 [US1] Update callers in cmd/gcal-organizer/"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001-T002)
2. Complete Phase 2: Foundational (T003-T009)
3. Complete Phase 3: User Story 1 (T010-T018)
4. **STOP and VALIDATE**: `go test ./...`, `auth login`, verify token in keychain
5. Deploy/demo if ready — OAuth tokens are secure

### Incremental Delivery

1. Setup + Foundational → SecretStore ready
2. Add US1 → Test independently → OAuth tokens secure (MVP!)
3. Add US2 → Test independently → API key secure
4. Add US3 → Test independently → Client credentials secure
5. Add US4 → Test independently → Existing users migrated
6. Add US5 → Test independently → Headless environments work
7. Polish → Doctor, docs, final validation
8. Each story adds value without breaking previous stories

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story is independently completable and testable
- Tests use `keyring.MockInit()` — no real keychain needed in CI
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Constitution requires TDD: write tests first, verify they fail, then implement
