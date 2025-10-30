package userController

import (
	"net/http"
	"strings"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"
	sqldb "soli/formations/src/db"
	groupDto "soli/formations/src/groups/dto"
	groupRegistration "soli/formations/src/groups/entityRegistration"
	groupModels "soli/formations/src/groups/models"
	organizationDto "soli/formations/src/organizations/dto"
	organizationRegistration "soli/formations/src/organizations/entityRegistration"
	organizationModels "soli/formations/src/organizations/models"

	"github.com/gin-gonic/gin"
)

// Get User godoc
//
//	@Summary		Récupérer un utilisateur
//	@Description	Récupère un utilisateur par son ID
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"User ID"
//	@Security		Bearer
//	@Success		200	{object}	dto.UserOutput
//	@Failure		400	{object}	errors.APIError	"Bad request"
//	@Failure		404	{object}	errors.APIError	"User not found"
//	@Router			/users/{id} [get]
func (u userController) GetUser(ctx *gin.Context) {
	userID := ctx.Param("id")
	if userID == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "User ID is required",
		})
		return
	}

	// Handle special "me" ID - use authenticated user's ID from JWT token
	if userID == "me" {
		userID = ctx.GetString("userId")
		if userID == "" {
			ctx.JSON(http.StatusUnauthorized, &errors.APIError{
				ErrorCode:    http.StatusUnauthorized,
				ErrorMessage: "User not authenticated",
			})
			return
		}
	}

	user, userError := u.service.GetUserById(userID)
	if userError != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: userError.Error(),
		})
		return
	}

	// Check if includes parameter is provided
	includesParam := ctx.Query("includes")
	if includesParam == "" {
		// No includes requested, return standard user output
		ctx.JSON(http.StatusOK, user)
		return
	}

	// Parse includes
	includes := strings.Split(includesParam, ",")
	extendedUser := dto.ExtendedUserOutput{
		UserOutput: *user,
	}

	// Load organization memberships if requested
	for _, include := range includes {
		include = strings.TrimSpace(include)

		if include == "organization_memberships" {
			var orgMemberships []organizationModels.OrganizationMember
			err := sqldb.DB.Where("user_id = ? AND is_active = ?", userID, true).
				Preload("Organization").
				Find(&orgMemberships).Error

			if err == nil {
				reg := organizationRegistration.OrganizationMemberRegistration{}
				memberOutputs := make([]organizationDto.OrganizationMemberOutput, 0)

				for _, membership := range orgMemberships {
					output, convErr := reg.EntityModelToEntityOutput(membership)
					if convErr == nil {
						memberOutputs = append(memberOutputs, output.(organizationDto.OrganizationMemberOutput))
					}
				}

				extendedUser.OrganizationMemberships = memberOutputs
			}
		}

		if include == "group_memberships" {
			var groupMemberships []groupModels.GroupMember
			err := sqldb.DB.Where("user_id = ? AND is_active = ?", userID, true).
				Preload("ClassGroup").
				Find(&groupMemberships).Error

			if err == nil {
				reg := groupRegistration.GroupMemberRegistration{}
				memberOutputs := make([]groupDto.GroupMemberOutput, 0)

				for _, membership := range groupMemberships {
					output, convErr := reg.EntityModelToEntityOutput(membership)
					if convErr == nil {
						memberOutputs = append(memberOutputs, output.(groupDto.GroupMemberOutput))
					}
				}

				extendedUser.GroupMemberships = memberOutputs
			}
		}
	}

	ctx.JSON(http.StatusOK, extendedUser)
}
