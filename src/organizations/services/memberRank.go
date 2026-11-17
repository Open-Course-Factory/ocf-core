package services

import (
	"errors"
	"fmt"

	"soli/formations/src/auth/access"
	"soli/formations/src/auth/casdoor"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	errOutranksActor    = errors.New("outranks the caller; skipped")
	errBelongsElsewhere = errors.New("also belongs elsewhere; skipped")
	errNotCreatedHere   = errors.New("not created by this organization; skipped")
)

// OrganizationAloneChecker refuses an account still tied to anything outside
// orgID. auth/services' UserDeletionService satisfies it.
type OrganizationAloneChecker interface {
	CheckBelongsToOrganizationAlone(orgID uuid.UUID, userID string) error
}

// newRankCheck resolves the actor once and returns the check for one target,
// with the target's membership when it has one: the rank rule of
// newRankOnlyCheck, then, for an actor who is not a platform administrator, the
// organization must have created the account (errNotCreatedHere otherwise) and
// the target must belong to this organization alone (errBelongsElsewhere otherwise).
func newRankCheck(db *gorm.DB, alone OrganizationAloneChecker, org *organizationModels.Organization, actorID string) (func(targetID string) (*organizationModels.OrganizationMember, error), error) {
	checkRank, actorIsAdmin, err := newRankOnlyCheck(db, org, actorID)
	if err != nil {
		return nil, err
	}
	return func(targetID string) (*organizationModels.OrganizationMember, error) {
		target, err := checkRank(targetID)
		if err != nil || actorIsAdmin {
			return target, err
		}
		if !target.AccountCreatedByOrganization {
			return target, errNotCreatedHere
		}
		if !belongsToOrganizationAlone(alone, org.ID, targetID) {
			return target, errBelongsElsewhere
		}
		return target, nil
	}, nil
}

// newRankOnlyCheck resolves the actor once and returns the rank check for one
// target, and whether the actor is a platform administrator: the target must
// have a membership row in the organization (ErrMemberNotFound otherwise).
// Platform administrators may act on any member. Any other actor may not act on
// a platform administrator; the owner may act on any other member, other actors
// only on plain members (ReachPlainMembers).
func newRankOnlyCheck(db *gorm.DB, org *organizationModels.Organization, actorID string) (func(targetID string) (*organizationModels.OrganizationMember, error), bool, error) {
	actorIsAdmin, _ := isPlatformAdmin(actorID)
	// Offboarding, reinstating, password regeneration and import change an account's
	// access or credentials: they keep the narrow reach until the product widens it.
	check, err := rankCheckAs(db, org, actorID, actorIsAdmin, ReachPlainMembers)
	return check, actorIsAdmin, err
}

// MemberReach is how far below the actor the rank rule lets a non-owner act.
type MemberReach int

const (
	// ReachPlainMembers: a non-owner acts only on members holding the plain member role.
	ReachPlainMembers MemberReach = iota
	// ReachLowerRoles: a non-owner acts on any member whose role they strictly outrank.
	ReachLowerRoles
)

// CheckActorOutranksMember applies the rank rule of newRankOnlyCheck, with the
// given reach, to one target for a caller whose platform-administrator status
// the request already resolved (the generic member hooks pass their
// HookContext's answer). Returns ErrOutranksCaller when the target is out of
// the actor's reach.
func CheckActorOutranksMember(db *gorm.DB, org *organizationModels.Organization, actorID string, actorIsAdmin bool, targetID string, reach MemberReach) error {
	check, err := rankCheckAs(db, org, actorID, actorIsAdmin, reach)
	if err != nil {
		return err
	}
	if _, err := check(targetID); err != nil {
		if errors.Is(err, errOutranksActor) {
			return fmt.Errorf("%w: %s", ErrOutranksCaller, targetID)
		}
		return err
	}
	return nil
}

func rankCheckAs(db *gorm.DB, org *organizationModels.Organization, actorID string, actorIsAdmin bool, reach MemberReach) (func(targetID string) (*organizationModels.OrganizationMember, error), error) {
	actorIsOwner := actorID == org.OwnerUserID
	var actorRole organizationModels.OrganizationMemberRole
	if !actorIsAdmin && !actorIsOwner {
		actor, err := findMembership(db, org.ID, actorID)
		if err != nil && !errors.Is(err, ErrMemberNotFound) {
			return nil, err
		}
		// An inactive (offboarded or deactivated) row grants no reach.
		if actor != nil && actor.IsActive {
			actorRole = actor.Role
		}
	}

	return func(targetID string) (*organizationModels.OrganizationMember, error) {
		target, err := findMembership(db, org.ID, targetID)
		if err != nil {
			return nil, err
		}
		if actorIsAdmin {
			return target, nil
		}
		// Fails closed: an unknown platform role for the target counts as administrator.
		if targetIsAdmin, err := isPlatformAdmin(targetID); err != nil || targetIsAdmin {
			return target, errOutranksActor
		}
		if !actorIsOwner && !withinReach(actorRole, target.Role, reach) {
			return target, errOutranksActor
		}
		return target, nil
	}, nil
}

// withinReach reports whether a non-owner holding actorRole (empty without an
// active membership) may act on a member holding targetRole; an empty or
// unknown role reaches no one. The narrow reach also requires a manager, as
// every caller's own gate does.
func withinReach(actorRole, targetRole organizationModels.OrganizationMemberRole, reach MemberReach) bool {
	if !access.IsKnownRole(string(actorRole)) {
		return false
	}
	if reach == ReachPlainMembers {
		return targetRole == organizationModels.OrgRoleMember && access.IsRoleAtLeast(string(actorRole), access.RoleManager)
	}
	return access.IsKnownRole(string(targetRole)) &&
		access.RolePriority(string(actorRole)) > access.RolePriority(string(targetRole))
}

// belongsToOrganizationAlone reports whether nothing outside orgID holds the
// account; an unknown (nil check or failed lookup) reports false.
func belongsToOrganizationAlone(alone OrganizationAloneChecker, orgID uuid.UUID, userID string) bool {
	if alone == nil {
		utils.Warn("No organization-alone check wired; treating user %s in %s as belonging elsewhere", userID, orgID)
		return false
	}
	// The cause stays in the log: it describes another person's account.
	if err := alone.CheckBelongsToOrganizationAlone(orgID, userID); err != nil {
		utils.Debug("User %s belongs elsewhere than %s: %v", userID, orgID, err)
		return false
	}
	return true
}

// isPlatformAdmin resolves the caller's platform roles the way the JWT resolver does.
func isPlatformAdmin(userID string) (bool, error) {
	if casdoor.Enforcer == nil {
		return false, errors.New("role enforcer not initialised")
	}
	roles, err := casdoor.Enforcer.GetRolesForUser(userID)
	if err != nil {
		return false, err
	}
	return access.IsAdmin(roles), nil
}
