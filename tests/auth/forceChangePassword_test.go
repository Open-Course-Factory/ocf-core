package auth_tests

import (
	"encoding/json"
	"testing"

	"soli/formations/src/auth/dto"

	"github.com/stretchr/testify/assert"
)

// TestForceChangePassword_DTOBinding validates JSON binding and field tags
func TestForceChangePassword_DTOBinding(t *testing.T) {
	t.Run("valid_input_parses_correctly", func(t *testing.T) {
		jsonInput := `{"new_password":"SecurePass123!","confirm_password":"SecurePass123!"}`
		var input dto.ForceChangePasswordInput
		err := json.Unmarshal([]byte(jsonInput), &input)

		assert.NoError(t, err)
		assert.Equal(t, "SecurePass123!", input.NewPassword)
		assert.Equal(t, "SecurePass123!", input.ConfirmPassword)
	})

	t.Run("json_tags_are_snake_case", func(t *testing.T) {
		input := dto.ForceChangePasswordInput{
			NewPassword:     "SecurePass123!",
			ConfirmPassword: "SecurePass123!",
		}

		jsonBytes, err := json.Marshal(input)
		assert.NoError(t, err)

		jsonStr := string(jsonBytes)
		assert.Contains(t, jsonStr, `"new_password"`)
		assert.Contains(t, jsonStr, `"confirm_password"`)
		assert.NotContains(t, jsonStr, `"NewPassword"`)
		assert.NotContains(t, jsonStr, `"ConfirmPassword"`)
	})

	t.Run("empty_json_parses_with_empty_fields", func(t *testing.T) {
		jsonInput := `{}`
		var input dto.ForceChangePasswordInput
		err := json.Unmarshal([]byte(jsonInput), &input)

		assert.NoError(t, err)
		assert.Empty(t, input.NewPassword)
		assert.Empty(t, input.ConfirmPassword)
	})

	t.Run("invalid_json_fails", func(t *testing.T) {
		jsonInput := `{invalid}`
		var input dto.ForceChangePasswordInput
		err := json.Unmarshal([]byte(jsonInput), &input)

		assert.Error(t, err)
	})
}

// TestCurrentUserOutput_ForcePasswordReset validates the DTO includes the new field
func TestCurrentUserOutput_ForcePasswordReset(t *testing.T) {
	t.Run("force_password_reset_false_by_default", func(t *testing.T) {
		output := dto.CurrentUserOutput{
			UserID:   "test-user-id",
			UserName: "testuser",
			Email:    "test@example.com",
			Roles:    []string{"member"},
		}

		jsonBytes, err := json.Marshal(output)
		assert.NoError(t, err)

		jsonStr := string(jsonBytes)
		assert.Contains(t, jsonStr, `"force_password_reset":false`)
	})

	t.Run("force_password_reset_true", func(t *testing.T) {
		output := dto.CurrentUserOutput{
			UserID:             "test-user-id",
			UserName:           "testuser",
			Email:              "test@example.com",
			Roles:              []string{"member"},
			ForcePasswordReset: true,
		}

		jsonBytes, err := json.Marshal(output)
		assert.NoError(t, err)

		jsonStr := string(jsonBytes)
		assert.Contains(t, jsonStr, `"force_password_reset":true`)
	})

	t.Run("json_tag_is_snake_case", func(t *testing.T) {
		output := dto.CurrentUserOutput{
			UserID:             "test-user-id",
			ForcePasswordReset: true,
			Roles:              []string{},
		}

		jsonBytes, err := json.Marshal(output)
		assert.NoError(t, err)

		jsonStr := string(jsonBytes)
		assert.Contains(t, jsonStr, `"force_password_reset"`)
		assert.NotContains(t, jsonStr, `"ForcePasswordReset"`)
	})
}
