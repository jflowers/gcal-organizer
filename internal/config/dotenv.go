package config

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

// ValidEnvKey matches a valid POSIX environment variable name.
var ValidEnvKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// LoadDotEnv reads a .env file and sets any KEY=VALUE pairs as environment
// variables, but only if they are not already set (env vars take precedence).
// Tilde (~) in values is expanded to the provided home directory.
func LoadDotEnv(path, home string) {
	f, err := os.Open(path)
	if err != nil {
		return // .env is optional
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		if !ValidEnvKey.MatchString(key) {
			continue
		}
		val := strings.TrimSpace(parts[1])

		// Strip surrounding quotes (double or single) for bash compatibility.
		// For single-quoted values also unescape the POSIX '\'' sequence that
		// generateEnvFile uses to embed a literal single-quote in the value.
		if len(val) >= 2 {
			switch {
			case val[0] == '"' && val[len(val)-1] == '"':
				val = val[1 : len(val)-1]
			case val[0] == '\'' && val[len(val)-1] == '\'':
				val = val[1 : len(val)-1]
				// Unescape '\'' → ' (POSIX single-quote escape sequence)
				val = strings.ReplaceAll(val, `'\''`, `'`)
			}
		}

		// Expand ~ to home directory
		if strings.HasPrefix(val, "~/") {
			val = home + val[1:]
		} else if val == "~" {
			val = home
		}

		// Only set if not already in environment (explicit env vars win)
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
		}
	}
}
