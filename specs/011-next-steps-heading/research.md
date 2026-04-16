# Research: Support Updated Notes by Gemini Heading and Section Position

**Feature**: 011-next-steps-heading
**Date**: 2026-04-16

## R1: Google Docs API Heading Detection

**Question**: How does the Google Docs API represent section headings, and how can we reliably distinguish headings from body text?

**Finding**: The Google Docs API represents headings via `Paragraph.ParagraphStyle.NamedStyleType`. Valid heading styles are:
- `HEADING_1` through `HEADING_6`
- `TITLE` and `SUBTITLE` (not relevant for section headings)

The current codebase already uses this pattern in `extractTranscriptContentFromDoc` (line 250–251 of `service.go`):
```go
if elem.Paragraph.ParagraphStyle != nil &&
    elem.Paragraph.ParagraphStyle.NamedStyleType == "HEADING_3" {
```

**Decision**: Use `ParagraphStyle.NamedStyleType` to identify heading paragraphs. A paragraph is a heading if its `NamedStyleType` starts with `"HEADING_"`. This is reliable because Google's Notes by Gemini uses standard heading styles (confirmed by spec Assumptions section).

**Confidence**: High — this is the documented API mechanism and is already used in the codebase.

## R2: Current Heading Matching Behavior (Bug Analysis)

**Question**: Why does the current implementation fail with the new "Next steps" heading?

**Finding**: The current code in `extractItemsFromSection` (line 123) uses:
```go
if strings.Contains(strings.ToLower(paraText), strings.ToLower(SuggestedNextStepsHeading)) {
```

This has two problems:
1. **Wrong heading name**: `SuggestedNextStepsHeading` is `"Suggested next steps"` — it doesn't match `"Next steps"`
2. **Substring match on body text**: `strings.Contains` would match any paragraph containing "suggested next steps" as a substring, including body text. This hasn't caused issues because the phrase is uncommon in body text, but it violates the spec's requirement for heading-only matching.

Additionally, there is no section boundary enforcement — once `inSuggestedSection` is set to `true`, it stays `true` for the rest of the document. When the section was at the end of the document, this was harmless. Now that it can be in the middle, bullets from subsequent sections would be incorrectly collected.

**Decision**: Fix both issues: multi-heading matching with heading style verification, and section boundary enforcement.

## R3: Section Boundary Enforcement Strategy

**Question**: How should we detect the end of the "Next steps" section?

**Finding**: In Google Docs structure, sections are delimited by heading paragraphs. The "Next steps" section ends when:
1. Another heading paragraph is encountered (any level H1–H6), OR
2. The document ends (no more structural elements)

The Google Docs API returns structural elements in document order, so we can simply iterate and check each paragraph's `NamedStyleType` after entering the target section.

**Implementation approach**:
```go
// After entering the section, check if current paragraph is a heading
if inSection && isHeadingParagraph(para) {
    break // Section boundary reached
}
```

**Edge case**: A heading paragraph that matches a candidate heading name IS the section start, not a boundary. The boundary check must only apply to headings AFTER the section start.

**Decision**: After setting `inSection = true`, break on the next heading paragraph. The `isHeadingParagraph` helper checks for `HEADING_1` through `HEADING_6`.

## R4: Browser Find-Bar Fallback Strategy

**Question**: How should the browser automation handle the heading name change?

**Finding**: The current `assign-tasks.ts` navigates to the section by:
1. Opening the find bar (`Cmd+F`)
2. Typing `"Suggested next steps"`
3. Pressing Enter to jump to the match
4. Closing the find bar

The find bar shows match count (e.g., "1 of 1" or "0 of 0"). We can use this to detect whether the heading was found.

**Implementation approach**:
1. Try `"Next steps"` first
2. Read the match count from `.docs-findinput-count`
3. If match count is 0, clear the input and try `"Suggested next steps"`
4. If still 0, log a warning and close the find bar

**Risk**: The find bar searches ALL text in the document, not just headings. "Next steps" could match body text. However, this is acceptable because:
- The find-bar navigation is just for scrolling to the approximate location
- The actual checkbox finding uses a separate per-item search (Phase 1 of `processAssignment`)
- False positives in navigation are low-impact (worst case: scrolls to wrong location, but per-item search still works)

**Decision**: Implement fallback with "Next steps" first, then "Suggested next steps". Accept that find-bar may match body text — this is a navigation aid, not a precision tool.

## R5: Heading Match Candidates — Ordering and Extensibility

**Question**: What order should candidate headings be checked, and how should the list be structured for future extensibility?

**Finding**: The spec says:
- FR-004: "Use the first matching heading encountered in document order when multiple candidate headings exist"
- FR-005: "Browser automation MUST attempt to locate 'Next steps' first, then fall back to 'Suggested next steps'"
- Assumptions: "Google may continue to evolve the heading name in the future; the design should accommodate adding new candidate heading names easily"

**Decision**: Use a package-level variable (not constant, since Go doesn't support constant slices):
```go
// NextStepsHeadingCandidates lists accepted heading names in priority order.
// The first match in document order wins (FR-004).
// Add new candidate names here as Google evolves the heading format.
var NextStepsHeadingCandidates = []string{
    "Next steps",
    "Suggested next steps",
}
```

For the Go-side extraction, candidates are checked against each paragraph as it's encountered. The first paragraph that matches ANY candidate (in document order) starts the section. This naturally satisfies FR-004.

For the browser-side, the candidates are tried in the listed order (FR-005: "Next steps" first).

## R6: Case-Insensitive Full Heading Match

**Question**: How should heading text comparison work?

**Finding**: The spec requires:
- Case-insensitive matching (FR-001, FR-002)
- Full heading match, not substring (Assumptions, Edge Cases)
- Ignore formatting (bold, italic) — match on plain text content (Edge Cases)

The current `extractParagraphText` function already strips formatting and returns plain text. We need to:
1. Trim whitespace from the extracted text
2. Compare using `strings.EqualFold` (case-insensitive equality)

**Decision**: Use `strings.EqualFold(strings.TrimSpace(paraText), candidate)` for heading matching. This handles:
- Case insensitivity: "NEXT STEPS", "next steps", "Next Steps" all match
- Whitespace: Leading/trailing whitespace is trimmed
- Formatting: Already handled by `extractParagraphText`

## R7: Backward Compatibility Verification

**Question**: Will the changes break any existing behavior?

**Finding**: The changes are additive:
1. **New heading name**: "Next steps" is added; "Suggested next steps" continues to work
2. **Section boundary**: Previously, all bullets after the heading were collected. Now, only bullets before the next heading are collected. For documents where the section is at the end (legacy behavior), there IS no next heading, so all remaining bullets are still collected — identical behavior.
3. **Heading style check**: The new code requires paragraphs to have a heading style. The old code used substring matching on any paragraph. This is a tightening of the match criteria. If a document has "Suggested next steps" as body text (not a heading), the old code would have matched it; the new code won't. This is the correct behavior per the spec.

**Risk**: If any existing document has "Suggested next steps" as plain text (not a heading style), the new code won't match it. This is extremely unlikely because Google's Notes by Gemini always uses heading styles for section headers (spec Assumptions).

**Decision**: The backward compatibility risk is negligible. The tighter matching is a correctness improvement.
