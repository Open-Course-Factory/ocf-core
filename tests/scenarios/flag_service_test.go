package scenarios_test

import (
	"regexp"
	"testing"

	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlagService_GenerateFlags(t *testing.T) {
	svc := services.NewFlagService()

	scenario := &models.Scenario{
		FlagSecret: "test-secret-key",
		Steps: []models.ScenarioStep{
			{Order: 1, Title: "Step 1", HasFlag: true},
			{Order: 2, Title: "Step 2", HasFlag: false},
			{Order: 3, Title: "Step 3", HasFlag: true},
			{Order: 4, Title: "Step 4", HasFlag: false},
		},
	}

	sessionID := uuid.New()
	userID := "user-123"

	flags := svc.GenerateFlags(scenario, sessionID, userID)

	// Should only generate flags for flag-enabled steps (steps 1 and 3)
	require.Len(t, flags, 2)
	assert.Equal(t, 1, flags[0].StepOrder)
	assert.Equal(t, 3, flags[1].StepOrder)
	assert.Equal(t, sessionID, flags[0].SessionID)
	assert.Equal(t, sessionID, flags[1].SessionID)
	assert.NotEmpty(t, flags[0].ExpectedFlag)
	assert.NotEmpty(t, flags[1].ExpectedFlag)
}

func TestFlagService_FlagsAreUnique(t *testing.T) {
	svc := services.NewFlagService()

	scenario := &models.Scenario{
		FlagSecret: "test-secret-key",
		Steps: []models.ScenarioStep{
			{Order: 1, Title: "Step 1", HasFlag: true},
		},
	}

	session1 := uuid.New()
	session2 := uuid.New()
	userID := "user-123"

	flags1 := svc.GenerateFlags(scenario, session1, userID)
	flags2 := svc.GenerateFlags(scenario, session2, userID)

	require.Len(t, flags1, 1)
	require.Len(t, flags2, 1)

	// Different sessions must produce different flags
	assert.NotEqual(t, flags1[0].ExpectedFlag, flags2[0].ExpectedFlag)

	// Same session + different user also produces different flags
	flags3 := svc.GenerateFlags(scenario, session1, "user-456")
	require.Len(t, flags3, 1)
	assert.NotEqual(t, flags1[0].ExpectedFlag, flags3[0].ExpectedFlag)
}

func TestFlagService_ValidateFlag_Correct(t *testing.T) {
	svc := services.NewFlagService()

	expected := "FLAG{abcdef1234567890}"
	assert.True(t, svc.ValidateFlag(expected, "FLAG{abcdef1234567890}"))
}

func TestFlagService_ValidateFlag_Incorrect(t *testing.T) {
	svc := services.NewFlagService()

	expected := "FLAG{abcdef1234567890}"
	assert.False(t, svc.ValidateFlag(expected, "FLAG{0000000000000000}"))
	assert.False(t, svc.ValidateFlag(expected, "wrong-flag"))
	assert.False(t, svc.ValidateFlag(expected, ""))
}

func TestFlagService_FlagFormat(t *testing.T) {
	svc := services.NewFlagService()

	scenario := &models.Scenario{
		FlagSecret: "another-secret",
		Steps: []models.ScenarioStep{
			{Order: 1, Title: "Step 1", HasFlag: true},
			{Order: 2, Title: "Step 2", HasFlag: true},
		},
	}

	sessionID := uuid.New()
	userID := "user-test"

	flags := svc.GenerateFlags(scenario, sessionID, userID)
	require.Len(t, flags, 2)

	// All flags must match the FLAG{<16-hex-chars>} pattern
	flagPattern := regexp.MustCompile(`^FLAG\{[0-9a-f]{16}\}$`)
	for _, flag := range flags {
		assert.Regexp(t, flagPattern, flag.ExpectedFlag,
			"Flag should match FLAG{<16-hex-chars>} pattern, got: %s", flag.ExpectedFlag)
	}

	// Flags for different steps should be different
	assert.NotEqual(t, flags[0].ExpectedFlag, flags[1].ExpectedFlag)
}

func TestFlagService_GenerateFlags_NoFlagSteps(t *testing.T) {
	svc := services.NewFlagService()

	scenario := &models.Scenario{
		FlagSecret: "test-secret",
		Steps: []models.ScenarioStep{
			{Order: 1, Title: "Step 1", HasFlag: false},
			{Order: 2, Title: "Step 2", HasFlag: false},
		},
	}

	flags := svc.GenerateFlags(scenario, uuid.New(), "user-123")
	assert.Empty(t, flags)
}

func TestFlagService_GenerateFlags_Deterministic(t *testing.T) {
	svc := services.NewFlagService()

	scenario := &models.Scenario{
		FlagSecret: "deterministic-test",
		Steps: []models.ScenarioStep{
			{Order: 1, Title: "Step 1", HasFlag: true},
		},
	}

	sessionID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	userID := "user-fixed"

	flags1 := svc.GenerateFlags(scenario, sessionID, userID)
	flags2 := svc.GenerateFlags(scenario, sessionID, userID)

	require.Len(t, flags1, 1)
	require.Len(t, flags2, 1)

	// Same inputs must produce the same flag (deterministic)
	assert.Equal(t, flags1[0].ExpectedFlag, flags2[0].ExpectedFlag)
}
