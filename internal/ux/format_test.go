package ux

import "testing"

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", "(not set)"},
		{"1 char", "a", "****"},
		{"3 chars", "abc", "****"},
		{"8 chars", "abcdefgh", "****"},
		{"9 chars", "abcdefghi", "abcd****fghi"},
		{"long key", "AIzaSyD-test-key-12345", "AIza****2345"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaskSecret(tt.input)
			if got != tt.want {
				t.Errorf("MaskSecret(%q): got %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"shorter than max", "hello", 10, "hello"},
		{"exactly max", "hello", 5, "hello"},
		{"longer than max", "hello world", 8, "hello..."},
		{"empty", "", 10, ""},
		{"max 0 clamps to 3", "hello", 0, "..."},
		{"max 1 clamps to 3", "hello", 1, "..."},
		{"max 3", "hello", 3, "..."},
		{"unicode multi-byte", "café latte", 7, "café..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateText(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("TruncateText(%q, %d): got %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}
