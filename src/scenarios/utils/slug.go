package utils

import "strings"

// GenerateSlug creates a URL-friendly name from a title.
// It lowercases the title, replaces spaces and underscores with dashes,
// removes non-alphanumeric characters, and deduplicates/trims dashes.
func GenerateSlug(title string) string {
	var slug strings.Builder
	for _, c := range title {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			slug.WriteString(string(c))
		} else if c >= 'A' && c <= 'Z' {
			slug.WriteString(string(c - 'A' + 'a'))
		} else if c == ' ' || c == '_' {
			slug.WriteString("-")
		}
	}
	// Remove consecutive dashes
	result := ""
	prev := byte(0)
	for i := 0; i < len(slug.String()); i++ {
		if slug.String()[i] == '-' && prev == '-' {
			continue
		}
		result += string(slug.String()[i])
		prev = slug.String()[i]
	}
	// Trim leading/trailing dashes
	for len(result) > 0 && result[0] == '-' {
		result = result[1:]
	}
	for len(result) > 0 && result[len(result)-1] == '-' {
		result = result[:len(result)-1]
	}
	return result
}
