package entityManagement_tests

// Tests for #410 (audit-P, MR P): the generic memberManagementService.UpdateMemberRole
// must cap the role a granter can assign at the granter's own rank. Today the method checks
// that the granter can *manage* the entity (owner or manager member), then updates the role
// WITHOUT capping it — so a manager could promote a member to owner. This service is
// currently unrouted, but any future wiring would reopen M4.
//
// Rule to enforce, after the canUserManage gate and before the write:
//
//	granter role >= newRole   (access.IsRoleAtLeast, owner100 > manager50 > member10)
//
// with an owner short-circuit: when entity.GetOwnerUserID() == requestingUserID the cap is
// skipped.
//
// The service works against generic MemberRepository / PermissionManager interfaces, so
// these tests use in-memory fakes (no DB, no Casbin) and assert the returned error AND the
// role persisted in the fake repository.

import (
	"fmt"
	"testing"
	"time"

	mmsvc "soli/formations/src/entityManagement/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// --- Fakes implementing the generic member-management interfaces ---

type fakeMemberEntity struct {
	id    uuid.UUID
	owner string
}

func (e fakeMemberEntity) GetID() uuid.UUID       { return e.id }
func (e fakeMemberEntity) GetOwnerUserID() string  { return e.owner }
func (e fakeMemberEntity) GetMaxMembers() int      { return 100 }
func (e fakeMemberEntity) GetCurrentMemberCount() int { return 0 }
func (e fakeMemberEntity) IsExpired() bool         { return false }
func (e fakeMemberEntity) IsActive() bool          { return true }

type fakeMember struct {
	userID string
	role   string
}

func (m *fakeMember) GetUserID() string      { return m.userID }
func (m *fakeMember) GetRole() string        { return m.role }
func (m *fakeMember) GetJoinedAt() time.Time { return time.Now() }
func (m *fakeMember) IsActive() bool         { return true }

type fakeMemberRepo struct {
	entity  mmsvc.MemberEntity
	members map[string]*fakeMember // userID -> member row
}

func (r *fakeMemberRepo) GetEntityByID(entityID uuid.UUID) (mmsvc.MemberEntity, error) {
	return r.entity, nil
}

func (r *fakeMemberRepo) AddMember(member mmsvc.Member) error {
	r.members[member.GetUserID()] = &fakeMember{userID: member.GetUserID(), role: member.GetRole()}
	return nil
}

func (r *fakeMemberRepo) RemoveMember(entityID uuid.UUID, userID string) error {
	delete(r.members, userID)
	return nil
}

func (r *fakeMemberRepo) UpdateMemberRole(entityID uuid.UUID, userID string, newRole string) error {
	m, ok := r.members[userID]
	if !ok {
		return fmt.Errorf("member %s not found", userID)
	}
	m.role = newRole
	return nil
}

// GetMember returns an untyped nil interface on miss so the service's `member == nil` check
// works (a typed nil pointer would defeat it).
func (r *fakeMemberRepo) GetMember(entityID uuid.UUID, userID string) (mmsvc.Member, error) {
	m, ok := r.members[userID]
	if !ok {
		return nil, fmt.Errorf("member %s not found", userID)
	}
	return m, nil
}

func (r *fakeMemberRepo) GetMembers(entityID uuid.UUID) ([]mmsvc.Member, error) {
	out := make([]mmsvc.Member, 0, len(r.members))
	for _, m := range r.members {
		out = append(out, m)
	}
	return out, nil
}

// noopPermissionManager satisfies the PermissionManager dependency without touching Casbin.
type noopPermissionManager struct{}

func (noopPermissionManager) GrantEntityPermissions(userID, entityType string, entityID uuid.UUID, methods []string) error {
	return nil
}
func (noopPermissionManager) RevokeEntityPermissions(userID, entityType string, entityID uuid.UUID) error {
	return nil
}

// buildMMS wires the service with an entity owned by ownerID and the given seeded member
// rows (userID -> role). Returns the service and the fake repo (for reading back roles).
func buildMMS(ownerID string, seeded map[string]string) (mmsvc.MemberManagementService, *fakeMemberRepo) {
	entity := fakeMemberEntity{id: uuid.New(), owner: ownerID}
	repo := &fakeMemberRepo{
		entity:  entity,
		members: map[string]*fakeMember{},
	}
	for userID, role := range seeded {
		repo.members[userID] = &fakeMember{userID: userID, role: role}
	}
	config := mmsvc.MemberConfig{
		EntityType:   "group",
		RoleOwner:    "owner",
		RoleManager:  "manager",
		AllowedRoles: []string{"owner", "manager", "member"},
	}
	svc := mmsvc.NewMemberManagementService(repo, noopPermissionManager{}, config)
	return svc, repo
}

// TestMemberManagementUpdateMemberRole_ManagerPromotesToOwner_Rejected: a manager granter
// must NOT promote a member to owner. Expected RED today (no cap → the update succeeds and
// the role becomes owner).
func TestMemberManagementUpdateMemberRole_ManagerPromotesToOwner_Rejected(t *testing.T) {
	const (
		ownerID   = "entity-owner-account"
		granterID = "manager-granter"
		targetID  = "role-cap-target"
	)

	svc, repo := buildMMS(ownerID, map[string]string{
		granterID: "manager",
		targetID:  "member",
	})
	entityID := repo.entity.GetID()

	err := svc.UpdateMemberRole(entityID, granterID, targetID, "owner")

	require.Error(t, err,
		"a manager must not promote a member to owner (owner100 > manager50); "+
			"UpdateMemberRole must reject, but it returned nil")
	require.Equal(t, "member", repo.members[targetID].role,
		"the rejected promotion must NOT persist — target role must remain member, got %q",
		repo.members[targetID].role)
}

// TestMemberManagementUpdateMemberRole_ManagerSetsRoleAtOrBelowOwn_Allowed: a manager may
// set manager (equal) and member (below). Green guard.
func TestMemberManagementUpdateMemberRole_ManagerSetsRoleAtOrBelowOwn_Allowed(t *testing.T) {
	const (
		ownerID   = "entity-owner-account"
		granterID = "manager-granter"
		targetID  = "role-cap-target"
	)

	t.Run("manager_to_manager", func(t *testing.T) {
		svc, repo := buildMMS(ownerID, map[string]string{
			granterID: "manager",
			targetID:  "member",
		})
		err := svc.UpdateMemberRole(repo.entity.GetID(), granterID, targetID, "manager")
		require.NoError(t, err, "a manager assigning manager must be allowed (manager50 >= manager50)")
		require.Equal(t, "manager", repo.members[targetID].role,
			"the allowed update must persist — target role must be manager")
	})

	t.Run("manager_to_member", func(t *testing.T) {
		svc, repo := buildMMS(ownerID, map[string]string{
			granterID: "manager",
			targetID:  "manager",
		})
		err := svc.UpdateMemberRole(repo.entity.GetID(), granterID, targetID, "member")
		require.NoError(t, err, "a manager assigning member must be allowed (manager50 >= member10)")
		require.Equal(t, "member", repo.members[targetID].role,
			"the allowed update must persist — target role must be member")
	})
}

// TestMemberManagementUpdateMemberRole_OwnerPromotesToOwner_Allowed: when the granter is the
// entity owner (entity.GetOwnerUserID() == requestingUserID) the cap is skipped, so
// promoting a member to owner is allowed. Green guard.
func TestMemberManagementUpdateMemberRole_OwnerPromotesToOwner_Allowed(t *testing.T) {
	const (
		ownerID  = "entity-owner-account"
		targetID = "role-cap-target"
	)

	svc, repo := buildMMS(ownerID, map[string]string{
		targetID: "member",
	})

	err := svc.UpdateMemberRole(repo.entity.GetID(), ownerID, targetID, "owner")

	require.NoError(t, err,
		"the entity owner must be able to promote a member to owner (owner short-circuit)")
	require.Equal(t, "owner", repo.members[targetID].role,
		"the owner-driven promotion must persist — target role must be owner")
}
