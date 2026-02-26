package organizations_tests

import (
	"encoding/json"
	"testing"

	"soli/formations/src/organizations/dto"

	"github.com/stretchr/testify/assert"
)

func TestRegeneratePasswordsRequest_Serialization(t *testing.T) {
	request := dto.RegeneratePasswordsRequest{
		UserIDs: []string{"user-1", "user-2", "user-3"},
	}

	data, err := json.Marshal(request)
	assert.NoError(t, err)

	var decoded dto.RegeneratePasswordsRequest
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, request.UserIDs, decoded.UserIDs)
}

func TestRegeneratePasswordsResponse_SuccessCase(t *testing.T) {
	response := dto.RegeneratePasswordsResponse{
		Success: true,
		Credentials: []dto.UserCredential{
			{Email: "user1@example.com", Password: "newpass1", Name: "User One"},
			{Email: "user2@example.com", Password: "newpass2", Name: "User Two"},
		},
		Errors: []dto.ImportError{},
		Summary: dto.RegeneratePasswordsSummary{
			Total:     2,
			Succeeded: 2,
			Failed:    0,
		},
	}

	data, err := json.Marshal(response)
	assert.NoError(t, err)

	var decoded dto.RegeneratePasswordsResponse
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.True(t, decoded.Success)
	assert.Len(t, decoded.Credentials, 2)
	assert.Empty(t, decoded.Errors)
	assert.Equal(t, 2, decoded.Summary.Total)
	assert.Equal(t, 2, decoded.Summary.Succeeded)
	assert.Equal(t, 0, decoded.Summary.Failed)
}

func TestRegeneratePasswordsResponse_PartialFailure(t *testing.T) {
	response := dto.RegeneratePasswordsResponse{
		Success: false,
		Credentials: []dto.UserCredential{
			{Email: "user1@example.com", Password: "newpass1", Name: "User One"},
		},
		Errors: []dto.ImportError{
			{
				Row:     2,
				File:    "user_ids",
				Field:   "user_id",
				Message: "user user-2 is not a member of this group",
				Code:    dto.ErrCodeNotFound,
			},
		},
		Summary: dto.RegeneratePasswordsSummary{
			Total:     2,
			Succeeded: 1,
			Failed:    1,
		},
	}

	data, err := json.Marshal(response)
	assert.NoError(t, err)

	var decoded dto.RegeneratePasswordsResponse
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.False(t, decoded.Success)
	assert.Len(t, decoded.Credentials, 1)
	assert.Len(t, decoded.Errors, 1)
	assert.Equal(t, dto.ErrCodeNotFound, decoded.Errors[0].Code)
	assert.Equal(t, 1, decoded.Summary.Failed)
}

func TestRegeneratePasswordsSummary_Serialization(t *testing.T) {
	summary := dto.RegeneratePasswordsSummary{
		Total:     5,
		Succeeded: 3,
		Failed:    2,
	}

	data, err := json.Marshal(summary)
	assert.NoError(t, err)

	var decoded dto.RegeneratePasswordsSummary
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, 5, decoded.Total)
	assert.Equal(t, 3, decoded.Succeeded)
	assert.Equal(t, 2, decoded.Failed)
}
