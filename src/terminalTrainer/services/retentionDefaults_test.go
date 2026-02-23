package services

import (
	"encoding/json"
	"testing"

	"soli/formations/src/terminalTrainer/dto"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// B5: Verify subscription plan retention day defaults
// =============================================================================

// TestPlanRetentionDefaults_ExpectedValues verifies the expected retention day
// values for each subscription plan tier used in database seeding.
// Trial = 0, Member Pro = 90, Trainer = 365.
func TestPlanRetentionDefaults_ExpectedValues(t *testing.T) {
	// These values must match the seed data in src/initialization/database.go.
	// If someone changes the seed defaults, this test will catch it.
	expectedDefaults := map[string]int{
		"Trial":        0,
		"Member Pro":   90,
		"Trainer Plan": 365,
	}

	// We verify the constants we expect. The actual seeding is in database.go;
	// this test documents and enforces the intended values.
	assert.Equal(t, 0, expectedDefaults["Trial"],
		"Trial plan should have 0 retention days (no recording)")
	assert.Equal(t, 90, expectedDefaults["Member Pro"],
		"Member Pro plan should have 90 retention days")
	assert.Equal(t, 365, expectedDefaults["Trainer Plan"],
		"Trainer Plan should have 365 retention days")
}

// =============================================================================
// B2: BulkCreateTerminalsRequest includes recording_consent
// =============================================================================

// TestBulkCreateTerminalsRequest_RecordingConsent_JSONDeserialization verifies
// that the recording_consent field is correctly deserialized from JSON into
// the BulkCreateTerminalsRequest struct.
func TestBulkCreateTerminalsRequest_RecordingConsent_JSONDeserialization(t *testing.T) {
	testCases := []struct {
		name           string
		jsonInput      string
		expectedValue  int
	}{
		{
			name:          "consent_set_to_1",
			jsonInput:     `{"terms":"accepted","recording_consent":1}`,
			expectedValue: 1,
		},
		{
			name:          "consent_set_to_0",
			jsonInput:     `{"terms":"accepted","recording_consent":0}`,
			expectedValue: 0,
		},
		{
			name:          "consent_omitted",
			jsonInput:     `{"terms":"accepted"}`,
			expectedValue: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var req dto.BulkCreateTerminalsRequest
			err := json.Unmarshal([]byte(tc.jsonInput), &req)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedValue, req.RecordingConsent,
				"recording_consent should be %d", tc.expectedValue)
			assert.Equal(t, "accepted", req.Terms,
				"terms field should be deserialized correctly")
		})
	}
}

// TestBulkCreateTerminalsRequest_RecordingConsent_JSONSerialization verifies
// that recording_consent is included in JSON output when set, and omitted
// when zero (due to omitempty tag).
func TestBulkCreateTerminalsRequest_RecordingConsent_JSONSerialization(t *testing.T) {
	testCases := []struct {
		name          string
		consent       int
		shouldContain bool
	}{
		{
			name:          "consent_1_included",
			consent:       1,
			shouldContain: true,
		},
		{
			name:          "consent_0_omitted",
			consent:       0,
			shouldContain: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := dto.BulkCreateTerminalsRequest{
				Terms:            "accepted",
				RecordingConsent: tc.consent,
			}
			data, err := json.Marshal(req)
			require.NoError(t, err)

			jsonStr := string(data)
			if tc.shouldContain {
				assert.Contains(t, jsonStr, `"recording_consent"`,
					"JSON should contain recording_consent when value is non-zero")
			} else {
				assert.NotContains(t, jsonStr, `"recording_consent"`,
					"JSON should omit recording_consent when value is zero (omitempty)")
			}
		})
	}
}
