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

// LearnerRoles is the SINGLE definition of which membership roles make someone
// an apprenant (issue #480), and the complement of ManagerRoles above.
//
// Teaching staff are on the roster like everyone else — the creator is
// auto-enrolled with the owner role by GroupOwnerSetupHook — so "active member"
// is NOT the population any learner-facing figure is about: how many apprenants
// a class has, how many of them are connected or idle, and who a teacher
// invigilates all filter on this list instead.
//
// The pair lives in one file on purpose: adding a role means deciding, right
// here, which side of the apprenant / staff line it falls on. A role that is on
// neither list disappears from both views.
var LearnerRoles = []GroupMemberRole{
	GroupMemberRoleMember,
}

// LearnerRoleScope narrows a query that already reads group_members under
// `alias` to learner-role memberships, so no call site spells the role list out.
// Raw-SQL callers, which no Scope can reach, bind LearnerRoles to their own
// `role IN ?` instead.
func LearnerRoleScope(alias string) func(*gorm.DB) *gorm.DB {
	return func(tx *gorm.DB) *gorm.DB {
		return tx.Where(alias+".role IN ?", LearnerRoles)
	}
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
