package groups_tests

// Deleting an organization archives its classes and stands their rosters
// down, leaving them to platform administrators. The class creator is one of
// those former staff: their authority over the class came through the
// organization, so it ends with it. A class with no organization keeps its
// creator.

import (
	"net/http"
	"testing"

	access "soli/formations/src/auth/access"
	"soli/formations/src/entityManagement/hooks"
	groupHooks "soli/formations/src/groups/hooks"
	groupModels "soli/formations/src/groups/models"
	orgHooks "soli/formations/src/organizations/hooks"
	orgModels "soli/formations/src/organizations/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const deletedOrgCreator = "class-creator-teacher"

// seedOrgClass seeds a team organization and a class in it created by a
// teacher who is a plain member of the organization (no manager rights).
func seedOrgClass(t *testing.T, env *classArchiveEnv) (orgModels.Organization, groupModels.ClassGroup) {
	t.Helper()
	org := seedOrganization(t, env.db, "org-owner")
	seedOrgMember(t, env.db, org.ID, deletedOrgCreator, orgModels.OrgRoleMember, true)
	group := seedClass(t, env.db, "promo", deletedOrgCreator, &org.ID)
	seedClassMember(t, env.db, group.ID, deletedOrgCreator, groupModels.GroupMemberRoleOwner)
	seedClassMember(t, env.db, group.ID, "student", groupModels.GroupMemberRoleMember)
	return org, group
}

// deleteOrg runs the production cascade (OrganizationCleanupHook on
// BeforeDelete, which archives the classes and deactivates the rosters), then
// soft-deletes the row the way the generic delete does.
func deleteOrg(t *testing.T, env *classArchiveEnv, org orgModels.Organization) {
	t.Helper()
	require.NoError(t, orgHooks.NewOrganizationCleanupHook(env.db).Execute(&hooks.HookContext{
		EntityName: "Organization", HookType: hooks.BeforeDelete, NewEntity: &org,
	}))
	require.NoError(t, env.db.Delete(&orgModels.Organization{}, "id = ?", org.ID).Error)
}

func classPath(groupID uuid.UUID) string { return classGroupsPath + "/" + groupID.String() }

func managedClassIDs(t *testing.T, env *classArchiveEnv, userID string) []uuid.UUID {
	t.Helper()
	var ids []uuid.UUID
	require.NoError(t, env.db.Model(&groupModels.ClassGroup{}).
		Scopes(groupModels.ManagedByScope(userID)).Pluck("class_groups.id", &ids).Error)
	return ids
}

func TestClassOfDeletedOrganization_CreatorEditsClass_Refused(t *testing.T) {
	env := setupClassArchiveEnv(t)
	org, group := seedOrgClass(t, env)
	deleteOrg(t, env, org)

	env.as(deletedOrgCreator)
	rec := env.do(http.MethodPatch, classPath(group.ID), map[string]any{"display_name": "renamed"})
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Equal(t, "promo", reloadClass(t, env.db, group.ID).DisplayName)
}

func TestClassOfDeletedOrganization_CreatorDeletesClass_Refused(t *testing.T) {
	env := setupClassArchiveEnv(t)
	org, group := seedOrgClass(t, env)
	deleteOrg(t, env, org)

	env.as(deletedOrgCreator)
	rec := env.do(http.MethodDelete, classPath(group.ID), nil)
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	var count int64
	env.db.Model(&groupModels.ClassGroup{}).Where("id = ?", group.ID).Count(&count)
	assert.EqualValues(t, 1, count, "a refused delete must leave the class in place")
}

func TestClassOfDeletedOrganization_CreatorUnarchivesClass_Refused(t *testing.T) {
	env := setupClassArchiveEnv(t)
	org, group := seedOrgClass(t, env)
	deleteOrg(t, env, org)
	require.NotNil(t, reloadClass(t, env.db, group.ID).ArchivedAt, "precondition: the cascade archived the class")

	env.as(deletedOrgCreator)
	rec := env.do(http.MethodPost, classPath(group.ID)+"/unarchive", nil)
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.NotNil(t, reloadClass(t, env.db, group.ID).ArchivedAt, "a refused unarchive must leave the class archived")
}

// The archive alone would refuse the enrolment (409), so an administrator
// reopens the class first; the creator must still be refused, for authority.
// The enrolment authority hook is run directly: the generic create cannot
// write a GroupMember on SQLite (metadata map), so an HTTP 201 is unreachable.
func TestClassOfDeletedOrganization_CreatorAddsMember_Refused(t *testing.T) {
	env := setupClassArchiveEnv(t)
	org, group := seedOrgClass(t, env)
	deleteOrg(t, env, org)

	env.as("platform-admin", access.RoleAdministrator)
	rec := env.do(http.MethodPost, classPath(group.ID)+"/unarchive", nil)
	require.Equal(t, http.StatusOK, rec.Code, "an administrator still manages the class: %s", rec.Body.String())

	err := groupHooks.NewGroupMemberValidationHook(env.db).Execute(&hooks.HookContext{
		EntityName: "GroupMember",
		HookType:   hooks.BeforeCreate,
		NewEntity:  &groupModels.GroupMember{GroupID: group.ID, UserID: "newcomer", Role: groupModels.GroupMemberRoleMember},
		UserID:     deletedOrgCreator,
		UserRoles:  platformMember,
	})
	assert.Error(t, err, "the creator of a class of a deleted organization must not enrol members")
}

func TestClassOfDeletedOrganization_ManagedByScope_ExcludesCreator(t *testing.T) {
	env := setupClassArchiveEnv(t)
	org, group := seedOrgClass(t, env)
	require.Contains(t, managedClassIDs(t, env, deletedOrgCreator), group.ID, "control: the creator manages the class while the organization lives")

	deleteOrg(t, env, org)
	assert.NotContains(t, managedClassIDs(t, env, deletedOrgCreator), group.ID,
		"a class of a deleted organization is no longer the creator's to manage")
}

// Controls: these pass today and must keep passing.

func TestClassOfLiveOrganization_CreatorEditsClass_Allowed(t *testing.T) {
	env := setupClassArchiveEnv(t)
	_, group := seedOrgClass(t, env)

	env.as(deletedOrgCreator)
	rec := env.do(http.MethodPatch, classPath(group.ID), map[string]any{"display_name": "renamed"})
	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
}

func TestClassWithoutOrganization_CreatorManagesClass_Allowed(t *testing.T) {
	env := setupClassArchiveEnv(t)
	group := seedClass(t, env.db, "personal", deletedOrgCreator, nil)
	seedClassMember(t, env.db, group.ID, deletedOrgCreator, groupModels.GroupMemberRoleOwner)

	env.as(deletedOrgCreator)
	rec := env.do(http.MethodPatch, classPath(group.ID), map[string]any{"display_name": "renamed"})
	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Contains(t, managedClassIDs(t, env, deletedOrgCreator), group.ID)
}

func TestClassOfDeletedOrganization_PlatformAdminEditsClass_Allowed(t *testing.T) {
	env := setupClassArchiveEnv(t)
	org, group := seedOrgClass(t, env)
	deleteOrg(t, env, org)

	env.as("platform-admin", access.RoleAdministrator)
	rec := env.do(http.MethodPatch, classPath(group.ID), map[string]any{"display_name": "renamed"})
	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
}

// The creator branch alone (no roster seat, no organization membership) holds
// while the organization lives.
func TestClassOfLiveOrganization_CreatorOutsideOrganization_ManagesClass(t *testing.T) {
	env := setupClassArchiveEnv(t)
	org := seedOrganization(t, env.db, "org-owner")
	group := seedClass(t, env.db, "promo", deletedOrgCreator, &org.ID)

	assert.Contains(t, managedClassIDs(t, env, deletedOrgCreator), group.ID)
}

// A later organization, even one the creator manages, does not revive the
// creator's authority over a class of the deleted one.
func TestClassOfDeletedOrganization_NewOrganizationCreated_CreatorStillRefused(t *testing.T) {
	env := setupClassArchiveEnv(t)
	org, group := seedOrgClass(t, env)
	deleteOrg(t, env, org)
	other := seedOrganization(t, env.db, deletedOrgCreator)
	seedOrgMember(t, env.db, other.ID, deletedOrgCreator, orgModels.OrgRoleOwner, true)

	env.as(deletedOrgCreator)
	rec := env.do(http.MethodPatch, classPath(group.ID), map[string]any{"display_name": "renamed"})
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.NotContains(t, managedClassIDs(t, env, deletedOrgCreator), group.ID)
}

// Fail closed: a class pointing at an organization id with no row at all is
// treated like one of a deleted organization.
func TestClassOfMissingOrganization_CreatorRefused(t *testing.T) {
	env := setupClassArchiveEnv(t)
	missing := uuid.New()
	group := seedClass(t, env.db, "orphan", deletedOrgCreator, &missing)

	env.as(deletedOrgCreator)
	rec := env.do(http.MethodPatch, classPath(group.ID), map[string]any{"display_name": "renamed"})
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.NotContains(t, managedClassIDs(t, env, deletedOrgCreator), group.ID)
}
