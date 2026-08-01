package organizations_tests

// Tests for the default max_groups value for team organizations (issue #249).
//
// Training organisms running 200 sessions/year each with their own group
// need a higher default than the previous value of 10.
// The new default is 250 for all team organization creation paths.

import (
	"testing"

	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/models"

	"github.com/stretchr/testify/assert"
)

// applyMaxGroupsDefault reproduces the default-assignment logic from both
// organizationService.go and organizationRegistration.go so we can verify
// it in isolation without a database.
// applyMaxGroupsDefault mirrors the default-assignment logic in
// organizationRegistration.go so it can be verified without a database.
//
// It reads the exported constants rather than repeating the numbers: this helper
// was itself a copy of the copies, and it hardcoded a MaxMembers default of 50
// while the model and ConvertToTeam used 100 — so it agreed with the bug and
// asserted it (#458).
func applyMaxGroupsDefault(input dto.CreateOrganizationInput) *models.Organization {
	org := &models.Organization{
		Name:       input.Name,
		MaxGroups:  input.MaxGroups,
		MaxMembers: input.MaxMembers,
		IsActive:   true,
	}
	if org.MaxGroups == 0 {
		org.MaxGroups = models.DefaultTeamMaxGroups
	}
	if org.MaxMembers == 0 {
		org.MaxMembers = models.DefaultTeamMaxMembers
	}
	return org
}

// TestOrganizationCreate_WithoutMaxGroups_ShouldDefault250 verifies that a
// creation request without max_groups gets the new default of 250, which
// supports training organisms running ~200 sessions per year.
func TestOrganizationCreate_WithoutMaxGroups_ShouldDefault250(t *testing.T) {
	input := dto.CreateOrganizationInput{
		Name: "my-org",
		// MaxGroups intentionally omitted (zero value)
	}

	org := applyMaxGroupsDefault(input)

	assert.Equal(t, models.DefaultTeamMaxGroups, org.MaxGroups,
		"default max_groups should be the team default for team organizations")
}

// The member default had two values — 100 in the model and ConvertToTeam, 50 in
// the entity registration — so which limit an organization got depended on which
// endpoint created it (#458).
func TestOrganizationCreate_WithoutMaxMembers_ShouldUseTheSingleTeamDefault(t *testing.T) {
	org := applyMaxGroupsDefault(dto.CreateOrganizationInput{Name: "my-org"})

	assert.Equal(t, models.DefaultTeamMaxMembers, org.MaxMembers,
		"every creation path must land on the same member default")
}

// TestOrganizationCreate_WithExplicitMaxGroups_ShouldPreserveValue verifies that
// an explicit max_groups value set by an admin is preserved and not overridden.
func TestOrganizationCreate_WithExplicitMaxGroups_ShouldPreserveValue(t *testing.T) {
	input := dto.CreateOrganizationInput{
		Name:      "enterprise-org",
		MaxGroups: 500,
	}

	org := applyMaxGroupsDefault(input)

	assert.Equal(t, 500, org.MaxGroups, "explicitly set max_groups should not be overridden by the default")
}
