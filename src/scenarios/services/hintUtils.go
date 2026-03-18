package services

import (
	"regexp"
	"strings"
)

// SplitHintContent splits a single hint content string into multiple hints
// by detecting `### Indice N` or `### Hint N` headers (case-insensitive, optional colon).
// If fewer than 2 headers are found, the entire content is returned as a single hint.
func SplitHintContent(content string) []string {
	re := regexp.MustCompile(`(?mi)^###[ \t]+(?:indice|hint)[ \t]+\d+[ \t]*:?[^\n]*$`)
	locs := re.FindAllStringIndex(content, -1)

	if len(locs) < 2 {
		return []string{strings.TrimSpace(content)}
	}

	var parts []string
	for i, loc := range locs {
		var chunk string
		if i+1 < len(locs) {
			chunk = content[loc[1]:locs[i+1][0]]
		} else {
			chunk = content[loc[1]:]
		}
		chunk = strings.TrimSpace(chunk)
		if chunk != "" {
			parts = append(parts, chunk)
		}
	}
	if len(parts) == 0 {
		return []string{strings.TrimSpace(content)}
	}
	return parts
}
