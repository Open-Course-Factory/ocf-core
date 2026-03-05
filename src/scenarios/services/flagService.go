package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"

	"soli/formations/src/scenarios/models"

	"github.com/google/uuid"
)

// FlagService generates and validates per-student, per-step unique flags using HMAC-SHA256
type FlagService struct{}

// NewFlagService creates a new FlagService instance
func NewFlagService() *FlagService {
	return &FlagService{}
}

// GenerateFlags creates flags for all flag-enabled steps in a scenario.
// Returns a slice of ScenarioFlag models ready to be saved.
// Flag format: FLAG{<16-hex-chars>}
// Algorithm: HMAC-SHA256(scenario.FlagSecret, sessionID:stepOrder:userID) truncated to 16 hex chars
func (s *FlagService) GenerateFlags(scenario *models.Scenario, sessionID uuid.UUID, userID string) []models.ScenarioFlag {
	var flags []models.ScenarioFlag

	for _, step := range scenario.Steps {
		if !step.HasFlag {
			continue
		}

		flagValue := s.computeFlag(scenario.FlagSecret, sessionID, step.Order, userID)

		flag := models.ScenarioFlag{
			SessionID:    sessionID,
			StepOrder:    step.Order,
			ExpectedFlag: flagValue,
		}

		flags = append(flags, flag)
	}

	return flags
}

// ValidateFlag checks if a submitted flag matches the expected flag for a step.
// Uses constant-time comparison to prevent timing attacks.
func (s *FlagService) ValidateFlag(expected string, submitted string) bool {
	return subtle.ConstantTimeCompare([]byte(expected), []byte(submitted)) == 1
}

// computeFlag generates a flag value using HMAC-SHA256
func (s *FlagService) computeFlag(secret string, sessionID uuid.UUID, stepOrder int, userID string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	input := fmt.Sprintf("%s:%d:%s", sessionID.String(), stepOrder, userID)
	mac.Write([]byte(input))
	hash := mac.Sum(nil)
	hexStr := hex.EncodeToString(hash)

	return fmt.Sprintf("FLAG{%s}", hexStr[:16])
}
