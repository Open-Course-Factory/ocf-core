package services

import "strings"

// injectSetE prepends "set -e" to a shell script if it doesn't already contain it.
// This ensures any failing command stops the script immediately rather than
// silently continuing with a broken environment.
func injectSetE(script string) string {
	// Don't inject if the script already has set -e (or set -euo pipefail, etc.)
	if strings.Contains(script, "set -e") {
		return script
	}

	// Insert after shebang if present, otherwise prepend
	if strings.HasPrefix(script, "#!") {
		if idx := strings.IndexByte(script, '\n'); idx != -1 {
			return script[:idx+1] + "set -e\n" + script[idx+1:]
		}
	}
	return "set -e\n" + script
}

// truncateLog truncates log output to a reasonable length for structured logging.
func truncateLog(s string) string {
	const maxLen = 500
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... (truncated)"
}
