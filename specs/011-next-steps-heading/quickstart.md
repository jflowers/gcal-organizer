# Quickstart: Support Updated Notes by Gemini Heading and Section Position

**Feature**: 011-next-steps-heading
**Date**: 2026-04-16

This guide provides step-by-step implementation instructions for each component.

---

## 1. Update Heading Constants and Add Helper (`internal/docs/service.go`)

### 1a. Replace the single constant with a candidate list

**Before** (lines 69–70):
```go
// SuggestedNextStepsHeading is the section heading to look for.
const SuggestedNextStepsHeading = "Suggested next steps"
```

**After**:
```go
// NextStepsHeadingCandidates lists accepted section heading names for checkbox extraction.
// The first match in document order wins (FR-004).
// Add new candidate names here as Google evolves the heading format.
var NextStepsHeadingCandidates = []string{
	"Next steps",
	"Suggested next steps",
}
```

> **Note**: Keep the old `SuggestedNextStepsHeading` constant temporarily if other code references it, or remove it and update all references. Check with `grep -r SuggestedNextStepsHeading`.

### 1b. Add `isHeadingParagraph` helper function

Add this helper near the `extractParagraphText` function:

```go
// isHeadingParagraph returns true if the paragraph has a heading style (H1–H6).
// Google Docs API represents headings via ParagraphStyle.NamedStyleType.
func isHeadingParagraph(para *docs.Paragraph) bool {
	if para.ParagraphStyle == nil {
		return false
	}
	return strings.HasPrefix(para.ParagraphStyle.NamedStyleType, "HEADING_")
}
```

### 1c. Add `matchesNextStepsHeading` helper function

```go
// matchesNextStepsHeading returns true if the paragraph is a heading whose text
// matches one of the NextStepsHeadingCandidates (case-insensitive, full match).
// Requires the paragraph to have a heading style to avoid matching body text.
func matchesNextStepsHeading(para *docs.Paragraph) bool {
	if !isHeadingParagraph(para) {
		return false
	}
	text := strings.TrimSpace(extractParagraphText(para))
	for _, candidate := range NextStepsHeadingCandidates {
		if strings.EqualFold(text, candidate) {
			return true
		}
	}
	return false
}
```

### 1d. Update `extractItemsFromSection` with multi-heading matching and section boundary

**Before** (lines 110–156):
```go
func (s *Service) extractItemsFromSection(content []*docs.StructuralElement) ([]*CheckboxItem, error) {
	var items []*CheckboxItem
	inSuggestedSection := false

	for _, elem := range content {
		if elem.Paragraph == nil {
			continue
		}

		para := elem.Paragraph

		// Check if this is the "Suggested next steps" heading
		paraText := extractParagraphText(para)
		if strings.Contains(strings.ToLower(paraText), strings.ToLower(SuggestedNextStepsHeading)) {
			inSuggestedSection = true
			continue
		}

		// If we haven't found the section yet, skip
		if !inSuggestedSection {
			continue
		}

		// Check if this is a list item (bullet or checkbox)
		if para.Bullet == nil {
			continue
		}
		// ... rest of function
	}
	return items, nil
}
```

**After**:
```go
func (s *Service) extractItemsFromSection(content []*docs.StructuralElement) ([]*CheckboxItem, error) {
	var items []*CheckboxItem
	inSection := false

	for _, elem := range content {
		if elem.Paragraph == nil {
			continue
		}

		para := elem.Paragraph

		// Check if this paragraph is the target section heading (FR-001, FR-002, FR-004)
		if !inSection {
			if matchesNextStepsHeading(para) {
				inSection = true
			}
			continue
		}

		// Section boundary: stop at the next heading (FR-003)
		if isHeadingParagraph(para) {
			break
		}

		// Check if this is a list item (bullet or checkbox)
		if para.Bullet == nil {
			continue
		}

		paraText := extractParagraphText(para)
		itemText := strings.TrimSpace(paraText)
		if itemText == "" {
			continue
		}

		// Check if already processed
		isProcessed := strings.Contains(itemText, ProcessedEmoji)

		items = append(items, &CheckboxItem{
			Text:        itemText,
			StartIndex:  elem.StartIndex,
			EndIndex:    elem.EndIndex,
			IsChecked:   false,
			IsProcessed: isProcessed,
		})
	}

	return items, nil
}
```

### 1e. Update `ExtractCheckboxItems` doc comment

```go
// ExtractCheckboxItems finds checkbox items in the "Next steps" or "Suggested next steps"
// section of the "Notes" tab. Returns an empty slice (not an error) if no matching
// section heading is found (FR-006).
```

---

## 2. Update Browser Automation (`browser/assign-tasks.ts`)

### 2a. Extract heading candidates as a constant

Near the top of the file (after the interfaces), add:

