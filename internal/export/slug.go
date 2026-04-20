// Package export provides markdown file export for extracted meeting decisions.
//
// slug.go implements topic slug generation for decision export filenames.
// Slugs are kebab-case, filename-safe strings derived from meeting titles.
package export

import (
	"regexp"
	"strings"
)

// nonAlphanumHyphen matches any character that is not a lowercase letter,
// digit, or hyphen. Used to sanitize topic slugs for filenames.
var nonAlphanumHyphen = regexp.MustCompile(`[^a-z0-9-]`)

// consecutiveHyphens matches two or more consecutive hyphens for collapsing.
var consecutiveHyphens = regexp.MustCompile(`-{2,}`)

// TopicSlug converts a meeting title into a kebab-case, filename-safe slug.
//
// Rules (per data-model.md):
//  1. Strip known prefixes (case-insensitive): "Notes by Gemini - ", "Notes by Gemini"
//  2. Strip known suffixes (case-insensitive): " - Transcript", "- Transcript"
//  3. Trim whitespace
//  4. Convert to lowercase
//  5. Replace any character not in [a-z0-9-] with a hyphen
//  6. Collapse consecutive hyphens to a single hyphen
//  7. Trim leading/trailing hyphens
//  8. Fallback to "meeting" for empty result
func TopicSlug(title string) string {
	s := title

	// 1. Strip known prefixes (case-insensitive)
	prefixes := []string{"Notes by Gemini - ", "Notes by Gemini"}
	for _, prefix := range prefixes {
		if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
			s = s[len(prefix):]
			break
		}
	}

	// 2. Strip known suffixes (case-insensitive)
	suffixes := []string{" - Transcript", "- Transcript"}
	for _, suffix := range suffixes {
		if len(s) >= len(suffix) && strings.EqualFold(s[len(s)-len(suffix):], suffix) {
			s = s[:len(s)-len(suffix)]
			break
		}
	}

	// 3. Trim whitespace
	s = strings.TrimSpace(s)

	// 4. Convert to lowercase
	s = strings.ToLower(s)

	// 5. Replace non-alphanumeric (except hyphens) with hyphens
	s = nonAlphanumHyphen.ReplaceAllString(s, "-")

	// 6. Collapse consecutive hyphens
	s = consecutiveHyphens.ReplaceAllString(s, "-")

	// 7. Trim leading/trailing hyphens
	s = strings.Trim(s, "-")

	// 8. Fallback for empty result
	if s == "" {
		return "meeting"
	}

	return s
}
