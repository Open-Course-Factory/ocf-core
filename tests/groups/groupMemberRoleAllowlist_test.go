package groups_tests

// #429 on the group side: the create hook capped the assigned role at the
// granter's own rank but never checked the value was a role at all, and the
// platform-admin bypass of the cap let "garbage" through. GroupMember has no
// PATCH route, so create is the only write that carries a role.

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	entityErrors "soli/formations/src/entityManagement/errors"
	groupModels "soli/formations/src/groups/models"
)

func TestGroupMemberCreateHook_AdminAssignsUnknownRole_Rejected(t *testing.T) {
	err := runGroupCreateHookRoleCapCase(t, []string{"administrator"}, groupModels.GroupMemberRole("garbage"))

	require.Error(t, err)
	var structured *entityErrors.EntityError
	require.True(t, errors.As(err, &structured), "expected a structured entity error, got %T: %v", err, err)
	assert.Equal(t, http.StatusBadRequest, structured.HTTPStatus, "an unknown role is a client error: %v", err)
}
