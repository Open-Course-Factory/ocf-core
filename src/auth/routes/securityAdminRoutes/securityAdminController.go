package securityAdminRoutes

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"soli/formations/src/auth/interfaces"
	services "soli/formations/src/auth/services"

	"gorm.io/gorm"
)

type SecurityAdminController struct {
	service *services.SecurityAdminService
}

func NewSecurityAdminController(enforcer interfaces.EnforcerInterface, db *gorm.DB) *SecurityAdminController {
	return &SecurityAdminController{
		service: services.NewSecurityAdminService(enforcer, db),
	}
}

// GetPolicyOverview godoc
// @Summary Get all Casbin policies grouped by subject type
// @Tags Security Admin
// @Security Bearer
// @Success 200 {object} dto.PolicyOverviewOutput
// @Router /api/v1/admin/security/policies [get]
func (c *SecurityAdminController) GetPolicyOverview(ctx *gin.Context) {
	result, err := c.service.GetPolicyOverview()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, result)
}

// GetUserPermissionLookup godoc
// @Summary Get full permission set for a specific user
// @Tags Security Admin
// @Security Bearer
// @Param userId query string true "User ID to look up"
// @Success 200 {object} dto.UserPermissionsOutput
// @Router /api/v1/admin/security/user-permissions [get]
func (c *SecurityAdminController) GetUserPermissionLookup(ctx *gin.Context) {
	userID := ctx.Query("userId")
	if userID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "userId query parameter is required"})
		return
	}
	result, err := c.service.GetUserPermissionLookup(userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, result)
}

// GetEntityRoleMatrix godoc
// @Summary Get role-to-method mapping for all registered entities
// @Tags Security Admin
// @Security Bearer
// @Success 200 {object} dto.EntityRoleMatrixOutput
// @Router /api/v1/admin/security/entity-roles [get]
func (c *SecurityAdminController) GetEntityRoleMatrix(ctx *gin.Context) {
	result, err := c.service.GetEntityRoleMatrix()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, result)
}

// GetPolicyHealthChecks godoc
// @Summary Analyze policies for potential security issues
// @Tags Security Admin
// @Security Bearer
// @Success 200 {object} dto.PolicyHealthCheckOutput
// @Router /api/v1/admin/security/health-checks [get]
func (c *SecurityAdminController) GetPolicyHealthChecks(ctx *gin.Context) {
	result, err := c.service.GetPolicyHealthChecks()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, result)
}
