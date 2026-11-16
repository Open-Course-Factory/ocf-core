package services

import (
	"errors"

	"soli/formations/src/auth/access"
	"soli/formations/src/auth/casdoor"
	organizationModels "soli/formations/src/organizations/models"

	"gorm.io/gorm"
)

var errOutranksActor = errors.New("outranks the caller; skipped")

// newRankCheck resolves the actor once and returns the role cap of the
// membership hooks for one target: the target must have a membership row in
// the organization (ErrMemberNotFound otherwise), and an actor may act only on
// members at or below their own rank. Platform administrators outrank every
// other actor and are not capped; the owner outranks every other organization role.
func newRankCheck(db *gorm.DB, org *organizationModels.Organization, actorID string) (func(targetID string) error, error) {
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

	return func(targetID string) error {
		target, err := findMembership(db, org.ID, targetID)
		if err != nil {
			return err
		}
		if actorIsAdmin {
			return nil
		}
		// Fails closed: an unknown platform role for the target counts as administrator.
		if targetIsAdmin, err := isPlatformAdmin(targetID); err != nil || targetIsAdmin {
			return errOutranksActor
		}
		if actorIsOwner {
			return nil
		}
		if targetID == org.OwnerUserID || actorRole == "" {
			return errOutranksActor
		}
		if !access.IsRoleAtLeast(string(actorRole), string(target.Role)) {
			return errOutranksActor
		}
		return nil
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
