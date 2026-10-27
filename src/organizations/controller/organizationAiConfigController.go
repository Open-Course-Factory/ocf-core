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
// @Description Sets the endpoint, model and on/off switch for the org's bring-your-own-key AI-assisted scenario authoring. api_key is optional: omit it to keep the stored key, or send "" to clear it — the frontend never holds the real value to send back. The endpoint must be an https URL to a public host, and changing it requires a new api_key in the same request. Organization managers and owners only.
// @Tags organizations
// @Accept json
// @Produce json
// @Param id path string true "Organization ID"
// @Param input body dto.UpdateAiConfigInput true "AI configuration"
// @Success 200 {object} dto.AiConfigOutput
// @Failure 400 {object} errors.APIError "Invalid organization ID or request body, ai_endpoint_invalid, or ai_key_required (endpoint changed without a new key)"
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
		if goerrors.Is(err, services.ErrAiEndpointInvalid) {
			errors.Abort(ctx, http.StatusBadRequest, "ai_endpoint_invalid")
			return
		}
		if goerrors.Is(err, services.ErrAiKeyRequired) {
			errors.Abort(ctx, http.StatusBadRequest, "ai_key_required")
			return
		}
		utils.Error("Failed to update AI config for organization %s: %v", orgID, err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to update AI configuration")
		return
	}
	ctx.JSON(http.StatusOK, out)
}

// maxAiGenerateRequestBytes caps the generate body. The prompt carries the
// scenario being improved, which stays well under this.
const maxAiGenerateRequestBytes = 512 << 10

// GenerateWithAi godoc
// @Summary Generate a scenario answer with the organization's configured AI assistant
// @Description Sends the given prompt to the organization's configured OpenAI-compatible endpoint and returns the raw completion text — the same text a teacher would otherwise paste in manually. Organization managers and owners only. Refuses with 409 if the organization has no usable configuration.
// @Tags organizations
// @Accept json
// @Produce json
// @Param id path string true "Organization ID"
// @Param input body dto.GenerateInput true "Prompt"
// @Success 200 {object} dto.GenerateOutput
// @Failure 400 {object} errors.APIError "Invalid organization ID or missing prompt"
// @Failure 409 {object} errors.APIError "ai_not_configured"
// @Failure 413 {object} errors.APIError "ai_request_too_large"
// @Failure 502 {object} errors.APIError "ai_provider_error: the endpoint failed, timed out or answered too much"
// @Security BearerAuth
// @Router /organizations/{id}/ai-config/generate [post]
func (oc *OrganizationController) GenerateWithAi(ctx *gin.Context) {
	orgID, ok := parseOrganizationID(ctx)
	if !ok {
		return
	}
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxAiGenerateRequestBytes)
	var input dto.GenerateInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		var tooLarge *http.MaxBytesError
		if goerrors.As(err, &tooLarge) {
			errors.Abort(ctx, http.StatusRequestEntityTooLarge, "ai_request_too_large")
			return
		}
		errors.Respond(ctx, http.StatusBadRequest, "prompt is required")
		return
	}

	answer, err := services.NewOrganizationAiService(oc.db).Generate(orgID, input.Prompt)
	if err != nil {
		if goerrors.Is(err, services.ErrAiNotConfigured) {
			errors.Abort(ctx, http.StatusConflict, "ai_not_configured")
			return
		}
		// Safe to log: ErrAiProvider errors never carry the key or the request.
		utils.Error("AI generate failed for organization %s: %v", orgID, err)
		errors.Abort(ctx, http.StatusBadGateway, "ai_provider_error")
		return
	}
	ctx.JSON(http.StatusOK, dto.GenerateOutput{Answer: answer})
}
