package controller

import (
	"soli/formations/src/auth/access"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	"soli/formations/src/entityManagement/repositories/filters"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// mayReadItem applies the per-row rules of GET /<entity>/:id that need a query.
// mayReadItem holds GET /:id to the list's membership rule, so a known UUID
// does not reveal a row the list hides.
func (genericController genericController) mayReadItem(ctx *gin.Context, entityName string, id uuid.UUID, entity any) (bool, error) {
	config := ems.GlobalEntityRegistrationService.GetMembershipConfig(entityName)
	if config == nil {
		return true, nil
	}
	return genericController.callerIsMember(ctx, config, id, entity)
}

// callerIsMember asks the list's own filter about this one row, so a row can be
// read by ID exactly when the caller could find it in the list.
func (genericController genericController) callerIsMember(ctx *gin.Context, config *entityManagementInterfaces.MembershipConfig, id uuid.UUID, entity any) (bool, error) {
	if access.IsAdmin(ctx.GetStringSlice("userRoles")) {
		return true, nil
	}
	userID := ctx.GetString("userId")
	if userID == "" {
		return false, nil
	}
	statement := &gorm.Statement{DB: genericController.db}
	if err := statement.Parse(entity); err != nil {
		return false, err
	}
	table := statement.Table
	query := genericController.db.Table(table).Where(table+".id = ?", id)
	query = filters.NewGenericMembershipFilter(config).Apply(query, "user_member_id", userID, table)
	var matches int64
	err := query.Count(&matches).Error
	return matches > 0, err
}
