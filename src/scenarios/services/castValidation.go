package services

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MaxCastBytes is the maximum size of an asciicast v2 effect asset. The
// frontend replays these in-browser, so anything larger than ~1MB is an
// authoring mistake (matches the seed DTO binding max).
const MaxCastBytes = 1_200_000

// asciicastHeader is the subset of the asciicast v2 header needed for validation.
type asciicastHeader struct {
	Version int `json:"version"`
}

// ValidateAsciicastV2 checks that content looks like an asciicast v2 recording:
// non-empty, within size limits, and with a first JSON line declaring version 2.
func ValidateAsciicastV2(content string) error {
	if content == "" {
		return fmt.Errorf("cast content is empty")
	}
	if len(content) > MaxCastBytes {
		return fmt.Errorf("cast content exceeds %d bytes", MaxCastBytes)
	}
	firstLine, _, _ := strings.Cut(content, "\n")
	var header asciicastHeader
	if err := json.Unmarshal([]byte(firstLine), &header); err != nil {
		return fmt.Errorf("first line is not a valid asciicast header: %w", err)
	}
	if header.Version != 2 {
		return fmt.Errorf("unsupported asciicast version %d (want 2)", header.Version)
	}
	return nil
}
