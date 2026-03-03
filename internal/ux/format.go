package ux

// MaskSecret returns a partially-redacted version of a secret string for display.
func MaskSecret(s string) string {
	if len(s) == 0 {
		return "(not set)"
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

// TruncateText shortens s to at most maxLen runes, appending "..." if truncated.
// Operates on runes (not bytes) to correctly handle multi-byte UTF-8 characters.
// maxLen < 3 is clamped to 3 to avoid a negative-index panic in the slice expression.
func TruncateText(s string, maxLen int) string {
	if maxLen < 3 {
		maxLen = 3
	}
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen-3]) + "..."
}
