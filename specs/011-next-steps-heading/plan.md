# Implementation Plan: Support Updated Notes by Gemini Heading and Section Position

**Branch**: `011-next-steps-heading` | **Date**: 2026-04-16 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/011-next-steps-heading/spec.md`

## Summary

Google's Notes by Gemini has changed the section heading from "Suggested next steps" to "Next steps" and moved it from the end of the document to the middle. This breaks both the Go-side checkbox extraction (`internal/docs/service.go`) and the browser-side section navigation (`browser/assign-tasks.ts`). The fix requires:

1. **Multi-heading matching**: Accept both "Next steps" and "Suggested next steps" (case-insensitive, full heading match, first match wins)
2. **Section boundary enforcement**: Stop collecting checkbox items when a subsequent heading is encountered (any level H1–H6)
3. **Browser fallback navigation**: Try "Next steps" first, fall back to "Suggested next steps" in the browser find bar
4. **Graceful degradation**: Return empty results (not errors) when no matching heading is found

No new data entities, API contracts, or dependencies are required. This is a behavioral fix to two existing files and their tests.

## Technical Context

**Language/Version**: Go 1.24+ (toolchain go1.24.12)
**Primary Dependencies**: `google.golang.org/api/docs/v1` (Docs API), Playwright via `npx tsx` (browser automation)
**Storage**: N/A (no data persistence changes)
**Testing**: `go test ./...` (Go), manual browser testing (TypeScript — no automated test runner for Playwright scripts)
**Target Platform**: macOS (primary), Linux (secondary)
**Project Type**: Single CLI application
**Performance Goals**: N/A (no performance-sensitive changes)
**Constraints**: Must maintain backward compatibility with existing "Suggested next steps" documents
**Scale/Scope**: 2 source files modified, 2 test files modified/created

> **Note**: Line numbers referenced in this plan and tasks.md are based on the codebase at spec creation time (2026-04-16). They may drift if other changes modify the target files before this feature is implemented. Use the before/after code blocks in quickstart.md as the authoritative reference.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. CLI-First Architecture | ✅ PASS | No CLI changes needed; fix is internal to extraction logic |
| II. API-Key Authentication Mode | ✅ PASS | No authentication changes |
| III. Test-Driven Development | ✅ PASS | Tests will be written/updated for all behavioral changes |
| IV. Idiomatic Go | ✅ PASS | Uses standard Go patterns (slice of candidate headings, paragraph style inspection) |
| V. Graceful Error Handling | ✅ PASS | FR-006 requires empty result (not error) for missing headings; FR-007 requires warning log |
| VI. Observability | ✅ PASS | Warning log when no heading found (FR-007); browser script already logs navigation steps |
| VII. Self-Serve Diagnostics | ✅ PASS | No new user-facing errors introduced |

**Quality Gates**:
- `go build ./...` — must compile ✅
- `go test ./...` — must pass ✅
- `go vet ./...` — no warnings ✅
- `gofmt` — formatted ✅
- `go mod tidy` — clean ✅
- New/modified functions (`isHeadingParagraph`, `matchesNextStepsHeading`, `extractItemsFromSection`) — ≥90% line coverage ✅

**Gate Violations**: None.

**Known Test Gaps**:
- Browser automation (`browser/assign-tasks.ts`) has no automated test runner — changes are verified via manual browser testing. The fallback heading logic (T020) is the most complex TypeScript change without automated coverage. This is accepted because the browser script interacts with Google Docs' live DOM, making unit testing impractical.

## Project Structure

### Documentation (this feature)

```text
specs/011-next-steps-heading/
├── plan.md              # This file
├── research.md          # Phase 0 output — technical research
├── quickstart.md        # Phase 1 output — implementation guide
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (files modified)

```text
internal/docs/
├── service.go           # [MODIFY] Heading matching + section boundary logic
└── service_test.go      # [MODIFY] New test cases for multi-heading + boundary

browser/
└── assign-tasks.ts      # [MODIFY] Fallback heading search in find bar

cmd/gcal-organizer/
└── assign_tasks.go      # [MODIFY] Update help text to mention both headings
```

**Structure Decision**: No new files or packages needed. All changes are modifications to existing files in the established project structure. The `internal/docs/service.go` file already contains the `extractItemsFromSection` function that needs updating, and `browser/assign-tasks.ts` already contains the find-bar navigation logic.

## Design Decisions

### D1: Candidate Heading List vs. Regex Pattern

**Decision**: Use an ordered slice of candidate heading strings (`[]string{"Next steps", "Suggested next steps"}`) rather than a regex pattern.

