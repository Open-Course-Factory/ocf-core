package services

import (
	"errors"
	"fmt"

	"soli/formations/src/auth/access"
	"soli/formations/src/auth/casdoor"
	organizationModels "soli/formations/src/organizations/models"

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
// with the target's membership when it has one: the target must have a
// membership row in the organization (ErrMemberNotFound otherwise). Platform
// administrators may act on any member. Any other actor may not act on a
// platform administrator nor on an account that also belongs elsewhere (a nil
// alone check refuses); the owner may act on any other member, other actors
// only on plain members.
func newRankCheck(db *gorm.DB, alone OrganizationAloneChecker, org *organizationModels.Organization, actorID string) (func(targetID string) (*organizationModels.OrganizationMember, error), error) {
	actorIsAdmin, _ := isPlatformAdmin(actorID)
	actorIsOwner := actorID == org.OwnerUserID
	var actorRole organizationModels.OrganizationMemberRole
	if !actorIsAdmin && !actorIsOwner {
		actor, err := findMembership(db, org.ID, actorID)
		if err != nil && !errors.Is(err, ErrMemberNotFound) {
			return nil, err
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
		if alone == nil {
			return target, fmt.Errorf("%w: no check available", errBelongsElsewhere)
		}
		if err := alone.CheckBelongsToOrganizationAlone(org.ID, targetID); err != nil {
			return target, fmt.Errorf("%w: %v", errBelongsElsewhere, err)
		}
		return target, nil
	}, nil
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
