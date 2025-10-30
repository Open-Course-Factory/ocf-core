package terminalController

import (
	"net/http"
	"soli/formations/src/auth/casdoor"

	"soli/formations/src/auth/errors"
	controller "soli/formations/src/entityManagement/routes"
	"soli/formations/src/terminalTrainer/dto"
	"soli/formations/src/terminalTrainer/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UserTerminalKeyController interface {
	// Méthodes génériques (héritées)
	AddEntity(ctx *gin.Context)
	EditEntity(ctx *gin.Context)
	DeleteEntity(ctx *gin.Context)
	GetEntities(ctx *gin.Context)
	GetEntity(ctx *gin.Context)

	// Méthodes spécialisées
	RegenerateKey(ctx *gin.Context)
	GetMyKey(ctx *gin.Context)
}

type userTerminalKeyController struct {
	controller.GenericController
	service services.TerminalTrainerService
}

func NewUserTerminalKeyController(db *gorm.DB) UserTerminalKeyController {
	return &userTerminalKeyController{
		GenericController: controller.NewGenericController(db, casdoor.Enforcer),
		service:           services.NewTerminalTrainerService(db),
	}
}

// DeleteEntity override pour gérer la suppression des clés avec hard delete
func (utc *userTerminalKeyController) DeleteEntity(ctx *gin.Context) {
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
//	@Failure		500	{object}	errors.APIError	"Internal server error"
//	@Router			/user-terminal-keys/regenerate [post]
func (utc *userTerminalKeyController) RegenerateKey(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	// Vérifier si l'utilisateur a une clé existante
	existingKey, _ := utc.service.GetUserKey(userId)

	if existingKey != nil {
		if err := utc.service.DisableUserKey(userId); err != nil {
			ctx.JSON(http.StatusInternalServerError, &errors.APIError{
				ErrorCode:    http.StatusInternalServerError,
				ErrorMessage: "Failed to disable old key: " + err.Error(),
			})
			return
		}
	}

	// Créer une nouvelle clé
	// TODO: Récupérer le nom d'utilisateur depuis Casdoor
	userName := "user-" + userId // Placeholder, à améliorer
	if err := utc.service.CreateUserKey(userId, userName); err != nil {
		ctx.JSON(http.StatusInternalServerError, &errors.APIError{
			ErrorCode:    http.StatusInternalServerError,
			ErrorMessage: "Failed to create new key: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Key regenerated successfully",
	})
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
func (utc *userTerminalKeyController) GetMyKey(ctx *gin.Context) {
	userId := ctx.GetString("userId")

	// Récupérer la clé de l'utilisateur
	userKey, err := utc.service.GetUserKey(userId)
	if err != nil {
		ctx.JSON(http.StatusNotFound, &errors.APIError{
			ErrorCode:    http.StatusNotFound,
			ErrorMessage: "No terminal key found for user",
		})
		return
	}

	// Convertir vers DTO (sans révéler la clé API pour des raisons de sécurité)
	keyOutput := dto.UserTerminalKeyOutput{
		ID:          userKey.ID,
		UserID:      userKey.UserID,
		KeyName:     userKey.KeyName,
		IsActive:    userKey.IsActive,
		MaxSessions: userKey.MaxSessions,
		CreatedAt:   userKey.CreatedAt,
	}

	ctx.JSON(http.StatusOK, keyOutput)
}
