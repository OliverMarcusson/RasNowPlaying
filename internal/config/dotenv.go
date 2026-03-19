package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// LoadDotEnv loads KEY=VALUE pairs from path into the process environment
// without overwriting variables that are already set.
func LoadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("%s:%d: expected KEY=VALUE", path, lineNo)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("%s:%d: empty key", path, lineNo)
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}

		value = strings.TrimSpace(value)
		switch {
		case len(value) >= 2 && strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\""):
			value = strings.TrimSuffix(strings.TrimPrefix(value, "\""), "\"")
		case len(value) >= 2 && strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'"):
			value = strings.TrimSuffix(strings.TrimPrefix(value, "'"), "'")
		}

		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("%s:%d: set %s: %w", path, lineNo, key, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