```typescript
/**
 * Section heading candidates to search for, in priority order (FR-005).
 * The browser tries each in order; the first match found is used.
 */
const NEXT_STEPS_HEADINGS = ['Next steps', 'Suggested next steps'] as const;
```

### 2b. Replace hardcoded heading search with fallback loop

**Before** (lines 184–210):
```typescript
// Navigate to the checkbox section by searching for the section header
const modifier = process.platform === 'darwin' ? 'Meta' : 'Control';
log('Navigating to checkbox section...');
await page.keyboard.press(`${modifier}+f`);
await page.waitForTimeout(500);

const navFindInput = page.locator('...').first();
try {
    await navFindInput.waitFor({ state: 'visible', timeout: 3000 });
    await navFindInput.fill('');
    await navFindInput.fill('Suggested next steps');
    await page.waitForTimeout(500);
} catch {
    await page.keyboard.type('Suggested next steps', { delay: 20 });
    await page.waitForTimeout(500);
}

await page.keyboard.press('Enter');
await page.waitForTimeout(500);
await page.keyboard.press('Escape');
await page.waitForTimeout(500);
```

**After**:
```typescript
// Navigate to the checkbox section by searching for the section header.
// Try each candidate heading in priority order (FR-005).
const modifier = process.platform === 'darwin' ? 'Meta' : 'Control';
log('Navigating to checkbox section...');
log(`  KEY: ${modifier}+f (open find)`);
await page.keyboard.press(`${modifier}+f`);
await page.waitForTimeout(500);

const navFindInput = page.locator('.docs-findinput-input, input[aria-label="Find in document"]').first();
let sectionFound = false;

for (const heading of NEXT_STEPS_HEADINGS) {
    try {
        await navFindInput.waitFor({ state: 'visible', timeout: 3000 });
        log(`  FILL: find input ← "${heading}"`);
        await navFindInput.fill('');
        await navFindInput.fill(heading);
        await page.waitForTimeout(500);
    } catch {
        log(`  TYPE: "${heading}" (fallback)`);
        await page.keyboard.type(heading, { delay: 20 });
        await page.waitForTimeout(500);
    }

    // Check if the heading was found in the document
    const matchInfo = await page.evaluate(() => {
        const countEl = document.querySelector('.docs-findinput-count');
        if (countEl) {
            const text = countEl.textContent || '';
            const match = text.match(/(\d+)\s+of\s+(\d+)/i);
            if (match) {
                return { current: parseInt(match[1]), total: parseInt(match[2]) };
            }
        }
        return null;
    });

    if (matchInfo && matchInfo.total > 0) {
        log(`  Found "${heading}" (${matchInfo.current} of ${matchInfo.total})`);
        sectionFound = true;
        break;
    }

    log(`  "${heading}" not found, trying next candidate...`);
}

if (!sectionFound) {
    log('  WARNING: No matching section heading found in document (FR-007)');
}

log('  KEY: Enter (jump to match)');
await page.keyboard.press('Enter');
await page.waitForTimeout(500);
log('  KEY: Escape (close find bar)');
await page.keyboard.press('Escape');
await page.waitForTimeout(500);
log('Navigated to checkbox section');
```

### 2c. Update file header comment

Change line 7 from:
```typescript
 * 2. Find checkboxes in "Suggested next steps" section
```
To:
```typescript
 * 2. Find checkboxes in "Next steps" (or "Suggested next steps") section
```

---

## 3. Update Help Text (`cmd/gcal-organizer/assign_tasks.go`)

### 3a. Update command Long description

**Before** (line 33):
```go
2. Finds checkboxes in the "Suggested next steps" section
```

**After**:
```go
2. Finds checkboxes in the "Next steps" (or "Suggested next steps") section
```

---

## 4. Add/Update Tests (`internal/docs/service_test.go`)

### 4a. New test: "Next steps" heading recognized

```go
func TestExtractItemsFromSection_NextStepsHeading(t *testing.T) {
	svc := &Service{}
	content := []*docs.StructuralElement{
		{
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "Next steps"}},
				},
				ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_2"},
			},
		},
		{
			StartIndex: 15, EndIndex: 50,
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "Jay will schedule meeting"}},
				},
				Bullet: &docs.Bullet{},
				ParagraphStyle: &docs.ParagraphStyle{},
			},
		},
	}

	items, err := svc.extractItemsFromSection(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if !strings.Contains(items[0].Text, "Jay will schedule meeting") {
		t.Errorf("item text: got %q", items[0].Text)
	}
}
```

### 4b. New test: Section boundary stops at next heading

