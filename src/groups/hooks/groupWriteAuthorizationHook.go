package groupHooks

import (
	"fmt"

	entityErrors "soli/formations/src/entityManagement/errors"
	"soli/formations/src/entityManagement/hooks"
	"soli/formations/src/groups/models"
	"soli/formations/src/groups/services"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GroupWriteAuthorizationHook decides WHO may modify, delete, archive or
// unarchive an existing group.
//
// It is the enforcement half of #459. The Casbin policy previously gave Member
// only GET and POST on class-groups, so a trainer could create a class and then
// never rename, deactivate or delete it — while the UI offered both buttons. The
// policy now includes PATCH and DELETE, and this hook is what keeps that from
// meaning "any member may edit any group".
//
// It has to be a hook rather than an OwnershipConfig: ownership config would
// derive an owner-only rule, which excludes the organization managers who
// legitimately administer their organization's classes. And it cannot be the
// built-in GroupRole rule either, since CheckGroupRole reads group_members alone
// and an org manager need not be a member of the class.
//
// The authority question therefore has one owner, GroupService.CanUserManageGroup,
// which already unites the three ways to hold it: the group's owner, a group
// manager, or a manager of the organization the group belongs to.
//
// Separate from GroupPlacementValidationHook on purpose: that one answers "may a
// group live in this organization", this one answers "may this user change this
// group". Same lifecycle event, two reasons to change.
type GroupWriteAuthorizationHook struct {
	db           *gorm.DB
	groupService services.GroupService
	enabled      bool
	priority     int
}

func NewGroupWriteAuthorizationHook(db *gorm.DB) hooks.Hook {
	return &GroupWriteAuthorizationHook{
		db:           db,
		groupService: services.NewGroupService(db),
		// Before GroupCleanupHook (10), which revokes member permissions on
		// delete: nothing should be torn down for a caller who was never allowed
		// to delete the group.
		priority: 5,
		enabled:  true,
	}
}

func (h *GroupWriteAuthorizationHook) GetName() string       { return "group_write_authorization" }
func (h *GroupWriteAuthorizationHook) GetEntityName() string { return "ClassGroup" }
func (h *GroupWriteAuthorizationHook) GetHookTypes() []hooks.HookType {
	// Archive and unarchive derive their Layer 2 rule from PATCH (SelfScoped),
	// so this hook IS their authority — the same one as for an edit.
	return []hooks.HookType{hooks.BeforeUpdate, hooks.BeforeDelete, hooks.BeforeArchive, hooks.BeforeUnarchive}
}
func (h *GroupWriteAuthorizationHook) IsEnabled() bool  { return h.enabled }
func (h *GroupWriteAuthorizationHook) GetPriority() int { return h.priority }

func (h *GroupWriteAuthorizationHook) Execute(ctx *hooks.HookContext) error {
	// No authenticated caller means this is not a user-facing write (seeding,
	// migrations, cleanup jobs). There is nobody to authorize.
	if ctx.UserID == "" {
		return nil
	}

	// Platform administrators operate the platform and hold no membership in a
	// customer's group.
	if ctx.IsAdmin() {
		return nil
	}

	groupID, err := h.targetGroupID(ctx)
	if err != nil {
		return err
	}

	canManage, err := h.groupService.CanUserManageGroup(groupID, ctx.UserID)
	if err != nil {
		return fmt.Errorf("failed to verify group permissions: %w", err)
	}
	if !canManage {
		// Structured so WrapHookError preserves the 403. A plain error would be
		// wrapped as a generic ENT007 and reach the client as a 500, which reads
		// as "we broke" rather than "you may not".
		return entityErrors.NewUnauthorizedError(ctx.UserID, "ClassGroup", string(ctx.HookType))
	}

	return nil
}

// targetGroupID resolves which group is being written.
//
// EntityID is set by the generic service for update, delete and archive. The
// delete and archive paths also carry the loaded entity, which is used as a fallback so the hook
// does not depend on one field being populated by a single code path.
func (h *GroupWriteAuthorizationHook) targetGroupID(ctx *hooks.HookContext) (uuid.UUID, error) {
	if id, ok := ctx.EntityID.(uuid.UUID); ok && id != uuid.Nil {
		return id, nil
	}
	if group, ok := ctx.NewEntity.(*models.ClassGroup); ok && group.ID != uuid.Nil {
		return group.ID, nil
	}
	if group, ok := ctx.OldEntity.(*models.ClassGroup); ok && group.ID != uuid.Nil {
		return group.ID, nil
	}
	// Fail closed: an unidentifiable target must not be treated as authorized.
	return uuid.Nil, fmt.Errorf("group_write_authorization: cannot determine which group is being modified")
}
