package permissionReferenceRoutes

import (
	"net/http"

	"github.com/gin-gonic/gin"

	access "soli/formations/src/auth/access"
)

// PermissionReferenceRoutes registers the public permission reference endpoint.
func PermissionReferenceRoutes(rg *gin.RouterGroup) {
	rg.GET("/permissions/reference", getPermissionReference)
}

// getPermissionReference returns the full permission reference built from
// declarative route registrations across all modules.
//
//	@Summary		Get permission reference
//	@Description	Returns all route permissions grouped by category, including platform roles and Layer 2 access rules. Available to all authenticated users.
//	@Tags			permissions
//	@Produce		json
//	@Security		Bearer
//	@Success		200	{object}	access.PermissionReference
//	@Router			/permissions/reference [get]
func getPermissionReference(ctx *gin.Context) {
	ref := access.RouteRegistry.GetReference()
	ctx.JSON(http.StatusOK, ref)
}
