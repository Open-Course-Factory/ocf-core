// tests/auth/userSettings_recordingAck_test.go
package auth_tests

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/casdoor"
	authMocks "soli/formations/src/auth/mocks"
	authModels "soli/formations/src/auth/models"
	authRegistration "soli/formations/src/auth/entityRegistration"
	"soli/formations/src/auth/dto"
	ems "soli/formations/src/entityManagement/entityManagementService"
)

// setupRecordingAckTestService creates a fresh EntityRegistrationService with
// UserSettings registered and a no-op mock Casbin enforcer injected.
func setupRecordingAckTestService(t *testing.T) *ems.EntityRegistrationService {
	t.Helper()

	mockEnforcer := authMocks.NewMockEnforcer()
	mockEnforcer.LoadPolicyFunc = func() error { return nil }
	mockEnforcer.AddPolicyFunc = func(params ...any) (bool, error) { return true, nil }
	mockEnforcer.GetFilteredPolicyFunc = func(_ int, _ ...string) ([][]string, error) {
		return [][]string{}, nil
	}

	origEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	t.Cleanup(func() { casdoor.Enforcer = origEnforcer })

	svc := ems.NewEntityRegistrationService()
	authRegistration.RegisterUserSettings(svc)
	return svc
}

// TestUserSettings_ModelToDto_IncludesRecordingAcknowledgedAt verifies that a
// UserSettings model with RecordingAcknowledgedAt set is correctly reflected in
// the output DTO returned by the registered ModelToDto converter.
func TestUserSettings_ModelToDto_IncludesRecordingAcknowledgedAt(t *testing.T) {
	svc := setupRecordingAckTestService(t)

	ops, ok := svc.GetEntityOps("UserSetting")
	require.True(t, ok, "UserSettings must be registered")

	now := time.Now().UTC().Truncate(time.Second)
	model := &authModels.UserSettings{
		UserID:                    "user-ack-test",
		Theme:                     "dark",
		RecordingAcknowledgedAt:   &now,
	}

	rawDto, err := ops.ConvertModelToDto(model)
	require.NoError(t, err)

	output, ok := rawDto.(dto.UserSettingsOutput)
	require.True(t, ok, "output must be a UserSettingsOutput")

	require.NotNil(t, output.RecordingAcknowledgedAt,
		"RecordingAcknowledgedAt must be non-nil in output DTO")
	assert.Equal(t, now, output.RecordingAcknowledgedAt.UTC().Truncate(time.Second))
}

// TestUserSettings_DtoToMap_AcceptsRecordingAcknowledgedAt verifies that an
// EditUserSettingsInput containing RecordingAcknowledgedAt produces a map entry
// with key "recording_acknowledged_at" via the registered DtoToMap converter.
func TestUserSettings_DtoToMap_AcceptsRecordingAcknowledgedAt(t *testing.T) {
	svc := setupRecordingAckTestService(t)

	ops, ok := svc.GetEntityOps("UserSetting")
	require.True(t, ok, "UserSettings must be registered")

	now := time.Now().UTC().Truncate(time.Second)
	editDto := dto.EditUserSettingsInput{
		RecordingAcknowledgedAt: &now,
	}

	updateMap, err := ops.ConvertEditDtoToMap(editDto)
	require.NoError(t, err)

	val, present := updateMap["recording_acknowledged_at"]
	assert.True(t, present, "map must contain key 'recording_acknowledged_at'")
	assert.Equal(t, now, val)
}

// TestUserSettings_Model_HasRecordingAcknowledgedAtField verifies via reflection
// that the UserSettings model struct has a field named RecordingAcknowledgedAt
// of type *time.Time, catching accidental renames.
func TestUserSettings_Model_HasRecordingAcknowledgedAtField(t *testing.T) {
	modelType := reflect.TypeOf(authModels.UserSettings{})

	field, found := modelType.FieldByName("RecordingAcknowledgedAt")
	assert.True(t, found, "UserSettings must have a field named RecordingAcknowledgedAt")

	if found {
		expectedType := reflect.TypeOf((*time.Time)(nil))
		assert.Equal(t, expectedType, field.Type,
			"RecordingAcknowledgedAt must be of type *time.Time")
	}
}
