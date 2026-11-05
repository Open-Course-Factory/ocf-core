package terminalController

import (
	"log/slog"
	"net/http"
	"soli/formations/src/auth/casdoor"
	authMiddleware "soli/formations/src/auth/middleware"

	"soli/formations/src/auth/errors"
	controller "soli/formations/src/entityManagement/routes"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UserTerminalKeyController struct {
	controller.GenericController
	service services.TerminalTrainerService
	db      *gorm.DB
}

func NewUserTerminalKeyController(db *gorm.DB) *UserTerminalKeyController {
	return &UserTerminalKeyController{
		GenericController: controller.NewGenericController(db, casdoor.Enforcer),
		service:           services.NewTerminalTrainerService(db),
		db:                db,
	}
}

// DeleteEntity override pour gérer la suppression des clés avec hard delete
func (utc *UserTerminalKeyController) DeleteEntity(ctx *gin.Context) {
	utc.GenericController.DeleteEntity(ctx, false) // hard delete
}

// Regenerate Key godoc
//
//	@Summary		Régénérer une clé terminal
//	@Description	Régénère une nouvelle clé Terminal Trainer pour l'utilisateur
//	@Tags			user-terminal-keys
//	@Accept			json
//	@Produce		json
//
//	@Security		Bearer
//
//	@Success		200	{object}	string
//	@Failure		403	{object}	errors.APIError	"Access denied"
//	@Failure		409	{object}	errors.APIError	"The user still has running or paused terminals (reason=terminals_in_use)"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/user-terminal-keys/regenerate [post]
func (utc *UserTerminalKeyController) RegenerateKey(ctx *gin.Context) {
	// Rotating the key disables the old one: irreversible and self-scoped,
	// so never under an impersonated session.
	if authMiddleware.RefuseIfImpersonated(ctx, "Regenerating the terminal key") {
		return
	}
	userId := ctx.GetString("userId")

	// Vérifier si l'utilisateur a une clé existante
	existingKey, _ := utc.service.GetUserKey(userId)

	if existingKey != nil {
		if utc.refuseIfTerminalsInUse(ctx, userId) {
			return
		}

		// Stop here if tt-backend could not disable the old key: the row stays
		// active, so retrying regenerates it instead of leaving it working.
		if err := utc.service.DisableUserKey(userId); err != nil {
			slog.Error("failed to disable old terminal key", "user", userId, "err", err)
			errors.Respond(ctx, http.StatusInternalServerError, "Failed to disable old key, please try again")
			return
		}
	}

	// Créer une nouvelle clé
	// This will create the key in BOTH databases (Terminal Trainer + OCF Core)
	userName := "user-" + userId // Placeholder, à améliorer
	if err := utc.service.CreateUserKey(userId, userName); err != nil {
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to create new key: "+err.Error())
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Key regenerated successfully (auto-repaired if databases were out of sync)",
	})
}

// refuseIfTerminalsInUse answers the request and returns true when the user
// still has running or paused terminals. tt-backend only serves a session to the
// key that created it, so revoking that key would cut the user off from them.
func (utc *UserTerminalKeyController) refuseIfTerminalsInUse(ctx *gin.Context, userID string) bool {
	var liveTerminals int64
	err := utc.db.Model(&models.Terminal{}).Scopes(models.UsageListScope).
		Where("terminals.user_id = ?", userID).Count(&liveTerminals).Error
	if err != nil {
		slog.Error("failed to count live terminals", "user", userID, "err", err)
		errors.Respond(ctx, http.StatusInternalServerError, "Failed to check your terminals, please try again")
		return true
	}
	if liveTerminals == 0 {
		return false
	}
	ctx.JSON(http.StatusConflict, gin.H{
		"error_code":    http.StatusConflict,
		"error_message": "Delete your running and paused terminals before regenerating your key.",
		"reason":        "terminals_in_use",
	})
	return true
}

// Get My Key godoc
//
//	@Summary		Récupérer ma clé terminal
//	@Description	Récupère les informations de la clé Terminal Trainer de l'utilisateur connecté
//	@Tags			user-terminal-keys
//	@Accept			json
//	@Produce		json
//
//	@Security		Bearer
//
//	@Success		200	{object}	dto.UserTerminalKeyOutput
//	@Failure		404	{object}	errors.APIError	"Key not found"
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/user-terminal-keys/my-key [get]
func (utc *UserTerminalKeyController) GetMyKey(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	// Récupérer la clé de l'utilisateur
	userKey, err := utc.service.GetUserKey(userId)
	if err != nil {
		errors.Respond(ctx, http.StatusNotFound, "No terminal key found for user")
		return
	}

	// Convertir vers DTO (sans révéler la clé API pour des raisons de sécurité)
	keyOutput := dto.UserTerminalKeyOutput{
		ID:        userKey.ID,
		UserID:    userKey.UserID,
		KeyName:   userKey.KeyName,
		IsActive:  userKey.IsActive,
		CreatedAt: userKey.CreatedAt,
	}

	ctx.JSON(http.StatusOK, keyOutput)
}
