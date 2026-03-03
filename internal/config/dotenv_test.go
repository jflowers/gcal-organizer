package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	tests := []struct {
		name    string
		content string
		key     string
		want    string
		preSet  string // pre-set env var value (to test precedence)
	}{
		{
			name:    "KEY=VALUE",
			content: "MY_KEY=my_value\n",
			key:     "MY_KEY",
			want:    "my_value",
		},
		{
			name:    "double-quoted value",
			content: `MY_QUOTED="hello world"` + "\n",
			key:     "MY_QUOTED",
			want:    "hello world",
		},
		{
			name:    "single-quoted value",
			content: "MY_SINGLE='hello world'\n",
			key:     "MY_SINGLE",
			want:    "hello world",
		},
		{
			name:    "comment lines skipped",
			content: "# This is a comment\nACTUAL_KEY=actual_value\n",
			key:     "ACTUAL_KEY",
			want:    "actual_value",
		},
		{
			name:    "blank lines skipped",
			content: "\n\n  \nBLANK_KEY=blank_value\n",
			key:     "BLANK_KEY",
			want:    "blank_value",
		},
		{
			name:    "POSIX escape single quote",
			content: "ESCAPED_KEY='it'\\''s here'\n",
			key:     "ESCAPED_KEY",
			want:    "it's here",
		},
		{
			name:    "tilde expansion",
			content: "TILDE_KEY=~/config\n",
			key:     "TILDE_KEY",
			want:    "/fake-home/config",
		},
		{
			name:    "tilde only",
			content: "TILDE_ONLY=~\n",
			key:     "TILDE_ONLY",
			want:    "/fake-home",
		},
		{
			name:    "env precedence - existing var wins",
			content: "PREEXIST_KEY=from-file\n",
			key:     "PREEXIST_KEY",
			want:    "from-env",
			preSet:  "from-env",
		},
		{
			name:    "invalid key skipped",
			content: "123INVALID=value\nVALID_KEY=ok\n",
			key:     "VALID_KEY",
			want:    "ok",
		},
		{
			name:    "no equals sign skipped",
			content: "NO_EQUALS\nHAS_EQUALS=value\n",
			key:     "HAS_EQUALS",
			want:    "value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up env after test
			os.Unsetenv(tt.key)
			defer os.Unsetenv(tt.key)

			if tt.preSet != "" {
				os.Setenv(tt.key, tt.preSet)
			}

			// Write temp .env file
			dir := t.TempDir()
			envPath := filepath.Join(dir, ".env")
			if err := os.WriteFile(envPath, []byte(tt.content), 0600); err != nil {
				t.Fatalf("write .env: %v", err)
			}

			LoadDotEnv(envPath, "/fake-home")

			got := os.Getenv(tt.key)
			if got != tt.want {
				t.Errorf("env %s: got %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestLoadDotEnv_MissingFile(t *testing.T) {
	// Should not panic or error for missing file
	LoadDotEnv("/nonexistent/path/.env", "/home")
}
