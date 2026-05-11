// tests/terminalTrainer/countOccupiedSlots_test.go
//
// Failing-first tests for the next SSOT step: two count helpers in
// src/terminalTrainer/models/terminal.go that wrap the OccupiesSlotScope
// predicate so every caller becomes a one-liner.
//
//	func CountUserOccupiedSlots(db *gorm.DB, userID string, orgID *uuid.UUID) (int64, error)
//	func CountOrgOccupiedSlots(db *gorm.DB, orgID uuid.UUID) (int64, error)
//
// Today the predicate is reused (good), but the "filter + count" dance is
// open-coded in 4+ places (effectivePlanService, userSubscriptionService x3,
// organizationSubscriptionService). The action itself must also be a single
// source of truth — otherwise the next call site will drift again.
//
// Investigation findings used to design these tests:
//
//   - terminals.organization_id is a DIRECT column on the terminals table
//     (terminal.go:54). So user+org scoping is a simple
//     `WHERE user_id = ? AND organization_id = ?` (see userSubscriptionService.go:328-333,
//     GetUserUsageMetrics with orgID). No JOIN through organization_members
//     for the user-scoped count.
//
//   - Org-scoped count (CountOrgOccupiedSlots) MUST JOIN organization_members
//     because terminals may be created by any member of the org (matches
//     organizationSubscriptionService.go:265-269 / GetOrganizationUsageLimits).
//     Mirroring that semantics — not the simpler `WHERE terminals.organization_id = ?`
//     used by GetOrgTerminalUsage at terminalTrainerService.go:1791-1795. The
//     two existing sites have different semantics; the helper aligns with the
//     plan-limit semantics (used to gate quota), which is what callers want.
//
//   - OccupiesSlotScope filters `status IN ('active','stopped') AND deleted_at IS NULL`.
//     It does NOT filter on expires_at. So a row with status='expired' (set in
//     terminalTrainerService.go:577 when the upstream session is gone) is
//     excluded; a row with status='active' but ExpiresAt in the past is NOT
//     excluded by the SSOT. The "expired" tests below use status='expired'
//     (the lifecycle-correct way), not just a past ExpiresAt.
package terminalTrainer_tests

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	orgModels "soli/formations/src/organizations/models"
	"soli/formations/src/terminalTrainer/models"
)

// seedTerminal inserts a Terminal row with explicit lifecycle fields.
// status is what OccupiesSlotScope filters on. softDelete=true sets
// deleted_at via db.Delete() (the gorm-native soft delete path).
func seedTerminal(t *testing.T, db *gorm.DB, userID, status string, orgID *uuid.UUID, softDelete bool) *models.Terminal {
	t.Helper()
	userKey, err := createTestUserKey(db, userID+"-"+uuid.New().String()[:6])
	require.NoError(t, err)

	terminal := &models.Terminal{
		SessionID:         "seed-session-" + uuid.New().String(),
		UserID:            userID,
		Name:              "Seed Terminal",
		Status:            status,
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
		OrganizationID:    orgID,
	}
	require.NoError(t, db.Create(terminal).Error)

	if softDelete {
		require.NoError(t, db.Delete(terminal).Error)
	}
	return terminal
}

// TestCountUserOccupiedSlots covers the user-scoped count helper without
// an org filter (orgID == nil).
func TestCountUserOccupiedSlots(t *testing.T) {
	cases := []struct {
		name     string
		seed     func(t *testing.T, db *gorm.DB, userID string)
		expected int64
	}{
		{
			name: "single active terminal counts as 1",
			seed: func(t *testing.T, db *gorm.DB, userID string) {
				seedTerminal(t, db, userID, "active", nil, false)
			},
			expected: 1,
		},
		{
			name: "single stopped terminal still occupies a slot (SSOT rule)",
			seed: func(t *testing.T, db *gorm.DB, userID string) {
				seedTerminal(t, db, userID, "stopped", nil, false)
			},
			expected: 1,
		},
		{
			name: "active + stopped both count",
			seed: func(t *testing.T, db *gorm.DB, userID string) {
				seedTerminal(t, db, userID, "active", nil, false)
				seedTerminal(t, db, userID, "stopped", nil, false)
			},
			expected: 2,
		},
		{
			name: "expired status is excluded",
			seed: func(t *testing.T, db *gorm.DB, userID string) {
				seedTerminal(t, db, userID, "active", nil, false)
				seedTerminal(t, db, userID, "expired", nil, false)
			},
			expected: 1,
		},
		{
			name: "soft-deleted row is excluded",
			seed: func(t *testing.T, db *gorm.DB, userID string) {
				seedTerminal(t, db, userID, "active", nil, false)
				seedTerminal(t, db, userID, "active", nil, true)
			},
			expected: 1,
		},
		{
			name: "active + stopped + expired + deleted → 2",
			seed: func(t *testing.T, db *gorm.DB, userID string) {
				seedTerminal(t, db, userID, "active", nil, false)
				seedTerminal(t, db, userID, "stopped", nil, false)
				seedTerminal(t, db, userID, "expired", nil, false)
				seedTerminal(t, db, userID, "active", nil, true)
			},
			expected: 2,
		},
		{
			name:     "user with no terminals returns 0",
			seed:     func(t *testing.T, db *gorm.DB, userID string) {},
			expected: 0,
		},
		{
			name: "different user's terminals are not counted (scoping)",
			seed: func(t *testing.T, db *gorm.DB, userID string) {
				// Seed terminals for somebody else, none for userID.
				seedTerminal(t, db, "someone-else", "active", nil, false)
				seedTerminal(t, db, "someone-else", "stopped", nil, false)
			},
			expected: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := freshTestDB(t)
			userID := "count-user-" + uuid.New().String()[:8]
			tc.seed(t, db, userID)

			got, err := models.CountUserOccupiedSlots(db, userID, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, got,
				"CountUserOccupiedSlots(%q, nil) must match the SSOT predicate", userID)
		})
	}
}

