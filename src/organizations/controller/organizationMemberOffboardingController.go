package controller

import (
	goerrors "errors"
	"net/http"

	"soli/formations/src/auth/errors"
	authServices "soli/formations/src/auth/services"
	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/services"
	"soli/formations/src/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// OffboardMembers godoc
// @Summary Offboard organization members
// @Description Deactivates the memberships, blocks sign-in, terminates running terminals, releases assigned seats and schedules erasure after the organization's retention period. Class rosters are untouched.
// @Tags organizations
// @Accept json
// @Produce json
// @Param id path string true "Organization ID"
// @Param input body dto.OffboardMembersInput true "Members to offboard"
// @Success 204
// @Failure 400 {object} errors.APIError "Invalid organization ID or empty selection"
// @Failure 404 {object} errors.APIError "Member not found"
// @Failure 409 {object} errors.APIError "The organization owner cannot be offboarded"
// @Failure 500 {object} errors.APIError "Internal server error"
// @Security BearerAuth
// @Router /organizations/{id}/members/offboard [post]
func (oc *OrganizationController) OffboardMembers(ctx *gin.Context) {
	orgID, ok := parseOrganizationID(ctx)
	if !ok {
		return
	}
	var input dto.OffboardMembersInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{ErrorCode: http.StatusBadRequest, ErrorMessage: "user_ids is required"})
		return
	}

	err := oc.offboarding.Offboard(orgID, input.UserIDs, ctx.GetString("userId"))
	if err != nil {
		status, message := offboardingErrorResponse(err)
		if status == http.StatusInternalServerError {
			utils.Error("Offboarding in organization %s failed: %v", orgID, err)
		}
		ctx.JSON(status, &errors.APIError{ErrorCode: status, ErrorMessage: message})
		return
	}
	ctx.Status(http.StatusNoContent)
}

// ReinstateMember godoc
// @Summary Reinstate an offboarded member
// @Description Reactivates the membership, clears the erasure schedule and unblocks sign-in.
// @Tags organizations
// @Produce json
// @Param id path string true "Organization ID"
// @Param userId path string true "User ID"
// @Success 204
// @Failure 400 {object} errors.APIError "Invalid organization ID"
// @Failure 404 {object} errors.APIError "Member not found"
// @Failure 500 {object} errors.APIError "Internal server error"
// @Security BearerAuth
// @Router /organizations/{id}/members/{userId}/reinstate [post]
func (oc *OrganizationController) ReinstateMember(ctx *gin.Context) {
	orgID, ok := parseOrganizationID(ctx)
	if !ok {
		return
	}
	err := oc.offboarding.Reinstate(orgID, ctx.Param("userId"))
	if err != nil {
		status, message := offboardingErrorResponse(err)
		if status == http.StatusInternalServerError {
			utils.Error("Reinstating %s in organization %s failed: %v", ctx.Param("userId"), orgID, err)
		}
		ctx.JSON(status, &errors.APIError{ErrorCode: status, ErrorMessage: message})
		return
	}
	ctx.Status(http.StatusNoContent)
}

// EraseMember godoc
// @Summary Erase an offboarded member now
// @Description Runs the full account erasure ahead of the scheduled date. Refused while the user is still active in another organization, holds a personal paid subscription, or still owns organizations or groups.
// @Tags organizations
// @Produce json
// @Param id path string true "Organization ID"
// @Param userId path string true "User ID"
// @Success 204
// @Failure 400 {object} errors.APIError "Invalid organization ID"
// @Failure 404 {object} errors.APIError "Member not found"
// @Failure 409 {object} errors.APIError "Member is not offboarded, or erasure is blocked"
// @Failure 500 {object} errors.APIError "Internal server error"
// @Security BearerAuth
// @Router /organizations/{id}/members/{userId}/erase [post]
func (oc *OrganizationController) EraseMember(ctx *gin.Context) {
	orgID, ok := parseOrganizationID(ctx)
	if !ok {
		return
	}
	err := oc.offboarding.EraseNow(orgID, ctx.Param("userId"))
	if err != nil {
		status, message := offboardingErrorResponse(err)
		if status == http.StatusInternalServerError {
			utils.Error("Erasing %s from organization %s failed: %v", ctx.Param("userId"), orgID, err)
		}
		ctx.JSON(status, &errors.APIError{ErrorCode: status, ErrorMessage: message})
		return
	}
	ctx.Status(http.StatusNoContent)
}

// erasureBlockedReason is what the members list shows next to an offboarded
// member: the pre-flight the erasure would refuse with, or nothing.
func (oc *OrganizationController) erasureBlockedReason(orgID uuid.UUID, userID string) string {
	if err := oc.deletionService.CheckDepartedMemberErasable(orgID, userID); err != nil {
		return err.Error()
	}
	return ""
}

// offboardingErrorResponse maps the offboarding lifecycle errors to HTTP;
// erasure errors keep the mapping every erasure handler shares.
func offboardingErrorResponse(err error) (int, string) {
	switch {
	case goerrors.Is(err, services.ErrNoMembersSelected):
		return http.StatusBadRequest, err.Error()
	case goerrors.Is(err, services.ErrMemberNotFound):
		return http.StatusNotFound, "Organization member not found"
	case goerrors.Is(err, services.ErrCannotOffboardOwner), goerrors.Is(err, services.ErrMemberNotOffboarded):
		return http.StatusConflict, err.Error()
	default:
		status, message := authServices.ErasureErrorResponse(err)
		if status == http.StatusInternalServerError {
			message = "Member operation failed"
		}
		return status, message
	}
}

func parseOrganizationID(ctx *gin.Context) (uuid.UUID, bool) {
	orgID, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{ErrorCode: http.StatusBadRequest, ErrorMessage: "Invalid organization ID"})
		return uuid.Nil, false
	}
	return orgID, true
}