**Rationale**: The spec assumes Google may continue to evolve the heading name (Assumptions section). A slice is:
- Easier to extend (add a new string to the list)
- Easier to read and test (no regex complexity)
- Consistent with the spec's requirement for "full heading match" (not substring/pattern)

**Alternatives rejected**:
- Regex (`(?i)^(suggested\s+)?next\s+steps$`): More fragile, harder to maintain, overkill for 2 candidates
- Map lookup: No ordering guarantee; spec requires "first match in document order" (FR-004)

### D2: Section Boundary Detection via ParagraphStyle.NamedStyleType

**Decision**: Detect section boundaries by checking `ParagraphStyle.NamedStyleType` for any heading style (`HEADING_1` through `HEADING_6`).

**Rationale**: Google Docs API represents headings via `NamedStyleType` on the paragraph style. This is the same mechanism used elsewhere in the codebase (e.g., `extractTranscriptContentFromDoc` checks for `HEADING_3`). Checking for any heading level satisfies FR-003's requirement to stop at "any heading level."

**Alternatives rejected**:
- Check only same-level headings: Spec explicitly says "any heading level" (FR-003, US3-AS2)
- Check for bold/large text: Spec assumes standard heading styles (Assumptions section)

### D3: Browser Fallback Order (New Heading First)

**Decision**: In `assign-tasks.ts`, search for "Next steps" first, then fall back to "Suggested next steps" if not found.

**Rationale**: FR-005 explicitly requires this order. New documents will use "Next steps"; trying it first avoids unnecessary find-bar operations on the majority of future documents.

### D4: Heading Match Strategy — Full Match vs. Substring

**Decision**: Change from `strings.Contains` (current) to exact full-heading match (case-insensitive `strings.EqualFold` after trimming).

**Rationale**: The current code uses `strings.Contains(strings.ToLower(paraText), strings.ToLower(SuggestedNextStepsHeading))`, which would match body text containing "suggested next steps" as a substring. The spec explicitly requires full heading match to avoid false positives on body text (Assumptions section, Edge Cases). Additionally, the spec requires matching only heading-styled paragraphs, not arbitrary body text (Edge Case: "What happens when the phrase 'next steps' appears in regular body text?").

**Implementation**: Check that the paragraph has a heading style (`HEADING_1` through `HEADING_6`) AND that the trimmed text matches a candidate heading name via `strings.EqualFold`.

## Complexity Tracking

No constitution violations to justify.

## Implementation Approach

### Component 1: Go-side Heading Matching + Section Boundary (`internal/docs/service.go`)

**Current behavior**:
- Single constant `SuggestedNextStepsHeading = "Suggested next steps"`
- Substring match via `strings.Contains` (no heading style check)
- No section boundary — collects all bullets after the heading to end of document

**New behavior**:
- Ordered candidate list: `["Next steps", "Suggested next steps"]`
- Full heading match: `strings.EqualFold(trimmedText, candidate)` + heading style check
- Section boundary: Stop collecting when a subsequent heading paragraph is encountered
- First match wins (FR-004): Once a matching heading is found, use it; ignore later candidates

**Key changes**:
1. Replace `SuggestedNextStepsHeading` constant with `NextStepsHeadingCandidates` variable (slice)
2. Add `isHeadingParagraph(para)` helper to check `NamedStyleType`
3. Modify `extractItemsFromSection` to:
   a. Match any candidate heading (case-insensitive, full match, heading style required)
   b. Break on next heading after entering the section

### Component 2: Browser-side Fallback Navigation (`browser/assign-tasks.ts`)

**Current behavior**:
- Hardcoded `'Suggested next steps'` in find-bar fill
- Single search attempt

**New behavior**:
- Try `'Next steps'` first
- If no match found (match count shows 0), try `'Suggested next steps'`
- If neither found, log warning and continue (browser will still attempt per-checkbox search)

### Component 3: Help Text Update (`cmd/gcal-organizer/assign_tasks.go`)

**Current behavior**: Help text says `"Suggested next steps" section`

**New behavior**: Help text says `"Next steps" (or "Suggested next steps") section`

### Component 4: Test Updates (`internal/docs/service_test.go`)

New test cases needed:
- "Next steps" heading recognized
- "Suggested next steps" still works (backward compat)
- Section boundary stops at next heading
- Section boundary with no following heading (legacy end-of-doc)
- Both headings present — first match wins
- No matching heading — empty result
- Heading text in body text (not a heading style) — not matched
- Case-insensitive matching
- Heading with extra whitespace