```go
func TestExtractItemsFromSection_SectionBoundary(t *testing.T) {
	svc := &Service{}
	content := []*docs.StructuralElement{
		{
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "Next steps"}},
				},
				ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_2"},
			},
		},
		{
			StartIndex: 15, EndIndex: 50,
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "Task in next steps section"}},
				},
				Bullet: &docs.Bullet{},
				ParagraphStyle: &docs.ParagraphStyle{},
			},
		},
		{
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "Meeting summary"}},
				},
				ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_2"},
			},
		},
		{
			StartIndex: 80, EndIndex: 120,
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "Unrelated bullet from later section"}},
				},
				Bullet: &docs.Bullet{},
				ParagraphStyle: &docs.ParagraphStyle{},
			},
		},
	}

	items, err := svc.extractItemsFromSection(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only the item under "Next steps" should be extracted; the one under "Meeting summary" should not
	if len(items) != 1 {
		t.Fatalf("expected 1 item (boundary enforcement), got %d", len(items))
	}
	if !strings.Contains(items[0].Text, "Task in next steps section") {
		t.Errorf("item text: got %q", items[0].Text)
	}
}
```

### 4c. New test: Both headings present — first match wins

```go
func TestExtractItemsFromSection_FirstMatchWins(t *testing.T) {
	svc := &Service{}
	content := []*docs.StructuralElement{
		{
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "Next steps"}},
				},
				ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_2"},
			},
		},
		{
			StartIndex: 15, EndIndex: 50,
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "First section task"}},
				},
				Bullet: &docs.Bullet{},
				ParagraphStyle: &docs.ParagraphStyle{},
			},
		},
		{
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "Suggested next steps"}},
				},
				ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_2"},
			},
		},
		{
			StartIndex: 80, EndIndex: 120,
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "Second section task"}},
				},
				Bullet: &docs.Bullet{},
				ParagraphStyle: &docs.ParagraphStyle{},
			},
		},
	}

	items, err := svc.extractItemsFromSection(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// First match ("Next steps") wins; "Suggested next steps" is a subsequent heading = boundary
	if len(items) != 1 {
		t.Fatalf("expected 1 item (first match wins), got %d", len(items))
	}
	if !strings.Contains(items[0].Text, "First section task") {
		t.Errorf("expected first section task, got %q", items[0].Text)
	}
}
```

### 4d. New test: Body text containing "next steps" is not matched

```go
func TestExtractItemsFromSection_BodyTextNotMatched(t *testing.T) {
	svc := &Service{}
	content := []*docs.StructuralElement{
		{
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "We discussed the next steps for the project"}},
				},
				ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "NORMAL_TEXT"},
			},
		},
		{
			StartIndex: 50, EndIndex: 80,
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "Some bullet item"}},
				},
				Bullet: &docs.Bullet{},
				ParagraphStyle: &docs.ParagraphStyle{},
			},
		},
	}

	items, err := svc.extractItemsFromSection(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Body text containing "next steps" should NOT trigger section matching
	if len(items) != 0 {
		t.Errorf("expected 0 items (body text should not match), got %d", len(items))
	}
}
```

### 4e. Update existing tests

The existing `TestExtractItemsFromSection_SuggestedNextSteps` test needs updating to include a heading style on the "Suggested next steps" paragraph:

```go
// Add ParagraphStyle with heading to the first element:
ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_2"},
```

Similarly update `TestExtractItemsFromSection_ProcessedEmoji`, `TestExtractItemsFromSection_EmptyBullet`, and `TestExtractItemsFromSection_NilParagraph`.

### 4f. New test: `isHeadingParagraph` helper

```go
func TestIsHeadingParagraph(t *testing.T) {
	tests := []struct {
		name string
		para *docs.Paragraph
		want bool
	}{
		{"HEADING_1", &docs.Paragraph{ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_1"}}, true},
		{"HEADING_6", &docs.Paragraph{ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_6"}}, true},
		{"NORMAL_TEXT", &docs.Paragraph{ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "NORMAL_TEXT"}}, false},
		{"TITLE", &docs.Paragraph{ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "TITLE"}}, false},
		{"nil style", &docs.Paragraph{}, false},
		{"nil ParagraphStyle", &docs.Paragraph{ParagraphStyle: nil}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHeadingParagraph(tt.para)
			if got != tt.want {
				t.Errorf("isHeadingParagraph: got %v, want %v", got, tt.want)
			}
		})
	}
}
```

---

## 5. Verification Checklist

After implementing all changes:

- [ ] `go build ./...` compiles without errors
- [ ] `go test ./...` passes all tests (including new ones)
- [ ] `go vet ./...` reports no warnings
- [ ] `gofmt -l .` reports no unformatted files
- [ ] `go mod tidy` produces no diff
- [ ] Existing tests for "Suggested next steps" still pass (backward compat)
- [ ] New tests for "Next steps" heading pass
- [ ] New tests for section boundary enforcement pass
- [ ] New tests for body text non-matching pass
- [ ] Browser script compiles (`npx tsx --check browser/assign-tasks.ts` or equivalent)
