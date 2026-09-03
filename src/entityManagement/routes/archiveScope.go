package controller

import (
	ems "soli/formations/src/entityManagement/entityManagementService"

	"github.com/gin-gonic/gin"
)

// archiveReadScope reports whether the generic list must hide archived rows on
// this request: the entity is archivable and the caller did not ask for them
// with ?include_archived=true. Only the list consults it — get-by-id never
// hides an archived row, a teacher must still be able to open an archived
// class. Non-archivable entities ignore the parameter entirely.
func archiveReadScope(ctx *gin.Context, entityName string) bool {
	return ems.GlobalEntityRegistrationService.IsArchivable(entityName) &&
		ctx.Query("include_archived") != "true"
}
