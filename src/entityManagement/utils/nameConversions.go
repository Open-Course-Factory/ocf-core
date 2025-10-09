package utils

import "strings"

func KebabToPascal(s string) string {
	if len(s) == 0 {
		return s
	}

	parts := strings.Split(s, "-")
	var result strings.Builder

	for _, part := range parts {
		if len(part) > 0 {
			result.WriteString(strings.ToUpper(string(part[0])) + strings.ToLower(part[1:]))
		}
	}

	return result.String()
}

func PascalToKebab(s string) string {
	if len(s) == 0 {
		return s
	}

	var result strings.Builder

	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('-')
		}
		result.WriteRune(r)
	}

	return strings.ToLower(result.String())
}
