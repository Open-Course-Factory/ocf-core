package auth_tests

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"soli/formations/src/auth/dto"
)

// TestLoginInputJSONTagsUnit tests the JSON binding tags work correctly (unit test)
func TestLoginInputJSONTagsUnit(t *testing.T) {
	tests := []struct {
		name        string
		jsonInput   string
		expectError bool
		description string
	}{
		{
			name:        "lowercase_fields_parse_correctly",
			jsonInput:   `{"email":"test@example.com","password":"Test123!"}`,
			expectError: false,
			description: "Lowercase JSON fields should parse correctly",
		},
		{
			name:        "empty_json_fails",
			jsonInput:   `{}`,
			expectError: false, // JSON will parse but fields will be empty
			description: "Empty JSON should parse but have empty fields",
		},
		{
			name:        "invalid_json_fails",
			jsonInput:   `{invalid json}`,
			expectError: true,
			description: "Invalid JSON should fail to parse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input dto.LoginInput
			err := json.Unmarshal([]byte(tt.jsonInput), &input)

			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}
		})
	}
}

// TestURLEncodingUnit tests URL encoding for special characters (unit test)
func TestURLEncodingUnit(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedParts []string
		description   string
	}{
		{
			name:          "exclamation_mark_encoded",
			input:         "Password!123",
			expectedParts: []string{"Password", "%21", "123"},
			description:   "Exclamation mark should be encoded as %21",
		},
		{
			name:          "ampersand_encoded",
			input:         "Pass&word",
			expectedParts: []string{"Pass", "%26", "word"},
			description:   "Ampersand should be encoded as %26",
		},
		{
			name:          "plus_sign_encoded",
			input:         "Pass+word",
			expectedParts: []string{"Pass", "%2B", "word"},
			description:   "Plus sign should be encoded as %2B",
		},
		{
			name:          "equals_sign_encoded",
			input:         "Pass=word",
			expectedParts: []string{"Pass", "%3D", "word"},
			description:   "Equals sign should be encoded as %3D",
		},
		{
			name:          "space_encoded",
			input:         "Pass word",
			expectedParts: []string{"Pass", "+", "word"},
			description:   "Space should be encoded as +",
		},
		{
			name:          "at_sign_encoded",
			input:         "user@example.com",
			expectedParts: []string{"user", "%40", "example.com"},
			description:   "@ sign should be encoded as %40",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := url.QueryEscape(tt.input)

			// Check that the encoded string contains the expected parts
			for _, part := range tt.expectedParts {
				assert.Contains(t, encoded, part, tt.description)
			}

			// Verify we can decode it back
			decoded, err := url.QueryUnescape(encoded)
			assert.NoError(t, err, "Should be able to decode the encoded string")
			assert.Equal(t, tt.input, decoded, "Decoded string should match original")
		})
	}
}

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

// TestPasswordComplexity tests password validation scenarios (unit test)
func TestPasswordComplexity(t *testing.T) {
	specialCharPasswords := []string{
		"Password!123",
		"Pass@word#123",
		"Test$123%",
		"My^Pass&word*",
		"P@ssw0rd!",
		"C0mpl3x!P@ss",
	}

	for _, password := range specialCharPasswords {
		t.Run("url_encode_"+password, func(t *testing.T) {
			encoded := url.QueryEscape(password)

			// Verify encoding doesn't lose information
			decoded, err := url.QueryUnescape(encoded)
			assert.NoError(t, err)
			assert.Equal(t, password, decoded, "Password should survive encoding/decoding")

			// Verify encoded version is different from original (if special chars exist)
			if password != encoded {
				t.Logf("Original: %s, Encoded: %s", password, encoded)
			}
		})
	}
}

// TestEmailURLEncoding tests email encoding scenarios (unit test)
func TestEmailURLEncoding(t *testing.T) {
	emails := []string{
		"user@example.com",
		"user+tag@example.com",
		"user.name@example.com",
		"user_name@example.com",
		"user-name@example.com",
	}

	for _, email := range emails {
		t.Run("encode_"+email, func(t *testing.T) {
			encoded := url.QueryEscape(email)
			decoded, err := url.QueryUnescape(encoded)

			assert.NoError(t, err, "Should decode successfully")
			assert.Equal(t, email, decoded, "Email should survive encoding/decoding")
		})
	}
}

// BenchmarkURLQueryEscape benchmarks URL encoding performance
func BenchmarkURLQueryEscape(b *testing.B) {
	testStrings := []string{
		"SimplePassword123",
		"C0mpl3x!P@ss#w0rd$",
		"user+tag@example.com",
		"Very Long Password With Many Special Characters !@#$%^&*()",
	}

	for _, str := range testStrings {
		b.Run("encode_"+str[:10], func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = url.QueryEscape(str)
			}
		})
	}
}

// TestJSONUnmarshalPerformance benchmarks JSON unmarshaling
func BenchmarkLoginInputUnmarshal(b *testing.B) {
	jsonData := []byte(`{"email":"test@example.com","password":"Test123!"}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var input dto.LoginInput
		json.Unmarshal(jsonData, &input)
	}
}
