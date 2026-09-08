package config

import (
	"bufio"
	"os"
	"strings"
)

// LoadDotEnv sets the KEY=VALUE pairs of a dotenv file into the process
// environment. Blank lines and `#` comments are skipped, surrounding single or
// double quotes are stripped, and variables already set are never overridden,
// matching what godotenv.Load did before.
// ponytail: no `export` prefix, `${VAR}` expansion or multi-line values; none of
// the repo's .env files use them.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		} else {
			value = stripInlineComment(value)
		}
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
	return scanner.Err()
}

// An unquoted value ends at the first "#" preceded by whitespace; "#" inside a word is data.
func stripInlineComment(value string) string {
	for _, marker := range []string{" #", "\t#"} {
		if i := strings.Index(value, marker); i >= 0 {
			value = value[:i]
		}
	}
	return strings.TrimSpace(value)
}
