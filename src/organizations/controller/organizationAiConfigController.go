package controller

import (
	goerrors "errors"
	"net/http"

	"soli/formations/src/auth/errors"
	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/services"
	"soli/formations/src/utils"

	"github.com/gin-gonic/gin"
)

// GetAiConfig godoc
// @Summary Get an organization's AI assistant configuration
// @Description Returns whether the org's bring-your-own-key AI-assisted scenario authoring is enabled and configured. Never returns the API key itself — has_api_key only says one is set.
// @Tags organizations
// @Produce json
// @Param id path string true "Organization ID"
// @Success 200 {object} dto.AiConfigOutput
// @Failure 400 {object} errors.APIError "Invalid organization ID"
// @Failure 500 {object} errors.APIError "Internal server error"
// @Security BearerAuth
// @Router /organizations/{id}/ai-config [get]
func (oc *OrganizationController) GetAiConfig(ctx *gin.Context) {
	orgID, ok := parseOrganizationID(ctx)
	if !ok {
		return
	}
	out, err := services.NewOrganizationAiService(oc.db).GetConfig(orgID)
	if err != nil {
		utils.Error("Failed to load AI config for organization %s: %v", orgID, err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to load AI configuration")
		return
	}
	ctx.JSON(http.StatusOK, out)
}

// UpdateAiConfig godoc
// @Summary Update an organization's AI assistant configuration
// @Description Sets the endpoint, model and on/off switch for the org's bring-your-own-key AI-assisted scenario authoring. api_key is optional: omit it to keep the stored key, or send "" to clear it — the frontend never holds the real value to send back. Organization managers and owners only.
// @Tags organizations
// @Accept json
// @Produce json
// @Param id path string true "Organization ID"
// @Param input body dto.UpdateAiConfigInput true "AI configuration"
// @Success 200 {object} dto.AiConfigOutput
// @Failure 400 {object} errors.APIError "Invalid organization ID or request body"
// @Failure 500 {object} errors.APIError "Internal server error"
// @Security BearerAuth
// @Router /organizations/{id}/ai-config [put]
func (oc *OrganizationController) UpdateAiConfig(ctx *gin.Context) {
	orgID, ok := parseOrganizationID(ctx)
	if !ok {
		return
	}
	var input dto.UpdateAiConfigInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		errors.Respond(ctx, http.StatusBadRequest, "Invalid request body")
		return
	}
	out, err := services.NewOrganizationAiService(oc.db).UpdateConfig(orgID, input)
	if err != nil {
		utils.Error("Failed to update AI config for organization %s: %v", orgID, err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to update AI configuration")
		return
	}
	ctx.JSON(http.StatusOK, out)
}

// GenerateWithAi godoc
// @Summary Generate a scenario answer with the organization's configured AI assistant
// @Description Sends the given prompt to the organization's configured OpenAI-compatible endpoint and returns the raw completion text — the same text a teacher would otherwise paste in manually. Refuses with 409 if the organization has no usable configuration.
// @Tags organizations
// @Accept json
// @Produce json
// @Param id path string true "Organization ID"
// @Param input body dto.GenerateInput true "Prompt"
// @Success 200 {object} dto.GenerateOutput
// @Failure 400 {object} errors.APIError "Invalid organization ID or missing prompt"
// @Failure 409 {object} errors.APIError "AI assistant not configured"
// @Failure 502 {object} errors.APIError "The configured endpoint failed or returned nothing usable"
// @Security BearerAuth
// @Router /organizations/{id}/ai-config/generate [post]
func (oc *OrganizationController) GenerateWithAi(ctx *gin.Context) {
	orgID, ok := parseOrganizationID(ctx)
	if !ok {
		return
	}
	var input dto.GenerateInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		errors.Respond(ctx, http.StatusBadRequest, "prompt is required")
		return
	}

	answer, err := services.NewOrganizationAiService(oc.db).Generate(orgID, input.Prompt)
	if err != nil {
		if goerrors.Is(err, services.ErrAiNotConfigured) {
			errors.Respond(ctx, http.StatusConflict, err.Error())
			return
		}
		utils.Error("AI generate failed for organization %s: %v", orgID, err)
		errors.Respond(ctx, http.StatusBadGateway, "The AI assistant did not answer — check the organization's configured endpoint and key")
		return
	}
	ctx.JSON(http.StatusOK, dto.GenerateOutput{Answer: answer})
}
