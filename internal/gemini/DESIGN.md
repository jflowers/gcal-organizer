# Package gemini — Design & Contracts

Package `gemini` provides Gemini AI integration for action item extraction
and decision extraction from meeting transcripts.

## Contractual Side Effects

### ExtractDecisions

**Sends** transcript text to the Gemini API and returns structured decisions.

| Side Effect | Classification | Description |
|-------------|---------------|-------------|
| Gemini API call | Contractual | Sends full transcript text with structured prompt via retry loop |
| Return `[]models.Decision` | Contractual | Parsed and validated decisions with category, text, timestamp, and context |
| Return `nil, nil` | Contractual | When `transcriptText` is empty (no API call made) |
| Return `error` | Contractual | On API failure or response parsing failure |

**Contract**: The prompt instructs Gemini to return a JSON array of decisions
categorized as `"made"`, `"deferred"`, or `"open"`. The response is parsed
with `parseDecisionsResponse`.

### parseDecisionsResponse (unexported)

Pure function that parses the Gemini response text into `[]models.Decision`.

| Side Effect | Classification | Description |
|-------------|---------------|-------------|
| None (pure) | N/A | Strips markdown fences, extracts JSON array via regex, unmarshals and validates |

**Validation rules** (from data model):
- Strips code fences and extracts `[...]` JSON array
- Filters out entries with empty `Text`
- Normalizes `Category` to lowercase; defaults to `"open"` if invalid
- Error messages never include raw response text (may contain confidential transcript content)

### ExtractAssigneesFromCheckboxes

**Sends** checkbox items to Gemini and returns assignee mappings.

| Side Effect | Classification | Description |
|-------------|---------------|-------------|
| Gemini API call | Contractual | Sends checkbox text with structured prompt via retry loop |
| Return `[]CheckboxAssignment` | Contractual | Matched assignments with email addresses |

### parseAssignmentsResponse (unexported)

Pure function that parses the Gemini response for checkbox assignments.

| Side Effect | Classification | Description |
|-------------|---------------|-------------|
| None (pure) | N/A | JSON parsing with validation against original checkbox items |

## Error Handling

All Gemini API calls use the `retry` package for transient error recovery.
API key validation happens at construction time in `NewClient`.

## Specification Reference

Full requirements: `specs/008-decision-extraction/spec.md` (FR-006, FR-007)
Data model: `specs/008-decision-extraction/data-model.md` (Gemini Response Schema)
