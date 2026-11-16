package services

import (
	"errors"

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
)

// OrganizationAloneChecker refuses an account still tied to anything outside
// orgID. auth/services' UserDeletionService satisfies it.
type OrganizationAloneChecker interface {
	CheckBelongsToOrganizationAlone(orgID uuid.UUID, userID string) error
}

// newRankCheck resolves the actor once and returns the check for one target,
// with the target's membership when it has one: the rank rule of
// newRankOnlyCheck, then, for an actor who is not a platform administrator, the
// target must belong to this organization alone (errBelongsElsewhere otherwise).
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
// only on plain members.
func newRankOnlyCheck(db *gorm.DB, org *organizationModels.Organization, actorID string) (func(targetID string) (*organizationModels.OrganizationMember, error), bool, error) {
	actorIsAdmin, _ := isPlatformAdmin(actorID)
	actorIsOwner := actorID == org.OwnerUserID
	var actorRole organizationModels.OrganizationMemberRole
	if !actorIsAdmin && !actorIsOwner {
		actor, err := findMembership(db, org.ID, actorID)
		if err != nil && !errors.Is(err, ErrMemberNotFound) {
			return nil, false, err
		}
		if actor != nil {
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
		if !actorIsOwner && (actorRole == "" || target.Role != organizationModels.OrgRoleMember) {
			return target, errOutranksActor
		}
		return target, nil
	}, actorIsAdmin, nil
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
