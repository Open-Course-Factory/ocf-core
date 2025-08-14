// src/courses/routes/courseRoutes/generationStatus.go
package courseController

import (
	"net/http"

	"soli/formations/src/auth/errors"

	"github.com/gin-gonic/gin"
)

// GetGenerationStatus godoc
//
//	@Summary		Statut d'une génération
//	@Description	Récupère le statut actuel d'une génération de cours
//	@Tags			generations
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID de génération"
//
//	@Security		Bearer
//
//	@Success		200	{object}	dto.GenerationStatusOutput
//
//	@Failure		400	{object}	errors.APIError	"ID invalide"
//	@Failure		404	{object}	errors.APIError	"Génération non trouvée"
//	@Router			/generations/{id}/status [get]
func (c courseController) GetGenerationStatus(ctx *gin.Context) {
	generationID := ctx.Param("id")

	if generationID == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "ID de génération requis",
		})
		return
	}

	status, err := c.service.CheckGenerationStatus(generationID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Génération non trouvée: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, status)
}

// DownloadGenerationResults godoc
//
//	@Summary		Téléchargement des résultats de génération
//	@Description	Télécharge les fichiers générés sous forme d'archive ZIP
//	@Tags			generations
//	@Accept			json
//	@Produce		application/zip
//	@Param			id	path	string	true	"ID de génération"
//
//	@Security		Bearer
//
//	@Success		200	{file}	application/zip
//
//	@Failure		400	{object}	errors.APIError	"ID invalide"
//	@Failure		404	{object}	errors.APIError	"Génération non trouvée"
//	@Failure		409	{object}	errors.APIError	"Génération non terminée"
//	@Router			/generations/{id}/download [get]
func (c courseController) DownloadGenerationResults(ctx *gin.Context) {
	generationID := ctx.Param("id")

	if generationID == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "ID de génération requis",
		})
		return
	}

	// Vérifier d'abord le statut
	status, err := c.service.CheckGenerationStatus(generationID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "Génération non trouvée: " + err.Error(),
		})
		return
	}

	if status.Status != "completed" {
		ctx.JSON(http.StatusConflict, &errors.APIError{
			ErrorCode:    http.StatusConflict,
			ErrorMessage: "La génération n'est pas terminée (statut: " + status.Status + ")",
		})
		return
	}

	// Télécharger les résultats
	zipData, err := c.service.DownloadGenerationResults(generationID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Erreur lors du téléchargement: " + err.Error(),
		})
		return
	}

	// Configurer les headers pour le téléchargement
	ctx.Header("Content-Type", "application/zip")
	ctx.Header("Content-Disposition", "attachment; filename=generation-"+generationID+".zip")
	ctx.Header("Content-Length", string(rune(len(zipData))))

	// Envoyer les données
	ctx.Data(http.StatusOK, "application/zip", zipData)
}

// RetryGeneration godoc
//
//	@Summary		Relancer une génération
//	@Description	Relance une génération qui a échoué
//	@Tags			generations
//	@Accept			json
//	@Produce		json
//	@Param			id	path	string	true	"ID de génération"
//
//	@Security		Bearer
//
//	@Success		202	{object}	dto.AsyncGenerationOutput
//
//	@Failure		400	{object}	errors.APIError	"ID invalide"
//	@Failure		404	{object}	errors.APIError	"Génération non trouvée"
//	@Failure		409	{object}	errors.APIError	"Génération en cours"
//	@Router			/generations/{id}/retry [post]
func (c courseController) RetryGeneration(ctx *gin.Context) {
	generationID := ctx.Param("id")

	if generationID == "" {
		ctx.JSON(http.StatusBadRequest, &errors.APIError{
			ErrorCode:    http.StatusBadRequest,
			ErrorMessage: "ID de génération requis",
		})
		return
	}

	result, err := c.service.RetryGeneration(generationID)
	if err != nil {
		// Déterminer le code d'erreur approprié
		statusCode := http.StatusInternalServerError
		if err.Error() == "generation is already in progress" {
			statusCode = http.StatusConflict
		} else if err.Error() == "failed to get generation: record not found" {
			statusCode = http.StatusNotFound
		}

		ctx.JSON(statusCode, &errors.APIError{
			ErrorCode:    statusCode,
			ErrorMessage: err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusAccepted, result)
}
