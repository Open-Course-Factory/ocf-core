package services

import "strings"

// insertAfterShebang puts a directive at the top of a script body, after the
// shebang when there is one. Inserting before "#!" would demote the shebang to
// an ordinary comment and silently change the interpreter.
func insertAfterShebang(script string, line string) string {
	if strings.HasPrefix(script, "#!") {
		if idx := strings.IndexByte(script, '\n'); idx != -1 {
			return script[:idx+1] + line + "\n" + script[idx+1:]
		}
	}
	return line + "\n" + script
}

// injectSetE prepends "set -e" to a shell script if it doesn't already contain it.
// This ensures any failing command stops the script immediately rather than
// silently continuing with a broken environment.
func injectSetE(script string) string {
	// Don't inject if the script already has set -e (or set -euo pipefail, etc.)
	if strings.Contains(script, "set -e") {
		return script
	}
	return insertAfterShebang(script, "set -e")
}

// injectForceFlag exports FORCE=1 into a background script's environment.
// Step scripts guard their work behind idempotency markers so an advance can be
// replayed safely; FORCE=1 is the agreed opt-out that tells a script to redo
// work it already marked done. The container exec API carries no environment,
// so the export has to live in the script text.
func injectForceFlag(script string) string {
	return insertAfterShebang(script, "export FORCE=1")
}

// truncateLog truncates log output to a reasonable length for structured logging.
func truncateLog(s string) string {
	const maxLen = 500
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... (truncated)"
}
