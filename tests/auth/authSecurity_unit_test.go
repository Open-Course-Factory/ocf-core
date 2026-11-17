package auth_tests

import (
	"encoding/json"
	"testing"

	"soli/formations/src/auth/dto"

	"github.com/stretchr/testify/assert"
)

// TestLoginDTOStructure tests the LoginInput DTO structure (unit test)
func TestLoginDTOStructure(t *testing.T) {
	t.Run("login_input_has_correct_json_tags", func(t *testing.T) {
		// Create a sample input
		input := dto.LoginInput{
			Email:    "test@example.com",
			Password: "Test123!",
		}

		// Marshal to JSON
		jsonBytes, err := json.Marshal(input)
		assert.NoError(t, err, "Should marshal successfully")

		// Verify JSON contains lowercase field names
		jsonStr := string(jsonBytes)
		assert.Contains(t, jsonStr, `"email"`, "JSON should have lowercase 'email' field")
		assert.Contains(t, jsonStr, `"password"`, "JSON should have lowercase 'password' field")
		assert.NotContains(t, jsonStr, `"Email"`, "JSON should not have uppercase 'Email' field")
		assert.NotContains(t, jsonStr, `"Password"`, "JSON should not have uppercase 'Password' field")
	})

	t.Run("login_output_has_correct_structure", func(t *testing.T) {
		output := dto.LoginOutput{
			UserName:         "testuser",
			DisplayName:      "Test User",
			UserId:           "test-id-123",
			AccessToken:      "test-token",
			RenewAccessToken: "test-refresh-token",
			UserRoles:        []string{"member"},
		}

		jsonBytes, err := json.Marshal(output)
		assert.NoError(t, err, "Should marshal successfully")

		jsonStr := string(jsonBytes)
		assert.Contains(t, jsonStr, `"user_name"`, "Should have snake_case user_name")
		assert.Contains(t, jsonStr, `"display_name"`, "Should have snake_case display_name")
		assert.Contains(t, jsonStr, `"user_id"`, "Should have snake_case user_id")
		assert.Contains(t, jsonStr, `"access_token"`, "Should have snake_case access_token")
	})
}
