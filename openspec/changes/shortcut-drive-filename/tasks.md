## 1. Change Title Resolution Logic

- [x] 1.1 In `internal/organizer/organizer.go` `SyncCalendarAttachments()`, add a `fileNameCache map[string]string` alongside the existing `ownershipCache` at the top of the function
- [x] 1.2 Replace the attachment title resolution block (lines 439-448) to always call `GetFileName()` first, check the cache before making the API call, and fall back to `att.Title` on error
- [x] 1.3 Use the cached/resolved file name for `CreateShortcut()`, `logCalendarAction()`, notes tracking, and decision doc tracking — ensuring all downstream consumers see the Drive file name

## 2. Tests

- [x] 2.1 Add test `TestSyncCalendarAttachments_UsesGetFileNameForShortcut` in `internal/organizer/organizer_test.go` verifying that `GetFileName()` is called and its result is passed to `CreateShortcut()` when `att.Title` is a generic name like "Notes by Gemini"
- [x] 2.2 Add test `TestSyncCalendarAttachments_FallsBackToAttTitle` verifying that when `GetFileName()` returns an error, `att.Title` is used as the shortcut name
- [x] 2.3 Add test `TestSyncCalendarAttachments_CachesFileNameLookups` verifying that for two events with the same attachment file ID, `GetFileName()` is called only once

## 3. Verification

- [x] 3.1 Run `go build ./cmd/gcal-organizer` to verify compilation
- [x] 3.2 Run `go test -race -count=1 ./...` to verify all tests pass
- [x] 3.3 Run `go vet ./...` and `gofmt -l .` to verify code quality
<!-- spec-review: passed -->
<!-- code-review: passed -->
