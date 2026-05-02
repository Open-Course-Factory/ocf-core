package routes

import (
	"net/http"

	"soli/formations/src/auth/errors"
	"soli/formations/src/groups/dto"
	groupModels "soli/formations/src/groups/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GroupController hosts custom (non-CRUD) group endpoints. CRUD is auto-handled
// by the entityManagement framework — only routes that don't fit the generic
// pattern live here.
type GroupController struct {
	db *gorm.DB
}

// NewGroupController returns a new GroupController bound to the given DB.
func NewGroupController(db *gorm.DB) *GroupController {
	return &GroupController{db: db}
}

// GetMyGroupMemberships godoc
// @Summary List the authenticated user's group memberships
// @Description Returns one entry per active group the caller belongs to, including the caller's role within that group. Used by the frontend to gate UI actions per-group without N round-trips.
// @Tags groups
// @Produce json
// @Success 200 {array} dto.MyGroupMembershipOutput
// @Failure 401 {object} errors.APIError "User not authenticated"
// @Failure 500 {object} errors.APIError "Internal server error"
// @Security BearerAuth
// @Router /groups/me/memberships [get]
func (gc *GroupController) GetMyGroupMemberships(ctx *gin.Context) {
	userID, exists := ctx.Get("userId")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, &errors.APIError{
			ErrorCode:    http.StatusUnauthorized,
			ErrorMessage: "User not authenticated",
		})
		return
	}

	uid, ok := userID.(string)
	if !ok || uid == "" {
		ctx.JSON(http.StatusUnauthorized, &errors.APIError{
			ErrorCode:    http.StatusUnauthorized,
			ErrorMessage: "User not authenticated",
		})
		return
	}

	var members []groupModels.GroupMember
	if err := gc.db.
		Where("user_id = ? AND is_active = ?", uid, true).
		Find(&members).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to load memberships",
		})
		return
	}

	output := make([]dto.MyGroupMembershipOutput, 0, len(members))
	for _, m := range members {
		output = append(output, dto.MyGroupMembershipOutput{
			GroupID: m.GroupID,
			Role:    m.Role,
		})
	}

	ctx.JSON(http.StatusOK, output)
}
