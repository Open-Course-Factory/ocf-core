package services

import "strings"

// parseShebang extracts the interpreter path from a script's shebang line.
// If no shebang is present, it returns "/bin/sh" as the default interpreter.
//
// For "#!/usr/bin/env X" shebangs, it resolves the program name to /bin/X,
// since we need to invoke the interpreter with "-c" (env doesn't support that).
//
// Examples:
//
//	"#!/bin/bash\necho hi"         → "/bin/bash"
//	"#!/bin/sh\necho hi"           → "/bin/sh"
//	"#!/usr/bin/bash\necho hi"     → "/usr/bin/bash"
//	"#!/usr/bin/env bash\necho hi" → "/bin/bash"
//	"echo hi"                      → "/bin/sh"
func parseShebang(script string) string {
	if !strings.HasPrefix(script, "#!") {
		return "/bin/sh"
	}

	// Extract the first line
	firstLine := script
	if idx := strings.IndexByte(script, '\n'); idx != -1 {
		firstLine = script[:idx]
	}

	// Remove the "#!" prefix and trim whitespace
	shebang := strings.TrimSpace(firstLine[2:])

	if shebang == "" {
		return "/bin/sh"
	}

	// Handle "#!/usr/bin/env X" → resolve to /bin/X
	const envPrefix = "/usr/bin/env "
	if strings.HasPrefix(shebang, envPrefix) {
		program := strings.TrimSpace(shebang[len(envPrefix):])
		if program == "" {
			return "/bin/sh"
		}
		return "/bin/" + program
	}

	// Use the shebang path directly (e.g., "/bin/bash", "/usr/bin/bash")
	// Take only the first token in case of flags like "#!/bin/bash -e"
	if idx := strings.IndexByte(shebang, ' '); idx != -1 {
		shebang = shebang[:idx]
	}

	return shebang
}
