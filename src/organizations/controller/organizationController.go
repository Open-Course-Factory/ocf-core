package controller

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"soli/formations/src/auth/errors"
	authServices "soli/formations/src/auth/services"
	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/services"
)

type OrganizationController struct {
	service     services.OrganizationService
	userService authServices.UserService
}

func NewOrganizationController(service services.OrganizationService) *OrganizationController {
	return &OrganizationController{
		service:     service,
		userService: authServices.NewUserService(),
	}
}

// GetOrganizationMembers godoc
// @Summary Get organization members
// @Description Get all members of a specific organization
// @Tags organizations
// @Accept json
// @Produce json
// @Param id path string true "Organization ID"
// @Param include query string false "Relations to preload (e.g., 'User')"
// @Success 200 {array} dto.OrganizationMemberOutput
// @Failure 400 {object} errors.APIError "Invalid organization ID"
// @Failure 404 {object} errors.APIError "Organization not found"
// @Failure 500 {object} errors.APIError "Internal server error"
// @Security BearerAuth
// @Router /organizations/{id}/members [get]
func (oc *OrganizationController) GetOrganizationMembers(ctx *gin.Context) {
	// Parse organization ID from URL parameter
	orgIDStr := ctx.Param("id")
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid organization ID",
		})
		return
	}

	// Parse include parameter for selective preloading
	includeParam := ctx.Query("include")
	var includes []string
	includeUser := false

	if includeParam != "" {
		allIncludes := strings.Split(includeParam, ",")
		// Separate database relationships from Casdoor relationships
		for _, inc := range allIncludes {
			trimmed := strings.TrimSpace(inc)
			if trimmed == "User" {
				includeUser = true
			} else {
				includes = append(includes, trimmed)
			}
		}
	}

	// Get members from service (only with database relationships)
	members, err := oc.service.GetOrganizationMembers(orgID, includes)
	if errors.HandleError(http.StatusInternalServerError, err, ctx) {
		return
	}

	if members == nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Organization not found",
		})
		return
	}

	// Convert to output DTOs
	membersOutput := make([]dto.OrganizationMemberOutput, 0, len(*members))
	for _, member := range *members {
		memberOutput := dto.OrganizationMemberOutput{
			ID:             member.ID,
			OrganizationID: member.OrganizationID,
			UserID:         member.UserID,
			Role:           member.Role,
			InvitedBy:      member.InvitedBy,
			JoinedAt:       member.JoinedAt,
			IsActive:       member.IsActive,
			Metadata:       member.Metadata,
			CreatedAt:      member.CreatedAt,
			UpdatedAt:      member.UpdatedAt,
		}

		// Include Organization if it was loaded
		if member.Organization.ID != uuid.Nil {
			orgOutput := dto.OrganizationOutput{
				ID:          member.Organization.ID,
				Name:        member.Organization.Name,
				DisplayName: member.Organization.DisplayName,
				// Add other organization fields as needed
			}
			memberOutput.Organization = &orgOutput
		}

		membersOutput = append(membersOutput, memberOutput)
	}

	// Fetch user data from Casdoor if requested
	if includeUser && len(membersOutput) > 0 {
		// Collect unique user IDs
		userIDs := make([]string, 0, len(membersOutput))
		for _, member := range membersOutput {
			userIDs = append(userIDs, member.UserID)
		}

		// Fetch users from Casdoor via UserService
		users, err := oc.userService.GetUsersByIds(userIDs)
		if err == nil && users != nil {
			// Create a map for quick lookup
			userMap := make(map[string]*dto.UserSummary)
			for i := range *users {
				user := &(*users)[i]
				userMap[user.Id.String()] = &dto.UserSummary{
					ID:          user.Id,
					Name:        user.UserName,
					DisplayName: user.DisplayName,
					Email:       user.Email,
				}
			}

			// Populate user data in members
			for i := range membersOutput {
				if userData, exists := userMap[membersOutput[i].UserID]; exists {
					membersOutput[i].User = userData
				}
			}
		}
		// If Casdoor fetch fails, we just don't include user data (graceful degradation)
	}

	ctx.JSON(http.StatusOK, membersOutput)
}
