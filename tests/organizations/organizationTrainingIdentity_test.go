package organizations_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/casdoor"
	authMocks "soli/formations/src/auth/mocks"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/organizations/dto"
	orgRegistration "soli/formations/src/organizations/entityRegistration"
)

func organizationOps(t *testing.T) entityManagementInterfaces.EntityOperations {
	t.Helper()
	enforcer := authMocks.NewMockEnforcer()
	enforcer.LoadPolicyFunc = func() error { return nil }
	enforcer.AddPolicyFunc = func(...any) (bool, error) { return true, nil }
	enforcer.GetFilteredPolicyFunc = func(int, ...string) ([][]string, error) { return [][]string{}, nil }
	original := casdoor.Enforcer
	casdoor.Enforcer = enforcer
	t.Cleanup(func() { casdoor.Enforcer = original })

	service := ems.NewEntityRegistrationService()
	orgRegistration.RegisterOrganization(service)
	ops, ok := service.GetEntityOps("Organization")
	require.True(t, ok)
	return ops
}

func TestOrganizationEdit_TrainingIdentity_CanBeSetAndCleared(t *testing.T) {
	ops := organizationOps(t)
	siret, cleared := "10299352400010", ""

	updates, err := ops.ConvertEditDtoToMap(dto.EditOrganizationInput{Siret: &siret, TrainingDeclarationNumber: &cleared})

	require.NoError(t, err)
	assert.Equal(t, siret, updates["siret"])
	assert.Equal(t, "", updates["training_declaration_number"], "an empty value clears it")
}

func TestOrganizationEdit_TrainingIdentityOmitted_IsLeftAlone(t *testing.T) {
	updates, err := organizationOps(t).ConvertEditDtoToMap(dto.EditOrganizationInput{})

	require.NoError(t, err)
	assert.NotContains(t, updates, "siret")
	assert.NotContains(t, updates, "training_declaration_number")
}
