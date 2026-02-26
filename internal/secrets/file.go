package secrets

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileStore implements SecretStore using the filesystem (current behavior).
// It maps well-known keys to specific files in the config directory.
type FileStore struct {
	// ConfigDir is the base directory for credential files (e.g., ~/.gcal-organizer).
	ConfigDir string
}

// Get retrieves a secret from the filesystem.
func (f *FileStore) Get(key string) (string, error) {
	switch key {
	case KeyOAuthToken:
		return f.readFile("token.json")
	case KeyGeminiAPIKey:
		return f.readEnvValue("GEMINI_API_KEY")
	case KeyClientCredentials:
		return f.readFile("credentials.json")
	default:
		return "", fmt.Errorf("unknown key: %s", key)
	}
}

// Set stores a secret to the filesystem.
func (f *FileStore) Set(key, value string) error {
	switch key {
	case KeyOAuthToken:
		return f.writeFile("token.json", value)
	case KeyGeminiAPIKey:
		return f.writeEnvValue("GEMINI_API_KEY", value)
	case KeyClientCredentials:
		return f.writeFile("credentials.json", value)
	default:
		return fmt.Errorf("unknown key: %s", key)
	}
}

// Delete removes a secret from the filesystem.
func (f *FileStore) Delete(key string) error {
	switch key {
	case KeyOAuthToken:
		return f.deleteFile("token.json")
	case KeyGeminiAPIKey:
		return f.deleteEnvLine("GEMINI_API_KEY")
	case KeyClientCredentials:
		return f.deleteFile("credentials.json")
	default:
		return fmt.Errorf("unknown key: %s", key)
	}
}

// readFile reads the entire contents of a file in the config directory.
func (f *FileStore) readFile(name string) (string, error) {
	path := filepath.Join(f.ConfigDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("read %s: %w", name, err)
	}
	return string(data), nil
}

// writeFile writes content to a file in the config directory with 0600 perms.
func (f *FileStore) writeFile(name, content string) error {
	if err := os.MkdirAll(f.ConfigDir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	path := filepath.Join(f.ConfigDir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

// deleteFile removes a file from the config directory. No error if absent.
func (f *FileStore) deleteFile(name string) error {
	path := filepath.Join(f.ConfigDir, name)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete %s: %w", name, err)
	}
	return nil
}

// readEnvValue parses the .env file and returns the value for the given key.
func (f *FileStore) readEnvValue(key string) (string, error) {
	path := filepath.Join(f.ConfigDir, ".env")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("open .env: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) == key {
			val := strings.TrimSpace(parts[1])
			// Strip surrounding quotes
			if len(val) >= 2 {
				if (val[0] == '"' && val[len(val)-1] == '"') ||
					(val[0] == '\'' && val[len(val)-1] == '\'') {
					val = val[1 : len(val)-1]
				}
			}
			// Unescape POSIX '\'' sequences used by writeEnvValue.
			val = strings.ReplaceAll(val, `'\''`, `'`)
			return val, nil
		}
	}

	return "", ErrNotFound
}

// writeEnvValue writes or updates a key=value line in the .env file.
func (f *FileStore) writeEnvValue(key, value string) error {
	if err := os.MkdirAll(f.ConfigDir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	path := filepath.Join(f.ConfigDir, ".env")
	lines, err := readLines(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read .env: %w", err)
	}

	// Escape single quotes for POSIX shell compatibility (e.g., end'quote → end'\''quote).
	escaped := strings.ReplaceAll(value, "'", "'\\''")

	// Update existing line or append
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) == key {
			lines[i] = fmt.Sprintf("%s='%s'", key, escaped)
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, fmt.Sprintf("%s='%s'", key, escaped))
	}

	return writeLines(path, lines)
}

// deleteEnvLine removes a key=value line from the .env file.
func (f *FileStore) deleteEnvLine(key string) error {
	path := filepath.Join(f.ConfigDir, ".env")
	lines, err := readLines(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to delete
		}
		return fmt.Errorf("read .env: %w", err)
	}

	var kept []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			kept = append(kept, line)
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) == key {
			continue // skip this line
		}
		kept = append(kept, line)
	}

	return writeLines(path, kept)
}

// readLines reads all lines from a file.
func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := string(data)
	if content == "" {
		return nil, nil
	}
	return strings.Split(content, "\n"), nil
}

// writeLines writes lines to a file atomically (temp file + rename).
func writeLines(path string, lines []string) error {
	content := strings.Join(lines, "\n")
	// Ensure file ends with newline
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0600); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) // best-effort cleanup
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
