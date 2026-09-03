package terminalController

import (
	stderrors "errors"
	"net/http"

	"soli/formations/src/auth/errors"
	"soli/formations/src/terminalTrainer/dto"
	services "soli/formations/src/terminalTrainer/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CreateExposedPort godoc
//
//	@Summary		Expose a session port publicly
//	@Description	Publishes a port from inside the caller's running terminal session to a public URL served by the operator's Traefik instance. Opt-in feature: requires EXPOSE_DOMAIN/TRAEFIK_PROVIDER_SECRET configured AND the session's plan to have port_exposure_enabled.
//	@Tags			terminals
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string						true	"Terminal session ID"
//	@Param			body	body	dto.CreateExposedPortInput	true	"Port to expose"
//	@Security		Bearer
//	@Success		201	{object}	dto.ExposedPortResponse
//	@Failure		400	{object}	errors.APIError	"Invalid port or session not running"
//	@Failure		403	{object}	errors.APIError	"Plan does not allow port exposure"
//	@Router			/terminals/{id}/exposed-ports [post]
func (tc *terminalController) CreateExposedPort(ctx *gin.Context) {
	sessionID := ctx.Param("id")

	var input dto.CreateExposedPortInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid request body: " + err.Error(),
		})
		return
	}

	response, err := tc.service.CreateExposedPort(sessionID, input.Port)
	if err != nil {
		var planErr *services.PlanDisabledError
		if stderrors.As(err, &planErr) {
			ctx.JSON(http.StatusForbidden, &errors.APIError{
				ErrorCode:    http.StatusForbidden,
				ErrorMessage: err.Error(),
			})
			return
		}
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusCreated, response)
}

// ListExposedPorts godoc
//
//	@Summary		List a session's exposed ports
//	@Tags			terminals
//	@Produce		json
//	@Param			id	path	string	true	"Terminal session ID"
//	@Security		Bearer
//	@Success		200	{array}	dto.ExposedPortResponse
//	@Router			/terminals/{id}/exposed-ports [get]
func (tc *terminalController) ListExposedPorts(ctx *gin.Context) {
	sessionID := ctx.Param("id")

	response, err := tc.service.ListExposedPorts(sessionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, response)
}

// DeleteExposedPort godoc
//
//	@Summary		Stop publicly exposing a session port
//	@Tags			terminals
//	@Produce		json
//	@Param			id		path	string	true	"Terminal session ID"
//	@Param			portId	path	string	true	"Exposed port ID"
//	@Security		Bearer
//	@Success		200	{object}	gin.H
//	@Failure		400	{object}	errors.APIError	"Invalid exposed port ID"
//	@Failure		404	{object}	errors.APIError	"Exposed port not found"
//	@Router			/terminals/{id}/exposed-ports/{portId} [delete]
func (tc *terminalController) DeleteExposedPort(ctx *gin.Context) {
	sessionID := ctx.Param("id")

	portID, err := uuid.Parse(ctx.Param("portId"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "Invalid exposed port ID",
		})
		return
	}

	if err := tc.service.DeleteExposedPort(sessionID, portID); err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Exposed port deleted successfully"})
}
