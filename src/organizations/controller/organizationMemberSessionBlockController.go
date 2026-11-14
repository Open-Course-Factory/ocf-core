package controller

import (
	"github.com/gin-gonic/gin"

	"soli/formations/src/terminalTrainer/httperrors"
	ttServices "soli/formations/src/terminalTrainer/services"
)

// SetMemberSessionBlock godoc
// @Summary Block a member from starting sessions in the organization
// @Description Blocks (or unblocks) every session start of the member in this organization. stop_running also stops their running sessions in it. Organization manager+ (or admin); the target must be an active member below manager, never the caller.
// @Tags organizations
// @Accept json
// @Produce json
// @Param id path string true "Organization ID"
// @Param userId path string true "User ID"
// @Param body body terminalDto.SetSessionBlockInput true "blocked, reason (500 characters max), stop_running"
// @Success 200 {object} map[string]any
// @Failure 400 {object} errors.APIError
// @Failure 403 {object} errors.APIError
// @Security BearerAuth
// @Router /organizations/{id}/members/{userId}/session-block [put]
func (oc *OrganizationController) SetMemberSessionBlock(ctx *gin.Context) {
	orgID, ok := parseOrganizationID(ctx)
	if !ok {
		return
	}
	actor := httperrors.ControlActorFrom(ctx)
	scope, err := ttServices.OrgControlScope(oc.db, orgID, actor, ctx.Param("userId"))
	if err != nil {
		httperrors.WriteParticipantControlError(ctx, err)
		return
	}
	input, ok := httperrors.BindSessionBlock(ctx)
	if !ok {
		return
	}
	results, err := oc.participants.SetSessionBlock(
		actor, ctx.Param("userId"), scope, *input.Blocked, input.Reason, input.StopRunning)
	httperrors.WriteSessionBlockResult(ctx, *input.Blocked, results, err)
}
