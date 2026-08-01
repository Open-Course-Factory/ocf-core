package organizations_tests

// #458: three defects with one cause — personal organizations are a documented
// product concept that almost nothing enforced.
//
//  1. An OrganizationSubscription assigned to a personal org was silently
//     ignored: plan resolution short-circuits personal orgs to the user's own
//     subscription, so the row existed and was never read.
//  2. ConvertToTeam checked ownership only, so a user with no classroom plan
//     could create a team organization they cannot use.
//  3. MaxMembers had two defaults, 100 in the model and ConvertToTeam, 50 in the
//     entity registration — so the limit depended on which endpoint created the
//     organization.

import (
	"reflect"
	"strings"
	"testing"

	organizationModels "soli/formations/src/organizations/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The GORM `default:` struct tags cannot interpolate constants, so they repeat
// the numbers. This pins them to the constants so the two cannot drift — which is
// the whole failure mode #458 is about.
func TestOrganizationDefaults_StructTagsMatchTheConstants(t *testing.T) {
	typ := reflect.TypeOf(organizationModels.Organization{})

	cases := []struct {
		field string
		want  int
	}{
		{"MaxGroups", organizationModels.DefaultTeamMaxGroups},
		{"MaxMembers", organizationModels.DefaultTeamMaxMembers},
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			field, found := typ.FieldByName(tc.field)
			require.True(t, found, "Organization must have a %s field", tc.field)

			tag := field.Tag.Get("gorm")
			assert.Containsf(t, tag, "default:"+itoa(tc.want),
				"the %s column default must match the exported constant (%d); "+
					"tags cannot reference constants, so this test is the only thing "+
					"keeping them in step", tc.field, tc.want)
		})
	}
}

// A personal organization is for one person. The constant and the model must say
// the same thing as the product does.
func TestPersonalOrganizationLimits_AllowOnlyTheOwner(t *testing.T) {
	assert.Equal(t, 1, organizationModels.PersonalMaxMembers,
		"a personal workspace holds its owner and nobody else")
}

func TestOrganizationTypePredicates_AreMutuallyExclusive(t *testing.T) {
	personal := &organizationModels.Organization{OrganizationType: organizationModels.OrgTypePersonal}
	team := &organizationModels.Organization{OrganizationType: organizationModels.OrgTypeTeam}

	assert.True(t, personal.IsPersonalOrg())
	assert.False(t, personal.IsTeamOrg())
	assert.True(t, team.IsTeamOrg())
	assert.False(t, team.IsPersonalOrg())
}

// An organization with an unset type must not be treated as personal: the
// BeforeSave hook normalises anything unrecognised to team, and a zero-value
// struct read as "personal" would make the guards refuse legitimate work.
func TestOrganizationTypePredicates_ZeroValueIsNotPersonal(t *testing.T) {
	var zero organizationModels.Organization

	assert.False(t, zero.IsPersonalOrg(),
		"an unset organization type must not read as personal — the guards refuse personal orgs")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return strings.TrimSpace(string(digits))
}
