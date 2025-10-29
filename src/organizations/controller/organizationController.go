package controller

import (
	"net/http"
	"strconv"
	"strings"

	"soli/formations/src/auth/errors"
	authServices "soli/formations/src/auth/services"
	groupDto "soli/formations/src/groups/dto"
	groupServices "soli/formations/src/groups/services"
	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrganizationController struct {
	service       services.OrganizationService
	userService   authServices.UserService
	importService services.ImportService
	groupService  groupServices.GroupService
	db            *gorm.DB
}

func NewOrganizationController(service services.OrganizationService, importService services.ImportService, db *gorm.DB) *OrganizationController {
	return &OrganizationController{
		service:       service,
		userService:   authServices.NewUserService(),
		importService: importService,
		groupService:  groupServices.NewGroupService(db),
		db:            db,
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

// GetOrganizationGroups godoc
// @Summary Get organization groups
// @Description Get all groups belonging to a specific organization
// @Tags organizations
// @Accept json
// @Produce json
// @Param id path string true "Organization ID"
// @Param includes query string false "Relations to preload (e.g., 'Members,ParentGroup')"
// @Success 200 {array} groupDto.GroupOutput
// @Failure 400 {object} errors.APIError "Invalid organization ID"
// @Failure 404 {object} errors.APIError "Organization not found"
// @Failure 500 {object} errors.APIError "Internal server error"
// @Security BearerAuth
// @Router /organizations/{id}/groups [get]
func (oc *OrganizationController) GetOrganizationGroups(ctx *gin.Context) {
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

	// Check if organization exists
	org, err := oc.service.GetOrganization(orgID, false)
	if errors.HandleError(http.StatusInternalServerError, err, ctx) {
		return
	}
	if org == nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Organization not found",
		})
		return
	}

	// Parse includes parameter for selective preloading
	includeParam := ctx.Query("includes")
	var includes []string
	includeMembersInOutput := false
	if includeParam != "" {
		rawIncludes := strings.Split(includeParam, ",")
		for _, inc := range rawIncludes {
			trimmed := strings.TrimSpace(inc)
			if trimmed != "" {
				includes = append(includes, trimmed)
				if trimmed == "Members" {
					includeMembersInOutput = true
				}
			}
		}
	}

	// Get groups from service
	groups, err := oc.groupService.GetGroupsByOrganization(orgID, includes)
	if errors.HandleError(http.StatusInternalServerError, err, ctx) {
		return
	}

	// Convert to output DTOs
	groupsOutput := make([]groupDto.GroupOutput, 0, len(*groups))
	for i := range *groups {
		groupOutput := groupDto.GroupModelToGroupOutput(&(*groups)[i])

		// Only include member details if explicitly requested
		if !includeMembersInOutput {
			groupOutput.Members = nil
		}

		groupsOutput = append(groupsOutput, *groupOutput)
	}

	ctx.JSON(http.StatusOK, groupsOutput)
}

// ImportOrganizationData godoc
// @Summary Bulk import users, groups, and memberships
// @Description Import multiple users, groups, and memberships from CSV files into an organization
// @Tags organizations
// @Accept multipart/form-data
// @Produce json
// @Param id path string true "Organization ID"
// @Param users formData file true "Users CSV file (email,first_name,last_name,password,role,external_id,force_reset)"
// @Param groups formData file false "Groups CSV file (group_name,display_name,description,parent_group,max_members,expires_at,external_id)"
// @Param memberships formData file false "Memberships CSV file (user_email,group_name,role)"
// @Param dry_run formData boolean false "Validate only without persisting changes"
// @Param update_existing formData boolean false "Update existing users and groups"
// @Success 200 {object} dto.ImportOrganizationDataResponse
// @Failure 400 {object} errors.APIError "Invalid request"
// @Failure 403 {object} errors.APIError "Not authorized to manage this organization"
// @Failure 404 {object} errors.APIError "Organization not found"
// @Failure 500 {object} errors.APIError "Internal server error"
// @Security BearerAuth
// @Router /organizations/{id}/import [post]
func (oc *OrganizationController) ImportOrganizationData(ctx *gin.Context) {
	// Parse organization ID
	orgIDStr := ctx.Param("id")
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid organization ID",
		})
		return
	}

	// Get current user from context
	userID, exists := ctx.Get("userId")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, &errors.APIError{
			ErrorCode:    http.StatusUnauthorized,
			ErrorMessage: "User not authenticated",
		})
		return
	}

	// Check if user can manage this organization
	org, err := oc.service.GetOrganization(orgID, false)
	if errors.HandleError(http.StatusInternalServerError, err, ctx) {
		return
	}
	if org == nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Organization not found",
		})
		return
	}

	if !org.CanUserManageOrganization(userID.(string)) {
		ctx.JSON(http.StatusForbidden, &errors.APIError{
			ErrorCode:    http.StatusForbidden,
			ErrorMessage: "You are not authorized to import data into this organization",
		})
		return
	}

	// Parse multipart form
	usersFile, err := ctx.FormFile("users")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Users file is required",
		})
		return
	}

	// Optional files
	groupsFile, _ := ctx.FormFile("groups")
	membershipsFile, _ := ctx.FormFile("memberships")

	// Parse form parameters
	dryRunStr := ctx.DefaultPostForm("dry_run", "false")
	updateExistingStr := ctx.DefaultPostForm("update_existing", "false")

	// Convert string parameters to boolean
	dryRun, _ := strconv.ParseBool(dryRunStr)
	updateExisting, _ := strconv.ParseBool(updateExistingStr)

	// Call import service
	response, err := oc.importService.ImportOrganizationData(
		orgID,
		userID.(string),
		usersFile,
		groupsFile,
		membershipsFile,
		dryRun,
		updateExisting,
	)

	if err != nil {
		// If there are validation errors, return 400 with the error details
		if response != nil && len(response.Errors) > 0 {
			ctx.JSON(http.StatusBadRequest, response)
			return
		}

		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to import data: " + err.Error(),
		})
		return
	}

	// Return response with appropriate status code
	statusCode := http.StatusOK
	if !response.Success {
		statusCode = http.StatusBadRequest
	}

	ctx.JSON(statusCode, response)
}
