package utils

// GenerateSlug creates a URL-friendly name from a title.
// It lowercases the title, replaces spaces and underscores with dashes,
// removes non-alphanumeric characters, and deduplicates/trims dashes.
func GenerateSlug(title string) string {
	slug := ""
	for _, c := range title {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			slug += string(c)
		} else if c >= 'A' && c <= 'Z' {
			slug += string(c - 'A' + 'a')
		} else if c == ' ' || c == '_' {
			slug += "-"
		}
	}
	// Remove consecutive dashes
	result := ""
	prev := byte(0)
	for i := 0; i < len(slug); i++ {
		if slug[i] == '-' && prev == '-' {
			continue
		}
		result += string(slug[i])
		prev = slug[i]
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
