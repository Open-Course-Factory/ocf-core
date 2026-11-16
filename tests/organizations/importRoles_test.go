// An organization import must not assign platform roles: the platform
// administrator role is granted by platform operators, never by a users.csv.
package organizations_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"soli/formations/src/auth/access"
	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/models"
	"soli/formations/src/organizations/services"
	orgUtils "soli/formations/src/organizations/utils"
)

var platformRoleSpellings = []string{"admin", "Admin", "ADMIN", "administrator", "Administrator", " admin ", "\tAdministrator", `"  admin  "`}

func TestParseUsersCSV_PlatformRole_IsRejected(t *testing.T) {
	for _, role := range platformRoleSpellings {
		t.Run(role, func(t *testing.T) {
			fh := createMultipartFileHeader(t, "users.csv", "email,first_name,last_name,role\nada@test.com,Ada,Lovelace,"+role)
			_, errs, _ := orgUtils.ParseUsersCSV(fh)

			require.Len(t, errs, 1, "role %q must not be accepted in users.csv", role)
			assert.Equal(t, "role", errs[0].Field)
			assert.Equal(t, dto.ErrCodeInvalidRole, errs[0].Code)
		})
	}
}

func TestCsvImport_PlatformRole_NeverGrantedOnEnforcer(t *testing.T) {
	for _, role := range platformRoleSpellings {
		t.Run(role, func(t *testing.T) {
			enforcer := installMockEnforcer(t)
			db := newOffboardingDB(t)
			identity := newFakeIdentity()
			orgID := seedTeamOrg(t, db, "owner-1", new(10))
			seedMember(t, db, orgID, "owner-1", models.OrgRoleOwner)

			importer := services.NewImportService(db, identity, newOffboardingService(db, identity))
			_, _ = importer.ImportOrganizationData(orgID, "owner-1",
				usersCSV(t, "ada@example.com,Ada,Lovelace,"+role+"\n"), nil, nil, false, false, "", true)

			for _, call := range enforcer.AddGroupingPolicyCalls {
				granted, _ := call[1].(string)
				assert.False(t, access.IsAdmin([]string{granted}),
					"an import must not assign a platform role (grouping %v)", call)
			}
		})
	}
}
