package services

import (
	"regexp"
	"strings"

	"soli/formations/src/scenarios/models"
)

// BuildStepHints turns a step's hint markdown into the progressive hint rows a
// learner reveals one by one. Empty content means no hints at all.
func BuildStepHints(content string) []models.ScenarioStepHint {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	parts := SplitHintContent(content)
	hints := make([]models.ScenarioStepHint, len(parts))
	for i, part := range parts {
		hints[i] = models.ScenarioStepHint{Level: i + 1, Content: part}
	}
	return hints
}

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
