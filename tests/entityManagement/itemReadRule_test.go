package entityManagement_tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
)

// The table a MembershipConfig points at, created by hand like the real ones.
const createWorkspaceMembers = `CREATE TABLE workspace_members (
	owner_scope_entity_id TEXT, user_id TEXT, is_active BOOLEAN
)`

func registerMembershipScopedEntity(t *testing.T, db *gorm.DB) OwnerScopeEntity {
	t.Helper()
	registerOwnerScopeEntity(t)
	config := ems.GlobalEntityRegistrationService.GetOwnershipConfig("OwnerScopeEntity")
	config.Operations = nil
	ems.GlobalEntityRegistrationService.RegisterMembershipConfig("OwnerScopeEntity", &entityManagementInterfaces.MembershipConfig{
		MemberTable:    "workspace_members",
		EntityIDColumn: "owner_scope_entity_id",
		UserIDColumn:   "user_id",
		IsActiveColumn: "is_active",
	})
	require.NoError(t, db.Exec(createWorkspaceMembers).Error)
	row := seedOwnerScopeRow(t, db, "creator", "row")
	require.NoError(t, db.Exec(`INSERT INTO workspace_members VALUES (?, 'member', true), (?, 'left', false)`, row.ID, row.ID).Error)
	return row
}

func readRowAs(t *testing.T, db *gorm.DB, row OwnerScopeEntity, userID string, roles ...string) int {
	t.Helper()
	return doGET(ownerScopeRouter(db, userID, roles), "/api/v1/owner-scope-entities/"+row.ID.String()).Code
}

func TestGetEntity_MembershipScoped_OnlyMembersReadTheRow(t *testing.T) {
	db := setupOwnerScopeDB(t)
	row := registerMembershipScopedEntity(t, db)

	assert.Equal(t, http.StatusOK, readRowAs(t, db, row, "member", "member"))
	assert.Equal(t, http.StatusNotFound, readRowAs(t, db, row, "outsider", "member"))
	assert.Equal(t, http.StatusNotFound, readRowAs(t, db, row, "left", "member"), "an inactive membership grants nothing, as in the list")
	assert.Equal(t, http.StatusNotFound, readRowAs(t, db, row, "", "member"), "an unknown caller is refused")
	assert.Equal(t, http.StatusOK, readRowAs(t, db, row, "platform-admin", "administrator"))
}

func TestGetEntity_MembershipScoped_RefusalLooksLikeAMissingRow(t *testing.T) {
	db := setupOwnerScopeDB(t)
	row := registerMembershipScopedEntity(t, db)
	router := ownerScopeRouter(db, "outsider", []string{"member"})

	refused := doGET(router, "/api/v1/owner-scope-entities/"+row.ID.String())
	missing := doGET(router, "/api/v1/owner-scope-entities/00000000-0000-0000-0000-000000000001")

	assert.Equal(t, missing.Code, refused.Code)
	assert.NotContains(t, refused.Body.String(), "secret-of-creator")
}
