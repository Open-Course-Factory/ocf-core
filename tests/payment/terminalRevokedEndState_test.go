// tests/payment/terminalRevokedEndState_test.go
//
// RED tests for issue #388 (June payment go-live blocker #3). They drive the
// REAL billing-revocation cleanup path (services.TerminateUserTerminals and
// TerminateOrganizationMemberTerminals) and pin its NEW end-state contract:
// a terminal killed by a billing lapse / plan revocation must be marked
// State="revoked", NOT the generic "deleted" it currently uses (which the
// frontend renders as "Session Expired — time limit"; see issue #272 +
// ocf-front TerminalSessionView.vue:604).
//
// Budget invariant (SSOT: models.OccupiesSlotScope): "revoked" must FREE the
// slot and CPU/RAM budget exactly like the "deleted" it replaces — a cancelled
// subscription must not keep consuming a plan the user no longer has. The label
// is presentation; the budget transition is unchanged (not forked). We assert
// this explicitly so a future implementation cannot make "revoked" occupy a
// slot the way a user-initiated "stopped" does.
//
// The wire string "revoked" is pinned as a literal (not models.StateRevoked)
// so the package compiles and fails at RUNTIME (clean RED) before the constant
// exists. Green work should introduce models.StateRevoked = "revoked".
package payment_tests

import (
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/services"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTerminateUserTerminals_MarksRevoked_NotDeleted pins the user-account
// billing-revocation path (Stripe sub cancel, bulk-license revoke): a live
// terminal is marked "revoked", never the generic "deleted", AND the slot is
// freed (revoked is excluded from OccupiesSlotScope, same as deleted was).
func TestTerminateUserTerminals_MarksRevoked_NotDeleted(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-revoked-endstate"
	sessionID := uuid.New().String()

	db.Exec(
		`INSERT INTO terminals (id, session_id, user_id, name, state, expires_at) VALUES (?, ?, ?, ?, 'running', ?)`,
		uuid.New().String(), sessionID, userID, "term-revoked", time.Now().Add(time.Hour),
	)

	require.NoError(t, services.TerminateUserTerminals(db, userID, nil))

	var state string
	db.Raw("SELECT state FROM terminals WHERE session_id = ?", sessionID).Scan(&state)
	assert.Equal(t, "revoked", state,
		"a billing-revoked terminal must be State='revoked' so the frontend shows honest "+
			"revocation copy, not the 'Session Expired — time limit' banner")
	assert.NotEqual(t, "deleted", state,
		"revoked must be a DISTINCT end state from the generic TTL 'deleted'")

	// Budget invariant: the revoked terminal frees the slot exactly like the
	// 'deleted' it replaces — a cancelled subscription must not keep consuming
	// budget. This guards against 'revoked' being (mis)classified as occupying.
	occupied, err := terminalModels.CountUserOccupiedSlots(db, userID, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(0), occupied,
		"revoked must free the slot/budget (excluded from OccupiesSlotScope), same as deleted")
}

// TestTerminateOrganizationMemberTerminals_MarksRevoked pins the org-cancellation
// billing path: cancelling an org subscription revokes members' org terminals
// with the distinct "revoked" state (and frees their slots).
func TestTerminateOrganizationMemberTerminals_MarksRevoked(t *testing.T) {
	db := freshTestDB(t)

	orgID := uuid.New()
	org := &organizationModels.Organization{
		BaseModel:   entityManagementModels.BaseModel{ID: orgID},
		Name:        "revoked-endstate-org",
		DisplayName: "Revoked End State Org",
		OwnerUserID: "owner_user",
		IsActive:    true,
	}
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	userID := "org-member-revoked"
	member := &organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: orgID,
		UserID:         userID,
		Role:           organizationModels.OrgRoleMember,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	require.NoError(t, db.Omit("Metadata").Create(member).Error)

	sessionID := uuid.New().String()
	db.Exec(
		`INSERT INTO terminals (id, session_id, user_id, name, state, organization_id, expires_at) VALUES (?, ?, ?, ?, 'running', ?, ?)`,
		uuid.New().String(), sessionID, userID, "org-term", orgID, time.Now().Add(time.Hour),
	)

	services.TerminateOrganizationMemberTerminals(db, orgID)

	var state string
	db.Raw("SELECT state FROM terminals WHERE session_id = ?", sessionID).Scan(&state)
	assert.Equal(t, "revoked", state,
		"org-subscription cancellation must mark members' terminals 'revoked', not generic 'deleted'")

	occupied, err := terminalModels.CountUserOccupiedSlots(db, userID, &orgID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), occupied, "revoked org terminal must free the org slot")
}
