package userController

import (
	"log"
	"net/http"
	"strings"

	"soli/formations/src/auth/dto"
	"soli/formations/src/auth/errors"
	sqldb "soli/formations/src/db"
	ems "soli/formations/src/entityManagement/entityManagementService"
	groupDto "soli/formations/src/groups/dto"
	groupModels "soli/formations/src/groups/models"
	organizationDto "soli/formations/src/organizations/dto"
	organizationModels "soli/formations/src/organizations/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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
func (u UserController) GetUser(ctx *gin.Context) {
	userID := ctx.Param("id")
	if userID == "" {
		errors.Respond(ctx, http.StatusBadRequest, "User ID is required")
		return
	}

	// Handle special "me" ID - use authenticated user's ID from JWT token
	if userID == "me" {
		userID = ctx.GetString("userId")
		if userID == "" {
			errors.Respond(ctx, http.StatusUnauthorized, "User not authenticated")
			return
		}
	}

	user, userError := u.service.GetUserById(userID)
	if userError != nil {
		errors.Respond(ctx, http.StatusNotFound, userError.Error())
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
				if ops, ok := ems.GlobalEntityRegistrationService.GetEntityOps("OrganizationMember"); ok {
					memberOutputs := make([]organizationDto.OrganizationMemberOutput, 0)

					for _, membership := range orgMemberships {
						output, convErr := ops.ConvertModelToDto(membership)
						if convErr == nil {
							memberOutputs = append(memberOutputs, output.(organizationDto.OrganizationMemberOutput))
						}
					}

					extendedUser.OrganizationMemberships = memberOutputs
				}
			}
		}

		if include == "group_memberships" {
			groupMemberships, err := ActiveGroupMemberships(sqldb.DB, userID)
			if err != nil {
				log.Printf("[ERROR] Failed to load group memberships for user %s: %v", userID, err)
			}

			if err == nil {
				if ops, ok := ems.GlobalEntityRegistrationService.GetEntityOps("GroupMember"); ok {
					memberOutputs := make([]groupDto.GroupMemberOutput, 0)

					for _, membership := range groupMemberships {
						output, convErr := ops.ConvertModelToDto(membership)
						if convErr == nil {
							memberOutputs = append(memberOutputs, output.(groupDto.GroupMemberOutput))
						}
					}

					extendedUser.GroupMemberships = memberOutputs
				}
			}
		}
	}

	ctx.JSON(http.StatusOK, extendedUser)
}

// ActiveGroupMemberships loads the user's active class memberships with their
// class. The relation is GroupMember.Group: preloading a "ClassGroup" relation
// that does not exist failed the whole query, and the include came back empty,
// so the front believed nobody managed any class.
func ActiveGroupMemberships(db *gorm.DB, userID string) ([]groupModels.GroupMember, error) {
	var memberships []groupModels.GroupMember
	err := db.Where("user_id = ? AND is_active = ?", userID, true).
		Preload("Group").
		Find(&memberships).Error
	return memberships, err
}
