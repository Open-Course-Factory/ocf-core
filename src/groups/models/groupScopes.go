package models

import (
	"gorm.io/gorm"
)

// ManagerRoles are the group_members roles that carry authority over a class-group.
// Exported so every surface asking "is this caller a manager+ of that group?" spells
// the role set the same way instead of re-listing the constants.
var ManagerRoles = []GroupMemberRole{
	GroupMemberRoleOwner,
	GroupMemberRoleManager,
}

// ManagedByScope is the single home of the "callerUserID manages this class-group"
// predicate as a composable WHERE clause on class_groups: the caller either OWNS
// the group (ClassGroup.OwnerUserID) or holds an ACTIVE manager/owner membership
// row. A plain member never manages.
//
// The scope deliberately says nothing about the group's own is_active flag, so
// the two questions stay separable:
//
//   - AUTHORITY ("may this caller act on the group?") ANDs it with
//     `is_active = true` — an archived class grants nothing.
//   - LISTING ("which classes are mine?") uses it bare, so a teacher still sees
//     the classes they archived, flagged rather than vanished.
//
// Mirrors the SSOT pattern of terminalTrainer/models' OccupiesSlotScope and
// RunningDisplayScope: the rule is expressed once, callers compose the rest.
// Columns are qualified with the `class_groups.` prefix so the scope survives
// JOINs against tables sharing these column names, and the whole condition is
// parenthesised so an OR can never leak past an AND added by the caller.
//
// Usage:
//
//	db.Model(&models.ClassGroup{}).Scopes(models.ManagedByScope(userID)).Find(&groups)
//	db.Model(&models.ClassGroup{}).Scopes(models.ManagedByScope(userID)).
//	    Where("class_groups.is_active = ?", true).Pluck("id", &ids)
func ManagedByScope(callerUserID string) func(*gorm.DB) *gorm.DB {
	return func(tx *gorm.DB) *gorm.DB {
		return tx.Where(
			`(class_groups.owner_user_id = ? OR EXISTS (
				SELECT 1 FROM group_members gm
				WHERE gm.group_id = class_groups.id
				  AND gm.user_id = ?
				  AND gm.is_active = ?
				  AND gm.deleted_at IS NULL
				  AND gm.role IN ?))`,
			callerUserID, callerUserID, true, ManagerRoles,
		)
	}
}