// TestCountUserOccupiedSlots_OrgScoped exercises the orgID != nil path.
// Contract mirrors GetUserUsageMetrics(userID, orgID) at
// userSubscriptionService.go:331-333: scoping by direct
// terminals.organization_id column, not by member join.
func TestCountUserOccupiedSlots_OrgScoped(t *testing.T) {
	db := freshTestDB(t)
	userID := "scoped-user"

	orgA := createTestOrgForHistory(t, db, "owner-a")
	orgB := createTestOrgForHistory(t, db, "owner-b")

	// User has 2 active terminals tied to org A, 1 active tied to org B,
	// and 1 active with no org (personal). 4 rows total occupying slots.
	seedTerminal(t, db, userID, "active", &orgA.ID, false)
	seedTerminal(t, db, userID, "active", &orgA.ID, false)
	seedTerminal(t, db, userID, "active", &orgB.ID, false)
	seedTerminal(t, db, userID, "active", nil, false)

	// orgID == nil → counts all 4 terminals owned by this user.
	gotAll, err := models.CountUserOccupiedSlots(db, userID, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(4), gotAll,
		"orgID=nil must count every slot-occupying terminal owned by the user")

	// orgID == orgA → only the 2 terminals tied to org A.
	gotA, err := models.CountUserOccupiedSlots(db, userID, &orgA.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), gotA,
		"orgID=A must count only terminals with terminals.organization_id = A")

	// orgID == orgB → only the 1 terminal tied to org B.
	gotB, err := models.CountUserOccupiedSlots(db, userID, &orgB.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), gotB,
		"orgID=B must count only terminals with terminals.organization_id = B")
}

// TestCountOrgOccupiedSlots covers the org-scoped count helper. The contract
// mirrors GetOrganizationUsageLimits at organizationSubscriptionService.go:265-269:
// JOIN organization_members ON organization_members.user_id = terminals.user_id
// WHERE organization_members.organization_id = orgID.
//
// Rationale: org plan quotas (MaxConcurrentTerminals) constrain "terminals
// launched by any member of this org", which the join captures even if a
// member's terminal row doesn't carry the organization_id column.
func TestCountOrgOccupiedSlots(t *testing.T) {
	t.Run("two members each with one active terminal → 2", func(t *testing.T) {
		db := freshTestDB(t)
		org := createTestOrgForHistory(t, db, "owner-1")
		createTestOrgMember(t, db, org.ID, "owner-1", orgModels.OrgRoleOwner)
		createTestOrgMember(t, db, org.ID, "member-1", orgModels.OrgRoleMember)

		seedTerminal(t, db, "owner-1", "active", &org.ID, false)
		seedTerminal(t, db, "member-1", "active", &org.ID, false)

		got, err := models.CountOrgOccupiedSlots(db, org.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(2), got)
	})

	t.Run("two members, one active + one stopped → 2", func(t *testing.T) {
		db := freshTestDB(t)
		org := createTestOrgForHistory(t, db, "owner-2")
		createTestOrgMember(t, db, org.ID, "owner-2", orgModels.OrgRoleOwner)
		createTestOrgMember(t, db, org.ID, "member-2", orgModels.OrgRoleMember)

		seedTerminal(t, db, "owner-2", "active", &org.ID, false)
		seedTerminal(t, db, "member-2", "stopped", &org.ID, false)

		got, err := models.CountOrgOccupiedSlots(db, org.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(2), got,
			"stopped terminals must still count against org quota (SSOT)")
	})

	t.Run("one member with active + expired → 1", func(t *testing.T) {
		db := freshTestDB(t)
		org := createTestOrgForHistory(t, db, "owner-3")
		createTestOrgMember(t, db, org.ID, "owner-3", orgModels.OrgRoleOwner)

		seedTerminal(t, db, "owner-3", "active", &org.ID, false)
		seedTerminal(t, db, "owner-3", "expired", &org.ID, false)

		got, err := models.CountOrgOccupiedSlots(db, org.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(1), got, "expired terminals must not occupy a slot")
	})

	t.Run("org with zero members → 0", func(t *testing.T) {
		db := freshTestDB(t)
		org := createTestOrgForHistory(t, db, "owner-4")
		// No createTestOrgMember calls — org has no members.

		got, err := models.CountOrgOccupiedSlots(db, org.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(0), got)
	})

	t.Run("non-member with terminal pointing at org → 0 (join enforces membership)", func(t *testing.T) {
		db := freshTestDB(t)
		org := createTestOrgForHistory(t, db, "owner-5")
		// Owner is NOT registered as an OrganizationMember row — only the
		// Organization.OwnerUserID field is set. The contract is that the
		// helper relies on organization_members, so this terminal must not
		// be counted.
		seedTerminal(t, db, "lone-wolf", "active", &org.ID, false)

		got, err := models.CountOrgOccupiedSlots(db, org.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(0), got,
			"non-members must not contribute to the org count, even if "+
				"the terminal carries organization_id — the join through "+
				"organization_members is the source of truth")
	})
}
